package fbhttp

import (
	"context"
	"errors"
	"sync"
)

var errPreviewBusy = errors.New("preview queue is full")

type previewFlight struct {
	ctx     context.Context
	cancel  context.CancelFunc
	done    chan struct{}
	waiters int
	value   []byte
	err     error
}

var previewFlights = struct {
	sync.Mutex
	active map[string]*previewFlight
}{active: make(map[string]*previewFlight)}

// A shared resize lives while at least one interested request is waiting.
// The map is bounded and completed/canceled entries do not accumulate.
func sharedPreview(ctx context.Context, key string, create func(context.Context) ([]byte, error)) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	previewFlights.Lock()
	flight := previewFlights.active[key]
	if flight != nil && flight.ctx.Err() != nil {
		delete(previewFlights.active, key)
		flight = nil
	}
	if flight == nil {
		if len(previewFlights.active) >= 128 {
			previewFlights.Unlock()
			return nil, errPreviewBusy
		}
		work, cancel := context.WithCancel(context.Background())
		flight = &previewFlight{ctx: work, cancel: cancel, done: make(chan struct{})}
		previewFlights.active[key] = flight
		go func(f *previewFlight) {
			value, err := create(f.ctx)
			previewFlights.Lock()
			f.value = value
			f.err = err
			if previewFlights.active[key] == f {
				delete(previewFlights.active, key)
			}
			close(f.done)
			f.cancel()
			previewFlights.Unlock()
		}(flight)
	}
	flight.waiters++
	previewFlights.Unlock()
	defer func() {
		previewFlights.Lock()
		flight.waiters--
		if flight.waiters == 0 {
			flight.cancel()
		}
		previewFlights.Unlock()
	}()
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-flight.done:
		return flight.value, flight.err
	}
}
