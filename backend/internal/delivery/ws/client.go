package ws

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/gofiber/contrib/websocket"
	"github.com/google/uuid"

	"github.com/loadify/backend/pkg/logger"
)

const (
	// writeWait bounds how long a single write may block.
	writeWait = 10 * time.Second
	// pongWait is how long we tolerate silence from a client before closing.
	// Mobile networks drop connections without a FIN, so an idle read timeout is
	// the only reliable way to notice.
	pongWait = 60 * time.Second
	// pingPeriod must be shorter than pongWait so a healthy client always
	// refreshes its deadline in time.
	pingPeriod = 25 * time.Second
	// maxMessageSize caps inbound frames; chat text and location points are tiny,
	// and anything larger is either a bug or an attack.
	maxMessageSize = 8 << 10
)

// Client is one live WebSocket connection.
type Client struct {
	hub    *Hub
	conn   *websocket.Conn
	userID uuid.UUID
	// companyID and driverID come from the verified token, never from the client.
	companyID *uuid.UUID
	driverID  *uuid.UUID
	roles     []string

	send_ chan []byte
	log   logger.Logger

	closeOnce sync.Once
}

// InboundMessage is what a client may send us.
type InboundMessage struct {
	Type    string         `json:"type"`
	Payload map[string]any `json:"payload"`
}

// send queues an outbound frame. A client that cannot keep up is disconnected
// rather than allowed to back-pressure the hub.
func (c *Client) send(data []byte) {
	select {
	case c.send_ <- data:
	default:
		c.log.Warn("dropping slow realtime client",
			logger.String("user_id", c.userID.String()))
		c.close()
	}
}

// close shuts the connection down exactly once.
func (c *Client) close() {
	c.closeOnce.Do(func() {
		close(c.send_)
		_ = c.conn.Close()
	})
}

// writePump drains the send queue and keeps the connection alive with pings.
func (c *Client) writePump() {
	ticker := time.NewTicker(pingPeriod)
	defer ticker.Stop()

	for {
		select {
		case data, ok := <-c.send_:
			if !ok {
				_ = c.conn.WriteControl(websocket.CloseMessage,
					websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""),
					time.Now().Add(writeWait))
				return
			}
			_ = c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.TextMessage, data); err != nil {
				return
			}
		case <-ticker.C:
			_ = c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// readPump processes inbound frames until the connection closes.
func (c *Client) readPump(ctx context.Context, router *Router) {
	defer func() {
		c.hub.unregister(c)
		c.close()
	}()

	c.conn.SetReadLimit(maxMessageSize)
	_ = c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error {
		return c.conn.SetReadDeadline(time.Now().Add(pongWait))
	})

	for {
		_, data, err := c.conn.ReadMessage()
		if err != nil {
			return
		}
		// Any traffic proves the client is alive, not only pongs — a chatty
		// client should never be timed out.
		_ = c.conn.SetReadDeadline(time.Now().Add(pongWait))

		var msg InboundMessage
		if err := json.Unmarshal(data, &msg); err != nil {
			c.sendError("BAD_MESSAGE", "Message is not valid JSON")
			continue
		}
		router.handle(ctx, c, msg)
	}
}

// sendEvent pushes a typed event to just this connection.
func (c *Client) sendEvent(eventType string, payload map[string]any) {
	data, err := json.Marshal(Envelope{
		Type:      eventType,
		Payload:   payload,
		Timestamp: time.Now().UTC(),
	})
	if err != nil {
		return
	}
	c.send(data)
}

// sendError reports a problem with something the client sent.
func (c *Client) sendError(code, message string) {
	c.sendEvent("error", map[string]any{"code": code, "message": message})
}
