package agent

import (
	"fmt"
	"hash/fnv"
	"sort"
	"strings"

	"github.com/grasberg/sofia/pkg/logger"
	"github.com/grasberg/sofia/pkg/providers"
)

// DoomRecoveryType enumerates the recovery actions the detector can recommend.
type DoomRecoveryType int

const (
	DoomRecoveryNone        DoomRecoveryType = iota
	DoomRecoveryRedirect                     // Inject "try different approach" message
	DoomRecoveryModelSwitch                  // Switch to fallback model
	DoomRecoveryAskHelp                      // Break loop, ask user for help
	DoomRecoveryAbort                        // Abort gracefully
)

// DoomRecoveryAction is the recommended action when a doom loop is detected.
type DoomRecoveryAction struct {
	Type   DoomRecoveryType
	Prompt string
}

// DoomLoopDetector tracks agent iteration patterns and detects when the
// agent is stuck in a non-productive loop.
type DoomLoopDetector struct {
	toolHistory    []uint64       // FNV-1a hashes of tool name+args
	errorPatterns  map[string]int // error message -> count
	outputHashes   []uint64       // hashes of LLM responses
	uniqueTools    map[string]int // tool name -> first seen iteration
	recoveryStage  int            // 0=normal, 1=redirect, 2=model-switch, 3=ask-help, 4=abort
	repThreshold   int            // consecutive repetitions to trigger
	currentIter    int
	noProgressRuns int // iterations without a new unique tool
}

// NewDoomLoopDetector creates a detector with the given repetition threshold.
func NewDoomLoopDetector(repThreshold int) *DoomLoopDetector {
	if repThreshold <= 0 {
		repThreshold = 3
	}
	return &DoomLoopDetector{
		errorPatterns: make(map[string]int),
		uniqueTools:   make(map[string]int),
		repThreshold:  repThreshold,
	}
}

// RecordToolCalls records tool calls from the current iteration.
func (d *DoomLoopDetector) RecordToolCalls(toolCalls []providers.ToolCall) {
	d.currentIter++
	sawNew := false
	for _, tc := range toolCalls {
		h := hashToolCall(tc.Name, tc.Arguments)
		d.toolHistory = append(d.toolHistory, h)
		if _, exists := d.uniqueTools[tc.Name]; !exists {
			d.uniqueTools[tc.Name] = d.currentIter
			sawNew = true
		}
	}
	// Cap history to 2x threshold to avoid unbounded growth across long sessions.
	if cap := d.repThreshold * 2; len(d.toolHistory) > cap {
		d.toolHistory = d.toolHistory[len(d.toolHistory)-cap:]
	}
	if !sawNew && len(toolCalls) > 0 {
		d.noProgressRuns++
	} else {
		d.noProgressRuns = 0
	}
}

// RecordError records a tool execution error.
func (d *DoomLoopDetector) RecordError(errMsg string) {
	// Normalize to first 100 chars to group similar errors
	key := errMsg
	if len(key) > 100 {
		key = key[:100]
	}
	d.errorPatterns[key]++

	// Cap the map to prevent unbounded growth from many distinct errors.
	const maxDistinctErrors = 50
	if len(d.errorPatterns) > maxDistinctErrors {
		// Evict entries with count == 1 (one-off errors unlikely to signal a doom loop).
		for k, count := range d.errorPatterns {
			if count <= 1 {
				delete(d.errorPatterns, k)
			}
		}
	}
}

// RecordOutput records the LLM response content hash.
func (d *DoomLoopDetector) RecordOutput(content string) {
	d.outputHashes = append(d.outputHashes, hashString(content))
	// Cap history to 2x threshold to avoid unbounded growth across long sessions.
	if cap := d.repThreshold * 2; len(d.outputHashes) > cap {
		d.outputHashes = d.outputHashes[len(d.outputHashes)-cap:]
	}
}

// Check evaluates all detection signals and returns true if a doom loop is detected.
func (d *DoomLoopDetector) Check() bool {
	return d.repeatedToolCalls() || d.repeatedErrors() || d.repeatedOutputs() || d.noProgress()
}

