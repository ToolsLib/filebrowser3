package fbhttp

import (
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/filebrowser/filebrowser/v2/auth"
	"github.com/filebrowser/filebrowser/v2/diskcache"
	"github.com/filebrowser/filebrowser/v2/settings"
	"github.com/filebrowser/filebrowser/v2/users"
)

func TestSecurityHeadersProtectFallbackWithUniqueNonce(t *testing.T) {
	st := scopedUserStorage(t, t.TempDir(), users.Permissions{}, []byte("key"))
	if err := st.Settings.Save(&settings.Settings{Key: []byte("key"), AuthMethod: auth.MethodJSONAuth}); err != nil {
		t.Fatal(err)
	}
	if err := st.Auth.Save(&auth.JSONAuth{}); err != nil {
		t.Fatal(err)
	}
	assets := fstest.MapFS{"public/index.html": &fstest.MapFile{Data: []byte(`<html><script nonce="[{[ .CSPNonce ]}]">window.test=true;</script></html>`)}, "ace/mode-test.js": &fstest.MapFile{Data: []byte("test")}}
	cache := newMemoryUploadCache()
	defer cache.Close()
	handler, err := NewHandler(nil, diskcache.NewNoOp(), cache, st, &settings.Server{BaseURL: "/files-app"}, assets)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, target := range []string{"/files-app/", "/files-app/files/", "/files-app/login", "/files-app/health"} {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, target, http.NoBody))
		policy := rec.Header().Get("Content-Security-Policy")
		match := regexp.MustCompile(`'nonce-([^']+)'`).FindStringSubmatch(policy)
		if rec.Code != 200 || len(match) != 2 {
			t.Fatalf("%s: %d, %q", target, rec.Code, policy)
		}
		if seen[match[1]] {
			t.Fatal("nonce reused")
		}
		seen[match[1]] = true
		if target != "/files-app/health" && !strings.Contains(rec.Body.String(), `nonce="`+match[1]+`"`) {
			t.Fatalf("HTML nonce differs from header: %s", rec.Body.String())
		}
		if strings.Contains(policy, "unsafe-eval") || !strings.Contains(policy, "frame-ancestors 'none'") {
			t.Fatalf("unexpected policy: %s", policy)
		}
	}
	// Locally emitted Ace files remain usable even without compressed variants.
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/files-app/static/ace/mode-test.js", http.NoBody))
	if rec.Code != 200 || rec.Body.String() != "test" {
		t.Fatalf("local asset: %d %q", rec.Code, rec.Body.String())
	}
}
