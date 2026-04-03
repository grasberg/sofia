package agent

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/grasberg/sofia/pkg/bus"
	"github.com/grasberg/sofia/pkg/constants"
	"github.com/grasberg/sofia/pkg/guardrails"
	"github.com/grasberg/sofia/pkg/logger"
	"github.com/grasberg/sofia/pkg/providers"
	"github.com/grasberg/sofia/pkg/reputation"
	"github.com/grasberg/sofia/pkg/routing"
	"github.com/grasberg/sofia/pkg/tools"
	"github.com/grasberg/sofia/pkg/trace"
	"github.com/grasberg/sofia/pkg/utils"
)

func (al *AgentLoop) runSpawnedTaskAsAgent(
	ctx context.Context,
	agentID, sessionKey, task, originChannel, originChatID string,
) (string, error) {
	if al.killed.Load() {
		return "", context.Canceled
	}

	target, ok := al.getRegistry().GetAgent(agentID)
	if !ok || target == nil {
		return "", fmt.Errorf("target agent %q not found", agentID)
	}

	if sessionKey == "" {
		sessionKey = "subagent:" + agentID
	}

	agentComp := fmt.Sprintf("agent:%s", agentID)
	agentName := target.Name
	if agentName == "" {
		agentName = agentID
	}
	taskPreview := utils.Truncate(task, 120)
	logger.InfoCF(agentComp, fmt.Sprintf("SUBAGENT: task started — %s", taskPreview),
		map[string]any{
			"agent_id":     agentID,
			"agent_name":   agentName,
			"model":        target.Model,
			"session_key":  sessionKey,
			"task_preview": taskPreview,
		})

	al.dashboardHub.Broadcast(map[string]any{
		"type":        "subagent_task_start",
		"agent_id":    agentID,
		"agent_name":  agentName,
		"task":        task,
		"session_key": sessionKey,
	})

	// Trace: delegation span (if a parent trace exists from the caller, we don't have
	// it here since this is a standalone spawn — start a fresh trace)
	var delegationSpan *trace.Span
	if al.tracer != nil {
		delegationSpan = al.tracer.StartTrace(agentID, sessionKey, "delegation:"+agentName)
		delegationSpan.Kind = trace.SpanDelegation
		delegationSpan.Attributes["task_preview"] = taskPreview
		delegationSpan.Attributes["origin_channel"] = originChannel
	}

	start := time.Now()
	result, err := al.runAgentLoop(ctx, target, processOptions{
		SessionKey:      sessionKey,
		Channel:         originChannel,
		ChatID:          originChatID,
		UserMessage:     task,
		DefaultResponse: defaultResponse,
		EnableSummary:   false,
		SendResponse:    false,
		NoHistory:       true,
		ParentSpan:      delegationSpan,
	})
	dur := time.Since(start).Milliseconds()

	// End delegation span
	if al.tracer != nil && delegationSpan != nil {
		status := trace.StatusOK
		attrs := map[string]any{"duration_ms": dur, "result_len": len(result)}
		if err != nil {
			status = trace.StatusError
			attrs["error"] = err.Error()
		}
		al.tracer.EndSpan(delegationSpan, status, attrs)
	}

	if err != nil {
		logger.WarnCF(agentComp, fmt.Sprintf("SUBAGENT: task failed after %dms", dur),
			map[string]any{
				"agent_id":    agentID,
				"agent_name":  agentName,
				"duration_ms": dur,
				"error":       err.Error(),
			})
		al.recordReputation(agentID, task, false, dur, err.Error())

		al.dashboardHub.Broadcast(map[string]any{
			"type":        "subagent_task_end",
			"agent_id":    agentID,
			"agent_name":  agentName,
			"session_key": sessionKey,
			"success":     false,
			"error":       err.Error(),
			"duration_ms": dur,
		})

		return result, err
	}

	logger.InfoCF(agentComp, fmt.Sprintf("SUBAGENT: task completed in %dms", dur),
		map[string]any{
			"agent_id":       agentID,
			"agent_name":     agentName,
			"duration_ms":    dur,
			"result_len":     len(result),
			"result_preview": utils.Truncate(result, 160),
		})
	al.recordReputation(agentID, task, true, dur, "")

	al.dashboardHub.Broadcast(map[string]any{
		"type":        "subagent_task_end",
		"agent_id":    agentID,
		"agent_name":  agentName,
		"session_key": sessionKey,
		"success":     true,
		"result":      result,
		"duration_ms": dur,
	})

	return result, nil
}

