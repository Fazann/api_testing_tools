package httpclient

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/gorilla/websocket"
)

// SocketRequest is a resolved WebSocket handshake.
type SocketRequest struct {
	// URL uses ws://, wss://, http:// or https:// (sent as ws:// and wss://).
	URL          string
	Header       http.Header
	Subprotocols []string
	// Timeout bounds the handshake only; the connection stays open after it.
	Timeout time.Duration
}

// SocketConn is an open WebSocket connection and its handshake response.
type SocketConn struct {
	Conn *websocket.Conn
	// Handshake is the server's 101 response (headers only).
	Handshake *Response
}

// DialWebSocket opens a WebSocket connection with the same rules as Do: TLS
// verification and extra CA certificates, the proxy setting, and the check
// against link-local and metadata destinations. Redirects are not followed.
// Messages larger than the response size limit close the connection.
func (c *Client) DialWebSocket(ctx context.Context, req SocketRequest) (*SocketConn, error) {
	u, err := url.Parse(req.URL)
	if err != nil {
		return nil, &Error{Kind: ErrInvalid, Err: fmt.Errorf("invalid URL: %w", err)}
	}
	switch u.Scheme {
	case "ws", "wss":
	case "http":
		u.Scheme = "ws"
	case "https":
		u.Scheme = "wss"
	default:
		return nil, &Error{Kind: ErrInvalid, Err: fmt.Errorf("URL scheme must be ws, wss, http or https, got %q", u.Scheme)}
	}
	if u.Host == "" {
		return nil, &Error{Kind: ErrInvalid, Err: errors.New("URL must include a host")}
	}
	if !c.opts.AllowSensitiveDestinations {
		if err := CheckHost(u.Hostname()); err != nil {
			return nil, &Error{Kind: ErrBlocked, Err: err}
		}
	}
	dctx := ctx
	if req.Timeout > 0 {
		var cancel context.CancelFunc
		dctx, cancel = context.WithTimeout(ctx, req.Timeout)
		defer cancel()
	}

	header := http.Header{}
	for k, vals := range req.Header {
		for _, v := range vals {
			header.Add(k, v)
		}
	}
	if header.Get("User-Agent") == "" {
		header.Set("User-Agent", "api-test-kits")
	}
	dialer := websocket.Dialer{
		NetDialContext:   c.dial,
		Proxy:            c.transport.Proxy,
		TLSClientConfig:  c.tlsConfig.Clone(),
		HandshakeTimeout: req.Timeout,
		Subprotocols:     req.Subprotocols,
	}
	start := time.Now()
	conn, resp, err := dialer.DialContext(dctx, u.String(), header)
	if err != nil {
		if resp != nil {
			resp.Body.Close()
		}
		if errors.Is(err, websocket.ErrBadHandshake) && resp != nil {
			return nil, &Error{Kind: ErrNetwork, Err: fmt.Errorf("the server refused the WebSocket upgrade with %s", resp.Status)}
		}
		return nil, classify(dctx, err)
	}
	conn.SetReadLimit(c.opts.MaxResponseBytes)
	return &SocketConn{Conn: conn, Handshake: &Response{
		StatusCode: resp.StatusCode,
		Status:     resp.Status,
		Header:     resp.Header,
		Duration:   time.Since(start),
	}}, nil
}
