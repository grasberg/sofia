package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/grasberg/sofia/pkg/bus"
	"github.com/grasberg/sofia/pkg/memory"
	"github.com/grasberg/sofia/pkg/providers"
	"github.com/grasberg/sofia/pkg/search"
)

func (al *AgentLoop) handleCommand(ctx context.Context, msg bus.InboundMessage) (string, bool) {
	content := strings.TrimSpace(msg.Content)
	if !strings.HasPrefix(content, "/") {
		return "", false
	}

	parts := strings.Fields(content)
	if len(parts) == 0 {
		return "", false
	}

	cmd := parts[0]
	args := parts[1:]

	switch cmd {
	case "/status":
		return al.handleStatusCommand(), true

	case "/show":
		if len(args) < 1 {
			return "Usage: /show [model|channel|agents]", true
		}
		switch args[0] {
		case "model":
			defaultAgent := al.getRegistry().GetDefaultAgent()
			if defaultAgent == nil {
				return "No default agent configured", true
			}
			return fmt.Sprintf("Current model: %s", defaultAgent.Model), true
		case "channel":
			return fmt.Sprintf("Current channel: %s", msg.Channel), true
		case "agents":
			agentIDs := al.getRegistry().ListAgentIDs()
			return fmt.Sprintf("Registered agents: %s", strings.Join(agentIDs, ", ")), true
		default:
			return fmt.Sprintf("Unknown show target: %s", args[0]), true
		}

	case "/list":
		if len(args) < 1 {
			return "Usage: /list [models|channels|agents]", true
		}
		switch args[0] {
		case "models":
			return "Available models: configured in config.json per agent", true
		case "channels":
			if al.channelManager == nil {
				return "Channel manager not initialized", true
			}
			channels := al.channelManager.GetEnabledChannels()
			if len(channels) == 0 {
				return "No channels enabled", true
			}
			return fmt.Sprintf("Enabled channels: %s", strings.Join(channels, ", ")), true
		case "agents":
			agentIDs := al.getRegistry().ListAgentIDs()
			return fmt.Sprintf("Registered agents: %s", strings.Join(agentIDs, ", ")), true
		default:
			return fmt.Sprintf("Unknown list target: %s", args[0]), true
		}

	case "/switch":
		if len(args) < 3 || args[1] != "to" {
			return "Usage: /switch [model|channel] to <name>", true
		}
		target := args[0]
		value := args[2]

		switch target {
		case "model":
			defaultAgent := al.getRegistry().GetDefaultAgent()
			if defaultAgent == nil {
				return "No default agent configured", true
			}
			// Validate the model name against model_list
			mc, err := al.cfg.GetModelConfig(value)
			if err != nil || mc == nil {
				return fmt.Sprintf(
					"Model %q not found in model_list. Use /list models to see available models.",
					value,
				), true
			}
			al.agentModelMu.Lock()
			oldModel := defaultAgent.Model
			defaultAgent.Model = value
			_, defaultAgent.ModelID = providers.ExtractProtocol(mc.Model)
			al.agentModelMu.Unlock()
			return fmt.Sprintf("Switched model from %s to %s", oldModel, value), true
		case "channel":
			if al.channelManager == nil {
				return "Channel manager not initialized", true
			}
			if _, exists := al.channelManager.GetChannel(value); !exists && value != "cli" {
				return fmt.Sprintf("Channel '%s' not found or not enabled", value), true
			}
			return fmt.Sprintf("Switched target channel to %s", value), true
		default:
			return fmt.Sprintf("Unknown switch target: %s", target), true
		}
	case "/optimize-prompts":
		return al.handleOptimizePromptsCommand(ctx, args), true
	}

	return "", false
}

