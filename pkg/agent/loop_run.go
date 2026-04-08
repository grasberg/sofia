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
	"github.com/grasberg/sofia/pkg/tools"
	"github.com/grasberg/sofia/pkg/trace"
	"github.com/grasberg/sofia/pkg/utils"
)

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
	var messages []providers.Message

	// Use compact system prompt for local models to reduce latency
	if agent.IsLocalModel {
		compactPrompt := agent.ContextBuilder.BuildCompactSystemPrompt()
		messages = []providers.Message{{Role: "system", Content: compactPrompt}}
		// Keep only last 6 history entries (3 exchanges) for local models
		hist := agent.Sessions.GetHistory(opts.SessionKey)
		maxHist := 6
		if len(hist) > maxHist {
			hist = hist[len(hist)-maxHist:]
		}
		for _, h := range hist {
			messages = append(messages, providers.Message{Role: h.Role, Content: h.Content})
		}
		messages = append(messages, providers.Message{Role: "user", Content: opts.UserMessage})
	} else {
		var history []providers.Message
		var summary string
		if !opts.NoHistory {
			history = agent.Sessions.GetHistory(opts.SessionKey)
			summary = agent.Sessions.GetSummary(opts.SessionKey)
		}
		messages = agent.ContextBuilder.BuildMessages(
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
	}

	// 3. Save user message to session (skipped for ephemeral calls)
	if !opts.Ephemeral {
		agent.Sessions.AddMessage(opts.SessionKey, "user", opts.UserMessage)
	}

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

	// 5b. Prepend [btw] marker for ephemeral calls so the caller can identify the response
	if opts.Ephemeral && finalContent != "" {
		finalContent = "[btw] " + finalContent
	}

	// 6. Save final assistant message to session (skipped for ephemeral calls)
	if !opts.Ephemeral {
		agent.Sessions.AddMessage(opts.SessionKey, "assistant", finalContent)
		agent.Sessions.Save(opts.SessionKey)

		// 6b. Cleanup auto-checkpoints after successful completion
		if err := al.checkpointMgr.Cleanup(opts.SessionKey); err != nil {
			logger.WarnCF(agentComp, "Failed to cleanup checkpoints", map[string]any{"error": err.Error()})
		}
	}

	// 7. Optional: summarization (skipped for ephemeral calls)
	if opts.EnableSummary && !opts.Ephemeral {
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

	if scrubbed, secretTypes := guardrails.ScrubSecrets(finalContent); len(secretTypes) > 0 {
		logger.WarnCF(agentComp, "Guardrail scrubbed secrets from final response", map[string]any{
			"secret_types": secretTypes,
		})
		finalContent = scrubbed
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
