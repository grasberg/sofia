package workflows

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/grasberg/sofia/pkg/memory"
)

// HeuristicTriager classifies inbound mail by keyword heuristics only — no
// LLM call. It's the default Triager so the workflow works with zero LLM
// configuration. Production deployments can swap in an LLM-backed Triager
// when richer classification is needed.
type HeuristicTriager struct {
	urgentKeywords   []string // drive P1/P2
	complaintHints   []string // drive sentiment
	urgentRegex      *regexp.Regexp
	complaintRegex   *regexp.Regexp
}

// NewHeuristicTriager builds a Triager with the default keyword lists.
// extraUrgent / extraComplaint are appended (case insensitive).
func NewHeuristicTriager(extraUrgent, extraComplaint []string) *HeuristicTriager {
	urgent := append([]string{
		"urgent", "asap", "emergency", "outage", "down", "cannot log in",
		"can't log in", "locked out", "production down", "data loss",
		"akut", "brådskande", "nere", "kritiskt", "funkar inte alls",
	}, extraUrgent...)

	complaint := append([]string{
		"refund", "chargeback", "lawsuit", "attorney", "angry", "furious",
		"unacceptable", "terrible", "useless", "fraud", "scam",
		"återbetalning", "arg", "förbannad", "besviken", "bluff",
	}, extraComplaint...)

	return &HeuristicTriager{
		urgentKeywords: urgent,
		complaintHints: complaint,
		urgentRegex:    compileWordBoundary(urgent),
		complaintRegex: compileWordBoundary(complaint),
	}
}

// Triage applies keyword rules to derive priority + sentiment. Summary is a
// trimmed first-sentence snippet suitable for a goal description.
func (t *HeuristicTriager) Triage(_ context.Context, subject, body string) (TriageResult, error) {
	combined := strings.ToLower(subject + "\n" + body)

	priority := PriorityP3
	sentiment := SentimentNeutral

	if t.urgentRegex != nil && t.urgentRegex.MatchString(combined) {
		priority = PriorityP2
	}
	// P1: urgent AND negative wording doubles the lift.
	if t.complaintRegex != nil && t.complaintRegex.MatchString(combined) {
		sentiment = SentimentNegative
		if priority == PriorityP2 {
			priority = PriorityP1
		}
	}
	if strings.Contains(combined, "thank") || strings.Contains(combined, "tack") {
		if sentiment == SentimentNeutral {
			sentiment = SentimentPositive
		}
	}

	return TriageResult{
		Priority:  priority,
		Sentiment: sentiment,
		Summary:   firstSentence(body, 160),
	}, nil
}

// TemplateDrafter composes a reply from KB hits using a simple template so
// the workflow has a working default even without an LLM. The output is
// plain text, locale-aware: Swedish greeting when Locale == "sv", English
// otherwise. Replace with an LLM-backed Drafter for richer responses.
type TemplateDrafter struct{}

// NewTemplateDrafter returns the zero-value default drafter.
func NewTemplateDrafter() *TemplateDrafter { return &TemplateDrafter{} }

// Draft produces a greeting + acknowledgement + KB-sourced answer (or a
// graceful "we'll look into this" note when no KB hit exists).
func (d *TemplateDrafter) Draft(_ context.Context, req DraftRequest) (string, error) {
	var b strings.Builder

	greet, sign, haveHits, noHits := templateStrings(req.Locale)

	firstName := extractFirstName(req.From)
	if firstName != "" {
		fmt.Fprintf(&b, "%s %s,\n\n", greet, firstName)
	} else {
		fmt.Fprintf(&b, "%s,\n\n", greet)
	}

	if len(req.KBHits) > 0 {
		b.WriteString(haveHits)
		b.WriteString("\n\n")
		for i, h := range req.KBHits {
			if i >= 2 {
				break // keep the template focused — LLM drafter can do more
			}
			b.WriteString(strings.TrimSpace(h.Answer))
			b.WriteString("\n\n")
		}
	} else {
		b.WriteString(noHits)
		b.WriteString("\n\n")
	}

	b.WriteString(sign)
	return b.String(), nil
}

