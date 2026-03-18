package guardrails

import "regexp"

// SecretPattern defines a pattern for detecting secrets in text.
type SecretPattern struct {
	Name    string
	Pattern *regexp.Regexp
	Mask    string // replacement text; defaults to "[REDACTED:<Name>]" if empty
}

var defaultSecretPatterns = []SecretPattern{
	{
		Name:    "AWS Access Key",
		Pattern: regexp.MustCompile(`AKIA[0-9A-Z]{16}`),
		Mask:    "[REDACTED:AWS_KEY]",
	},
	{
		Name:    "AWS Secret Key",
		Pattern: regexp.MustCompile(`(?i)aws.{0,20}secret.{0,20}['"]([0-9a-zA-Z/+]{40})['"]`),
		Mask:    "[REDACTED:AWS_SECRET]",
	},
	{
		Name:    "Generic API Key",
		Pattern: regexp.MustCompile(`(?i)(api[_-]?key|apikey|api_secret)\s*[:=]\s*['"]?([a-zA-Z0-9_\-]{20,})['"]?`),
		Mask:    "[REDACTED:API_KEY]",
	},
	{
		Name:    "Bearer Token",
		Pattern: regexp.MustCompile(`(?i)bearer\s+[a-zA-Z0-9_\-\.]{20,}`),
		Mask:    "[REDACTED:BEARER_TOKEN]",
	},
	{
		Name:    "GitHub Token",
		Pattern: regexp.MustCompile(`gh[ps]_[A-Za-z0-9_]{36,}`),
		Mask:    "[REDACTED:GITHUB_TOKEN]",
	},
	{
		Name:    "GitLab Token",
		Pattern: regexp.MustCompile(`glpat-[A-Za-z0-9\-]{20,}`),
		Mask:    "[REDACTED:GITLAB_TOKEN]",
	},
	{
		Name:    "Slack Token",
		Pattern: regexp.MustCompile(`xox[bprs]-[0-9A-Za-z\-]{10,}`),
		Mask:    "[REDACTED:SLACK_TOKEN]",
	},
	{
		Name:    "Private Key",
		Pattern: regexp.MustCompile(`-----BEGIN (RSA |EC |DSA |OPENSSH )?PRIVATE KEY-----`),
		Mask:    "[REDACTED:PRIVATE_KEY]",
	},
	{
		Name: "JWT Token",
		Pattern: regexp.MustCompile(
			`eyJ[A-Za-z0-9_-]{10,}\.eyJ[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}`,
		),
		Mask: "[REDACTED:JWT]",
	},
	{
		Name:    "Password in URL",
		Pattern: regexp.MustCompile(`://([^:]+):([^@]{8,})@`),
		Mask:    "[REDACTED:URL_PASSWORD]",
	},
	{
		Name: "Hex Secret (32+)",
		Pattern: regexp.MustCompile(
			`(?i)(secret|password|token|key)\s*[:=]\s*['"]?([0-9a-f]{32,})['"]?`,
		),
		Mask: "[REDACTED:HEX_SECRET]",
	},
}

// ScrubSecrets applies all default secret patterns to text, returning the
// scrubbed string and a list of matched secret type names.
func ScrubSecrets(text string) (string, []string) {
	return ScrubSecretsWithPatterns(text, defaultSecretPatterns)
}

// ScrubSecretsWithPatterns applies the given secret patterns to text, returning
// the scrubbed string and a list of matched secret type names.
func ScrubSecretsWithPatterns(text string, patterns []SecretPattern) (string, []string) {
	scrubbed := text
	var found []string

	for _, sp := range patterns {
		if sp.Pattern.MatchString(scrubbed) {
			found = append(found, sp.Name)
			mask := sp.Mask
			if mask == "" {
				mask = "[REDACTED]"
			}
			scrubbed = sp.Pattern.ReplaceAllString(scrubbed, mask)
		}
	}

	return scrubbed, found
}

// ContainsSecrets returns true if the text matches any default secret pattern.
func ContainsSecrets(text string) bool {
	for _, sp := range defaultSecretPatterns {
		if sp.Pattern.MatchString(text) {
			return true
		}
	}
	return false
}