// GetRecoveryAction returns the recommended action and advances the recovery stage.
func (d *DoomLoopDetector) GetRecoveryAction() DoomRecoveryAction {
	d.recoveryStage++

	switch d.recoveryStage {
	case 1:
		logger.InfoCF("doom_loop", "Doom loop detected — injecting redirect",
			map[string]any{"stage": d.recoveryStage, "iter": d.currentIter})
		return DoomRecoveryAction{
			Type: DoomRecoveryRedirect,
			Prompt: "[SYSTEM] You appear to be repeating the same actions without making progress. " +
				"STOP and try a fundamentally different approach. " +
				"Consider: different tools, different strategy, or simplifying the task.",
		}
	case 2:
		logger.InfoCF("doom_loop", "Doom loop persists — recommending model switch",
			map[string]any{"stage": d.recoveryStage, "iter": d.currentIter})
		return DoomRecoveryAction{
			Type: DoomRecoveryModelSwitch,
			Prompt: "[SYSTEM] Switching to a different model to attempt a fresh approach. " +
				"Previous attempts were stuck in a loop.",
		}
	case 3:
		logger.InfoCF("doom_loop", "Doom loop unresolved — asking user for help",
			map[string]any{"stage": d.recoveryStage, "iter": d.currentIter})
		return DoomRecoveryAction{
			Type: DoomRecoveryAskHelp,
			Prompt: "I've been unable to make progress on this task after several attempts. " +
				"I keep running into the same issue. Could you provide guidance or clarify what you'd like me to do?",
		}
	default:
		logger.InfoCF("doom_loop", "Doom loop — aborting",
			map[string]any{"stage": d.recoveryStage, "iter": d.currentIter})
		return DoomRecoveryAction{
			Type:   DoomRecoveryAbort,
			Prompt: "I was unable to complete this task — I got stuck in a loop. Here's what I tried before stopping.",
		}
	}
}

// repeatedToolCalls checks if the last N tool call hashes are identical.
func (d *DoomLoopDetector) repeatedToolCalls() bool {
	n := d.repThreshold
	if len(d.toolHistory) < n {
		return false
	}
	tail := d.toolHistory[len(d.toolHistory)-n:]
	first := tail[0]
	for _, h := range tail[1:] {
		if h != first {
			return false
		}
	}
	return true
}

// repeatedErrors checks if any single error has occurred N+ times.
func (d *DoomLoopDetector) repeatedErrors() bool {
	for _, count := range d.errorPatterns {
		if count >= d.repThreshold {
			return true
		}
	}
	return false
}

// repeatedOutputs checks if the last N LLM outputs have the same hash.
func (d *DoomLoopDetector) repeatedOutputs() bool {
	n := d.repThreshold
	if len(d.outputHashes) < n {
		return false
	}
	tail := d.outputHashes[len(d.outputHashes)-n:]
	first := tail[0]
	for _, h := range tail[1:] {
		if h != first {
			return false
		}
	}
	return true
}

// noProgress checks if no new unique tools have been seen for N consecutive iterations.
func (d *DoomLoopDetector) noProgress() bool {
	return d.noProgressRuns >= d.repThreshold+1 // slightly higher threshold for this signal
}

func hashToolCall(name string, args map[string]any) uint64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(name))
	
	// Sort keys to ensure deterministic hashing
	keys := make([]string, 0, len(args))
	for k := range args {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	
	for _, k := range keys {
		_, _ = h.Write([]byte(k))
		_, _ = fmt.Fprint(h, args[k])
	}
	return h.Sum64()
}

func hashString(s string) uint64 {
	h := fnv.New64a()
	norm := s
	if len(norm) > 500 {
		norm = norm[:500]
	}
	_, _ = h.Write([]byte(strings.ToLower(strings.TrimSpace(norm))))
	return h.Sum64()
}

// FormatAttemptSummary builds a brief summary of what was attempted, for the abort message.
func (d *DoomLoopDetector) FormatAttemptSummary() string {
	tools := make([]string, 0, len(d.uniqueTools))
	for name := range d.uniqueTools {
		tools = append(tools, name)
	}
	return fmt.Sprintf("Tools attempted: %s. Total iterations: %d.",
		strings.Join(tools, ", "), d.currentIter)
}
