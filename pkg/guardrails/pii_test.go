package guardrails

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDetectPII_Email(t *testing.T) {
	matches := DetectPII("Contact me at user@example.com for details.")
	require.Len(t, matches, 1)
	assert.Equal(t, PIIEmail, matches[0].Type)
	assert.Equal(t, "user@example.com", matches[0].Value)
	assert.Equal(t, "[REDACTED:EMAIL]", matches[0].Masked)
}

func TestDetectPII_CreditCard(t *testing.T) {
	// 4111 1111 1111 1111 is a well-known test Visa number that passes Luhn.
	matches := DetectPII("My card is 4111 1111 1111 1111 please charge it.")
	require.Len(t, matches, 1)
	assert.Equal(t, PIICreditCard, matches[0].Type)
	assert.Equal(t, "[REDACTED:CREDIT_CARD]", matches[0].Masked)
}

func TestDetectPII_CreditCard_InvalidLuhn(t *testing.T) {
	// 1234 5678 9012 3456 does NOT pass Luhn — should not be flagged.
	matches := DetectPII("Not a card: 1234 5678 9012 3456 really.")
	ccMatches := filterByType(matches, PIICreditCard)
	assert.Empty(t, ccMatches, "number that fails Luhn must not be detected as credit_card")
}

func TestDetectPII_SSN(t *testing.T) {
	matches := DetectPII("SSN: 123-45-6789")
	require.Len(t, matches, 1)
	assert.Equal(t, PIISSN, matches[0].Type)
	assert.Equal(t, "123-45-6789", matches[0].Value)
	assert.Equal(t, "[REDACTED:SSN]", matches[0].Masked)
}

func TestDetectPII_Phone(t *testing.T) {
	matches := DetectPII("Call me at (555) 123-4567 anytime.")
	phoneMatches := filterByType(matches, PIIPhoneUS)
	require.NotEmpty(t, phoneMatches)
	assert.Equal(t, PIIPhoneUS, phoneMatches[0].Type)
	assert.Equal(t, "[REDACTED:PHONE]", phoneMatches[0].Masked)
}

func TestDetectPII_IPAddress(t *testing.T) {
	matches := DetectPII("Server at 192.168.1.100 is down.")
	require.Len(t, matches, 1)
	assert.Equal(t, PIIIPAddress, matches[0].Type)
	assert.Equal(t, "192.168.1.100", matches[0].Value)
	assert.Equal(t, "[REDACTED:IP]", matches[0].Masked)
}

func TestRedactPII(t *testing.T) {
	input := "Email user@example.com, SSN 123-45-6789, IP 10.0.0.200"
	redacted, matches := RedactPII(input)

	assert.NotContains(t, redacted, "user@example.com")
	assert.NotContains(t, redacted, "123-45-6789")
	assert.NotContains(t, redacted, "10.0.0.200")

	assert.Contains(t, redacted, "[REDACTED:EMAIL]")
	assert.Contains(t, redacted, "[REDACTED:SSN]")
	assert.Contains(t, redacted, "[REDACTED:IP]")

	types := matchTypes(matches)
	assert.Contains(t, types, PIIEmail)
	assert.Contains(t, types, PIISSN)
	assert.Contains(t, types, PIIIPAddress)
}

func TestContainsPII(t *testing.T) {
	assert.True(t, ContainsPII("email: test@domain.org"))
	assert.True(t, ContainsPII("SSN 999-88-7777"))
	assert.True(t, ContainsPII("IP: 172.16.0.1"))
	assert.False(t, ContainsPII("just a normal sentence with no PII"))
}

func TestDetectPII_IPAddress_Valid(t *testing.T) {
	// Valid private IP with an octet >= 20 — should be detected.
	matches := DetectPII("Server at 192.168.1.1 is responding.")
	ipMatches := filterByType(matches, PIIIPAddress)
	require.Len(t, ipMatches, 1)
	assert.Equal(t, "192.168.1.1", ipMatches[0].Value)
}

func TestDetectPII_IPAddress_VersionLike(t *testing.T) {
	// All octets < 20 — looks like a version number, should NOT be detected.
	matches := DetectPII("Using library version 1.2.3.4 now.")
	ipMatches := filterByType(matches, PIIIPAddress)
	assert.Empty(t, ipMatches, "version-like pattern 1.2.3.4 must not be detected as IP")
}

func TestDetectPII_IPAddress_InvalidOctets(t *testing.T) {
	// Octets exceed 255 — should NOT be detected.
	matches := DetectPII("Value is 999.999.999.999 which is invalid.")
	ipMatches := filterByType(matches, PIIIPAddress)
	assert.Empty(t, ipMatches, "999.999.999.999 has invalid octets and must not be detected")
}

func TestDetectPII_IPAddress_PrivateValid(t *testing.T) {
	// Private IPs with all-small octets should now be detected because
	// RFC1918/loopback ranges are whitelisted from the "allSmall" heuristic.
	tests := []struct {
		name  string
		input string
		ip    string
	}{
		{"10.0.0.1", "Route via 10.0.0.1 is active.", "10.0.0.1"},
		{"10.0.0.200", "Route via 10.0.0.200 is active.", "10.0.0.200"},
		{"10.1.2.3", "Server at 10.1.2.3 responds.", "10.1.2.3"},
		{"127.0.0.1", "Listening on 127.0.0.1 port 8080.", "127.0.0.1"},
		{"192.168.1.1", "Gateway 192.168.1.1 unreachable.", "192.168.1.1"},
		{"172.16.0.1", "Subnet 172.16.0.1 allocated.", "172.16.0.1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matches := DetectPII(tt.input)
			ipMatches := filterByType(matches, PIIIPAddress)
			require.Len(t, ipMatches, 1, "expected %s to be detected", tt.ip)
			assert.Equal(t, tt.ip, ipMatches[0].Value)
		})
	}
}

func TestDetectPII_NoPII(t *testing.T) {
	matches := DetectPII("Hello, this is a perfectly clean message with no personal data.")
	assert.Empty(t, matches)
}

func TestLuhnValidation(t *testing.T) {
	tests := []struct {
		name  string
		input string
		valid bool
	}{
		{"Visa test number", "4111111111111111", true},
		{"Visa with spaces", "4111 1111 1111 1111", true},
		{"Visa with dashes", "4111-1111-1111-1111", true},
		{"MasterCard test", "5500000000000004", true},
		{"Amex test", "378282246310005", true},
		{"Invalid sequence", "1234567890123456", false},
		{"All zeros 16 digits", "0000000000000000", true}, // Luhn checksum is 0 mod 10
		{"Too short", "12345", false},
		{"Too long", "12345678901234567890", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.valid, isValidLuhn(tt.input))
		})
	}
}

// ---------- helpers ----------

func filterByType(matches []PIIMatch, t PIIType) []PIIMatch {
	var out []PIIMatch
	for _, m := range matches {
		if m.Type == t {
			out = append(out, m)
		}
	}
	return out
}

func matchTypes(matches []PIIMatch) []PIIType {
	var out []PIIType
	for _, m := range matches {
		out = append(out, m.Type)
	}
	return out
}
