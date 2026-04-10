package channels

import (
	"context"
	"fmt"
	"net/smtp"
	"strings"
	"time"

	"github.com/grasberg/sofia/pkg/bus"
	"github.com/grasberg/sofia/pkg/config"
	"github.com/grasberg/sofia/pkg/logger"
)

// EmailReceiver defines how to receive emails. Implementations may use IMAP,
// a local maildir, or any other retrieval mechanism.
type EmailReceiver interface {
	Connect(ctx context.Context) error
	Poll(ctx context.Context) ([]IncomingEmail, error)
	Close() error
}

// IncomingEmail represents a single inbound email message.
type IncomingEmail struct {
	From      string
	Subject   string
	Body      string
	Date      time.Time
	MessageID string
}

// EmailSender defines how to send emails.
type EmailSender interface {
	Send(ctx context.Context, to, subject, body string) error
}

// SMTPSender implements EmailSender using Go's net/smtp package.
type SMTPSender struct {
	server   string
	username string
	password string
}

// NewSMTPSender creates a new SMTPSender. The server string should include the
// port, e.g. "smtp.gmail.com:587".
func NewSMTPSender(server, username, password string) *SMTPSender {
	return &SMTPSender{
		server:   server,
		username: username,
		password: password,
	}
}

// FormatMessage builds an RFC 2822 message from the given fields.
func (s *SMTPSender) FormatMessage(to, subject, body string) string {
	var b strings.Builder
	b.WriteString("From: " + s.username + "\r\n")
	b.WriteString("To: " + to + "\r\n")
	b.WriteString("Subject: " + subject + "\r\n")
	b.WriteString("MIME-Version: 1.0\r\n")
	b.WriteString("Content-Type: text/plain; charset=\"utf-8\"\r\n")
	b.WriteString("\r\n")
	b.WriteString(body)
	return b.String()
}

// Send delivers an email via SMTP using PlainAuth.
func (s *SMTPSender) Send(_ context.Context, to, subject, body string) error {
	host := s.server
	if idx := strings.LastIndex(s.server, ":"); idx > 0 {
		host = s.server[:idx]
	}

	auth := smtp.PlainAuth("", s.username, s.password, host)
	msg := s.FormatMessage(to, subject, body)

	return smtp.SendMail(s.server, auth, s.username, []string{to}, []byte(msg))
}

// stubReceiver is a placeholder EmailReceiver that logs a warning. Replace it
// with a real IMAP implementation when a third-party library is added.
type stubReceiver struct{}

func (r *stubReceiver) Connect(_ context.Context) error {
	logger.WarnC("email", "IMAP receiver not implemented; email polling is disabled")
	return nil
}

func (r *stubReceiver) Poll(_ context.Context) ([]IncomingEmail, error) {
	return nil, nil
}

func (r *stubReceiver) Close() error {
	return nil
}

// EmailChannel implements the Channel interface for email communication.
type EmailChannel struct {
	*BaseChannel
	cfg      config.EmailConfig
	sender   EmailSender
	receiver EmailReceiver
	stopCh   chan struct{}
}

// NewEmailChannel creates a new EmailChannel from the given config and message bus.
// When UseGmailAPI is true, or when the username is a Gmail address and no SMTP
// server is configured, the channel sends via the Gmail API using the gog CLI
// (which handles Google OAuth authentication). Otherwise it falls back to SMTP.
func NewEmailChannel(cfg config.EmailConfig, msgBus *bus.MessageBus) *EmailChannel {
	base := NewBaseChannel("email", cfg, msgBus, cfg.AllowFrom)

	var sender EmailSender
	useGmail := cfg.UseGmailAPI || (cfg.SMTPServer == "" && isGmailAddress(cfg.Username))
	if useGmail {
		sender = NewGmailSender(cfg.GogBinary, cfg.Username, 90)
		logger.InfoCF("email", "Using Gmail API sender (gog CLI)", map[string]any{
			"account": cfg.Username,
		})
	} else if cfg.SMTPServer != "" {
		sender = NewSMTPSender(cfg.SMTPServer, cfg.Username, cfg.Password)
	}

	return &EmailChannel{
		BaseChannel: base,
		cfg:         cfg,
		sender:      sender,
		receiver:    &stubReceiver{},
		stopCh:      make(chan struct{}),
	}
}

