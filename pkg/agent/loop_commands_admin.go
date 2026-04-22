package agent

import (
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/grasberg/sofia/pkg/memory"
)

// handleStatusCommand returns a human-readable status of what Sofia is doing.
func (al *AgentLoop) handleStatusCommand() string {
	activeID, _ := al.activeAgentID.Load().(string)
	status, _ := al.activeStatus.Load().(string)

	if activeID == "" || status == "Idle" || status == "" {
		return "Sofia is idle — no active tasks."
	}

	// Find agent name
	agentName := activeID
	if agent, ok := al.getRegistry().GetAgent(activeID); ok && agent.Name != "" {
		agentName = agent.Name
	}

	return fmt.Sprintf("Sofia is busy:\n• Agent: %s\n• Status: %s", agentName, status)
}

// handleHealthCommand implements /health for session diagnostics.
func (al *AgentLoop) handleHealthCommand(args []string, agent *AgentInstance, sessionKey string) string {
	if len(args) != 0 {
		return "Usage: /health"
	}

	history := agent.Sessions.GetHistory(sessionKey)
	summary := agent.Sessions.GetSummary(sessionKey)
	historyTokens := al.estimateTokens(history)
	summaryTokens := utf8.RuneCountInString(summary) * 2 / 5
	totalTokens := historyTokens + summaryTokens

	window := agent.ContextWindow
	if window <= 0 {
		window = agent.MaxTokens
	}
	if window <= 0 {
		window = 1
	}
	tokenPercent := totalTokens * 100 / window

	var meta *memory.SessionRow
	if al.memDB != nil {
		meta = al.memDB.GetSessionMeta(sessionKey)
	}

	checkpointCount := 0
	latestCheckpoint := ""
	if al.checkpointMgr != nil && al.memDB != nil {
		if checkpoints, err := al.checkpointMgr.List(sessionKey); err == nil {
			checkpointCount = len(checkpoints)
			if checkpointCount > 0 {
				latestCheckpoint = fmt.Sprintf(
					"%d %q at %s",
					checkpoints[0].ID,
					checkpoints[0].Name,
					checkpoints[0].CreatedAt.Format(time.RFC3339),
				)
			}
		}
	}

	rootKey := al.resolveRootSessionKey(sessionKey)
	branchCount := len(al.branchManager.ListBranches(rootKey))
	usage := al.usageTracker.GetSession(sessionKey)
	_, isSummarizing := al.summarizing.Load(agent.ID + ":" + sessionKey)

	status := "GOOD"
	signals := make([]string, 0, 3)
	recommendations := make([]string, 0, 3)

	if tokenPercent >= 90 {
		status = "CRITICAL"
		signals = append(signals,
			fmt.Sprintf("context pressure is high at %d%% of the configured window", tokenPercent))
	} else if tokenPercent >= 75 {
		status = "ATTENTION"
		signals = append(signals,
			fmt.Sprintf("context pressure is elevated at %d%% of the configured window", tokenPercent))
	}

	if len(history) > 40 {
		status = "CRITICAL"
		signals = append(signals, fmt.Sprintf("message history is very long (%d messages)", len(history)))
	} else if len(history) > 20 {
		if status == "GOOD" {
			status = "ATTENTION"
		}
		signals = append(signals,
			fmt.Sprintf("message history crossed Sofia's auto-summary threshold (%d messages)", len(history)))
	}

	if summary == "" && len(history) > 12 {
		if status == "GOOD" {
			status = "ATTENTION"
		}
		signals = append(signals, "no saved summary is available yet")
	}

	if tokenPercent >= 75 || len(history) > 20 {
		recommendations = append(recommendations,
			"Run /compact before the next large step to reduce context pressure.")
	}
	if checkpointCount == 0 && (len(history) > 8 || tokenPercent >= 75) {
		recommendations = append(recommendations,
			"Create a recovery point with /checkpoint create <name> before risky work.")
	}
	if status == "CRITICAL" {
		recommendations = append(recommendations,
			"Consider /branch if you want to explore an alternative without adding more load to this session.")
	}
	if len(recommendations) == 0 {
		recommendations = append(recommendations, "No immediate action needed.")
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "Session health: %s\n", status)
	fmt.Fprintf(&sb, "- Session: %s\n", sessionKey)
	fmt.Fprintf(&sb, "- Messages: %d\n", len(history))
	fmt.Fprintf(&sb, "- Estimated context: %d/%d tokens (%d%%)\n", totalTokens, window, tokenPercent)
	if summary == "" {
		sb.WriteString("- Summary: none\n")
	} else {
		fmt.Fprintf(&sb, "- Summary: %d chars (%d estimated tokens)\n",
			utf8.RuneCountInString(summary),
			summaryTokens)
	}
	if meta != nil {
		fmt.Fprintf(&sb, "- Last activity: %s\n", meta.UpdatedAt.Format(time.RFC3339))
	}
	if al.checkpointMgr == nil || al.memDB == nil {
		sb.WriteString("- Checkpoints: unavailable\n")
	} else if checkpointCount == 0 {
		sb.WriteString("- Checkpoints: 0\n")
	} else {
		fmt.Fprintf(&sb, "- Checkpoints: %d (latest: %s)\n", checkpointCount, latestCheckpoint)
	}
	fmt.Fprintf(&sb, "- Branches from root: %d\n", branchCount)
	if usage != nil {
		fmt.Fprintf(&sb, "- Usage: %d calls, %d total tokens\n", usage.CallCount, usage.TotalTokens)
	}
	if isSummarizing {
		sb.WriteString("- Background summarization: running\n")
	} else {
		sb.WriteString("- Background summarization: idle\n")
	}

	if len(signals) > 0 {
		sb.WriteString("\nSignals:\n")
		for _, signal := range signals {
			sb.WriteString("- " + signal + "\n")
		}
	}

	sb.WriteString("\nRecommended next step:\n")
	for _, recommendation := range recommendations {
		sb.WriteString("- " + recommendation + "\n")
	}

	return strings.TrimRight(sb.String(), "\n")
}
