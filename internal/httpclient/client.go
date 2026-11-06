// Package httpclient sends prepared HTTP requests with explicit limits for
// time, response size, redirects, TLS verification and destinations.
package httpclient

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

const (
	DefaultMaxResponseBytes = 10 << 20
	DefaultMaxRedirects     = 10
)

type ProxyMode string

const (
	// ProxyNone connects directly and ignores proxy environment variables.
	ProxyNone ProxyMode = "none"
	// ProxySystem uses HTTP_PROXY, HTTPS_PROXY and NO_PROXY.
	ProxySystem ProxyMode = "system"
	// ProxyCustom uses Options.ProxyURL for every request.
	ProxyCustom ProxyMode = "custom"
)

// Options configure network behavior. The zero value is safe: TLS is
// verified, redirects are capped and sensitive destinations are blocked.
type Options struct {
	MaxResponseBytes int64
	MaxRedirects     int
	// InsecureSkipVerify disables TLS certificate verification. Never default.
	InsecureSkipVerify bool
	// CACertFile adds a PEM bundle of extra trusted roots.
	CACertFile string
	ProxyMode  ProxyMode
	ProxyURL   string
	// AllowSensitiveDestinations permits link-local and metadata addresses.
	AllowSensitiveDestinations bool
}

// Request is a fully resolved request ready to send.
type Request struct {
	Method          string
	URL             string
	Header          http.Header
	Body            []byte
	Timeout         time.Duration
	FollowRedirects bool
	// CredentialHeaders are removed when a redirect leaves the original
	// origin, in addition to Authorization, Proxy-Authorization and Cookie.
	CredentialHeaders []string
}

// Response is what came back.
type Response struct {
	StatusCode int
	Status     string
	Header     http.Header
	Body       []byte
	// BodyBytes is the number of bytes read; when Truncated it is the limit.
	BodyBytes int64
	Truncated bool
	Duration  time.Duration
	Redirects []string
}

// ErrorKind classifies transport failures for clear messages.
type ErrorKind string

const (
	ErrTimeout   ErrorKind = "timeout"
	ErrCancelled ErrorKind = "cancelled"
	ErrTLS       ErrorKind = "tls"
	ErrBlocked   ErrorKind = "blocked"
	ErrNetwork   ErrorKind = "network"
	ErrInvalid   ErrorKind = "invalid_request"
)

// Error wraps a transport failure with its kind.
type Error struct {
	Kind ErrorKind
	Err  error
}

func (e *Error) Error() string { return string(e.Kind) + ": " + e.Err.Error() }
func (e *Error) Unwrap() error { return e.Err }

// Client sends requests. It is safe for concurrent use.
type Client struct {
	opts      Options
	transport *http.Transport
	// dial and tlsConfig are shared with WebSocket connections (see websocket.go).
	dial      func(ctx context.Context, network, addr string) (net.Conn, error)
	tlsConfig *tls.Config
}

// New builds a client from opts.
func New(opts Options) (*Client, error) {
	if opts.MaxResponseBytes <= 0 {
		opts.MaxResponseBytes = DefaultMaxResponseBytes
	}
	if opts.MaxRedirects <= 0 {
		opts.MaxRedirects = DefaultMaxRedirects
	}
	if opts.ProxyMode == "" {
		opts.ProxyMode = ProxySystem
	}

	tlsCfg := &tls.Config{MinVersion: tls.VersionTLS12, InsecureSkipVerify: opts.InsecureSkipVerify} //nolint:gosec // opt-in only, shown in UI and reports
	if opts.CACertFile != "" {
		pem, err := os.ReadFile(opts.CACertFile)
		if err != nil {
			return nil, fmt.Errorf("read CA certificate file: %w", err)
		}
		pool, err := x509.SystemCertPool()
		if err != nil || pool == nil {
			pool = x509.NewCertPool()
		}
		if !pool.AppendCertsFromPEM(pem) {
			return nil, fmt.Errorf("CA certificate file %s contains no PEM certificates", opts.CACertFile)
		}
		tlsCfg.RootCAs = pool
	}

	dialer := &net.Dialer{Timeout: 15 * time.Second, KeepAlive: 30 * time.Second}
	if !opts.AllowSensitiveDestinations {
		dialer.Control = dialControl
	}

	t := &http.Transport{
		DialContext:           dialer.DialContext,
		TLSClientConfig:       tlsCfg,
		TLSHandshakeTimeout:   15 * time.Second,
		MaxIdleConns:          32,
		MaxIdleConnsPerHost:   8,
		IdleConnTimeout:       60 * time.Second,
		ExpectContinueTimeout: time.Second,
		ForceAttemptHTTP2:     true,
		// Keep the body exactly as sent by the server so sizes are honest.
		DisableCompression: false,
	}
	switch opts.ProxyMode {
	case ProxyNone:
		t.Proxy = nil
	case ProxySystem:
		t.Proxy = http.ProxyFromEnvironment
	case ProxyCustom:
		u, err := url.Parse(opts.ProxyURL)
		if err != nil || u.Scheme == "" || u.Host == "" {
			return nil, fmt.Errorf("invalid proxy URL %q", opts.ProxyURL)
		}
		t.Proxy = http.ProxyURL(u)
	default:
		return nil, fmt.Errorf("unknown proxy mode %q", opts.ProxyMode)
	}
	return &Client{opts: opts, transport: t, dial: dialer.DialContext, tlsConfig: tlsCfg}, nil
}

