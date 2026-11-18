package fbhttp

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/filebrowser/filebrowser/v2/diskcache"
	"github.com/filebrowser/filebrowser/v2/settings"
	"github.com/filebrowser/filebrowser/v2/storage"
	"github.com/filebrowser/filebrowser/v2/users"
	"github.com/gorilla/mux"
)

func TestEffectiveUploadLimit(t *testing.T) {
	for _, tc := range []struct {
		name                           string
		deployment, global, user, want int64
	}{
		{"default", 0, 0, 0, 10 << 30},
		{"global can raise default", 0, 100 << 30, 0, 100 << 30},
		{"deployment inherited", 100, 0, 0, 100},
		{"deployment caps global", 10, 100, 50, 10},
		{"global lowers deployment", 100, 20, 50, 20},
		{"user lowers global", 100, 20, 5, 5},
		{"safety ceiling", 0, 0, 0, settings.DefaultMaxUploadSize},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := &data{server: &settings.Server{MaxUploadSize: tc.deployment}, settings: &settings.Settings{MaxUploadSize: tc.global}, user: &users.User{MaxUploadSize: tc.user}}
			if got := effectiveUploadLimit(d); got != tc.want {
				t.Fatalf("limit=%d, want %d", got, tc.want)
			}
		})
	}
}

