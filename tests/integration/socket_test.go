package integration

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"api-test-kits/internal/cli"
	"api-test-kits/internal/domain"
	"api-test-kits/internal/service"
	"api-test-kits/internal/socket/sockettest"
	"api-test-kits/internal/storage"
)

const socketToken = "sio-s3cret"

type socketFixture struct {
	svc  *service.Service
	srv  *sockettest.SocketIO
	echo *sockettest.Echo
	env  domain.Environment
	coll domain.Collection
}

func setupSockets(t *testing.T) *socketFixture {
	t.Helper()
	ctx := context.Background()
	store, err := storage.Open(filepath.Join(t.TempDir(), "data.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	svc, err := service.New(store)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(svc.CloseSockets)
	st := svc.GetSettings()
	st.ProxyMode = "none"
	if err := svc.SaveSettings(ctx, st); err != nil {
		t.Fatal(err)
	}
	srv := sockettest.NewSocketIO(t)
	srv.Token = socketToken
	echo := sockettest.NewEcho(t)
	env, err := svc.SaveEnvironment(ctx, domain.Environment{Name: "Sockets", Variables: []domain.Variable{
		{Key: "sio_url", Value: srv.URL, Enabled: true},
		{Key: "ws_url", Value: echo.URL(""), Enabled: true},
		{Key: "token", Value: socketToken, Secret: true, Enabled: true},
	}})
	if err != nil {
		t.Fatal(err)
	}
	coll, err := svc.SaveCollection(ctx, domain.Collection{Name: "Realtime"})
	if err != nil {
		t.Fatal(err)
	}
	return &socketFixture{svc: svc, srv: srv, echo: echo, env: env, coll: coll}
}

func (f *socketFixture) saveTests(t *testing.T) {
	t.Helper()
	ctx := context.Background()
	sendAck := domain.SocketStep{Kind: domain.StepSend, Enabled: true,
		Message: domain.SocketMessage{Event: "echo", Body: `{"user":"{{token}}","ok":true}`, BodyType: domain.SocketBodyJSON, Ack: true}}
	expectAck := domain.SocketStep{Kind: domain.StepExpect, Enabled: true, Event: "echo", ExpectAck: true, TimeoutMs: 2000,
		Assertions: []domain.Assertion{{Kind: domain.AssertJSONPath, Path: "ok", Operator: domain.OpEq, Expected: json.RawMessage("true"), Enabled: true}}}
	if _, err := f.svc.SaveRequest(ctx, domain.Request{CollectionID: f.coll.ID, Name: "Socket.IO echo", Kind: domain.KindSocketIO,
		URLTemplate: "{{sio_url}}/admin", SortOrder: 1,
		Socket: &domain.SocketConfig{AuthPayload: `{"token":"{{token}}"}`, Steps: []domain.SocketStep{sendAck, expectAck}}}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.SaveRequest(ctx, domain.Request{CollectionID: f.coll.ID, Name: "WebSocket welcome", Kind: domain.KindWebSocket,
		URLTemplate: "{{ws_url}}/chat", SortOrder: 2,
		Socket: &domain.SocketConfig{Steps: []domain.SocketStep{{Kind: domain.StepExpect, Enabled: true, TimeoutMs: 2000,
			Assertions: []domain.Assertion{{Kind: domain.AssertBody, Operator: domain.OpEq, Expected: json.RawMessage(`"welcome"`), Enabled: true}}}}}}); err != nil {
		t.Fatal(err)
	}
}

func TestSocketRequestsRunInCollectionsHistoryExportAndCLI(t *testing.T) {
	f := setupSockets(t)
	f.saveTests(t)
	ctx := context.Background()

	run, err := f.svc.RunCollection(ctx, "", f.coll.ID, "", f.env.ID, true, nil)
	if err != nil {
		t.Fatal(err)
	}
	if run.Status != domain.StatusPassed || run.PassedCount != 2 {
		t.Fatalf("run %s passed %d: %+v", run.Status, run.PassedCount, run.Results)
	}
	stored, err := f.svc.GetRun(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Results[0].Socket == nil || len(stored.Results[0].Socket.Frames) == 0 || stored.Results[0].Request.Method != "SIO" {
		t.Fatalf("history lost the transcript: %+v", stored.Results[0])
	}
	raw, _ := json.Marshal(stored)
	if strings.Contains(string(raw), socketToken) {
		t.Fatal("history leaked the token")
	}
	report, err := f.svc.ExportReport(ctx, run.ID, "html")
	if err != nil || strings.Contains(string(report), socketToken) || !strings.Contains(string(report), "Messages · ") ||
		!strings.Contains(string(report), "↑ sent") {
		t.Fatalf("html report: %v", err)
	}

	// Export, then run the export with the CLI.
	data, err := f.svc.ExportCollection(ctx, f.coll.ID, nil, false, false)
	if err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(t.TempDir(), "realtime.collection.json")
	if err := os.WriteFile(file, data, 0o600); err != nil {
		t.Fatal(err)
	}
	res := runCLI(t, []string{"APIGUARD_VAR_token=" + socketToken}, "run", file,
		"--var", "sio_url="+f.srv.URL, "--var", "ws_url="+f.echo.URL(""))
	if res.code != cli.ExitPassed || !strings.Contains(res.stdout, "2 passed") {
		t.Fatalf("exit %d\n%s\n%s", res.code, res.stdout, res.stderr)
	}
	if strings.Contains(res.stdout, socketToken) {
		t.Fatal("CLI output leaked the token")
	}
	imp, err := f.svc.ImportCollection(ctx, data)
	if err != nil {
		t.Fatal(err)
	}
	tree, err := f.svc.GetCollectionTree(ctx, imp.Collection.ID)
	if err != nil {
		t.Fatal(err)
	}
	if tree.Requests[0].Kind != domain.KindSocketIO || len(tree.Requests[0].Socket.Steps) != 2 {
		t.Fatalf("import lost socket settings: %+v", tree.Requests[0])
	}
}

func TestSocketRequestsAreRefusedByRaceTestsAndMatrices(t *testing.T) {
	f := setupSockets(t)
	f.saveTests(t)
	ctx := context.Background()
	tree, _ := f.svc.GetCollectionTree(ctx, f.coll.ID)
	_, err := f.svc.RunRaceTest(ctx, "", domain.RaceTest{RequestID: tree.Requests[0].ID, Count: 2, SuccessOp: "lte", SuccessCount: 1}, f.env.ID, true)
	if err == nil || !strings.Contains(err.Error(), "not supported") {
		t.Fatalf("race test: %v", err)
	}
	m, _ := f.svc.GetPermissionMatrix(ctx, f.coll.ID)
	m.Roles = []domain.Role{{ID: "r", Name: "anon", Anonymous: true}}
	m.RequestIDs = []string{tree.Requests[0].ID}
	if _, err := f.svc.SavePermissionMatrix(ctx, m); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.RunPermissionMatrix(ctx, "", f.coll.ID, f.env.ID, true, nil); err == nil {
		t.Fatal("a matrix of only socket requests ran")
	}
}

type events struct {
	mu   sync.Mutex
	list []service.SocketEvent
	got  chan struct{}
}

func (e *events) emit(ev service.SocketEvent) {
	e.mu.Lock()
	e.list = append(e.list, ev)
	e.mu.Unlock()
	select {
	case e.got <- struct{}{}:
	default:
	}
}

// waitFor waits until an event satisfies ok.
func (e *events) waitFor(t *testing.T, ok func(service.SocketEvent) bool) service.SocketEvent {
	t.Helper()
	deadline := time.After(3 * time.Second)
	for {
		e.mu.Lock()
		for _, ev := range e.list {
			if ok(ev) {
				e.mu.Unlock()
				return ev
			}
		}
		e.mu.Unlock()
		select {
		case <-e.got:
		case <-deadline:
			t.Fatalf("event not received; got %+v", e.list)
		}
	}
}

func TestSocketConsoleConnectSendReceiveAndDisconnect(t *testing.T) {
	f := setupSockets(t)
	ctx := context.Background()
	ev := &events{got: make(chan struct{}, 1)}
	req := domain.Request{CollectionID: f.coll.ID, Name: "console", Kind: domain.KindSocketIO, URLTemplate: "{{sio_url}}",
		PreRequestScript: `pm.environment.set("greeting", "hi");`,
		Socket:           &domain.SocketConfig{AuthPayload: `{"token":"{{token}}"}`}}

	sess, err := f.svc.ConnectSocket(ctx, "s1", req, f.env.ID, false, ev.emit)
	if err != nil {
		t.Fatal(err)
	}
	if sess.StatusCode != 101 || sess.EngineSessionID != "socket-1" {
		t.Fatalf("session %+v", sess)
	}
	frame, err := f.svc.SendSocketMessage("s1", domain.SocketMessage{Event: "echo", Body: `{"say":"{{greeting}}","t":"{{token}}"}`,
		BodyType: domain.SocketBodyJSON})
	if err != nil {
		t.Fatal(err)
	}
	if frame.Direction != domain.FrameSent || strings.Contains(frame.Data, socketToken) || !strings.Contains(frame.Data, `"hi"`) {
		t.Fatalf("sent frame %+v", frame)
	}
	got := ev.waitFor(t, func(e service.SocketEvent) bool { return e.Frame != nil && e.Frame.Event == "echo" })
	if strings.Contains(got.Frame.Data, socketToken) || !strings.Contains(got.Frame.Data, "[REDACTED]") {
		t.Fatalf("received frame not redacted: %+v", got.Frame)
	}
	if _, err := f.svc.SendSocketMessage("s1", domain.SocketMessage{Event: "echo", Body: "{{missing}}"}); err == nil {
		t.Fatal("missing variable was sent")
	}
	if !f.svc.DisconnectSocket("s1") {
		t.Fatal("session was not open")
	}
	ev.waitFor(t, func(e service.SocketEvent) bool { return e.Closed && e.Error == "" })
	if _, err := f.svc.SendSocketMessage("s1", domain.SocketMessage{Event: "echo"}); err == nil {
		t.Fatal("sent on a closed session")
	}

	// A protected environment needs confirmation before connecting.
	f.env.Protected = true
	if _, err := f.svc.SaveEnvironment(ctx, f.env); err != nil {
		t.Fatal(err)
	}
	var confirm *service.ConfirmationRequiredError
	if _, err := f.svc.ConnectSocket(ctx, "s2", req, f.env.ID, false, ev.emit); !errors.As(err, &confirm) {
		t.Fatalf("protected environment: %v", err)
	}
	if _, err := f.svc.ConnectSocket(ctx, "s2", req, f.env.ID, true, ev.emit); err != nil {
		t.Fatalf("confirmed: %v", err)
	}
}
