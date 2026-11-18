package fbhttp

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"image"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/filebrowser/filebrowser/v2/settings"
	"github.com/filebrowser/filebrowser/v2/share"
	"github.com/filebrowser/filebrowser/v2/users"
	"github.com/spf13/afero"
)

func downloadTestController(rate int64) *downloadController {
	return &downloadController{budgets: make(map[uint]*downloadBudget), resolve: func(uint) (int64, error) { return rate, nil }}
}

func TestDownloadBudgetBoundsAndInheritance(t *testing.T) {
	base := &settings.Settings{DownloadRate: 1024}
	for _, tc := range []struct {
		value *int64
		want  int64
	}{{nil, 1024}, {new(int64(0)), 0}, {new(int64(512)), 512}, {new(int64(2048)), 2048}} {
		if got := effectiveDownloadRate(base, &users.User{DownloadRate: tc.value}); got != tc.want {
			t.Fatalf("rate=%d want %d", got, tc.want)
		}
	}
	now := time.Now()
	for _, rate := range []int64{1, 16, 1024, 1024 * 1024, users.MaximumDownloadRate} {
		b := &downloadBudget{rate: rate, tokens: float64(downloadBurst(rate)), updated: now}
		var sent int64
		for step := 0; step <= 100; step++ {
			at := now.Add(time.Duration(step) * 10 * time.Millisecond)
			for caller := 0; caller < 10; caller++ {
				count, _ := b.grant(at, 32<<10)
				sent += int64(count)
			}
			bound := downloadBurst(rate) + rate*int64(step)/100
			if sent > bound {
				t.Fatalf("rate %d sent %d > %d at step %d", rate, sent, bound, step)
			}
		}
	}
}

func TestDownloadRegistryRetainsBudgetsAndFailsClosedAtCapacity(t *testing.T) {
	c := downloadTestController(1024)
	b, done, err := c.acquire(1, 1024)
	if err != nil {
		t.Fatal(err)
	}
	b.tokens = 0
	done()
	again, done, err := c.acquire(1, 1024)
	if err != nil || again != b || again.tokens != 0 {
		t.Fatal("reconnection minted another burst")
	}
	done()
	for id := uint(2); id <= maxDownloadBudgets; id++ {
		c.budgets[id] = &downloadBudget{refs: 1}
	}
	if _, _, err := c.acquire(maxDownloadBudgets+1, 1024); !errors.Is(err, errDownloadCapacity) {
		t.Fatal("registry did not fail closed")
	}
	c.budgets[1].idle = time.Now().Add(-2 * downloadBudgetIdle)
	if _, done, err := c.acquire(maxDownloadBudgets+1, 1024); err != nil {
		t.Fatal(err)
	} else {
		done()
	}
	u := downloadTestController(0)
	_, done, err = u.acquire(1, 0)
	if err != nil {
		t.Fatal(err)
	}
	done()
	if len(u.budgets) != 0 {
		t.Fatal("unlimited idle budget retained")
	}
}

func TestDownloadCancellationPolicyChangesAndCopySafety(t *testing.T) {
	c := downloadTestController(1)
	b, done, _ := c.acquire(1, 1)
	defer done()
	b.tokens = 0
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	w := &downloadWriter{ResponseWriter: httptest.NewRecorder(), controller: c, budget: b, ctx: ctx, userID: 1}
	if _, ok := interface{}(w).(io.ReaderFrom); ok {
		t.Fatal("ReaderFrom bypasses the limiter")
	}
	start := time.Now()
	if n, err := io.Copy(w, strings.NewReader("content")); n != 0 || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("cancel: %d %v", n, err)
	}
	if time.Since(start) > time.Second {
		t.Fatal("cancellation was not responsive")
	}
	w.ctx = context.Background()
	c.resolve = func(uint) (int64, error) { return 0, nil }
	b.refresh = time.Time{}
	if n, err := w.Write([]byte("now unlimited")); n != 13 || err != nil || b.rate != 0 {
		t.Fatalf("live policy: %d %v", n, err)
	}
	c.resolve = func(uint) (int64, error) { return 0, errors.New("database unavailable") }
	b.refresh = time.Time{}
	if n, err := w.Write([]byte("must not leak")); n != 0 || err == nil {
		t.Fatal("policy failure allowed traffic")
	}
}

type downloadWriteSignal struct {
	http.ResponseWriter
	once   *sync.Once
	signal chan struct{}
}

func (w *downloadWriteSignal) Unwrap() http.ResponseWriter { return w.ResponseWriter }
func (w *downloadWriteSignal) Write(body []byte) (int, error) {
	n, err := w.ResponseWriter.Write(body)
	if n > 0 {
		w.once.Do(func() { close(w.signal) })
	}
	return n, err
}

