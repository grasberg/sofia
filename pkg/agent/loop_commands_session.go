package agent

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// handleBranchCommand implements /branch [label] and /branch switch <key>.
func (al *AgentLoop) handleBranchCommand(
	args []string, agent *AgentInstance, sessionKey string,
) string {
	// /branch switch <key>
	if len(args) >= 2 && args[0] == "switch" {
		targetKey := args[1]
		// Verify the target session has history (i.e. it exists).
		history := agent.Sessions.GetHistory(targetKey)
		if len(history) == 0 {
			// Also check if it is a known branch key.
			if _, ok := al.branchManager.GetParent(targetKey); !ok {
				return fmt.Sprintf("Branch %q not found.", targetKey)
			}
		}
		return fmt.Sprintf("Switched to branch %s", targetKey)
	}

	// /branch [label] — create a new branch from the current session.
	label := strings.Join(args, " ")
	info, err := al.branchManager.Branch(agent.Sessions, sessionKey, label)
	if err != nil {
		return fmt.Sprintf("Failed to create branch: %v", err)
	}

	msg := fmt.Sprintf(
		"Branch created.\n• Key: %s\n• Messages copied: %d",
		info.BranchKey, info.MessageCount,
	)
	if info.Label != "" {
		msg += fmt.Sprintf("\n• Label: %s", info.Label)
	}
	return msg
}

// handleBranchesCommand implements /branches — list all branches of the current session.
func (al *AgentLoop) handleBranchesCommand(sessionKey string) string {
	// Resolve to the root parent so we list siblings regardless of which branch we're on.
	rootKey := al.resolveRootSessionKey(sessionKey)

	branches := al.branchManager.ListBranches(rootKey)
	if len(branches) == 0 {
		return "No branches for this session."
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "Branches of %s:\n", rootKey)
	for _, b := range branches {
		line := fmt.Sprintf("• %s (%d msgs, %s)",
			b.BranchKey, b.MessageCount,
			b.BranchedAt.Format(time.RFC3339))
		if b.Label != "" {
			line += fmt.Sprintf(" [%s]", b.Label)
		}
		sb.WriteString(line + "\n")
	}
	return strings.TrimRight(sb.String(), "\n")
}

// handleCheckpointCommand implements /checkpoint list|create|rollback|cleanup.
func (al *AgentLoop) handleCheckpointCommand(
	args []string, agent *AgentInstance, sessionKey string,
) string {
	if al.checkpointMgr == nil || al.memDB == nil {
		return "Checkpointing unavailable: memory database not initialized."
	}

	if len(args) == 0 || args[0] == "list" {
		checkpoints, err := al.checkpointMgr.List(sessionKey)
		if err != nil {
			return fmt.Sprintf("Failed to list checkpoints: %v", err)
		}
		if len(checkpoints) == 0 {
			return "No checkpoints for this session. Usage: /checkpoint [list|create <name>|rollback <id|latest>|cleanup]"
		}

		var sb strings.Builder
		fmt.Fprintf(&sb, "Checkpoints for %s:\n", sessionKey)
		for _, cp := range checkpoints {
			fmt.Fprintf(&sb, "- %d %q msgs=%d created=%s\n",
				cp.ID,
				cp.Name,
				cp.MsgCount,
				cp.CreatedAt.Format(time.RFC3339))
		}
		return strings.TrimRight(sb.String(), "\n")
	}

	switch args[0] {
	case "create":
		name := strings.TrimSpace(strings.Join(args[1:], " "))
		if name == "" {
			name = "manual"
		}

		cp, err := al.checkpointMgr.Create(sessionKey, agent.ID, name, 0)
		if err != nil {
			return fmt.Sprintf("Failed to create checkpoint: %v", err)
		}

		return fmt.Sprintf(
			"Checkpoint created.\n- ID: %d\n- Name: %s\n- Messages saved: %d",
			cp.ID,
			cp.Name,
			cp.MsgCount,
		)

	case "rollback":
		if len(args) < 2 || strings.EqualFold(args[1], "latest") {
			cp, _, err := al.checkpointMgr.RollbackToLatest(sessionKey)
			if err != nil {
				return fmt.Sprintf("Failed to rollback: %v", err)
			}
			if cp == nil {
				return "No checkpoints to rollback to."
			}

			return fmt.Sprintf(
				"Rolled back to latest checkpoint %d (%q).\n- Restored messages: %d\n- Session summary restored.",
				cp.ID,
				cp.Name,
				cp.MsgCount,
			)
		}

		checkpointID, err := strconv.ParseInt(args[1], 10, 64)
		if err != nil {
			return "Checkpoint ID must be an integer or 'latest'."
		}

		cp, err := al.checkpointMgr.Rollback(sessionKey, checkpointID)
		if err != nil {
			return fmt.Sprintf("Failed to rollback: %v", err)
		}

		return fmt.Sprintf(
			"Rolled back to checkpoint %d (%q).\n- Restored messages: %d\n- Session summary restored.",
			cp.ID,
			cp.Name,
			cp.MsgCount,
		)

	case "cleanup":
		if err := al.checkpointMgr.Cleanup(sessionKey); err != nil {
			return fmt.Sprintf("Failed to remove checkpoints: %v", err)
		}
		return "All checkpoints removed for this session."

	default:
		return "Usage: /checkpoint [list|create <name>|rollback <id|latest>|cleanup]"
	}
}

