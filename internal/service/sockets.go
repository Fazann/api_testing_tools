package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"api-test-kits/internal/domain"
	"api-test-kits/internal/environments"
	"api-test-kits/internal/redact"
	"api-test-kits/internal/runner"
	"api-test-kits/internal/socket"
)

// MaxSocketSessions bounds how many console connections can be open at once.
const MaxSocketSessions = 20

// SocketEvent tells the console what happened on an open session.
type SocketEvent struct {
	SessionID string `json:"sessionId"`
	// Frame is a received message or a connection event, already redacted.
	Frame *domain.SocketFrame `json:"frame,omitempty"`
	// Closed is set once, when the session has ended.
	Closed    bool   `json:"closed,omitempty"`
	CloseCode int    `json:"closeCode,omitempty"`
	Error     string `json:"error,omitempty"`
}

// SocketSession describes a console connection that has just opened.
type SocketSession struct {
	SessionID   string             `json:"sessionId"`
	Kind        domain.RequestKind `json:"kind"`
	URL         string             `json:"url"`
	Subprotocol string             `json:"subprotocol,omitempty"`
	// EngineSessionID is the Socket.IO session ID.
	EngineSessionID string            `json:"engineSessionId,omitempty"`
	StatusCode      int               `json:"statusCode"`
	Headers         []domain.KeyValue `json:"headers"`
	// Logs are the pre-request scripts' console output, redacted.
	Logs []domain.ScriptLog `json:"logs,omitempty"`
}

type socketSession struct {
	kind  domain.RequestKind
	conn  socket.Conn
	red   *redact.Redactor
	env   *domain.Environment
	start time.Time
}

// ConnectSocket opens a console connection for a WebSocket or Socket.IO
// request (saved or a draft). It runs the pre-request scripts and keeps the
// variables they set, like Send. Received messages and the end of the
// connection are reported through emit, from another goroutine. In a
// protected environment the user must confirm first, because messages sent
// from the console may change data. The handshake can be cancelled with
// Cancel(sessionID).
func (s *Service) ConnectSocket(ctx context.Context, sessionID string, req domain.Request, envID string, confirmed bool, emit func(SocketEvent)) (SocketSession, error) {
	if sessionID == "" {
		return SocketSession{}, errors.New("a session ID is required")
	}
	req.Normalize()
	if !req.Kind.IsSocket() {
		return SocketSession{}, errors.New("only WebSocket and Socket.IO requests can be connected")
	}
	if err := req.Validate(); err != nil {
		return SocketSession{}, err
	}
	s.DisconnectSocket(sessionID)
	s.mu.Lock()
	open := len(s.sockets)
	s.mu.Unlock()
	if open >= MaxSocketSessions {
		return SocketSession{}, fmt.Errorf("at most %d socket connections can be open; disconnect one first", MaxSocketSessions)
	}

	warnings, err := s.Preflight(ctx, []domain.Request{req}, nil, envID)
	if err != nil {
		return SocketSession{}, err
	}
	env, err := s.environment(ctx, envID)
	if err != nil {
		return SocketSession{}, err
	}
	if env != nil && env.Protected && !confirmed {
		warnings = append(warnings, Warning{Level: LevelWarning, Code: "protected_socket", RequiresConfirmation: true,
			Message: fmt.Sprintf("%q is a protected environment and messages sent from the console may modify data", env.Name)})
		return SocketSession{}, &ConfirmationRequiredError{Warnings: warnings}
	}
	reqs, _, c, err := s.scope(ctx, []domain.Request{req}, env)
	if err != nil {
		return SocketSession{}, err
	}
	scope := runScope(env, c)
	locals := map[string]string{}

	opCtx, done := s.beginOp(ctx, sessionID)
	defer done()
	req, resolved, prepared, logs, err := runner.PreRequest(opCtx, reqs[0], scope, locals)
	red := redact.New(scope.Vars.Resolved(locals).SecretValues()...)
	if prepared != nil {
		red = prepared.Redactor
	}
	for i := range logs {
		logs[i].Message = red.String(logs[i].Message)
	}
	if err != nil {
		return SocketSession{}, err
	}
	if err := s.saveScriptVariables(context.WithoutCancel(ctx), env, c, scope.Vars); err != nil {
		return SocketSession{}, fmt.Errorf("variables set by scripts could not be saved: %w", err)
	}
	client, _ := s.current()
	conn, err := runner.OpenSocket(opCtx, client, req, prepared)
	if err != nil {
		return SocketSession{}, errors.New(red.String(err.Error()))
	}

	sess := &socketSession{kind: req.Kind, conn: conn, red: red, env: resolved, start: time.Now()}
	s.mu.Lock()
	s.sockets[sessionID] = sess
	s.mu.Unlock()
	go s.pumpSocket(sessionID, sess, emit)

	info := conn.Info()
	out := SocketSession{SessionID: sessionID, Kind: req.Kind, URL: prepared.Summary.URL, Subprotocol: info.Subprotocol,
		EngineSessionID: info.SessionID, Headers: []domain.KeyValue{}, Logs: logs}
	if h := info.Handshake; h != nil {
		out.StatusCode, out.Headers = h.StatusCode, red.HTTPHeader(h.Header)
	}
	return out, nil
}

