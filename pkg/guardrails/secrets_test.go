package guardrails

import (
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestScrubSecrets_AWSKey(t *testing.T) {
	input := "Here is my key: AKIAIOSFODNN7EXAMPLE and some text"
	scrubbed, found := ScrubSecrets(input)

	assert.Contains(t, found, "AWS Access Key")
	assert.NotContains(t, scrubbed, "AKIAIOSFODNN7EXAMPLE")
	assert.Contains(t, scrubbed, "[REDACTED:AWS_KEY]")
}

func TestScrubSecrets_GitHubToken(t *testing.T) {
	token := "ghp_ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmn"
	input := "Use this token: " + token
	scrubbed, found := ScrubSecrets(input)

	assert.Contains(t, found, "GitHub Token")
	assert.NotContains(t, scrubbed, token)
	assert.Contains(t, scrubbed, "[REDACTED:GITHUB_TOKEN]")
}

func TestScrubSecrets_BearerToken(t *testing.T) {
	input := "Authorization: Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9abcdef"
	scrubbed, found := ScrubSecrets(input)

	assert.Contains(t, found, "Bearer Token")
	assert.Contains(t, scrubbed, "[REDACTED:BEARER_TOKEN]")
}

func TestScrubSecrets_JWT(t *testing.T) {
	jwt := "eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.dozjgNryP4J3jVmNHl0w5N_XgL0n3I9PlFUP0THsR8U"
	input := "Token: " + jwt
	scrubbed, found := ScrubSecrets(input)

	assert.Contains(t, found, "JWT Token")
	assert.NotContains(t, scrubbed, jwt)
	assert.Contains(t, scrubbed, "[REDACTED:JWT]")
}

func TestScrubSecrets_PrivateKey(t *testing.T) {
	input := "-----BEGIN RSA PRIVATE KEY-----\nMIIEowIBAAKCAQEA..."
	scrubbed, found := ScrubSecrets(input)

	assert.Contains(t, found, "Private Key")
	assert.NotContains(t, scrubbed, "-----BEGIN RSA PRIVATE KEY-----")
	assert.Contains(t, scrubbed, "[REDACTED:PRIVATE_KEY]")
}

func TestScrubSecrets_PasswordInURL(t *testing.T) {
	input := "Database: postgres://admin:supersecretpassword@db.example.com:5432/mydb"
	scrubbed, found := ScrubSecrets(input)

	assert.Contains(t, found, "Password in URL")
	assert.NotContains(t, scrubbed, "supersecretpassword")
	assert.Contains(t, scrubbed, "[REDACTED:URL_PASSWORD]")
}

func TestScrubSecrets_NoSecrets(t *testing.T) {
	input := "This is just a normal message with no secrets whatsoever."
	scrubbed, found := ScrubSecrets(input)

	assert.Empty(t, found)
	assert.Equal(t, input, scrubbed)
}

func TestScrubSecrets_MultipleSecrets(t *testing.T) {
	input := "Key: AKIAIOSFODNN7EXAMPLE, " +
		"token: ghp_ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmn, " +
		"db: postgres://user:longpassword123@host:5432/db"
	scrubbed, found := ScrubSecrets(input)

	require.Len(t, found, 3)
	assert.Contains(t, found, "AWS Access Key")
	assert.Contains(t, found, "GitHub Token")
	assert.Contains(t, found, "Password in URL")

	assert.NotContains(t, scrubbed, "AKIAIOSFODNN7EXAMPLE")
	assert.NotContains(t, scrubbed, "ghp_ABCDEFGHIJKLMNOPQRSTUVWXYZ")
	assert.NotContains(t, scrubbed, "longpassword123")
}

func TestContainsSecrets(t *testing.T) {
	assert.True(t, ContainsSecrets("key: AKIAIOSFODNN7EXAMPLE"))
	assert.True(t, ContainsSecrets("ghp_ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmn"))
	assert.True(t, ContainsSecrets("-----BEGIN PRIVATE KEY-----"))
	assert.False(t, ContainsSecrets("hello world, nothing secret here"))
}

func TestScrubSecrets_SlackToken(t *testing.T) {
	input := "Slack bot token: xoxb-1234567890-abcdefghij"
	scrubbed, found := ScrubSecrets(input)

	assert.Contains(t, found, "Slack Token")
	assert.NotContains(t, scrubbed, "xoxb-1234567890-abcdefghij")
	assert.Contains(t, scrubbed, "[REDACTED:SLACK_TOKEN]")
}

func TestScrubSecrets_GitLabToken(t *testing.T) {
	input := "glpat-abcdefghij1234567890AB"
	scrubbed, found := ScrubSecrets(input)

	assert.Contains(t, found, "GitLab Token")
	assert.NotContains(t, scrubbed, "glpat-abcdefghij1234567890AB")
	assert.Contains(t, scrubbed, "[REDACTED:GITLAB_TOKEN]")
}

func TestScrubSecrets_GenericAPIKey(t *testing.T) {
	input := `api_key = "SAMPLE_API_KEY_1234567890abcdef"`
	scrubbed, found := ScrubSecrets(input)

	assert.Contains(t, found, "Generic API Key")
	assert.Contains(t, scrubbed, "[REDACTED:API_KEY]")
}

func TestScrubSecrets_HexSecret(t *testing.T) {
	input := `secret = "aabbccddee00112233445566778899aabbccddee00112233445566778899aabb"`
	scrubbed, found := ScrubSecrets(input)

	assert.Contains(t, found, "Hex Secret (32+)")
	assert.Contains(t, scrubbed, "[REDACTED:HEX_SECRET]")
}

func TestScrubSecretsWithPatterns_Custom(t *testing.T) {
	custom := []SecretPattern{
		{
			Name:    "Custom Secret",
			Pattern: regexp.MustCompile(`CUSTOM_[A-Z]{10}`),
			Mask:    "[REDACTED:CUSTOM]",
		},
	}

	input := "my secret: CUSTOM_ABCDEFGHIJ"
	scrubbed, found := ScrubSecretsWithPatterns(input, custom)

	assert.Contains(t, found, "Custom Secret")
	assert.NotContains(t, scrubbed, "CUSTOM_ABCDEFGHIJ")
	assert.Contains(t, scrubbed, "[REDACTED:CUSTOM]")
}

func TestScrubSecretsWithPatterns_EmptyMaskFallback(t *testing.T) {
	custom := []SecretPattern{
		{
			Name:    "No Mask",
			Pattern: regexp.MustCompile(`SECRET_[0-9]{8}`),
			// Mask intentionally left empty
		},
	}

	input := "value: SECRET_12345678"
	scrubbed, found := ScrubSecretsWithPatterns(input, custom)

	assert.Contains(t, found, "No Mask")
	assert.Contains(t, scrubbed, "[REDACTED]")
	assert.NotContains(t, scrubbed, "SECRET_12345678")
}
