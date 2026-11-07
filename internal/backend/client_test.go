package backend

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func newClient(t *testing.T, srv *httptest.Server, ics ...Interceptor) *Client {
	t.Helper()
	c, err := New(Options{BaseURL: srv.URL + "/", Interceptors: ics})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestConfiguredURLPrefersEnvironment(t *testing.T) {
	old := BaseURL
	t.Cleanup(func() { BaseURL = old })
	BaseURL = "https://build.example/"
	t.Setenv(EnvBaseURL, "")
	if got := ConfiguredURL(); got != "https://build.example" {
		t.Fatalf("build URL: got %q", got)
	}
	t.Setenv(EnvBaseURL, " http://localhost:8080/ ")
	if got := ConfiguredURL(); got != "http://localhost:8080" {
		t.Fatalf("env URL: got %q", got)
	}
}

func TestNewRejectsBadURLs(t *testing.T) {
	for _, u := range []string{"localhost:8080", "ftp://x.example", "http://"} {
		if _, err := New(Options{BaseURL: u}); err == nil {
			t.Errorf("%q: expected an error", u)
		}
	}
}

func TestNotConfiguredNeverSends(t *testing.T) {
	c, _ := New(Options{})
	if c.Configured() {
		t.Fatal("empty URL should not be configured")
	}
	if err := c.Health(context.Background()); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("got %v", err)
	}
	st := c.CheckStatus(context.Background())
	if st.Configured || st.Reachable || st.Error != "" {
		t.Fatalf("status: %+v", st)
	}
}

func TestCallSendsJSONAndDecodes(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.URL.EscapedPath() != "/teams/a%2Fb/items" || r.URL.Query().Get("limit") != "5" {
			t.Errorf("request: %s %s", r.Method, r.URL)
		}
		if r.Header.Get("Content-Type") != "application/json" || r.Header.Get("X-Extra") != "1" {
			t.Errorf("headers: %v", r.Header)
		}
		var in map[string]string
		_ = json.NewDecoder(r.Body).Decode(&in)
		_ = json.NewEncoder(w).Encode(map[string]string{"echo": in["name"]})
	}))
	defer srv.Close()

	ep := Endpoint{Method: "POST", Path: "/teams/{team}/items"}
	out, err := Call[map[string]string](context.Background(), newClient(t, srv), ep, Request{
		PathParams: map[string]string{"team": "a/b"},
		Query:      url.Values{"limit": {"5"}},
		Body:       map[string]string{"name": "x"},
		Header:     http.Header{"X-Extra": {"1"}},
	})
	if err != nil || out["echo"] != "x" {
		t.Fatalf("got %v, %v", out, err)
	}
}

func TestCallEmptyBodyAndMissingParam(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()
	c := newClient(t, srv)
	if _, err := Call[struct{}](context.Background(), c, Endpoint{"DELETE", "/x"}, Request{}); err != nil {
		t.Fatal(err)
	}
	_, err := Call[struct{}](context.Background(), c, Endpoint{"GET", "/x/{id}"}, Request{})
	if err == nil || !strings.Contains(err.Error(), `missing path parameter "id"`) {
		t.Fatalf("got %v", err)
	}
}

func TestAPIErrorReadsMessage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/a":
			w.WriteHeader(http.StatusConflict)
			_, _ = io.WriteString(w, `{"code":"taken","message":"name is taken"}`)
		case "/b":
			w.WriteHeader(http.StatusBadRequest)
			_, _ = io.WriteString(w, `{"error":"bad input"}`)
		default:
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = io.WriteString(w, "boom")
		}
	}))
	defer srv.Close()
	c := newClient(t, srv)
	cases := []struct{ path, code, msg string }{
		{"/a", "taken", "name is taken"},
		{"/b", "", "bad input"},
		{"/c", "", "Internal Server Error"},
	}
	for _, tc := range cases {
		_, err := Call[struct{}](context.Background(), c, Endpoint{"GET", tc.path}, Request{})
		var apiErr *APIError
		if !errors.As(err, &apiErr) || apiErr.Code != tc.code || apiErr.Message != tc.msg {
			t.Errorf("%s: got %#v", tc.path, err)
		}
	}
}

func TestCheckStatus(t *testing.T) {
	healthy := true
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if !healthy {
			w.WriteHeader(http.StatusServiceUnavailable)
		}
	}))
	c := newClient(t, srv)
	if st := c.CheckStatus(context.Background()); !st.Reachable || st.Error != "" || st.LatencyMs < 1 {
		t.Fatalf("healthy: %+v", st)
	}
	healthy = false
	if st := c.CheckStatus(context.Background()); !st.Reachable || st.Error == "" {
		t.Fatalf("unhealthy: %+v", st)
	}
	srv.Close()
	st := c.CheckStatus(context.Background())
	if st.Reachable || st.Error == "" || strings.Contains(st.Error, srv.URL+"/health?") {
		t.Fatalf("down: %+v", st)
	}
}

