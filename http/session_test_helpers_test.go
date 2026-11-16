package fbhttp

import (
	"testing"
	"time"

	"github.com/filebrowser/filebrowser/v2/sessions"
	"github.com/filebrowser/filebrowser/v2/storage"
)

func seedSession(t *testing.T, st *storage.Storage, id uint) string {
	t.Helper()
	user, err := st.Users.Get("", false, id)
	if err != nil {
		t.Fatal(err)
	}
	settings, err := st.Settings.Get()
	if err != nil {
		t.Fatal(err)
	}
	sessionID, err := sessionSecret()
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Sessions.Create(&sessions.Session{ID: sessionID, UserID: id, Credential: credentialFingerprint(user, settings), RefreshHash: secretHash("test-refresh"), Expires: time.Now().Add(2 * time.Hour), AbsoluteExpires: time.Now().Add(sessions.AbsoluteLifetime)}); err != nil {
		t.Fatal(err)
	}
	return sessionID
}
