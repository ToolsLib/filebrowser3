//go:build performance_review

package fbhttp

import (
	"context"
	"fmt"
	"io"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/filebrowser/filebrowser/v2/files"
	"github.com/filebrowser/filebrowser/v2/img"
	"github.com/spf13/afero"
)

type performanceEmptyPreviewCache struct{}

func (performanceEmptyPreviewCache) Load(context.Context, string) ([]byte, bool, error) {
	return nil, false, nil
}
func (performanceEmptyPreviewCache) Store(context.Context, string, []byte) error { return nil }
func (performanceEmptyPreviewCache) Delete(context.Context, string) error        { return nil }

type performancePreviewService struct {
	calls   atomic.Int32
	started chan struct{}
	release chan struct{}
}

func (*performancePreviewService) FormatFromExtension(string) (img.Format, error) {
	return img.FormatJpeg, nil
}
func (s *performancePreviewService) Resize(ctx context.Context, _ io.Reader, _, _ int, out io.Writer, _ ...img.Option) error {
	s.calls.Add(1)
	s.started <- struct{}{}
	select {
	case <-s.release:
	case <-ctx.Done():
		return ctx.Err()
	}
	_, err := out.Write([]byte("fixture preview"))
	return err
}

// Reports work amplification with cold/no-op cache, not an image-speed benchmark.
func TestPerformancePreviewColdConcurrency(t *testing.T) {
	const requests = 8
	fs := afero.NewMemMapFs()
	if err := afero.WriteFile(fs, "/photo.jpg", []byte("fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	info := &files.FileInfo{Fs: fs, Path: "/photo.jpg", Name: "photo.jpg", Extension: ".jpg", ModTime: time.Now()}
	svc := &performancePreviewService{started: make(chan struct{}, requests), release: make(chan struct{})}
	var wg sync.WaitGroup
	errors := make(chan error, requests)
	for n := 0; n < requests; n++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			w := httptest.NewRecorder()
			r := httptest.NewRequest("GET", "/", nil)
			status, err := handleImagePreview(w, r, svc, performanceEmptyPreviewCache{}, info, PreviewSizeThumb, true, true)
			if err != nil {
				errors <- err
			} else if status != 0 || w.Code != 200 {
				errors <- fmt.Errorf("unexpected status %d/%d", status, w.Code)
			}
		}()
	}
	deadline := time.NewTimer(3 * time.Second)
	defer deadline.Stop()
awaitStarted:
	for n := 0; n < requests; n++ {
		select {
		case <-svc.started:
		case <-deadline.C:
			break awaitStarted
		}
	}
	close(svc.release)
	wg.Wait()
	close(errors)
	for err := range errors {
		t.Fatal(err)
	}
	t.Logf("%d simultaneous requests for one thumbnail invoked Resize %d times", requests, svc.calls.Load())
}
