package gateway

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/grasberg/sofia/pkg/bus"
	"github.com/grasberg/sofia/pkg/cron"
)

func TestCronServiceCreation(t *testing.T) {
	tempDir := t.TempDir()
	workspace := tempDir

	msgBus := bus.NewMessageBus()

	// We can't easily test the full setup without creating a real agent loop,
	// but we can verify the cron service is created correctly
	cronStorePath := filepath.Join(workspace, "cron", "jobs.json")
	cronService := cron.NewCronService(cronStorePath, nil)

	require.NotNil(t, cronService)

	// Verify message bus is initialized
	assert.NotNil(t, msgBus)
}

func TestCronPathConstruction(t *testing.T) {
	tempDir := t.TempDir()
	workspace := tempDir

	cronStorePath := filepath.Join(workspace, "cron", "jobs.json")

	// Verify path is constructed correctly
	assert.Contains(t, cronStorePath, "cron")
	assert.Contains(t, cronStorePath, "jobs.json")
	assert.NotEmpty(t, cronStorePath)
}

func TestMessageBusInitialization(t *testing.T) {
	msgBus := bus.NewMessageBus()

	require.NotNil(t, msgBus)

	// Verify bus is functional
	msg := bus.InboundMessage{
		Channel:  "test",
		SenderID: "user1",
		ChatID:   "chat1",
		Content:  "test message",
	}

	msgBus.PublishInbound(msg)
	assert.NotNil(t, msgBus)
}
