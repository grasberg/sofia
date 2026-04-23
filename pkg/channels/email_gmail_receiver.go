package channels

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/grasberg/sofia/pkg/logger"
)

// gogRunner runs a gog subcommand and returns stdout. It is abstracted behind
// an interface so unit tests can mock the CLI invocation without actually
// launching a subprocess.
type gogRunner interface {
	Run(ctx context.Context, args ...string) ([]byte, error)
}

// execRunner is the production runner that shells out to the gog binary.
type execRunner struct {
	binary  string
	account string
	timeout time.Duration
}

// Run invokes gog with the configured binary, account, and timeout.
func (r *execRunner) Run(ctx context.Context, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()

	if r.account != "" {
		args = append([]string{"--account", r.account}, args...)
	}

	cmd := exec.CommandContext(ctx, r.binary, args...)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return nil, fmt.Errorf("gog %s: %s", strings.Join(args, " "), msg)
	}
	return stdout.Bytes(), nil
}

// GmailReceiver implements EmailReceiver by shelling out to the `gog` CLI,
// which wraps the Gmail API with the same OAuth credentials used by the
// sender. It avoids bringing in an IMAP client dependency. Deduplication of
// already-seen messages is the caller's concern — EmailChannel handles it.
type GmailReceiver struct {
	runner       gogRunner
	query        string // Gmail search query, e.g. "is:unread in:inbox"
	maxPerPoll   int
	maxBodyBytes int
	markAsRead   bool
}

// GmailReceiverOptions bundles construction parameters to keep the constructor
// signature stable as new fields are added.
type GmailReceiverOptions struct {
	BinaryPath   string
	Account      string
	Query        string
	MaxPerPoll   int
	MaxBodyBytes int
	MarkAsRead   bool
	TimeoutSec   int
}

// NewGmailReceiver constructs a receiver wired to the real `gog` binary.
func NewGmailReceiver(opts GmailReceiverOptions) *GmailReceiver {
	binary := strings.TrimSpace(opts.BinaryPath)
	if binary == "" {
		binary = "gog"
	}
	// Reject any configured path separators to prevent config-driven command
	// execution — match the sender's guard in email_gmail.go.
	if strings.ContainsAny(binary, "/\\") {
		logger.WarnCF("email", "gog_binary contains path separators, falling back to 'gog'", map[string]any{
			"configured": binary,
		})
		binary = "gog"
	}

	timeoutSec := opts.TimeoutSec
	if timeoutSec <= 0 {
		timeoutSec = 90
	}

	runner := &execRunner{
		binary:  binary,
		account: opts.Account,
		timeout: time.Duration(timeoutSec) * time.Second,
	}

	return newGmailReceiver(runner, opts)
}

// newGmailReceiver is the internal constructor used by tests to inject a mock
// runner.
func newGmailReceiver(runner gogRunner, opts GmailReceiverOptions) *GmailReceiver {
	query := strings.TrimSpace(opts.Query)
	if query == "" {
		query = "is:unread in:inbox"
	}

	maxPerPoll := opts.MaxPerPoll
	if maxPerPoll <= 0 {
		maxPerPoll = 10
	}

	maxBodyBytes := opts.MaxBodyBytes
	if maxBodyBytes <= 0 {
		maxBodyBytes = 64 * 1024
	}

	return &GmailReceiver{
		runner:       runner,
		query:        query,
		maxPerPoll:   maxPerPoll,
		maxBodyBytes: maxBodyBytes,
		markAsRead:   opts.MarkAsRead,
	}
}

// Connect is a no-op for Gmail — auth is managed by gog.
func (r *GmailReceiver) Connect(_ context.Context) error {
	logger.InfoCF("email", "Gmail receiver ready", map[string]any{
		"query":          r.query,
		"max_per_poll":   r.maxPerPoll,
		"max_body_bytes": r.maxBodyBytes,
		"mark_as_read":   r.markAsRead,
	})
	return nil
}

// Close is a no-op — nothing to release.
func (r *GmailReceiver) Close() error { return nil }