// templateStrings returns the localized greeting / ack / closing lines.
func templateStrings(locale string) (greet, sign, haveHits, noHits string) {
	if strings.ToLower(locale) == "sv" {
		return "Hej",
			"Vänliga hälsningar,\nSofia",
			"Tack för ditt meddelande. Här är svaret baserat på hur vi brukar hantera liknande frågor:",
			"Tack för ditt meddelande. Jag återkommer så snart jag har kollat upp det här."
	}
	return "Hi",
		"Best,\nSofia",
		"Thanks for reaching out. Here's the answer based on how we typically handle this:",
		"Thanks for reaching out. I'll look into this and get back to you shortly."
}

// extractFirstName turns "Alice Smith <alice@example.com>" → "Alice". Falls
// back to the local-part of the address when no display name exists.
func extractFirstName(from string) string {
	from = strings.TrimSpace(from)
	if from == "" {
		return ""
	}
	// Display name portion is before the first '<' (if any).
	displayEnd := strings.Index(from, "<")
	var display string
	if displayEnd > 0 {
		display = strings.TrimSpace(from[:displayEnd])
	}
	display = strings.Trim(display, `"`)
	if display != "" {
		return strings.Fields(display)[0]
	}
	// Fall back to local-part — strip quotes/brackets if present.
	email := from
	if displayEnd >= 0 && strings.Contains(from, ">") {
		start := displayEnd + 1
		end := strings.Index(from[start:], ">")
		if end > 0 {
			email = from[start : start+end]
		}
	}
	at := strings.Index(email, "@")
	if at <= 0 {
		return ""
	}
	return email[:at]
}

// firstSentence returns up to maxLen chars of the first sentence. A sentence
// ends at '.', '!', '?', or newline — whichever comes first.
func firstSentence(body string, maxLen int) string {
	trimmed := strings.TrimSpace(body)
	if trimmed == "" {
		return ""
	}
	end := len(trimmed)
	for _, stop := range []string{".", "!", "?", "\n"} {
		if idx := strings.Index(trimmed, stop); idx > 0 && idx < end {
			end = idx
		}
	}
	result := strings.TrimSpace(trimmed[:end])
	if maxLen > 0 && len(result) > maxLen {
		result = result[:maxLen] + "…"
	}
	return result
}

// compileWordBoundary builds a regex that matches any of the keywords with
// Unicode-aware word boundaries. Returns nil when the list is empty.
func compileWordBoundary(keywords []string) *regexp.Regexp {
	cleaned := make([]string, 0, len(keywords))
	for _, k := range keywords {
		k = strings.TrimSpace(strings.ToLower(k))
		if k == "" {
			continue
		}
		cleaned = append(cleaned, regexp.QuoteMeta(k))
	}
	if len(cleaned) == 0 {
		return nil
	}
	// (?i) → case insensitive; \b anchors to ASCII word boundaries which is
	// fine for our keyword set (multi-word phrases are handled via literal
	// match inside the alternation).
	pattern := `(?i)(?:` + strings.Join(cleaned, "|") + `)`
	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil
	}
	return re
}

// kbKeyFor derives a stable key for the inbound message used as the "source"
// field when upserting to the KB. Keeps the format distinct from the raw
// message-id so maintenance scripts can tell them apart.
func kbKeyFor(messageID string) string {
	if messageID == "" {
		return ""
	}
	return "email:" + messageID
}

// kbTagsFrom derives a small set of topic tags from triage + KB hits. The
// heuristic is shallow by design — richer tagging can come from the Drafter
// later.
func kbTagsFrom(tri TriageResult, hits []memory.KBEntry) []string {
	tags := make([]string, 0, 4)
	if tri.Priority != "" {
		tags = append(tags, strings.ToLower(tri.Priority))
	}
	if tri.Sentiment != "" && tri.Sentiment != SentimentNeutral {
		tags = append(tags, strings.ToLower(tri.Sentiment))
	}
	// Propagate tags from the highest-ranked KB hit so reused answers stay
	// grouped.
	if len(hits) > 0 && len(hits[0].Tags) > 0 {
		tags = append(tags, hits[0].Tags[0])
	}
	return tags
}
