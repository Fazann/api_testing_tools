package backend

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"
)

// ErrOffline is returned without contacting the backend while it is known to
// be down, so the app never waits on a dead server again and again.
var ErrOffline = errors.New("the backend is offline")

// breaker remembers that the backend is down. After a network error, a
// timeout or a 502/503/504, requests fail at once with ErrOffline for the
// cool-down. After that, the next request tries the server again.
type breaker struct {
	mu        sync.Mutex
	cooldown  time.Duration
	openUntil time.Time
	lastErr   string
	now       func() time.Time
}

func newBreaker(cooldown time.Duration) *breaker {
	return &breaker{cooldown: cooldown, now: time.Now}
}

// offline reports whether requests are being refused, and why.
func (b *breaker) offline() (bool, string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.now().Before(b.openUntil), b.lastErr
}

func (b *breaker) interceptor() Interceptor {
	return func(req *http.Request, next Handler) (*http.Response, error) {
		if off, why := b.offline(); off {
			return nil, fmt.Errorf("%w: %s", ErrOffline, why)
		}
		resp, err := next(req)
		switch {
		case err != nil && errors.Is(err, context.Canceled):
			// The caller gave up; that says nothing about the server.
		case err != nil:
			b.trip(err.Error())
		case retryable(resp, nil):
			b.trip(resp.Status)
		default:
			b.reset()
		}
		return resp, err
	}
}

func (b *breaker) trip(why string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.openUntil, b.lastErr = b.now().Add(b.cooldown), why
}

func (b *breaker) reset() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.openUntil, b.lastErr = time.Time{}, ""
}
