// Package backend is the app's client for the remote backend server. The Vue
// frontend never calls that server itself: it calls Go through Wails, and Go
// calls the server through this package.
//
//	endpoints.go    Endpoint "enum": every route as {Method, Path}
//	client.go       Client, the Call helper, request options and errors
//	interceptors.go UserAgent, BearerAuth, and the Interceptor type (retries are an Option)
//	mock.go         Mock and DefaultMock: in-process answers while there is no server
//	cache.go        Cache: GET answers kept on disk, served first and while offline
//	breaker.go      fails fast with ErrOffline for a while after the server fails
//	services.go     typed functions for each endpoint
//
// The server address comes from BaseURL (set at build time) or the
// API_TEST_KITS_BACKEND_URL environment variable, which wins when set. The
// client then works in one of three modes:
//
//	live  a URL is set: requests go to the server (through the cache and breaker)
//	mock  no URL but a Mock: requests are answered in-process, never over the network
//	off   neither: every call returns ErrNotConfigured
package backend

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// BaseURL is the backend address baked in at build time with
// -ldflags "-X api-test-kits/internal/backend.BaseURL=https://…".
var BaseURL = ""

// EnvBaseURL overrides BaseURL at run time, e.g. for local development.
const EnvBaseURL = "API_TEST_KITS_BACKEND_URL"

// ErrNotConfigured is returned by every call when no backend URL is set.
var ErrNotConfigured = errors.New("the backend is not configured")

// maxResponseBytes caps how much of a response is read, so a broken server cannot exhaust memory.
const maxResponseBytes = 32 << 20

// ConfiguredURL returns the backend URL from the environment or the build, without a trailing slash.
func ConfiguredURL() string {
	u := strings.TrimSpace(os.Getenv(EnvBaseURL))
	if u == "" {
		u = strings.TrimSpace(BaseURL)
	}
	return strings.TrimRight(u, "/")
}

// Client sends requests to the backend through its interceptors.
type Client struct {
	baseURL string
	mode    Mode
	send    Handler
	breaker *breaker
	// disabled explains why a client from NewOrDisabled has no backend (a bad URL).
	disabled error
}

// Mode says where a client's requests go.
type Mode string

const (
	ModeLive Mode = "live"
	ModeMock Mode = "mock"
	ModeOff  Mode = "off"
)

// mockBaseURL is the host mocked requests are addressed to; nothing is ever sent there.
const mockBaseURL = "http://backend.mock"

// Options configure a Client. Zero values are fine.
type Options struct {
	// BaseURL is the server root; empty means not configured.
	BaseURL string
	// Timeout per request, including retries. Default 30s.
	Timeout time.Duration
	// HTTPClient replaces the default http.Client (its Timeout is ignored in favor of Timeout).
	HTTPClient *http.Client
	// Interceptors run around every request, first one outermost, before the
	// cache, so headers they set (like Authorization) are part of the cache key.
	Interceptors []Interceptor
	// Retries is how many times in total an idempotent request is tried after
	// network errors and 502/503/504 in live mode. 0 or 1 means no retry.
	Retries int
	// RetryBackoff is the wait before the first retry, doubled each time. Default 500ms.
	RetryBackoff time.Duration
	// Mock answers requests when BaseURL is empty. Nil leaves the client off.
	Mock *Mock
	// Cache keeps GET answers in live mode. Nil disables caching.
	Cache *Cache
	// OfflineCooldown is how long requests fail at once with ErrOffline after
	// the server failed, in live mode. Default 30s; negative disables it.
	OfflineCooldown time.Duration
}

// New creates a client. It returns an error only for a malformed BaseURL.
func New(opts Options) (*Client, error) {
	base := strings.TrimRight(strings.TrimSpace(opts.BaseURL), "/")
	if base != "" {
		u, err := url.Parse(base)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return nil, fmt.Errorf("backend URL %q must be an http(s) URL with a host", base)
		}
	}
	hc := opts.HTTPClient
	if hc == nil {
		hc = &http.Client{}
	}
	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	c := &Client{baseURL: base, mode: ModeOff}
	final := func(r *http.Request) (*http.Response, error) { return hc.Do(r) }
	// Order, outermost first: your interceptors → cache → breaker → retry → network.
	// The cache can then stand in when the breaker or the retries give up.
	interceptors := append([]Interceptor(nil), opts.Interceptors...)
	switch {
	case base != "":
		c.mode = ModeLive
		if opts.Cache != nil {
			interceptors = append(interceptors, opts.Cache.interceptor())
		}
		cooldown := opts.OfflineCooldown
		if cooldown == 0 {
			cooldown = 30 * time.Second
		}
		if cooldown > 0 {
			c.breaker = newBreaker(cooldown)
			interceptors = append(interceptors, c.breaker.interceptor())
		}
		if opts.Retries > 1 {
			backoff := opts.RetryBackoff
			if backoff <= 0 {
				backoff = 500 * time.Millisecond
			}
			interceptors = append(interceptors, retry(opts.Retries, backoff))
		}
	case opts.Mock != nil:
		c.mode = ModeMock
		final = opts.Mock.handler()
	}
	inner := chain(final, interceptors)
	send := func(r *http.Request) (*http.Response, error) {
		ctx, cancel := context.WithTimeout(r.Context(), timeout)
		resp, err := inner(r.WithContext(ctx))
		if err != nil {
			cancel()
			return nil, err
		}
		resp.Body = cancelOnClose{resp.Body, cancel}
		return resp, nil
	}
	c.send = send
	return c, nil
}

