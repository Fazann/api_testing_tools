package runner

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"api-test-kits/internal/domain"
	"api-test-kits/internal/httpclient"
)

// The gate releases every copy together: the server sees all of them in
// flight at once even though each pre-request script takes a different time.
func TestRunBurstReleasesTogether(t *testing.T) {
	var inFlight, peak int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&inFlight, 1)
		for {
			p := atomic.LoadInt32(&peak)
			if n <= p || atomic.CompareAndSwapInt32(&peak, p, n) {
				break
			}
		}
		time.Sleep(50 * time.Millisecond)
		atomic.AddInt32(&inFlight, -1)
		w.WriteHeader(200)
	}))
	defer srv.Close()
	req := domain.Request{Name: "burst", Method: "POST", URLTemplate: srv.URL,
		// Uneven script durations; without a gate the first copies would finish early.
		PreRequestScript: `var end = Date.now() + Math.floor(Math.random() * 40); while (Date.now() < end) {}`}
	results := New(client(t)).RunBurst(context.Background(), req, NewScope(nil), 8)
	for i, r := range results {
		if r.Status != domain.StatusPassed {
			t.Fatalf("copy %d: %s %s", i, r.Status, r.Error)
		}
	}
	if peak != 8 {
		t.Fatalf("expected all 8 in flight at once, peak was %d", peak)
	}
}

type okSender struct{ calls int32 }

func (s *okSender) Do(ctx context.Context, req httpclient.Request) (*httpclient.Response, error) {
	atomic.AddInt32(&s.calls, 1)
	return &httpclient.Response{StatusCode: 200}, nil
}

// A copy that fails before sending (script error, missing variable) must not
// keep the others waiting at the gate.
func TestGateReleasesWhenACopyNeverSends(t *testing.T) {
	next := &okSender{}
	g := &gate{next: next, pending: 3, open: make(chan struct{})}
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			arrived := new(bool)
			defer g.leave(arrived)
			if _, err := g.Do(context.WithValue(context.Background(), gateKey{}, arrived), httpclient.Request{}); err != nil {
				t.Error(err)
			}
		}()
	}
	time.Sleep(20 * time.Millisecond)
	if atomic.LoadInt32(&next.calls) != 0 {
		t.Fatal("requests were sent before every copy was ready")
	}
	g.leave(new(bool)) // the third copy failed before sending
	waited := make(chan struct{})
	go func() { wg.Wait(); close(waited) }()
	select {
	case <-waited:
	case <-time.After(2 * time.Second):
		t.Fatal("the gate deadlocked")
	}
	if next.calls != 2 {
		t.Fatalf("expected 2 sends, got %d", next.calls)
	}
}

func TestRunBurstCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	results := New(client(t)).RunBurst(ctx, domain.Request{Name: "x", Method: "GET", URLTemplate: "http://127.0.0.1:1"}, NewScope(nil), 3)
	for _, r := range results {
		if r.Status != domain.StatusCancelled {
			t.Fatalf("expected cancelled, got %s", r.Status)
		}
	}
}