// handleSessionCommand handles commands that need session context (agent + sessionKey).
// Called after routing, before delegation/LLM processing.
func (al *AgentLoop) handleSessionCommand(
	_ context.Context, msg bus.InboundMessage, agent *AgentInstance, sessionKey string,
) (string, bool) {
	content := strings.TrimSpace(msg.Content)
	if !strings.HasPrefix(content, "/") {
		return "", false
	}

	parts := strings.Fields(content)
	if len(parts) == 0 {
		return "", false
	}

	cmd := parts[0]
	args := parts[1:]

	switch cmd {
	case "/new", "/reset":
		if err := agent.Sessions.DeleteSession(sessionKey); err != nil {
			return fmt.Sprintf("Failed to reset session: %v", err), true
		}
		al.usageTracker.Reset(sessionKey)
		al.verboseMode.Delete(sessionKey)
		al.thinkingLevel.Delete(sessionKey)
		al.elevatedMgr.Revoke(sessionKey)
		al.personaManager.Clear(sessionKey)
		return "Session cleared. Starting fresh.", true

	case "/compact":
		al.forceCompression(agent, sessionKey)
		return "Session compacted. History summarized.", true

	case "/checkpoint":
		return al.handleCheckpointCommand(args, agent, sessionKey), true

	case "/health":
		return al.handleHealthCommand(args, agent, sessionKey), true

	case "/pause":
		return al.handlePauseCommand(args, agent, sessionKey), true

	case "/resume":
		return al.handleResumeCommand(args, agent, sessionKey), true

	case "/verbose":
		if len(args) < 1 {
			if v, ok := al.verboseMode.Load(sessionKey); ok && v.(bool) {
				return "Verbose mode is ON. Usage: /verbose on|off", true
			}
			return "Verbose mode is OFF. Usage: /verbose on|off", true
		}
		switch args[0] {
		case "on":
			al.verboseMode.Store(sessionKey, true)
			return "Verbose mode enabled. Reasoning content will be shown.", true
		case "off":
			al.verboseMode.Store(sessionKey, false)
			return "Verbose mode disabled.", true
		default:
			return "Usage: /verbose on|off", true
		}

	case "/yolo":
		if al.approvalGate == nil {
			return "Approval gate is not configured for this agent.", true
		}
		if len(args) < 1 {
			if al.approvalGate.IsBypassed(sessionKey) {
				return "YOLO mode is ON — all approvals bypassed. Usage: /yolo on|off", true
			}
			return "YOLO mode is OFF. Usage: /yolo on|off", true
		}
		switch args[0] {
		case "on":
			al.approvalGate.SetBypass(sessionKey, true)
			return "YOLO mode enabled — all tool approvals bypassed for this session.", true
		case "off":
			al.approvalGate.SetBypass(sessionKey, false)
			return "YOLO mode disabled — normal approval rules restored.", true
		default:
			return "Usage: /yolo on|off", true
		}

	case "/usage":
		usage := al.usageTracker.GetSession(sessionKey)
		if usage == nil {
			return "No usage data for this session yet.", true
		}
		return fmt.Sprintf(
			"Session usage:\n"+
				"  Model: %s\n"+
				"  Calls: %d\n"+
				"  Prompt tokens: %d\n"+
				"  Completion tokens: %d\n"+
				"  Total tokens: %d",
			agent.Model, usage.CallCount,
			usage.PromptTokens, usage.CompletionTokens, usage.TotalTokens,
		), true

	case "/think":
		if len(args) < 1 {
			if v, ok := al.thinkingLevel.Load(sessionKey); ok {
				return fmt.Sprintf("Current thinking level: %s. Usage: /think off|minimal|low|medium|high", v), true
			}
			return "Thinking level: default. Usage: /think off|minimal|low|medium|high", true
		}
		level := ThinkingLevel(args[0])
		if !IsValidThinkingLevel(level) {
			return "Invalid level. Options: off, minimal, low, medium, high", true
		}
		if level == ThinkingOff {
			al.thinkingLevel.Delete(sessionKey)
			return "Thinking mode disabled.", true
		}
		al.thinkingLevel.Store(sessionKey, level)
		budget := ThinkingBudgetTokens(level)
		return fmt.Sprintf("Thinking level set to %s (budget: %d tokens).", level, budget), true

	case "/elevated":
		if len(args) < 1 {
			if al.elevatedMgr.IsElevated(sessionKey) {
				state := al.elevatedMgr.GetState(sessionKey)
				remaining := time.Until(state.ExpiresAt).Round(time.Second)
				return fmt.Sprintf(
					"Elevated mode is ON (%s remaining). Usage: /elevated on|off", remaining,
				), true
			}
			return "Elevated mode is OFF. Usage: /elevated on|off", true
		}
		switch args[0] {
		case "on":
			al.elevatedMgr.Elevate(sessionKey, msg.SenderID, msg.Channel, 30*time.Minute)
			return "Elevated mode enabled for 30 minutes. Some shell restrictions relaxed.", true
		case "off":
			al.elevatedMgr.Revoke(sessionKey)
			return "Elevated mode disabled.", true
		default:
			return "Usage: /elevated on|off", true
		}

	case "/persona":
		return al.handlePersonaCommand(args, sessionKey), true

	case "/role":
		return al.handleRoleCommand(args, sessionKey), true

	case "/branch":
		return al.handleBranchCommand(args, agent, sessionKey), true

	case "/branches":
		return al.handleBranchesCommand(sessionKey), true

	case "/search":
		if len(args) == 0 {
			return "Usage: /search <query>", true
		}
		return al.handleSearchCommand(strings.Join(args, " ")), true

	case "/evolve":
		return al.handleEvolveCommand(args, sessionKey)
	}

	return "", false
}