// Poll searches the inbox for the configured query and fetches each message
// body. Messages already seen (based on Gmail message ID) are the caller's
// concern — the receiver returns everything matching the query.
func (r *GmailReceiver) Poll(ctx context.Context) ([]IncomingEmail, error) {
	ids, err := r.searchMessageIDs(ctx)
	if err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return nil, nil
	}

	out := make([]IncomingEmail, 0, len(ids))
	for _, id := range ids {
		if ctx.Err() != nil {
			return out, ctx.Err()
		}

		email, err := r.fetchMessage(ctx, id)
		if err != nil {
			logger.WarnCF("email", "Failed to fetch message", map[string]any{
				"message_id": id, "error": err.Error(),
			})
			continue
		}
		if email == nil {
			continue
		}
		out = append(out, *email)

		if r.markAsRead {
			if err := r.removeUnreadLabel(ctx, id); err != nil {
				logger.WarnCF("email", "Failed to mark message as read", map[string]any{
					"message_id": id, "error": err.Error(),
				})
			}
		}
	}
	return out, nil
}

// searchMessageIDs calls `gog gmail messages search <query>` and extracts the
// message IDs. The gog JSON envelope varies in shape; this parser tolerates
// both `{"messages":[{"id":..}]}` and bare arrays of strings or objects.
func (r *GmailReceiver) searchMessageIDs(ctx context.Context) ([]string, error) {
	args := []string{
		"--json", "--results-only",
		"gmail", "messages", "search", r.query,
		"--max", strconv.Itoa(r.maxPerPoll),
	}
	out, err := r.runner.Run(ctx, args...)
	if err != nil {
		return nil, fmt.Errorf("gmail search: %w", err)
	}
	return parseMessageIDs(out)
}

// parseMessageIDs handles the shape variance in gog's JSON output.
func parseMessageIDs(out []byte) ([]string, error) {
	out = bytes.TrimSpace(out)
	if len(out) == 0 {
		return nil, nil
	}

	// Shape 1: wrapped {"messages":[...]} or {"results":[...]}
	var envelope struct {
		Messages []struct {
			ID       string `json:"id"`
			ThreadID string `json:"threadId"`
		} `json:"messages"`
		Results []struct {
			ID       string `json:"id"`
			ThreadID string `json:"threadId"`
		} `json:"results"`
	}
	if err := json.Unmarshal(out, &envelope); err == nil {
		ids := make([]string, 0, len(envelope.Messages)+len(envelope.Results))
		for _, m := range envelope.Messages {
			if m.ID != "" {
				ids = append(ids, m.ID)
			}
		}
		for _, m := range envelope.Results {
			if m.ID != "" {
				ids = append(ids, m.ID)
			}
		}
		if len(ids) > 0 {
			return ids, nil
		}
	}

	// Shape 2: bare array of objects.
	var arr []struct {
		ID       string `json:"id"`
		ThreadID string `json:"threadId"`
	}
	if err := json.Unmarshal(out, &arr); err == nil {
		ids := make([]string, 0, len(arr))
		for _, m := range arr {
			if m.ID != "" {
				ids = append(ids, m.ID)
			}
		}
		return ids, nil
	}

	// Shape 3: bare array of IDs (strings).
	var strs []string
	if err := json.Unmarshal(out, &strs); err == nil {
		return strs, nil
	}

	return nil, fmt.Errorf("unrecognized gog search output: %s", truncate(string(out), 200))
}

// fetchMessage retrieves full message details via `gog gmail get <id>`.
func (r *GmailReceiver) fetchMessage(ctx context.Context, messageID string) (*IncomingEmail, error) {
	args := []string{
		"--json", "--results-only",
		"gmail", "get", messageID, "--format", "full",
	}
	out, err := r.runner.Run(ctx, args...)
	if err != nil {
		return nil, fmt.Errorf("gmail get %s: %w", messageID, err)
	}
	return parseGmailMessage(out, r.maxBodyBytes)
}

