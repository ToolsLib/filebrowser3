package fbhttp

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/filebrowser/filebrowser/v2/settings"
	"github.com/filebrowser/filebrowser/v2/users"
	"github.com/spf13/afero"
)

func TestTusReplacementIsPrivateUntilComplete(t *testing.T) {
	scope := t.TempDir()
	target := filepath.Join(scope, "target")
	if err := os.WriteFile(target, []byte("original"), 0600); err != nil {
		t.Fatal(err)
	}
	key := []byte("staging-test-key")
	perm := users.Permissions{Create: true, Modify: true}
	st := scopedUserStorage(t, scope, perm, key)
	cache := newMemoryUploadCache()
	t.Cleanup(cache.Close)
	token := signToken(t, perm, key)
	request := func(method, target, body, offset string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, target, strings.NewReader(body))
		req.Header.Set("X-Auth", token)
		req.Header.Set("Upload-Length", "6")
		req.Header.Set("Upload-Offset", offset)
		req.Header.Set("Content-Type", "application/offset+octet-stream")
		rec := httptest.NewRecorder()
		var handler handleFunc
		switch method {
		case "POST":
			handler = tusPostHandler(cache)
		case "PATCH":
			handler = tusPatchHandler(cache)
		case "DELETE":
			handler = tusDeleteHandler(cache)
		}
		handle(handler, "", st, &settings.Server{}).ServeHTTP(rec, req)
		return rec
	}
	create := request("POST", "/target?override=true", "", "")
	if create.Code != 201 {
		t.Fatalf("create: %d %s", create.Code, create.Body.String())
	}
	location, err := url.Parse(create.Header().Get("Location"))
	if err != nil {
		t.Fatal(err)
	}
	upload := "/target?" + location.RawQuery
	if response := request("PATCH", upload, "new", "0"); response.Code != 204 {
		t.Fatalf("partial: %d %s", response.Code, response.Body.String())
	}
	if body, _ := os.ReadFile(target); string(body) != "original" {
		t.Fatalf("partial replaced original: %q", body)
	}
	if response := request("POST", "/target?override=true", "", ""); response.Code != 409 {
		t.Fatalf("concurrent create: %d", response.Code)
	}
	if response := request("DELETE", upload, "", ""); response.Code != 204 {
		t.Fatalf("cancel: %d", response.Code)
	}
	if body, _ := os.ReadFile(target); string(body) != "original" {
		t.Fatalf("cancel replaced original: %q", body)
	}
	create = request("POST", "/target?override=true", "", "")
	location, _ = url.Parse(create.Header().Get("Location"))
	upload = "/target?" + location.RawQuery
	if response := request("PATCH", upload, "newnew", "0"); response.Code != 204 {
		t.Fatalf("complete: %d %s", response.Code, response.Body.String())
	}
	if body, _ := os.ReadFile(target); string(body) != "newnew" {
		t.Fatalf("complete = %q", body)
	}
	entries, err := os.ReadDir(scope)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("staging files left after completion: %v", entries)
	}
}

type failingRegistrationCache struct{ UploadCache }

func (failingRegistrationCache) Register(string, UploadState, func() error) error {
	return errors.New("cache unavailable")
}

func TestTusRegistrationFailurePreservesDestination(t *testing.T) {
	scope := t.TempDir()
	if err := os.WriteFile(filepath.Join(scope, "target"), []byte("original"), 0600); err != nil {
		t.Fatal(err)
	}
	key := []byte("test-key")
	perm := users.Permissions{Create: true, Modify: true}
	st := scopedUserStorage(t, scope, perm, key)
	request := httptest.NewRequest("POST", "/target?override=true", http.NoBody)
	request.Header.Set("X-Auth", signToken(t, perm, key))
	request.Header.Set("Upload-Length", "6")
	rec := httptest.NewRecorder()
	handle(tusPostHandler(failingRegistrationCache{}), "", st, &settings.Server{}).ServeHTTP(rec, request)
	if rec.Code != 503 {
		t.Fatalf("registration status = %d", rec.Code)
	}
	if body, _ := os.ReadFile(filepath.Join(scope, "target")); string(body) != "original" {
		t.Fatalf("registration destroyed target: %q", body)
	}
	entries, _ := os.ReadDir(scope)
	if len(entries) != 1 {
		t.Fatalf("failed registration leaked files: %v", entries)
	}
}