// handlePersonaCommand implements the /persona session command.
func (al *AgentLoop) handlePersonaCommand(args []string, sessionKey string) string {
	if len(args) == 0 {
		// Show current persona and list available
		names := al.personaManager.List()
		if len(names) == 0 {
			return "No personas configured. Add personas to agents.defaults.personas in config.json."
		}

		active := al.personaManager.GetActive(sessionKey)
		var sb strings.Builder
		if active != nil {
			fmt.Fprintf(&sb, "Active persona: %s", active.Name)
			if active.Description != "" {
				fmt.Fprintf(&sb, " — %s", active.Description)
			}
			sb.WriteString("\n\n")
		} else {
			sb.WriteString("No active persona (using default).\n\n")
		}
		sb.WriteString("Available personas:\n")
		for _, name := range names {
			p := al.personaManager.GetActive(sessionKey)
			// Look up the persona by name for its description
			_ = al.personaManager.Switch("__peek__", name)
			peeked := al.personaManager.GetActive("__peek__")
			al.personaManager.Clear("__peek__")

			marker := "  "
			if p != nil && p.Name == name {
				marker = "* "
			}
			if peeked != nil && peeked.Description != "" {
				fmt.Fprintf(&sb, "%s%s — %s\n", marker, name, peeked.Description)
			} else {
				fmt.Fprintf(&sb, "%s%s\n", marker, name)
			}
		}
		return sb.String()
	}

	target := args[0]

	if target == "off" {
		al.personaManager.Clear(sessionKey)
		return "Persona cleared. Using default behavior."
	}

	if err := al.personaManager.Switch(sessionKey, target); err != nil {
		names := al.personaManager.List()
		return fmt.Sprintf(
			"Unknown persona %q. Available: %s",
			target, strings.Join(names, ", "),
		)
	}

	p := al.personaManager.GetActive(sessionKey)
	if p != nil && p.Description != "" {
		return fmt.Sprintf("Switched to persona: %s — %s", p.Name, p.Description)
	}
	return fmt.Sprintf("Switched to persona: %s", target)
}