// parseGmailMessage turns a Gmail API message JSON into an IncomingEmail. The
// Gmail message format nests the body as base64url-encoded data inside the
// payload tree; we walk it looking for the first text/plain part.
func parseGmailMessage(raw []byte, maxBodyBytes int) (*IncomingEmail, error) {
	var msg gmailMessage
	if err := json.Unmarshal(raw, &msg); err != nil {
		return nil, fmt.Errorf("parse gmail message: %w", err)
	}
	if msg.ID == "" {
		return nil, nil
	}

	from := headerValue(msg.Payload.Headers, "From")
	subject := headerValue(msg.Payload.Headers, "Subject")
	messageID := headerValue(msg.Payload.Headers, "Message-ID")
	if messageID == "" {
		messageID = msg.ID
	}

	body := extractPlainTextBody(msg.Payload)
	if maxBodyBytes > 0 && len(body) > maxBodyBytes {
		body = body[:maxBodyBytes] + "\n… [truncated]"
	}

	date := time.Now().UTC()
	if msg.InternalDate != "" {
		if ms, err := strconv.ParseInt(msg.InternalDate, 10, 64); err == nil && ms > 0 {
			date = time.Unix(ms/1000, 0).UTC()
		}
	}

	return &IncomingEmail{
		From:      extractEmailAddress(from),
		Subject:   subject,
		Body:      body,
		Date:      date,
		MessageID: messageID,
	}, nil
}

// removeUnreadLabel strips the UNREAD label so the message doesn't show up in
// the next "is:unread" poll. Errors are non-fatal at the caller.
func (r *GmailReceiver) removeUnreadLabel(ctx context.Context, messageID string) error {
	args := []string{
		"gmail", "messages", "modify", messageID,
		"--remove-label", "UNREAD",
	}
	_, err := r.runner.Run(ctx, args...)
	return err
}

// gmailMessage mirrors the relevant fields of the Gmail API Message resource.
type gmailMessage struct {
	ID           string         `json:"id"`
	ThreadID     string         `json:"threadId"`
	InternalDate string         `json:"internalDate"`
	Payload      gmailPayload   `json:"payload"`
	Snippet      string         `json:"snippet"`
	LabelIDs     []string       `json:"labelIds"`
}

type gmailPayload struct {
	MimeType string         `json:"mimeType"`
	Headers  []gmailHeader  `json:"headers"`
	Body     gmailBody      `json:"body"`
	Parts    []gmailPayload `json:"parts"`
}

type gmailHeader struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type gmailBody struct {
	Data string `json:"data"`
	Size int    `json:"size"`
}

func headerValue(headers []gmailHeader, name string) string {
	for _, h := range headers {
		if strings.EqualFold(h.Name, name) {
			return h.Value
		}
	}
	return ""
}

// extractPlainTextBody walks the MIME tree and prefers the first text/plain
// part anywhere in the tree; only if none exists does it fall back to the
// root body. HTML parts are not parsed — a text/plain twin is almost always
// present in support mail.
func extractPlainTextBody(p gmailPayload) string {
	if body := findTextPlain(p); body != "" {
		return body
	}
	if p.Body.Data != "" {
		return decodeGmailBody(p.Body.Data)
	}
	return ""
}

// findTextPlain recursively searches for the first text/plain body in the
// MIME tree, ignoring HTML parts entirely.
func findTextPlain(p gmailPayload) string {
	if strings.HasPrefix(strings.ToLower(p.MimeType), "text/plain") && p.Body.Data != "" {
		return decodeGmailBody(p.Body.Data)
	}
	for _, part := range p.Parts {
		if body := findTextPlain(part); body != "" {
			return body
		}
	}
	return ""
}

// decodeGmailBody decodes a Gmail API body blob (URL-safe base64, no padding).
func decodeGmailBody(data string) string {
	decoded, err := base64.URLEncoding.DecodeString(padBase64(data))
	if err != nil {
		// Some gog builds emit already-decoded text — return as-is.
		return data
	}
	return string(decoded)
}

func padBase64(s string) string {
	if m := len(s) % 4; m != 0 {
		return s + strings.Repeat("=", 4-m)
	}
	return s
}

// extractEmailAddress strips display-name wrapping — "Alice <a@example.com>"
// becomes "a@example.com".
func extractEmailAddress(from string) string {
	from = strings.TrimSpace(from)
	if from == "" {
		return ""
	}
	if start := strings.LastIndex(from, "<"); start >= 0 {
		if end := strings.Index(from[start:], ">"); end > 0 {
			return strings.TrimSpace(from[start+1 : start+end])
		}
	}
	return from
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
