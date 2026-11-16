package fbhttp

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/filebrowser/filebrowser/v2/auth"
	"github.com/filebrowser/filebrowser/v2/diskcache"
	"github.com/filebrowser/filebrowser/v2/settings"
	"github.com/filebrowser/filebrowser/v2/storage"
	"github.com/filebrowser/filebrowser/v2/users"
)

func sessionFixture(t *testing.T) (*storage.Storage, http.Handler) {
	t.Helper()
	st := scopedUserStorage(t, t.TempDir(), users.Permissions{Download: true, Modify: true}, []byte("key"))
	password, err := users.HashPwd("Original-password-42!")
	if err != nil {
		t.Fatal(err)
	}
	u, err := st.Users.Get("", false, uint(1))
	if err != nil {
		t.Fatal(err)
	}
	u.Password = password
	if err := st.Users.Update(u, "Password"); err != nil {
		t.Fatal(err)
	}
	if err := st.Settings.Save(&settings.Settings{Key: []byte("key"), AuthMethod: auth.MethodJSONAuth, MinimumPasswordLength: 12}); err != nil {
		t.Fatal(err)
	}
	if err := st.Auth.Save(&auth.JSONAuth{}); err != nil {
		t.Fatal(err)
	}
	cache := newMemoryUploadCache()
	t.Cleanup(cache.Close)
	handler, err := NewHandler(nil, diskcache.NewNoOp(), cache, st, &settings.Server{BaseURL: "/app"}, fstest.MapFS{"public/index.html": &fstest.MapFile{Data: []byte("<html>app</html>")}})
	if err != nil {
		t.Fatal(err)
	}
	return st, handler
}

func sessionCall(handler http.Handler, method, path, body string, headers map[string]string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, "https://example.test/app/api/"+path, strings.NewReader(body))
	for key, value := range headers {
		r.Header.Set(key, value)
	}
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	return w
}

func sessionLogin(t *testing.T, handler http.Handler) (string, string) {
	t.Helper()
	res := sessionCall(handler, "POST", "login", `{"username":"u","password":"Original-password-42!"}`, map[string]string{"X-Session-Transport": "header"})
	if res.Code != 200 {
		t.Fatalf("login: %d %s", res.Code, res.Body.String())
	}
	return res.Body.String(), res.Header().Get("X-Refresh-Token")
}

func TestSessionRotationRejectsTokenOnlyRenewalAndReplay(t *testing.T) {
	_, handler := sessionFixture(t)
	access, refresh := sessionLogin(t, handler)
	res := sessionCall(handler, "POST", "renew", "", map[string]string{"X-Session-Transport": "header", "X-Auth": access})
	if res.Code != 401 {
		t.Fatalf("access token renewed without secret: %d", res.Code)
	}
	res = sessionCall(handler, "POST", "renew", "", map[string]string{"X-Session-Transport": "header", "X-Refresh-Token": refresh})
	if res.Code != 200 || res.Header().Get("X-Refresh-Token") == refresh {
		t.Fatalf("refresh rotation: %d", res.Code)
	}
	nextAccess := res.Body.String()
	res = sessionCall(handler, "POST", "renew", "", map[string]string{"X-Session-Transport": "header", "X-Refresh-Token": refresh})
	if res.Code != 401 {
		t.Fatalf("refresh replay accepted: %d", res.Code)
	}
	res = sessionCall(handler, "GET", "resources/", "", map[string]string{"X-Auth": nextAccess})
	if res.Code != 401 {
		t.Fatalf("replay did not revoke access: %d", res.Code)
	}
}

func TestSessionLogoutAndPasswordChange(t *testing.T) {
	_, handler := sessionFixture(t)
	first, _ := sessionLogin(t, handler)
	second, _ := sessionLogin(t, handler)
	res := sessionCall(handler, "POST", "logout", "", map[string]string{"X-Requested-With": "FileBrowser", "X-Auth": first})
	if res.Code != 204 {
		t.Fatalf("logout: %d", res.Code)
	}
	if res := sessionCall(handler, "GET", "resources/", "", map[string]string{"X-Auth": first}); res.Code != 401 {
		t.Fatalf("logged out token accepted: %d", res.Code)
	}
	if res := sessionCall(handler, "GET", "resources/", "", map[string]string{"X-Auth": second}); res.Code != 200 {
		t.Fatalf("other session affected: %d", res.Code)
	}
	third, refresh := sessionLogin(t, handler)
	res = sessionCall(handler, "PUT", "users/1", `{"what":"user","which":["password"],"data":{"id":1,"password":"Replacement-password-42!"},"current_password":"Original-password-42!"}`, map[string]string{"X-Auth": second})
	if res.Code != 200 {
		t.Fatalf("password change: %d %s", res.Code, res.Body.String())
	}
	for _, access := range []string{second, third} {
		if res := sessionCall(handler, "GET", "resources/", "", map[string]string{"X-Auth": access}); res.Code != 401 {
			t.Fatalf("password change left session active: %d", res.Code)
		}
	}
	res = sessionCall(handler, "POST", "renew", "", map[string]string{"X-Session-Transport": "header", "X-Refresh-Token": refresh})
	if res.Code != 401 {
		t.Fatalf("password change left refresh active: %d", res.Code)
	}
}

