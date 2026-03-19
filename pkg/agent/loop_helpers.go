package agent

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/grasberg/sofia/pkg/autonomy"
	"github.com/grasberg/sofia/pkg/bus"
	"github.com/grasberg/sofia/pkg/logger"
	"github.com/grasberg/sofia/pkg/notifications"
	"github.com/grasberg/sofia/pkg/providers"
	"github.com/grasberg/sofia/pkg/routing"
	"github.com/grasberg/sofia/pkg/tools"
	"github.com/grasberg/sofia/pkg/utils"
)

func (al *AgentLoop) startAutonomyServices(provider providers.LLMProvider, pushService *notifications.PushService) {
	al.registryMu.RLock()
	defer al.registryMu.RUnlock()

	ctx := context.Background()

	for _, agentID := range al.registry.ListAgentIDs() {
		agent, ok := al.registry.GetAgent(agentID)
		if !ok {
			continue
		}

		subagentManager := tools.NewSubagentManager(agent.Provider, agent.ModelID, agent.Workspace, al.bus)
		subagentManager.SetAgentTaskRunner(al.runSpawnedTaskAsAgent)
		subagentManager.SetSkillsLoader(agent.ContextBuilder.GetSkillsLoader())

		svc := autonomy.NewService(
			&al.cfg.Autonomy,
			al.memDB,
			al.bus,
			provider,
			subagentManager,
			agentID,
			agent.ModelID,
			agent.Workspace,
			pushService,
		)
		svc.SetDashboardHub(al.dashboardHub)
		svc.SetTaskRunner(al.runSpawnedTaskAsAgent)
		if al.state != nil {
			svc.SetLastChannelFunc(al.state.GetLastChannel)
		}

		if err := svc.Start(ctx); err == nil {
			al.autonomyServices[agentID] = svc
		}
	}
}

func (al *AgentLoop) stopAutonomyServices() {
	al.registryMu.Lock()
	defer al.registryMu.Unlock()

	for _, svc := range al.autonomyServices {
		if svc != nil {
			svc.Stop()
		}
	}
	al.autonomyServices = make(map[string]*autonomy.Service)
}

// updateToolContexts updates the context for tools that need channel/chatID info.
func (al *AgentLoop) updateToolContexts(agent *AgentInstance, channel, chatID string) {
	contextualTools := []string{"message", "spawn", "subagent", "orchestrate"}
	for _, name := range contextualTools {
		if tool, ok := agent.Tools.Get(name); ok {
			if ct, ok := tool.(tools.ContextualTool); ok {
				ct.SetContext(channel, chatID)
			}
		}
	}
}

// correctionPatterns detects user messages that contain corrections or preferences.
var correctionPatterns = []string{
	"no, ",
	"actually,",
	"actually ",
	"i meant",
	"i prefer",
	"always use",
	"never use",
	"don't use",
	"do not use",
	"please always",
	"please never",
	"from now on",
	"remember that",
	"keep in mind",
	"i want you to",
	"use this instead",
	"that's wrong",
	"that's not right",
	"incorrect",
	"that is wrong",
}

