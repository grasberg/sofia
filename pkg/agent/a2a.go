package agent

import (
	"fmt"
	"sync"
	"time"
)

// A2AMessageType defines the type of inter-agent message.
type A2AMessageType string

const (
	A2ARequest   A2AMessageType = "request"
	A2AResponse  A2AMessageType = "response"
	A2ABroadcast A2AMessageType = "broadcast"
	A2AQuery     A2AMessageType = "query"
)

// A2AMessage represents a standardized message between agents.
type A2AMessage struct {
	ID        string         `json:"id"`
	From      string         `json:"from"`
	To        string         `json:"to"`
	Type      A2AMessageType `json:"type"`
	Subject   string         `json:"subject"`
	Payload   string         `json:"payload"`
	ReplyTo   string         `json:"reply_to,omitempty"`
	Timestamp time.Time      `json:"timestamp"`
}

type A2ARouter struct {
	mailboxes       map[string]chan *A2AMessage
	agents          []string
	mu              sync.RWMutex
	nextID          int
	monitorCallback func(msg *A2AMessage)
}

const defaultMailboxSize = 64

// NewA2ARouter creates a new A2A message router.
func NewA2ARouter() *A2ARouter {
	return &A2ARouter{
		mailboxes: make(map[string]chan *A2AMessage),
	}
}

// SetMonitorCallback sets a function to be called whenever a message is sent.
func (r *A2ARouter) SetMonitorCallback(cb func(msg *A2AMessage)) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.monitorCallback = cb
}

// Register ensures an agent has a mailbox. Safe to call multiple times.
func (r *A2ARouter) Register(agentID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.mailboxes[agentID]; !ok {
		r.mailboxes[agentID] = make(chan *A2AMessage, defaultMailboxSize)
		r.agents = append(r.agents, agentID)
	}
}

// Send delivers a message to a specific agent's mailbox.
// Returns an error if the target agent is not registered or mailbox is full.
func (r *A2ARouter) Send(msg *A2AMessage) error {
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
		r.mu.RLock()
		cb := r.monitorCallback
		r.mu.RUnlock()
		if cb != nil {
			cb(msg)
		}
		return nil
	default:
		return fmt.Errorf("a2a: mailbox full for agent %q", msg.To)
	}
}

// Broadcast sends a message to all registered agents except the sender.
func (r *A2ARouter) Broadcast(from, subject, payload string) int {
	r.mu.RLock()
	agents := make([]string, len(r.agents))
	copy(agents, r.agents)
	r.mu.RUnlock()

	sent := 0
	for _, agentID := range agents {
		if agentID == from {
			continue
		}
		msg := &A2AMessage{
			From:      from,
			To:        agentID,
			Type:      A2ABroadcast,
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

// Receive waits for a message in the agent's mailbox up to the given timeout.
// Returns nil if no message arrives within the timeout.
func (r *A2ARouter) Receive(agentID string, timeout time.Duration) *A2AMessage {
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

// Poll returns the next pending message without blocking.
// Returns nil if no messages are pending.
func (r *A2ARouter) Poll(agentID string) *A2AMessage {
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

// PendingCount returns the number of pending messages for an agent.
func (r *A2ARouter) PendingCount(agentID string) int {
	r.mu.RLock()
	ch, ok := r.mailboxes[agentID]
	r.mu.RUnlock()

	if !ok {
		return 0
	}
	return len(ch)
}
