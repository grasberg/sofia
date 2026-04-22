package agent

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/grasberg/sofia/pkg/bus"
	"github.com/grasberg/sofia/pkg/logger"
	"github.com/grasberg/sofia/pkg/providers"
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
	ctx context.Context, msg bus.InboundMessage, agent *AgentInstance, sessionKey string,
) (string, bool) {
	content := strings.TrimSpace(msg.Content)
	if !strings.HasPrefix(content, "/") {
		return "", false
	}

	// /btw must be detected before the switch — it's a prefix match, not exact.
	// Only supported in CLI and Web channels; in gateway channels treat as a normal message.
	if strings.HasPrefix(content, "/btw ") || content == "/btw" {
		if msg.Channel != "cli" && msg.Channel != "web" {
			return "[btw not supported in this channel — use a new message instead]", true
		}
		question := strings.TrimPrefix(content, "/btw")
		question = strings.TrimSpace(question)
		if question == "" {
			return "Usage: /btw <question>", true
		}
		result, err := al.runAgentLoop(ctx, agent, processOptions{
			SessionKey:      sessionKey,
			Channel:         msg.Channel,
			ChatID:          msg.ChatID,
			UserMessage:     question,
			DefaultResponse: "[btw] (no response)",
			EnableSummary:   false,
			SendResponse:    false,
			Ephemeral:       true,
		})
		if err != nil {
			return fmt.Sprintf("[btw] Error: %v", err), true
		}
		return result, true
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
			logger.WarnCF("approval", "YOLO mode enabled", map[string]any{"session_key": sessionKey})
			return "YOLO mode enabled — all tool approvals bypassed for this session.", true
		case "off":
			al.approvalGate.SetBypass(sessionKey, false)
			logger.WarnCF("approval", "YOLO mode disabled", map[string]any{"session_key": sessionKey})
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
