package socket

import (
	"errors"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// closeWait is how long Close waits for the server to answer a close frame.
const closeWait = time.Second

// base holds what WebSocket and Socket.IO connections share: the underlying
// connection, the message channel, serialized writes and how the connection ended.
type base struct {
	conn *websocket.Conn
	info Info
	msgs chan Message
	done chan struct{}
	// stop is closed by Close so the reader never blocks on a full channel.
	stop     chan struct{}
	stopOnce sync.Once

	writeMu sync.Mutex

	mu     sync.Mutex
	code   int
	reason string
	err    error
	closed bool
}

func newBase(conn *websocket.Conn, info Info) *base {
	return &base{conn: conn, info: info, msgs: make(chan Message, 256), done: make(chan struct{}), stop: make(chan struct{})}
}

func (b *base) Info() Info               { return b.info }
func (b *base) Messages() <-chan Message { return b.msgs }
func (b *base) Done() <-chan struct{}    { return b.done }

func (b *base) Result() (int, string, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.code, b.reason, b.err
}

// deliver hands a message to the reader of Messages; false means Close was called.
func (b *base) deliver(m Message) bool {
	select {
	case b.msgs <- m:
		return true
	case <-b.stop:
		return false
	}
}

// end is deferred by the read loop: it closes the channels and the connection.
func (b *base) end() {
	b.conn.Close()
	close(b.msgs)
	close(b.done)
}

// fail records why the connection ended, unless a reason is already known.
func (b *base) fail(err error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.err != nil || b.code != 0 {
		return
	}
	var ce *websocket.CloseError
	if errors.As(err, &ce) {
		normal := ce.Code == websocket.CloseNormalClosure || ce.Code == websocket.CloseGoingAway
		// Once we have started closing, however the server ends the
		// connection (Socket.IO servers often just drop it) is normal.
		if b.closed && !normal {
			return
		}
		b.code, b.reason = ce.Code, ce.Text
		if !normal {
			b.err = err
		}
		return
	}
	if !b.closed {
		b.err = err
	}
}

func (b *base) write(data []byte) error {
	select {
	case <-b.done:
		return ErrClosed
	default:
	}
	b.writeMu.Lock()
	defer b.writeMu.Unlock()
	_ = b.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
	if err := b.conn.WriteMessage(websocket.TextMessage, data); err != nil {
		return err
	}
	return nil
}

// closeWith sends goodbye (the Socket.IO disconnect packet, if any) and a
// close frame, then waits briefly for the server to close its side.
func (b *base) closeWith(goodbye []byte) {
	b.mu.Lock()
	already := b.closed
	b.closed = true
	b.mu.Unlock()
	b.stopOnce.Do(func() { close(b.stop) })
	if already {
		<-b.done
		return
	}
	b.writeMu.Lock()
	deadline := time.Now().Add(closeWait)
	if goodbye != nil {
		_ = b.conn.SetWriteDeadline(deadline)
		_ = b.conn.WriteMessage(websocket.TextMessage, goodbye)
	}
	_ = b.conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""), deadline)
	b.writeMu.Unlock()
	select {
	case <-b.done:
	case <-time.After(closeWait):
		b.conn.Close()
		<-b.done
	}
}
