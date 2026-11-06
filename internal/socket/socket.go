// Package socket opens WebSocket and Socket.IO connections for socket
// requests. It hides the protocol differences behind Conn: both deliver
// received messages on a channel and send messages with an optional Socket.IO
// event name. Network rules (TLS, proxy, blocked destinations) come from
// httpclient.Client.DialWebSocket.
package socket

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"api-test-kits/internal/domain"
	"api-test-kits/internal/httpclient"
)

// Dialer is the subset of httpclient.Client used to open connections.
type Dialer interface {
	DialWebSocket(ctx context.Context, req httpclient.SocketRequest) (*httpclient.SocketConn, error)
}

// Message is a received message.
type Message struct {
	// Event is the Socket.IO event name (for an acknowledgement, the event
	// that was acknowledged).
	Event string
	// Data is the message text, or for Socket.IO the event's argument as
	// JSON (a JSON array when there are several, empty when there are none).
	Data   []byte
	Binary bool
	Ack    bool
	At     time.Time
}

// Outgoing is a message to send.
type Outgoing struct {
	// Event is required for Socket.IO and ignored for WebSocket.
	Event string
	Body  string
	// JSON sends Body as a JSON value (Socket.IO); for WebSocket it is only
	// checked to be valid JSON.
	JSON bool
	// Ack asks the Socket.IO server to acknowledge the event.
	Ack bool
}

// Info describes an open connection.
type Info struct {
	Subprotocol string
	SessionID   string
	Handshake   *httpclient.Response
}

// Conn is an open WebSocket or Socket.IO connection. It is safe to call Send
// and Close from several goroutines.
type Conn interface {
	Info() Info
	// Messages delivers received messages and is closed when the connection ends.
	Messages() <-chan Message
	Send(Outgoing) error
	// Close ends the connection politely and waits briefly for the server.
	Close()
	// Done is closed when the connection has ended.
	Done() <-chan struct{}
	// Result is how the connection ended; err is nil for a normal close.
	Result() (code int, reason string, err error)
}

// Options configure Open.
type Options struct {
	Kind domain.RequestKind
	// URL is the resolved request URL (with query and auth parameters).
	URL          string
	Header       map[string][]string
	Subprotocols []string
	// Path is the Socket.IO server path.
	Path string
	// AuthPayload is the resolved Socket.IO CONNECT payload (a JSON object), or "".
	AuthPayload string
	// Timeout bounds the handshake, including the Socket.IO CONNECT.
	Timeout time.Duration
}

// ErrClosed is returned by Send after the connection has ended.
var ErrClosed = errors.New("the connection is closed")

// Open connects and, for Socket.IO, completes the namespace CONNECT.
func Open(ctx context.Context, d Dialer, o Options) (Conn, error) {
	switch o.Kind {
	case domain.KindWebSocket:
		sc, err := d.DialWebSocket(ctx, httpclient.SocketRequest{URL: o.URL, Header: o.Header,
			Subprotocols: o.Subprotocols, Timeout: o.Timeout})
		if err != nil {
			return nil, err
		}
		return newWSConn(sc), nil
	case domain.KindSocketIO:
		dialURL, ns, err := EngineURL(o.URL, o.Path)
		if err != nil {
			return nil, err
		}
		if strings.TrimSpace(o.AuthPayload) != "" {
			var obj map[string]any
			if err := json.Unmarshal([]byte(o.AuthPayload), &obj); err != nil {
				return nil, fmt.Errorf("the Socket.IO auth payload must be a JSON object: %w", err)
			}
		}
		start := time.Now()
		sc, err := d.DialWebSocket(ctx, httpclient.SocketRequest{URL: dialURL, Header: o.Header, Timeout: o.Timeout})
		if err != nil {
			return nil, err
		}
		remaining := o.Timeout - time.Since(start)
		if o.Timeout <= 0 || remaining < time.Second {
			remaining = time.Second
		}
		return openSocketIO(ctx, sc, ns, strings.TrimSpace(o.AuthPayload), remaining)
	}
	return nil, fmt.Errorf("%q is not a socket request kind", o.Kind)
}

// EngineURL turns a Socket.IO URL like https://host/admin?x=1 into the
// Engine.IO WebSocket URL wss://host/socket.io/?x=1&EIO=4&transport=websocket
// and the namespace "/admin", the way the official client does.
func EngineURL(raw, path string) (dialURL, namespace string, err error) {
	u, err := url.Parse(raw)
	if err != nil {
		return "", "", fmt.Errorf("invalid URL: %w", err)
	}
	switch u.Scheme {
	case "http", "ws":
		u.Scheme = "ws"
	case "https", "wss":
		u.Scheme = "wss"
	default:
		return "", "", fmt.Errorf("URL scheme must be http, https, ws or wss, got %q", u.Scheme)
	}
	namespace = u.Path
	if namespace == "" {
		namespace = "/"
	}
	if path == "" {
		path = domain.DefaultSocketIOPath
	}
	if !strings.HasSuffix(path, "/") {
		path += "/"
	}
	u.Path, u.RawPath = path, ""
	q := u.Query()
	q.Set("EIO", "4")
	q.Set("transport", "websocket")
	u.RawQuery = q.Encode()
	return u.String(), namespace, nil
}

// encodeBody returns a JSON body's value for Socket.IO, or checks it for WebSocket.
func checkJSON(body string) (json.RawMessage, error) {
	if !json.Valid([]byte(body)) {
		return nil, errors.New("the message body is not valid JSON")
	}
	return json.RawMessage(body), nil
}
