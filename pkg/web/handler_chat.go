package web

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/grasberg/sofia/pkg/logger"
)

func (s *Server) handleChat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.sendJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	limitBody(r)
	var req struct {
		Message    string `json:"message"`
		SessionKey string `json:"session_key"`
		Files      []struct {
			Name    string `json:"name"`
			Type    string `json:"type"`
			IsText  bool   `json:"isText"`
			Content string `json:"content"` // plain text if isText, base64 data URL if image/binary
		} `json:"files"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.sendJSONError(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Determine session key: use provided key, or generate a new timestamped one.
	sessionKey := req.SessionKey
	if sessionKey == "" {
		sessionKey = "web:ui:" + time.Now().UTC().Format(time.RFC3339)
	}

	// Separate images (for native vision) from other files (inline text context)
	fullMessage := req.Message
	var imageDataURLs []string

	if len(req.Files) > 0 {
		var fileContext strings.Builder
		hasTextFiles := false

		for _, f := range req.Files {
			// Image files → pass as native vision data URLs
			if strings.HasPrefix(f.Type, "image/") {
				imageDataURLs = append(imageDataURLs, "data:"+f.Type+";base64,"+f.Content)
				continue
			}
			// Other files → embed as text context
			hasTextFiles = true
			fileContext.WriteString("=== FILE: " + f.Name + " ===\n")
			if f.IsText {
				fileContext.WriteString(f.Content)
			} else {
				fileContext.WriteString("[Binary file: " + f.Name + " (" + f.Type + ") — cannot display inline]\n")
			}
			fileContext.WriteString("\n=== END OF: " + f.Name + " ===\n\n")
		}

		if hasTextFiles {
			prefix := "The user has attached the following file(s) for you to read:\n\n" + fileContext.String()
			if fullMessage != "" {
				prefix += "User message: " + fullMessage
			}
			fullMessage = prefix
		}
	}

	ctx := r.Context()
	var response string
	var err error

	if len(imageDataURLs) > 0 {
		response, err = s.agentLoop.ProcessDirectWithImages(ctx, fullMessage, sessionKey, imageDataURLs)
	} else {
		response, err = s.agentLoop.ProcessDirect(ctx, fullMessage, sessionKey)
	}

	if err != nil {
		s.sendJSONError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"response":    response,
		"session_key": sessionKey,
		"model":       s.agentLoop.GetActiveModelLabel(),
	})
}

func (s *Server) handleChatStream(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.sendJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	limitBody(r)
	var req struct {
		Message    string `json:"message"`
		SessionKey string `json:"session_key"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.sendJSONError(w, err.Error(), http.StatusBadRequest)
		return
	}

	sessionKey := req.SessionKey
	if sessionKey == "" {
		sessionKey = "web:ui:" + time.Now().UTC().Format(time.RFC3339)
	}

	// Set SSE headers.
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	flusher, ok := w.(http.Flusher)
	if !ok {
		s.sendJSONError(w, "Streaming not supported", http.StatusInternalServerError)
		return
	}

	// Send initial session event.
	fmt.Fprintf(w, "data: %s\n\n", mustJSON(map[string]string{
		"type":        "session",
		"session_key": sessionKey,
	}))
	flusher.Flush()

	ctx := r.Context()
	modelLabel := s.agentLoop.GetActiveModelLabel()
	err := s.agentLoop.ProcessDirectStream(ctx, req.Message, sessionKey, func(text string, done bool) {
		if done {
			fmt.Fprintf(w, "data: %s\n\n", mustJSON(map[string]string{"type": "done", "model": modelLabel}))
			flusher.Flush()
			return
		}
		fmt.Fprintf(w, "data: %s\n\n", mustJSON(map[string]string{
			"type":    "delta",
			"content": text,
		}))
		flusher.Flush()
	})
	if err != nil {
		fmt.Fprintf(w, "data: %s\n\n", mustJSON(map[string]string{
			"type":  "error",
			"error": err.Error(),
		}))
		flusher.Flush()
	}
}

func (s *Server) handleLogs(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	// Restrict CORS to same-origin requests only (SSE endpoint)
	origin := r.Header.Get("Origin")
	if origin != "" {
		expectedOrigin := fmt.Sprintf("http://%s:%d", s.cfg.WebUI.Host, s.cfg.WebUI.Port)
		if s.cfg.WebUI.Host == "" || s.cfg.WebUI.Host == "0.0.0.0" {
			// Allow localhost variants when binding to all interfaces
			if origin == fmt.Sprintf("http://localhost:%d", s.cfg.WebUI.Port) ||
				origin == fmt.Sprintf("http://127.0.0.1:%d", s.cfg.WebUI.Port) {
				w.Header().Set("Access-Control-Allow-Origin", origin)
			}
		} else if origin == expectedOrigin {
			w.Header().Set("Access-Control-Allow-Origin", origin)
		}
	}

	logChan := logger.Subscribe()
	defer logger.Unsubscribe(logChan)

	// Send history
	history := logger.GetHistory()
	for _, line := range history {
		fmt.Fprintf(w, "data: %s\n\n", line)
	}

	// Send initial message if history is empty
	if len(history) == 0 {
		fmt.Fprintf(
			w,
			"data: {\"message\": \"Connected to log stream...\", \"level\": \"INFO\", \"component\": \"web\"}\n\n",
		)
	}

	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}

	for {
		select {
		case msg := <-logChan:
			fmt.Fprintf(w, "data: %s\n\n", msg)
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
		case <-r.Context().Done():
			return
		}
	}
}
