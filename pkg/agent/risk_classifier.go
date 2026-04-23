package agent

import (
	"context"
	"regexp"
	"strings"
)

// RiskLevel expresses how much scrutiny a tool call warrants before execution.
// The approval gate turns this into an auto/approve decision: Low runs
// immediately, Medium/High go through human approval.
type RiskLevel string

const (
	RiskLow     RiskLevel = "low"
	RiskMedium  RiskLevel = "medium"
	RiskHigh    RiskLevel = "high"
	RiskUnknown RiskLevel = "unknown"
)

// ToolCallDescriptor is the minimal surface a classifier needs to judge a call
// without pulling in the full tool invocation machinery. It deliberately omits
// raw user-provided content from the outer context: classifiers that send
// data to LLMs must not receive injected email bodies blindly.
type ToolCallDescriptor struct {
	ToolName string
	// Arguments is the JSON-encoded tool arguments. Classifiers may match
	// against it but should be conservative — hostile input can appear here.
	Arguments string
	// Hints carry structured signals the caller has already extracted
	// (e.g. "sentiment":"negative", "from_domain":"new", "files_changed":"7").
	// Classifiers SHOULD prefer hints over parsing raw Arguments.
	Hints map[string]string
}

// RiskClassifier judges a pending tool call. Implementations should be
// deterministic for a given input when possible so approval decisions remain
// reproducible.
type RiskClassifier interface {
	Classify(ctx context.Context, d ToolCallDescriptor) RiskLevel
}

// defaultAngryHints are substring tokens (case-insensitive) that the
// heuristic classifier treats as elevated-risk signals when they appear in
// either Hints["content"] or Arguments.
var defaultAngryHints = []string{
	"refund", "chargeback", "lawsuit", "attorney", "unacceptable",
	"angry", "furious", "disappointed", "cancel my subscription",
	"terrible", "useless", "never again", "fraud", "scam",
	"återbetalning", "arg", "förbannad", "avsluta", "besviken",
}

// HeuristicClassifier applies cheap regex/substring rules that cover the
// obvious cases (money amounts, escalation keywords, many-file patches)
// without an LLM round-trip. Defaults are safe: ambiguity resolves to Medium
// so approval is requested rather than missed.
type HeuristicClassifier struct {
	amountThreshold float64
	angryHints      []string
	moneyRegex      *regexp.Regexp
}

// NewHeuristicClassifier builds a classifier with configurable thresholds.
// A non-positive amountThreshold uses the default of 100. extraAngry extends
// (does not replace) the built-in list.
func NewHeuristicClassifier(amountThreshold float64, extraAngry []string) *HeuristicClassifier {
	if amountThreshold <= 0 {
		amountThreshold = 100
	}

	hints := make([]string, 0, len(defaultAngryHints)+len(extraAngry))
	hints = append(hints, defaultAngryHints...)
	for _, h := range extraAngry {
		if t := strings.TrimSpace(h); t != "" {
			hints = append(hints, strings.ToLower(t))
		}
	}

	// Match currency-prefixed or suffixed numbers: "$500", "€1,200",
	// "500 USD", "SEK 2500". Captures the numeric portion.
	money := regexp.MustCompile(`(?i)(?:\b(?:usd|eur|sek|nok|gbp|kr|sek)\s*|\$|€|£)\s*([0-9]{1,3}(?:[,.][0-9]{3})*(?:[,.][0-9]{1,2})?|[0-9]+(?:[,.][0-9]{1,2})?)|([0-9]{1,3}(?:[,.][0-9]{3})*(?:[,.][0-9]{1,2})?|[0-9]+(?:[,.][0-9]{1,2})?)\s*(?:usd|eur|sek|nok|gbp|kr)\b`)

	return &HeuristicClassifier{
		amountThreshold: amountThreshold,
		angryHints:      hints,
		moneyRegex:      money,
	}
}

