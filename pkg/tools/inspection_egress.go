package tools

import (
	"fmt"
	"regexp"
	"strings"
)

// egressPattern pairs a regex with a human-readable label.
type egressPattern struct {
	Pattern *regexp.Regexp
	Label   string
}

// EgressInspector detects outbound network traffic in shell commands.
type EgressInspector struct {
	patterns []egressPattern
}

// NewEgressInspector constructs an EgressInspector with default patterns.
func NewEgressInspector() *EgressInspector {
	return &EgressInspector{patterns: defaultEgressPatterns()}
}

func (e *EgressInspector) Name() string { return "egress" }

func (e *EgressInspector) Inspect(toolName string, args map[string]any, argsJSON string) *InspectionVerdict {
	text := argsJSON
	if cmd, ok := args["command"].(string); ok && cmd != "" {
		text = cmd
	}
	lower := strings.ToLower(text)

	for _, ep := range e.patterns {
		matches := ep.Pattern.FindAllString(lower, -1)
		if len(matches) > 0 {
			dest := extractDestinations(lower)
			reason := fmt.Sprintf("outbound %s detected", ep.Label)
			if dest != "" {
				reason += fmt.Sprintf(" (destinations: %s)", dest)
			}
			return blockedVerdict(e.Name(), reason, "high", 0.85)
		}
	}
	return allowedVerdict(e.Name())
}

func defaultEgressPatterns() []egressPattern {
	ep := func(expr, label string) egressPattern {
		return egressPattern{
			Pattern: regexp.MustCompile(expr),
			Label:   label,
		}
	}
	return []egressPattern{
		ep(`https?://`, "URL"),
		ep(`ftp://`, "FTP"),
		ep(`\bgit\s+(push|clone)\b`, "git remote operation"),
		ep(`\bs3\s+(cp|sync|mv)\b`, "S3 operation"),
		ep(`\bgsutil\s+(cp|rsync|mv)\b`, "GCS operation"),
		ep(`\bscp\s`, "SCP transfer"),
		ep(`\bssh\s`, "SSH connection"),
		ep(`\bdocker\s+push\b`, "Docker registry push"),
		ep(`\bnpm\s+publish\b`, "npm publish"),
		ep(`\bpip\s+upload\b`, "pip upload"),
		ep(`\btwine\s+upload\b`, "PyPI upload"),
		ep(`\b(nc|ncat|socat|telnet)\s`, "generic network tool"),
	}
}

// destinationRe extracts hostname/IP targets from URLs, SSH targets, and SCP targets.
// Each alternative uses a separate capture group; extractDestinations collects
// all non-empty sub-matches.
var destinationRe = regexp.MustCompile(
	`(?:https?|ftp)://([^\s/:]+)` + // URL hosts
		`|\bssh\s+(?:\S+@)?([^\s@:]+)` + // SSH: optional user@ prefix, then host
		`|\bscp\s+\S+\s+(?:\S+@)?([^\s@:]+)`, // SCP: source, then optional user@, then host
)

// extractDestinations returns a comma-separated string of detected destinations.
func extractDestinations(text string) string {
	matches := destinationRe.FindAllStringSubmatch(text, -1)
	if len(matches) == 0 {
		return ""
	}
	seen := make(map[string]bool)
	var dests []string
	for _, m := range matches {
		for _, sub := range m[1:] {
			sub = strings.TrimSpace(sub)
			if sub != "" && !seen[sub] {
				seen[sub] = true
				dests = append(dests, sub)
			}
		}
	}
	return strings.Join(dests, ", ")
}