// Options returns the effective options.
func (c *Client) Options() Options { return c.opts }

// Close releases idle connections.
func (c *Client) Close() { c.transport.CloseIdleConnections() }

// Do sends req. It never retries. The context cancels the request at any point,
// including while the body is being read.
func (c *Client) Do(ctx context.Context, req Request) (*Response, error) {
	u, err := url.Parse(req.URL)
	if err != nil {
		return nil, &Error{Kind: ErrInvalid, Err: fmt.Errorf("invalid URL: %w", err)}
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, &Error{Kind: ErrInvalid, Err: fmt.Errorf("URL scheme must be http or https, got %q", u.Scheme)}
	}
	if u.Host == "" {
		return nil, &Error{Kind: ErrInvalid, Err: errors.New("URL must include a host")}
	}
	if !c.opts.AllowSensitiveDestinations {
		if err := CheckHost(u.Hostname()); err != nil {
			return nil, &Error{Kind: ErrBlocked, Err: err}
		}
	}

	if req.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, req.Timeout)
		defer cancel()
	}

	var body io.Reader
	if len(req.Body) > 0 {
		body = bytes.NewReader(req.Body)
	}
	httpReq, err := http.NewRequestWithContext(ctx, req.Method, u.String(), body)
	if err != nil {
		return nil, &Error{Kind: ErrInvalid, Err: err}
	}
	for k, vals := range req.Header {
		for _, v := range vals {
			httpReq.Header.Add(k, v)
		}
	}
	if httpReq.Header.Get("User-Agent") == "" {
		httpReq.Header.Set("User-Agent", "api-regression-guard")
	}

	var redirects []string
	origin := originOf(u)
	client := &http.Client{
		Transport: c.transport,
		CheckRedirect: func(next *http.Request, via []*http.Request) error {
			if !req.FollowRedirects {
				return http.ErrUseLastResponse
			}
			if len(via) > c.opts.MaxRedirects {
				return fmt.Errorf("stopped after %d redirects", c.opts.MaxRedirects)
			}
			if !c.opts.AllowSensitiveDestinations {
				if err := CheckHost(next.URL.Hostname()); err != nil {
					return err
				}
			}
			if originOf(next.URL) != origin {
				// Never forward credentials to a different origin.
				for _, h := range append([]string{"Authorization", "Proxy-Authorization", "Cookie"}, req.CredentialHeaders...) {
					next.Header.Del(h)
				}
			}
			redirects = append(redirects, next.URL.String())
			return nil
		},
	}

	start := time.Now()
	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, classify(ctx, err)
	}
	defer resp.Body.Close()

	limited := io.LimitReader(resp.Body, c.opts.MaxResponseBytes+1)
	data, err := io.ReadAll(limited)
	duration := time.Since(start)
	if err != nil {
		return nil, classify(ctx, err)
	}
	truncated := int64(len(data)) > c.opts.MaxResponseBytes
	if truncated {
		data = data[:c.opts.MaxResponseBytes]
	}
	return &Response{
		StatusCode: resp.StatusCode,
		Status:     resp.Status,
		Header:     resp.Header,
		Body:       data,
		BodyBytes:  int64(len(data)),
		Truncated:  truncated,
		Duration:   duration,
		Redirects:  redirects,
	}, nil
}

func originOf(u *url.URL) string {
	host := strings.ToLower(u.Hostname())
	port := u.Port()
	if port == "" {
		if u.Scheme == "https" {
			port = "443"
		} else {
			port = "80"
		}
	}
	return u.Scheme + "://" + host + ":" + port
}

func classify(ctx context.Context, err error) error {
	var blocked *BlockedError
	if errors.As(err, &blocked) {
		return &Error{Kind: ErrBlocked, Err: blocked}
	}
	if errors.Is(ctx.Err(), context.Canceled) {
		return &Error{Kind: ErrCancelled, Err: errors.New("request was cancelled")}
	}
	if errors.Is(ctx.Err(), context.DeadlineExceeded) || errors.Is(err, context.DeadlineExceeded) {
		return &Error{Kind: ErrTimeout, Err: errors.New("request timed out")}
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return &Error{Kind: ErrTimeout, Err: err}
	}
	var certErr *tls.CertificateVerificationError
	var unknownAuth x509.UnknownAuthorityError
	var hostErr x509.HostnameError
	var invalidErr x509.CertificateInvalidError
	var recordErr tls.RecordHeaderError
	if errors.As(err, &certErr) || errors.As(err, &unknownAuth) || errors.As(err, &hostErr) ||
		errors.As(err, &invalidErr) || errors.As(err, &recordErr) || strings.Contains(err.Error(), "tls:") {
		return &Error{Kind: ErrTLS, Err: err}
	}
	return &Error{Kind: ErrNetwork, Err: err}
}
