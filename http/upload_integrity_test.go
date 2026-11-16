package fbhttp

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"time"

	"github.com/filebrowser/filebrowser/v2/diskcache"
	"github.com/filebrowser/filebrowser/v2/settings"
	"github.com/filebrowser/filebrowser/v2/users"
	"github.com/spf13/afero"
)

type failedUploadReader struct{}

func (failedUploadReader) Read([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }

func TestUploadIntegrityInterruptedWritePreservesDestination(t *testing.T) {
	for _, existing := range []bool{false, true} {
		t.Run(fmt.Sprint(existing), func(t *testing.T) {
			root := t.TempDir()
			fs := afero.NewBasePathFs(afero.NewOsFs(), root)
			if existing {
				if err := afero.WriteFile(fs, "/target", []byte("original"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			_, err := writeFile(fs, "/target", io.MultiReader(strings.NewReader("partial"), failedUploadReader{}), 0600, 0700)
			if !errors.Is(err, io.ErrUnexpectedEOF) {
				t.Fatalf("write error = %v", err)
			}
			content, readErr := afero.ReadFile(fs, "/target")
			if existing && (readErr != nil || string(content) != "original") {
				t.Fatalf("destination changed: %q, %v", content, readErr)
			}
			if !existing && !os.IsNotExist(readErr) {
				t.Fatalf("failed write published a target: %q, %v", content, readErr)
			}
			entries, err := os.ReadDir(root)
			if err != nil {
				t.Fatal(err)
			}
			for _, entry := range entries {
				if strings.HasPrefix(entry.Name(), internalWritePrefix) {
					t.Errorf("temporary file leaked: %s", entry.Name())
				}
			}
		})
	}
}

func TestUploadIntegrityAtomicReplacement(t *testing.T) {
	fs := afero.NewBasePathFs(afero.NewOsFs(), t.TempDir())
	if err := afero.WriteFile(fs, "/target", []byte("original"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := writeFile(fs, "/target", strings.NewReader("replacement"), 0600, 0700); err != nil {
		t.Fatal(err)
	}
	content, err := afero.ReadFile(fs, "/target")
	if err != nil || string(content) != "replacement" {
		t.Fatalf("replacement = %q, %v", content, err)
	}
}

func TestUploadIntegrityFailedUploadPreservesDirectory(t *testing.T) {
	scope := t.TempDir()
	victim := filepath.Join(scope, "directory", "keep.txt")
	if err := os.MkdirAll(filepath.Dir(victim), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(victim, []byte("original"), 0600); err != nil {
		t.Fatal(err)
	}
	key := []byte("review-key")
	perm := users.Permissions{Create: true, Modify: true}
	st := scopedUserStorage(t, scope, perm, key)
	req := httptest.NewRequest(http.MethodPost, "/directory?override=true", strings.NewReader("data"))
	req.Header.Set("X-Auth", signToken(t, perm, key))
	rec := httptest.NewRecorder()
	handle(resourcePostHandler(diskcache.NewNoOp()), "", st, &settings.Server{}).ServeHTTP(rec, req)
	if _, err := os.Stat(victim); err != nil {
		t.Fatalf("upload returned %d and deleted existing directory contents despite Delete=false: %v", rec.Code, err)
	}
}

func TestUploadIntegrityInvalidTusLengthPreservesExistingFile(t *testing.T) {
	scope := t.TempDir()
	target := filepath.Join(scope, "keep.txt")
	if err := os.WriteFile(target, []byte("original"), 0600); err != nil {
		t.Fatal(err)
	}
	key := []byte("review-key")
	perm := users.Permissions{Create: true, Modify: true}
	st := scopedUserStorage(t, scope, perm, key)
	cache := newMemoryUploadCache()
	t.Cleanup(cache.Close)
	req := httptest.NewRequest(http.MethodPost, "/keep.txt?override=true", http.NoBody)
	req.Header.Set("X-Auth", signToken(t, perm, key))
	req.Header.Set("Upload-Length", "invalid")
	rec := httptest.NewRecorder()
	handle(tusPostHandler(cache), "", st, &settings.Server{}).ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("got %d, want 400", rec.Code)
	}
	content, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "original" {
		t.Fatalf("rejected TUS POST altered existing file: got %q", content)
	}
}

type reviewGatedBody struct {
	io.Reader
	once    sync.Once
	arrived chan<- struct{}
	release <-chan struct{}
}

func (r *reviewGatedBody) Read(p []byte) (int, error) {
	r.once.Do(func() { r.arrived <- struct{}{}; <-r.release })
	return r.Reader.Read(p)
}

func TestUploadIntegrityConcurrentTusEnforcesLength(t *testing.T) {
	scope := t.TempDir()
	key := []byte("review-key")
	perm := users.Permissions{Create: true, Modify: true}
	st := scopedUserStorage(t, scope, perm, key)
	token := signToken(t, perm, key)
	cache := newMemoryUploadCache()
	t.Cleanup(cache.Close)
	post := httptest.NewRequest(http.MethodPost, "/file.txt", http.NoBody)
	post.Header.Set("X-Auth", token)
	post.Header.Set("Upload-Length", "5")
	rec := httptest.NewRecorder()
	handle(tusPostHandler(cache), "", st, &settings.Server{}).ServeHTTP(rec, post)
	if rec.Code != http.StatusCreated {
		t.Fatalf("POST got %d", rec.Code)
	}
	uploadQuery := strings.SplitN(rec.Header().Get("Location"), "?", 2)[1]
	arrived := make(chan struct{}, 2)
	release := make(chan struct{})
	done := make(chan int, 2)
	handler := handle(tusPatchHandler(cache), "", st, &settings.Server{})
	for _, body := range []string{"hello", "world"} {
		go func(body string) {
			req := httptest.NewRequest(http.MethodPatch, "/file.txt?"+uploadQuery, &reviewGatedBody{Reader: bytes.NewBufferString(body), arrived: arrived, release: release})
			req.Header.Set("X-Auth", token)
			req.Header.Set("Content-Type", "application/offset+octet-stream")
			req.Header.Set("Upload-Offset", "0")
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, req)
			done <- response.Code
		}(body)
	}
	select {
	case <-arrived:
	case <-time.After(5 * time.Second):
		close(release)
		t.Fatal("first PATCH did not reach body reader")
	}
	// Give a competing request time to pass the size check while the first
	// body is blocked. A serialized implementation may keep it waiting here.
	select {
	case <-arrived:
	case <-time.After(time.Second):
	}
	close(release)
	first, second := <-done, <-done
	info, err := os.Stat(filepath.Join(scope, "file.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() > 5 {
		t.Fatalf("PATCH statuses %d/%d; declared 5 bytes but wrote %d", first, second, info.Size())
	}
}