// handleRoleCommand implements the /role session command. It applies a built-in
// role template as a temporary persona for the current session.
func (al *AgentLoop) handleRoleCommand(args []string, sessionKey string) string {
	if len(args) == 0 {
		// List available roles and indicate which is active.
		roles := ListBuiltinRoles()
		active := al.personaManager.GetActive(sessionKey)

		var sb strings.Builder
		if active != nil && strings.HasPrefix(active.Name, "role:") {
			roleName := strings.TrimPrefix(active.Name, "role:")
			fmt.Fprintf(&sb, "Active role: %s\n\n", roleName)
		} else {
			sb.WriteString("No active role.\n\n")
		}
		sb.WriteString("Available roles:\n")
		for _, r := range roles {
			fmt.Fprintf(&sb, "  %s — %s\n", strings.ToLower(r.Name), r.Description)
		}
		sb.WriteString("\nUsage: /role <name> | /role off")
		return sb.String()
	}

	target := strings.ToLower(args[0])

	if target == "off" {
		active := al.personaManager.GetActive(sessionKey)
		if active != nil && strings.HasPrefix(active.Name, "role:") {
			roleName := strings.TrimPrefix(active.Name, "role:")
			al.personaManager.Unregister("role:" + roleName)
		}
		al.personaManager.Clear(sessionKey)
		return "Role cleared. Using default behavior."
	}

	role, ok := GetBuiltinRole(target)
	if !ok {
		names := make([]string, 0, len(BuiltinRoles))
		for k := range BuiltinRoles {
			names = append(names, k)
		}
		return fmt.Sprintf("Unknown role %q. Available: %s", target, strings.Join(names, ", "))
	}

	// Clear any previously registered role persona for this session.
	active := al.personaManager.GetActive(sessionKey)
	if active != nil && strings.HasPrefix(active.Name, "role:") {
		al.personaManager.Unregister(active.Name)
	}

	// Register the role as a temporary persona and switch to it.
	personaName := "role:" + target
	al.personaManager.Register(personaName, &Persona{
		Name:         personaName,
		Description:  role.Description,
		SystemPrompt: role.SystemPrompt,
		AllowedTools: role.SuggestedTools,
	})

	if err := al.personaManager.Switch(sessionKey, personaName); err != nil {
		return fmt.Sprintf("Failed to apply role: %v", err)
	}

	return fmt.Sprintf("Switched to role: %s — %s", role.Name, role.Description)
}

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

// handleSearchCommand searches across all sessions for messages matching the query.
func (al *AgentLoop) handleSearchCommand(query string) string {
	if al.memDB == nil {
		return "Search unavailable: memory database not initialized."
	}

	dbRows, err := al.memDB.SearchMessages(query, 500)
	if err != nil {
		return fmt.Sprintf("Search failed: %v", err)
	}

	if len(dbRows) == 0 {
		return "No results found."
	}

	entries := dbRowsToSearchEntries(dbRows)
	results := search.KeywordSearch(query, entries, 10)
	if len(results) == 0 {
		return "No results found."
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "Search results for %q (%d matches):\n\n", query, len(results))
	for i, r := range results {
		preview := r.Content
		if len(preview) > 120 {
			preview = preview[:120] + "..."
		}
		fmt.Fprintf(&sb, "%d. [%s] %s (session: %s, score: %.0f%%)\n",
			i+1, r.Role, preview, r.SessionKey, r.Score*100)
	}
	return strings.TrimRight(sb.String(), "\n")
}

// dbRowsToSearchEntries converts memory.SearchMessageRow slices to search.MessageEntry slices.
func dbRowsToSearchEntries(rows []memory.SearchMessageRow) []search.MessageEntry {
	entries := make([]search.MessageEntry, len(rows))
	for i, r := range rows {
		entries[i] = search.MessageEntry{
			SessionKey: r.SessionKey,
			Content:    r.Content,
			Role:       r.Role,
			Timestamp:  r.CreatedAt,
		}
	}
	return entries
}

