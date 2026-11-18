package fbhttp

import (
	"bytes"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/filebrowser/filebrowser/v2/settings"
)

const maxMetadataBytes = 1 << 20
const streamIdleTimeout = time.Minute

func uploadSizeLimit(server *settings.Server) int64 {
	if server.MaxUploadSize <= 0 {
		return maxUploadLength // The authenticated handler applies the global/user policy.
	}
	if server.MaxUploadSize > maxUploadLength {
		return maxUploadLength
	}
	return server.MaxUploadSize
}

type requestBucket struct {
	tokens float64
	last   time.Time
}
type authLimiter struct {
	mu      sync.Mutex
	buckets map[string]requestBucket
	swept   time.Time
}

func (l *authLimiter) allow(key string, now time.Time, rate, burst float64) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if now.Sub(l.swept) > time.Minute {
		for key, bucket := range l.buckets {
			if now.Sub(bucket.last) > 10*time.Minute {
				delete(l.buckets, key)
			}
		}
		l.swept = now
	}
	bucket, exists := l.buckets[key]
	if !exists {
		if len(l.buckets) >= 4096 {
			return false
		}
		bucket = requestBucket{tokens: burst, last: now}
	}
	bucket.tokens = min(burst, bucket.tokens+now.Sub(bucket.last).Seconds()*rate)
	bucket.last = now
	allowed := bucket.tokens >= 1
	if allowed {
		bucket.tokens--
	}
	l.buckets[key] = bucket
	return allowed
}

type idleBody struct {
	io.ReadCloser
	controller *http.ResponseController
}

func (b *idleBody) Read(buffer []byte) (int, error) {
	_ = b.controller.SetReadDeadline(time.Now().Add(streamIdleTimeout))
	return b.ReadCloser.Read(buffer)
}

// Keep deadline updates on streaming writes, rather than imposing a maximum
// transfer duration. Unwrap preserves ResponseController support for flushing.
type idleWriter struct {
	http.ResponseWriter
	controller *http.ResponseController
}

func (w *idleWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }
func (w *idleWriter) Write(body []byte) (int, error) {
	_ = w.controller.SetWriteDeadline(time.Now().Add(streamIdleTimeout))
	return w.ResponseWriter.Write(body)
}
func (w *idleWriter) Flush() {
	_ = w.controller.SetWriteDeadline(time.Now().Add(streamIdleTimeout))
	_ = w.controller.Flush()
}

func requestLimits(next http.Handler, server *settings.Server) http.Handler {
	limiter := &authLimiter{buckets: make(map[string]requestBucket)}
	requests := make(chan struct{}, 64)
	authentication := make(chan struct{}, 4)
	work := make(chan struct{}, 8)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/api/") {
			next.ServeHTTP(w, r)
			return
		}
		reject := func() {
			w.Header().Set("Retry-After", "4")
			http.Error(w, "429 Too Many Requests", http.StatusTooManyRequests)
		}
		select {
		case requests <- struct{}{}:
			defer func() { <-requests }()
		default:
			reject()
			return
		}
		login := r.URL.Path == "/api/login" || r.URL.Path == "/api/signup"
		if login || r.URL.Path == "/api/renew" {
			peer, _, err := net.SplitHostPort(r.RemoteAddr)
			if err != nil {
				peer = r.RemoteAddr
			}
			rate, burst := 0.25, 10.0
			kind := "login"
			if !login {
				rate, burst, kind = 2, 20, "renew"
			}
			if !limiter.allow(kind+":"+peer, time.Now(), rate, burst) {
				reject()
				return
			}
		}
		if login {
			select {
			case authentication <- struct{}{}:
				defer func() { <-authentication }()
			default:
				reject()
				return
			}
		}
		expensive := strings.HasPrefix(r.URL.Path, "/api/search") || strings.HasPrefix(r.URL.Path, "/api/preview/") || strings.HasPrefix(r.URL.Path, "/api/raw") || strings.HasPrefix(r.URL.Path, "/api/public/dl/") || strings.HasPrefix(r.URL.Path, "/api/command") || strings.HasPrefix(r.URL.Path, "/api/subtitle") || strings.HasPrefix(r.URL.Path, "/api/resources/recursive")
		if expensive {
			select {
			case work <- struct{}{}:
				defer func() { <-work }()
			default:
				reject()
				return
			}
		}
		controller := http.NewResponseController(w)
		_ = controller.SetReadDeadline(time.Now().Add(streamIdleTimeout))
		originalBody := r.Body
		defer func() {
			// Keep a deadline while the server drains an early-rejected body.
			if originalBody != nil {
				_ = originalBody.Close()
			}
			_ = controller.SetReadDeadline(time.Time{})
		}()
		defer controller.SetWriteDeadline(time.Time{}) //nolint:errcheck
		if r.Body != nil {
			stream := ((r.Method == http.MethodPost || r.Method == http.MethodPut) && strings.HasPrefix(r.URL.Path, "/api/resources")) || (r.Method == http.MethodPatch && strings.HasPrefix(r.URL.Path, "/api/tus"))
			maximum := int64(maxMetadataBytes)
			if stream {
				maximum = uploadSizeLimit(server)
			}
			if r.ContentLength > maximum {
				if stream {
					writeUploadLimitError(w, maximum)
					return
				}
				http.Error(w, "413 Request Entity Too Large", http.StatusRequestEntityTooLarge)
				return
			}
			r.Body = http.MaxBytesReader(w, &idleBody{ReadCloser: r.Body, controller: controller}, maximum)
			if !stream {
				body, err := io.ReadAll(r.Body)
				if err != nil {
					http.Error(w, "413 Request Entity Too Large", http.StatusRequestEntityTooLarge)
					return
				}
				r.Body = io.NopCloser(bytes.NewReader(body))
			}
		}
		// The WebSocket handler manages its own deadlines and needs Hijacker.
		if strings.HasPrefix(r.URL.Path, "/api/command") {
			next.ServeHTTP(w, r)
			return
		}
		next.ServeHTTP(&idleWriter{ResponseWriter: w, controller: controller}, r)
	})
}
