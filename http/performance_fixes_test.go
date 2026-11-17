package fbhttp

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"testing/fstest"
	"time"

	"github.com/filebrowser/filebrowser/v2/files"
	"github.com/filebrowser/filebrowser/v2/rules"
	"github.com/filebrowser/filebrowser/v2/settings"
	"github.com/filebrowser/filebrowser/v2/users"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/require"
)

func TestEncodingQualityAndJSON(t *testing.T) {
	for _, tc := range []struct{ header, want string }{
		{"br, gzip;q=0.5", "br"}, {"gzip;q=0, br;q=0.5", "br"}, {"gzip;q=0.9, br;q=0.2", "gzip"}, {"*;q=0.5, br;q=0", "gzip"}, {"gzip;q=0", ""}, {"*;q=0", "not-acceptable"}, {"br;q=0.2, identity;q=0.8", ""}, {"", ""},
	} {
		require.Equal(t, tc.want, chooseEncoding(tc.header, "br", "gzip"), tc.header)
	}
	r := httptest.NewRequest("GET", "/api/resources/", nil)
	r.Header.Set("Accept-Encoding", "gzip")
	w := httptest.NewRecorder()
	status, err := renderJSON(w, r, map[string]string{"name": strings.Repeat("abc", 1000)})
	require.NoError(t, err)
	require.Zero(t, status)
	require.Equal(t, "gzip", w.Header().Get("Content-Encoding"))
	require.Contains(t, w.Header().Get("Vary"), "Accept-Encoding")
	z, err := gzip.NewReader(w.Body)
	require.NoError(t, err)
	body, err := io.ReadAll(z)
	require.NoError(t, err)
	require.True(t, json.Valid(body))
	require.Less(t, w.Body.Len(), 200)
}

func TestSmallJSONRespectsIdentityExclusion(t *testing.T) {
	req := httptest.NewRequest("GET", "/api/resources/", nil)
	req.Header.Set("Accept-Encoding", "gzip, identity;q=0")
	w := httptest.NewRecorder()
	status, err := renderJSON(w, req, map[string]int{"value": 1})
	require.NoError(t, err)
	require.Zero(t, status)
	require.Equal(t, "gzip", w.Header().Get("Content-Encoding"))
	decoder, err := gzip.NewReader(w.Body)
	require.NoError(t, err)
	body, err := io.ReadAll(decoder)
	require.NoError(t, err)
	require.JSONEq(t, `{"value":1}`, string(body))
	req.Header.Set("Accept-Encoding", "*;q=0")
	w = httptest.NewRecorder()
	status, err = renderJSON(w, req, map[string]int{"value": 1})
	require.NoError(t, err)
	require.Equal(t, 406, status)
}

func TestStaticPrecompressedAndMutableCaching(t *testing.T) {
	st := scopedUserStorage(t, t.TempDir(), users.Permissions{}, []byte("key"))
	assets := fstest.MapFS{"assets/app-abcdefgh.js": {Data: []byte("original")}, "assets/app-abcdefgh.js.br": {Data: []byte("brotli")}, "assets/app-abcdefgh.js.gz": {Data: []byte("gzip")}, "custom.css": {Data: []byte("css")}}
	_, handler := getStaticHandlers(st, &settings.Server{}, assets)
	for _, tc := range []struct {
		encoding, body string
		status         int
	}{{"br", "brotli", 200}, {"gzip", "gzip", 200}, {"gzip;q=0", "original", 200}, {"*;q=0", "", 406}} {
		req := httptest.NewRequest("GET", "/static/assets/app-abcdefgh.js", nil)
		req.Header.Set("Accept-Encoding", tc.encoding)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		require.Equal(t, tc.status, w.Code)
		if tc.status == 200 {
			require.Equal(t, tc.body, w.Body.String())
			require.Contains(t, w.Header().Get("Cache-Control"), "immutable")
		}
	}
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest("GET", "/static/custom.css", nil))
	require.NotContains(t, w.Header().Get("Cache-Control"), "immutable")
}

func TestPreviewSharedWorkAndCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	release := make(chan struct{})
	started := make(chan struct{})
	var calls atomic.Int32
	create := func(ctx context.Context) ([]byte, error) {
		calls.Add(1)
		close(started)
		select {
		case <-release:
			return []byte("image"), nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	done1 := make(chan error, 1)
	go func() { _, err := sharedPreview(ctx, t.Name(), create); done1 <- err }()
	<-started
	done2 := make(chan error, 1)
	go func() {
		value, err := sharedPreview(context.Background(), t.Name(), create)
		if err == nil && string(value) != "image" {
			err = errors.New("bad result")
		}
		done2 <- err
	}()
	require.Eventually(t, func() bool {
		previewFlights.Lock()
		defer previewFlights.Unlock()
		return previewFlights.active[t.Name()].waiters == 2
	}, time.Second, time.Millisecond)
	cancel()
	require.ErrorIs(t, <-done1, context.Canceled)
	close(release)
	require.NoError(t, <-done2)
	require.EqualValues(t, 1, calls.Load())
	ctx2, cancel2 := context.WithCancel(context.Background())
	stopped := make(chan struct{})
	started2 := make(chan struct{})
	go func() {
		_, _ = sharedPreview(ctx2, t.Name()+"-cancel", func(work context.Context) ([]byte, error) {
			close(started2)
			<-work.Done()
			close(stopped)
			return nil, work.Err()
		})
	}()
	<-started2
	cancel2()
	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("abandoned preview did not stop")
	}
}

