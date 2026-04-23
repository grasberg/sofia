package agent

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/grasberg/sofia/pkg/config"
	"github.com/grasberg/sofia/pkg/logger"
)

// ApprovalRequest represents a pending approval for a tool call.
type ApprovalRequest struct {
	ID         string    `json:"id"`
	ToolName   string    `json:"tool_name"`
	Arguments  string    `json:"arguments"`
	AgentID    string    `json:"agent_id"`
	SessionKey string    `json:"session_key"`
	Channel    string    `json:"channel"`
	ChatID     string    `json:"chat_id"`
	CreatedAt  time.Time `json:"created_at"`
	Status     string    `json:"status"`     // "pending", "approved", "denied", "timeout"
	RiskLevel  RiskLevel `json:"risk_level"` // populated when a RiskClassifier is wired
}

// ApprovalBroadcaster is the callback the gate invokes whenever the pending
// queue changes (create / approve / deny / timeout). The map carries:
//
//	"type"       → "approval_created" | "approval_resolved"
//	"id"         → gate-<uuid>
//	"tool"       → tool name
//	"risk_level" → "low"|"medium"|"high"|"unknown"
//	"status"     → "pending"|"approved"|"denied"|"timeout"
//
// Wire this to dashboard.Hub.Broadcast (or any sink) via SetBroadcaster.
type ApprovalBroadcaster func(event map[string]any)

// ApprovalGate manages human-in-the-loop approval for high-risk tool calls.
type ApprovalGate struct {
	mu             sync.Mutex
	config         config.ApprovalConfig
	pending        map[string]*approvalEntry
	patterns       []*regexp.Regexp
	approvalBypass sync.Map // sessionKey -> bool
	classifier     RiskClassifier
	broadcaster    ApprovalBroadcaster
}

type approvalEntry struct {
	Request  ApprovalRequest
	ResultCh chan bool
}

// NewApprovalGate creates a new ApprovalGate from the given config.
// Invalid regex patterns in PatternMatch are logged and skipped.
func NewApprovalGate(cfg config.ApprovalConfig) *ApprovalGate {
	patterns := make([]*regexp.Regexp, 0, len(cfg.PatternMatch))
	for _, p := range cfg.PatternMatch {
		re, err := regexp.Compile(p)
		if err != nil {
			logger.WarnCF("approval", "Invalid approval pattern, skipping",
				map[string]any{"pattern": p, "error": err.Error()})
			continue
		}
		patterns = append(patterns, re)
	}

	gate := &ApprovalGate{
		config:   cfg,
		pending:  make(map[string]*approvalEntry),
		patterns: patterns,
	}

	switch strings.ToLower(strings.TrimSpace(cfg.RiskClassifier)) {
	case "heuristic":
		gate.classifier = NewHeuristicClassifier(cfg.RiskAmountThreshold, cfg.RiskAngryKeywords)
	case "", "off", "none", "disabled":
		// leave classifier nil — preserves legacy behavior
	case "llm":
		// LLM classifier is wired externally by the caller via SetClassifier
		// because it needs a provider handle.
	default:
		logger.WarnCF("approval", "Unknown risk_classifier value, classifier disabled",
			map[string]any{"value": cfg.RiskClassifier})
	}

	return gate
}

// SetClassifier installs a risk classifier consulted when a tool call is not
// already flagged by RequireFor / PatternMatch. Pass nil to disable.
func (ag *ApprovalGate) SetClassifier(c RiskClassifier) {
	ag.mu.Lock()
	defer ag.mu.Unlock()
	ag.classifier = c
}

// SetBroadcaster installs a callback the gate invokes on every pending-queue
// change. Passing nil disables broadcasting.
func (ag *ApprovalGate) SetBroadcaster(b ApprovalBroadcaster) {
	ag.mu.Lock()
	defer ag.mu.Unlock()
	ag.broadcaster = b
}

