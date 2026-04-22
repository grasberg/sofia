package tools

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/grasberg/sofia/pkg/logger"
)

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
