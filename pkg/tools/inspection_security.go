package tools

import (
	"fmt"
	"regexp"
	"strings"
)

// threatPattern holds a compiled regex together with its category metadata.
type threatPattern struct {
	Pattern    *regexp.Regexp
	Category   string
	RiskLevel  string
	Confidence float64
}

// securityInspectorToolNames is the set of tool names the SecurityInspector cares about.
var securityInspectorToolNames = map[string]bool{
	"exec":  true,
	"shell": true,
}

// SecurityInspector performs pattern-based threat detection on shell/exec tool calls.
type SecurityInspector struct {
	patterns []threatPattern
}

// NewSecurityInspector constructs a SecurityInspector pre-loaded with threat patterns.
func NewSecurityInspector() *SecurityInspector {
	return &SecurityInspector{patterns: defaultThreatPatterns()}
}

func (s *SecurityInspector) Name() string { return "security" }

func (s *SecurityInspector) Inspect(toolName string, args map[string]any, argsJSON string) *InspectionVerdict {
	// Only inspect exec/shell tools or tools whose args contain a "command" field.
	if !securityInspectorToolNames[toolName] {
		if _, hasCmd := args["command"]; !hasCmd {
			return allowedVerdict(s.Name())
		}
	}

	// Build the text to scan: prefer the "command" arg, fall back to argsJSON.
	text := argsJSON
	if cmd, ok := args["command"].(string); ok && cmd != "" {
		text = cmd
	}
	lower := strings.ToLower(text)

	for _, tp := range s.patterns {
		if tp.Pattern.MatchString(lower) {
			return blockedVerdict(
				s.Name(),
				fmt.Sprintf("%s: pattern %q matched", tp.Category, tp.Pattern.String()),
				tp.RiskLevel,
				tp.Confidence,
			)
		}
	}
	return allowedVerdict(s.Name())
}

// defaultThreatPatterns returns the built-in set of 30+ patterns across 8 categories.
func defaultThreatPatterns() []threatPattern {
	p := func(expr, category, risk string, conf float64) threatPattern {
		return threatPattern{
			Pattern:    regexp.MustCompile(expr),
			Category:   category,
			RiskLevel:  risk,
			Confidence: conf,
		}
	}

	return []threatPattern{
		// --- FileSystemDestruction ---
		p(`rm\s+-rf\s+/\s*$`, "FileSystemDestruction", "critical", 0.99),
		p(`rm\s+-rf\s+/\b`, "FileSystemDestruction", "critical", 0.99),
		p(`rm\s+-rf\s+~`, "FileSystemDestruction", "critical", 0.95),
		p(`rm\s+-rf\s+\*`, "FileSystemDestruction", "high", 0.90),
		p(`\bdd\s+if=`, "FileSystemDestruction", "high", 0.85),
		p(`\bmkfs\b`, "FileSystemDestruction", "critical", 0.95),
		p(`\bformat\s+[a-z]:`, "FileSystemDestruction", "critical", 0.90),

		// --- RemoteCodeExecution ---
		p(`curl\s.*\|\s*(sh|bash|zsh)`, "RemoteCodeExecution", "critical", 0.98),
		p(`wget\s.*\|\s*(sh|bash|zsh)`, "RemoteCodeExecution", "critical", 0.98),
		p(`curl\s.*\|\s*python`, "RemoteCodeExecution", "critical", 0.95),
		p(`wget\s.*\|\s*python`, "RemoteCodeExecution", "critical", 0.95),
		p(`python\s+-c\s+.*urllib`, "RemoteCodeExecution", "high", 0.80),
		p(`python\s+-c\s+.*exec\(`, "RemoteCodeExecution", "high", 0.85),

		// --- DataExfiltration ---
		p(`curl\s.*--data|curl\s.*-d\s|curl\s.*-X\s*POST`, "DataExfiltration", "high", 0.85),
		p(`\bnc\s+-l`, "DataExfiltration", "high", 0.80),
		p(`bash\s+-i\s+>&\s*/dev/tcp/`, "DataExfiltration", "critical", 0.98),
		p(`python\s+-c\s+.*socket\.`, "DataExfiltration", "high", 0.85),
		p(`perl\s+-e\s+.*socket`, "DataExfiltration", "high", 0.85),
		p(`\bnc\s+.*-e\s+/bin/(sh|bash)`, "DataExfiltration", "critical", 0.98),
		p(`\bncat\s+.*-e\s+/bin/(sh|bash)`, "DataExfiltration", "critical", 0.98),

		// --- SystemModification ---
		p(`\bcrontab\b`, "SystemModification", "medium", 0.70),
		p(`chmod\s+777`, "SystemModification", "high", 0.85),
		p(`chown\s+root`, "SystemModification", "high", 0.80),
		p(`\binsmod\b`, "SystemModification", "critical", 0.90),
		p(`\bmodprobe\b`, "SystemModification", "critical", 0.90),

		// --- NetworkAccess ---
		p(`>>\s*~?/?\.ssh/authorized_keys`, "NetworkAccess", "critical", 0.95),
		p(`/etc/hosts`, "NetworkAccess", "high", 0.75),

		// --- ProcessManipulation ---
		p(`kill\s+-9\s+-1`, "ProcessManipulation", "critical", 0.95),
		p(`:\(\)\{\s*:\|:&\s*\};:`, "ProcessManipulation", "critical", 0.99),
		p(`\.\(\)\s*\{\s*\.\|\.\&\s*\}\s*;`, "ProcessManipulation", "critical", 0.95),

		// --- PrivilegeEscalation ---
		p(`\bsudo\s`, "PrivilegeEscalation", "high", 0.80),
		p(`\bsu\s+-\b`, "PrivilegeEscalation", "high", 0.85),
		p(`\bpasswd\b`, "PrivilegeEscalation", "medium", 0.70),
		p(`/etc/shadow`, "PrivilegeEscalation", "critical", 0.95),

		// --- CommandInjection ---
		p("`[^`]+`", "CommandInjection", "medium", 0.65),
		p(`\$\([^)]+\)`, "CommandInjection", "medium", 0.60),
	}
}