func TestResourcePagesMetadataAndConflictPaths(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(root, "nested"), 0700))
	for n := 0; n < 12; n++ {
		require.NoError(t, os.WriteFile(filepath.Join(root, fmt.Sprintf("file-%02d.txt", n)), []byte("hello"), 0600))
	}
	require.NoError(t, os.WriteFile(filepath.Join(root, "nested", "a #%.csv"), []byte("a,b\n1,2"), 0600))
	perm := users.Permissions{Download: true, Create: true}
	key := []byte("key")
	st := scopedUserStorage(t, root, perm, key)
	token := signToken(t, st, perm, key)
	handler := handle(resourceGetHandler, "/api/resources", st, &settings.Server{TypeDetectionByHeader: true})
	request := func(method, target, body string, h http.Handler) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, target, strings.NewReader(body))
		r.Header.Set("X-Auth", token)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	w := request("GET", "/api/resources/?limit=5&offset=5", "", handler)
	require.Equal(t, 200, w.Code)
	var listing files.FileInfo
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &listing))
	require.Len(t, listing.Items, 5)
	require.Equal(t, 13, listing.Total)
	require.Equal(t, 5, listing.Offset)
	w = request("GET", "/api/resources/file-00.txt?meta=true", "", handler)
	require.Equal(t, 200, w.Code)
	require.NotContains(t, w.Body.String(), "hello")
	checker := handle(resourceCheckHandler, "", st, &settings.Server{})
	w = request("POST", "/api/check-paths", `{"base":"/","paths":["nested/a #%.csv","missing/deep/file.txt"]}`, checker)
	require.Equal(t, 200, w.Code)
	require.Contains(t, w.Body.String(), "a #%.csv")
	require.NotContains(t, w.Body.String(), "missing")
	for _, relative := range []string{"../file-00.txt", "nested/../file-00.txt", "/file-00.txt", "nested\\file"} {
		body, _ := json.Marshal(map[string]interface{}{"base": "/", "paths": []string{relative}})
		require.Equal(t, 400, request("POST", "/api/check-paths", string(body), checker).Code)
	}
	searchHandler := handle(searchHandler, "/api/search", st, &settings.Server{})
	w = request("GET", "/api/search/?query=file-&limit=5", "", searchHandler)
	require.Equal(t, 200, w.Code)
	require.Equal(t, 6, bytes.Count(w.Body.Bytes(), []byte("\n")))
	require.Contains(t, w.Body.String(), `"nextCursor":5`)
	w = request("GET", "/api/search/?query=file-&limit=5&cursor=10", "", searchHandler)
	require.Equal(t, 3, bytes.Count(w.Body.Bytes(), []byte("\n")))
	require.Contains(t, w.Body.String(), `"nextCursor":0`)
}

func TestArchiveManifestCanceledAndDeduplicated(t *testing.T) {
	fs := afero.NewMemMapFs()
	require.NoError(t, afero.WriteFile(fs, "/dir/file.txt", []byte("hello"), 0600))
	d := &data{user: &users.User{Fs: fs}, server: &settings.Server{}, settings: &settings.Settings{}}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := archiveManifest(ctx, d, []string{"/dir"}, "/")
	require.ErrorIs(t, err, context.Canceled)
	entries, err := archiveManifest(context.Background(), d, []string{"/dir", "/dir/file.txt", "/dir"}, "/")
	require.NoError(t, err)
	require.Len(t, entries, 2)
	d.settings.Rules = []rules.Rule{{Path: "/dir", Allow: false}}
	require.True(t, d.CanSkipDir("/dir"))
	d.user.Rules = []rules.Rule{{Path: "/dir/file.txt", Allow: true}}
	require.False(t, d.CanSkipDir("/dir"))
}

type countedFs struct {
	afero.Fs
	bytes atomic.Int64
}
type countedFile struct {
	afero.File
	total *atomic.Int64
}

func (f *countedFs) Open(name string) (afero.File, error) {
	file, err := f.Fs.Open(name)
	if err != nil {
		return nil, err
	}
	return &countedFile{file, &f.bytes}, nil
}
func (f *countedFile) Read(b []byte) (int, error) {
	n, err := f.File.Read(b)
	f.total.Add(int64(n))
	return n, err
}

func TestChecksumSkipsUnusedContentRead(t *testing.T) {
	perm := users.Permissions{Download: true, Modify: true}
	key := []byte("key")
	st := scopedUserStorage(t, t.TempDir(), perm, key)
	fs := &countedFs{Fs: afero.NewMemMapFs()}
	body := bytes.Repeat([]byte("x"), 16384)
	require.NoError(t, afero.WriteFile(fs, "/large.txt", body, 0600))
	st.Users = &customFSUser{Store: st.Users, fs: fs, followExternal: true}
	token := signToken(t, st, perm, key)
	handler := handle(resourceGetHandler, "/api/resources", st, &settings.Server{})
	req := httptest.NewRequest("GET", "/api/resources/large.txt?checksum=sha256", nil)
	req.Header.Set("X-Auth", token)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	require.Equal(t, 200, w.Code)
	require.Equal(t, int64(len(body)+512), fs.bytes.Load())
	require.NotContains(t, w.Body.String(), strings.Repeat("x", 100))
}
