package socket

import (
	"time"

	"github.com/gorilla/websocket"

	"api-test-kits/internal/httpclient"
)

// wsConn is a plain WebSocket connection: every text or binary frame is a message.
type wsConn struct{ *base }

func newWSConn(sc *httpclient.SocketConn) *wsConn {
	c := &wsConn{newBase(sc.Conn, Info{Subprotocol: sc.Conn.Subprotocol(), Handshake: sc.Handshake})}
	go c.read()
	return c
}

func (c *wsConn) read() {
	defer c.end()
	for {
		typ, data, err := c.conn.ReadMessage()
		if err != nil {
			c.fail(err)
			return
		}
		if !c.deliver(Message{Data: data, Binary: typ == websocket.BinaryMessage, At: time.Now()}) {
			return
		}
	}
}

func (c *wsConn) Send(o Outgoing) error {
	if o.JSON {
		if _, err := checkJSON(o.Body); err != nil {
			return err
		}
	}
	return c.write([]byte(o.Body))
}

func (c *wsConn) Close() { c.closeWith(nil) }
