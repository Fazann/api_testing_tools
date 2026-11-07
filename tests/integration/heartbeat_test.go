package integration

import (
	"context"
	"strings"
	"testing"
	"time"

	"api-test-kits/internal/domain"
	"api-test-kits/internal/service"
	"api-test-kits/internal/socket/sockettest"
)

// The console heartbeat keeps a chat-style server from marking the user
// offline; once it stops, the server drops the connection.
func TestConsoleHeartbeatKeepsUserOnlineUntilStopped(t *testing.T) {
	f := setupSockets(t)
	ctx := context.Background()
	srv := sockettest.NewPresence(t, 400*time.Millisecond)
	ev := &events{got: make(chan struct{}, 1)}
	req := domain.Request{CollectionID: f.coll.ID, Name: "presence", Kind: domain.KindWebSocket, URLTemplate: srv.URL(),
		Socket: &domain.SocketConfig{}}
	if _, err := f.svc.ConnectSocket(ctx, "hb", req, f.env.ID, false, ev.emit); err != nil {
		t.Fatal(err)
	}

	beat := domain.SocketMessage{Body: `{"type":"ping","token":"{{token}}","id":"{{$uuid}}"}`, BodyType: domain.SocketBodyJSON}
	if _, err := f.svc.StartSocketHeartbeat("hb", beat, 50); err == nil {
		t.Fatal("an interval below 100 ms was accepted")
	}
	if _, err := f.svc.StartSocketHeartbeat("hb", domain.SocketMessage{Body: "{{nope}}"}, 100); err == nil {
		t.Fatal("a heartbeat with a missing variable was started")
	}
	st, err := f.svc.StartSocketHeartbeat("hb", beat, 100)
	if err != nil || !st.Running || st.IntervalMs != 100 {
		t.Fatalf("start = %+v, %v", st, err)
	}

	// Heartbeats arrive as sent frames marked Heartbeat, with a running count; the server answers each.
	got := ev.waitFor(t, func(e service.SocketEvent) bool { return e.Heartbeat != nil && e.Heartbeat.Sent >= 8 })
	if !got.Heartbeat.Running || got.Frame == nil || !got.Frame.Heartbeat || got.Frame.Direction != domain.FrameSent {
		t.Fatalf("heartbeat event = %+v", got)
	}
	if strings.Contains(got.Frame.Data, socketToken) || !strings.Contains(got.Frame.Data, "[REDACTED]") {
		t.Fatalf("heartbeat frame not redacted: %s", got.Frame.Data)
	}
	ev.waitFor(t, func(e service.SocketEvent) bool { return e.Frame != nil && strings.Contains(e.Frame.Data, `"pong"`) })
	// 8 × 100 ms is twice the idle timeout: still connected only thanks to the heartbeat.
	for _, e := range ev.snapshot() {
		if e.Closed {
			t.Fatalf("closed while the heartbeat was running: %+v", e)
		}
	}

	stopped := f.svc.StopSocketHeartbeat("hb")
	if stopped.Running || stopped.Sent < 8 || stopped.Error != "" {
		t.Fatalf("stop = %+v", stopped)
	}
	ev.waitFor(t, func(e service.SocketEvent) bool {
		return e.Heartbeat != nil && !e.Heartbeat.Running && e.Frame != nil && strings.HasPrefix(e.Frame.Data, "heartbeat stopped after")
	})
	// Silent now: the server marks the user offline by closing with 4001.
	closed := ev.waitFor(t, func(e service.SocketEvent) bool { return e.Closed })
	if closed.CloseCode != 4001 {
		t.Fatalf("closed = %+v", closed)
	}
	_, data := srv.Received()
	if len(data) < 8 || data[0] == data[len(data)-1] || !strings.Contains(data[0], socketToken) {
		t.Fatalf("the server received %d heartbeats: %q", len(data), data)
	}
	if st := f.svc.StopSocketHeartbeat("hb"); st.Running || st.Sent != 0 {
		t.Fatalf("stopping a closed session = %+v", st)
	}
}

// Disconnecting stops a running heartbeat.
func TestDisconnectStopsConsoleHeartbeat(t *testing.T) {
	f := setupSockets(t)
	ctx := context.Background()
	srv := sockettest.NewPresence(t, time.Second)
	ev := &events{got: make(chan struct{}, 1)}
	req := domain.Request{CollectionID: f.coll.ID, Name: "p", Kind: domain.KindWebSocket, URLTemplate: srv.URL(),
		Socket: &domain.SocketConfig{}}
	if _, err := f.svc.ConnectSocket(ctx, "hb2", req, f.env.ID, false, ev.emit); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.StartSocketHeartbeat("hb2", domain.SocketMessage{Body: "ping", BodyType: domain.SocketBodyText}, 100); err != nil {
		t.Fatal(err)
	}
	ev.waitFor(t, func(e service.SocketEvent) bool { return e.Heartbeat != nil && e.Heartbeat.Sent >= 2 })
	f.svc.DisconnectSocket("hb2")
	ev.waitFor(t, func(e service.SocketEvent) bool { return e.Closed })
	n := len(func() []time.Time { at, _ := srv.Received(); return at }())
	time.Sleep(300 * time.Millisecond)
	if at, _ := srv.Received(); len(at) != n {
		t.Fatalf("heartbeats continued after disconnecting: %d → %d", n, len(at))
	}
}
