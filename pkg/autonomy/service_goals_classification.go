package autonomy

import (
	"regexp"
	"runtime"
	"strings"
)

// Patterns that indicate a missing tool or credential rather than a logic error.
// When matched in a failed step's output, the user is notified so they can
// install the tool or add the credential in Settings.
var (
	missingToolPatterns = []string{
		"command not found",
		"not found in PATH",
		"binary missing",
		"no such file or directory",
		"executable file not found",
		"program not found",
		"is not recognized",
	}
	missingCredentialPatterns = []string{
		"unauthorized",
		"authentication failed",
		"invalid api key",
		"invalid token",
		"401",
		"403",
		"forbidden",
		"access denied",
		"no credentials",
		"no api key",
		"permission denied",
		"credential",
		"re-authenticate",
		"oauth token",
	}
	missingNetworkPatterns = []string{
		"no such host",
		"could not resolve host",
		"name or service not known",
		"dns lookup failed",
		"connection refused",
		"network is unreachable",
		"network unreachable",
		"no route to host",
		"connection reset by peer",
		"tls handshake timeout",
		"i/o timeout",
		"dial tcp",
		"connect: connection timed out",
	}
	diskExhaustedPatterns = []string{
		"no space left on device",
		"disk quota exceeded",
		"enospc",
		"out of disk space",
		"write error: no space",
	}
	rateLimitPatterns = []string{
		"rate limit",
		"rate-limited",
		"ratelimited",
		"too many requests",
		"429",
		"quota exceeded",
		"usage limit exceeded",
		"retry-after",
		"throttled",
	}
	// Filesystem/OS-level permission issues — distinct from API credentials.
	// The credential check already catches the ambiguous "permission denied"
	// substring, so these patterns target unambiguous OS signals.
	osPermissionPatterns = []string{
		"eacces",
		"eperm",
		"read-only file system",
		"operation not permitted",
		"requires sudo",
		"must be root",
		"must be run as root",
	}
	missingConfigPatterns = []string{
		"environment variable not set",
		"env var not set",
		"env variable not set",
		"required environment variable",
		"missing required config",
		"required setting",
		"config not found",
		"configuration file not found",
		"configuration key not found",
		"required configuration",
	}
)

// autoInstallMethods maps binary names to per-platform install commands.
// The map is intentionally conservative: only binaries where a single
// well-known command produces a working install. Only macOS (brew) is
// supported initially because it doesn't require elevated privileges for
// installs. Linux/apt requires root and is deferred until a safe story
// for non-interactive sudo lands.
var autoInstallMethods = map[string]map[string]string{
	"jq":          {"darwin": "brew install jq"},
	"rg":          {"darwin": "brew install ripgrep"},
	"ripgrep":     {"darwin": "brew install ripgrep"},
	"fd":          {"darwin": "brew install fd"},
	"bat":         {"darwin": "brew install bat"},
	"gh":          {"darwin": "brew install gh"},
	"tree":        {"darwin": "brew install tree"},
	"wget":        {"darwin": "brew install wget"},
	"yq":          {"darwin": "brew install yq"},
	"terraform":   {"darwin": "brew install hashicorp/tap/terraform"},
	"kubectl":     {"darwin": "brew install kubectl"},
	"helm":        {"darwin": "brew install helm"},
	"node":        {"darwin": "brew install node"},
	"npm":         {"darwin": "brew install node"},
	"python3":     {"darwin": "brew install python"},
	"pip3":        {"darwin": "brew install python"},
	"cargo":       {"darwin": "brew install rust"},
	"rustc":       {"darwin": "brew install rust"},
	"deno":        {"darwin": "brew install deno"},
	"bun":         {"darwin": "brew install bun"},
	"ffmpeg":      {"darwin": "brew install ffmpeg"},
	"imagemagick": {"darwin": "brew install imagemagick"},
	"pandoc":      {"darwin": "brew install pandoc"},
	"sqlite3":     {"darwin": "brew install sqlite"},
	"postgres":    {"darwin": "brew install postgresql"},
	"psql":        {"darwin": "brew install postgresql"},
	"redis-cli":   {"darwin": "brew install redis"},
	"aws":         {"darwin": "brew install awscli"},
	"gcloud":      {"darwin": "brew install --cask google-cloud-sdk"},
}

