package dashboard

import (
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"github.com/grasberg/sofia/pkg/logger"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		origin := r.Header.Get("Origin")
		if origin == "" {
			return true // Non-browser clients (curl, etc.) may not send Origin.
		}
		host := r.Host // Host header includes hostname and optional port.
		// Accept if the origin matches the request host.
		// Typical origins: "http://localhost:8080", "http://127.0.0.1:8080".
		allowed := []string{
			"http://" + host,
			"https://" + host,
		}
		// When the Host is "0.0.0.0:<port>", also allow localhost variants.
		if strings.HasPrefix(host, "0.0.0.0") {
			port := ""
			if idx := strings.LastIndex(host, ":"); idx >= 0 {
				port = host[idx:]
			}
			allowed = append(allowed,
				"http://localhost"+port,
				"http://127.0.0.1"+port,
				"https://localhost"+port,
				"https://127.0.0.1"+port,
			)
		}
		for _, a := range allowed {
			if origin == a {
				return true
			}
		}
		return false
	},
}

// writeTimeout is applied to every WebSocket write to prevent
// blocking forever on a stuck or slow client.
const writeTimeout = 5 * time.Second

// sendBufSize is the capacity of each client's outbound channel.
// If a client falls this far behind, new messages are dropped.
const sendBufSize = 256

// clientInfo holds per-client state: the underlying WebSocket
// connection and a buffered channel for outbound messages.
type clientInfo struct {
	conn *websocket.Conn
	send chan []byte
}

// PresenceInfo holds the current presence state of the agent loop.
type PresenceInfo struct {
	AgentID string `json:"agent_id"`
	Status  string `json:"status"`
	Since   int64  `json:"since"`
}

// Hub manages a set of WebSocket clients and broadcasts messages
// to them without blocking the caller.
type Hub struct {
	clients       map[*websocket.Conn]*clientInfo
	mu            sync.Mutex
	presenceMu    sync.RWMutex
	presenceState map[string]PresenceInfo
}

func NewHub() *Hub {
	return &Hub{
		clients:       make(map[*websocket.Conn]*clientInfo),
		presenceState: make(map[string]PresenceInfo),
	}
}

// UpdatePresence updates the internal presence state. When status is
// "idle", the entry is stored under the empty-string key so callers
// can always find the current state.
func (h *Hub) UpdatePresence(agentID, status string) {
	h.presenceMu.Lock()
	defer h.presenceMu.Unlock()
	h.presenceState["current"] = PresenceInfo{
		AgentID: agentID,
		Status:  status,
		Since:   time.Now().Unix(),
	}
}

// GetPresence returns a copy of the current presence state map.
func (h *Hub) GetPresence() map[string]PresenceInfo {
	h.presenceMu.RLock()
	defer h.presenceMu.RUnlock()
	result := make(map[string]PresenceInfo, len(h.presenceState))
	for k, v := range h.presenceState {
		result[k] = v
	}
	return result
}

// Broadcast marshals msg once and enqueues the payload into every
// client's send channel. Clients whose channels are full are
// skipped (the message is dropped for that client) so that a slow
// consumer never blocks the agent loop.
func (h *Hub) Broadcast(msg any) {
	data, err := json.Marshal(map[string]any{
		"type": "update",
		"data": msg,
	})
	if err != nil {
		return
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	for _, ci := range h.clients {
		select {
		case ci.send <- data:
		default:
			logger.ErrorCF("web",
				"Dashboard client send buffer full, dropping message",
				map[string]any{
					"remote": ci.conn.RemoteAddr().String(),
				})
		}
	}
}

// RegisterClient upgrades the HTTP connection to a WebSocket,
// sends an initial snapshot, starts a write goroutine, and then
// blocks on a read loop to detect client disconnect.
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
		// Inject current presence state into the snapshot.
		if m, ok := snapshot.(map[string]any); ok {
			m["presence"] = h.GetPresence()
		}
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

	ci := &clientInfo{
		conn: conn,
		send: make(chan []byte, sendBufSize),
	}

	h.mu.Lock()
	h.clients[conn] = ci
	h.mu.Unlock()

	// Write goroutine: drains the send channel and writes to
	// the WebSocket. Exits when the channel is closed or on a
	// write error.
	go h.writePump(ci)

	// Keep connection open — read loop detects disconnect.
	for {
		_, _, err := conn.ReadMessage()
		if err != nil {
			break
		}
	}
}

// writePump reads from the client's send channel and writes each
// payload to the WebSocket with a deadline. On any write error it
// closes the connection, which causes the read loop in
// RegisterClient to exit and trigger cleanup.
func (h *Hub) writePump(ci *clientInfo) {
	for data := range ci.send {
		_ = ci.conn.SetWriteDeadline( //nolint:errcheck
			time.Now().Add(writeTimeout),
		)
		if err := ci.conn.WriteMessage(
			websocket.TextMessage, data,
		); err != nil {
			_ = ci.conn.Close() //nolint:errcheck
			return
		}
	}
}

// UnregisterClient removes the client from the hub and closes its
// send channel so the write goroutine exits.
func (h *Hub) UnregisterClient(conn *websocket.Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if ci, ok := h.clients[conn]; ok {
		close(ci.send)
		delete(h.clients, conn)
	}
}