// NewOrDisabled is New for app startup: a malformed BaseURL never stops the
// app. It returns a client without the URL (mock mode when opts.Mock is set,
// otherwise off) whose status reports the URL problem, together with that
// error so the caller can log it.
func NewOrDisabled(opts Options) (*Client, error) {
	c, err := New(opts)
	if err == nil {
		return c, nil
	}
	opts.BaseURL = ""
	c, _ = New(opts)
	c.disabled = err
	return c, err
}

type cancelOnClose struct {
	io.ReadCloser
	cancel context.CancelFunc
}

func (c cancelOnClose) Close() error {
	err := c.ReadCloser.Close()
	c.cancel()
	return err
}

// Configured reports whether the client has a backend URL.
func (c *Client) Configured() bool { return c != nil && c.baseURL != "" }

// Mode says where requests go: live, mock or off.
func (c *Client) Mode() Mode {
	if c == nil {
		return ModeOff
	}
	return c.mode
}

// BaseURL returns the backend address, or "" when not configured.
func (c *Client) BaseURL() string {
	if c == nil {
		return ""
	}
	return c.baseURL
}

// Request is everything about one call besides the endpoint.
type Request struct {
	// PathParams fill {name} placeholders in the endpoint path (escaped).
	PathParams map[string]string
	// Query is added to the URL.
	Query url.Values
	// Body is sent as JSON when not nil.
	Body any
	// Header adds request headers.
	Header http.Header
	// NoCache skips the cache: the answer comes from the server or the call fails.
	NoCache bool
}

// APIError is a non-2xx answer from the backend.
type APIError struct {
	Endpoint Endpoint
	Status   int
	// Code and Message come from a JSON body like {"code": "...", "message": "..."}
	// or {"error": "..."}; Message falls back to the status text.
	Code    string
	Message string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("backend %s: %d %s", e.Endpoint, e.Status, e.Message)
}

// Do sends a request and returns the raw response for a 2xx status; the
// caller closes its body. Any other status is returned as *APIError.
func (c *Client) Do(ctx context.Context, ep Endpoint, r Request) (*http.Response, error) {
	if c.Mode() == ModeOff {
		if c != nil && c.disabled != nil {
			return nil, fmt.Errorf("%w: %v", ErrNotConfigured, c.disabled)
		}
		return nil, ErrNotConfigured
	}
	if r.NoCache {
		ctx = withNoCache(ctx)
	}
	path, err := fillPath(ep.Path, r.PathParams)
	if err != nil {
		return nil, fmt.Errorf("backend %s: %w", ep, err)
	}
	base := c.baseURL
	if c.mode == ModeMock {
		base = mockBaseURL
	}
	target := base + path
	if len(r.Query) > 0 {
		target += "?" + r.Query.Encode()
	}

	var body io.Reader
	if r.Body != nil {
		data, err := json.Marshal(r.Body)
		if err != nil {
			return nil, fmt.Errorf("backend %s: encode body: %w", ep, err)
		}
		body = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, ep.Method, target, body)
	if err != nil {
		return nil, fmt.Errorf("backend %s: %w", ep, err)
	}
	for k, vs := range r.Header {
		for _, v := range vs {
			req.Header.Add(k, v)
		}
	}
	req.Header.Set("Accept", "application/json")
	if r.Body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.send(req)
	if err != nil {
		// Drop the URL from *url.Error: it is already in the endpoint and the query may hold user data.
		if ue, ok := errors.AsType[*url.Error](err); ok {
			err = ue.Err
		}
		return nil, fmt.Errorf("backend %s: %w", ep, err)
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		defer resp.Body.Close()
		return nil, readAPIError(ep, resp)
	}
	return resp, nil
}

// Call sends a request and decodes a JSON answer into T. An empty body (e.g.
// 204) leaves T at its zero value. Use struct{} for T when there is no answer.
func Call[T any](ctx context.Context, c *Client, ep Endpoint, r Request) (T, error) {
	var out T
	resp, err := c.Do(ctx, ep, r)
	if err != nil {
		return out, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
	if err != nil {
		return out, fmt.Errorf("backend %s: read response: %w", ep, err)
	}
	if len(data) > maxResponseBytes {
		return out, fmt.Errorf("backend %s: response is larger than %d MB", ep, maxResponseBytes>>20)
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return out, nil
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return out, fmt.Errorf("backend %s: decode response: %w", ep, err)
	}
	return out, nil
}

func fillPath(path string, params map[string]string) (string, error) {
	var b strings.Builder
	for {
		start := strings.IndexByte(path, '{')
		if start < 0 {
			b.WriteString(path)
			return b.String(), nil
		}
		end := strings.IndexByte(path[start:], '}')
		if end < 0 {
			return "", fmt.Errorf("unclosed { in path")
		}
		name := path[start+1 : start+end]
		v, ok := params[name]
		if !ok || v == "" {
			return "", fmt.Errorf("missing path parameter %q", name)
		}
		b.WriteString(path[:start])
		b.WriteString(url.PathEscape(v))
		path = path[start+end+1:]
	}
}

func readAPIError(ep Endpoint, resp *http.Response) *APIError {
	e := &APIError{Endpoint: ep, Status: resp.StatusCode, Message: http.StatusText(resp.StatusCode)}
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	var body struct {
		Code    string `json:"code"`
		Message string `json:"message"`
		Error   string `json:"error"`
	}
	if json.Unmarshal(data, &body) == nil {
		e.Code = body.Code
		if body.Message != "" {
			e.Message = body.Message
		} else if body.Error != "" {
			e.Message = body.Error
		}
	}
	return e
}
