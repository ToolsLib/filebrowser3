package fbhttp

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/filebrowser/filebrowser/v2/share"
	"github.com/filebrowser/filebrowser/v2/users"
	"github.com/spf13/afero"
)

func TestShareEditingPreservesLinksAndRevokesOldPasswords(t *testing.T) {
	st, handler := sessionFixture(t)
	u, _ := st.Users.Get("", false, uint(1))
	u.Perm.Share = true
	if err := st.Users.Update(u, "Perm"); err != nil {
		t.Fatal(err)
	}
	if err := afero.WriteFile(u.Fs, "/editable.txt", []byte("shared content"), 0600); err != nil {
		t.Fatal(err)
	}
	token, _ := sessionLogin(t, handler)
	headers := map[string]string{"X-Auth": token}
	created := sessionCall(handler, "POST", "share/editable.txt", `{"password":"Original-share-password","expires":"2","unit":"days"}`, headers)
	if created.Code != 200 {
		t.Fatalf("create: %d %s", created.Code, created.Body.String())
	}
	var original shareResponse
	if err := json.Unmarshal(created.Body.Bytes(), &original); err != nil {
		t.Fatal(err)
	}
	stored, _ := st.Share.GetByHash(original.Hash)
	oldToken, oldHash := stored.Token, stored.PasswordHash
	update := func(body string) shareResponse {
		t.Helper()
		res := sessionCall(handler, "PUT", "share/"+original.Hash, body, headers)
		if res.Code != 200 {
			t.Fatalf("update: %d %s", res.Code, res.Body.String())
		}
		if res.Header().Get("Cache-Control") != "no-store" || strings.Contains(res.Body.String(), "password_hash") || strings.Contains(res.Body.String(), `"token"`) {
			t.Fatalf("sensitive/cached response: %s", res.Body.String())
		}
		var link shareResponse
		if err := json.Unmarshal(res.Body.Bytes(), &link); err != nil {
			t.Fatal(err)
		}
		if link.Hash != original.Hash || link.Path != original.Path || link.UserID != original.UserID {
			t.Fatal("share URL or ownership changed")
		}
		return link
	}
	public := func(password, query string) int {
		return sessionCall(handler, "GET", "public/dl/"+original.Hash+query, "", map[string]string{"X-SHARE-PASSWORD": password}).Code
	}
	if public("Original-share-password", "") != 200 {
		t.Fatal("initial password failed")
	}
	now := time.Now().Unix()
	changed := update(`{"expires":"2","unit":"hours"}`)
	if changed.Expire < now+7200 || changed.Expire > time.Now().Unix()+7200 {
		t.Fatal("duration did not start at save time")
	}
	stored, _ = st.Share.GetByHash(original.Hash)
	if stored.Token != oldToken || stored.PasswordHash != oldHash {
		t.Fatal("expiration edit changed password credentials")
	}
	newExpiry := changed.Expire
	changed = update(`{"password":"Replacement-share-password"}`)
	stored, _ = st.Share.GetByHash(original.Hash)
	if !changed.HasPassword || stored.Token == oldToken || stored.PasswordHash == oldHash || changed.Expire != newExpiry {
		t.Fatal("password edit did not rotate credentials or changed expiry")
	}
	if public("Original-share-password", "") != 401 || public("", "?token="+oldToken) != 401 || public("Replacement-share-password", "") != 200 {
		t.Fatal("old password/token remained authorized")
	}
	changed = update(`{"password":""}`)
	stored, _ = st.Share.GetByHash(original.Hash)
	if changed.HasPassword || stored.Token != "" || stored.PasswordHash != "" || public("", "") != 200 {
		t.Fatal("removing protection failed")
	}
	if update(`{"expires":""}`).Expire != 0 {
		t.Fatal("permanent expiration not saved")
	}
	for _, body := range []string{`{}`, `{"path":"/other"}`, `{"userID":2,"password":"change"}`, `{"expires":"-1"}`, `{"expires":"0"}`, `{"expires":"1.5"}`, `{"expires":"18446744073709551615","unit":"days"}`, `{"expires":"1","unit":"years"}`, `{"unit":"days","password":"change"}`, `{"password":"ok"} {"password":"other"}`, fmt.Sprintf(`{"password":%q}`, strings.Repeat("é", 37))} {
		res := sessionCall(handler, "PUT", "share/"+original.Hash, body, headers)
		if res.Code != 400 {
			t.Fatalf("invalid update accepted: %d %s", res.Code, body)
		}
	}
	unchanged, _ := st.Share.GetByHash(original.Hash)
	if unchanged.PasswordHash != "" || unchanged.Expire != 0 {
		t.Fatal("invalid edit mutated the share")
	}
}

func TestShareEditAuthorizationAndExpiredLinks(t *testing.T) {
	st, handler := sessionFixture(t)
	u, _ := st.Users.Get("", false, uint(1))
	u.Perm.Share = true
	if err := st.Users.Update(u, "Perm"); err != nil {
		t.Fatal(err)
	}
	token, _ := sessionLogin(t, handler)
	headers := map[string]string{"X-Auth": token}
	other := &users.User{Username: "other", Password: u.Password, Perm: users.Permissions{Share: true, Download: true}}
	if err := st.Users.Save(other); err != nil {
		t.Fatal(err)
	}
	for _, link := range []*share.Link{{Hash: "otherlink", Path: "/file", UserID: other.ID}, {Hash: "expired", Path: "/file", UserID: 1, Expire: time.Now().Unix() - 1}} {
		if err := st.Share.Save(link); err != nil {
			t.Fatal(err)
		}
	}
	if res := sessionCall(handler, "PUT", "share/otherlink", `{"expires":""}`, headers); res.Code != 403 {
		t.Fatalf("non-owner edit: %d", res.Code)
	}
	if res := sessionCall(handler, "PUT", "share/otherlink", `{"expires":""}`, nil); res.Code != 401 {
		t.Fatalf("anonymous edit: %d", res.Code)
	}
	u.Perm.Admin = true
	if err := st.Users.Update(u, "Perm"); err != nil {
		t.Fatal(err)
	}
	if res := sessionCall(handler, "PUT", "share/otherlink", `{"expires":"1","unit":"days"}`, headers); res.Code != 200 {
		t.Fatalf("admin edit: %d %s", res.Code, res.Body.String())
	}
	for _, hash := range []string{"expired", "deleted"} {
		if res := sessionCall(handler, "PUT", "share/"+hash, `{"expires":""}`, headers); res.Code != 404 {
			t.Fatalf("resurrected %s: %d", hash, res.Code)
		}
	}
	u.Perm.Share = false
	if err := st.Users.Update(u, "Perm"); err != nil {
		t.Fatal(err)
	}
	if res := sessionCall(handler, "PUT", "share/otherlink", `{"expires":""}`, headers); res.Code != 403 {
		t.Fatalf("revoked share permission: %d", res.Code)
	}
}
