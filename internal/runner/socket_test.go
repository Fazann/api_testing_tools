package runner

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"api-test-kits/internal/domain"
	"api-test-kits/internal/httpclient"
	"api-test-kits/internal/socket/sockettest"
)

func netClient(t *testing.T) *httpclient.Client {
	t.Helper()
	c, err := httpclient.New(httpclient.Options{ProxyMode: httpclient.ProxyNone})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func send(event, body string, typ domain.SocketBodyType) domain.SocketStep {
	return domain.SocketStep{Kind: domain.StepSend, Enabled: true, Message: domain.SocketMessage{Event: event, Body: body, BodyType: typ}}
}

func expect(event string, timeoutMs int, checks ...domain.Assertion) domain.SocketStep {
	return domain.SocketStep{Kind: domain.StepExpect, Enabled: true, Event: event, TimeoutMs: timeoutMs, Assertions: checks}
}

func check(path string, op domain.Operator, expected string) domain.Assertion {
	return domain.Assertion{Kind: domain.AssertJSONPath, Path: path, Operator: op, Expected: json.RawMessage(expected), Enabled: true}
}

func env(vars ...domain.Variable) *domain.Environment {
	return &domain.Environment{Name: "test", Variables: vars}
}

func TestWebSocketTestPassesWithVariablesAndRedactsSecrets(t *testing.T) {
	srv := sockettest.NewEcho(t)
	req := domain.Request{Name: "chat", Kind: domain.KindWebSocket, URLTemplate: "{{base}}/chat",
		Auth: domain.Auth{Type: domain.AuthBearer, Token: "{{token}}"},
		Socket: &domain.SocketConfig{Steps: []domain.SocketStep{
			expect("", 2000, domain.Assertion{Kind: domain.AssertBody, Operator: domain.OpEq, Expected: json.RawMessage(`"welcome"`), Enabled: true}),
			send("", `{"token":"{{token}}","n":"{{n}}"}`, domain.SocketBodyJSON),
			expect("", 2000, check("n", domain.OpEq, `"{{n}}"`)),
		}}}
	e := env(domain.Variable{Key: "base", Value: srv.URL(""), Enabled: true},
		domain.Variable{Key: "token", Value: "s3cret", Secret: true, Enabled: true},
		domain.Variable{Key: "n", Value: "7", Enabled: true})

	res := New(netClient(t)).Run(context.Background(), req, e, nil)
	if res.Status != domain.StatusPassed {
		t.Fatalf("status %s: %s %+v", res.Status, res.Error, res.AssertionResults)
	}
	if res.Request.Method != "WS" || res.Response == nil || res.Response.StatusCode != 101 {
		t.Fatalf("summary %+v response %+v", res.Request, res.Response)
	}
	if got := srv.Headers()[0].Get("Authorization"); got != "Bearer s3cret" {
		t.Fatalf("Authorization = %q", got)
	}
	if len(res.AssertionResults) != 3 {
		t.Fatalf("results = %+v", res.AssertionResults)
	}
	raw, _ := json.Marshal(res)
	if strings.Contains(string(raw), "s3cret") {
		t.Fatalf("secret leaked into the result: %s", raw)
	}
	var sent, recv int
	for _, f := range res.Socket.Frames {
		switch f.Direction {
		case domain.FrameSent:
			sent++
		case domain.FrameReceived:
			recv++
		}
	}
	if sent != 1 || recv != 2 {
		t.Fatalf("frames %+v", res.Socket.Frames)
	}
}

func TestExpectFailsWithClosestReasonAndKeepsUnmatchedMessages(t *testing.T) {
	srv := sockettest.NewEcho(t)
	req := domain.Request{Name: "chat", Kind: domain.KindWebSocket, URLTemplate: srv.URL("/"), TimeoutMs: 5000,
		Socket: &domain.SocketConfig{Steps: []domain.SocketStep{
			send("", `{"n":1}`, domain.SocketBodyJSON),
			send("", `{"n":2}`, domain.SocketBodyJSON),
			// Out of order: the second reply is expected first.
			expect("", 1000, check("n", domain.OpEq, "2")),
			expect("", 1000, check("n", domain.OpEq, "1")),
			expect("", 200, check("n", domain.OpEq, "3")),
		}}}
	res := New(netClient(t)).Run(context.Background(), req, nil, nil)
	if res.Status != domain.StatusFailed {
		t.Fatalf("status %s %s", res.Status, res.Error)
	}
	r := res.AssertionResults
	if !r[2].Passed || !r[3].Passed || r[4].Passed {
		t.Fatalf("results %+v", r)
	}
	if !strings.Contains(r[4].Message, "nothing matched within 200 ms") {
		t.Fatalf("message %q", r[4].Message)
	}
}

func TestSocketIOTestWithAckNamespaceAndConnectionErrors(t *testing.T) {
	srv := sockettest.NewSocketIO(t)
	srv.Token = "good"
	ack := expect("echo", 2000, check("ok", domain.OpEq, "true"))
	ack.ExpectAck = true
	sendAck := send("echo", `{"ok":true}`, domain.SocketBodyJSON)
	sendAck.Message.Ack = true
	req := domain.Request{Name: "events", Kind: domain.KindSocketIO, URLTemplate: srv.URL + "/admin",
		Socket: &domain.SocketConfig{AuthPayload: `{"token":"{{t}}"}`, Steps: []domain.SocketStep{
			sendAck,
			ack,
			expect("echo", 2000, check("ok", domain.OpEq, "true")),
			{Kind: domain.StepWait, Enabled: true, TimeoutMs: 10},
		}}}
	e := env(domain.Variable{Key: "t", Value: "good", Secret: true, Enabled: true})
	res := New(netClient(t)).Run(context.Background(), req, e, nil)
	if res.Status != domain.StatusPassed {
		t.Fatalf("status %s: %s %+v", res.Status, res.Error, res.AssertionResults)
	}
	if res.Request.Method != "SIO" || res.Socket.SessionID != "socket-1" {
		t.Fatalf("summary %+v socket %+v", res.Request, res.Socket)
	}
	if got := srv.Received(); len(got) != 1 || !strings.HasPrefix(got[0], "42/admin,0[") {
		t.Fatalf("server received %q", got)
	}

	req.Socket.AuthPayload = `{"token":"bad"}`
	res = New(netClient(t)).Run(context.Background(), req, nil, nil)
	if res.Status != domain.StatusError || !strings.Contains(res.Error, "unauthorized") {
		t.Fatalf("refused connection: %s %q", res.Status, res.Error)
	}
}

func TestStepsAfterServerCloseAreNotRunAndTimeoutBoundsTheTest(t *testing.T) {
	srv := sockettest.NewEcho(t)
	req := domain.Request{Name: "chat", Kind: domain.KindWebSocket, URLTemplate: srv.URL("/"),
		Socket: &domain.SocketConfig{Steps: []domain.SocketStep{
			send("", "close", domain.SocketBodyText),
			expect("", 2000, check("x", domain.OpExists, "")),
			send("", "after", domain.SocketBodyText),
		}}}
	res := New(netClient(t)).Run(context.Background(), req, nil, nil)
	if res.Status != domain.StatusFailed || res.Socket.CloseCode != 4000 {
		t.Fatalf("status %s close %d %+v", res.Status, res.Socket.CloseCode, res.AssertionResults)
	}
	if !strings.Contains(res.AssertionResults[1].Message, "connection closed") {
		t.Fatalf("expect after close: %q", res.AssertionResults[1].Message)
	}
	if !strings.HasPrefix(res.AssertionResults[2].Message, "could not send") && !strings.HasPrefix(res.AssertionResults[2].Message, "not run") {
		t.Fatalf("send after close: %q", res.AssertionResults[2].Message)
	}

	req = domain.Request{Name: "slow", Kind: domain.KindWebSocket, URLTemplate: srv.URL("/"), TimeoutMs: 300,
		Socket: &domain.SocketConfig{Steps: []domain.SocketStep{
			{Kind: domain.StepWait, Enabled: true, TimeoutMs: 5000},
			send("", "x", domain.SocketBodyText),
		}}}
	start := time.Now()
	res = New(netClient(t)).Run(context.Background(), req, nil, nil)
	if time.Since(start) > 2*time.Second || res.Status != domain.StatusFailed {
		t.Fatalf("took %v status %s", time.Since(start), res.Status)
	}
	if !strings.Contains(res.AssertionResults[1].Message, "request timeout (300 ms)") {
		t.Fatalf("after timeout: %q", res.AssertionResults[1].Message)
	}
}

func TestMissingVariableInStepIsReportedBeforeConnecting(t *testing.T) {
	req := domain.Request{Name: "chat", Kind: domain.KindWebSocket, URLTemplate: "ws://127.0.0.1:1/",
		Socket: &domain.SocketConfig{Steps: []domain.SocketStep{send("", "{{nope}}", domain.SocketBodyText)}}}
	res := New(netClient(t)).Run(context.Background(), req, nil, nil)
	if res.Status != domain.StatusError || !strings.Contains(res.Error, "nope") {
		t.Fatalf("%s %q", res.Status, res.Error)
	}
}

func TestSocketValidation(t *testing.T) {
	cases := map[string]domain.SocketConfig{
		"event name":     {Steps: []domain.SocketStep{send("", "x", domain.SocketBodyText)}},
		"reserved event": {Steps: []domain.SocketStep{send("connect", "x", domain.SocketBodyText)}},
		"status check":   {Steps: []domain.SocketStep{expect("", 100, domain.Assertion{Kind: domain.AssertStatus, Operator: domain.OpEq, Expected: json.RawMessage("200"), Enabled: true})}},
		"wait range":     {Steps: []domain.SocketStep{{Kind: domain.StepWait, Enabled: true, TimeoutMs: 120_000}}},
	}
	for name, cfg := range cases {
		cfg := cfg
		r := domain.Request{Name: "x", Kind: domain.KindSocketIO, URLTemplate: "http://h", Socket: &cfg}
		r.Normalize()
		if err := r.Validate(); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	r := domain.Request{Name: "x", Kind: domain.KindWebSocket, URLTemplate: "ws://h", Method: "POST"}
	r.Normalize()
	if err := r.Validate(); err != nil || r.Method != "GET" || r.Socket == nil {
		t.Fatalf("websocket defaults: %v %+v", err, r)
	}
	h := domain.Request{Name: "x", URLTemplate: "http://h", Socket: &domain.SocketConfig{}}
	h.Normalize()
	if h.Kind != domain.KindHTTP || h.Socket != nil {
		t.Fatalf("http request kept socket settings: %+v", h)
	}
}