// pumpSocket forwards received messages until the connection ends.
func (s *Service) pumpSocket(id string, sess *socketSession, emit func(SocketEvent)) {
	for m := range sess.conn.Messages() {
		f := runner.Frame(m, sess.red, sess.start)
		emit(SocketEvent{SessionID: id, Frame: &f})
	}
	code, reason, err := sess.conn.Result()
	ev := SocketEvent{SessionID: id, Closed: true, CloseCode: code}
	text := "closed"
	switch {
	case err != nil:
		ev.Error = sess.red.String(err.Error())
		text = "closed: " + ev.Error
	case code != 0:
		text = fmt.Sprintf("closed (%d %s)", code, sess.red.String(reason))
	}
	ev.Frame = &domain.SocketFrame{Direction: domain.FrameSystem, Data: text, AtMs: time.Since(sess.start).Milliseconds()}
	s.mu.Lock()
	if s.sockets[id] == sess {
		delete(s.sockets, id)
	}
	s.mu.Unlock()
	emit(ev)
}

// SendSocketMessage resolves variables in m and sends it on an open console
// session. It returns the transcript frame of what was sent.
func (s *Service) SendSocketMessage(sessionID string, m domain.SocketMessage) (domain.SocketFrame, error) {
	s.mu.Lock()
	sess := s.sockets[sessionID]
	s.mu.Unlock()
	if sess == nil {
		return domain.SocketFrame{}, errors.New("the connection is closed; connect again")
	}
	cfg := domain.SocketConfig{Messages: []domain.SocketMessage{m}}
	probe := domain.Request{Name: "message", Kind: sess.kind, URLTemplate: "ws://x", Socket: &cfg}
	probe.Normalize()
	if err := probe.Validate(); err != nil {
		return domain.SocketFrame{}, err
	}
	res := environments.NewResolver(sess.env)
	resolved := runner.ResolveSocketMessage(probe.Socket.Messages[0], res)
	if missing := res.Missing(); len(missing) > 0 {
		return domain.SocketFrame{}, &runner.MissingVariablesError{Names: missing}
	}
	if err := sess.conn.Send(runner.Outgoing(resolved)); err != nil {
		return domain.SocketFrame{}, errors.New(sess.red.String(err.Error()))
	}
	return runner.SentFrame(resolved, sess.red, time.Since(sess.start)), nil
}

// DisconnectSocket closes a console session; it reports whether one was open.
func (s *Service) DisconnectSocket(sessionID string) bool {
	s.mu.Lock()
	sess := s.sockets[sessionID]
	delete(s.sockets, sessionID)
	s.mu.Unlock()
	if sess == nil {
		return false
	}
	sess.conn.Close()
	return true
}

// CloseSockets closes every console session, for shutdown.
func (s *Service) CloseSockets() {
	s.mu.Lock()
	all := s.sockets
	s.sockets = map[string]*socketSession{}
	s.mu.Unlock()
	for _, sess := range all {
		sess.conn.Close()
	}
}
