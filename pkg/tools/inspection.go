package tools

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"github.com/grasberg/sofia/pkg/logger"
)

// InspectionVerdict represents the outcome of inspecting a tool call.
type InspectionVerdict struct {
	Allowed    bool
	Reason     string
	Inspector  string  // which inspector flagged it
	RiskLevel  string  // "low", "medium", "high", "critical"
	Confidence float64 // 0.0 to 1.0
}

// allowedVerdict is a convenience constructor for a passing verdict.
func allowedVerdict(inspector string) *InspectionVerdict {
	return &InspectionVerdict{
		Allowed:    true,
		Inspector:  inspector,
		RiskLevel:  "low",
		Confidence: 1.0,
	}
}

// blockedVerdict is a convenience constructor for a blocking verdict.
func blockedVerdict(inspector, reason, riskLevel string, confidence float64) *InspectionVerdict {
	return &InspectionVerdict{
		Allowed:    false,
		Reason:     reason,
		Inspector:  inspector,
		RiskLevel:  riskLevel,
		Confidence: confidence,
	}
}

// ToolInspector inspects a tool call before execution.
type ToolInspector interface {
	Name() string
	Inspect(toolName string, args map[string]any, argsJSON string) *InspectionVerdict
}

// ---------------------------------------------------------------------------
// InspectionPipeline
// ---------------------------------------------------------------------------

// InspectionPipeline chains multiple inspectors, executing them in order.
// The first non-allowed verdict short-circuits the pipeline.
type InspectionPipeline struct {
	inspectors []ToolInspector
}

// NewInspectionPipeline constructs a pipeline from the given inspectors.
func NewInspectionPipeline(inspectors ...ToolInspector) *InspectionPipeline {
	return &InspectionPipeline{inspectors: inspectors}
}

// Inspect runs every inspector in registration order. If any inspector returns
// a non-allowed verdict, the pipeline short-circuits and returns that verdict.
// When all inspectors pass, an allowed verdict is returned.
func (p *InspectionPipeline) Inspect(toolName string, args map[string]any, argsJSON string) *InspectionVerdict {
	for _, insp := range p.inspectors {
		v := insp.Inspect(toolName, args, argsJSON)
		if v != nil && !v.Allowed {
			logger.InfoCF("inspection", "Tool call blocked",
				map[string]any{
					"tool":      toolName,
					"inspector": v.Inspector,
					"reason":    v.Reason,
					"risk":      v.RiskLevel,
				})
			return v
		}
	}
	return allowedVerdict("pipeline")
}

// AddInspector appends an inspector to the pipeline.
func (p *InspectionPipeline) AddInspector(insp ToolInspector) {
	p.inspectors = append(p.inspectors, insp)
}

// ---------------------------------------------------------------------------
// SecurityInspector
// ---------------------------------------------------------------------------

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

// ---------------------------------------------------------------------------
// EgressInspector
// ---------------------------------------------------------------------------

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

// ---------------------------------------------------------------------------
// PermissionInspector
// ---------------------------------------------------------------------------

// PermissionConfig specifies per-tool permission tiers.
type PermissionConfig struct {
	AlwaysAllow []string // tool names that are auto-approved
	AskBefore   []string // tool names that require confirmation
	NeverAllow  []string // tool names that are always blocked
}

// PermissionInspector gates tool calls based on a three-tier permission model.
type PermissionInspector struct {
	always map[string]bool
	ask    map[string]bool
	never  map[string]bool
}

// NewPermissionInspector builds a PermissionInspector from the given config.
func NewPermissionInspector(cfg PermissionConfig) *PermissionInspector {
	toSet := func(s []string) map[string]bool {
		m := make(map[string]bool, len(s))
		for _, v := range s {
			m[v] = true
		}
		return m
	}
	return &PermissionInspector{
		always: toSet(cfg.AlwaysAllow),
		ask:    toSet(cfg.AskBefore),
		never:  toSet(cfg.NeverAllow),
	}
}

func (p *PermissionInspector) Name() string { return "permission" }

func (p *PermissionInspector) Inspect(toolName string, _ map[string]any, _ string) *InspectionVerdict {
	if p.never[toolName] {
		return blockedVerdict(p.Name(),
			fmt.Sprintf("tool %q is in the never-allow list", toolName),
			"critical", 1.0,
		)
	}
	if p.ask[toolName] {
		return &InspectionVerdict{
			Allowed:    false,
			Reason:     fmt.Sprintf("tool %q requires confirmation before execution", toolName),
			Inspector:  p.Name(),
			RiskLevel:  "medium",
			Confidence: 1.0,
		}
	}
	// AlwaysAllow or unlisted tools pass through.
	return allowedVerdict(p.Name())
}

// ---------------------------------------------------------------------------
// RepetitionInspector
// ---------------------------------------------------------------------------

// callRecord holds a tool name and a hash of its arguments.
type callRecord struct {
	ToolName string
	ArgsHash string
}