// broadcastEvent invokes the broadcaster under a released lock. Callers must
// not hold ag.mu when they call this.
func (ag *ApprovalGate) broadcastEvent(ev map[string]any) {
	ag.mu.Lock()
	b := ag.broadcaster
	ag.mu.Unlock()
	if b == nil {
		return
	}
	// Isolate sink errors / panics from gate critical paths.
	defer func() { _ = recover() }()
	b(ev)
}

// Classify exposes the configured classifier so callers (e.g. workflow steps)
// can assess risk ahead of sending a side-effect through the gate. Returns
// RiskUnknown when no classifier is configured.
func (ag *ApprovalGate) Classify(ctx context.Context, d ToolCallDescriptor) RiskLevel {
	ag.mu.Lock()
	c := ag.classifier
	ag.mu.Unlock()
	if c == nil {
		return RiskUnknown
	}
	return c.Classify(ctx, d)
}

// SetBypass enables or disables approval bypass for the given session.
// When bypass is on, RequiresApproval always returns false for that session.
func (ag *ApprovalGate) SetBypass(sessionKey string, on bool) {
	if on {
		ag.approvalBypass.Store(sessionKey, true)
	} else {
		ag.approvalBypass.Delete(sessionKey)
	}
}

// IsBypassed returns true if approval is currently bypassed for the given session.
func (ag *ApprovalGate) IsBypassed(sessionKey string) bool {
	_, ok := ag.approvalBypass.Load(sessionKey)
	return ok
}

// RequiresApproval checks whether a tool call needs human approval.
// It returns true if the tool name is in the RequireFor list, if argsJSON
// matches any PatternMatch regex, or if the configured risk classifier rates
// the call as Medium or higher. Returns false immediately if bypass is set
// for the given session.
func (ag *ApprovalGate) RequiresApproval(sessionKey string, toolName string, argsJSON string) bool {
	return ag.RequiresApprovalWithHints(sessionKey, toolName, argsJSON, nil)
}

// RequiresApprovalWithHints is the extended form that lets callers pass
// structured hints (sentiment, files_changed, content, subject, ...) to the
// risk classifier. argsJSON is still matched against PatternMatch regex.
func (ag *ApprovalGate) RequiresApprovalWithHints(sessionKey string, toolName string, argsJSON string, hints map[string]string) bool {
	if ag.IsBypassed(sessionKey) {
		return false
	}

	if !ag.config.Enabled {
		return false
	}

	for _, name := range ag.config.RequireFor {
		if name == toolName {
			return true
		}
	}

	for _, re := range ag.patterns {
		if re.MatchString(argsJSON) {
			return true
		}
	}

	ag.mu.Lock()
	classifier := ag.classifier
	ag.mu.Unlock()
	if classifier != nil {
		level := classifier.Classify(context.Background(), ToolCallDescriptor{
			ToolName:  toolName,
			Arguments: argsJSON,
			Hints:     hints,
		})
		if level == RiskMedium || level == RiskHigh {
			return true
		}
	}

	return false
}

