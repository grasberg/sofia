package channels

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// GmailArchiver marks a processed message as read (and optionally labels it)
// via the `gog gmail messages modify` command. It implements the narrow
// EmailArchiver contract the workflows package expects without taking a
// dependency on that package.
type GmailArchiver struct {
	runner    gogRunner
	addLabel  string // e.g. "handled" — appended with --add-label; empty skips
	markRead  bool   // when true, removes the UNREAD label
}

// GmailArchiverOptions bundles construction parameters for clarity.
type GmailArchiverOptions struct {
	BinaryPath string
	Account    string
	AddLabel   string // optional label applied after processing
	MarkRead   bool
	TimeoutSec int
}

// NewGmailArchiver builds an archiver that shells out to `gog`. Reuses the
// same `gog` invocation style as GmailSender/GmailReceiver.
func NewGmailArchiver(opts GmailArchiverOptions) *GmailArchiver {
	binary := strings.TrimSpace(opts.BinaryPath)
	if binary == "" {
		binary = "gog"
	}
	if strings.ContainsAny(binary, "/\\") {
		// Match the security guard used by GmailSender/GmailReceiver.
		binary = "gog"
	}
	timeoutSec := opts.TimeoutSec
	if timeoutSec <= 0 {
		timeoutSec = 30
	}
	runner := &execRunner{
		binary:  binary,
		account: opts.Account,
		timeout: time.Duration(timeoutSec) * time.Second,
	}
	return newGmailArchiver(runner, opts)
}

func newGmailArchiver(runner gogRunner, opts GmailArchiverOptions) *GmailArchiver {
	return &GmailArchiver{
		runner:   runner,
		addLabel: strings.TrimSpace(opts.AddLabel),
		markRead: opts.MarkRead,
	}
}

// Archive removes UNREAD and/or adds the configured label. When neither
// option is set Archive is a silent no-op.
func (a *GmailArchiver) Archive(ctx context.Context, messageID string) error {
	if a == nil || messageID == "" {
		return nil
	}
	if !a.markRead && a.addLabel == "" {
		return nil
	}

	args := []string{"gmail", "messages", "modify", messageID}
	if a.markRead {
		args = append(args, "--remove-label", "UNREAD")
	}
	if a.addLabel != "" {
		args = append(args, "--add-label", a.addLabel)
	}

	if _, err := a.runner.Run(ctx, args...); err != nil {
		return fmt.Errorf("gog archive %s: %w", messageID, err)
	}
	return nil
}