// RepetitionInspector detects repeated identical tool calls using a ring buffer.
type RepetitionInspector struct {
	mu             sync.Mutex
	buffer         []callRecord
	head           int
	size           int
	capacity       int
	maxRepetitions int
}

// NewRepetitionInspector creates a RepetitionInspector with the given ring buffer
// capacity and maximum allowed repetitions. Defaults: capacity 64, maxRepetitions 3.
func NewRepetitionInspector(capacity, maxRepetitions int) *RepetitionInspector {
	if capacity <= 0 {
		capacity = 64
	}
	if maxRepetitions <= 0 {
		maxRepetitions = 3
	}
	return &RepetitionInspector{
		buffer:         make([]callRecord, capacity),
		capacity:       capacity,
		maxRepetitions: maxRepetitions,
	}
}

func (r *RepetitionInspector) Name() string { return "repetition" }

func (r *RepetitionInspector) Inspect(toolName string, _ map[string]any, argsJSON string) *InspectionVerdict {
	h := hashArgs(argsJSON)

	r.mu.Lock()
	defer r.mu.Unlock()

	// Count how many times this exact call appears in the ring buffer.
	count := 0
	n := r.size
	if n > r.capacity {
		n = r.capacity
	}
	for i := range n {
		idx := (r.head - 1 - i + r.capacity) % r.capacity
		rec := r.buffer[idx]
		if rec.ToolName == toolName && rec.ArgsHash == h {
			count++
		}
	}

	// Record the current call in the ring buffer.
	r.buffer[r.head%r.capacity] = callRecord{ToolName: toolName, ArgsHash: h}
	r.head = (r.head + 1) % r.capacity
	if r.size < r.capacity {
		r.size++
	}

	if count >= r.maxRepetitions {
		return blockedVerdict(
			r.Name(),
			fmt.Sprintf(
				"tool %q called %d times with identical arguments (threshold %d)",
				toolName,
				count+1,
				r.maxRepetitions,
			),
			"medium",
			0.95,
		)
	}
	return allowedVerdict(r.Name())
}

// hashArgs produces a hex-encoded SHA-256 hash of the arguments string.
func hashArgs(argsJSON string) string {
	sum := sha256.Sum256([]byte(argsJSON))
	return hex.EncodeToString(sum[:])
}

// ---------------------------------------------------------------------------
// AdversaryInspector (stub/rules-file check)
// ---------------------------------------------------------------------------

// adversaryRule is a simple keyword-matching rule parsed from the rules file.
type adversaryRule struct {
	ToolPattern string // substring to match against tool name ("*" matches all)
	Keyword     string // keyword to search for in the args
	RiskLevel   string
}

// AdversaryInspector performs a configurable rules-file check. If no rules file
// exists at ~/.sofia/adversary.md it fails open (allows everything).
type AdversaryInspector struct {
	rules []adversaryRule
}

// NewAdversaryInspector loads rules from ~/.sofia/adversary.md if the file exists.
// The file format is one rule per line: TOOL_PATTERN KEYWORD RISK_LEVEL
// Lines starting with "#" or blank lines are ignored.
// Example:
//
//	# block curl from posting secrets
//	exec  /etc/passwd  critical
//	*     api_key      high
func NewAdversaryInspector() *AdversaryInspector {
	a := &AdversaryInspector{}
	a.loadRules()
	return a
}

func (a *AdversaryInspector) Name() string { return "adversary" }

func (a *AdversaryInspector) Inspect(toolName string, _ map[string]any, argsJSON string) *InspectionVerdict {
	if len(a.rules) == 0 {
		// No rules file: fail open.
		return allowedVerdict(a.Name())
	}

	lower := strings.ToLower(argsJSON)
	for _, rule := range a.rules {
		if rule.ToolPattern != "*" && !strings.Contains(toolName, rule.ToolPattern) {
			continue
		}
		if strings.Contains(lower, strings.ToLower(rule.Keyword)) {
			return blockedVerdict(
				a.Name(),
				fmt.Sprintf("adversary rule matched: tool=%q keyword=%q", rule.ToolPattern, rule.Keyword),
				rule.RiskLevel,
				0.75,
			)
		}
	}
	return allowedVerdict(a.Name())
}

func (a *AdversaryInspector) loadRules() {
	home, err := os.UserHomeDir()
	if err != nil {
		return
	}
	path := filepath.Join(home, ".sofia", "adversary.md")
	f, err := os.Open(path)
	if err != nil {
		return // File doesn't exist or isn't readable: fail open.
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}
		a.rules = append(a.rules, adversaryRule{
			ToolPattern: fields[0],
			Keyword:     fields[1],
			RiskLevel:   fields[2],
		})
	}

	if len(a.rules) > 0 {
		logger.InfoCF("inspection", "Adversary rules loaded",
			map[string]any{"count": len(a.rules), "path": path})
	}
}