// recordReputation persists a task outcome for reputation tracking.
func (al *AgentLoop) recordReputation(
	agentID, task string, success bool, latencyMs int64, errMsg string,
) {
	if al.memDB == nil {
		return
	}
	mgr := reputation.NewManager(al.memDB)
	_, err := mgr.RecordOutcome(reputation.TaskOutcome{
		AgentID:   agentID,
		Task:      task,
		Success:   success,
		LatencyMs: latencyMs,
		Error:     errMsg,
	})
	if err != nil {
		logger.WarnCF("reputation",
			"Failed to record reputation outcome",
			map[string]any{
				"agent_id": agentID,
				"error":    err.Error(),
			})
	}
}

// RecordLastChannel records the last active channel for this workspace.
// This uses the atomic state save mechanism to prevent data loss on crash.
func (al *AgentLoop) RecordLastChannel(channel string) error {
	if al.state == nil {
		return nil
	}
	return al.state.SetLastChannel(channel)
}

// RecordLastChatID records the last active chat ID for this workspace.
// This uses the atomic state save mechanism to prevent data loss on crash.
func (al *AgentLoop) RecordLastChatID(chatID string) error {
	if al.state == nil {
		return nil
	}
	return al.state.SetLastChatID(chatID)
}

// broadcastPresence sends a presence_update event via the dashboard hub
// and updates the hub's internal presence state.
func (al *AgentLoop) broadcastPresence(agentID, status string) {
	al.dashboardHub.UpdatePresence(agentID, status)
	al.dashboardHub.Broadcast(map[string]any{
		"type":     "presence_update",
		"agent_id": agentID,
		"status":   status,
		"since":    time.Now().Unix(),
	})
}

func (al *AgentLoop) ProcessDirect(ctx context.Context, content, sessionKey string) (string, error) {
	return al.ProcessDirectWithChannel(ctx, content, sessionKey, "cli", "direct")
}

// ProcessDirectWithImages sends a message with optional image attachments directly
// to the default agent, bypassing channel routing. Images must be base64 data URLs.
func (al *AgentLoop) ProcessDirectWithImages(
	ctx context.Context,
	content, sessionKey string,
	images []string,
) (string, error) {
	agent := al.getRegistry().GetDefaultAgent()
	if agent == nil {
		return "", fmt.Errorf("no default agent configured")
	}
	return al.runAgentLoop(ctx, agent, processOptions{
		SessionKey:      sessionKey,
		Channel:         "cli",
		ChatID:          "direct",
		UserMessage:     content,
		UserImages:      images,
		DefaultResponse: defaultResponse,
		EnableSummary:   true,
		SendResponse:    false,
	})
}

func (al *AgentLoop) ProcessDirectWithChannel(
	ctx context.Context,
	content, sessionKey, channel, chatID string,
) (string, error) {
	msg := bus.InboundMessage{
		Channel:    channel,
		SenderID:   "cron",
		ChatID:     chatID,
		Content:    content,
		SessionKey: sessionKey,
	}

	return al.processMessage(ctx, msg)
}

// ProcessHeartbeat processes a heartbeat request without session history.
// Each heartbeat is independent and doesn't accumulate context.
func (al *AgentLoop) ProcessHeartbeat(ctx context.Context, content, channel, chatID string) (string, error) {
	agent := al.getRegistry().GetDefaultAgent()
	if agent == nil {
		return "", fmt.Errorf("no default agent configured")
	}
	return al.runAgentLoop(ctx, agent, processOptions{
		SessionKey:      "heartbeat",
		Channel:         channel,
		ChatID:          chatID,
		UserMessage:     content,
		DefaultResponse: defaultResponse,
		EnableSummary:   false,
		SendResponse:    false,
		NoHistory:       true,                   // Don't load session history for heartbeat
		ModelOverride:   al.cfg.Heartbeat.Model, // Use dedicated heartbeat model if configured
	})
}

