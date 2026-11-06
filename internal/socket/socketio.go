package socket

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"api-test-kits/internal/httpclient"
)

// Engine.IO v4 packet types (the first character of every text frame).
const (
	eioOpen    = '0'
	eioClose   = '1'
	eioPing    = '2'
	eioPong    = '3'
	eioMessage = '4'
	eioNoop    = '6'
)

// Socket.IO v5 packet types (the character after eioMessage).
const (
	sioConnect      = '0'
	sioDisconnect   = '1'
	sioEvent        = '2'
	sioAck          = '3'
	sioConnectError = '4'
	sioBinaryEvent  = '5'
	sioBinaryAck    = '6'
)

// sioConn is a Socket.IO client for one namespace over the WebSocket transport.
type sioConn struct {
	*base
	ns string
	// readTimeout is the server's ping interval plus ping timeout: without a
	// ping in that time the server is gone.
	readTimeout time.Duration

	ackMu   sync.Mutex
	nextAck int
	pending map[int]string
}

type packet struct {
	typ         byte
	attachments int
	ns          string
	id          int // -1 when absent
	data        json.RawMessage
}

// parsePacket parses a Socket.IO packet (the text after the Engine.IO "4").
func parsePacket(s string) (packet, error) {
	p := packet{id: -1, ns: "/"}
	if s == "" {
		return p, errors.New("empty packet")
	}
	p.typ = s[0]
	i := 1
	if p.typ == sioBinaryEvent || p.typ == sioBinaryAck {
		j := strings.IndexByte(s[i:], '-')
		if j < 0 {
			return p, errors.New("binary packet without attachment count")
		}
		p.attachments, _ = strconv.Atoi(s[i : i+j])
		i += j + 1
	}
	if i < len(s) && s[i] == '/' {
		j := strings.IndexByte(s[i:], ',')
		if j < 0 {
			p.ns = s[i:]
			return p, nil
		}
		p.ns = s[i : i+j]
		i += j + 1
	}
	j := i
	for j < len(s) && s[j] >= '0' && s[j] <= '9' {
		j++
	}
	if j > i {
		p.id, _ = strconv.Atoi(s[i:j])
	}
	if rest := s[j:]; rest != "" {
		if !json.Valid([]byte(rest)) {
			return p, fmt.Errorf("packet payload is not valid JSON")
		}
		p.data = json.RawMessage(rest)
	}
	return p, nil
}

// nsPrefix is the namespace part of a packet: "" for the main namespace, "/admin," otherwise.
func nsPrefix(ns string) string {
	if ns == "/" {
		return ""
	}
	return ns + ","
}

func openSocketIO(ctx context.Context, sc *httpclient.SocketConn, ns, auth string, timeout time.Duration) (Conn, error) {
	conn := sc.Conn
	fail := func(err error) (Conn, error) {
		conn.Close()
		return nil, err
	}
	// Cancelling ctx aborts the handshake.
	handshakeDone := make(chan struct{})
	defer close(handshakeDone)
	go func() {
		select {
		case <-ctx.Done():
			conn.Close()
		case <-handshakeDone:
		}
	}()
	deadline := time.Now().Add(timeout)
	_ = conn.SetReadDeadline(deadline)
	read := func() (string, error) {
		typ, data, err := conn.ReadMessage()
		if err != nil {
			if ctx.Err() != nil {
				return "", ctx.Err()
			}
			var ne interface{ Timeout() bool }
			if errors.As(err, &ne) && ne.Timeout() {
				return "", errors.New("the Socket.IO server did not complete the handshake in time")
			}
			return "", err
		}
		if typ != websocket.TextMessage {
			return "", nil
		}
		return string(data), nil
	}

	first, err := read()
	if err != nil {
		return fail(err)
	}
	if first == "" || first[0] != eioOpen {
		return fail(fmt.Errorf("this does not look like a Socket.IO v3/v4 server (expected an Engine.IO open packet, got %q)", clip(first, 60)))
	}
	var open struct {
		SID          string `json:"sid"`
		PingInterval int    `json:"pingInterval"`
		PingTimeout  int    `json:"pingTimeout"`
	}
	if err := json.Unmarshal([]byte(first[1:]), &open); err != nil {
		return fail(fmt.Errorf("invalid Engine.IO open packet: %w", err))
	}
	if open.PingInterval <= 0 {
		open.PingInterval = 25_000
	}
	if open.PingTimeout <= 0 {
		open.PingTimeout = 20_000
	}

	c := &sioConn{
		base:        newBase(conn, Info{Handshake: sc.Handshake}),
		ns:          ns,
		readTimeout: time.Duration(open.PingInterval+open.PingTimeout) * time.Millisecond,
		pending:     map[int]string{},
	}
	if err := conn.WriteMessage(websocket.TextMessage, []byte(string(eioMessage)+string(sioConnect)+nsPrefix(ns)+auth)); err != nil {
		return fail(err)
	}
	for {
		msg, err := read()
		if err != nil {
			return fail(err)
		}
		if msg == "" {
			continue
		}
		switch msg[0] {
		case eioPing:
			if err := conn.WriteMessage(websocket.TextMessage, []byte{eioPong}); err != nil {
				return fail(err)
			}
			continue
		case eioClose:
			return fail(errors.New("the Socket.IO server closed the session during the handshake"))
		case eioMessage:
		default:
			continue
		}
		p, err := parsePacket(msg[1:])
		if err != nil || p.ns != ns {
			continue
		}
		switch p.typ {
		case sioConnect:
			var ok struct {
				SID string `json:"sid"`
			}
			_ = json.Unmarshal(p.data, &ok)
			c.info.SessionID = ok.SID
			_ = conn.SetReadDeadline(time.Time{})
			go c.read()
			return c, nil
		case sioConnectError:
			var e struct {
				Message string `json:"message"`
			}
			if json.Unmarshal(p.data, &e) != nil || e.Message == "" {
				e.Message = string(p.data)
			}
			return fail(fmt.Errorf("the Socket.IO server refused the connection to namespace %s: %s", ns, e.Message))
		}
	}
}

