package channels

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/grasberg/sofia/pkg/logger"
)

// GmailSender implements EmailSender using the gog CLI to send via the Gmail API.
// This avoids SMTP credentials entirely — gog handles OAuth authentication
// with the user's Google account.
type GmailSender struct {
	binaryPath string
	account    string // Gmail address (passed as --account)
	timeout    time.Duration
}

// NewGmailSender creates a GmailSender. binaryPath is the path to the gog
// binary (defaults to "gog"). account is the Gmail address to send from.
func NewGmailSender(binaryPath, account string, timeoutSeconds int) *GmailSender {
	if strings.TrimSpace(binaryPath) == "" {
		binaryPath = "gog"
	}
	// Only allow bare command names resolved via PATH — reject absolute/relative
	// paths to prevent config-driven code execution.
	if strings.ContainsAny(binaryPath, "/\\") {
		logger.WarnCF("email", "gog_binary contains path separators, falling back to 'gog'", map[string]any{
			"configured": binaryPath,
		})
		binaryPath = "gog"
	}
	if timeoutSeconds <= 0 {
		timeoutSeconds = 90
	}
	return &GmailSender{
		binaryPath: binaryPath,
		account:    account,
		timeout:    time.Duration(timeoutSeconds) * time.Second,
	}
}

// Send delivers an email via the Gmail API through the gog CLI.
func (g *GmailSender) Send(ctx context.Context, to, subject, body string) error {
	ctx, cancel := context.WithTimeout(ctx, g.timeout)
	defer cancel()

	args := []string{"gmail", "send",
		"--to", to,
		"--subject", subject,
		"--body", body,
	}
	if g.account != "" {
		args = append([]string{"--account", g.account}, args...)
	}

	cmd := exec.CommandContext(ctx, g.binaryPath, args...)

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		errMsg := strings.TrimSpace(stderr.String())
		if errMsg == "" {
			errMsg = err.Error()
		}
		logger.ErrorCF("email", "Gmail send failed", map[string]any{
			"to":    to,
			"error": errMsg,
		})
		return fmt.Errorf("gmail send failed: %s", errMsg)
	}

	logger.DebugCF("email", "Email sent via Gmail API", map[string]any{
		"to":      to,
		"account": g.account,
	})
	return nil
}
