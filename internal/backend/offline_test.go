package backend

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

var epItem = Endpoint{Method: "GET", Path: "/items/{id}"}

func TestMockModeAnswersWithoutNetwork(t *testing.T) {
	var seenUA string
	mock := NewMock().
		Handle(epItem, func(r *http.Request) (any, error) {
			if r.PathValue("id") == "missing" {
				return nil, &APIError{Status: http.StatusNotFound, Code: "not_found", Message: "no such item"}
			}
			return map[string]string{"id": r.PathValue("id")}, nil
		})
	ua := func(req *http.Request, next Handler) (*http.Response, error) {
		seenUA = "seen"
		return next(req)
	}
	c, err := New(Options{Mock: mock, Interceptors: []Interceptor{ua}})
	if err != nil {
		t.Fatal(err)
	}
	if c.Mode() != ModeMock || c.Configured() {
		t.Fatalf("mode %s configured=%v", c.Mode(), c.Configured())
	}

	got, err := Call[map[string]string](context.Background(), c, epItem, Request{PathParams: map[string]string{"id": "7"}})
	if err != nil || got["id"] != "7" || seenUA != "seen" {
		t.Fatalf("got %v, %v; interceptors ran: %q", got, err, seenUA)
	}
	_, err = Call[struct{}](context.Background(), c, epItem, Request{PathParams: map[string]string{"id": "missing"}})
	if apiErr, ok := errors.AsType[*APIError](err); !ok || apiErr.Status != 404 || apiErr.Code != "not_found" {
		t.Fatalf("mocked error: %v", err)
	}
	_, err = Call[struct{}](context.Background(), c, Endpoint{"GET", "/unmocked"}, Request{})
	if apiErr, ok := errors.AsType[*APIError](err); !ok || apiErr.Code != "no_mock" {
		t.Fatalf("unmocked route: %v", err)
	}
}

func TestDefaultMockStatus(t *testing.T) {
	c, _ := New(Options{Mock: DefaultMock()})
	st := c.CheckStatus(context.Background())
	if st.Mode != ModeMock || !st.Reachable || st.Error != "" || st.Configured {
		t.Fatalf("status: %+v", st)
	}
}

func TestMockDelayRespectsCancel(t *testing.T) {
	c, _ := New(Options{Mock: DefaultMock().WithDelay(time.Hour)})
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	start := time.Now()
	if err := c.Health(ctx); !errors.Is(err, context.DeadlineExceeded) || time.Since(start) > time.Second {
		t.Fatalf("got %v after %s", err, time.Since(start))
	}
}

func TestBadURLFallsBackToMock(t *testing.T) {
	c, err := NewOrDisabled(Options{BaseURL: "localhost:8080", Mock: DefaultMock()})
	if err == nil || c.Mode() != ModeMock {
		t.Fatalf("want mock mode and an error, got %s, %v", c.Mode(), err)
	}
	st := c.CheckStatus(context.Background())
	if !st.Reachable || !strings.Contains(st.Error, "must be an http(s) URL") {
		t.Fatalf("status should work from the mock and still show the URL problem: %+v", st)
	}
}

