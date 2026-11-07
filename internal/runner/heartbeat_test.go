package runner

import (
	"context"
	"strings"
	"testing"
	"time"

	"api-test-kits/internal/domain"
	"api-test-kits/internal/socket/sockettest"
)

func heartbeatStart(body string, intervalMs int) domain.SocketStep {
	return domain.SocketStep{Kind: domain.StepHeartbeatStart, Enabled: true, IntervalMs: intervalMs,
		Message: domain.SocketMessage{Body: body, BodyType: domain.SocketBodyJSON}}
}

func wait(ms int) domain.SocketStep {
	return domain.SocketStep{Kind: domain.StepWait, Enabled: true, TimeoutMs: ms}
}

func presenceRequest(url string, steps ...domain.SocketStep) domain.Request {
	return domain.Request{Name: "presence", Kind: domain.KindWebSocket, URLTemplate: url, TimeoutMs: 10_000,
		Socket: &domain.SocketConfig{Steps: steps}}
}

// A chat server keeps a client online while heartbeats arrive and drops it
// once they stop: start, stay online, stop, expect the disconnect.
func TestHeartbeatKeepsConnectionOnlineAndExpectDisconnectAfterStop(t *testing.T) {
	srv := sockettest.NewPresence(t, 300*time.Millisecond)
	req := presenceRequest(srv.URL(),
		heartbeatStart(`{"type":"ping","at":"{{$timestamp}}","id":"{{$uuid}}"}`, 100),
		expect("", 1000, check("type", domain.OpEq, `"pong"`)),
		wait(700), // longer than the idle timeout: only the heartbeat keeps us online
		domain.SocketStep{Kind: domain.StepHeartbeatStop, Enabled: true},
		domain.SocketStep{Kind: domain.StepExpectClose, Enabled: true, TimeoutMs: 2000},
		send("", `"too late"`, domain.SocketBodyJSON),
	)
	res := New(netClient(t)).Run(context.Background(), req, env(), nil)
	r := res.AssertionResults
	if len(r) != 6 {
		t.Fatalf("results = %+v", r)
	}
	for i := 0; i < 5; i++ {
		if !r[i].Passed {
			t.Fatalf("step %d failed: %+v", i+1, r[i])
		}
	}
	if r[0].Actual != "every 100 ms" || !strings.HasPrefix(r[0].Description, "Step 1: start heartbeat {") ||
		!strings.HasSuffix(r[0].Description, " every 100 ms") {
		t.Errorf("start result = %+v", r[0])
	}
	if !strings.Contains(r[3].Actual, "heartbeat(s) sent") {
		t.Errorf("stop result = %+v", r[3])
	}
	if r[4].Actual != "closed (4001 offline)" {
		t.Errorf("expect disconnect = %+v", r[4])
	}
	if r[5].Passed || r[5].Message != "not run: the server closed the connection" {
		t.Errorf("a step after the disconnect = %+v", r[5])
	}

	at, data := srv.Received()
	if len(at) < 6 {
		t.Fatalf("only %d heartbeats arrived", len(at))
	}
	// Built-in variables are filled in again for every heartbeat.
	if data[0] == data[1] || !strings.Contains(data[1], `"type":"ping"`) {
		t.Errorf("heartbeats should differ: %q / %q", data[0], data[1])
	}
	// The rhythm is the interval (generous bounds for slow CI machines).
	for i := 1; i < len(at); i++ {
		if gap := at[i].Sub(at[i-1]); gap < 60*time.Millisecond || gap > 250*time.Millisecond {
			t.Errorf("gap %d = %v, want about 100ms", i, gap)
		}
	}
	beats := 0
	for _, f := range res.Socket.Frames {
		if f.Heartbeat {
			beats++
		}
	}
	if beats != len(at) {
		t.Errorf("%d heartbeat frames in the transcript, %d heartbeats received", beats, len(at))
	}
}

func TestExpectDisconnectFailsWhileHeartbeatKeepsItOpen(t *testing.T) {
	srv := sockettest.NewPresence(t, 300*time.Millisecond)
	req := presenceRequest(srv.URL(),
		heartbeatStart(`{"type":"ping"}`, 100),
		domain.SocketStep{Kind: domain.StepExpectClose, Enabled: true, TimeoutMs: 600},
	)
	res := New(netClient(t)).Run(context.Background(), req, env(), nil)
	r := res.AssertionResults
	if res.Status != domain.StatusFailed || len(r) != 2 || r[1].Passed ||
		r[1].Message != "the connection was still open after 600 ms" {
		t.Fatalf("status %s, results %+v", res.Status, r)
	}
	// The heartbeat still running at the end is stopped and noted.
	noted := false
	for _, f := range res.Socket.Frames {
		noted = noted || (f.Direction == domain.FrameSystem && strings.HasPrefix(f.Data, "heartbeat stopped at the end of the test: "))
	}
	if !noted {
		t.Errorf("no note that the heartbeat was stopped: %+v", res.Socket.Frames)
	}
}

func TestHeartbeatStopsWhenTheServerCloses(t *testing.T) {
	srv := sockettest.NewEcho(t)
	req := domain.Request{Name: "echo", Kind: domain.KindWebSocket, URLTemplate: srv.URL(""), TimeoutMs: 10_000,
		Socket: &domain.SocketConfig{Steps: []domain.SocketStep{
			{Kind: domain.StepHeartbeatStart, Enabled: true, IntervalMs: 100, Message: domain.SocketMessage{Body: "close", BodyType: domain.SocketBodyText}},
			wait(300),
			{Kind: domain.StepHeartbeatStop, Enabled: true},
		}}}
	res := New(netClient(t)).Run(context.Background(), req, env(), nil)
	r := res.AssertionResults
	if len(r) != 3 || !r[0].Passed || !strings.Contains(r[2].Actual, "stopped early") {
		t.Fatalf("status %s error %q results = %+v", res.Status, res.Error, r)
	}
}

func TestHeartbeatValidation(t *testing.T) {
	for _, ms := range []int{50, 600_001} {
		req := presenceRequest("ws://x", heartbeatStart(`{}`, ms))
		req.Normalize()
		if err := req.Validate(); err == nil || !strings.Contains(err.Error(), "heartbeat interval") {
			t.Errorf("interval %d: %v", ms, err)
		}
	}
	req := presenceRequest("ws://x", heartbeatStart(`{}`, 0), domain.SocketStep{Kind: domain.StepExpectClose, Enabled: true})
	req.Normalize()
	if err := req.Validate(); err != nil {
		t.Fatal(err)
	}
	if req.Socket.Steps[0].IntervalMs != domain.DefaultHeartbeatIntervalMs || req.Socket.Steps[1].TimeoutMs != domain.DefaultExpectTimeout {
		t.Errorf("defaults = %+v", req.Socket.Steps)
	}
	if !req.Socket.SendsMessages() {
		t.Error("a heartbeat sends messages")
	}
}
