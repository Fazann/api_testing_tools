package domain

import (
	"fmt"
	"strings"
)

// RequestKind says which protocol a request speaks.
type RequestKind string

const (
	KindHTTP      RequestKind = "http"
	KindWebSocket RequestKind = "websocket"
	// KindSocketIO is Socket.IO v3/v4 (Engine.IO protocol 4) over the WebSocket transport.
	KindSocketIO RequestKind = "socketio"
)

// IsSocket reports whether the kind keeps a connection open (WebSocket or Socket.IO).
func (k RequestKind) IsSocket() bool { return k == KindWebSocket || k == KindSocketIO }

// Badge is the short label shown instead of an HTTP method.
func (k RequestKind) Badge(method string) string {
	switch k {
	case KindWebSocket:
		return "WS"
	case KindSocketIO:
		return "SIO"
	}
	return method
}

const (
	DefaultSocketIOPath   = "/socket.io/"
	DefaultExpectTimeout  = 5_000
	MaxSocketSteps        = 100
	MaxSocketMessages     = 100
	MaxSocketWaitMs       = 60_000
	MaxSocketEventLength  = 200
	MaxSocketSubprotocols = 10
)

// SocketBodyType says how a message body is sent: JSON is parsed and sent as a
// JSON value (Socket.IO) or as text (WebSocket); text is sent as a string.
type SocketBodyType string

const (
	SocketBodyJSON SocketBodyType = "json"
	SocketBodyText SocketBodyType = "text"
)

// SocketMessage is a message the user can send: saved for the console, or the
// payload of a send step. Body may contain {{variables}}.
type SocketMessage struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	// Event is the Socket.IO event name; WebSocket messages ignore it.
	Event    string         `json:"event"`
	Body     string         `json:"body"`
	BodyType SocketBodyType `json:"bodyType"`
	// Ack asks the Socket.IO server to acknowledge the event; the
	// acknowledgement arrives as a received message with Ack set.
	Ack bool `json:"ack,omitempty"`
}

// SocketStepKind is the action of one test step.
type SocketStepKind string

const (
	// StepSend sends Message.
	StepSend SocketStepKind = "send"
	// StepExpect waits up to TimeoutMs for a received message that has Event
	// (when set) and passes every assertion. Each received message can satisfy
	// at most one expect step; messages that match nothing are kept for later steps.
	StepExpect SocketStepKind = "expect"
	// StepWait pauses for TimeoutMs.
	StepWait SocketStepKind = "wait"
)

// SocketStep is one step of a socket request's test.
type SocketStep struct {
	ID          string         `json:"id"`
	Kind        SocketStepKind `json:"kind"`
	Description string         `json:"description,omitempty"`
	Enabled     bool           `json:"enabled"`
	// Message is sent by StepSend.
	Message SocketMessage `json:"message"`
	// Event filters StepExpect to one Socket.IO event ("" matches any message).
	Event string `json:"event,omitempty"`
	// ExpectAck makes StepExpect wait for the server's acknowledgement of a
	// sent event instead of an event; other expect steps never match acknowledgements.
	ExpectAck bool `json:"expectAck,omitempty"`
	// Assertions are evaluated against a received message's body (json_path,
	// body, valid_json, required_fields, aggregate).
	Assertions []Assertion `json:"assertions"`
	// TimeoutMs is how long StepExpect waits, or how long StepWait pauses.
	TimeoutMs int `json:"timeoutMs"`
}

// SocketConfig holds everything specific to WebSocket and Socket.IO requests.
type SocketConfig struct {
	// Subprotocols are offered in Sec-WebSocket-Protocol (WebSocket only).
	Subprotocols []string `json:"subprotocols"`
	// Path is the Socket.IO server path, "/socket.io/" by default. The URL's
	// own path is the namespace, as in the official client: io("https://host/admin").
	Path string `json:"path"`
	// AuthPayload is a JSON object sent with the Socket.IO CONNECT packet
	// (socket.handshake.auth on the server). It may contain {{variables}}.
	AuthPayload string `json:"authPayload"`
	// Messages are saved for sending from the console.
	Messages []SocketMessage `json:"messages"`
	// Steps are the request's test, run by Send, collection runs and the CLI.
	Steps []SocketStep `json:"steps"`
}

// Assertion kinds that make sense on a message body.
var socketAssertionKinds = map[AssertionKind]bool{
	AssertJSONPath: true, AssertBody: true, AssertValidJSON: true, AssertRequiredFields: true, AssertAggregate: true,
}

// IsSocketAssertionKind reports whether an assertion kind can check a socket message.
func IsSocketAssertionKind(k AssertionKind) bool { return socketAssertionKinds[k] }

func (c *SocketConfig) normalize(kind RequestKind) {
	if c.Subprotocols == nil {
		c.Subprotocols = []string{}
	}
	if c.Messages == nil {
		c.Messages = []SocketMessage{}
	}
	if c.Steps == nil {
		c.Steps = []SocketStep{}
	}
	c.Path = strings.TrimSpace(c.Path)
	if kind == KindSocketIO && c.Path == "" {
		c.Path = DefaultSocketIOPath
	}
	for i := range c.Messages {
		c.Messages[i].normalize()
	}
	for i := range c.Steps {
		s := &c.Steps[i]
		s.Message.normalize()
		if s.Assertions == nil {
			s.Assertions = []Assertion{}
		}
		if s.Kind == StepExpect && s.TimeoutMs <= 0 {
			s.TimeoutMs = DefaultExpectTimeout
		}
	}
}

func (m *SocketMessage) normalize() {
	if m.BodyType == "" {
		m.BodyType = SocketBodyText
	}
	m.Event = strings.TrimSpace(m.Event)
}