// server counts requests and answers {"n": <count>} or fails while down is set.
func server(t *testing.T) (*httptest.Server, *atomic.Int32, *atomic.Bool) {
	var hits atomic.Int32
	var down atomic.Bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		n := hits.Add(1)
		if down.Load() {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"n":` + string('0'+rune(n)) + `}`))
	}))
	t.Cleanup(srv.Close)
	return srv, &hits, &down
}

func TestCacheFirstThenStaleWhileDown(t *testing.T) {
	srv, hits, down := server(t)
	cache := NewCache(CacheOptions{Fresh: time.Minute})
	now := time.Now()
	cache.now = func() time.Time { return now }
	c, _ := New(Options{BaseURL: srv.URL, Cache: cache, OfflineCooldown: -1})
	get := func() (map[string]int, error) {
		return Call[map[string]int](context.Background(), c, Endpoint{"GET", "/items"}, Request{})
	}

	if v, err := get(); err != nil || v["n"] != 1 {
		t.Fatalf("first: %v, %v", v, err)
	}
	if v, err := get(); err != nil || v["n"] != 1 || hits.Load() != 1 {
		t.Fatalf("fresh entry should be served without a request: %v, %v, hits %d", v, err, hits.Load())
	}

	now = now.Add(2 * time.Minute) // no longer fresh: ask the server
	if v, _ := get(); v["n"] != 2 {
		t.Fatalf("expired entry should be refreshed, got %v", v)
	}

	now = now.Add(2 * time.Minute)
	down.Store(true)
	if v, err := get(); err != nil || v["n"] != 2 {
		t.Fatalf("server down: want the stale copy, got %v, %v", v, err)
	}

	_, err := Call[struct{}](context.Background(), c, Endpoint{"GET", "/items"}, Request{NoCache: true})
	if apiErr, ok := errors.AsType[*APIError](err); !ok || apiErr.Status != 503 {
		t.Fatalf("NoCache must reach the server: %v", err)
	}
}

func TestCacheSkipsWritesAndSeparatesUsers(t *testing.T) {
	srv, hits, _ := server(t)
	token := "alice"
	c, _ := New(Options{
		BaseURL: srv.URL, Cache: NewCache(CacheOptions{}), OfflineCooldown: -1,
		Interceptors: []Interceptor{BearerAuth(func() string { return token })},
	})
	ep := Endpoint{"GET", "/me"}
	_, _ = Call[map[string]int](context.Background(), c, ep, Request{})
	_, _ = Call[map[string]int](context.Background(), c, Endpoint{"POST", "/me"}, Request{})
	_, _ = Call[map[string]int](context.Background(), c, Endpoint{"POST", "/me"}, Request{})
	token = "bob"
	_, _ = Call[map[string]int](context.Background(), c, ep, Request{})
	if hits.Load() != 4 {
		t.Fatalf("POSTs are never cached and users never share entries: hits %d", hits.Load())
	}
}

func TestCachePersistsAcrossRestarts(t *testing.T) {
	srv, hits, down := server(t)
	path := filepath.Join(t.TempDir(), "cache.json")
	c, _ := New(Options{BaseURL: srv.URL, Cache: NewCache(CacheOptions{Path: path}), OfflineCooldown: -1})
	_, _ = Call[map[string]int](context.Background(), c, Endpoint{"GET", "/items"}, Request{})

	down.Store(true)
	restarted, _ := New(Options{BaseURL: srv.URL, Cache: NewCache(CacheOptions{Path: path, Fresh: time.Nanosecond}), OfflineCooldown: -1})
	v, err := Call[map[string]int](context.Background(), restarted, Endpoint{"GET", "/items"}, Request{})
	if err != nil || v["n"] != 1 || hits.Load() != 2 {
		t.Fatalf("after restart: %v, %v, hits %d", v, err, hits.Load())
	}
}

func TestBreakerFailsFastWhileOffline(t *testing.T) {
	srv, hits, down := server(t)
	c, _ := New(Options{BaseURL: srv.URL, OfflineCooldown: time.Hour})
	down.Store(true)
	if err := c.Health(context.Background()); err == nil {
		t.Fatal("want the 503")
	}
	start := time.Now()
	err := c.Health(context.Background())
	if !errors.Is(err, ErrOffline) || hits.Load() != 1 || time.Since(start) > 100*time.Millisecond {
		t.Fatalf("second call should fail fast without a request: %v, hits %d", err, hits.Load())
	}
	st := c.CheckStatus(context.Background())
	if !st.Offline || st.Reachable || st.Mode != ModeLive {
		t.Fatalf("status: %+v", st)
	}

	c.breaker.now = func() time.Time { return time.Now().Add(2 * time.Hour) } // cool-down over
	down.Store(false)
	if st := c.CheckStatus(context.Background()); st.Offline || !st.Reachable {
		t.Fatalf("should be back online: %+v", st)
	}
}

func TestOfflineServesCacheInstantly(t *testing.T) {
	srv, hits, _ := server(t)
	c, _ := New(Options{BaseURL: srv.URL, Cache: NewCache(CacheOptions{Fresh: time.Nanosecond}), OfflineCooldown: time.Hour})
	ep := Endpoint{"GET", "/items"}
	_, _ = Call[map[string]int](context.Background(), c, ep, Request{})
	srv.Close() // the server disappears
	_ = c.Health(context.Background())

	start := time.Now()
	v, err := Call[map[string]int](context.Background(), c, ep, Request{})
	if err != nil || v["n"] != 1 || time.Since(start) > 100*time.Millisecond || hits.Load() != 1 {
		t.Fatalf("offline read should come from the cache at once: %v, %v in %s", v, err, time.Since(start))
	}
}

func TestRetriesFinishBeforeTheBreakerTrips(t *testing.T) {
	srv, hits, down := server(t)
	down.Store(true)
	c, _ := New(Options{BaseURL: srv.URL, Retries: 3, RetryBackoff: time.Millisecond, OfflineCooldown: time.Hour})
	if err := c.Health(context.Background()); err == nil || hits.Load() != 3 {
		t.Fatalf("all 3 tries should reach the server before going offline: %v, hits %d", err, hits.Load())
	}
	if err := c.Health(context.Background()); !errors.Is(err, ErrOffline) || hits.Load() != 3 {
		t.Fatalf("then fail fast: %v, hits %d", err, hits.Load())
	}
}
