package fbhttp

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/filebrowser/filebrowser/v2/rules"
	"github.com/filebrowser/filebrowser/v2/settings"
	"github.com/filebrowser/filebrowser/v2/share"
	"github.com/filebrowser/filebrowser/v2/storage"
	"github.com/filebrowser/filebrowser/v2/users"
	"github.com/spf13/afero"
)

func singleFileShareFixture(t *testing.T, virtual bool) (*storage.Storage, []byte) {
	t.Helper()
	scope := t.TempDir()
	content := []byte{'M', 'Z', 0, 1, 2, 3, 4, 5}
	backing := afero.NewOsFs()
	root := scope
	if virtual {
		backing = afero.NewMemMapFs()
		root = "/"
	}
	if err := backing.MkdirAll(filepath.Join(root, "private", "project"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := afero.WriteFile(backing, filepath.Join(root, "private", "project", "package.exe"), content, 0600); err != nil {
		t.Fatal(err)
	}
	if err := afero.WriteFile(backing, filepath.Join(root, "private", "project", "neighbor.txt"), []byte("private neighbor"), 0600); err != nil {
		t.Fatal(err)
	}
	st := scopedUserStorage(t, scope, users.Permissions{Share: true, Download: true}, []byte("key"))
	if virtual {
		st.Users = &customFSUser{Store: st.Users, fs: backing}
	}
	if err := st.Share.Save(&share.Link{Hash: "h", UserID: 1, Path: "/private/project/package.exe"}); err != nil {
		t.Fatal(err)
	}
	return st, content
}

func singleFileShareRequest(st *storage.Storage, fn handleFunc, suffix, query, byteRange string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(http.MethodGet, "/?"+query, http.NoBody)
	r.URL.Path = "h" + suffix
	if byteRange != "" {
		r.Header.Set("Range", byteRange)
	}
	w := httptest.NewRecorder()
	handle(fn, "", st, &settings.Server{}).ServeHTTP(w, r)
	return w
}

func TestPublicSingleFileShareDownload(t *testing.T) {
	for _, virtual := range []bool{false, true} {
		name := "native"
		if virtual {
			name = "memory"
		}
		t.Run(name, func(t *testing.T) {
			st, content := singleFileShareFixture(t, virtual)
			for _, tc := range []struct {
				name, suffix, query, byteRange, disposition string
				status                                      int
				body                                        []byte
			}{
				{"attachment", "", "", "", "attachment;", 200, content},
				{"inline", "", "inline=true", "", "inline;", 200, content},
				{"range", "", "inline=true", "bytes=1-3", "inline;", 206, content[1:4]},
				{"filename suffix", "/package.exe", "", "", "attachment;", 200, content},
				{"suffix cannot select neighbor", "/neighbor.txt", "", "", "attachment;", 200, content},
			} {
				t.Run(tc.name, func(t *testing.T) {
					w := singleFileShareRequest(st, publicDlHandler, tc.suffix, tc.query, tc.byteRange)
					if w.Code != tc.status || !bytes.Equal(w.Body.Bytes(), tc.body) {
						t.Fatalf("download = %d %q; want %d %q", w.Code, w.Body.Bytes(), tc.status, tc.body)
					}
					if disposition := w.Header().Get("Content-Disposition"); !strings.HasPrefix(disposition, tc.disposition) || !strings.Contains(disposition, "package.exe") {
						t.Fatalf("disposition = %q", disposition)
					}
					if tc.byteRange != "" && w.Header().Get("Content-Range") != "bytes 1-3/8" {
						t.Fatalf("range = %q", w.Header().Get("Content-Range"))
					}
				})
			}
			w := singleFileShareRequest(st, publicShareHandler, "", "", "")
			var info struct {
				Path, Name string
				IsDir      bool
			}
			if err := json.Unmarshal(w.Body.Bytes(), &info); err != nil {
				t.Fatalf("metadata = %d %q: %v", w.Code, w.Body.String(), err)
			}
			if w.Code != 200 || info.Path != "" || info.Name != "package.exe" || info.IsDir {
				t.Fatalf("unexpected public metadata: %d %+v", w.Code, info)
			}
		})
	}
}

func TestPublicSingleFileShareAccessControls(t *testing.T) {
	t.Run("denied file", func(t *testing.T) {
		st, _ := singleFileShareFixture(t, false)
		if err := st.Settings.Save(&settings.Settings{Key: []byte("key"), Rules: []rules.Rule{{Path: "/private/project/package.exe", Allow: false}}}); err != nil {
			t.Fatal(err)
		}
		for _, fn := range []handleFunc{publicDlHandler, publicShareHandler} {
			if w := singleFileShareRequest(st, fn, "", "", ""); w.Code != 403 {
				t.Fatalf("denied share returned %d", w.Code)
			}
		}
	})
	t.Run("owner permission revoked", func(t *testing.T) {
		st, _ := singleFileShareFixture(t, false)
		user, err := st.Users.Get("", false, uint(1))
		if err != nil {
			t.Fatal(err)
		}
		user.Perm.Download = false
		if err := st.Users.Update(user, "Perm"); err != nil {
			t.Fatal(err)
		}
		if w := singleFileShareRequest(st, publicDlHandler, "", "", ""); w.Code != 403 {
			t.Fatalf("revoked share returned %d", w.Code)
		}
	})
	t.Run("password required", func(t *testing.T) {
		st, content := singleFileShareFixture(t, false)
		password, err := users.HashPwd("share-test-password")
		if err != nil {
			t.Fatal(err)
		}
		if err := st.Share.Save(&share.Link{Hash: "h", UserID: 1, Path: "/private/project/package.exe", PasswordHash: password, Token: "share-test-token"}); err != nil {
			t.Fatal(err)
		}
		if w := singleFileShareRequest(st, publicDlHandler, "", "", ""); w.Code != 401 {
			t.Fatalf("private share returned %d", w.Code)
		}
		w := singleFileShareRequest(st, publicDlHandler, "", "token=share-test-token", "")
		if w.Code != 200 || !bytes.Equal(w.Body.Bytes(), content) {
			t.Fatalf("authorized share returned %d %q", w.Code, w.Body.Bytes())
		}
	})
	t.Run("outside symlink", func(t *testing.T) {
		st, _ := singleFileShareFixture(t, false)
		user, err := st.Users.Get("", false, uint(1))
		if err != nil {
			t.Fatal(err)
		}
		outside := filepath.Join(t.TempDir(), "secret.txt")
		if err := os.WriteFile(outside, []byte("outside"), 0600); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(outside, user.FullPath("/outside-link")); err != nil {
			t.Skip(err)
		}
		if err := st.Share.Save(&share.Link{Hash: "h", UserID: 1, Path: "/outside-link"}); err != nil {
			t.Fatal(err)
		}
		if w := singleFileShareRequest(st, publicDlHandler, "", "", ""); w.Code != 403 {
			t.Fatalf("outside link returned %d %q", w.Code, w.Body.String())
		}
	})
}
