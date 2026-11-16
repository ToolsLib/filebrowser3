package fbhttp

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/filebrowser/filebrowser/v2/settings"
)

func TestRequestLimitsBodiesAndForwardedAddressSpoofing(t *testing.T) {
	called := 0
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { called++; _, _ = io.Copy(w, r.Body) })
	handler := requestLimits(next, &settings.Server{MaxUploadSize: 10})
	for _, entry := range []struct {
		path string
		size int
	}{{"/api/users", maxMetadataBytes + 1}, {"/api/resources/file", 11}} {
		r := httptest.NewRequest("POST", entry.path, bytes.NewReader(make([]byte, entry.size)))
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != 413 {
			t.Fatalf("%s oversized: %d", entry.path, w.Code)
		}
	}
	if called != 0 {
		t.Fatal("oversized body reached handler")
	}
	for i := 0; i < 11; i++ {
		r := httptest.NewRequest("POST", "/api/login", http.NoBody)
		r.RemoteAddr = "192.0.2.1:1234"
		r.Header.Set("X-Forwarded-For", "198.51.100."+strconv.Itoa(i))
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if i == 10 && (w.Code != 429 || w.Header().Get("Retry-After") == "") {
			t.Fatalf("forwarded headers bypassed peer throttle: %d", w.Code)
		}
	}
}

func TestRequestLimitsBoundConcurrentWork(t *testing.T) {
	entered, release := make(chan struct{}, 8), make(chan struct{})
	handler := requestLimits(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) { entered <- struct{}{}; <-release }), &settings.Server{})
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/api/raw/file", http.NoBody))
		}()
	}
	for i := 0; i < 8; i++ {
		select {
		case <-entered:
		case <-time.After(time.Second):
			t.Fatal("worker did not enter")
		}
	}
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest("GET", "/api/raw/file", http.NoBody))
	close(release)
	wg.Wait()
	if w.Code != 429 {
		t.Fatalf("unbounded concurrent work: %d", w.Code)
	}
}

func TestAuthLimiterIsBoundedAndRecovers(t *testing.T) {
	limiter := &authLimiter{buckets: map[string]requestBucket{}}
	now := time.Now()
	for i := 0; i < 4096; i++ {
		if !limiter.allow(strconv.Itoa(i), now, 1, 1) {
			t.Fatal("unexpected early cap")
		}
	}
	if limiter.allow("overflow", now, 1, 1) || len(limiter.buckets) != 4096 {
		t.Fatal("unbounded peer bookkeeping")
	}
	if !limiter.allow("overflow", now.Add(11*time.Minute), 1, 1) {
		t.Fatal("expired peers did not release capacity")
	}
}

type deadlineResponse struct {
	*httptest.ResponseRecorder
	reads, writes []time.Time
}

func (r *deadlineResponse) SetReadDeadline(at time.Time) error {
	r.reads = append(r.reads, at)
	return nil
}
func (r *deadlineResponse) SetWriteDeadline(at time.Time) error {
	r.writes = append(r.writes, at)
	return nil
}

func TestStreamingDeadlinesFollowProgressAndReset(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = io.Copy(w, r.Body) })
	handler := requestLimits(next, &settings.Server{})
	w := &deadlineResponse{ResponseRecorder: httptest.NewRecorder()}
	handler.ServeHTTP(w, httptest.NewRequest("POST", "/api/resources/file", bytes.NewBufferString("stream")))
	if w.Body.String() != "stream" || len(w.reads) < 2 || len(w.writes) < 2 {
		t.Fatal("stream was not deadline protected")
	}
	if !w.reads[len(w.reads)-1].IsZero() || !w.writes[len(w.writes)-1].IsZero() {
		t.Fatal("deadlines leaked into the next keep-alive request")
	}
	if time.Until(w.reads[0]) < 50*time.Second || time.Until(w.writes[0]) < 50*time.Second {
		t.Fatal("deadline was not extended on progress")
	}
}