func TestSessionLogoutAll(t *testing.T) {
	_, handler := sessionFixture(t)
	first, _ := sessionLogin(t, handler)
	second, _ := sessionLogin(t, handler)
	res := sessionCall(handler, "POST", "logout-all", "", map[string]string{"X-Requested-With": "FileBrowser", "X-Auth": first})
	if res.Code != 204 {
		t.Fatalf("logout all: %d", res.Code)
	}
	if res := sessionCall(handler, "GET", "resources/", "", map[string]string{"X-Auth": second}); res.Code != 401 {
		t.Fatal("other session survived logout-all")
	}
}

func TestSessionCredentialChangeOutsideHTTP(t *testing.T) {
	st, handler := sessionFixture(t)
	access, refresh := sessionLogin(t, handler)
	user, err := st.Users.Get("", false, uint(1))
	if err != nil {
		t.Fatal(err)
	}
	user.Password = "different-credential"
	if err := st.Users.Update(user, "Password"); err != nil {
		t.Fatal(err)
	}
	if res := sessionCall(handler, "GET", "resources/", "", map[string]string{"X-Auth": access}); res.Code != 401 {
		t.Fatalf("external credential change left access valid: %d", res.Code)
	}
	if res := sessionCall(handler, "POST", "renew", "", map[string]string{"X-Session-Transport": "header", "X-Refresh-Token": refresh}); res.Code != 401 {
		t.Fatalf("external credential change left refresh valid: %d", res.Code)
	}
}

func TestSessionCookiesAndCSRF(t *testing.T) {
	_, handler := sessionFixture(t)
	body := `{"username":"u","password":"Original-password-42!"}`
	for _, headers := range []map[string]string{{}, {"X-Requested-With": "FileBrowser", "Origin": "https://attacker.test"}, {"X-Requested-With": "FileBrowser", "Sec-Fetch-Site": "cross-site"}} {
		if res := sessionCall(handler, "POST", "login", body, headers); res.Code != 403 {
			t.Fatalf("cross-origin/form login: %d", res.Code)
		}
	}
	res := sessionCall(handler, "POST", "login", body, map[string]string{"X-Requested-With": "FileBrowser", "Origin": "https://example.test"})
	if res.Code != 200 || res.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("cookie login: %d", res.Code)
	}
	cookies := res.Result().Cookies()
	found := map[string]*http.Cookie{}
	for _, cookie := range cookies {
		found[cookie.Name] = cookie
		if cookie.Name == "auth" {
			continue
		}
		if !cookie.HttpOnly || !cookie.Secure || cookie.SameSite != http.SameSiteStrictMode || cookie.Domain != "" {
			t.Fatalf("unsafe cookie: %+v", cookie)
		}
	}
	if found[accessCookie] == nil || found[accessCookie].Path != "/app/" || found[refreshCookie] == nil || found[refreshCookie].Path != "/app/api/" {
		t.Fatal("incorrect cookie paths")
	}
	headers := map[string]string{"Cookie": accessCookie + "=" + found[accessCookie].Value + "; " + refreshCookie + "=" + found[refreshCookie].Value}
	if res := sessionCall(handler, "POST", "logout", "", headers); res.Code != 403 {
		t.Fatalf("cookie logout without CSRF proof: %d", res.Code)
	}
	if res := sessionCall(handler, "PUT", "resources/new", "data", headers); res.Code != 401 {
		t.Fatalf("cookie authenticated an ordinary mutation: %d", res.Code)
	}
	headers["X-Requested-With"] = "FileBrowser"
	if res := sessionCall(handler, "POST", "renew", "", headers); res.Code != 200 {
		t.Fatalf("cookie refresh: %d", res.Code)
	}
}
