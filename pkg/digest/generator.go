package digest

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/grasberg/sofia/pkg/memory"
)

// DigestConfig configures digest generation.
type DigestConfig struct {
	Period        string `json:"period"`           // "daily", "weekly"
	Channel       string `json:"channel"`          // target channel for delivery
	ChatID        string `json:"chat_id"`          // target chat
	AgentID       string `json:"agent_id"`         // which agent generates
	IncludeMemory bool   `json:"include_memory"`
	IncludeUsage  bool   `json:"include_usage"`
}

// DigestGenerator compiles periodic summary reports.
type DigestGenerator struct {
	memDB *memory.MemoryDB
}

// NewDigestGenerator creates a DigestGenerator using the given MemoryDB.
func NewDigestGenerator(memDB *memory.MemoryDB) *DigestGenerator {
	return &DigestGenerator{memDB: memDB}
}

// Generate creates a digest prompt for the given time range.
// The prompt is designed to be sent to an LLM agent which will produce the actual digest.
func (dg *DigestGenerator) Generate(
	ctx context.Context,
	since, until time.Time,
	opts DigestConfig,
) (string, error) {
	sessions, err := dg.memDB.ListSessions()
	if err != nil {
		return "", fmt.Errorf("digest: list sessions: %w", err)
	}

	// Filter sessions that overlap the requested time range.
	var matched []memory.SessionRow
	for _, s := range sessions {
		if s.UpdatedAt.Before(since) || s.CreatedAt.After(until) {
			continue
		}
		matched = append(matched, s)
	}

	// Count messages and tool calls across matching sessions.
	totalMessages := 0
	totalToolCalls := 0
	topics := make([]string, 0, len(matched))

	for _, s := range matched {
		totalMessages += s.MsgCount

		msgs, msgErr := dg.memDB.GetMessages(s.Key)
		if msgErr != nil {
			continue
		}
		for _, m := range msgs {
			totalToolCalls += len(m.ToolCalls)
		}

		// Extract a topic hint from the session summary or preview.
		topic := s.Summary
		if topic == "" {
			topic = s.Preview
		}
		if topic != "" {
			if len(topic) > 60 {
				topic = topic[:60]
			}
			topics = append(topics, topic)
		}
	}

	// Collect memory notes updated in the range.
	var noteLines []string
	if opts.IncludeMemory {
		notes, notesErr := dg.memDB.ListNotes()
		if notesErr == nil {
			for _, n := range notes {
				if n.UpdatedAt.Before(since) || n.UpdatedAt.After(until) {
					continue
				}
				noteLines = append(noteLines, fmt.Sprintf("- [%s/%s] %s", n.Kind, n.DateKey, n.Content))
			}
		}
	}

	// If there was no activity at all, return a short no-activity prompt.
	if len(matched) == 0 && len(noteLines) == 0 {
		return fmt.Sprintf(
			"Generate a brief digest report for the period %s to %s.\n\nNo activity was recorded during this period.",
			since.Format(time.RFC3339),
			until.Format(time.RFC3339),
		), nil
	}

	// Build the structured prompt.
	var b strings.Builder

	fmt.Fprintf(&b, "Generate a concise digest report for the period %s to %s.\n\n",
		since.Format(time.RFC3339), until.Format(time.RFC3339))

	b.WriteString("Activity summary:\n")
	fmt.Fprintf(&b, "- %d conversations across %d sessions\n", totalMessages, len(matched))
	fmt.Fprintf(&b, "- %d tool calls executed\n", totalToolCalls)

	if len(topics) > 0 {
		fmt.Fprintf(&b, "- Topics discussed: %s\n", strings.Join(topics, "; "))
	}

	if len(noteLines) > 0 {
		b.WriteString("\nRecent memory notes:\n")
		for _, line := range noteLines {
			b.WriteString(line)
			b.WriteByte('\n')
		}
	}

	b.WriteString("\nPlease provide:\n")
	b.WriteString("1. A brief overview of activity\n")
	b.WriteString("2. Key topics and decisions\n")
	b.WriteString("3. Any outstanding items or follow-ups\n")
	b.WriteString("4. Notable patterns or trends\n")

	return b.String(), nil
}