func TestInterceptorsRunInOrder(t *testing.T) {
	var seen []string
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		seen = append(seen, r.Header.Get("User-Agent"), r.Header.Get("Authorization"))
	}))
	defer srv.Close()
	token := ""
	var order []string
	trace := func(name string) Interceptor {
		return func(req *http.Request, next Handler) (*http.Response, error) {
			order = append(order, name+">")
			resp, err := next(req)
			order = append(order, "<"+name)
			return resp, err
		}
	}
	c := newClient(t, srv, trace("a"), UserAgent("api-test-kits/1.0"), BearerAuth(func() string { return token }), trace("b"))
	_ = c.Health(context.Background())
	token = "t1"
	_ = c.Health(context.Background())

	want := []string{"api-test-kits/1.0", "", "api-test-kits/1.0", "Bearer t1"}
	if strings.Join(seen, ",") != strings.Join(want, ",") {
		t.Fatalf("headers: %v", seen)
	}
	if got := strings.Join(order[:4], " "); got != "a> b> <b <a" {
		t.Fatalf("order: %s", got)
	}
}

func TestInterceptorCanShortCircuit(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Error("server should not be called")
	}))
	defer srv.Close()
	blocked := errors.New("offline mode")
	c := newClient(t, srv, func(*http.Request, Handler) (*http.Response, error) { return nil, blocked })
	if err := c.Health(context.Background()); !errors.Is(err, blocked) {
		t.Fatalf("got %v", err)
	}
}

func TestRetry(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := hits.Add(1)
		body, _ := io.ReadAll(r.Body)
		if r.Method == "PUT" && string(body) != `{"v":1}` {
			t.Errorf("try %d lost the body: %q", n, body)
		}
		if n < 3 {
			w.WriteHeader(http.StatusBadGateway)
		}
	}))
	defer srv.Close()
	// The breaker would stop the later calls after the 502s; this test is about retries alone.
	c, _ := New(Options{BaseURL: srv.URL, OfflineCooldown: -1, Retries: 3, RetryBackoff: time.Millisecond})

	if _, err := Call[struct{}](context.Background(), c, Endpoint{"PUT", "/x"}, Request{Body: map[string]int{"v": 1}}); err != nil {
		t.Fatalf("PUT should succeed on the third try: %v", err)
	}
	if hits.Load() != 3 {
		t.Fatalf("PUT tries: %d", hits.Load())
	}

	hits.Store(0)
	_, err := Call[struct{}](context.Background(), c, Endpoint{"POST", "/x"}, Request{})
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.Status != http.StatusBadGateway || hits.Load() != 1 {
		t.Fatalf("POST must not be retried: %v after %d tries", err, hits.Load())
	}

	hits.Store(-10)
	_, err = Call[struct{}](context.Background(), c, Endpoint{"GET", "/x"}, Request{})
	if !errors.As(err, &apiErr) || hits.Load() != -7 {
		t.Fatalf("GET should stop after 3 tries: %v, hits %d", err, hits.Load())
	}
}

func TestTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer srv.Close()
	c, _ := New(Options{BaseURL: srv.URL, Timeout: 50 * time.Millisecond})
	if err := c.Health(context.Background()); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("got %v", err)
	}
}

func TestNewOrDisabledKeepsTheAppRunning(t *testing.T) {
	c, err := NewOrDisabled(Options{BaseURL: "localhost:8080"})
	if err == nil || c == nil {
		t.Fatalf("want a disabled client and an error, got %v, %v", c, err)
	}
	if c.Configured() {
		t.Fatal("a bad URL must not count as configured")
	}
	callErr := c.Health(context.Background())
	if !errors.Is(callErr, ErrNotConfigured) || !strings.Contains(callErr.Error(), "localhost:8080") {
		t.Fatalf("calls should explain the bad URL: %v", callErr)
	}
	st := c.CheckStatus(context.Background())
	if st.Configured || st.Reachable || !strings.Contains(st.Error, "must be an http(s) URL") {
		t.Fatalf("status should show the URL problem: %+v", st)
	}

	ok, err := NewOrDisabled(Options{BaseURL: "https://api.example"})
	if err != nil || !ok.Configured() {
		t.Fatalf("a good URL: %v, configured=%v", err, ok.Configured())
	}
}