func (al *AgentLoop) processMessage(ctx context.Context, msg bus.InboundMessage) (result string, err error) {
	// Killswitch check — abort immediately if Reset() was called.
	if al.killed.Load() {
		return "", context.Canceled
	}

	preview := utils.Truncate(msg.Content, 120)
	logger.InfoCF("agent:main", fmt.Sprintf("SOFIA: message received — %s", preview),
		map[string]any{
			"channel":     msg.Channel,
			"chat_id":     msg.ChatID,
			"sender_id":   msg.SenderID,
			"session_key": msg.SessionKey,
			"preview":     preview,
		})

	// Route system messages to processSystemMessage
	if msg.Channel == "system" {
		return al.processSystemMessage(ctx, msg)
	}

	// Guardrail: Input Validation
	if al.cfg.Guardrails.InputValidation.Enabled {
		if maxLen := al.cfg.Guardrails.InputValidation.MaxMessageLength; maxLen > 0 && len(msg.Content) > maxLen {
			errMsg := fmt.Sprintf("Error: message exceeds maximum allowed length of %d characters.", maxLen)
			logger.WarnCF("agent:main", "Guardrail blocked input: length exceeded", map[string]any{
				"length":     len(msg.Content),
				"max_length": maxLen,
			})
			logger.Audit("Input Validation Blocked", map[string]any{
				"reason":     "length exceeded",
				"length":     len(msg.Content),
				"max_length": maxLen,
				"sender":     msg.SenderID,
				"channel":    msg.Channel,
			})
			return errMsg, nil
		}

		for _, pattern := range al.cfg.Guardrails.InputValidation.DenyPatterns {
			if re := getCachedRegex(pattern); re != nil && re.MatchString(msg.Content) {
				errMsg := "Error: message blocked by input security policy."
				logger.WarnCF("agent:main", "Guardrail blocked input: pattern match", map[string]any{
					"pattern": pattern,
				})
				logger.Audit("Input Validation Blocked", map[string]any{
					"reason":  "pattern match",
					"pattern": pattern,
					"sender":  msg.SenderID,
					"channel": msg.Channel,
				})
				return errMsg, nil
			}
		}
	}

	// Guardrail: Prompt Injection Defense (Heuristic check)
	if al.cfg.Guardrails.PromptInjection.Enabled {
		for _, re := range promptInjectionPatterns {
			if re.MatchString(msg.Content) {
				pattern := re.String()
				if al.cfg.Guardrails.PromptInjection.Action == "block" {
					errMsg := "Error: input rejected due to potential prompt injection attempt."
					logger.WarnCF(
						"agent:main",
						"Guardrail blocked input: prompt injection blocked",
						map[string]any{"pattern": pattern},
					)
					logger.Audit("Prompt Injection Blocked", map[string]any{
						"pattern": pattern,
						"sender":  msg.SenderID,
						"channel": msg.Channel,
					})
					return errMsg, nil
				} else {
					// Warn action - just log it and maybe append a strong warning to the system prompt later
					logger.WarnCF(
						"agent:main",
						"Guardrail detected potential prompt injection",
						map[string]any{"pattern": pattern},
					)
					logger.Audit("Prompt Injection Detected (Warn)", map[string]any{
						"pattern": pattern,
						"sender":  msg.SenderID,
						"channel": msg.Channel,
					})
				}
			}
		}
	}

	// Guardrail: PII Detection
	if al.cfg.Guardrails.PIIDetection.Enabled {
		piiAction := al.cfg.Guardrails.PIIDetection.Action
		if piiAction == "" {
			piiAction = "warn"
		}
		switch piiAction {
		case "block":
			piiMatches := guardrails.DetectPII(msg.Content)
			if len(piiMatches) > 0 {
				piiTypes := make([]string, 0, len(piiMatches))
				for _, m := range piiMatches {
					piiTypes = append(piiTypes, string(m.Type))
				}
				logger.WarnCF("agent:main", "Guardrail blocked input: PII detected", map[string]any{
					"pii_types": piiTypes,
					"count":     len(piiMatches),
				})
				logger.Audit("PII Detection Blocked", map[string]any{
					"pii_types": piiTypes,
					"count":     len(piiMatches),
					"sender":    msg.SenderID,
					"channel":   msg.Channel,
				})
				return "Error: message blocked — personal information detected.", nil
			}
		case "redact":
			redacted, piiMatches := guardrails.RedactPII(msg.Content)
			if len(piiMatches) > 0 {
				piiTypes := make([]string, 0, len(piiMatches))
				for _, m := range piiMatches {
					piiTypes = append(piiTypes, string(m.Type))
				}
				logger.WarnCF("agent:main", "Guardrail redacted PII from input", map[string]any{
					"pii_types": piiTypes,
					"count":     len(piiMatches),
				})
				logger.Audit("PII Detection Redacted", map[string]any{
					"pii_types": piiTypes,
					"count":     len(piiMatches),
					"sender":    msg.SenderID,
					"channel":   msg.Channel,
				})
				msg.Content = redacted
			}
		default: // "warn"
			piiMatches := guardrails.DetectPII(msg.Content)
			if len(piiMatches) > 0 {
				piiTypes := make([]string, 0, len(piiMatches))
				for _, m := range piiMatches {
					piiTypes = append(piiTypes, string(m.Type))
				}
				logger.WarnCF("agent:main", "PII detected in input (warn mode)", map[string]any{
					"pii_types": piiTypes,
					"count":     len(piiMatches),
				})
				logger.Audit("PII Detection Warning", map[string]any{
					"pii_types": piiTypes,
					"count":     len(piiMatches),
					"sender":    msg.SenderID,
					"channel":   msg.Channel,
				})
			}
		}
	}

	// Check for commands
	if response, handled := al.handleCommand(ctx, msg); handled {
		return response, nil
	}

	// Route to determine agent and session key
	route := al.getRegistry().ResolveRoute(routing.RouteInput{
		Channel:    msg.Channel,
		AccountID:  msg.Metadata["account_id"],
		Peer:       extractPeer(msg),
		ParentPeer: extractParentPeer(msg),
		GuildID:    msg.Metadata["guild_id"],
		TeamID:     msg.Metadata["team_id"],
	})

	agent, ok := al.getRegistry().GetAgent(route.AgentID)
	if !ok {
		agent = al.getRegistry().GetDefaultAgent()
	}
	if agent == nil {
		return "", fmt.Errorf("no agent available for route %q", route.AgentID)
	}

	al.activeAgentID.Store(agent.ID)
	al.activeStatus.Store("Thinking...")
	al.broadcastPresence(agent.ID, "processing")
	defer func() {
		al.activeAgentID.Store("")
		al.activeStatus.Store("Idle")
		al.broadcastPresence("", "idle")
	}()
	agentName := agent.Name
	if agentName == "" {
		agentName = agent.ID
	}

	al.dashboardHub.Broadcast(map[string]any{
		"type":       "agent_thinking",
		"agent_id":   agent.ID,
		"agent_name": agentName,
		"model":      agent.Model,
	})
	al.dashboardHub.Broadcast(map[string]any{
		"type":       "message_routed",
		"agent_id":   agent.ID,
		"agent_name": agentName,
		"channel":    msg.Channel,
		"sender_id":  msg.SenderID,
	})

	// Use routed session key, but honor pre-set agent-scoped keys (for ProcessDirect/cron)
	sessionKey := route.SessionKey
	if msg.SessionKey != "" && strings.HasPrefix(msg.SessionKey, "agent:") {
		sessionKey = msg.SessionKey
	}

	// Trace: create root span for this request
	var rootSpan *trace.Span
	if al.tracer != nil {
		rootSpan = al.tracer.StartTrace(agent.ID, sessionKey, "processMessage")
		rootSpan.Attributes["channel"] = msg.Channel
		rootSpan.Attributes["sender_id"] = msg.SenderID
		rootSpan.Attributes["content_preview"] = utils.Truncate(msg.Content, 200)
		defer func() {
			status := trace.StatusOK
			if err != nil {
				status = trace.StatusError
				rootSpan.Attributes["error"] = err.Error()
			}
			al.tracer.EndSpan(rootSpan, status, nil)
		}()
	}

	agentComp := fmt.Sprintf("agent:%s", agent.ID)
	logger.InfoCF(agentComp, fmt.Sprintf("ROUTER: routed to agent %q via %s", agent.ID, route.MatchedBy),
		map[string]any{
			"agent_id":    agent.ID,
			"session_key": sessionKey,
			"matched_by":  route.MatchedBy,
			"channel":     msg.Channel,
			"chat_id":     msg.ChatID,
		})

	// Check for session-aware commands (need agent + sessionKey)
	if response, handled := al.handleSessionCommand(ctx, msg, agent, sessionKey); handled {
		return response, nil
	}

	// --- Auto-spawn agents for capabilities/skills not covered by any existing agent ---
	missingCaps := al.findMissingCapabilities(msg.Content)
	for _, cap := range missingCaps {
		newAgent, err := al.spawnAgentForCapability(cap)
		if err != nil {
			logger.WarnCF(agentComp, "Failed to auto-create agent for capability",
				map[string]any{"capability": cap.ID, "error": err.Error()})
		} else {
			logger.InfoCF(agentComp,
				fmt.Sprintf("Auto-created %s agent %q", cap.Name, newAgent.Name),
				map[string]any{"agent_id": newAgent.ID, "name": newAgent.Name, "capability": cap.ID})
		}
	}
	// Also check workspace skills not covered
	missingSkills := al.findMissingSkills(msg.Content)
	if len(missingSkills) > 0 {
		newAgent, err := al.spawnAgentForSkills(missingSkills)
		if err != nil {
			logger.WarnCF(agentComp, "Failed to auto-create agent for missing skills",
				map[string]any{"skills": missingSkills, "error": err.Error()})
		} else {
			logger.InfoCF(agentComp,
				fmt.Sprintf("Auto-created agent %q for skills %v", newAgent.Name, missingSkills),
				map[string]any{"agent_id": newAgent.ID, "name": newAgent.Name, "skills": missingSkills})
		}
	}

	// --- Multi-delegation: run ALL qualifying agents in parallel ---
	candidates := al.delegateToAll(msg.Content)
	delegationReason := "keyword_match"
	if len(candidates) == 0 {
		// Semantic fallback: ask LLM which agents should handle this
		candidates = al.semanticDelegateToAll(ctx, msg.Content)
		delegationReason = "semantic_match"
	}

	if len(candidates) > 0 {
		agentNames := make([]string, len(candidates))
		for i, c := range candidates {
			n := c.Agent.Name
			if n == "" {
				n = c.Agent.ID
			}
			agentNames[i] = fmt.Sprintf("%s(%.2f)", n, c.Score)
		}
		logger.InfoCF(agentComp,
			fmt.Sprintf("SOFIA: delegating to %d agent(s): %s", len(candidates), strings.Join(agentNames, ", ")),
			map[string]any{
				"from_agent": agent.ID,
				"count":      len(candidates),
				"agents":     agentNames,
				"reason":     delegationReason,
				"preview":    utils.Truncate(msg.Content, 120),
			},
		)

		al.activeStatus.Store(fmt.Sprintf("Delegating to %d agent(s)...", len(candidates)))
		al.broadcastPresence(agent.ID, "processing")

		delegateStart := time.Now()
		combinedResult, err := al.runMultiDelegation(ctx, candidates, msg.Content, msg.Channel, msg.ChatID)
		delegateDur := time.Since(delegateStart).Milliseconds()

		if err != nil {
			logger.WarnCF(agentComp,
				fmt.Sprintf("SOFIA: multi-delegation failed after %dms — falling back to Sofia", delegateDur),
				map[string]any{"duration_ms": delegateDur, "error": err.Error()})
		} else {
			logger.InfoCF(agentComp,
				fmt.Sprintf("SOFIA: %d agent(s) done in %dms, synthesizing results", len(candidates), delegateDur),
				map[string]any{
					"count":       len(candidates),
					"duration_ms": delegateDur,
					"result_len":  len(combinedResult),
				})
			synthesisMsg := fmt.Sprintf("[Combined results from %d subagents]\n\n%s", len(candidates), combinedResult)
			return al.runAgentLoop(ctx, agent, processOptions{
				SessionKey:      sessionKey,
				Channel:         msg.Channel,
				ChatID:          msg.ChatID,
				UserMessage:     synthesisMsg,
				DefaultResponse: defaultResponse,
				EnableSummary:   true,
				SendResponse:    false,
				ParentSpan:      rootSpan,
			})
		}
	}

	return al.runAgentLoop(ctx, agent, processOptions{
		SessionKey:      sessionKey,
		Channel:         msg.Channel,
		ChatID:          msg.ChatID,
		UserMessage:     msg.Content,
		DefaultResponse: defaultResponse,
		EnableSummary:   true,
		SendResponse:    false,
		ParentSpan:      rootSpan,
	})
}

