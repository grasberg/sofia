package agent

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestA2ARouter_SendReceive(t *testing.T) {
	router := NewA2ARouter()
	router.Register("agent-a")
	router.Register("agent-b")

	err := router.Send(&A2AMessage{
		From:    "agent-a",
		To:      "agent-b",
		Type:    A2ARequest,
		Subject: "help",
		Payload: "Can you analyze this data?",
	})
	require.NoError(t, err)

	msg := router.Receive("agent-b", time.Second)
	require.NotNil(t, msg)
	assert.Equal(t, "agent-a", msg.From)
	assert.Equal(t, "agent-b", msg.To)
	assert.Equal(t, A2ARequest, msg.Type)
	assert.Equal(t, "help", msg.Subject)
	assert.Equal(t, "Can you analyze this data?", msg.Payload)
	assert.NotEmpty(t, msg.ID)
	assert.False(t, msg.Timestamp.IsZero())
}

func TestA2ARouter_ReceiveTimeout(t *testing.T) {
	router := NewA2ARouter()
	router.Register("agent-a")

	msg := router.Receive("agent-a", 50*time.Millisecond)
	assert.Nil(t, msg, "should timeout with nil on empty mailbox")
}

func TestA2ARouter_Poll(t *testing.T) {
	router := NewA2ARouter()
	router.Register("agent-a")
	router.Register("agent-b")

	// Poll empty mailbox
	msg := router.Poll("agent-a")
	assert.Nil(t, msg)

	// Send then poll
	_ = router.Send(&A2AMessage{
		From: "agent-b", To: "agent-a", Type: A2AQuery, Subject: "status", Payload: "ready?",
	})
	msg = router.Poll("agent-a")
	require.NotNil(t, msg)
	assert.Equal(t, "agent-b", msg.From)

	// Poll again should be empty
	msg = router.Poll("agent-a")
	assert.Nil(t, msg)
}

func TestA2ARouter_Broadcast(t *testing.T) {
	router := NewA2ARouter()
	router.Register("agent-a")
	router.Register("agent-b")
	router.Register("agent-c")

	sent := router.Broadcast("agent-a", "announcement", "system update")
	assert.Equal(t, 2, sent, "should send to all except sender")

	// Both b and c should have messages
	msgB := router.Poll("agent-b")
	require.NotNil(t, msgB)
	assert.Equal(t, "announcement", msgB.Subject)
	assert.Equal(t, A2ABroadcast, msgB.Type)

	msgC := router.Poll("agent-c")
	require.NotNil(t, msgC)
	assert.Equal(t, "announcement", msgC.Subject)

	// Sender should NOT have a message
	msgA := router.Poll("agent-a")
	assert.Nil(t, msgA)
}

func TestA2ARouter_SendUnregistered(t *testing.T) {
	router := NewA2ARouter()
	router.Register("agent-a")

	err := router.Send(&A2AMessage{
		From: "agent-a", To: "nonexistent", Type: A2ARequest, Subject: "test",
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not registered")
}

func TestA2ARouter_PendingCount(t *testing.T) {
	router := NewA2ARouter()
	router.Register("agent-a")
	router.Register("agent-b")

	assert.Equal(t, 0, router.PendingCount("agent-b"))

	_ = router.Send(&A2AMessage{From: "agent-a", To: "agent-b", Type: A2ARequest, Subject: "1"})
	_ = router.Send(&A2AMessage{From: "agent-a", To: "agent-b", Type: A2ARequest, Subject: "2"})

	assert.Equal(t, 2, router.PendingCount("agent-b"))
}

func TestA2ARouter_PollUnregistered(t *testing.T) {
	router := NewA2ARouter()
	msg := router.Poll("nonexistent")
	assert.Nil(t, msg)
	assert.Equal(t, 0, router.PendingCount("nonexistent"))
}