// RequestApproval creates a pending approval entry and blocks until the request
// is approved, denied, or times out. Returns true if approved, false otherwise.
func (ag *ApprovalGate) RequestApproval(ctx context.Context, req ApprovalRequest) (bool, error) {
	timeout := ag.config.TimeoutSec
	if timeout <= 0 {
		timeout = 300
	}

	req.Status = "pending"
	req.CreatedAt = time.Now()

	entry := &approvalEntry{
		Request:  req,
		ResultCh: make(chan bool, 1),
	}

	ag.mu.Lock()
	ag.pending[req.ID] = entry
	ag.mu.Unlock()

	logger.InfoCF("approval", fmt.Sprintf("Approval requested for tool %q", req.ToolName),
		map[string]any{
			"request_id": req.ID,
			"tool":       req.ToolName,
			"agent_id":   req.AgentID,
			"channel":    req.Channel,
			"timeout":    timeout,
		})

	ag.broadcastEvent(map[string]any{
		"type":       "approval_created",
		"id":         "gate-" + req.ID,
		"tool":       req.ToolName,
		"agent_id":   req.AgentID,
		"channel":    req.Channel,
		"risk_level": string(req.RiskLevel),
		"status":     "pending",
	})

	defer func() {
		ag.mu.Lock()
		delete(ag.pending, req.ID)
		ag.mu.Unlock()
	}()

	timer := time.NewTimer(time.Duration(timeout) * time.Second)
	defer timer.Stop()

	select {
	case approved := <-entry.ResultCh:
		status := "denied"
		if approved {
			status = "approved"
		}
		logger.InfoCF("approval", fmt.Sprintf("Approval %s for tool %q", status, req.ToolName),
			map[string]any{"request_id": req.ID, "tool": req.ToolName})
		return approved, nil

	case <-timer.C:
		defaultAllow := ag.config.DefaultAction == "allow"
		action := "deny"
		if defaultAllow {
			action = "allow"
		}
		logger.WarnCF("approval",
			fmt.Sprintf("Approval timed out for tool %q, default action: %s", req.ToolName, action),
			map[string]any{"request_id": req.ID, "tool": req.ToolName, "timeout": timeout})
		fields := approvalAuditFields(req)
		fields["default_action"] = action
		fields["timeout_sec"] = timeout
		logger.Audit("Approval Decision: TIMEOUT", fields)
		ag.broadcastEvent(map[string]any{
			"type":           "approval_resolved",
			"id":             "gate-" + req.ID,
			"status":         "timeout",
			"default_action": action,
			"tool":           req.ToolName,
		})
		return defaultAllow, nil

	case <-ctx.Done():
		return false, ctx.Err()
	}
}

// Approve approves a pending request, unblocking the waiting goroutine.
func (ag *ApprovalGate) Approve(requestID string) error {
	ag.mu.Lock()
	entry, ok := ag.pending[requestID]
	ag.mu.Unlock()

	if !ok {
		return fmt.Errorf("approval request %q not found or already resolved", requestID)
	}

	logger.Audit("Approval Decision: APPROVED", approvalAuditFields(entry.Request))

	select {
	case entry.ResultCh <- true:
	default:
	}

	ag.broadcastEvent(map[string]any{
		"type":   "approval_resolved",
		"id":     "gate-" + requestID,
		"status": "approved",
		"tool":   entry.Request.ToolName,
	})
	return nil
}

// Deny denies a pending request, unblocking the waiting goroutine.
func (ag *ApprovalGate) Deny(requestID string) error {
	ag.mu.Lock()
	entry, ok := ag.pending[requestID]
	ag.mu.Unlock()

	if !ok {
		return fmt.Errorf("approval request %q not found or already resolved", requestID)
	}

	logger.Audit("Approval Decision: DENIED", approvalAuditFields(entry.Request))

	select {
	case entry.ResultCh <- false:
	default:
	}

	ag.broadcastEvent(map[string]any{
		"type":   "approval_resolved",
		"id":     "gate-" + requestID,
		"status": "denied",
		"tool":   entry.Request.ToolName,
	})
	return nil
}

// approvalAuditFields returns the common audit log fields for an approval request.
func approvalAuditFields(req ApprovalRequest) map[string]any {
	return map[string]any{
		"request_id": req.ID,
		"tool":       req.ToolName,
		"agent_id":   req.AgentID,
		"channel":    req.Channel,
		"chat_id":    req.ChatID,
		"session":    req.SessionKey,
	}
}

// ListPending returns a snapshot of all currently pending approval requests.
func (ag *ApprovalGate) ListPending() []ApprovalRequest {
	ag.mu.Lock()
	defer ag.mu.Unlock()

	requests := make([]ApprovalRequest, 0, len(ag.pending))
	for _, entry := range ag.pending {
		requests = append(requests, entry.Request)
	}
	return requests
}
