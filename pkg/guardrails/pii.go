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
		Type:     PIIIPAddress,
		Pattern:  regexp.MustCompile(`\b(?:\d{1,3}\.){3}\d{1,3}\b`),
		Mask:     "[REDACTED:IP]",
		Validate: isValidIPv4,
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

// isValidIPv4 checks whether a matched dotted-quad string is a plausible IPv4
// address: all octets 0-255, no leading zeros. Rejects version-number patterns
// (all octets < 20) unless the address is in a known private/reserved range.
func isValidIPv4(match string) bool {
	parts := strings.Split(match, ".")
	if len(parts) != 4 {
		return false
	}
	var octets [4]int
	for i, p := range parts {
		// Reject leading zeros (e.g., "01.02.03.04" is likely not an IP).
		if len(p) > 1 && p[0] == '0' {
			return false
		}
		n := 0
		for _, c := range p {
			if c < '0' || c > '9' {
				return false
			}
			n = n*10 + int(c-'0')
		}
		if n > 255 {
			return false
		}
		octets[i] = n
	}

	// Always accept known private/reserved ranges — these are real IPs even
	// when all octets happen to be small (e.g. 10.0.0.1, 10.1.2.3).
	if isPrivateOrReservedIP(octets) {
		return true
	}

	// Reject version-like patterns where all octets are < 20.
	allSmall := true
	for _, n := range octets {
		if n > 19 {
			allSmall = false
			break
		}
	}
	return !allSmall
}

// isPrivateOrReservedIP returns true for RFC1918 private, loopback, and
// link-local addresses that should always be treated as real IPs.
func isPrivateOrReservedIP(o [4]int) bool {
	// 10.0.0.0/8
	if o[0] == 10 {
		return true
	}
	// 172.16.0.0/12
	if o[0] == 172 && o[1] >= 16 && o[1] <= 31 {
		return true
	}
	// 192.168.0.0/16
	if o[0] == 192 && o[1] == 168 {
		return true
	}
	// 127.0.0.0/8 (loopback)
	if o[0] == 127 {
		return true
	}
	// 169.254.0.0/16 (link-local)
	if o[0] == 169 && o[1] == 254 {
		return true
	}
	return false
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
