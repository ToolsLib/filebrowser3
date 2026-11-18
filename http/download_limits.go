package fbhttp

import (
	"context"
	"errors"
	"io"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/filebrowser/filebrowser/v2/settings"
	"github.com/filebrowser/filebrowser/v2/users"
)

const (
	maxDownloadBudgets    = 4096
	downloadBudgetIdle    = time.Minute
	downloadPolicyRefresh = time.Second
	downloadWriteChunk    = 32 << 10
)

var errDownloadCapacity = errors.New("download budget capacity reached; retry later")
var errDownloadPolicy = errors.New("invalid download speed policy")

// One controller belongs to one HTTP server, shared by every private/public
// content route. It intentionally does not coordinate independent processes.
type downloadController struct {
	mu      sync.Mutex
	budgets map[uint]*downloadBudget
	resolve func(uint) (int64, error)
}

type downloadBudget struct {
	mu      sync.Mutex
	rate    int64
	tokens  float64
	updated time.Time
	refresh time.Time
	// refs/idle are protected by the controller, never by a stream writer.
	refs int
	idle time.Time
}

func effectiveDownloadRate(set *settings.Settings, user *users.User) int64 {
	if user.DownloadRate != nil {
		return *user.DownloadRate
	}
	return set.DownloadRate
}

func downloadBurst(rate int64) int64 { return min(64<<10, max(1, rate/4)) }

func (c *downloadController) acquire(id uint, initial int64) (*downloadBudget, func(), error) {
	if !users.ValidDownloadRate(&initial) {
		return nil, nil, errDownloadPolicy
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	now := time.Now()
	for key, budget := range c.budgets {
		if budget.refs == 0 && now.Sub(budget.idle) >= downloadBudgetIdle {
			delete(c.budgets, key)
		}
	}
	budget := c.budgets[id]
	if budget != nil {
		// New requests use the current database policy immediately, even if
		// another stream still holds this user's previously unlimited bucket.
		budget.mu.Lock()
		budget.refresh = time.Time{}
		budget.mu.Unlock()
	}
	if budget == nil {
		if len(c.budgets) >= maxDownloadBudgets {
			return nil, nil, errDownloadCapacity
		}
		budget = &downloadBudget{rate: initial, tokens: float64(downloadBurst(initial)), updated: now, refresh: now.Add(downloadPolicyRefresh)}
		c.budgets[id] = budget
	}
	budget.refs++
	release := func() {
		c.mu.Lock()
		defer c.mu.Unlock()
		budget.refs--
		budget.idle = time.Now()
		budget.mu.Lock()
		unlimited := budget.rate == 0
		budget.mu.Unlock()
		if budget.refs == 0 && unlimited {
			delete(c.budgets, id)
		}
	}
	return budget, release, nil
}

// grant is called under budget.mu. Grants make progress at least once a second
// even at 1 B/s, keeping cancellation and idle deadlines responsive.
func (b *downloadBudget) grant(now time.Time, requested int) (int, time.Duration) {
	if b.rate == 0 {
		return requested, 0
	}
	burst := downloadBurst(b.rate)
	b.tokens = min(float64(burst), b.tokens+max(0, now.Sub(b.updated).Seconds())*float64(b.rate))
	b.updated = now
	chunk := int(min(int64(requested), int64(downloadWriteChunk), max(1, b.rate/16), burst))
	if b.tokens >= float64(chunk) {
		b.tokens -= float64(chunk)
		return chunk, 0
	}
	delay := time.Duration((float64(chunk) - b.tokens) / float64(b.rate) * float64(time.Second))
	return 0, max(delay, time.Microsecond)
}

func (c *downloadController) take(ctx context.Context, id uint, b *downloadBudget, requested int) (int, bool, error) {
	for {
		if err := ctx.Err(); err != nil {
			return 0, false, err
		}
		b.mu.Lock()
		now := time.Now()
		if !now.Before(b.refresh) {
			rate, err := c.resolve(id)
			if err != nil {
				b.mu.Unlock()
				return 0, false, err
			}
			if !users.ValidDownloadRate(&rate) {
				b.mu.Unlock()
				return 0, false, errDownloadPolicy
			}
			// Policy changes never mint a fresh burst on an existing budget.
			if b.rate > 0 {
				b.tokens = min(float64(downloadBurst(b.rate)), b.tokens+max(0, now.Sub(b.updated).Seconds())*float64(b.rate))
			} else {
				b.tokens = 0
			}
			b.rate, b.updated, b.refresh = rate, now, now.Add(downloadPolicyRefresh)
			b.tokens = min(b.tokens, float64(downloadBurst(rate)))
		}
		count, delay := b.grant(now, requested)
		limited := b.rate > 0
		b.mu.Unlock()
		if count > 0 {
			return count, limited, nil
		}
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return 0, false, ctx.Err()
		case <-timer.C:
		}
	}
}

