package agent

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

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
			oldModel := defaultAgent.Model
			defaultAgent.Model = value
			_, defaultAgent.ModelID = providers.ExtractProtocol(mc.Model)
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

	case "/usage":
		usage := al.usageTracker.GetSession(sessionKey)
		if usage == nil {
			return "No usage data for this session yet.", true
		}
		cost := EstimateCost(usage, agent.Model)
		costStr := "unknown"
		if cost > 0 {
			costStr = fmt.Sprintf("$%.4f", cost)
		}
		return fmt.Sprintf(
			"Session usage:\n"+
				"  Model: %s\n"+
				"  Calls: %d\n"+
				"  Prompt tokens: %d\n"+
				"  Completion tokens: %d\n"+
				"  Total tokens: %d\n"+
				"  Estimated cost: %s",
			agent.Model, usage.CallCount,
			usage.PromptTokens, usage.CompletionTokens, usage.TotalTokens,
			costStr,
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
			sb.WriteString(fmt.Sprintf("Active persona: %s", active.Name))
			if active.Description != "" {
				sb.WriteString(fmt.Sprintf(" — %s", active.Description))
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
				sb.WriteString(fmt.Sprintf("%s%s — %s\n", marker, name, peeked.Description))
			} else {
				sb.WriteString(fmt.Sprintf("%s%s\n", marker, name))
			}
		}
		return sb.String()
	}

	target := args[0]

	if target == "off" {
		al.personaManager.Clear(sessionKey)
		return "Persona cleared. Using default behaviour."
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
			sb.WriteString(fmt.Sprintf("Active role: %s\n\n", roleName))
		} else {
			sb.WriteString("No active role.\n\n")
		}
		sb.WriteString("Available roles:\n")
		for _, r := range roles {
			sb.WriteString(fmt.Sprintf("  %s — %s\n", strings.ToLower(r.Name), r.Description))
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
		return "Role cleared. Using default behaviour."
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
	rootKey := sessionKey
	for {
		parent, ok := al.branchManager.GetParent(rootKey)
		if !ok {
			break
		}
		rootKey = parent
	}

	branches := al.branchManager.ListBranches(rootKey)
	if len(branches) == 0 {
		return "No branches for this session."
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Branches of %s:\n", rootKey))
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
	sb.WriteString(fmt.Sprintf("Search results for %q (%d matches):\n\n", query, len(results)))
	for i, r := range results {
		preview := r.Content
		if len(preview) > 120 {
			preview = preview[:120] + "..."
		}
		sb.WriteString(fmt.Sprintf(
			"%d. [%s] %s (session: %s, score: %.0f%%)\n",
			i+1, r.Role, preview, r.SessionKey, r.Score*100,
		))
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
			sb.WriteString(fmt.Sprintf("  %s | %s | %s%s\n",
				e.Timestamp.Format("01-02 15:04"), e.Action, e.Summary, outcome))
		}
		return sb.String(), true
	case "run":
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		go func() {
			defer cancel()
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
