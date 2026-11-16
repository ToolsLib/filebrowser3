package fbhttp

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/filebrowser/filebrowser/v2/settings"
	"github.com/filebrowser/filebrowser/v2/share"
	"github.com/filebrowser/filebrowser/v2/users"
)

func TestRulesDenySymlinkAliases(t *testing.T) {
	scope := t.TempDir()
	if err := os.Mkdir(filepath.Join(scope, "private"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(scope, "private", "secret.txt"), []byte("secret"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(scope, "private"), filepath.Join(scope, "alias")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	key := []byte("alias-test-key")
	perm := users.Permissions{Download: true, Modify: true, Create: true, Share: true}
	st := denyRuleStorage(t, scope, "/private", perm, key)
	if err := st.Share.Save(&share.Link{Hash: "alias-share", UserID: 1, Path: "/"}); err != nil {
		t.Fatal(err)
	}
	signed := signToken(t, st, perm, key)
	for _, tc := range []struct {
		name, method, path string
		fn                 handleFunc
	}{
		{"read", http.MethodGet, "/alias/secret.txt", rawHandler},
		{"write", http.MethodPut, "/alias/secret.txt", resourcePutHandler},
		{"public read", http.MethodGet, "alias-share/alias/secret.txt", publicDlHandler},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, "/", strings.NewReader("replacement"))
			req.URL.Path = tc.path
			req.Header.Set("X-Auth", signed)
			rec := httptest.NewRecorder()
			handle(tc.fn, "", st, &settings.Server{}).ServeHTTP(rec, req)
			if rec.Code != http.StatusForbidden {
				t.Fatalf("status %d, body %q", rec.Code, rec.Body.String())
			}
		})
	}
	body, err := os.ReadFile(filepath.Join(scope, "private", "secret.txt"))
	if err != nil || string(body) != "secret" {
		t.Fatalf("denied file changed: %q, %v", body, err)
	}
}
