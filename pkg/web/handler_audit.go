package web

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/grasberg/sofia/pkg/agent"
	"github.com/grasberg/sofia/pkg/audit"
	"github.com/grasberg/sofia/pkg/evolution"
)

// unifiedApproval is the shape returned by the /api/approvals endpoint. It
// accommodates both tool-call approvals (from ApprovalGate) and evolution
// proposals (from EvolutionEngine) so the UI can render a single list.
type unifiedApproval struct {
	ID         string         `json:"id"`         // "gate-<id>" or "evo-<id>" — prefix routes POST actions
	Kind       string         `json:"kind"`       // "tool_call" | "evolution"
	Title      string         `json:"title"`
	Summary    string         `json:"summary,omitempty"`
	Arguments  string         `json:"arguments,omitempty"`
	AgentID    string         `json:"agent_id,omitempty"`
	Channel    string         `json:"channel,omitempty"`
	ChatID     string         `json:"chat_id,omitempty"`
	RiskLevel  string         `json:"risk_level,omitempty"`
	Status     string         `json:"status"`
	CreatedAt  string         `json:"created_at"`
	SessionKey string         `json:"session_key,omitempty"`
	Action     map[string]any `json:"action,omitempty"` // evolution-only payload for preview
}

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

// handleApprovals returns pending approvals from both the tool-call gate and
// the evolution-engine proposal queue as a single JSON array.
func (s *Server) handleApprovals(w http.ResponseWriter, _ *http.Request) {
	list := make([]unifiedApproval, 0, 4)

	if gate := s.agentLoop.GetApprovalGate(); gate != nil {
		for _, req := range gate.ListPending() {
			list = append(list, toolCallToUnified(req))
		}
	}
	if engine := s.agentLoop.GetEvolutionEngine(); engine != nil {
		for _, p := range engine.GetPendingProposals() {
			list = append(list, proposalToUnified(p))
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(list)
}

// handleApprovalAction routes POST /api/approvals/{id}/{approve|deny} to
// either the ApprovalGate or the EvolutionEngine based on the id prefix:
//   - "gate-<uuid>" → ApprovalGate.Approve/Deny
//   - "evo-<uuid>"  → EvolutionEngine.Approve/RejectProposal
//
// Unprefixed ids are routed to the gate for backwards compatibility.
func (s *Server) handleApprovalAction(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.sendJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
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

	prefixedID := parts[0]
	action := parts[1]

	if action != "approve" && action != "deny" {
		s.sendJSONError(
			w, "Invalid action: must be 'approve' or 'deny'",
			http.StatusBadRequest,
		)
		return
	}

	var err error
	switch {
	case strings.HasPrefix(prefixedID, "evo-"):
		engine := s.agentLoop.GetEvolutionEngine()
		if engine == nil {
			s.sendJSONError(w, "Evolution engine not running", http.StatusNotFound)
			return
		}
		id := strings.TrimPrefix(prefixedID, "evo-")
		if action == "approve" {
			err = engine.ApproveProposal(r.Context(), id)
		} else {
			err = engine.RejectProposal(id)
		}
	default:
		gate := s.agentLoop.GetApprovalGate()
		if gate == nil {
			s.sendJSONError(w, "Approval gates not enabled", http.StatusNotFound)
			return
		}
		id := strings.TrimPrefix(prefixedID, "gate-")
		if action == "approve" {
			err = gate.Approve(id)
		} else {
			err = gate.Deny(id)
		}
	}

	if err != nil {
		s.sendJSONError(w, err.Error(), http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"status":"ok"}`))
}

// toolCallToUnified converts an ApprovalGate request into the common shape.
func toolCallToUnified(req agent.ApprovalRequest) unifiedApproval {
	return unifiedApproval{
		ID:         "gate-" + req.ID,
		Kind:       "tool_call",
		Title:      req.ToolName,
		Arguments:  req.Arguments,
		AgentID:    req.AgentID,
		Channel:    req.Channel,
		ChatID:     req.ChatID,
		RiskLevel:  string(req.RiskLevel),
		Status:     req.Status,
		CreatedAt:  req.CreatedAt.UTC().Format("2006-01-02T15:04:05Z"),
		SessionKey: req.SessionKey,
	}
}

// proposalToUnified converts an EvolutionEngine proposal into the common shape.
func proposalToUnified(p evolution.Proposal) unifiedApproval {
	actionMap := map[string]any{
		"type":     string(p.Action.Type),
		"agent_id": p.Action.AgentID,
		"params":   p.Action.Params,
		"reason":   p.Action.Reason,
	}
	return unifiedApproval{
		ID:        "evo-" + p.ID,
		Kind:      "evolution",
		Title:     string(p.Action.Type),
		Summary:   p.Action.Reason,
		Status:    p.Status,
		CreatedAt: p.CreatedAt.UTC().Format("2006-01-02T15:04:05Z"),
		Action:    actionMap,
	}
}
