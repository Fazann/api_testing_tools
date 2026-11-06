package socket

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"api-test-kits/internal/domain"
	"api-test-kits/internal/httpclient"
	"api-test-kits/internal/socket/sockettest"
)

func client(t *testing.T) *httpclient.Client {
	t.Helper()
	c, err := httpclient.New(httpclient.Options{ProxyMode: httpclient.ProxyNone})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func next(t *testing.T, c Conn) Message {
	t.Helper()
	select {
	case m, ok := <-c.Messages():
		if !ok {
			_, _, err := c.Result()
			t.Fatalf("connection ended: %v", err)
		}
		return m
	case <-time.After(3 * time.Second):
		t.Fatal("no message within 3s")
	}
	return Message{}
}

func TestWebSocketSendReceiveAndServerClose(t *testing.T) {
	srv := sockettest.NewEcho(t)
	c, err := Open(context.Background(), client(t), Options{Kind: domain.KindWebSocket, URL: srv.URL("/chat"),
		Header: http.Header{"Authorization": {"Bearer x"}}, Subprotocols: []string{"chat.v1"}, Timeout: 3 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	if c.Info().Subprotocol != "chat.v1" || c.Info().Handshake.StatusCode != http.StatusSwitchingProtocols {
		t.Fatalf("info = %+v", c.Info())
	}
	if got := srv.Headers()[0].Get("Authorization"); got != "Bearer x" {
		t.Fatalf("Authorization = %q", got)
	}
	if m := next(t, c); string(m.Data) != "welcome" {
		t.Fatalf("first message = %q", m.Data)
	}
	if err := c.Send(Outgoing{Body: `{"a":1}`, JSON: true}); err != nil {
		t.Fatal(err)
	}
	if m := next(t, c); string(m.Data) != `{"a":1}` {
		t.Fatalf("echo = %q", m.Data)
	}
	if err := c.Send(Outgoing{Body: `{bad`, JSON: true}); err == nil {
		t.Fatal("invalid JSON was sent")
	}
	if err := c.Send(Outgoing{Body: "binary"}); err != nil {
		t.Fatal(err)
	}
	if m := next(t, c); !m.Binary || len(m.Data) != 2 {
		t.Fatalf("binary = %+v", m)
	}
	_ = c.Send(Outgoing{Body: "close"})
	select {
	case <-c.Done():
	case <-time.After(3 * time.Second):
		t.Fatal("connection did not end")
	}
	code, reason, err := c.Result()
	if code != 4000 || reason != "bye" || err == nil {
		t.Fatalf("result = %d %q %v", code, reason, err)
	}
	if err := c.Send(Outgoing{Body: "x"}); err != ErrClosed {
		t.Fatalf("send after close = %v", err)
	}
}

func TestWebSocketCloseIsNormalAndDoesNotBlockWithUnreadMessages(t *testing.T) {
	srv := sockettest.NewEcho(t)
	c, err := Open(context.Background(), client(t), Options{Kind: domain.KindWebSocket, URL: srv.URL("/"), Timeout: 3 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	// Nobody reads Messages: Close must still return.
	for i := 0; i < 300; i++ {
		_ = c.Send(Outgoing{Body: "x"})
	}
	done := make(chan struct{})
	go func() { c.Close(); close(done) }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Close blocked")
	}
	if _, _, err := c.Result(); err != nil {
		t.Fatalf("normal close reported %v", err)
	}
}

func TestEngineURL(t *testing.T) {
	cases := []struct{ in, path, url, ns string }{
		{"http://h:3000", "", "ws://h:3000/socket.io/?EIO=4&transport=websocket", "/"},
		{"https://h/admin?token=a", "/io", "wss://h/io/?EIO=4&token=a&transport=websocket", "/admin"},
		{"wss://h/", "/socket.io/", "wss://h/socket.io/?EIO=4&transport=websocket", "/"},
	}
	for _, c := range cases {
		u, ns, err := EngineURL(c.in, c.path)
		if err != nil || u != c.url || ns != c.ns {
			t.Errorf("EngineURL(%q, %q) = %q %q %v, want %q %q", c.in, c.path, u, ns, err, c.url, c.ns)
		}
	}
	if _, _, err := EngineURL("ftp://h", ""); err == nil {
		t.Error("ftp accepted")
	}
}

func TestParsePacket(t *testing.T) {
	p, err := parsePacket(`2/admin,12["ev",{"a":1}]`)
	if err != nil || p.typ != sioEvent || p.ns != "/admin" || p.id != 12 || string(p.data) != `["ev",{"a":1}]` {
		t.Fatalf("%+v %v", p, err)
	}
	p, err = parsePacket(`51-["up",{"_placeholder":true,"num":0}]`)
	if err != nil || p.typ != sioBinaryEvent || p.attachments != 1 || p.ns != "/" || p.id != -1 {
		t.Fatalf("%+v %v", p, err)
	}
	if p, _ := parsePacket("1/admin"); p.ns != "/admin" || p.typ != sioDisconnect {
		t.Fatalf("%+v", p)
	}
	if _, err := parsePacket(`2{bad`); err == nil {
		t.Fatal("invalid payload accepted")
	}
}

func TestSocketIOEventsAcksPingsAndNamespace(t *testing.T) {
	srv := sockettest.NewSocketIO(t)
	srv.Token = "good"
	c, err := Open(context.Background(), client(t), Options{Kind: domain.KindSocketIO, URL: srv.URL + "/admin",
		AuthPayload: `{"token":"good"}`, Timeout: 3 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if c.Info().SessionID != "socket-1" {
		t.Fatalf("session = %q", c.Info().SessionID)
	}
	if err := c.Send(Outgoing{Event: "echo", Body: `{"n":1}`, JSON: true, Ack: true}); err != nil {
		t.Fatal(err)
	}
	m := next(t, c)
	if m.Event != "echo" || string(m.Data) != `{"n":1}` || m.Ack {
		t.Fatalf("event = %+v", m)
	}
	m = next(t, c)
	if m.Event != "echo" || !m.Ack || string(m.Data) != `{"n":1}` {
		t.Fatalf("ack = %+v data %s", m, m.Data)
	}
	if got := srv.Received(); len(got) != 1 || got[0] != `42/admin,0["echo",{"n":1}]` {
		t.Fatalf("server received %q", got)
	}
	_ = c.Send(Outgoing{Event: "multi"})
	if m := next(t, c); m.Event != "multi" || string(m.Data) != "[1,2]" {
		t.Fatalf("multi = %+v %s", m, m.Data)
	}
	_ = c.Send(Outgoing{Event: "echo", Body: "plain"})
	if m := next(t, c); string(m.Data) != `"plain"` {
		t.Fatalf("text arg = %s", m.Data)
	}
	if err := c.Send(Outgoing{Body: "x"}); err == nil {
		t.Fatal("sent without an event name")
	}
	// The server pings every 200 ms; the client must answer.
	time.Sleep(500 * time.Millisecond)
	if srv.Pongs() == 0 {
		t.Fatal("pings were not answered")
	}
	_ = c.Send(Outgoing{Event: "kick"})
	select {
	case <-c.Done():
	case <-time.After(3 * time.Second):
		t.Fatal("kick did not end the connection")
	}
	if _, _, err := c.Result(); err == nil || !strings.Contains(err.Error(), "disconnected") {
		t.Fatalf("result = %v", err)
	}
}

func TestSocketIOConnectErrorAndWrongServer(t *testing.T) {
	srv := sockettest.NewSocketIO(t)
	srv.Token = "good"
	_, err := Open(context.Background(), client(t), Options{Kind: domain.KindSocketIO, URL: srv.URL,
		AuthPayload: `{"token":"bad"}`, Timeout: 3 * time.Second})
	if err == nil || !strings.Contains(err.Error(), "unauthorized") {
		t.Fatalf("err = %v", err)
	}
	if _, err := Open(context.Background(), client(t), Options{Kind: domain.KindSocketIO, URL: srv.URL,
		AuthPayload: `[1]`, Timeout: time.Second}); err == nil || !strings.Contains(err.Error(), "JSON object") {
		t.Fatalf("array auth payload: %v", err)
	}
	echo := sockettest.NewEcho(t)
	_, err = Open(context.Background(), client(t), Options{Kind: domain.KindSocketIO, URL: echo.URL("/"),
		Path: "/", Timeout: 3 * time.Second})
	if err == nil || !strings.Contains(err.Error(), "does not look like a Socket.IO") {
		t.Fatalf("plain WebSocket server: %v", err)
	}
}

func TestDialBlocksMetadataAndRefusedUpgrade(t *testing.T) {
	_, err := Open(context.Background(), client(t), Options{Kind: domain.KindWebSocket, URL: "ws://169.254.169.254/", Timeout: time.Second})
	if err == nil || !strings.Contains(err.Error(), "blocked") {
		t.Fatalf("metadata address: %v", err)
	}
	srv := sockettest.NewSocketIO(t)
	// /chat is not the Socket.IO path, so the upgrade is refused with 404.
	_, err = Open(context.Background(), client(t), Options{Kind: domain.KindWebSocket, URL: srv.URL + "/chat", Timeout: time.Second})
	if err == nil || !strings.Contains(err.Error(), "404") {
		t.Fatalf("refused upgrade: %v", err)
	}
}

func TestSocketIOClientDisconnectIsNormalEvenWhenTheServerDropsTheConnection(t *testing.T) {
	srv := sockettest.NewSocketIO(t)
	c, err := Open(context.Background(), client(t), Options{Kind: domain.KindSocketIO, URL: srv.URL, Timeout: 3 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	// The test server answers the disconnect packet by dropping the TCP
	// connection without a close frame, like real Socket.IO servers.
	c.Close()
	if code, reason, err := c.Result(); err != nil || code != 0 {
		t.Fatalf("result = %d %q %v", code, reason, err)
	}
}