func (al *AgentLoop) processSystemMessage(ctx context.Context, msg bus.InboundMessage) (string, error) {
	if msg.Channel != "system" {
		return "", fmt.Errorf("processSystemMessage called with non-system message channel: %s", msg.Channel)
	}

	logger.InfoCF("agent", "Processing system message",
		map[string]any{
			"sender_id": msg.SenderID,
			"chat_id":   msg.ChatID,
		})

	// Parse origin channel from chat_id (format: "channel:chat_id")
	var originChannel, originChatID string
	if idx := strings.Index(msg.ChatID, ":"); idx > 0 {
		originChannel = msg.ChatID[:idx]
		originChatID = msg.ChatID[idx+1:]
	} else {
		originChannel = "cli"
		originChatID = msg.ChatID
	}

	// Extract subagent result from message content
	// Format: "Task 'label' completed.\n\nResult:\n<actual content>"
	content := msg.Content
	if idx := strings.Index(content, "Result:\n"); idx >= 0 {
		content = content[idx+8:] // Extract just the result part
	}

	al.dashboardHub.Broadcast(map[string]any{
		"type":           "subagent_result_received",
		"sender_id":      msg.SenderID,
		"content_len":    len(content),
		"origin_channel": originChannel,
	})

	// Skip internal channels - only log, don't send to user
	if constants.IsInternalChannel(originChannel) {
		logger.InfoCF("agent", "Subagent completed (internal channel)",
			map[string]any{
				"sender_id":   msg.SenderID,
				"content_len": len(content),
				"channel":     originChannel,
			})
		return "", nil
	}

	// Use default agent for system messages
	agent := al.getRegistry().GetDefaultAgent()
	if agent == nil {
		return "", fmt.Errorf("no default agent configured")
	}

	// Use the origin session for context
	sessionKey := routing.BuildAgentMainSessionKey(agent.ID)

	return al.runAgentLoop(ctx, agent, processOptions{
		SessionKey:      sessionKey,
		Channel:         originChannel,
		ChatID:          originChatID,
		UserMessage:     fmt.Sprintf("[Subagent result from %s]\n\n%s", msg.SenderID, content),
		DefaultResponse: "Background task completed.",
		EnableSummary:   false,
		SendResponse:    true,
	})
}

