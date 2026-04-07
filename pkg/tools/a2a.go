package tools

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
)

// A2AMessageForTool mirrors the agent-level A2AMessage for tool-layer use,
// avoiding a circular import between tools and agent packages.
type A2AMessageForTool struct {
	ID        string
	From      string
	To        string
	Type      string
	Subject   string
	Payload   string
	ReplyTo   string
	Timestamp time.Time
}

// A2AToolRouter is the interface the A2ATool needs from the agent-level A2ARouter.
type A2AToolRouter interface {
	Send(msg *A2AMessageForTool) error
	Broadcast(from, subject, payload string) int
	Receive(agentID string, timeout time.Duration) *A2AMessageForTool
	Poll(agentID string) *A2AMessageForTool
	PendingCount(agentID string) int
}

// A2ATool provides LLM access to the inter-agent communication protocol.
type A2ATool struct {
	router  A2AToolRouter
	agentID string
}

// NewA2ATool creates a new A2ATool for the given agent.
func NewA2ATool(router A2AToolRouter, agentID string) *A2ATool {
	return &A2ATool{router: router, agentID: agentID}
}

func (t *A2ATool) Name() string { return "a2a" }
func (t *A2ATool) Description() string {
	return "Agent-to-Agent communication protocol. Send messages to other agents, broadcast to all, receive messages, or poll for pending messages. Operations: send, broadcast, receive, poll."
}

func (t *A2ATool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"operation": map[string]any{
				"type":        "string",
				"enum":        []string{"send", "broadcast", "receive", "poll"},
				"description": "The A2A operation to perform",
			},
			"to": map[string]any{
				"type":        "string",
				"description": "Target agent ID (required for send)",
			},
			"subject": map[string]any{
				"type":        "string",
				"description": "Message subject (required for send/broadcast)",
			},
			"payload": map[string]any{
				"type":        "string",
				"description": "Message content (required for send/broadcast)",
			},
			"reply_to": map[string]any{
				"type":        "string",
				"description": "Message ID being replied to (optional, for send)",
			},
			"message_type": map[string]any{
				"type":        "string",
				"enum":        []string{"request", "response", "query"},
				"description": "Type of message (optional, default: request)",
			},
			"timeout_seconds": map[string]any{
				"type":        "integer",
				"description": "Timeout in seconds for receive operation (default: 5)",
			},
		},
		"required": []string{"operation"},
	}
}

func (t *A2ATool) Execute(ctx context.Context, args map[string]any) *ToolResult {
	op, _ := args["operation"].(string)

	switch op {
	case "send":
		return t.executeSend(args)
	case "broadcast":
		return t.executeBroadcast(args)
	case "receive":
		return t.executeReceive(args)
	case "poll":
		return t.executePoll()
	default:
		return ErrorResult(fmt.Sprintf("unknown A2A operation: %q", op))
	}
}

func (t *A2ATool) executeSend(args map[string]any) *ToolResult {
	to, _ := args["to"].(string)
	if to == "" {
		return ErrorResult("'to' is required for send operation")
	}
	subject, _ := args["subject"].(string)
	if subject == "" {
		return ErrorResult("'subject' is required for send operation")
	}
	payload, _ := args["payload"].(string)
	replyTo, _ := args["reply_to"].(string)
	msgType, _ := args["message_type"].(string)
	if msgType == "" {
		msgType = "request"
	}

	msg := &A2AMessageForTool{
		From:    t.agentID,
		To:      to,
		Type:    msgType,
		Subject: subject,
		Payload: payload,
		ReplyTo: replyTo,
	}

	if err := t.router.Send(msg); err != nil {
		return ErrorResult(fmt.Sprintf("failed to send A2A message: %v", err))
	}

	return SilentResult(fmt.Sprintf("A2A message sent to %q (subject: %q, id: %s)", to, subject, msg.ID))
}

func (t *A2ATool) executeBroadcast(args map[string]any) *ToolResult {
	subject, _ := args["subject"].(string)
	if subject == "" {
		return ErrorResult("'subject' is required for broadcast operation")
	}
	payload, _ := args["payload"].(string)

	sent := t.router.Broadcast(t.agentID, subject, payload)
	return SilentResult(fmt.Sprintf("A2A broadcast sent to %d agents (subject: %q)", sent, subject))
}

func (t *A2ATool) executeReceive(args map[string]any) *ToolResult {
	timeout := 5 * time.Second
	if ts, ok := args["timeout_seconds"].(float64); ok && ts > 0 {
		timeout = time.Duration(ts * float64(time.Second))
	}

	msg := t.router.Receive(t.agentID, timeout)
	if msg == nil {
		return SilentResult("No A2A messages received (timeout)")
	}

	return SilentResult(formatA2AMessage(msg))
}

func (t *A2ATool) executePoll() *ToolResult {
	msg := t.router.Poll(t.agentID)
	if msg == nil {
		pending := t.router.PendingCount(t.agentID)
		return SilentResult(fmt.Sprintf("No pending A2A messages (mailbox: %d)", pending))
	}
	return SilentResult(formatA2AMessage(msg))
}