// maybLearnFromFeedback checks if the user message contains correction patterns
// and extracts preferences to store in long-term memory.
func (al *AgentLoop) maybLearnFromFeedback(agent *AgentInstance, userMsg string) {
	if userMsg == "" || al.memDB == nil {
		return
	}

	lower := strings.ToLower(userMsg)
	isCorrection := false
	for _, pattern := range correctionPatterns {
		if strings.Contains(lower, pattern) {
			isCorrection = true
			break
		}
	}

	if !isCorrection {
		return
	}

	// Extract and store the preference
	memStore := NewMemoryStore(al.memDB, agent.ID)
	existing := memStore.ReadLongTerm()

	// Truncate user message for storage
	preference := userMsg
	if len(preference) > 200 {
		preference = preference[:200] + "..."
	}

	entry := fmt.Sprintf("- User preference: %s", preference)

	// Avoid duplicates
	if strings.Contains(existing, preference[:min(len(preference), 50)]) {
		return
	}

	var newContent string
	if existing == "" {
		newContent = "## User Preferences (auto-learned)\n\n" + entry
	} else {
		newContent = existing + "\n" + entry
	}

	if err := memStore.WriteLongTerm(newContent); err != nil {
		logger.WarnCF("agent", "Failed to save learned preference",
			map[string]any{"error": err.Error()})
	} else {
		logger.InfoCF("agent", "Learned user preference from feedback",
			map[string]any{"preference": utils.Truncate(preference, 80)})
	}

	// Feedback-Driven Evolution: Also append directly to USER.md
	userMDPath := filepath.Join(agent.Workspace, "USER.md")
	f, err := os.OpenFile(userMDPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err == nil {
		defer f.Close()
		if _, writeErr := f.WriteString(fmt.Sprintf("\n- Auto-learned preference from feedback: %s\n", preference)); writeErr != nil {
			logger.WarnCF("agent", "Failed to append to USER.md", map[string]any{"error": writeErr.Error()})
		}
	}
}

// maybeReflect runs a post-task self-evaluation asynchronously.
// It uses the LLM to evaluate the conversation quality and stores structured reflections.
func (al *AgentLoop) maybeReflect(
	agent *AgentInstance,
	sessionKey, finalContent string,
	iteration, errorCount int,
	durationMs int64,
) {
	if al.memDB == nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	engine := NewReflectionEngine(al.memDB, agent.ID)
	if err := engine.Reflect(ctx, agent, sessionKey, finalContent, iteration, errorCount, durationMs); err != nil {
		logger.WarnCF("reflection", "Post-task reflection failed",
			map[string]any{"agent_id": agent.ID, "error": err.Error()})
	}
}

// extractPeer extracts the routing peer from inbound message metadata.
func extractPeer(msg bus.InboundMessage) *routing.RoutePeer {
	peerKind := msg.Metadata["peer_kind"]
	if peerKind == "" {
		return nil
	}
	peerID := msg.Metadata["peer_id"]
	if peerID == "" {
		if peerKind == "direct" {
			peerID = msg.SenderID
		} else {
			peerID = msg.ChatID
		}
	}
	return &routing.RoutePeer{Kind: peerKind, ID: peerID}
}

// extractParentPeer extracts the parent peer (reply-to) from inbound message metadata.
func extractParentPeer(msg bus.InboundMessage) *routing.RoutePeer {
	parentKind := msg.Metadata["parent_peer_kind"]
	parentID := msg.Metadata["parent_peer_id"]
	if parentKind == "" || parentID == "" {
		return nil
	}
	return &routing.RoutePeer{Kind: parentKind, ID: parentID}
}

// looksLikeTask checks if a user message appears to be a task/request rather than
// a simple greeting or question. Used to decide whether to nudge the LLM to use tools.
func looksLikeTask(msg string) bool {
	msg = strings.ToLower(strings.TrimSpace(msg))
	if len(msg) < 10 {
		return false
	}
	// Simple greetings / acknowledgments are not tasks
	greetings := []string{"hej", "hello", "hi", "tack", "thanks", "ok", "okej", "bra", "good", "nice"}
	for _, g := range greetings {
		if msg == g || msg == g+"!" || msg == g+"." {
			return false
		}
	}
	// Task indicators (Swedish + English)
	taskWords := []string{
		"skapa", "skicka", "skriv", "bygg", "fixa", "gör", "kör", "lägg till", "ta bort", "uppdatera", "installera",
		"create", "build", "write", "fix", "run", "add", "remove", "update", "install", "deploy", "make", "set up",
		"generate", "implement", "configure", "send", "fetch", "download",
	}
	for _, tw := range taskWords {
		if strings.Contains(msg, tw) {
			return true
		}
	}
	// If message is long enough, it's likely a task
	return len(msg) > 80
}

// formatMessagesForLog formats messages for logging
func formatMessagesForLog(messages []providers.Message) string {
	if len(messages) == 0 {
		return "[]"
	}

	var sb strings.Builder
	sb.WriteString("[\n")
	for i, msg := range messages {
		fmt.Fprintf(&sb, "  [%d] Role: %s\n", i, msg.Role)
		if len(msg.ToolCalls) > 0 {
			sb.WriteString("  ToolCalls:\n")
			for _, tc := range msg.ToolCalls {
				fmt.Fprintf(&sb, "    - ID: %s, Type: %s, Name: %s\n", tc.ID, tc.Type, tc.Name)
				if tc.Function != nil {
					fmt.Fprintf(&sb, "      Arguments: %s\n", utils.Truncate(tc.Function.Arguments, 200))
				}
			}
		}
		if msg.Content != "" {
			content := utils.Truncate(msg.Content, 200)
			fmt.Fprintf(&sb, "  Content: %s\n", content)
		}
		if msg.ToolCallID != "" {
			fmt.Fprintf(&sb, "  ToolCallID: %s\n", msg.ToolCallID)
		}
		sb.WriteString("\n")
	}
	sb.WriteString("]")
	return sb.String()
}

// formatToolsForLog formats tool definitions for logging
func formatToolsForLog(toolDefs []providers.ToolDefinition) string {
	if len(toolDefs) == 0 {
		return "[]"
	}

	var sb strings.Builder
	sb.WriteString("[\n")
	for i, tool := range toolDefs {
		fmt.Fprintf(&sb, "  [%d] Type: %s, Name: %s\n", i, tool.Type, tool.Function.Name)
		fmt.Fprintf(&sb, "      Description: %s\n", tool.Function.Description)
		if len(tool.Function.Parameters) > 0 {
			fmt.Fprintf(&sb, "      Parameters: %s\n", utils.Truncate(fmt.Sprintf("%v", tool.Function.Parameters), 200))
		}
	}
	sb.WriteString("]")
	return sb.String()
}

// runPlanDispatcher periodically checks for unclaimed pending plan steps
// and dispatches them to available subagents in parallel.
func (al *AgentLoop) runPlanDispatcher(ctx context.Context) {
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			al.dispatchPendingSteps(ctx)
		}
	}
}