func TestDownloadLivePolicyRefreshOnActiveStream(t *testing.T) {
	st, handler := sessionFixture(t)
	u, _ := st.Users.Get("", false, uint(1))
	if err := afero.WriteFile(u.Fs, "/live.txt", bytes.Repeat([]byte("x"), 4096), 0600); err != nil {
		t.Fatal(err)
	}
	set, _ := st.Settings.Get()
	set.DownloadRate = 1024
	if err := st.Settings.Save(set); err != nil {
		t.Fatal(err)
	}
	token, _ := sessionLogin(t, handler)
	signal := make(chan struct{})
	var once sync.Once
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handler.ServeHTTP(&downloadWriteSignal{w, &once, signal}, r)
	}))
	defer server.Close()
	result := make(chan error, 1)
	start := time.Now()
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
		defer cancel()
		r, _ := http.NewRequestWithContext(ctx, "GET", server.URL+"/app/api/raw/live.txt", http.NoBody)
		r.Header.Set("X-Auth", token)
		res, err := server.Client().Do(r)
		if err != nil {
			result <- err
			return
		}
		defer res.Body.Close()
		data, err := io.ReadAll(res.Body)
		if err == nil && len(data) != 4096 {
			err = io.ErrUnexpectedEOF
		}
		result <- err
	}()
	select {
	case <-signal:
	case <-time.After(3 * time.Second):
		t.Fatal("download did not start")
	}
	set.DownloadRate = 0
	if err := st.Settings.Save(set); err != nil {
		t.Fatal(err)
	}
	if err := <-result; err != nil {
		t.Fatal(err)
	}
	if time.Since(start) > 3*time.Second {
		t.Fatal("active stream did not reload its policy")
	}
}

func TestVerySlowDownloadFlushesAndCancels(t *testing.T) {
	st, handler := sessionFixture(t)
	u, _ := st.Users.Get("", false, uint(1))
	if err := afero.WriteFile(u.Fs, "/slow.txt", []byte("ab"), 0600); err != nil {
		t.Fatal(err)
	}
	u.DownloadRate = new(int64(1))
	if err := st.Users.Update(u, "DownloadRate"); err != nil {
		t.Fatal(err)
	}
	token, _ := sessionLogin(t, handler)
	server := httptest.NewServer(handler)
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	r, _ := http.NewRequestWithContext(ctx, "GET", server.URL+"/app/api/raw/slow.txt", http.NoBody)
	r.Header.Set("X-Auth", token)
	start := time.Now()
	res, err := server.Client().Do(r)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	first := make([]byte, 1)
	if _, err := io.ReadFull(res.Body, first); err != nil || first[0] != 'a' {
		t.Fatalf("first byte: %q %v", first, err)
	}
	if time.Since(start) > 900*time.Millisecond {
		t.Fatal("paced bytes remained buffered instead of reaching the client")
	}
	cancel()
}

func TestDownloadPartialErrorsDoNotCorruptFiles(t *testing.T) {
	d := &data{user: &users.User{ID: 1, Perm: users.Permissions{Download: true}}}
	fn := withDownload(func(w http.ResponseWriter, _ *http.Request, _ *data) (int, error) {
		_, _ = w.Write([]byte("partial archive"))
		w.(http.Flusher).Flush()
		return http.StatusInternalServerError, io.ErrUnexpectedEOF
	})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = fn(w, r, d) }))
	defer server.Close()
	res, err := server.Client().Get(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	body, err := io.ReadAll(res.Body)
	if !errors.Is(err, io.ErrUnexpectedEOF) || string(body) != "partial archive" {
		t.Fatalf("partial download was not aborted cleanly: %q %v", body, err)
	}
}

func TestDownloadConcurrentUserBudget(t *testing.T) {
	c := downloadTestController(8 << 10)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	start := time.Now()
	var wg sync.WaitGroup
	for i := 0; i < 3; i++ {
		b, done, err := c.acquire(1, 8<<10)
		if err != nil {
			t.Fatal(err)
		}
		wg.Go(func() {
			defer done()
			w := &downloadWriter{ResponseWriter: httptest.NewRecorder(), controller: c, budget: b, ctx: ctx, userID: 1}
			if n, err := io.Copy(w, bytes.NewReader(make([]byte, 2048))); err != nil || n != 2048 {
				t.Errorf("copy: %d %v", n, err)
			}
		})
	}
	wg.Wait()
	if time.Since(start) < 450*time.Millisecond {
		t.Fatal("parallel downloads multiplied the rate")
	}
}

