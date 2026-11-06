package runner

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"api-test-kits/internal/assertions"
	"api-test-kits/internal/domain"
	"api-test-kits/internal/httpclient"
	"api-test-kits/internal/redact"
	"api-test-kits/internal/socket"
)

// maxFrameData bounds how much of one message a transcript keeps.
const maxFrameData = 64 << 10

// OpenSocket connects a prepared WebSocket or Socket.IO request.
func OpenSocket(ctx context.Context, d socket.Dialer, req domain.Request, p *Prepared) (socket.Conn, error) {
	if p.Socket == nil {
		return nil, errors.New("not a socket request")
	}
	return socket.Open(ctx, d, socket.Options{
		Kind:         req.Kind,
		URL:          p.HTTP.URL,
		Header:       p.HTTP.Header,
		Subprotocols: p.Socket.Subprotocols,
		Path:         p.Socket.Path,
		AuthPayload:  p.Socket.AuthPayload,
		Timeout:      p.HTTP.Timeout,
	})
}

// Outgoing converts a (resolved) message for sending.
func Outgoing(m domain.SocketMessage) socket.Outgoing {
	return socket.Outgoing{Event: m.Event, Body: m.Body, JSON: m.BodyType == domain.SocketBodyJSON, Ack: m.Ack}
}

// Frame turns a received message into a redacted transcript frame.
func Frame(m socket.Message, red *redact.Redactor, since time.Time) domain.SocketFrame {
	f := domain.SocketFrame{Direction: domain.FrameReceived, Event: m.Event, Binary: m.Binary, Ack: m.Ack,
		AtMs: m.At.Sub(since).Milliseconds()}
	if m.Binary || !utf8.Valid(m.Data) {
		f.Binary = true
		f.Data = base64.StdEncoding.EncodeToString(m.Data)
	} else {
		f.Data = red.String(string(m.Data))
	}
	if len(f.Data) > maxFrameData {
		f.Data, f.Truncated = f.Data[:maxFrameData], true
	}
	return f
}

// SentFrame is the transcript frame of a sent message.
func SentFrame(m domain.SocketMessage, red *redact.Redactor, at time.Duration) domain.SocketFrame {
	f := domain.SocketFrame{Direction: domain.FrameSent, Event: red.String(m.Event), Data: red.String(m.Body), AtMs: at.Milliseconds()}
	if len(f.Data) > maxFrameData {
		f.Data, f.Truncated = f.Data[:maxFrameData], true
	}
	return f
}

// received is a message waiting to be matched by an expect step.
type received struct {
	msg     socket.Message
	matched bool
}

// socketRun holds the state of one socket test.
type socketRun struct {
	conn    socket.Conn
	red     *redact.Redactor
	start   time.Time
	tr      *domain.SocketTranscript
	mu      sync.Mutex
	inbox   []*received
	arrived chan struct{} // signalled (non-blocking) on every new message
	// collected is closed once every received message is in the inbox.
	collected chan struct{}
}

func (s *socketRun) addFrame(f domain.SocketFrame) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.tr.Frames) >= domain.MaxTranscriptFrames {
		s.tr.Dropped++
		return
	}
	s.tr.Frames = append(s.tr.Frames, f)
}

func (s *socketRun) system(text string) {
	s.addFrame(domain.SocketFrame{Direction: domain.FrameSystem, Data: s.red.String(text), AtMs: time.Since(s.start).Milliseconds()})
}

// collect moves received messages into the inbox and the transcript until the connection ends.
func (s *socketRun) collect() {
	for m := range s.conn.Messages() {
		s.addFrame(Frame(m, s.red, s.start))
		s.mu.Lock()
		s.inbox = append(s.inbox, &received{msg: m})
		s.mu.Unlock()
		select {
		case s.arrived <- struct{}{}:
		default:
		}
	}
}

// match returns the first unmatched message that satisfies the step, marking
// it matched, plus the number of messages it looked at and the reason the
// closest candidate failed.
func (s *socketRun) match(step domain.SocketStep) (*received, int, string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	looked, closest := 0, ""
	for _, r := range s.inbox {
		if r.matched || r.msg.Ack != step.ExpectAck || (step.Event != "" && r.msg.Event != step.Event) {
			continue
		}
		looked++
		failed := ""
		for _, res := range assertions.Evaluate(assertions.Response{Body: r.msg.Data}, step.Assertions) {
			if !res.Passed {
				failed = res.Message
				break
			}
		}
		if failed == "" {
			r.matched = true
			return r, looked, ""
		}
		closest = failed
	}
	return nil, looked, closest
}

