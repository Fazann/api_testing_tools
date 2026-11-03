package runner

import (
	"context"
	"sync"

	"api-test-kits/internal/domain"
)

// MaxConcurrency bounds parallel requests so a collection run cannot flood a
// target or exhaust local resources.
const MaxConcurrency = 8

// Item is one request in a collection run with its optional baseline.
type Item struct {
	Request  domain.Request
	Baseline *domain.Baseline
}

// RunAll executes items with at most concurrency requests in flight and
// returns results in the same order as items. With concurrency 1 requests run
// strictly in order, which matters for business flows that depend on state.
// progress, if set, is called once per finished item, serialized, in
// completion order.
// scope may be nil when no environment or collection applies.
func (r *Runner) RunAll(ctx context.Context, items []Item, scope *Scope, concurrency int, progress func(index int, res domain.RunResult)) []domain.RunResult {
	if concurrency < 1 {
		concurrency = 1
	}
	if concurrency > MaxConcurrency {
		concurrency = MaxConcurrency
	}
	results := make([]domain.RunResult, len(items))
	var mu sync.Mutex
	report := func(i int, res domain.RunResult) {
		results[i] = res
		if progress != nil {
			mu.Lock()
			progress(i, res)
			mu.Unlock()
		}
	}

	if concurrency == 1 {
		for i, it := range items {
			report(i, r.RunScoped(ctx, it.Request, scope, it.Baseline))
		}
		return results
	}

	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup
	for i, it := range items {
		// Once cancelled, remaining items finish immediately as cancelled,
		// so they do not need a slot.
		acquired := false
		select {
		case sem <- struct{}{}:
			acquired = true
		case <-ctx.Done():
		}
		wg.Add(1)
		go func(i int, it Item, acquired bool) {
			defer wg.Done()
			if acquired {
				defer func() { <-sem }()
			}
			report(i, r.RunScoped(ctx, it.Request, scope, it.Baseline))
		}(i, it, acquired)
	}
	wg.Wait()
	return results
}
