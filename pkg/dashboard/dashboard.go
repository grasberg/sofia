package dashboard

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

// writeTimeout is applied to every WebSocket write to prevent
// blocking forever on a stuck or slow client.
const writeTimeout = 5 * time.Second

type Hub struct {
	clients map[*websocket.Conn]bool
	mu      sync.Mutex
}

func NewHub() *Hub {
	return &Hub{
		clients: make(map[*websocket.Conn]bool),
	}
}

func (h *Hub) Broadcast(msg any) {
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
			time.Now().Add(writeTimeout),
		)
		if err := client.WriteMessage(
			websocket.TextMessage, data,
		); err != nil {
			// Client is dead or too slow — remove it.
			_ = client.Close() //nolint:errcheck
			delete(h.clients, client)
		}
	}
}

func (h *Hub) RegisterClient(
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
		h.UnregisterClient(conn)
		_ = conn.Close() //nolint:errcheck
	}()

	// Send initial snapshot before registering so it arrives
	// before any broadcast messages.
	if getSnapshot != nil {
		snapshot := getSnapshot()
		snapshotData, _ := json.Marshal(map[string]any{ //nolint:errcheck
			"type": "snapshot",
			"data": snapshot,
		})
		_ = conn.SetWriteDeadline( //nolint:errcheck
			time.Now().Add(writeTimeout),
		)
		if err := conn.WriteMessage(
			websocket.TextMessage, snapshotData,
		); err != nil {
			return // Client already gone.
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

func (h *Hub) UnregisterClient(conn *websocket.Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.clients, conn)
}