// runSocket connects, runs the enabled test steps in order and fills result.
func (r *Runner) runSocket(ctx context.Context, req domain.Request, p *Prepared, result *domain.RunResult) {
	red := p.Redactor
	d, ok := r.client.(socket.Dialer)
	if !ok {
		result.Status = domain.StatusError
		result.Error = "this runner cannot open socket connections"
		return
	}
	// The request timeout bounds the whole test, not each step.
	timeout := p.HTTP.Timeout
	tctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	start := r.now()
	conn, err := OpenSocket(tctx, d, req, p)
	if err != nil {
		result.DurationMs = r.now().Sub(start).Milliseconds()
		result.Status = domain.StatusError
		var he *httpclient.Error
		if ctx.Err() != nil || errors.As(err, &he) && he.Kind == httpclient.ErrCancelled {
			result.Status = domain.StatusCancelled
		}
		result.Error = red.String(err.Error())
		return
	}
	info := conn.Info()
	tr := &domain.SocketTranscript{Kind: req.Kind, Subprotocol: info.Subprotocol, SessionID: info.SessionID, Frames: []domain.SocketFrame{}}
	result.Socket = tr
	if h := info.Handshake; h != nil {
		result.Response = &domain.ResponseSummary{StatusCode: h.StatusCode, Status: h.Status, Headers: red.HTTPHeader(h.Header)}
	}
	run := &socketRun{conn: conn, red: red, start: time.Now(), tr: tr, arrived: make(chan struct{}, 1),
		collected: make(chan struct{})}
	run.system("connected")
	go func() { run.collect(); close(run.collected) }()

	results := []domain.AssertionResult{}
	stopped := "" // set when the remaining steps cannot run
	for i, step := range p.Socket.Steps {
		if !step.Enabled {
			continue
		}
		res := domain.AssertionResult{AssertionID: step.ID, Kind: domain.AssertSocketStep, Description: red.String(stepLabel(i, step))}
		switch {
		case stopped != "":
			res.Message = "not run: " + stopped
		case step.Kind == domain.StepSend:
			if err := conn.Send(Outgoing(step.Message)); err != nil {
				res.Message = red.String("could not send: " + err.Error())
				if errors.Is(err, socket.ErrClosed) {
					stopped = "the connection closed"
				}
			} else {
				res.Passed = true
				run.addFrame(SentFrame(step.Message, red, time.Since(run.start)))
			}
		case step.Kind == domain.StepWait:
			select {
			case <-time.After(time.Duration(step.TimeoutMs) * time.Millisecond):
				res.Passed = true
			case <-tctx.Done():
				res.Message = "interrupted"
			}
		case step.Kind == domain.StepExpect:
			run.expect(tctx, step, &res)
		}
		if !res.Passed && stopped == "" {
			switch {
			case ctx.Err() != nil:
				stopped = "the run was cancelled"
			case tctx.Err() != nil:
				stopped = fmt.Sprintf("the request timeout (%d ms) was reached", timeout.Milliseconds())
			}
		}
		results = append(results, res)
	}

	conn.Close()
	<-run.collected
	code, reason, cerr := conn.Result()
	tr.CloseCode, tr.CloseReason = code, red.String(reason)
	switch {
	case cerr != nil:
		run.system("closed: " + cerr.Error())
	case code != 0:
		run.system(fmt.Sprintf("closed (%d %s)", code, reason))
	default:
		run.system("closed")
	}
	result.AssertionResults = results
	result.DurationMs = r.now().Sub(start).Milliseconds()
	result.Status = domain.StatusPassed
	for _, a := range results {
		if !a.Passed {
			result.Status = domain.StatusFailed
		}
	}
	if ctx.Err() != nil {
		result.Status = domain.StatusCancelled
		result.Error = "the run was cancelled"
	}
}

// expect waits for a matching message within the step's timeout.
func (s *socketRun) expect(ctx context.Context, step domain.SocketStep, res *domain.AssertionResult) {
	res.Expected = s.red.String(expectSummary(step))
	timer := time.NewTimer(time.Duration(step.TimeoutMs) * time.Millisecond)
	defer timer.Stop()
	for {
		m, looked, closest := s.match(step)
		if m != nil {
			res.Passed = true
			res.Actual = s.red.String(clipText(string(m.msg.Data), 300))
			return
		}
		wait := func(reason string) {
			res.Message = s.red.String(fmt.Sprintf("%s: %d candidate message(s), none matched", reason, looked))
			if closest != "" {
				res.Message += s.red.String(" — closest: " + closest)
			}
		}
		select {
		case <-s.arrived:
		case <-s.collected:
			// Every message is in the inbox now; look once more.
			if m, _, _ := s.match(step); m != nil {
				res.Passed = true
				res.Actual = s.red.String(clipText(string(m.msg.Data), 300))
				return
			}
			wait("the connection closed")
			return
		case <-timer.C:
			wait(fmt.Sprintf("nothing matched within %d ms", step.TimeoutMs))
			return
		case <-ctx.Done():
			wait("interrupted")
			return
		}
	}
}

func stepLabel(i int, s domain.SocketStep) string {
	if strings.TrimSpace(s.Description) != "" {
		return s.Description
	}
	n := fmt.Sprintf("Step %d: ", i+1)
	switch s.Kind {
	case domain.StepSend:
		if s.Message.Event != "" {
			return n + "send " + s.Message.Event
		}
		return n + "send " + clipText(s.Message.Body, 40)
	case domain.StepWait:
		return fmt.Sprintf("%swait %d ms", n, s.TimeoutMs)
	}
	return n + "expect " + expectSummary(s)
}

func expectSummary(s domain.SocketStep) string {
	var parts []string
	what := "a message"
	if s.ExpectAck {
		what = "an acknowledgement"
	}
	if s.Event != "" {
		what += " " + s.Event
	}
	parts = append(parts, what)
	for _, a := range s.Assertions {
		if !a.Enabled {
			continue
		}
		c := string(a.Kind)
		if a.Path != "" {
			c = a.Path
		}
		if a.Operator != "" {
			c += " " + string(a.Operator)
		}
		if len(a.Expected) > 0 {
			c += " " + string(a.Expected)
		}
		parts = append(parts, c)
	}
	return strings.Join(parts, ", ")
}

func clipText(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