// isGmailAddress returns true if the address is a Gmail or Google Workspace
// address that can use the Gmail API.
func isGmailAddress(addr string) bool {
	addr = strings.ToLower(strings.TrimSpace(addr))
	return strings.HasSuffix(addr, "@gmail.com") ||
		strings.HasSuffix(addr, "@googlemail.com")
}

// Start begins polling for inbound emails.
func (ec *EmailChannel) Start(ctx context.Context) error {
	logger.InfoC("email", "Starting email channel")

	if err := ec.receiver.Connect(ctx); err != nil {
		return fmt.Errorf("email receiver connect: %w", err)
	}

	ec.setRunning(true)

	interval := ec.cfg.PollInterval
	if interval <= 0 {
		interval = 60
	}

	go ec.pollLoop(ctx, time.Duration(interval)*time.Second)

	senderType := "smtp"
	if _, ok := ec.sender.(*GmailSender); ok {
		senderType = "gmail-api"
	}
	logger.InfoCF("email", "Email channel started", map[string]any{
		"sender":        senderType,
		"smtp_server":   ec.cfg.SMTPServer,
		"imap_server":   ec.cfg.IMAPServer,
		"poll_interval": interval,
	})

	return nil
}

// Stop shuts down the email channel and closes the receiver.
func (ec *EmailChannel) Stop(_ context.Context) error {
	logger.InfoC("email", "Stopping email channel")
	ec.setRunning(false)
	close(ec.stopCh)

	if err := ec.receiver.Close(); err != nil {
		logger.ErrorCF("email", "Error closing email receiver", map[string]any{
			"error": err.Error(),
		})
	}

	return nil
}

// Send delivers an outbound message as an email reply.
func (ec *EmailChannel) Send(_ context.Context, msg bus.OutboundMessage) error {
	if !ec.IsRunning() {
		return fmt.Errorf("email channel not running")
	}

	if ec.sender == nil {
		return fmt.Errorf("email sender not configured — set use_gmail_api:true with a Gmail username, or configure an SMTP server")
	}

	to := msg.ChatID
	if to == "" {
		return fmt.Errorf("recipient address (ChatID) is empty")
	}

	subject := "Re: Sofia"
	if msg.Type != "" {
		subject = "Re: Sofia [" + msg.Type + "]"
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := ec.sender.Send(ctx, to, subject, msg.Content); err != nil {
		return fmt.Errorf("failed to send email to %s: %w", to, err)
	}

	logger.DebugCF("email", "Email sent", map[string]any{
		"to": to,
	})

	return nil
}

// pollLoop periodically checks for new emails and publishes them to the bus.
func (ec *EmailChannel) pollLoop(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ec.stopCh:
			return
		case <-ticker.C:
			ec.fetchAndPublish(ctx)
		}
	}
}

// fetchAndPublish retrieves new emails and forwards them to the message bus.
func (ec *EmailChannel) fetchAndPublish(ctx context.Context) {
	emails, err := ec.receiver.Poll(ctx)
	if err != nil {
		logger.ErrorCF("email", "Failed to poll emails", map[string]any{
			"error": err.Error(),
		})
		return
	}

	for _, email := range emails {
		if !ec.IsAllowed(email.From) {
			logger.DebugCF("email", "Email rejected by allowlist", map[string]any{
				"from": email.From,
			})
			continue
		}

		metadata := map[string]string{
			"message_id": email.MessageID,
			"subject":    email.Subject,
			"date":       email.Date.Format(time.RFC3339),
			"peer_kind":  "direct",
			"peer_id":    email.From,
		}

		content := email.Body
		if email.Subject != "" {
			content = "[Subject: " + email.Subject + "]\n" + content
		}

		ec.HandleMessage(email.From, email.From, content, nil, metadata)
	}
}
