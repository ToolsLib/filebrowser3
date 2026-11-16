package fbhttp

import (
	"archive/zip"
	"bytes"
	"io"
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

func TestPublicShareAliasRootPreservesCanonicalChildRules(t *testing.T) {
	scope := t.TempDir()
	if err := os.Mkdir(filepath.Join(scope, "private"), 0700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"secret.txt", "open.txt"} {
		if err := os.WriteFile(filepath.Join(scope, "private", name), []byte(name), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink(filepath.Join(scope, "private"), filepath.Join(scope, "alias")); err != nil {
		t.Skip(err)
	}
	st := denyRuleStorage(t, scope, "/private/secret.txt", users.Permissions{Share: true, Download: true}, []byte("key"))
	if err := st.Share.Save(&share.Link{Hash: "alias", UserID: 1, Path: "/alias"}); err != nil {
		t.Fatal(err)
	}
	handler := handle(publicDlHandler, "", st, &settings.Server{})
	for _, tc := range []struct {
		path   string
		status int
	}{{"alias/secret.txt", 403}, {"alias/open.txt", 200}} {
		r := httptest.NewRequest("GET", "/", http.NoBody)
		r.URL.Path = tc.path
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != tc.status {
			t.Fatalf("%s: %d, %s", tc.path, w.Code, w.Body.String())
		}
	}
	r := httptest.NewRequest("GET", "/?algo=zip", http.NoBody)
	r.URL.Path = "alias/"
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	if w.Code != 200 {
		t.Fatalf("archive: %d, %s", w.Code, w.Body.String())
	}
	archive, err := zip.NewReader(bytes.NewReader(w.Body.Bytes()), int64(w.Body.Len()))
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range archive.File {
		reader, err := file.Open()
		if err != nil {
			t.Fatal(err)
		}
		body, err := io.ReadAll(reader)
		_ = reader.Close()
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(file.Name, "secret") || bytes.Contains(body, []byte("secret.txt")) {
			t.Fatal("archive disclosed denied alias child")
		}
	}
}
