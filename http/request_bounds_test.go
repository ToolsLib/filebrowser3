package fbhttp

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/filebrowser/filebrowser/v2/files"
	"github.com/filebrowser/filebrowser/v2/settings"
	"github.com/filebrowser/filebrowser/v2/users"
	"github.com/gorilla/websocket"
	"github.com/spf13/afero"
)

func TestCommandAuthorizationBeforeUpgrade(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		key := []byte("command-test-key")
		perm := users.Permissions{}
		st := scopedUserStorage(t, t.TempDir(), perm, key)
		req := httptest.NewRequest("GET", "/", http.NoBody)
		req.Header.Set("X-Auth", signToken(t, perm, key))
		rec := httptest.NewRecorder()
		handle(commandsHandler, "", st, &settings.Server{EnableExec: enabled}).ServeHTTP(rec, req)
		if rec.Code != http.StatusForbidden {
			t.Fatalf("enabled=%v: status=%d, want 403 before websocket parsing", enabled, rec.Code)
		}
	}
}

func TestCommandMessageSizeIsBounded(t *testing.T) {
	key := []byte("command-test-key")
	perm := users.Permissions{Execute: true}
	st := scopedUserStorage(t, t.TempDir(), perm, key)
	server := httptest.NewServer(handle(commandsHandler, "", st, &settings.Server{EnableExec: true}))
	defer server.Close()
	header := http.Header{"X-Auth": []string{signToken(t, perm, key)}}
	conn, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http"), header)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	if err := conn.WriteMessage(websocket.TextMessage, []byte(strings.Repeat("x", maxCommandBytes+1))); err != nil {
		t.Fatal(err)
	}
	if _, _, err := conn.ReadMessage(); err == nil {
		t.Fatal("oversized command was accepted")
	}
}

func TestSubtitleInputAndOutputLimits(t *testing.T) {
	fs := afero.NewMemMapFs()
	f, err := fs.Create("/large.srt")
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Truncate(maxSubtitleInput + 1); err != nil {
		t.Fatal(err)
	}
	_ = f.Close()
	status, err := subtitleFileHandler(httptest.NewRecorder(), httptest.NewRequest("GET", "/", http.NoBody), &files.FileInfo{Fs: fs, Path: "/large.srt", Name: "large.srt"})
	if status != 413 || !errors.Is(err, errSubtitleLimit) {
		t.Fatalf("input limit: %d %v", status, err)
	}
	buf := &subtitleBuffer{}
	if _, err := buf.WriteString(strings.Repeat("x", maxSubtitleOutput)); err != nil {
		t.Fatal(err)
	}
	if _, err := buf.WriteString("x"); !errors.Is(err, errSubtitleLimit) {
		t.Fatalf("output limit bypass: %v", err)
	}
	if buf.Len() != maxSubtitleOutput {
		t.Fatalf("buffer grew past limit: %d", buf.Len())
	}
}