func (al *AgentLoop) runAgentLoop(ctx context.Context, agent *AgentInstance, opts processOptions) (string, error) {
	// Killswitch check — abort immediately if Reset() was called.
	if al.killed.Load() {
		return "", context.Canceled
	}

	// Wrap context so Reset() can cancel this call. Use the session key
	// as the tracking key (unique enough for concurrent calls).
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	trackKey := opts.SessionKey
	if trackKey == "" {
		trackKey = fmt.Sprintf("direct-%d", time.Now().UnixNano())
	}
	al.directCancelsMu.Lock()
	al.directCancels[trackKey] = cancel
	al.directCancelsMu.Unlock()
	defer func() {
		al.directCancelsMu.Lock()
		delete(al.directCancels, trackKey)
		al.directCancelsMu.Unlock()
	}()

	// Apply model override (e.g. heartbeat using a cheaper model).
	// Shallow-copy the agent so the shared instance isn't mutated.
	// If the override model has its own provider config in model_list,
	// create a dedicated provider so the correct API endpoint and model ID are used.
	if opts.ModelOverride != "" {
		copy := *agent
		mc, mcErr := al.cfg.GetModelConfig(opts.ModelOverride)
		if mcErr == nil && mc != nil {
			// Model found in model_list — create a provider from its config
			prov, modelID, provErr := providers.CreateProviderFromConfig(mc)
			if provErr == nil {
				copy.Model = opts.ModelOverride
				copy.ModelID = modelID
				copy.Provider = prov
				agent = &copy
				logger.InfoCF(
					fmt.Sprintf("agent:%s", agent.ID),
					fmt.Sprintf(
						"Model override applied: %s (%s) with dedicated provider",
						opts.ModelOverride,
						modelID,
					),
					nil,
				)
			} else {
				logger.WarnCF(
					fmt.Sprintf("agent:%s", agent.ID),
					fmt.Sprintf(
						"Failed to create provider for override model %s: %v",
						opts.ModelOverride,
						provErr,
					),
					nil,
				)
			}
		} else {
			// Not in model_list — resolve as a raw model ID on the existing provider
			overrideID := resolveAgentModelID(opts.ModelOverride, al.cfg)
			if overrideID != "" {
				copy.Model = opts.ModelOverride
				copy.ModelID = overrideID
				agent = &copy
				logger.InfoCF(fmt.Sprintf("agent:%s", agent.ID),
					fmt.Sprintf("Model override applied: %s (%s)", opts.ModelOverride, overrideID), nil)
			}
		}
	}

	agentComp := fmt.Sprintf("agent:%s", agent.ID)

	// Guard: if no provider is configured, return a friendly message
	if agent.Provider == nil {
		noModelMsg := "No model is configured. Please open the Web UI → Models page and add a model to get started."
		logger.WarnCF(agentComp, noModelMsg, nil)
		if opts.SendResponse && opts.Channel != "" && opts.ChatID != "" {
			al.bus.PublishOutbound(bus.OutboundMessage{
				Channel: opts.Channel,
				ChatID:  opts.ChatID,
				Content: noModelMsg,
			})
		}
		return noModelMsg, nil
	}

	// 0. Record last channel for heartbeat notifications (skip internal channels)
	if opts.Channel != "" && opts.ChatID != "" {
		// Don't record internal channels (cli, system, subagent)
		if !constants.IsInternalChannel(opts.Channel) {
			channelKey := fmt.Sprintf("%s:%s", opts.Channel, opts.ChatID)
			if err := al.RecordLastChannel(channelKey); err != nil {
				logger.WarnCF(agentComp, "Failed to record last channel", map[string]any{"error": err.Error()})
			}
		}
	}

	// 1. Update tool contexts
	al.updateToolContexts(agent, opts.Channel, opts.ChatID)

	// 1a-pre. Set elevated flag on exec tool
	if execTool, ok := agent.Tools.Get("exec"); ok {
		if et, ok := execTool.(*tools.ExecTool); ok {
			et.SetElevated(al.elevatedMgr.IsElevated(opts.SessionKey))
		}
	}

	// 1a. Set checkpoint tool session key
	if cpTool, ok := agent.Tools.Get("checkpoint"); ok {
		if ct, ok := cpTool.(*tools.CheckpointTool); ok {
			ct.SetSessionKey(opts.SessionKey)
		}
	}

	// 1b. Thinking indicator removed — users prefer only real updates.

	// 2. Build messages (skip history for heartbeat)
	var history []providers.Message
	var summary string
	if !opts.NoHistory {
		history = agent.Sessions.GetHistory(opts.SessionKey)
		summary = agent.Sessions.GetSummary(opts.SessionKey)
	}
	messages := agent.ContextBuilder.BuildMessages(
		history,
		summary,
		opts.UserMessage,
		nil,
		opts.Channel,
		opts.ChatID,
	)

	// Inject images into the last user message for vision-capable providers
	if len(opts.UserImages) > 0 {
		for i := len(messages) - 1; i >= 0; i-- {
			if messages[i].Role == "user" {
				messages[i].Images = opts.UserImages
				break
			}
		}
	}

	// 3. Save user message to session
	agent.Sessions.AddMessage(opts.SessionKey, "user", opts.UserMessage)

	// 4. Run LLM iteration loop
	isSynthesis := strings.HasPrefix(opts.UserMessage, "[Subagent result from")
	if isSynthesis {
		logger.InfoCF(
			agentComp,
			fmt.Sprintf("SOFIA: synthesis start — presenting sub-agent result via model %s", agent.Model),
			map[string]any{
				"model":       agent.Model,
				"session_key": opts.SessionKey,
				"input_len":   len(opts.UserMessage),
			},
		)
	} else {
		logger.InfoCF(agentComp, fmt.Sprintf("SOFIA: generating response — model %s", agent.Model),
			map[string]any{
				"model":       agent.Model,
				"session_key": opts.SessionKey,
			})
	}
	// 4a. Auto-escalation: adjust iteration limits and model based on message complexity
	if al.cfg.Agents.Defaults.AutoEscalation.Enabled {
		complexity := DetectComplexity(opts.UserMessage)
		needsCopy := false
		agentCopy := *agent

		if complexity.MaxIterations > 0 && complexity.MaxIterations != agent.MaxIterations {
			agentCopy.MaxIterations = complexity.MaxIterations
			needsCopy = true
		}

		// Smart model routing: use fallback model for simple messages
		if al.cfg.Agents.Defaults.AutoEscalation.SmartModelRouting &&
			complexity.ModelTier == ModelTierFallback &&
			len(agent.Fallbacks) > 0 {
			fallbackModel := agent.Fallbacks[0]
			mc, mcErr := al.cfg.GetModelConfig(fallbackModel)
			if mcErr == nil && mc != nil {
				prov, modelID, provErr := providers.CreateProviderFromConfig(mc)
				if provErr == nil {
					agentCopy.Model = fallbackModel
					agentCopy.ModelID = modelID
					agentCopy.Provider = prov
					needsCopy = true
					logger.InfoCF(agentComp, "Smart routing: using fallback model for simple message",
						map[string]any{"model": fallbackModel})
				}
			}
		}

		if needsCopy {
			agent = &agentCopy
			logger.DebugCF(agentComp, "Auto-escalation applied",
				map[string]any{
					"level":          complexity.Level,
					"max_iterations": agentCopy.MaxIterations,
					"model":          agentCopy.Model,
				})
		}
	}

	// Trace: wrap LLM iteration in a span
	var llmSpan *trace.Span
	if al.tracer != nil && opts.ParentSpan != nil {
		llmSpan = al.tracer.StartSpan(opts.ParentSpan, trace.SpanLLMCall, "runLLMIteration")
		llmSpan.Attributes["model"] = agent.Model
		llmSpan.Attributes["model_id"] = agent.ModelID
		llmSpan.Attributes["max_iterations"] = agent.MaxIterations
	}

	llmStart := time.Now()
	finalContent, iteration, errorCount, err := al.runLLMIteration(ctx, agent, messages, opts)
	llmDur := time.Since(llmStart).Milliseconds()

	if al.tracer != nil && llmSpan != nil {
		status := trace.StatusOK
		attrs := map[string]any{
			"iterations":  iteration,
			"errors":      errorCount,
			"duration_ms": llmDur,
		}
		if err != nil {
			status = trace.StatusError
			attrs["error"] = err.Error()
		}
		al.tracer.EndSpan(llmSpan, status, attrs)
	}
	if err != nil {
		return "", err
	}

	// 4b. Evaluation loop — score response and retry if below threshold
	if al.cfg.Agents.Defaults.EvaluationLoop.Enabled && !opts.NoHistory && finalContent != "" {
		evalLoop := NewEvaluationLoop(al.memDB, agent.ID, al.cfg.Agents.Defaults.EvaluationLoop)
		improved, evalErr := evalLoop.EvaluateAndRetry(
			ctx, agent, messages, opts, al, finalContent, iteration, errorCount, llmDur,
		)
		if evalErr != nil {
			logger.WarnCF(agentComp, "Evaluation loop error, keeping original response",
				map[string]any{"error": evalErr.Error()})
		} else if improved != "" {
			finalContent = improved
		}
	}

	// If last tool had ForUser content and we already sent it, we might not need to send final response
	// This is controlled by the tool's Silent flag and ForUser content

	// 5. Handle empty response
	if finalContent == "" {
		finalContent = opts.DefaultResponse
	}

	// 6. Save final assistant message to session
	agent.Sessions.AddMessage(opts.SessionKey, "assistant", finalContent)
	agent.Sessions.Save(opts.SessionKey)

	// 6b. Cleanup auto-checkpoints after successful completion
	if err := al.checkpointMgr.Cleanup(opts.SessionKey); err != nil {
		logger.WarnCF(agentComp, "Failed to cleanup checkpoints", map[string]any{"error": err.Error()})
	}

	// 7. Optional: summarization
	if opts.EnableSummary {
		al.maybeSummarize(agent, opts.SessionKey, opts.Channel, opts.ChatID)
	}

	// 7b. Learn from feedback (#8) — detect correction patterns in user message
	if al.cfg.Agents.Defaults.LearnFromFeedback {
		al.maybeLearnFromFeedback(agent, opts.UserMessage)
	}

	// 7c. Post-task self-reflection
	if al.cfg.Agents.Defaults.PostTaskReflection && !opts.NoHistory {
		go al.maybeReflect(agent, opts.SessionKey, finalContent, iteration, errorCount, llmDur)
	}

	// 7d. Attach multi-dimensional scores to the trace root span
	if al.tracer != nil && opts.ParentSpan != nil {
		scorer := NewPerformanceScorer()
		scores := scorer.MultiScore(iteration, errorCount, finalContent != "")
		scores["model"] = 0 // placeholder so model name is on the span attrs instead
		opts.ParentSpan.Attributes["model"] = agent.Model
		for dim, val := range scores {
			if dim != "model" {
				al.tracer.SetScore(opts.ParentSpan, dim, val)
			}
		}
	}

	// Guardrail: Output Filtering
	if al.cfg.Guardrails.OutputFiltering.Enabled {
		for _, pattern := range al.cfg.Guardrails.OutputFiltering.RedactPatterns {
			re := getCachedRegex(pattern)
			if re == nil {
				continue
			}
			if al.cfg.Guardrails.OutputFiltering.Action == "block" {
				if re.MatchString(finalContent) {
					finalContent = "[CONTENT BLOCKED BY OUTPUT FILTER]"
					logger.WarnCF(
						agentComp,
						"Guardrail blocked output: pattern match",
						map[string]any{"pattern": pattern},
					)
					break
				}
			} else {
				// Default to redact
				if re.MatchString(finalContent) {
					finalContent = re.ReplaceAllString(finalContent, "[REDACTED]")
					logger.WarnCF(
						agentComp,
						"Guardrail redacted output: pattern match",
						map[string]any{"pattern": pattern},
					)
				}
			}
		}
	}

	// 8. Optional: send response via bus
	if opts.SendResponse {
		al.bus.PublishOutbound(bus.OutboundMessage{
			Channel: opts.Channel,
			ChatID:  opts.ChatID,
			Content: finalContent,
		})
	}

	responsePreview := utils.Truncate(finalContent, 120)
	logger.InfoCF(agentComp, fmt.Sprintf("SOFIA: response ready — %s", responsePreview),
		map[string]any{
			"agent_id":         agent.ID,
			"session_key":      opts.SessionKey,
			"iterations":       iteration,
			"duration_ms":      llmDur,
			"response_len":     len(finalContent),
			"response_preview": responsePreview,
		})

	al.dashboardHub.Broadcast(map[string]any{
		"type":         "agent_done",
		"agent_id":     agent.ID,
		"iterations":   iteration,
		"duration_ms":  llmDur,
		"response_len": len(finalContent),
	})

	// Broadcast quick-reply suggestions for the dashboard
	hadToolCalls := iteration > 1
	suggestions := GenerateSuggestions(finalContent, hadToolCalls)
	if len(suggestions) > 0 {
		al.dashboardHub.Broadcast(map[string]any{
			"type":        "suggestions",
			"suggestions": suggestions,
		})
	}

	return finalContent, nil
}
