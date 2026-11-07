// Package sockettest provides local WebSocket and Socket.IO v4 servers for tests.
package sockettest

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin:  func(*http.Request) bool { return true },
	Subprotocols: []string{"chat.v1"},
}

// Echo is a WebSocket server that sends "welcome" on connect and echoes every
// text message. The message "close" makes it close with code 4000 "bye"; the
// message "binary" makes it send a binary frame.
type Echo struct {
	*httptest.Server
	mu      sync.Mutex
	headers []http.Header
}

// Headers returns the handshake headers of every connection so far.
func (e *Echo) Headers() []http.Header {
	e.mu.Lock()
	defer e.mu.Unlock()
	return append([]http.Header{}, e.headers...)
}

// URL returns the ws:// URL of the server with path appended.
func (e *Echo) URL(path string) string { return "ws" + strings.TrimPrefix(e.Server.URL, "http") + path }

func NewEcho(t testing.TB) *Echo {
	e := &Echo{}
	e.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		e.mu.Lock()
		e.headers = append(e.headers, r.Header.Clone())
		e.mu.Unlock()
		c, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer c.Close()
		_ = c.WriteMessage(websocket.TextMessage, []byte("welcome"))
		for {
			typ, data, err := c.ReadMessage()
			if err != nil {
				return
			}
			switch string(data) {
			case "close":
				_ = c.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(4000, "bye"), time.Now().Add(time.Second))
				return
			case "binary":
				_ = c.WriteMessage(websocket.BinaryMessage, []byte{0xde, 0xad})
				continue
			}
			_ = c.WriteMessage(typ, data)
		}
	}))
	t.Cleanup(e.Close)
	return e
}

// SocketIO is a minimal Socket.IO v4 server (Engine.IO 4, WebSocket transport)
// on /socket.io/. It accepts any namespace. With Token set, CONNECT must carry
// {"token": Token} or is refused with "unauthorized". Events:
//
//	"echo"  → emits "echo" with the same arguments, and acknowledges with them
//	"multi" → emits "multi" with arguments 1, 2
//	"kick"  → disconnects the client from the namespace
type SocketIO struct {
	*httptest.Server
	Token string
	// PingInterval in ms; the server pings the client this often.
	PingInterval int

	mu       sync.Mutex
	pongs    int
	auths    []string
	received []string
}

// Pongs is how many pings clients answered.
func (s *SocketIO) Pongs() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.pongs
}

// Auths returns the CONNECT payloads received.
func (s *SocketIO) Auths() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string{}, s.auths...)
}

// Received returns every Socket.IO packet received after CONNECT.
func (s *SocketIO) Received() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string{}, s.received...)
}

func NewSocketIO(t testing.TB) *SocketIO {
	s := &SocketIO{PingInterval: 200}
	s.Server = httptest.NewServer(http.HandlerFunc(s.handle))
	t.Cleanup(s.Close)
	return s
}

func (s *SocketIO) handle(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/socket.io/" || r.URL.Query().Get("EIO") != "4" || r.URL.Query().Get("transport") != "websocket" {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	c, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer c.Close()
	var wmu sync.Mutex
	send := func(s string) error {
		wmu.Lock()
		defer wmu.Unlock()
		return c.WriteMessage(websocket.TextMessage, []byte(s))
	}
	open, _ := json.Marshal(map[string]any{"sid": "engine-1", "upgrades": []string{}, "pingInterval": s.PingInterval,
		"pingTimeout": 1000, "maxPayload": 1_000_000})
	if send("0"+string(open)) != nil {
		return
	}
	stop := make(chan struct{})
	defer close(stop)
	go func() {
		t := time.NewTicker(time.Duration(s.PingInterval) * time.Millisecond)
		defer t.Stop()
		for {
			select {
			case <-stop:
				return
			case <-t.C:
				if send("2") != nil {
					return
				}
			}
		}
	}()

	ns := ""
	for {
		_, data, err := c.ReadMessage()
		if err != nil {
			return
		}
		msg := string(data)
		if msg == "3" {
			s.mu.Lock()
			s.pongs++
			s.mu.Unlock()
			continue
		}
		if !strings.HasPrefix(msg, "4") || len(msg) < 2 {
			continue
		}
		body := msg[2:]
		prefix := ""
		if strings.HasPrefix(body, "/") {
			i := strings.IndexByte(body, ',')
			if i < 0 {
				i = len(body) - 1
			}
			prefix, body = body[:i+1], body[i+1:]
		}
		switch msg[1] {
		case '0':
			s.mu.Lock()
			s.auths = append(s.auths, body)
			s.mu.Unlock()
			if s.Token != "" {
				var a struct {
					Token string `json:"token"`
				}
				if json.Unmarshal([]byte(body), &a) != nil || a.Token != s.Token {
					_ = send("44" + prefix + `{"message":"unauthorized"}`)
					continue
				}
			}
			ns = prefix
			_ = send("40" + ns + `{"sid":"socket-1"}`)
		case '1':
			return
		case '2':
			s.mu.Lock()
			s.received = append(s.received, msg)
			s.mu.Unlock()
			i := 0
			for i < len(body) && body[i] >= '0' && body[i] <= '9' {
				i++
			}
			id, payload := body[:i], body[i:]
			var arr []json.RawMessage
			if json.Unmarshal([]byte(payload), &arr) != nil || len(arr) == 0 {
				continue
			}
			var event string
			_ = json.Unmarshal(arr[0], &event)
			switch event {
			case "echo":
				_ = send("42" + ns + payload)
				if id != "" {
					args, _ := json.Marshal(arr[1:])
					n, _ := strconv.Atoi(id)
					_ = send("43" + ns + strconv.Itoa(n) + string(args))
				}
			case "multi":
				_ = send("42" + ns + `["multi",1,2]`)
			case "kick":
				_ = send("41" + ns)
			}
		}
	}
}

// Presence is a WebSocket server that behaves like a chat backend tracking
// online users: it answers every text message with {"type":"pong","seen":n}
// and closes the connection with 4001 "offline" when no message arrives for
// Idle.
type Presence struct {
	*httptest.Server
	Idle time.Duration
	mu   sync.Mutex
	at   []time.Time
	data []string
}

// Received returns the arrival time and text of every message so far.
func (p *Presence) Received() ([]time.Time, []string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]time.Time{}, p.at...), append([]string{}, p.data...)
}

// URL returns the ws:// URL of the server.
func (p *Presence) URL() string { return "ws" + strings.TrimPrefix(p.Server.URL, "http") + "/presence" }

func NewPresence(t testing.TB, idle time.Duration) *Presence {
	p := &Presence{Idle: idle}
	p.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer c.Close()
		for n := 1; ; n++ {
			_ = c.SetReadDeadline(time.Now().Add(p.Idle))
			_, data, err := c.ReadMessage()
			if err != nil {
				var ne interface{ Timeout() bool }
				if errors.As(err, &ne) && ne.Timeout() {
					_ = c.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(4001, "offline"), time.Now().Add(time.Second))
				}
				return
			}
			p.mu.Lock()
			p.at = append(p.at, time.Now())
			p.data = append(p.data, string(data))
			p.mu.Unlock()
			reply, _ := json.Marshal(map[string]any{"type": "pong", "seen": n})
			_ = c.WriteMessage(websocket.TextMessage, reply)
		}
	}))
	t.Cleanup(p.Close)
	return p
}