type downloadWriter struct {
	http.ResponseWriter
	controller *downloadController
	budget     *downloadBudget
	ctx        context.Context
	userID     uint
	started    bool
	noStore    bool
	lastFlush  time.Time
	failure    error
}

func (w *downloadWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }
func (w *downloadWriter) WriteHeader(status int) {
	if w.noStore {
		w.Header().Set("Cache-Control", "private, no-store")
	}
	w.started = true
	w.ResponseWriter.WriteHeader(status)
}
func (w *downloadWriter) Flush() {
	if !w.started {
		w.WriteHeader(http.StatusOK)
	}
	if err := http.NewResponseController(w.ResponseWriter).Flush(); err != nil && !errors.Is(err, http.ErrNotSupported) {
		w.failure = err
	}
	w.lastFlush = time.Now()
}

func (w *downloadWriter) failed(count int, err error) (int, error) {
	w.failure = err
	return count, err
}

func (w *downloadWriter) abortIfFailed(status int, err error) {
	if !w.started {
		return
	}
	if err == nil {
		err = w.failure
	}
	if err == nil && status < 400 {
		return
	}
	if !errors.Is(err, context.Canceled) {
		log.Printf("download interrupted: status=%d error=%v", status, err)
	}
	// Close HTTP/1 streams/reset HTTP/2 streams: a partial archive must not be
	// reported as a successful chunked response or receive an appended error page.
	panic(http.ErrAbortHandler)
}

// Do not expose io.ReaderFrom: io.Copy/ServeContent fast paths must pass through
// Write, including Range responses and os.File.WriteTo.
func (w *downloadWriter) Write(body []byte) (int, error) {
	if w.noStore && !w.started {
		w.Header().Set("Cache-Control", "private, no-store")
	}
	if w.controller == nil {
		w.started = true
		n, err := w.ResponseWriter.Write(body)
		if err != nil {
			return w.failed(n, err)
		}
		return n, nil
	}
	written := 0
	for len(body) > 0 {
		count, limited, err := w.controller.take(w.ctx, w.userID, w.budget, len(body))
		if err != nil {
			return w.failed(written, err)
		}
		w.started = true
		n, err := w.ResponseWriter.Write(body[:count])
		written += n
		body = body[n:]
		if err != nil {
			return w.failed(written, err)
		}
		if n != count {
			return w.failed(written, io.ErrShortWrite)
		}
		// Small paced writes must reach the socket instead of sitting in the
		// HTTP buffer long enough for a proxy to consider the stream idle.
		if limited && time.Since(w.lastFlush) >= 250*time.Millisecond {
			if err := http.NewResponseController(w.ResponseWriter).Flush(); err != nil && !errors.Is(err, http.ErrNotSupported) {
				return w.failed(written, err)
			}
			w.lastFlush = time.Now()
		}
	}
	return written, nil
}

func limitDownload(w http.ResponseWriter, r *http.Request, d *data) (*downloadWriter, func(), error) {
	if d.downloads == nil {
		return &downloadWriter{ResponseWriter: w}, func() {}, nil
	}
	budget, release, err := d.downloads.acquire(d.user.ID, effectiveDownloadRate(d.settings, d.user))
	if err != nil {
		w.Header().Set("Retry-After", "1")
		return nil, nil, err
	}
	return &downloadWriter{ResponseWriter: w, controller: d.downloads, budget: budget, ctx: r.Context(), userID: d.user.ID, noStore: effectiveDownloadRate(d.settings, d.user) > 0}, release, nil
}

func withDownload(fn handleFunc) handleFunc {
	return func(w http.ResponseWriter, r *http.Request, d *data) (int, error) {
		if !d.user.Perm.Download {
			return fn(w, r, d)
		}
		stream, release, err := limitDownload(w, r, d)
		if err != nil {
			return http.StatusServiceUnavailable, err
		}
		defer release()
		status, err := fn(stream, r, d)
		stream.abortIfFailed(status, err)
		return status, err
	}
}
