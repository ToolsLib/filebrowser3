package fbhttp

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/filebrowser/filebrowser/v2/files"
	"github.com/spf13/afero"
)

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
