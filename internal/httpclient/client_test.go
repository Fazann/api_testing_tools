package httpclient

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func newClient(t *testing.T, opts Options) *Client {
	t.Helper()
	c, err := New(opts)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(c.Close)
	return c
}

func kindOf(err error) ErrorKind {
	var e *Error
	if errors.As(err, &e) {
		return e.Kind
	}
	return ""
}

func TestBasicRequest(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Test") != "1" || r.Method != http.MethodPost {
			w.WriteHeader(400)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(201)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()
	c := newClient(t, Options{ProxyMode: ProxyNone})
	resp, err := c.Do(context.Background(), Request{Method: "POST", URL: srv.URL, Header: http.Header{"X-Test": {"1"}}, Body: []byte("{}")})
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 201 || string(resp.Body) != `{"ok":true}` || resp.Truncated {
		t.Fatalf("unexpected response %+v", resp)
	}
}

func TestTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-time.After(2 * time.Second):
		case <-r.Context().Done():
		}
	}))
	defer srv.Close()
	c := newClient(t, Options{ProxyMode: ProxyNone})
	start := time.Now()
	_, err := c.Do(context.Background(), Request{Method: "GET", URL: srv.URL, Timeout: 100 * time.Millisecond})
	if kindOf(err) != ErrTimeout {
		t.Fatalf("expected timeout, got %v", err)
	}
	if time.Since(start) > time.Second {
		t.Fatal("timeout was not enforced promptly")
	}
}

func TestCancel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer srv.Close()
	c := newClient(t, Options{ProxyMode: ProxyNone})
	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(50*time.Millisecond, cancel)
	_, err := c.Do(ctx, Request{Method: "GET", URL: srv.URL, Timeout: 5 * time.Second})
	if kindOf(err) != ErrCancelled {
		t.Fatalf("expected cancelled, got %v", err)
	}
}

func TestOversizedResponseIsTruncated(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(strings.Repeat("x", 5000)))
	}))
	defer srv.Close()
	c := newClient(t, Options{ProxyMode: ProxyNone, MaxResponseBytes: 1024})
	resp, err := c.Do(context.Background(), Request{Method: "GET", URL: srv.URL})
	if err != nil {
		t.Fatal(err)
	}
	if !resp.Truncated || len(resp.Body) != 1024 {
		t.Fatalf("expected truncation at 1024, got truncated=%v len=%d", resp.Truncated, len(resp.Body))
	}
}

func TestRedirectsKeepCredentialsSameOrigin(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/start" {
			http.Redirect(w, r, "/end", http.StatusFound)
			return
		}
		gotAuth = r.Header.Get("Authorization")
	}))
	defer srv.Close()
	c := newClient(t, Options{ProxyMode: ProxyNone})
	resp, err := c.Do(context.Background(), Request{Method: "GET", URL: srv.URL + "/start", FollowRedirects: true,
		Header: http.Header{"Authorization": {"Bearer t"}}})
	if err != nil {
		t.Fatal(err)
	}
	if gotAuth != "Bearer t" || len(resp.Redirects) != 1 {
		t.Fatalf("same-origin redirect should keep auth: auth=%q redirects=%v", gotAuth, resp.Redirects)
	}
}

func TestCrossOriginRedirectStripsCredentials(t *testing.T) {
	var got http.Header
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Clone()
	}))
	defer other.Close()
	// Same host, different port is a different origin.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, other.URL+"/landing", http.StatusTemporaryRedirect)
	}))
	defer srv.Close()
	c := newClient(t, Options{ProxyMode: ProxyNone})
	_, err := c.Do(context.Background(), Request{Method: "GET", URL: srv.URL, FollowRedirects: true,
		Header: http.Header{
			"Authorization": {"Bearer t"}, "Cookie": {"sid=1"}, "X-Api-Key": {"k"}, "Accept": {"application/json"},
		},
		CredentialHeaders: []string{"X-Api-Key"},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, h := range []string{"Authorization", "Cookie", "X-Api-Key"} {
		if got.Get(h) != "" {
			t.Fatalf("%s forwarded across origins", h)
		}
	}
	if got.Get("Accept") != "application/json" {
		t.Fatal("non-sensitive headers should still be forwarded")
	}
}

func TestRedirectNotFollowedWhenDisabled(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/elsewhere", http.StatusMovedPermanently)
	}))
	defer srv.Close()
	c := newClient(t, Options{ProxyMode: ProxyNone})
	resp, err := c.Do(context.Background(), Request{Method: "GET", URL: srv.URL})
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusMovedPermanently {
		t.Fatalf("expected the redirect response itself, got %d", resp.StatusCode)
	}
}

func TestRedirectLimit(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, r.URL.Path+"x", http.StatusFound)
	}))
	defer srv.Close()
	c := newClient(t, Options{ProxyMode: ProxyNone, MaxRedirects: 3})
	_, err := c.Do(context.Background(), Request{Method: "GET", URL: srv.URL + "/", FollowRedirects: true})
	if err == nil || !strings.Contains(err.Error(), "redirects") {
		t.Fatalf("expected redirect limit error, got %v", err)
	}
}

func TestTLSFailureIsReported(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer srv.Close()
	c := newClient(t, Options{ProxyMode: ProxyNone})
	_, err := c.Do(context.Background(), Request{Method: "GET", URL: srv.URL})
	if kindOf(err) != ErrTLS {
		t.Fatalf("self-signed certificate must fail verification by default, got %v", err)
	}
	insecure := newClient(t, Options{ProxyMode: ProxyNone, InsecureSkipVerify: true})
	if _, err := insecure.Do(context.Background(), Request{Method: "GET", URL: srv.URL}); err != nil {
		t.Fatalf("explicit opt-out should connect: %v", err)
	}
}

func TestBlockedDestinations(t *testing.T) {
	c := newClient(t, Options{ProxyMode: ProxyNone})
	for _, u := range []string{
		"http://169.254.169.254/latest/meta-data/",
		"http://metadata.google.internal/computeMetadata/v1/",
		"http://[fe80::1]/",
	} {
		_, err := c.Do(context.Background(), Request{Method: "GET", URL: u, Timeout: time.Second})
		if kindOf(err) != ErrBlocked {
			t.Fatalf("%s: expected blocked, got %v", u, err)
		}
	}
}

func TestRedirectToBlockedDestination(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://169.254.169.254/", http.StatusFound)
	}))
	defer srv.Close()
	c := newClient(t, Options{ProxyMode: ProxyNone})
	_, err := c.Do(context.Background(), Request{Method: "GET", URL: srv.URL, FollowRedirects: true})
	if kindOf(err) != ErrBlocked {
		t.Fatalf("redirect to metadata must be blocked, got %v", err)
	}
}

func TestInvalidURLs(t *testing.T) {
	c := newClient(t, Options{ProxyMode: ProxyNone})
	for _, u := range []string{"ftp://x/", "file:///etc/passwd", "http://", "::bad"} {
		if _, err := c.Do(context.Background(), Request{Method: "GET", URL: u}); kindOf(err) != ErrInvalid {
			t.Fatalf("%s: expected invalid_request, got %v", u, err)
		}
	}
}

func TestBadProxyAndCAConfig(t *testing.T) {
	if _, err := New(Options{ProxyMode: ProxyCustom, ProxyURL: "not a url"}); err == nil {
		t.Fatal("expected invalid proxy error")
	}
	if _, err := New(Options{CACertFile: "/does/not/exist.pem"}); err == nil {
		t.Fatal("expected CA file error")
	}
}