// Classify runs the rule set and returns the highest level any rule triggers.
func (c *HeuristicClassifier) Classify(_ context.Context, d ToolCallDescriptor) RiskLevel {
	text := buildClassifierText(d)
	lower := strings.ToLower(text)

	// Money amounts — numeric compared against threshold.
	if amount := largestAmount(text, c.moneyRegex); amount >= c.amountThreshold {
		return RiskMedium
	}

	// Angry / escalation hints.
	for _, hint := range c.angryHints {
		if strings.Contains(lower, hint) {
			return RiskMedium
		}
	}

	// Broad-scope file changes flagged via hint "files_changed".
	if n := parseIntHint(d.Hints, "files_changed"); n >= 5 {
		return RiskMedium
	}

	// Sentiment hint overrides when classifier caller has already scored.
	switch strings.ToLower(d.Hints["sentiment"]) {
	case "negative", "hostile":
		return RiskMedium
	}

	return RiskLow
}

// buildClassifierText concatenates the inputs the classifier may inspect. It
// avoids emitting nil/empty fields to keep regex matches clean.
func buildClassifierText(d ToolCallDescriptor) string {
	var b strings.Builder
	if s, ok := d.Hints["content"]; ok && s != "" {
		b.WriteString(s)
		b.WriteByte(' ')
	}
	if s, ok := d.Hints["subject"]; ok && s != "" {
		b.WriteString(s)
		b.WriteByte(' ')
	}
	if d.Arguments != "" {
		b.WriteString(d.Arguments)
	}
	return b.String()
}

// largestAmount finds every currency expression in text and returns the
// largest numeric value, or 0 if none parse.
func largestAmount(text string, re *regexp.Regexp) float64 {
	if re == nil {
		return 0
	}
	var best float64
	for _, m := range re.FindAllStringSubmatch(text, -1) {
		raw := strings.TrimSpace(m[1])
		if raw == "" && len(m) > 2 {
			raw = strings.TrimSpace(m[2])
		}
		if raw == "" {
			continue
		}
		v := parseDecimal(raw)
		if v > best {
			best = v
		}
	}
	return best
}

// parseDecimal tolerates both US ("1,000.50") and European ("1.000,50")
// notation. When only one separator is present, a run of exactly 3 digits
// after it is treated as a thousands group rather than a decimal fraction.
func parseDecimal(s string) float64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}

	lastDot := strings.LastIndex(s, ".")
	lastComma := strings.LastIndex(s, ",")

	switch {
	case lastDot == -1 && lastComma == -1:
		// pure digits
	case lastDot > -1 && lastComma > -1:
		// Both present — the rightmost one is the decimal separator.
		if lastDot > lastComma {
			s = strings.ReplaceAll(s, ",", "")
		} else {
			s = strings.ReplaceAll(s, ".", "")
			s = strings.Replace(s, ",", ".", 1)
		}
	case lastDot > -1:
		// Only '.' — if followed by exactly 3 digits and it's the only dot,
		// treat it as a thousands group.
		if strings.Count(s, ".") == 1 && len(s)-lastDot-1 == 3 {
			s = strings.ReplaceAll(s, ".", "")
		}
	case lastComma > -1:
		// Only ',' — same heuristic. Otherwise interpret comma as decimal.
		if strings.Count(s, ",") == 1 && len(s)-lastComma-1 == 3 {
			s = strings.ReplaceAll(s, ",", "")
		} else {
			s = strings.Replace(s, ",", ".", 1)
		}
	}

	var value float64
	var frac float64 = 1
	inFrac := false
	for _, r := range s {
		switch {
		case r == '.':
			inFrac = true
		case r >= '0' && r <= '9':
			if inFrac {
				frac *= 10
				value += float64(r-'0') / frac
			} else {
				value = value*10 + float64(r-'0')
			}
		}
	}
	return value
}

func parseIntHint(hints map[string]string, key string) int {
	if hints == nil {
		return 0
	}
	v, ok := hints[key]
	if !ok {
		return 0
	}
	n := 0
	for _, r := range strings.TrimSpace(v) {
		if r < '0' || r > '9' {
			return 0
		}
		n = n*10 + int(r-'0')
	}
	return n
}
