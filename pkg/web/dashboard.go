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

type DashboardHub struct {
	clients map[*websocket.Conn]bool
	mu      sync.Mutex
}

func NewDashboardHub() *DashboardHub {
	return &DashboardHub{
		clients: make(map[*websocket.Conn]bool),
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
	defer func() {
		h.mu.Lock()
		delete(h.clients, conn)
		h.mu.Unlock()
		_ = conn.Close() //nolint:errcheck
	}()

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
			return
		}
	}

	h.mu.Lock()
	h.clients[conn] = true
	h.mu.Unlock()

	// Keep connection open — read loop detects disconnect.
	for {
		_, _, err := conn.ReadMessage()
		if err != nil {
			break
		}
	}
}

func (h *DashboardHub) Broadcast(msg any) {
	h.mu.Lock()
	defer h.mu.Unlock()

	data, err := json.Marshal(map[string]any{
		"type": "update",
		"data": msg,
	})
	if err != nil {
		return
	}

	for client := range h.clients {
		_ = client.SetWriteDeadline( //nolint:errcheck
			time.Now().Add(wsWriteTimeout),
		)
		if err := client.WriteMessage(
			websocket.TextMessage, data,
		); err != nil {
			_ = client.Close() //nolint:errcheck
			delete(h.clients, client)
		}
	}
}