// Handoff is the JSON-serializable record written by /pause and read by /resume.
type Handoff struct {
	SessionKey string    `json:"session_key"`
	AgentID    string    `json:"agent_id"`
	AgentName  string    `json:"agent_name,omitempty"`
	Note       string    `json:"note,omitempty"`
	Summary    string    `json:"summary,omitempty"`
	Messages   int       `json:"messages"`
	Checkpoint int64     `json:"checkpoint,omitempty"`
	Context    []string  `json:"context,omitempty"`
	PausedAt   time.Time `json:"paused_at"`
}

const handoffNoteKind = "handoff"

// handlePauseCommand implements /pause [note] — save session state for later resumption.
func (al *AgentLoop) handlePauseCommand(
	args []string, agent *AgentInstance, sessionKey string,
) string {
	if al.memDB == nil {
		return "Pause unavailable: memory database not initialized."
	}

	history := agent.Sessions.GetHistory(sessionKey)
	if len(history) == 0 {
		return "Nothing to pause — session is empty."
	}

	note := strings.TrimSpace(strings.Join(args, " "))

	// Auto-checkpoint so the user has a recovery point.
	var checkpointID int64
	if al.checkpointMgr != nil {
		cpName := "pause"
		if note != "" && len(note) <= 40 {
			cpName = "pause: " + note
		}
		if cp, err := al.checkpointMgr.Create(sessionKey, agent.ID, cpName, 0); err == nil {
			checkpointID = cp.ID
		}
	}

	// Capture the last few messages as context breadcrumbs.
	contextLines := make([]string, 0, 4)
	start := len(history) - 4
	if start < 0 {
		start = 0
	}
	for _, m := range history[start:] {
		preview := m.Content
		if len(preview) > 150 {
			preview = preview[:150] + "…"
		}
		contextLines = append(contextLines, m.Role+": "+preview)
	}

	handoff := Handoff{
		SessionKey: sessionKey,
		AgentID:    agent.ID,
		AgentName:  agent.Name,
		Note:       note,
		Summary:    agent.Sessions.GetSummary(sessionKey),
		Messages:   len(history),
		Checkpoint: checkpointID,
		Context:    contextLines,
		PausedAt:   time.Now().UTC(),
	}

	data, err := json.Marshal(handoff)
	if err != nil {
		return fmt.Sprintf("Failed to serialize handoff: %v", err)
	}

	if err := al.memDB.SetNote(agent.ID, handoffNoteKind, sessionKey, string(data)); err != nil {
		return fmt.Sprintf("Failed to save handoff: %v", err)
	}

	var sb strings.Builder
	sb.WriteString("Session paused.\n")
	if note != "" {
		fmt.Fprintf(&sb, "- Note: %s\n", note)
	}
	fmt.Fprintf(&sb, "- Messages: %d\n", len(history))
	if checkpointID > 0 {
		fmt.Fprintf(&sb, "- Checkpoint: %d\n", checkpointID)
	}
	fmt.Fprintf(&sb, "- Session: %s\n", sessionKey)
	sb.WriteString("\nUse /resume to pick up where you left off.")
	return sb.String()
}

