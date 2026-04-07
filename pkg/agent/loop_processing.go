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
	"github.com/grasberg/sofia/pkg/routing"
	"github.com/grasberg/sofia/pkg/trace"
	"github.com/grasberg/sofia/pkg/utils"
)

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

	if scrubbed, secretTypes := guardrails.ScrubSecrets(msg.Content); len(secretTypes) > 0 {
		logger.WarnCF("agent:main", "Guardrail scrubbed secrets from inbound message", map[string]any{
			"secret_types": secretTypes,
			"sender":       msg.SenderID,
			"channel":      msg.Channel,
		})
		logger.Audit("Inbound Secret Scrubbed", map[string]any{
			"secret_types": secretTypes,
			"sender":       msg.SenderID,
			"channel":      msg.Channel,
		})
		msg.Content = scrubbed
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

	// Skip delegation for local models — small models handle direct responses
	// better than coordinating subagents, and each delegation adds a full
	// LLM round-trip with the same prompt overhead.
	defaultAgent := al.getRegistry().GetDefaultAgent()
	skipDelegation := defaultAgent != nil && defaultAgent.IsLocalModel

	if !skipDelegation {
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