func TestUploadLimitsEnforceDirectAndResumableWrites(t *testing.T) {
	root := t.TempDir()
	perm := users.Permissions{Create: true, Modify: true}
	key := []byte("upload-policy-key")
	st := scopedUserStorage(t, root, perm, key)
	u, _ := st.Users.Get("", false, uint(1))
	u.MaxUploadSize = 5
	if err := st.Users.Update(u, "MaxUploadSize"); err != nil {
		t.Fatal(err)
	}
	signed := signToken(t, st, perm, key)
	server := &settings.Server{MaxUploadSize: 10}
	cache := newMemoryUploadCache()
	t.Cleanup(cache.Close)
	call := func(fn handleFunc, method, path, body string, unknown bool, headers map[string]string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		if unknown {
			r.ContentLength = -1
		}
		r.Header.Set("X-Auth", signed)
		for key, value := range headers {
			r.Header.Set(key, value)
		}
		w := httptest.NewRecorder()
		handle(fn, "", st, server).ServeHTTP(w, r)
		return w
	}
	assertLimit := func(w *httptest.ResponseRecorder, want int64) {
		t.Helper()
		var response struct {
			Code string `json:"code"`
			Max  int64  `json:"maxUploadSize"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
			t.Fatal(err)
		}
		if w.Code != 413 || response.Code != "upload_too_large" || response.Max != want {
			t.Fatalf("limit response: %d %s", w.Code, w.Body.String())
		}
	}
	if err := os.WriteFile(filepath.Join(root, "file"), []byte("old"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, unknown := range []bool{false, true} {
		assertLimit(call(resourcePostHandler(diskcache.NewNoOp()), "POST", "/file?override=true", "123456", unknown, nil), 5)
		assertLimit(call(resourcePutHandler, "PUT", "/file", "123456", unknown, nil), 5)
		body, _ := os.ReadFile(filepath.Join(root, "file"))
		if string(body) != "old" {
			t.Fatal("oversized upload changed the destination")
		}
	}
	assertLimit(call(tusPostHandler(cache), "POST", "/file?override=true", "", false, map[string]string{"Upload-Length": "6"}), 5)
	created := call(tusPostHandler(cache), "POST", "/file?override=true", "", false, map[string]string{"Upload-Length": "5"})
	if created.Code != 201 {
		t.Fatalf("create: %d %s", created.Code, created.Body.String())
	}
	location, _ := url.Parse(created.Header().Get("Location"))
	location.Path = strings.TrimPrefix(location.Path, "/api/tus")
	patch := func(offset, body string) *httptest.ResponseRecorder {
		return call(tusPatchHandler(cache), "PATCH", location.RequestURI(), body, false, map[string]string{"Content-Type": "application/offset+octet-stream", "Upload-Offset": offset})
	}
	if w := patch("0", "12"); w.Code != 204 {
		t.Fatalf("first chunk: %d %s", w.Code, w.Body.String())
	}
	// A token issued before an administrator's change must obey the new policy.
	u.MaxUploadSize = 4
	if err := st.Users.Update(u, "MaxUploadSize"); err != nil {
		t.Fatal(err)
	}
	assertLimit(patch("2", "345"), 4)
	body, _ := os.ReadFile(filepath.Join(root, "file"))
	if string(body) != "old" {
		t.Fatal("limit change published an incomplete upload")
	}
	u.MaxUploadSize = 5
	if err := st.Users.Update(u, "MaxUploadSize"); err != nil {
		t.Fatal(err)
	}
	if w := patch("2", "345"); w.Code != 204 {
		t.Fatalf("last chunk: %d %s", w.Code, w.Body.String())
	}
	body, _ = os.ReadFile(filepath.Join(root, "file"))
	if string(body) != "12345" {
		t.Fatalf("wrong published content: %q", body)
	}
}

func TestUploadSettingsPermissionsAndLargeFileAdmission(t *testing.T) {
	key := []byte("upload-admin-key")
	perm := users.Permissions{Create: true, Modify: true}
	st := scopedUserStorage(t, t.TempDir(), perm, key)
	signed := signToken(t, st, perm, key)
	server := &settings.Server{}
	call := func(fn handleFunc, method, path, body string, headers map[string]string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		r.Header.Set("X-Auth", signed)
		for key, value := range headers {
			r.Header.Set(key, value)
		}
		w := httptest.NewRecorder()
		handle(fn, "", st, server).ServeHTTP(w, r)
		return w
	}
	// Build the actual router to exercise ID parsing and authorization.
	for _, which := range []string{"MaxUploadSize", "maxuploadsize", "MAXUPLOADSIZE", "all"} {
		w := sessionCallForUploadUser(t, st, server, signed, fmt.Sprintf(`{"what":"user","which":[%q],"data":{"id":1,"maxUploadSize":1}}`, which))
		if w.Code != 403 {
			t.Fatalf("self update %q: %d %s", which, w.Code, w.Body.String())
		}
	}
	if w := call(settingsPutHandler, "PUT", "/api/settings", `{"maxUploadSize":100}`, nil); w.Code != 403 {
		t.Fatalf("non-admin settings: %d", w.Code)
	}
	u, _ := st.Users.Get("", false, uint(1))
	u.Perm.Admin = true
	if err := st.Users.Update(u, "Perm"); err != nil {
		t.Fatal(err)
	}
	cache := newMemoryUploadCache()
	t.Cleanup(cache.Close)
	headers := map[string]string{"Upload-Length": fmt.Sprint(int64(24) << 30)}
	if w := call(tusPostHandler(cache), "POST", "/large", "", headers); w.Code != 413 {
		t.Fatalf("default allowed large upload: %d", w.Code)
	}
	if w := call(settingsPutHandler, "PUT", "/api/settings", fmt.Sprintf(`{"maxUploadSize":%d,"uploadLimits":{"deploymentLimit":999999}}`, int64(100)<<30), nil); w.Code != 200 {
		t.Fatalf("admin settings: %d %s", w.Code, w.Body.String())
	}
	if w := call(tusPostHandler(cache), "POST", "/large", "", headers); w.Code != 201 {
		t.Fatalf("configured large upload: %d %s", w.Code, w.Body.String())
	}
	server.MaxUploadSize, server.MaxUploadSizeSource = 10<<30, "environment"
	if w := call(tusPostHandler(cache), "POST", "/other", "", headers); w.Code != 413 {
		t.Fatalf("global setting bypassed environment: %d", w.Code)
	}
	w := call(settingsGetHandler, "GET", "/api/settings", "", nil)
	var result settingsData
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.MaxUploadSize != 100<<30 || result.UploadLimits.DeploymentLimit != 10<<30 || result.UploadLimits.DeploymentSource != "environment" || result.UploadLimits.GlobalLimit != 10<<30 {
		t.Fatalf("incorrect ceiling metadata: %+v", result.UploadLimits)
	}
	for _, body := range []string{`{"maxUploadSize":-1}`, `{"maxUploadSize":1125899906842625}`, `{"defaults":{"maxUploadSize":-1}}`} {
		if w := call(settingsPutHandler, "PUT", "/api/settings", body, nil); w.Code != 400 {
			t.Fatalf("invalid limit accepted: %d", w.Code)
		}
	}
	w = sessionCallForUploadUser(t, st, server, signed, `{"what":"user","which":["MaxUploadSize"],"data":{"id":1,"maxUploadSize":2}}`)
	if w.Code != 200 {
		t.Fatalf("admin user limit: %d %s", w.Code, w.Body.String())
	}
	w = call(uploadLimitGetHandler, "GET", "/api/upload-limit", "", nil)
	if !strings.Contains(w.Body.String(), `"maxUploadSize":2`) {
		t.Fatalf("stale user limit: %s", w.Body.String())
	}
}

func sessionCallForUploadUser(t *testing.T, st *storage.Storage, server *settings.Server, token, body string) *httptest.ResponseRecorder {
	t.Helper()
	router := mux.NewRouter()
	router.Handle("/api/users/{id:[0-9]+}", handle(userPutHandler, "", st, server)).Methods("PUT")
	r := httptest.NewRequest("PUT", "/api/users/1", strings.NewReader(body))
	r.Header.Set("X-Auth", token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, r)
	return w
}