func (c *SocketConfig) validate(kind RequestKind) error {
	if len(c.Subprotocols) > MaxSocketSubprotocols {
		return fmt.Errorf("at most %d subprotocols are allowed", MaxSocketSubprotocols)
	}
	for _, p := range c.Subprotocols {
		if !isValidHeaderName(p) {
			return fmt.Errorf("subprotocol %q is invalid", p)
		}
	}
	if kind == KindSocketIO && !strings.HasPrefix(c.Path, "/") {
		return fmt.Errorf("Socket.IO path must start with /")
	}
	if len(c.AuthPayload) > MaxBodyLength {
		return fmt.Errorf("Socket.IO auth payload must be at most %d bytes", MaxBodyLength)
	}
	if len(c.Messages) > MaxSocketMessages {
		return fmt.Errorf("at most %d saved messages are allowed", MaxSocketMessages)
	}
	for i := range c.Messages {
		if err := c.Messages[i].validate(kind); err != nil {
			return fmt.Errorf("saved message %d: %w", i+1, err)
		}
	}
	if len(c.Steps) > MaxSocketSteps {
		return fmt.Errorf("at most %d test steps are allowed", MaxSocketSteps)
	}
	for i := range c.Steps {
		if err := c.Steps[i].validate(kind); err != nil {
			return fmt.Errorf("test step %d: %w", i+1, err)
		}
	}
	return nil
}

func (m *SocketMessage) validate(kind RequestKind) error {
	if len(m.Name) > MaxNameLength {
		return fmt.Errorf("message name must be at most %d characters", MaxNameLength)
	}
	if len(m.Body) > MaxBodyLength {
		return fmt.Errorf("message body must be at most %d bytes", MaxBodyLength)
	}
	if m.BodyType != SocketBodyJSON && m.BodyType != SocketBodyText {
		return fmt.Errorf("unsupported message body type %q", m.BodyType)
	}
	if len(m.Event) > MaxSocketEventLength {
		return fmt.Errorf("event name must be at most %d characters", MaxSocketEventLength)
	}
	if kind == KindSocketIO {
		if m.Event == "" {
			return fmt.Errorf("Socket.IO messages need an event name")
		}
		if isReservedSocketIOEvent(m.Event) {
			return fmt.Errorf("%q is a reserved Socket.IO event name", m.Event)
		}
	}
	return nil
}

func (s *SocketStep) validate(kind RequestKind) error {
	switch s.Kind {
	case StepSend:
		return s.Message.validate(kind)
	case StepExpect:
		if s.TimeoutMs < 1 || s.TimeoutMs > MaxSocketWaitMs {
			return fmt.Errorf("expect timeout must be between 1 and %d ms", MaxSocketWaitMs)
		}
		if len(s.Event) > MaxSocketEventLength {
			return fmt.Errorf("event name must be at most %d characters", MaxSocketEventLength)
		}
		for i := range s.Assertions {
			a := &s.Assertions[i]
			if !IsSocketAssertionKind(a.Kind) {
				return fmt.Errorf("check %d: %s checks do not apply to socket messages", i+1, a.Kind)
			}
			if err := a.Validate(); err != nil {
				return fmt.Errorf("check %d: %w", i+1, err)
			}
		}
	case StepWait:
		if s.TimeoutMs < 1 || s.TimeoutMs > MaxSocketWaitMs {
			return fmt.Errorf("wait must be between 1 and %d ms", MaxSocketWaitMs)
		}
	default:
		return fmt.Errorf("unknown step kind %q", s.Kind)
	}
	return nil
}

// Events the Socket.IO client and server use themselves.
func isReservedSocketIOEvent(e string) bool {
	switch e {
	case "connect", "connect_error", "disconnect", "disconnecting", "newListener", "removeListener":
		return true
	}
	return false
}

// SendsMessages reports whether running the request's test sends anything,
// which may change server state.
func (c *SocketConfig) SendsMessages() bool {
	if c == nil {
		return false
	}
	for _, s := range c.Steps {
		if s.Enabled && s.Kind == StepSend {
			return true
		}
	}
	return false
}

// Directions of a transcript frame.
const (
	FrameSent     = "sent"
	FrameReceived = "received"
	// FrameSystem is a connection event: connected, closed, errors.
	FrameSystem = "system"
)

// SocketFrame is one message in a transcript, already redacted.
type SocketFrame struct {
	Direction string `json:"direction"`
	// Event is the Socket.IO event name.
	Event string `json:"event,omitempty"`
	// Data is the message text; for Socket.IO it is the event's argument as
	// JSON, or a JSON array when the event has several. Binary WebSocket
	// frames are base64-encoded.
	Data   string `json:"data"`
	Binary bool   `json:"binary,omitempty"`
	// Ack marks a Socket.IO acknowledgement of a sent event.
	Ack bool `json:"ack,omitempty"`
	// AtMs is the time since the connection opened.
	AtMs int64 `json:"atMs"`
	// Truncated is set when Data was cut at the history size limit.
	Truncated bool `json:"truncated,omitempty"`
}

// MaxTranscriptFrames bounds how many frames a run result keeps.
const MaxTranscriptFrames = 1_000

// SocketTranscript is what happened on a socket connection during a test run.
type SocketTranscript struct {
	Kind RequestKind `json:"kind"`
	// Subprotocol is the one the server chose, if any.
	Subprotocol string `json:"subprotocol,omitempty"`
	// SessionID is the Socket.IO session ID.
	SessionID string        `json:"sessionId,omitempty"`
	Frames    []SocketFrame `json:"frames"`
	// Dropped counts frames left out after MaxTranscriptFrames.
	Dropped     int    `json:"dropped,omitempty"`
	CloseCode   int    `json:"closeCode,omitempty"`
	CloseReason string `json:"closeReason,omitempty"`
}
