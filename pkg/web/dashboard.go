package web

import (
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"github.com/grasberg/sofia/pkg/logger"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

// wsWriteTimeout is applied to every WebSocket write.
const wsWriteTimeout = 5 * time.Second

// wsSendBuffer is the capacity of each client's send channel.
const wsSendBuffer = 256

// wsClient wraps a WebSocket connection with a buffered send channel.
type wsClient struct {
	conn *websocket.Conn
	send chan []byte
}

// DashboardHub manages connected dashboard WebSocket clients.
type DashboardHub struct {
	clients map[*websocket.Conn]*wsClient
	mu      sync.Mutex
}

// NewDashboardHub creates a new hub with no connected clients.
func NewDashboardHub() *DashboardHub {
	return &DashboardHub{
		clients: make(map[*websocket.Conn]*wsClient),
	}
}

// writePump drains the client's send channel and writes messages to the
// WebSocket connection. It exits on send channel close or write error.
func (h *DashboardHub) writePump(c *wsClient) {
	defer func() {
		h.mu.Lock()
		delete(h.clients, c.conn)
		h.mu.Unlock()
		_ = c.conn.Close() //nolint:errcheck
	}()

	for msg := range c.send {
		_ = c.conn.SetWriteDeadline( //nolint:errcheck
			time.Now().Add(wsWriteTimeout),
		)
		if err := c.conn.WriteMessage(
			websocket.TextMessage, msg,
		); err != nil {
			return
		}
	}
}

// HandleDashboardWS upgrades the connection and streams
// dashboard events to the client.
func (h *DashboardHub) HandleDashboardWS(
	w http.ResponseWriter,
	r *http.Request,
	getSnapshot func() any,
) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		logger.ErrorCF("web",
			"Failed to upgrade dashboard WS",
			map[string]any{"error": err.Error()})
		return
	}

	// Send initial snapshot before registering.
	if getSnapshot != nil {
		snapshot := getSnapshot()
		snapshotData, _ := json.Marshal(map[string]any{ //nolint:errcheck
			"type": "snapshot",
			"data": snapshot,
		})
		_ = conn.SetWriteDeadline( //nolint:errcheck
			time.Now().Add(wsWriteTimeout),
		)
		if err := conn.WriteMessage(
			websocket.TextMessage, snapshotData,
		); err != nil {
			_ = conn.Close() //nolint:errcheck
			return
		}
	}

	c := &wsClient{
		conn: conn,
		send: make(chan []byte, wsSendBuffer),
	}

	h.mu.Lock()
	h.clients[conn] = c
	h.mu.Unlock()

	go h.writePump(c)

	// Keep connection open — read loop detects disconnect.
	for {
		_, _, err := conn.ReadMessage()
		if err != nil {
			break
		}
	}

	// Client disconnected: close send channel so writePump exits.
	h.mu.Lock()
	if _, ok := h.clients[conn]; ok {
		close(c.send)
		delete(h.clients, conn)
	}
	h.mu.Unlock()
}

// Broadcast sends a message to all connected dashboard clients.
// Sends are non-blocking; slow clients that cannot keep up are dropped.
func (h *DashboardHub) Broadcast(msg any) {
	data, err := json.Marshal(map[string]any{
		"type": "update",
		"data": msg,
	})
	if err != nil {
		return
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	for _, c := range h.clients {
		select {
		case c.send <- data:
		default:
			// Client too slow — drop it.
			close(c.send)
			delete(h.clients, c.conn)
		}
	}
}