func (al *AgentLoop) dispatchPendingSteps(ctx context.Context) {
	if al.planManager == nil || !al.planManager.HasPendingSteps() {
		return
	}

	// Get all available subagent IDs
	agentIDs := al.getRegistry().ListAgentIDs()
	subagentIDs := make([]string, 0, len(agentIDs))
	for _, id := range agentIDs {
		if routing.NormalizeAgentID(id) != routing.DefaultAgentID {
			subagentIDs = append(subagentIDs, id)
		}
	}

	// If no subagents exist, use the default agent
	if len(subagentIDs) == 0 {
		subagentIDs = []string{routing.DefaultAgentID}
	}

	for _, agentID := range subagentIDs {
		planID, stepIdx, description, ok := al.planManager.ClaimPendingStep(agentID)
		if !ok {
			break // no more pending steps
		}

		agent, exists := al.getRegistry().GetAgent(agentID)
		if !exists {
			continue
		}
		agentName := agent.Name
		if agentName == "" {
			agentName = agentID
		}

		logger.InfoCF("plan-dispatch",
			fmt.Sprintf("Dispatching step #%d to %s: %s", stepIdx+1, agentName, utils.Truncate(description, 80)),
			map[string]any{"plan_id": planID, "step": stepIdx, "agent": agentID})

		al.dashboardHub.Broadcast(map[string]any{
			"type":            "plan_step_assigned",
			"agent_id":        agentID,
			"agent_name":      agentName,
			"plan_id":         planID,
			"step_index":      stepIdx,
			"step_description": utils.Truncate(description, 120),
			"from_agent_id":   routing.DefaultAgentID,
			"to_agent_id":     agentID,
			"to_agent_name":   agentName,
			"reason":          "plan_dispatch",
		})

		go func(pID string, sIdx int, desc, aID, aName string) {
			start := time.Now()
			result, err := al.runSpawnedTaskAsAgent(ctx, aID, "", desc, "cli", "plan")
			dur := time.Since(start).Milliseconds()

			if err != nil {
				logger.WarnCF("plan-dispatch",
					fmt.Sprintf("Step #%d failed (%s, %dms): %v", sIdx+1, aName, dur, err),
					map[string]any{"plan_id": pID, "step": sIdx, "agent": aID})
				al.planManager.CompleteStep(pID, sIdx, false, "Error: "+err.Error())
				al.dashboardHub.Broadcast(map[string]any{
					"type":       "plan_step_done",
					"agent_id":   aID,
					"agent_name": aName,
					"plan_id":    pID,
					"step_index": sIdx,
					"success":    false,
					"error":      err.Error(),
				})
			} else {
				logger.InfoCF("plan-dispatch",
					fmt.Sprintf("Step #%d done (%s, %dms)", sIdx+1, aName, dur),
					map[string]any{"plan_id": pID, "step": sIdx, "agent": aID, "result_len": len(result)})
				if len(result) > 500 {
					result = result[:500] + "..."
				}
				al.planManager.CompleteStep(pID, sIdx, true, result)
				al.dashboardHub.Broadcast(map[string]any{
					"type":       "plan_step_done",
					"agent_id":   aID,
					"agent_name": aName,
					"plan_id":    pID,
					"step_index": sIdx,
					"success":    true,
				})
			}
		}(planID, stepIdx, description, agentID, agentName)
	}
}