func (c *sioConn) read() {
	defer c.end()
	for {
		_ = c.conn.SetReadDeadline(time.Now().Add(c.readTimeout))
		typ, data, err := c.conn.ReadMessage()
		if err != nil {
			var ne interface{ Timeout() bool }
			if errors.As(err, &ne) && ne.Timeout() {
				err = errors.New("the Socket.IO server stopped sending pings (connection lost)")
			}
			c.fail(err)
			return
		}
		if typ != websocket.TextMessage || len(data) == 0 {
			// Binary frames are attachments of binary events, which are reported below.
			continue
		}
		switch data[0] {
		case eioPing:
			if err := c.write([]byte{eioPong}); err != nil {
				c.fail(err)
				return
			}
			continue
		case eioClose:
			c.fail(errors.New("the Socket.IO server closed the session"))
			return
		case eioMessage:
		default:
			continue
		}
		p, err := parsePacket(string(data[1:]))
		if err != nil || p.ns != c.ns {
			continue
		}
		m, ok := c.message(p)
		if p.typ == sioDisconnect {
			c.fail(errors.New("the Socket.IO server disconnected this client from the namespace"))
			return
		}
		if ok && !c.deliver(m) {
			return
		}
	}
}

// message turns an event or acknowledgement packet into a Message.
func (c *sioConn) message(p packet) (Message, bool) {
	now := time.Now()
	switch p.typ {
	case sioEvent, sioBinaryEvent:
		var arr []json.RawMessage
		if json.Unmarshal(p.data, &arr) != nil || len(arr) == 0 {
			return Message{}, false
		}
		var event string
		if json.Unmarshal(arr[0], &event) != nil {
			return Message{}, false
		}
		m := Message{Event: event, Data: argsData(arr[1:]), At: now}
		if p.typ == sioBinaryEvent {
			m.Data = []byte(`"(binary event: attachments are not supported)"`)
		}
		return m, true
	case sioAck, sioBinaryAck:
		c.ackMu.Lock()
		event, ok := c.pending[p.id]
		delete(c.pending, p.id)
		c.ackMu.Unlock()
		if !ok {
			return Message{}, false
		}
		var arr []json.RawMessage
		_ = json.Unmarshal(p.data, &arr)
		return Message{Event: event, Data: argsData(arr), Ack: true, At: now}, true
	}
	return Message{}, false
}

// argsData is the argument of an event: itself when there is one, an array of
// them when there are several, empty when there are none.
func argsData(args []json.RawMessage) []byte {
	switch len(args) {
	case 0:
		return nil
	case 1:
		return args[0]
	}
	out, _ := json.Marshal(args)
	return out
}

func (c *sioConn) Send(o Outgoing) error {
	if strings.TrimSpace(o.Event) == "" {
		return errors.New("Socket.IO messages need an event name")
	}
	payload := []any{o.Event}
	if o.Body != "" {
		if o.JSON {
			v, err := checkJSON(o.Body)
			if err != nil {
				return err
			}
			payload = append(payload, v)
		} else {
			payload = append(payload, o.Body)
		}
	}
	enc, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	id := ""
	if o.Ack {
		c.ackMu.Lock()
		n := c.nextAck
		c.nextAck++
		c.pending[n] = o.Event
		c.ackMu.Unlock()
		id = strconv.Itoa(n)
	}
	return c.write([]byte(string(eioMessage) + string(sioEvent) + nsPrefix(c.ns) + id + string(enc)))
}

func (c *sioConn) Close() {
	c.closeWith([]byte(string(eioMessage) + string(sioDisconnect) + nsPrefix(c.ns)))
}

func clip(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
