package channels

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/grasberg/sofia/pkg/bus"
	"github.com/grasberg/sofia/pkg/config"
)

func TestEmailChannel_Name(t *testing.T) {
	cfg := config.EmailConfig{
		Enabled:    true,
		SMTPServer: "smtp.example.com:587",
		Username:   "bot@example.com",
		Password:   "secret",
	}
	msgBus := bus.NewMessageBus()

	ch := NewEmailChannel(cfg, msgBus)

	assert.Equal(t, "email", ch.Name())
}

func TestEmailChannel_NewWithConfig(t *testing.T) {
	cfg := config.EmailConfig{
		Enabled:      true,
		IMAPServer:   "imap.example.com:993",
		SMTPServer:   "smtp.example.com:587",
		Username:     "bot@example.com",
		Password:     "secret",
		PollInterval: 120,
		AllowFrom:    []string{"user@example.com"},
	}
	msgBus := bus.NewMessageBus()

	ch := NewEmailChannel(cfg, msgBus)

	require.NotNil(t, ch)
	assert.Equal(t, "email", ch.Name())
	assert.False(t, ch.IsRunning())
	assert.NotNil(t, ch.sender, "sender should be initialized when SMTPServer is set")
	assert.NotNil(t, ch.receiver, "receiver should always be initialized (stub or real)")

	// AllowFrom should be forwarded to the BaseChannel allowlist.
	assert.True(t, ch.IsAllowed("user@example.com"))
	assert.False(t, ch.IsAllowed("stranger@example.com"))
}

func TestEmailChannel_NewWithoutSMTP(t *testing.T) {
	cfg := config.EmailConfig{
		Enabled:  true,
		Username: "bot@example.com",
		Password: "secret",
	}
	msgBus := bus.NewMessageBus()

	ch := NewEmailChannel(cfg, msgBus)

	require.NotNil(t, ch)
	assert.Nil(t, ch.sender, "sender should be nil when SMTPServer is empty")
}

func TestSMTPSender_FormatMessage(t *testing.T) {
	sender := NewSMTPSender("smtp.example.com:587", "bot@example.com", "secret")

	msg := sender.FormatMessage("user@example.com", "Test Subject", "Hello, world!")

	assert.Contains(t, msg, "From: bot@example.com\r\n")
	assert.Contains(t, msg, "To: user@example.com\r\n")
	assert.Contains(t, msg, "Subject: Test Subject\r\n")
	assert.Contains(t, msg, "MIME-Version: 1.0\r\n")
	assert.Contains(t, msg, "Content-Type: text/plain; charset=\"utf-8\"\r\n")
	assert.Contains(t, msg, "\r\n\r\nHello, world!")
}

func TestSMTPSender_FormatMessageMultiline(t *testing.T) {
	sender := NewSMTPSender("smtp.example.com:587", "bot@example.com", "pw")

	body := "Line one\nLine two\nLine three"
	msg := sender.FormatMessage("to@test.com", "Multi", body)

	// Body should be present after the blank line separator.
	assert.Contains(t, msg, "\r\n\r\nLine one\nLine two\nLine three")
}
