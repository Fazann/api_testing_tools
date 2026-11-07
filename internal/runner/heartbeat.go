package runner

import (
	"sync"
	"time"

	"api-test-kits/internal/domain"
	"api-test-kits/internal/socket"
)

// Heartbeat sends a message on a fixed interval until it is stopped or a
// send fails, like a chat client keeping its presence alive. It is used by
// the console and by heartbeat test steps.
type Heartbeat struct {
	interval time.Duration
	stop     chan struct{}
	done     chan struct{}
	// first is closed once the first message was sent or failed.
	first chan struct{}
	once  sync.Once

	mu   sync.Mutex
	sent int
	err  error
}

// StartHeartbeat sends next() at once and then every interval on conn. next
// builds each message (so built-in variables such as {{$timestamp}} change);
// onSent, if set, is called after every successful send with the message and
// the count so far. The heartbeat stops on the first error.
func StartHeartbeat(conn socket.Conn, interval time.Duration, next func() (domain.SocketMessage, error),
	onSent func(m domain.SocketMessage, sent int)) *Heartbeat {
	h := &Heartbeat{interval: interval, stop: make(chan struct{}), done: make(chan struct{}), first: make(chan struct{})}
	go h.loop(conn, next, onSent)
	return h
}

func (h *Heartbeat) loop(conn socket.Conn, next func() (domain.SocketMessage, error), onSent func(domain.SocketMessage, int)) {
	defer close(h.done)
	ticker := time.NewTicker(h.interval)
	defer ticker.Stop()
	signalFirst := sync.OnceFunc(func() { close(h.first) })
	defer signalFirst()
	for {
		m, err := next()
		if err == nil {
			err = conn.Send(Outgoing(m))
		}
		if err != nil {
			h.mu.Lock()
			h.err = err
			h.mu.Unlock()
			return
		}
		h.mu.Lock()
		h.sent++
		n := h.sent
		h.mu.Unlock()
		if onSent != nil {
			onSent(m, n)
		}
		signalFirst()
		select {
		case <-ticker.C:
		case <-h.stop:
			return
		}
	}
}

// Stop stops the heartbeat, waits for a send in progress, and returns how
// many messages were sent and the error that stopped it early, if any.
func (h *Heartbeat) Stop() (int, error) {
	h.once.Do(func() { close(h.stop) })
	<-h.done
	return h.Status()
}

// First waits until the first message was sent and returns the error if it failed.
func (h *Heartbeat) First() error {
	<-h.first
	_, err := h.Status()
	return err
}

// Done is closed when the heartbeat has stopped, by Stop or by an error.
func (h *Heartbeat) Done() <-chan struct{} { return h.done }

// Status returns the messages sent so far and the error that stopped it, if any.
func (h *Heartbeat) Status() (int, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.sent, h.err
}

// Interval returns the time between messages.
func (h *Heartbeat) Interval() time.Duration { return h.interval }
