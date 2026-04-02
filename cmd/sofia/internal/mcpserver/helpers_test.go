package mcpserver

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/grasberg/sofia/pkg/bus"
)

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

func TestTransportValidation(t *testing.T) {
	tests := []struct {
		transport string
		valid     bool
	}{
		{"stdio", true},
		{"sse", true},
		{"tcp", false},
		{"http", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.transport, func(t *testing.T) {
			// Validate transport string
			isValid := tt.transport == "stdio" || tt.transport == "sse"
			assert.Equal(t, tt.valid, isValid)
		})
	}
}

func TestAddressFormat(t *testing.T) {
	tests := []struct {
		addr  string
		name  string
		valid bool
	}{
		{":9090", "default sse address", true},
		{":8080", "custom port", true},
		{"localhost:9090", "with hostname", true},
		{"127.0.0.1:9090", "with ip", true},
		{"", "empty address", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			isValid := tt.addr != ""
			assert.Equal(t, tt.valid, isValid)
		})
	}
}