func formatA2AMessage(msg *A2AMessageForTool) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "A2A Message [%s]\n", msg.ID)
	fmt.Fprintf(&sb, "  From: %s\n", msg.From)
	fmt.Fprintf(&sb, "  Type: %s\n", msg.Type)
	fmt.Fprintf(&sb, "  Subject: %s\n", msg.Subject)
	if msg.ReplyTo != "" {
		fmt.Fprintf(&sb, "  ReplyTo: %s\n", msg.ReplyTo)
	}
	fmt.Fprintf(&sb, "  Payload: %s\n", msg.Payload)
	return sb.String()
}

// A2ARouterAdapter wraps an underlying router to convert message types,
// bridging the tools and agent packages without circular imports.
type A2ARouterAdapter struct {
	sendFn         func(from, to, msgType, subject, payload, replyTo string) (string, error)
	broadcastFn    func(from, subject, payload string) int
	receiveFn      func(agentID string, timeout time.Duration) *A2AMessageForTool
	pollFn         func(agentID string) *A2AMessageForTool
	pendingCountFn func(agentID string) int
}

func (a *A2ARouterAdapter) Send(msg *A2AMessageForTool) error {
	id, err := a.sendFn(msg.From, msg.To, msg.Type, msg.Subject, msg.Payload, msg.ReplyTo)
	if err != nil {
		return err
	}
	msg.ID = id
	return nil
}

func (a *A2ARouterAdapter) Broadcast(from, subject, payload string) int {
	return a.broadcastFn(from, subject, payload)
}

func (a *A2ARouterAdapter) Receive(agentID string, timeout time.Duration) *A2AMessageForTool {
	return a.receiveFn(agentID, timeout)
}

func (a *A2ARouterAdapter) Poll(agentID string) *A2AMessageForTool {
	return a.pollFn(agentID)
}

func (a *A2ARouterAdapter) PendingCount(agentID string) int {
	return a.pendingCountFn(agentID)
}

// NewA2ARouterAdapter creates an adapter with the provided callback functions.
func NewA2ARouterAdapter(
	sendFn func(from, to, msgType, subject, payload, replyTo string) (string, error),
	broadcastFn func(from, subject, payload string) int,
	receiveFn func(agentID string, timeout time.Duration) *A2AMessageForTool,
	pollFn func(agentID string) *A2AMessageForTool,
	pendingCountFn func(agentID string) int,
) *A2ARouterAdapter {
	return &A2ARouterAdapter{
		sendFn:         sendFn,
		broadcastFn:    broadcastFn,
		receiveFn:      receiveFn,
		pollFn:         pollFn,
		pendingCountFn: pendingCountFn,
	}
}

// InMemoryA2ARouter is a simple in-process implementation of A2AToolRouter
// for testing purposes.
type InMemoryA2ARouter struct {
	mailboxes map[string]chan *A2AMessageForTool
	agents    []string
	mu        sync.RWMutex
	nextID    int
}

func NewInMemoryA2ARouter() *InMemoryA2ARouter {
	return &InMemoryA2ARouter{
		mailboxes: make(map[string]chan *A2AMessageForTool),
	}
}

func (r *InMemoryA2ARouter) RegisterAgent(agentID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.mailboxes[agentID]; !ok {
		r.mailboxes[agentID] = make(chan *A2AMessageForTool, 64)
		r.agents = append(r.agents, agentID)
	}
}

func (r *InMemoryA2ARouter) Send(msg *A2AMessageForTool) error {
	r.mu.RLock()
	ch, ok := r.mailboxes[msg.To]
	r.mu.RUnlock()
	if !ok {
		return fmt.Errorf("a2a: target agent %q not registered", msg.To)
	}
	if msg.ID == "" {
		r.mu.Lock()
		r.nextID++
		msg.ID = fmt.Sprintf("a2a-%d", r.nextID)
		r.mu.Unlock()
	}
	if msg.Timestamp.IsZero() {
		msg.Timestamp = time.Now()
	}
	select {
	case ch <- msg:
		return nil
	default:
		return fmt.Errorf("a2a: mailbox full for agent %q", msg.To)
	}
}

func (r *InMemoryA2ARouter) Broadcast(from, subject, payload string) int {
	r.mu.RLock()
	agents := make([]string, len(r.agents))
	copy(agents, r.agents)
	r.mu.RUnlock()

	sent := 0
	for _, agentID := range agents {
		if agentID == from {
			continue
		}
		msg := &A2AMessageForTool{
			From:      from,
			To:        agentID,
			Type:      "broadcast",
			Subject:   subject,
			Payload:   payload,
			Timestamp: time.Now(),
		}
		if err := r.Send(msg); err == nil {
			sent++
		}
	}
	return sent
}

func (r *InMemoryA2ARouter) Receive(agentID string, timeout time.Duration) *A2AMessageForTool {
	r.mu.RLock()
	ch, ok := r.mailboxes[agentID]
	r.mu.RUnlock()
	if !ok {
		return nil
	}
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	select {
	case msg := <-ch:
		return msg
	case <-time.After(timeout):
		return nil
	}
}

func (r *InMemoryA2ARouter) Poll(agentID string) *A2AMessageForTool {
	r.mu.RLock()
	ch, ok := r.mailboxes[agentID]
	r.mu.RUnlock()
	if !ok {
		return nil
	}
	select {
	case msg := <-ch:
		return msg
	default:
		return nil
	}
}

func (r *InMemoryA2ARouter) PendingCount(agentID string) int {
	r.mu.RLock()
	ch, ok := r.mailboxes[agentID]
	r.mu.RUnlock()
	if !ok {
		return 0
	}
	return len(ch)
}