// handleResumeCommand implements /resume — restore context from a paused session.
func (al *AgentLoop) handleResumeCommand(
	args []string, agent *AgentInstance, sessionKey string,
) string {
	if al.memDB == nil {
		return "Resume unavailable: memory database not initialized."
	}

	// If the user provides a specific session key, try that one.
	targetKey := sessionKey
	if len(args) > 0 {
		targetKey = strings.TrimSpace(strings.Join(args, " "))
	}

	// Try to load a handoff for the target session.
	raw := al.memDB.GetNote(agent.ID, handoffNoteKind, targetKey)
	if raw != "" {
		return al.formatAndClearHandoff(agent, targetKey, raw)
	}

	// No handoff for the current/target session — list all pending handoffs.
	allHandoffs, err := al.memDB.ListNotesByKind(handoffNoteKind)
	if err != nil {
		return fmt.Sprintf("Failed to list handoffs: %v", err)
	}

	if len(allHandoffs) == 0 {
		return "No paused sessions found. Use /pause to save your place before stopping."
	}

	var sb strings.Builder
	sb.WriteString("No handoff for the current session. Paused sessions:\n\n")
	for _, n := range allHandoffs {
		var h Handoff
		if err := json.Unmarshal([]byte(n.Content), &h); err != nil {
			continue
		}
		line := fmt.Sprintf("- %s", h.SessionKey)
		if h.AgentName != "" {
			line += fmt.Sprintf(" [%s]", h.AgentName)
		}
		line += fmt.Sprintf(" (%d msgs, paused %s)", h.Messages, h.PausedAt.Format("2006-01-02 15:04"))
		if h.Note != "" {
			line += fmt.Sprintf(" — %s", h.Note)
		}
		sb.WriteString(line + "\n")
	}
	sb.WriteString("\nUse /resume <session-key> to restore a specific session.")
	return sb.String()
}

// formatAndClearHandoff renders a handoff note for the user and removes it from the DB.
func (al *AgentLoop) formatAndClearHandoff(agent *AgentInstance, sessionKey, raw string) string {
	var h Handoff
	if err := json.Unmarshal([]byte(raw), &h); err != nil {
		return fmt.Sprintf("Failed to parse handoff data: %v", err)
	}

	// Clear the handoff now that it has been consumed.
	_ = al.memDB.DeleteNote(agent.ID, handoffNoteKind, sessionKey)

	var sb strings.Builder
	sb.WriteString("Resuming session.\n")
	fmt.Fprintf(&sb, "- Session: %s\n", h.SessionKey)
	if h.AgentName != "" {
		fmt.Fprintf(&sb, "- Agent: %s\n", h.AgentName)
	}
	fmt.Fprintf(&sb, "- Messages: %d\n", h.Messages)
	fmt.Fprintf(&sb, "- Paused: %s\n", h.PausedAt.Format("2006-01-02 15:04 UTC"))
	if h.Checkpoint > 0 {
		fmt.Fprintf(&sb, "- Checkpoint: %d (use /checkpoint rollback %d to go back)\n",
			h.Checkpoint, h.Checkpoint)
	}

	if h.Summary != "" {
		fmt.Fprintf(&sb, "\nSession summary:\n%s\n", h.Summary)
	}

	if h.Note != "" {
		fmt.Fprintf(&sb, "\nHandoff note:\n%s\n", h.Note)
	}

	if len(h.Context) > 0 {
		sb.WriteString("\nLast activity:\n")
		for _, line := range h.Context {
			sb.WriteString("  " + line + "\n")
		}
	}

	return strings.TrimRight(sb.String(), "\n")
}

func (al *AgentLoop) resolveRootSessionKey(sessionKey string) string {
	if al.branchManager == nil {
		return sessionKey
	}

	rootKey := sessionKey
	for {
		parent, ok := al.branchManager.GetParent(rootKey)
		if !ok {
			return rootKey
		}
		rootKey = parent
	}
}
