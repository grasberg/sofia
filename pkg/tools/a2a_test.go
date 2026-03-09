package tools

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestA2ARouter() *InMemoryA2ARouter {
	r := NewInMemoryA2ARouter()
	r.RegisterAgent("alice")
	r.RegisterAgent("bob")
	r.RegisterAgent("charlie")
	return r
}

func TestA2ATool_Send(t *testing.T) {
	router := newTestA2ARouter()
	tool := NewA2ATool(router, "alice")

	result := tool.Execute(context.Background(), map[string]any{
		"operation":    "send",
		"to":           "bob",
		"subject":      "help",
		"payload":      "Need analysis",
		"message_type": "request",
	})

	assert.False(t, result.IsError)
	assert.Contains(t, result.ForLLM, "bob")
	assert.Contains(t, result.ForLLM, "help")

	// Verify bob received it
	msg := router.Poll("bob")
	require.NotNil(t, msg)
	assert.Equal(t, "alice", msg.From)
	assert.Equal(t, "help", msg.Subject)
	assert.Equal(t, "Need analysis", msg.Payload)
}

func TestA2ATool_SendMissingTo(t *testing.T) {
	router := newTestA2ARouter()
	tool := NewA2ATool(router, "alice")

	result := tool.Execute(context.Background(), map[string]any{
		"operation": "send",
		"subject":   "test",
	})
	assert.True(t, result.IsError)
	assert.Contains(t, result.ForLLM, "'to' is required")
}

func TestA2ATool_Broadcast(t *testing.T) {
	router := newTestA2ARouter()
	tool := NewA2ATool(router, "alice")

	result := tool.Execute(context.Background(), map[string]any{
		"operation": "broadcast",
		"subject":   "update",
		"payload":   "system restart",
	})

	assert.False(t, result.IsError)
	assert.Contains(t, result.ForLLM, "2 agents")

	// Bob and charlie should have messages
	assert.NotNil(t, router.Poll("bob"))
	assert.NotNil(t, router.Poll("charlie"))
	assert.Nil(t, router.Poll("alice"))
}

func TestA2ATool_Receive(t *testing.T) {
	router := newTestA2ARouter()
	tool := NewA2ATool(router, "bob")

	// Send a message to bob first
	_ = router.Send(&A2AMessageForTool{
		From: "alice", To: "bob", Type: "request", Subject: "query", Payload: "data?",
	})

	result := tool.Execute(context.Background(), map[string]any{
		"operation":       "receive",
		"timeout_seconds": float64(1),
	})

	assert.False(t, result.IsError)
	assert.Contains(t, result.ForLLM, "alice")
	assert.Contains(t, result.ForLLM, "query")
}

func TestA2ATool_ReceiveTimeout(t *testing.T) {
	router := newTestA2ARouter()
	tool := NewA2ATool(router, "bob")

	start := time.Now()
	result := tool.Execute(context.Background(), map[string]any{
		"operation":       "receive",
		"timeout_seconds": float64(0.1),
	})
	elapsed := time.Since(start)

	assert.False(t, result.IsError)
	assert.Contains(t, result.ForLLM, "timeout")
	assert.Less(t, elapsed, 2*time.Second)
}

func TestA2ATool_Poll(t *testing.T) {
	router := newTestA2ARouter()
	tool := NewA2ATool(router, "bob")

	// Empty poll
	result := tool.Execute(context.Background(), map[string]any{
		"operation": "poll",
	})
	assert.False(t, result.IsError)
	assert.Contains(t, result.ForLLM, "No pending")

	// Send then poll
	_ = router.Send(&A2AMessageForTool{
		From: "alice", To: "bob", Type: "request", Subject: "ping", Payload: "pong",
	})
	result = tool.Execute(context.Background(), map[string]any{
		"operation": "poll",
	})
	assert.False(t, result.IsError)
	assert.Contains(t, result.ForLLM, "ping")
}

func TestA2ATool_UnknownOperation(t *testing.T) {
	router := newTestA2ARouter()
	tool := NewA2ATool(router, "alice")

	result := tool.Execute(context.Background(), map[string]any{
		"operation": "invalid",
	})
	assert.True(t, result.IsError)
}
