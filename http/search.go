package fbhttp

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/filebrowser/filebrowser/v2/search"
)

var errSearchPage = errors.New("search page complete")

var searchHandler = withUser(func(w http.ResponseWriter, r *http.Request, d *data) (int, error) {
	limit, offset := 200, 0
	var err error
	if raw := r.URL.Query().Get("limit"); raw != "" {
		limit, err = strconv.Atoi(raw)
		if err != nil || limit < 1 || limit > 1000 {
			return 400, nil
		}
	}
	if raw := r.URL.Query().Get("cursor"); raw != "" {
		offset, err = strconv.Atoi(raw)
		if err != nil || offset < 0 || offset > 1000000 {
			return 400, nil
		}
	}
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
	defer cancel()
	type result struct {
		Dir  bool   `json:"dir"`
		Path string `json:"path"`
	}
	results := make(chan result, 64)
	finished := make(chan struct{})
	var walkErr error
	go func() {
		matched, sent := 0, 0
		walkErr = search.Search(ctx, d.user.Fs, r.URL.Path, r.URL.Query().Get("query"), d, func(path string, f os.FileInfo) error {
			matched++
			if matched <= offset {
				return nil
			}
			if sent == limit {
				return errSearchPage
			}
			select {
			case results <- result{f.IsDir(), path}:
				sent++
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		})
		close(results)
		close(finished)
	}()
	// Only this goroutine writes. Cancellation also waits for the walker to exit.
	defer func() { cancel(); <-finished }()
	w.Header().Set("Content-Type", "application/x-ndjson")
	w.Header().Set("Cache-Control", "private, no-store")
	encoder := json.NewEncoder(w)
	ticker := time.NewTicker(35 * time.Millisecond)
	defer ticker.Stop()
	heartbeat := time.NewTicker(5 * time.Second)
	defer heartbeat.Stop()
	flush := func() {
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
	}
	sent, pending := 0, 0
	for {
		select {
		case item, ok := <-results:
			if !ok {
				<-finished
				if walkErr != nil && !errors.Is(walkErr, errSearchPage) && !errors.Is(walkErr, context.Canceled) {
					return 0, walkErr
				}
				next := 0
				if errors.Is(walkErr, errSearchPage) {
					next = offset + sent
				}
				if err := encoder.Encode(map[string]interface{}{"type": "page", "nextCursor": next}); err != nil {
					return 0, err
				}
				flush()
				return 0, nil
			}
			if err := encoder.Encode(item); err != nil {
				return 0, err
			}
			sent++
			pending++
			if sent == 1 || pending >= 64 {
				flush()
				pending = 0
			}
		case <-ticker.C:
			if pending > 0 {
				flush()
				pending = 0
			}
		case <-heartbeat.C:
			if _, err := w.Write([]byte("\n")); err != nil {
				return 0, err
			}
			flush()
		case <-ctx.Done():
			return 0, nil
		}
	}
})
