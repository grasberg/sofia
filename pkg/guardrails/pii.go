package guardrails

import (
	"regexp"
	"strings"
	"unicode"
)

// PIIType identifies the category of detected PII.
type PIIType string

const (
	PIIEmail      PIIType = "email"
	PIICreditCard PIIType = "credit_card"
	PIIPhoneUS    PIIType = "phone_us"
	PIISSN        PIIType = "ssn"
	PIIIPAddress  PIIType = "ip_address"
	PIIPassport   PIIType = "passport"
)

// PIIMatch represents a single PII detection result.
type PIIMatch struct {
	Type   PIIType `json:"type"`
	Value  string  `json:"value"`  // the matched text (for logging)
	Masked string  `json:"masked"` // redacted version
}

// piiPatternEntry pairs a PII type with a compiled regex and its replacement mask.
type piiPatternEntry struct {
	Type    PIIType
	Pattern *regexp.Regexp
	Mask    string
	// Validate is an optional post-match filter. If non-nil it must return true
	// for the match to be considered a real detection. This lets us add checks
	// such as Luhn validation for credit-card numbers.
	Validate func(match string) bool
}

var piiPatterns = []piiPatternEntry{
	{
		Type:    PIIEmail,
		Pattern: regexp.MustCompile(`[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}`),
		Mask:    "[REDACTED:EMAIL]",
	},
	{
		Type:     PIICreditCard,
		Pattern:  regexp.MustCompile(`\b(?:\d[ -]*?){13,19}\b`),
		Mask:     "[REDACTED:CREDIT_CARD]",
		Validate: func(m string) bool { return isValidLuhn(m) },
	},
	{
		Type:    PIIPhoneUS,
		Pattern: regexp.MustCompile(`\b(?:\+1[-.\s]?)?\(?\d{3}\)?[-.\s]?\d{3}[-.\s]?\d{4}\b`),
		Mask:    "[REDACTED:PHONE]",
	},
	{
		Type:    PIISSN,
		Pattern: regexp.MustCompile(`\b\d{3}-\d{2}-\d{4}\b`),
		Mask:    "[REDACTED:SSN]",
	},
	{
		Type:    PIIIPAddress,
		Pattern: regexp.MustCompile(`\b(?:\d{1,3}\.){3}\d{1,3}\b`),
		Mask:    "[REDACTED:IP]",
	},
}

// isValidLuhn checks whether a numeric string (with optional spaces/dashes)
// passes the Luhn algorithm. Non-digit characters are stripped first.
func isValidLuhn(number string) bool {
	var digits []int
	for _, r := range number {
		if unicode.IsDigit(r) {
			digits = append(digits, int(r-'0'))
		}
	}
	n := len(digits)
	if n < 13 || n > 19 {
		return false
	}

	sum := 0
	alt := false
	for i := n - 1; i >= 0; i-- {
		d := digits[i]
		if alt {
			d *= 2
			if d > 9 {
				d -= 9
			}
		}
		sum += d
		alt = !alt
	}
	return sum%10 == 0
}

// DetectPII scans text for all known PII types and returns the matches found.
func DetectPII(text string) []PIIMatch {
	var matches []PIIMatch
	for _, p := range piiPatterns {
		for _, loc := range p.Pattern.FindAllStringIndex(text, -1) {
			value := text[loc[0]:loc[1]]
			if p.Validate != nil && !p.Validate(value) {
				continue
			}
			matches = append(matches, PIIMatch{
				Type:   p.Type,
				Value:  value,
				Masked: p.Mask,
			})
		}
	}
	return matches
}

// RedactPII replaces all detected PII in text with their masks and returns
// the scrubbed string together with the list of matches.
func RedactPII(text string) (string, []PIIMatch) {
	var allMatches []PIIMatch
	result := text

	for _, p := range piiPatterns {
		if p.Validate == nil {
			// Simple replacement — no per-match validation needed.
			locs := p.Pattern.FindAllStringIndex(result, -1)
			if len(locs) == 0 {
				continue
			}
			var b strings.Builder
			prev := 0
			for _, loc := range locs {
				value := result[loc[0]:loc[1]]
				allMatches = append(allMatches, PIIMatch{
					Type:   p.Type,
					Value:  value,
					Masked: p.Mask,
				})
				b.WriteString(result[prev:loc[0]])
				b.WriteString(p.Mask)
				prev = loc[1]
			}
			b.WriteString(result[prev:])
			result = b.String()
		} else {
			// Need per-match validation (e.g. Luhn for credit cards).
			// Replace valid matches while preserving invalid ones.
			locs := p.Pattern.FindAllStringIndex(result, -1)
			if len(locs) == 0 {
				continue
			}
			var b strings.Builder
			prev := 0
			replaced := false
			for _, loc := range locs {
				value := result[loc[0]:loc[1]]
				if p.Validate(value) {
					allMatches = append(allMatches, PIIMatch{
						Type:   p.Type,
						Value:  value,
						Masked: p.Mask,
					})
					b.WriteString(result[prev:loc[0]])
					b.WriteString(p.Mask)
					prev = loc[1]
					replaced = true
				}
			}
			if replaced {
				b.WriteString(result[prev:])
				result = b.String()
			}
		}
	}
	return result, allMatches
}

// ContainsPII returns true if the text contains any detectable PII.
func ContainsPII(text string) bool {
	for _, p := range piiPatterns {
		for _, loc := range p.Pattern.FindAllStringIndex(text, -1) {
			value := text[loc[0]:loc[1]]
			if p.Validate != nil && !p.Validate(value) {
				continue
			}
			return true
		}
	}
	return false
}
