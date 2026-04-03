package agent

import (
	"context"
	"fmt"
	"regexp"
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
	Status     string    `json:"status"` // "pending", "approved", "denied", "timeout"
}

// ApprovalGate manages human-in-the-loop approval for high-risk tool calls.
type ApprovalGate struct {
	mu       sync.Mutex
	config   config.ApprovalConfig
	pending  map[string]*approvalEntry
	patterns []*regexp.Regexp
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

	return &ApprovalGate{
		config:   cfg,
		pending:  make(map[string]*approvalEntry),
		patterns: patterns,
	}
}

// RequiresApproval checks whether a tool call needs human approval.
// It returns true if the tool name is in the RequireFor list or if argsJSON
// matches any PatternMatch regex.
func (ag *ApprovalGate) RequiresApproval(toolName string, argsJSON string) bool {
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
		logger.Audit("Approval Decision: TIMEOUT", map[string]any{
			"request_id":     req.ID,
			"tool":           req.ToolName,
			"agent_id":       req.AgentID,
			"channel":        req.Channel,
			"chat_id":        req.ChatID,
			"session":        req.SessionKey,
			"default_action": action,
			"timeout_sec":    timeout,
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

	logger.Audit("Approval Decision: APPROVED", map[string]any{
		"request_id": requestID,
		"tool":       entry.Request.ToolName,
		"agent_id":   entry.Request.AgentID,
		"channel":    entry.Request.Channel,
		"chat_id":    entry.Request.ChatID,
		"session":    entry.Request.SessionKey,
	})

	select {
	case entry.ResultCh <- true:
	default:
	}
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

	logger.Audit("Approval Decision: DENIED", map[string]any{
		"request_id": requestID,
		"tool":       entry.Request.ToolName,
		"agent_id":   entry.Request.AgentID,
		"channel":    entry.Request.Channel,
		"chat_id":    entry.Request.ChatID,
		"session":    entry.Request.SessionKey,
	})

	select {
	case entry.ResultCh <- false:
	default:
	}
	return nil
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
