package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestReCaptchaBoundsResponseAndHonorsCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(strings.Repeat(" ", 65<<10) + `{"success":true}`))
	}))
	defer server.Close()
	captcha := &ReCaptcha{Host: server.URL, Secret: "test"}
	if ok, err := captcha.Ok("test"); ok || err == nil {
		t.Fatal("oversized verification response accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := captcha.OkContext(ctx, "test"); err == nil {
		t.Fatal("canceled verification continued")
	}
}
