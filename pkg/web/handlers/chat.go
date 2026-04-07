package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/grasberg/sofia/pkg/web"
)

// ChatHandler handles chat-related endpoints.
type ChatHandler struct {
	Server *web.Server
}

// HandleChat processes a chat request.
func (h *ChatHandler) HandleChat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Message   string `json:"message"`
		AgentID   string `json:"agent_id"`
		SessionID string `json:"session_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if req.Message == "" {
		http.Error(w, "message is required", http.StatusBadRequest)
		return
	}

	// Process chat message through the server's agent loop
	response, err := h.Server.ProcessChatMessage(r.Context(), req.AgentID, req.SessionID, req.Message)
	if err != nil {
		http.Error(w, fmt.Sprintf("Chat error: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"response": response,
	})
}

// HandleChatStream handles streaming chat responses.
func (h *ChatHandler) HandleChatStream(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Set headers for SSE streaming
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	var req struct {
		Message   string `json:"message"`
		AgentID   string `json:"agent_id"`
		SessionID string `json:"session_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		fmt.Fprintf(w, "event: error\ndata: %s\n\n", err.Error())
		return
	}

	// Stream the response
	err := h.Server.StreamChatResponse(r.Context(), w, req.AgentID, req.SessionID, req.Message)
	if err != nil {
		fmt.Fprintf(w, "event: error\ndata: %s\n\n", err.Error())
	}
}