// handleEvolveCommand implements the /evolve family of session commands.
func (al *AgentLoop) handleEvolveCommand(args []string, _ string) (string, bool) {
	if al.evolutionEngine == nil {
		return "Evolution engine is not enabled. Set evolution.enabled=true in config.", true
	}
	if len(args) == 0 {
		return "Usage: /evolve status|history|run|pause|resume|revert <id>", true
	}
	switch args[0] {
	case "status":
		return al.evolutionEngine.FormatStatus(), true
	case "history":
		n := 10
		if len(args) > 1 {
			if parsed, err := strconv.Atoi(args[1]); err == nil && parsed > 0 {
				n = parsed
			}
		}
		entries, err := al.evolutionEngine.RecentHistory(n)
		if err != nil {
			return fmt.Sprintf("Error: %v", err), true
		}
		if len(entries) == 0 {
			return "No evolution history yet.", true
		}
		var sb strings.Builder
		sb.WriteString("Evolution History:\n")
		for _, e := range entries {
			outcome := ""
			if e.Outcome != "" {
				outcome = fmt.Sprintf(" [%s]", e.Outcome)
			}
			fmt.Fprintf(&sb, "  %s | %s | %s%s\n",
				e.Timestamp.Format("01-02 15:04"), e.Action, e.Summary, outcome)
		}
		return sb.String(), true
	case "run":
		if al.evolveRunning.Load() {
			return "An evolution cycle is already running.", true
		}
		al.evolveRunning.Store(true)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		go func() {
			defer cancel()
			defer al.evolveRunning.Store(false)
			al.evolutionEngine.RunNow(ctx)
		}()
		return "Evolution cycle triggered.", true
	case "pause":
		al.evolutionEngine.Pause()
		return "Evolution paused.", true
	case "resume":
		al.evolutionEngine.Resume()
		return "Evolution resumed.", true
	case "revert":
		if len(args) < 2 {
			return "Usage: /evolve revert <id>", true
		}
		if err := al.evolutionEngine.Revert(args[1]); err != nil {
			return fmt.Sprintf("Revert failed: %v", err), true
		}
		return fmt.Sprintf("Reverted action %s.", args[1]), true
	default:
		return "Unknown. Usage: /evolve status|history|run|pause|resume|revert <id>", true
	}
}

// handleOptimizePromptsCommand implements /optimize-prompts [agent_id].
func (al *AgentLoop) handleOptimizePromptsCommand(ctx context.Context, args []string) string {
	if !al.cfg.Agents.Defaults.PromptOptimization.Enabled {
		return "Prompt optimization is disabled. Enable it in config.json: agents.defaults.prompt_optimization.enabled = true"
	}

	agentID := ""
	if len(args) > 0 {
		agentID = args[0]
	}

	// Default to the default agent
	agent := al.getRegistry().GetDefaultAgent()
	if agentID != "" {
		if a, ok := al.getRegistry().GetAgent(agentID); ok {
			agent = a
		} else {
			return fmt.Sprintf("Agent %q not found.", agentID)
		}
	}
	if agent == nil {
		return "No agent available."
	}

	optimizer := NewPromptOptimizer(
		al.tracer, al.memDB, agent.Provider,
		al.cfg.Agents.Defaults.PromptOptimization,
	)

	// Step 1: Evaluate
	review, err := optimizer.Evaluate(agent.ID)
	if err != nil {
		return fmt.Sprintf("Evaluation failed: %v", err)
	}

	if !review.NeedsWork {
		return fmt.Sprintf(
			"Agent %s is performing well (avg score: %.2f). No optimization needed.",
			agent.ID,
			review.AvgScore,
		)
	}

	// Step 2: Get current system prompt
	currentPrompt := ""
	if agent.ContextBuilder != nil {
		currentPrompt = agent.ContextBuilder.BuildSystemPrompt()
	}
	if currentPrompt == "" {
		return "Could not read current system prompt for optimization."
	}

	// Step 3: Generate variants
	variants, err := optimizer.GenerateVariants(ctx, review, currentPrompt)
	if err != nil {
		return fmt.Sprintf("Variant generation failed: %v", err)
	}

	// Step 4: Create experiment
	exp, err := optimizer.CreateExperiment(agent.ID, variants)
	if err != nil {
		return fmt.Sprintf("Failed to create A/B experiment: %v", err)
	}

	return fmt.Sprintf(
		"Prompt optimization started for agent %s:\n"+
			"• Avg score: %.2f (threshold: %.2f)\n"+
			"• Low-scoring traces: %d\n"+
			"• Experiment: %s\n"+
			"• Variants: %d (including control)\n"+
			"• Trials needed: %d per variant\n\n"+
			"The experiment will run automatically. Check results with /evolve status.",
		agent.ID, review.AvgScore, al.cfg.Agents.Defaults.PromptOptimization.ScoreThreshold,
		len(review.LowTraces), exp.Name, len(variants),
		al.cfg.Agents.Defaults.PromptOptimization.TrialsPerVariant,
	)
}