func TestUploadCacheGenerationIsolation(t *testing.T) {
	factories := map[string]func(*testing.T) UploadCache{
		"memory": func(t *testing.T) UploadCache { cache := newMemoryUploadCache(); t.Cleanup(cache.Close); return cache },
		"redis": func(t *testing.T) UploadCache {
			endpoint := os.Getenv("FILEBROWSER_TEST_REDIS_URL")
			if endpoint == "" {
				t.Skip("set FILEBROWSER_TEST_REDIS_URL for Redis integration tests")
			}
			cache, err := newRedisUploadCache(endpoint)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(cache.Close)
			return cache
		},
	}
	for name, factory := range factories {
		t.Run(name, func(t *testing.T) {
			cache := factory(t)
			key := filepath.Join(t.TempDir(), "generation")
			first := UploadState{ID: "first", Length: 4}
			second := UploadState{ID: "second", Length: 8}
			if err := cache.Register(key, first, func() error { return nil }); err != nil {
				t.Fatal(err)
			}
			if err := cache.Register(key, second, func() error { return nil }); !errors.Is(err, ErrUploadExists) {
				t.Fatalf("duplicate registration: %v", err)
			}
			if err := cache.Complete(key, first.ID); err != nil {
				t.Fatal(err)
			}
			if err := cache.Register(key, second, func() error { return nil }); err != nil {
				t.Fatal(err)
			}
			if err := cache.Complete(key, first.ID); !errors.Is(err, ErrUploadNotFound) {
				t.Fatalf("stale completion: %v", err)
			}
			if err := cache.Touch(key, first.ID); !errors.Is(err, ErrUploadNotFound) {
				t.Fatalf("stale heartbeat: %v", err)
			}
			state, err := cache.Get(key)
			if err != nil || state.ID != second.ID {
				t.Fatalf("replacement lost: %+v %v", state, err)
			}
			if err := cache.Complete(key, second.ID); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestRedisUploadResumesOnAnotherReplica(t *testing.T) {
	endpoint := os.Getenv("FILEBROWSER_TEST_REDIS_URL")
	if endpoint == "" {
		t.Skip("set FILEBROWSER_TEST_REDIS_URL for Redis integration tests")
	}
	first, err := newRedisUploadCache(endpoint)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(first.Close)
	second, err := newRedisUploadCache(endpoint)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(second.Close)
	scope := t.TempDir()
	if err := os.WriteFile(filepath.Join(scope, "target"), []byte("original"), 0600); err != nil {
		t.Fatal(err)
	}
	key := []byte("replica-key")
	perm := users.Permissions{Create: true, Modify: true}
	st := scopedUserStorage(t, scope, perm, key)
	token := signToken(t, perm, key)
	request := func(cache UploadCache, method, target, body, offset string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, target, strings.NewReader(body))
		req.Header.Set("X-Auth", token)
		req.Header.Set("Upload-Length", "6")
		req.Header.Set("Upload-Offset", offset)
		req.Header.Set("Content-Type", "application/offset+octet-stream")
		rec := httptest.NewRecorder()
		var handler handleFunc
		switch method {
		case "POST":
			handler = tusPostHandler(cache)
		case "PATCH":
			handler = tusPatchHandler(cache)
		case "HEAD":
			handler = tusHeadHandler(cache)
		}
		handle(handler, "", st, &settings.Server{}).ServeHTTP(rec, req)
		return rec
	}
	created := request(first, "POST", "/target?override=true", "", "")
	if created.Code != 201 {
		t.Fatalf("create: %d %s", created.Code, created.Body.String())
	}
	location, _ := url.Parse(created.Header().Get("Location"))
	target := "/target?" + location.RawQuery
	if patched := request(second, "PATCH", target, "new", "0"); patched.Code != 204 {
		t.Fatalf("replica patch: %d %s", patched.Code, patched.Body.String())
	}
	if head := request(first, "HEAD", target, "", ""); head.Code != 200 || head.Header().Get("Upload-Offset") != "3" {
		t.Fatalf("replica HEAD: %d %v", head.Code, head.Header())
	}
	if patched := request(first, "PATCH", target, "new", "3"); patched.Code != 204 {
		t.Fatalf("complete: %d %s", patched.Code, patched.Body.String())
	}
	if body, _ := os.ReadFile(filepath.Join(scope, "target")); string(body) != "newnew" {
		t.Fatalf("published: %q", body)
	}

	created = request(first, "POST", "/abandoned", "", "")
	if created.Code != 201 {
		t.Fatalf("abandoned create: %d", created.Code)
	}
	cacheKey := physicalPathKey(filepath.Join(scope, "abandoned"))
	state, err := first.Get(cacheKey)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := redisContext()
	defer cancel()
	if err := first.client.PExpire(ctx, first.filePathKey(cacheKey), time.Millisecond).Err(); err != nil {
		t.Fatal(err)
	}
	time.Sleep(10 * time.Millisecond)
	if _, err := second.Get(cacheKey); !errors.Is(err, ErrUploadNotFound) {
		t.Fatalf("Redis expiry: %v", err)
	}
	cleanup := first.cleanup.cache.Get(cacheKey + "|" + state.ID)
	if cleanup == nil {
		t.Fatal("missing cleanup callback")
	}
	if err := cleanup.Value().remove(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(scope, filepath.FromSlash(strings.TrimPrefix(state.Stage, "/")))); !os.IsNotExist(err) {
		t.Fatalf("expired stage remains: %v", err)
	}
	if body, _ := os.ReadFile(filepath.Join(scope, "target")); string(body) != "newnew" {
		t.Fatalf("expiry changed published file: %q", body)
	}
}

func TestUploadStageLockAcrossProcesses(t *testing.T) {
	if root := os.Getenv("FILEBROWSER_LOCK_HELPER_ROOT"); root != "" {
		fs := afero.NewBasePathFs(afero.NewOsFs(), root)
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		unlock, err := lockUploadStage(ctx, fs, "/stage")
		if err != nil {
			t.Fatal(err)
		}
		defer unlock()
		_, _ = os.Stdout.WriteString("locked\n")
		_, _ = os.Stdin.Read(make([]byte, 1))
		return
	}
	root := t.TempDir()
	child := exec.Command(os.Args[0], "-test.run=^TestUploadStageLockAcrossProcesses$")
	child.Env = append(os.Environ(), "FILEBROWSER_LOCK_HELPER_ROOT="+root)
	input, err := child.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	output, err := child.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := child.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = input.Close(); _ = child.Process.Kill(); _ = child.Wait() })
	var ready [7]byte
	if _, err := io.ReadFull(output, ready[:]); err != nil || string(ready[:]) != "locked\n" {
		t.Fatalf("child readiness %q %v", ready, err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	if unlock, err := lockUploadStage(ctx, afero.NewBasePathFs(afero.NewOsFs(), root), "/stage"); err == nil {
		unlock()
		t.Fatal("second process acquired a held lock")
	} else if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal(err)
	}
	_ = input.Close()
	if err := child.Wait(); err != nil {
		t.Fatal(err)
	}
	unlock, err := lockUploadStage(context.Background(), afero.NewBasePathFs(afero.NewOsFs(), root), "/stage")
	if err != nil {
		t.Fatal(err)
	}
	unlock()
}

func TestCleanupUploadsPreservesPublishedFiles(t *testing.T) {
	root := t.TempDir()
	stage := internalWritePrefix + "generation-partial"
	if err := os.WriteFile(filepath.Join(root, stage), []byte("partial"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "target"), []byte("original"), 0600); err != nil {
		t.Fatal(err)
	}
	journal, _ := json.Marshal(uploadJournal{Key: "test-key", State: UploadState{ID: "generation", Stage: "/target"}})
	if err := os.WriteFile(filepath.Join(root, stage+".json"), journal, 0600); err != nil {
		t.Fatal(err)
	}
	before := time.Now().Add(time.Minute)
	if found, err := CleanupUploads(root, nil, before, false); err != nil || len(found) != 1 {
		t.Fatalf("dry run: %v %v", found, err)
	}
	if _, err := os.Stat(filepath.Join(root, stage)); err != nil {
		t.Fatal("dry run removed stage")
	}
	if found, err := CleanupUploads(root, nil, before, true); err != nil || len(found) != 1 {
		t.Fatalf("cleanup: %v %v", found, err)
	}
	if body, _ := os.ReadFile(filepath.Join(root, "target")); string(body) != "original" {
		t.Fatalf("cleanup followed untrusted destination: %q", body)
	}
	entries, _ := os.ReadDir(root)
	if len(entries) != 1 {
		t.Fatalf("cleanup leaked metadata: %v", entries)
	}
}
