package web

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/grasberg/sofia/pkg/audit"
)

// handleAudit returns recent audit entries as JSON with optional query params:
// action, agent_id, limit, offset.
func (s *Server) handleAudit(w http.ResponseWriter, r *http.Request) {
	if s.auditLogger == nil {
		s.sendJSONError(w, "Audit logging is not enabled", http.StatusServiceUnavailable)
		return
	}

	q := r.URL.Query()
	opts := audit.QueryOpts{
		AgentID: q.Get("agent_id"),
		Action:  q.Get("action"),
	}

	if v := q.Get("limit"); v != "" {
		fmt.Sscanf(v, "%d", &opts.Limit)
	}
	if v := q.Get("offset"); v != "" {
		fmt.Sscanf(v, "%d", &opts.Offset)
	}

	entries, err := s.auditLogger.Query(opts)
	if err != nil {
		s.sendJSONError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if entries == nil {
		entries = []audit.AuditEntry{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(entries)
}

// handleApprovals returns pending approval requests as JSON.
func (s *Server) handleApprovals(w http.ResponseWriter, _ *http.Request) {
	gate := s.agentLoop.GetApprovalGate()
	if gate == nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]any{})
		return
	}
	pending := gate.ListPending()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(pending)
}

// handleApprovalAction handles POST /api/approvals/{id}/approve and
// POST /api/approvals/{id}/deny.
func (s *Server) handleApprovalAction(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.sendJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	gate := s.agentLoop.GetApprovalGate()
	if gate == nil {
		s.sendJSONError(w, "Approval gates not enabled", http.StatusNotFound)
		return
	}

	path := strings.TrimPrefix(r.URL.Path, "/api/approvals/")
	parts := strings.SplitN(path, "/", 2)
	if len(parts) != 2 || parts[0] == "" {
		s.sendJSONError(
			w,
			"Invalid path: expected /api/approvals/{id}/{action}",
			http.StatusBadRequest,
		)
		return
	}

	requestID := parts[0]
	action := parts[1]

	var err error
	switch action {
	case "approve":
		err = gate.Approve(requestID)
	case "deny":
		err = gate.Deny(requestID)
	default:
		s.sendJSONError(
			w, "Invalid action: must be 'approve' or 'deny'",
			http.StatusBadRequest,
		)
		return
	}

	if err != nil {
		s.sendJSONError(w, err.Error(), http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"status":"ok"}`))
}