func TestDownloadPrivatePublicRangesArchivesAndContentAPIs(t *testing.T) {
	st, handler := sessionFixture(t)
	u, _ := st.Users.Get("", false, uint(1))
	u.Perm.Create, u.Perm.Share = true, true
	if err := st.Users.Update(u, "Perm"); err != nil {
		t.Fatal(err)
	}
	payload := make([]byte, 8192)
	if _, err := rand.Read(payload); err != nil {
		t.Fatal(err)
	}
	if err := afero.WriteFile(u.Fs, "/payload.bin", payload, 0600); err != nil {
		t.Fatal(err)
	}
	if err := afero.WriteFile(u.Fs, "/content.csv", []byte("a,b\n1,2\n"), 0600); err != nil {
		t.Fatal(err)
	}
	var picture bytes.Buffer
	if err := png.Encode(&picture, image.NewRGBA(image.Rect(0, 0, 1, 1))); err != nil {
		t.Fatal(err)
	}
	if err := afero.WriteFile(u.Fs, "/preview.png", picture.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	if err := afero.WriteFile(u.Fs, "/captions.vtt", []byte("WEBVTT\n\n00:00:00.000 --> 00:00:01.000\nCaption\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := st.Share.Save(&share.Link{Hash: "bandwidth", UserID: 1, Path: "/payload.bin"}); err != nil {
		t.Fatal(err)
	}
	if err := st.Share.Save(&share.Link{Hash: "folderbandwidth", UserID: 1, Path: "/"}); err != nil {
		t.Fatal(err)
	}
	set, _ := st.Settings.Get()
	set.DownloadRate = 8 << 10
	if err := st.Settings.Save(set); err != nil {
		t.Fatal(err)
	}
	token, _ := sessionLogin(t, handler)
	visitor := &users.User{Username: "visitor", Password: u.Password, Perm: users.Permissions{Download: true}, DownloadRate: new(int64(0))}
	if err := st.Users.Save(visitor); err != nil {
		t.Fatal(err)
	}
	login := sessionCall(handler, "POST", "login", `{"username":"visitor","password":"Original-password-42!"}`, map[string]string{"X-Session-Transport": "header"})
	if login.Code != 200 {
		t.Fatalf("visitor login: %d", login.Code)
	}
	server := httptest.NewServer(handler)
	defer server.Close()
	type result struct {
		name    string
		body    []byte
		status  int
		headers http.Header
		err     error
	}
	results := make(chan result, 3)
	requests := []struct{ name, path, token, byteRange string }{
		{"private", "/app/api/raw/payload.bin", token, "bytes=0-2047"},
		{"public", "/app/api/public/dl/bandwidth", login.Body.String(), "bytes=2048-4095"},
		{"archive", "/app/api/public/dl/folderbandwidth?files=payload.bin&algo=zip", "", ""},
	}
	start := time.Now()
	for _, entry := range requests {
		go func() {
			r, _ := http.NewRequest("GET", server.URL+entry.path, http.NoBody)
			r.Header.Set("X-Auth", entry.token)
			r.Header.Set("Range", entry.byteRange)
			res, err := server.Client().Do(r)
			if err != nil {
				results <- result{name: entry.name, err: err}
				return
			}
			defer res.Body.Close()
			body, err := io.ReadAll(res.Body)
			results <- result{entry.name, body, res.StatusCode, res.Header, err}
		}()
	}
	total := 0
	for i := 0; i < 3; i++ {
		res := <-results
		if res.err != nil {
			t.Fatal(res.err)
		}
		if res.headers.Get("Cache-Control") != "private, no-store" {
			t.Fatalf("%s cache: %v", res.name, res.headers)
		}
		total += len(res.body)
		if res.name == "archive" {
			if res.status != 200 {
				t.Fatalf("archive status: %d %s", res.status, res.body)
			}
			archive, err := zip.NewReader(bytes.NewReader(res.body), int64(len(res.body)))
			if err != nil {
				t.Fatal(err)
			}
			if len(archive.File) != 1 {
				t.Fatalf("single selected file produced %d archive entries", len(archive.File))
			}
			file, err := archive.File[0].Open()
			if err != nil {
				t.Fatal(err)
			}
			data, err := io.ReadAll(file)
			_ = file.Close()
			if err != nil || !bytes.Equal(data, payload) {
				t.Fatal("archive content changed")
			}
		} else if res.status != 206 || len(res.body) != 2048 || res.headers.Get("Content-Range") == "" {
			t.Fatalf("%s range: %d %v", res.name, res.status, res.headers)
		}
	}
	minimum := time.Duration(float64(total-int(downloadBurst(8<<10))) / float64(8<<10) * float64(time.Second))
	if time.Since(start)+30*time.Millisecond < minimum {
		t.Fatal("private/public/archive transfers did not share a budget")
	}
	// Content APIs cannot be used as an alternate unthrottled file route.
	for _, path := range []string{"resources/content.csv", "resources/content.csv?preview=true", "resources/content.csv?checksum=sha256"} {
		r := sessionCall(handler, "GET", path, "", map[string]string{"X-Auth": token, "X-Encoding": "true"})
		if r.Code != 200 || r.Header().Get("Cache-Control") != "private, no-store" || r.Body.String() != "a,b\n1,2\n" {
			t.Fatalf("content API bypass: %d %s", r.Code, r.Body.String())
		}
	}
	metadata := sessionCall(handler, "GET", "resources/?limit=10", "", map[string]string{"X-Auth": token})
	if metadata.Code != 200 || metadata.Header().Get("Cache-Control") == "private, no-store" {
		t.Fatal("directory metadata was throttled")
	}
	for _, path := range []string{"preview/thumb/preview.png", "subtitle/captions.vtt", "resources/content.csv"} {
		res := sessionCall(handler, "GET", path, "", map[string]string{"X-Auth": token})
		if res.Code != 200 || res.Body.Len() == 0 || res.Header().Get("Cache-Control") != "private, no-store" {
			t.Fatalf("unlimited content route %s: %d %s", path, res.Code, res.Body.String())
		}
	}
}

func TestDownloadSettingsAndUserPermissions(t *testing.T) {
	st, handler := sessionFixture(t)
	token, _ := sessionLogin(t, handler)
	headers := map[string]string{"X-Auth": token}
	for _, field := range []string{"DownloadRate", "downloadRate", "DOWNLOADRATE"} {
		body := `{"what":"user","which":["` + field + `"],"current_password":"Original-password-42!","data":{"id":1,"downloadRate":0}}`
		if res := sessionCall(handler, "PUT", "users/1", body, headers); res.Code != 403 {
			t.Fatalf("self speed update %s: %d %s", field, res.Code, res.Body.String())
		}
	}
	if res := sessionCall(handler, "PUT", "settings", `{"downloadRate":1024}`, headers); res.Code != 403 {
		t.Fatalf("non-admin global update: %d", res.Code)
	}
	u, _ := st.Users.Get("", false, uint(1))
	u.Perm.Admin = true
	if err := st.Users.Update(u, "Perm"); err != nil {
		t.Fatal(err)
	}
	get := sessionCall(handler, "GET", "settings", "", headers)
	var configuration settingsData
	if err := json.Unmarshal(get.Body.Bytes(), &configuration); err != nil {
		t.Fatal(err)
	}
	configuration.DownloadRate = 4096
	configuration.Defaults.DownloadRate = new(int64(2048))
	body, _ := json.Marshal(configuration)
	if res := sessionCall(handler, "PUT", "settings", string(body), headers); res.Code != 200 {
		t.Fatalf("global speed: %d %s", res.Code, res.Body.String())
	}
	saved, _ := st.Settings.Get()
	if saved.DownloadRate != 4096 || saved.Defaults.DownloadRate == nil || *saved.Defaults.DownloadRate != 2048 {
		t.Fatal("download settings did not persist")
	}
	newUser := &users.User{}
	saved.Defaults.Apply(newUser)
	if newUser.DownloadRate == saved.Defaults.DownloadRate || newUser.DownloadRate == nil || *newUser.DownloadRate != 2048 {
		t.Fatal("new user default was not copied")
	}
	for _, value := range []string{"8192", "0", "null"} {
		update := `{"what":"user","which":["DownloadRate"],"current_password":"Original-password-42!","data":{"id":1,"downloadRate":` + value + `}}`
		if res := sessionCall(handler, "PUT", "users/1", update, headers); res.Code != 200 {
			t.Fatalf("admin override %s: %d %s", value, res.Code, res.Body.String())
		}
		u, _ = st.Users.Get("", false, uint(1))
		if value == "null" && u.DownloadRate != nil {
			t.Fatal("inherit did not persist")
		}
		if value != "null" && u.DownloadRate == nil {
			t.Fatal("explicit override became inherit")
		}
	}
	for _, invalid := range []string{`{"downloadRate":-1}`, `{"downloadRate":1099511627777}`, `{"defaults":{"downloadRate":-1}}`} {
		if res := sessionCall(handler, "PUT", "settings", invalid, headers); res.Code != 400 {
			t.Fatalf("invalid global speed accepted: %d", res.Code)
		}
	}
	for _, value := range []string{"-1", "1099511627777"} {
		update := `{"what":"user","which":["DownloadRate"],"current_password":"Original-password-42!","data":{"id":1,"downloadRate":` + value + `}}`
		if res := sessionCall(handler, "PUT", "users/1", update, headers); res.Code != 400 {
			t.Fatalf("invalid user speed accepted: %d", res.Code)
		}
	}
}
