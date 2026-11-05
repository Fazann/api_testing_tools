package runner

import (
	"context"
	"sync"

	"api-test-kits/internal/domain"
	"api-test-kits/internal/httpclient"
)

// RunBurst runs n copies of req at the same moment, for race-condition tests.
// Every copy runs its pre-request scripts first (so each can sign its own
// request or make its own idempotency key); the requests are then held at a
// start gate and released together once every copy is ready or has failed
// before sending. Results are in copy order. No baseline is compared.
func (r *Runner) RunBurst(ctx context.Context, req domain.Request, scope *Scope, n int) []domain.RunResult {
	if n < 1 {
		return nil
	}
	g := &gate{next: r.client, pending: n, open: make(chan struct{})}
	gated := &Runner{client: g, now: r.now}
	results := make([]domain.RunResult, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			arrived := new(bool)
			defer g.leave(arrived)
			results[i] = gated.RunScoped(context.WithValue(ctx, gateKey{}, arrived), req, scope, nil)
		}(i)
	}
	wg.Wait()
	return results
}

type gateKey struct{}

// gate holds every Do call until all n participants have either reached Do
// or finished without reaching it, then lets them all through at once.
type gate struct {
	next    Sender
	mu      sync.Mutex
	pending int
	open    chan struct{}
}

func (g *gate) countDown() {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.pending--
	if g.pending == 0 {
		close(g.open)
	}
}

func (g *gate) Do(ctx context.Context, req httpclient.Request) (*httpclient.Response, error) {
	if arrived, ok := ctx.Value(gateKey{}).(*bool); ok && !*arrived {
		*arrived = true
		g.countDown()
	}
	select {
	case <-g.open:
	case <-ctx.Done():
		return nil, &httpclient.Error{Kind: httpclient.ErrCancelled, Err: ctx.Err()}
	}
	return g.next.Do(ctx, req)
}

// leave accounts for a participant that finished without sending, for
// example because its pre-request script failed.
func (g *gate) leave(arrived *bool) {
	if !*arrived {
		*arrived = true
		g.countDown()
	}
}