// classifyStepError checks a failed step's output and returns a category of
// user action needed, or ("", "") for a generic failure the agent should
// retry on its own. Categories are checked from most-specific to least-
// specific so unambiguous OS/network signals win over broader auth matches.
func classifyStepError(result string) (kind, detail string) {
	lower := strings.ToLower(result)
	for _, p := range diskExhaustedPatterns {
		if strings.Contains(lower, p) {
			return "disk", ""
		}
	}
	for _, p := range missingNetworkPatterns {
		if strings.Contains(lower, p) {
			return "network", extractHostHint(result)
		}
	}
	for _, p := range rateLimitPatterns {
		if strings.Contains(lower, p) {
			return "rate_limit", extractCredentialHint(result)
		}
	}
	for _, p := range osPermissionPatterns {
		if strings.Contains(lower, p) {
			return "permission", extractPathHint(result)
		}
	}
	for _, p := range missingToolPatterns {
		if strings.Contains(lower, p) {
			return "tool", extractToolHint(result)
		}
	}
	for _, p := range missingConfigPatterns {
		if strings.Contains(lower, p) {
			return "config", extractConfigHint(result)
		}
	}
	for _, p := range missingCredentialPatterns {
		if strings.Contains(lower, p) {
			return "credential", extractCredentialHint(result)
		}
	}
	return "", ""
}

// Pre-compiled regexes for extracting tool/binary names from error text.
var (
	reShCommandNotFound = regexp.MustCompile(`(?:sh|bash|zsh):\s*(\S+):\s*(?:command )?not found`)
	reExecNotFound      = regexp.MustCompile(`exec:\s*"?(\S+?)"?:\s*executable`)
	// Hostname in messages like `dial tcp: lookup example.com: no such host`
	// or `Get "https://api.example.com/…": dial tcp 1.2.3.4:443: connect: …`.
	reLookupHost = regexp.MustCompile(`lookup\s+([A-Za-z0-9][A-Za-z0-9.\-]*\.[A-Za-z]{2,})`)
	reURLHost    = regexp.MustCompile(`https?://([A-Za-z0-9][A-Za-z0-9.\-]*\.[A-Za-z]{2,})`)
	// Absolute filesystem path after "permission denied:" or similar.
	rePathAfterColon = regexp.MustCompile(`(?:permission denied|operation not permitted|read-only file system)[^/]*?(/[^\s:'"]+)`)
	// Env var / config key names like "FOO_BAR is not set" or
	// "missing required config: FOO_BAR".
	reEnvVarName = regexp.MustCompile(`\b([A-Z][A-Z0-9_]{2,})\b`)
)

// extractToolHint tries to pull the binary name from error text like
// "sh: gog: command not found" or "exec: pip: executable file not found".
func extractToolHint(result string) string {
	if m := reShCommandNotFound.FindStringSubmatch(result); len(m) > 1 {
		return m[1]
	}
	if m := reExecNotFound.FindStringSubmatch(result); len(m) > 1 {
		return m[1]
	}
	return ""
}

// extractCredentialHint tries to identify the service from auth error text.
func extractCredentialHint(result string) string {
	lower := strings.ToLower(result)
	services := map[string]string{
		"gmail":      "Gmail / Google",
		"google":     "Google",
		"openai":     "OpenAI",
		"anthropic":  "Anthropic",
		"openrouter": "OpenRouter",
		"github":     "GitHub",
		"smtp":       "Email (SMTP)",
		"imap":       "Email (IMAP)",
		"docker":     "Docker",
		"ollama.com": "Ollama Cloud",
	}
	for keyword, name := range services {
		if strings.Contains(lower, keyword) {
			return name
		}
	}
	return ""
}

// extractHostHint pulls a hostname out of network error text for display.
func extractHostHint(result string) string {
	if m := reLookupHost.FindStringSubmatch(result); len(m) > 1 {
		return m[1]
	}
	if m := reURLHost.FindStringSubmatch(result); len(m) > 1 {
		return m[1]
	}
	return ""
}

// extractPathHint pulls an absolute filesystem path from permission errors.
func extractPathHint(result string) string {
	if m := rePathAfterColon.FindStringSubmatch(strings.ToLower(result)); len(m) > 1 {
		return m[1]
	}
	return ""
}

// extractConfigHint tries to identify the missing config key (env var name).
func extractConfigHint(result string) string {
	if m := reEnvVarName.FindStringSubmatch(result); len(m) > 1 {
		return m[1]
	}
	return ""
}

// autoInstallCommandFor returns the platform-specific install command for a
// whitelisted binary, or ("", false) if the binary isn't in the map or the
// current platform isn't supported.
func autoInstallCommandFor(binary string) (string, bool) {
	methods, ok := autoInstallMethods[binary]
	if !ok {
		return "", false
	}
	cmd, ok := methods[runtime.GOOS]
	return cmd, ok
}
