package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/grasberg/sofia/pkg/audit"
	"github.com/grasberg/sofia/pkg/bus"
	"github.com/grasberg/sofia/pkg/config"
	"github.com/grasberg/sofia/pkg/constants"
	"github.com/grasberg/sofia/pkg/guardrails"
	"github.com/grasberg/sofia/pkg/logger"
	"github.com/grasberg/sofia/pkg/providers"
	"github.com/grasberg/sofia/pkg/tools"
	"github.com/grasberg/sofia/pkg/trace"
	"github.com/grasberg/sofia/pkg/utils"
)

func (al *AgentLoop) runLLMIteration(
	ctx context.Context,
	agent *AgentInstance,
	messages []providers.Message,
	opts processOptions,
) (string, int, int, error) {
	agentComp := fmt.Sprintf("agent:%s", agent.ID)
	iteration := 0
	errorCount := 0
	var finalContent string
	var lastReasoningContent string

	reflectionInterval := al.cfg.Agents.Defaults.ReflectionInterval
	parallelTools := al.cfg.Agents.Defaults.ParallelToolCalls

	// Doom loop detector — tracks repeated actions/errors and suggests recovery.
	var doomDetector *DoomLoopDetector
	if al.cfg.Agents.Defaults.DoomLoopDetection.Enabled {
		doomDetector = NewDoomLoopDetector(al.cfg.Agents.Defaults.DoomLoopDetection.RepetitionThreshold)
	}

	// Cache tool definitions across iterations — user intent doesn't change,
	// only tool results do, so re-filtering is wasted work.
	var cachedToolDefs []providers.ToolDefinition
	var cachedToolDefsVersion int64 // track registry version for cache invalidation

	for iteration < agent.MaxIterations {
		iteration++

		// Killswitch + context cancellation check at each iteration boundary.
		if al.killed.Load() {
			logger.InfoCF(agentComp, "LLM loop aborted: killswitch active", nil)
			return "", iteration, errorCount, context.Canceled
		}
		select {
		case <-ctx.Done():
			logger.InfoCF(agentComp, "LLM loop aborted: context canceled", nil)
			return "", iteration, errorCount, ctx.Err()
		default:
		}

		// Feature: Self-reflection checkpoint (#2)
		// At every N iterations, inject a system reflection prompt
		if reflectionInterval > 0 && iteration > 1 && (iteration-1)%reflectionInterval == 0 {
			reflectionPrompt := "[REFLECTION CHECKPOINT] " +
				"Pause and assess your progress. " +
				"What have you accomplished so far? Are you making meaningful progress? " +
				"Should you change your approach or strategy? " +
				"If stuck, consider alternative methods."

			// Inject plan status if active
			if plan := al.planManager.GetActivePlan(); plan != nil {
				reflectionPrompt += "\n\nCurrent plan status:\n" + plan.FormatStatus()
			}

			messages = append(messages, providers.Message{
				Role:    "user",
				Content: reflectionPrompt,
			})

			logger.InfoCF(agentComp, fmt.Sprintf("REFLECT: injected reflection at iteration %d", iteration),
				map[string]any{"agent_id": agent.ID, "iteration": iteration})
		}

		// Feature: Inject active plan context (#1)
		// On every iteration, if there's an active plan, include its status
		if iteration == 1 {
			if plan := al.planManager.GetActivePlan(); plan != nil {
				planStatus := plan.FormatStatus()
				// Append to the last system or user message rather than adding a new one
				for i := len(messages) - 1; i >= 0; i-- {
					if messages[i].Role == "user" {
						messages[i].Content += "\n\n[Active Plan Status]\n" + planStatus
						break
					}
				}
			}
		}

		logger.DebugCF(agentComp, "LLM iteration",
			map[string]any{
				"agent_id":  agent.ID,
				"iteration": iteration,
				"max":       agent.MaxIterations,
			})

		// Build tool definitions.
		// Strategy: on the first iteration, only include tools if the message
		// looks like an actionable task. Small/local models compulsively call
		// tools even for conversational messages, so withholding them on the
		// first call lets the model respond naturally. Tools are provided on
		// subsequent iterations (after the model has seen the conversation).
		var providerToolDefs []providers.ToolDefinition

		// Build or retrieve cached tool definitions.
		// Always populate the cache even if we won't use tools this iteration,
		// so subsequent iterations don't re-run the expensive matcher.
		currentVersion := agent.Tools.GetVersion()
		if cachedToolDefs == nil || cachedToolDefsVersion != currentVersion {
			var activeTools []tools.Tool
			allToolNames := agent.Tools.List()
			filterTopK := 15

			var allToolsList []tools.Tool
			for _, name := range allToolNames {
				if t, ok := agent.Tools.Get(name); ok {
					allToolsList = append(allToolsList, t)
				}
			}

			if len(allToolNames) > filterTopK {
				var intent string
				for i := len(messages) - 1; i >= 0; i-- {
					if messages[i].Role == "user" {
						intent = messages[i].Content
						break
					}
				}

				if al.semanticMatcher != nil {
					activeTools = al.semanticMatcher.MatchTools(ctx, intent, allToolsList, filterTopK)
				} else {
					activeTools = tools.KeywordMatchTools(intent, allToolsList, filterTopK)
				}
			} else {
				activeTools = allToolsList
			}

			var builtDefs []providers.ToolDefinition
			for _, t := range activeTools {
				schema := tools.ToolToSchema(t)
				if fn, ok := schema["function"].(map[string]any); ok {
					name, _ := fn["name"].(string)
					desc, _ := fn["description"].(string)
					params, _ := fn["parameters"].(map[string]any)
					builtDefs = append(builtDefs, providers.ToolDefinition{
						Type: "function",
						Function: providers.ToolFunctionDefinition{
							Name:        name,
							Description: desc,
							Parameters:  params,
						},
					})
				}
			}
			cachedToolDefs = builtDefs
			cachedToolDefsVersion = currentVersion
		}

		// Decide whether to include tools this iteration.
		// On first iteration of conversational messages, withhold tools to let
		// the model respond naturally (small/local models compulsively call tools).
		isTask := looksLikeTask(opts.UserMessage)
		if !isTask && iteration == 1 {
			providerToolDefs = nil
		} else {
			providerToolDefs = cachedToolDefs
		}

		// Log LLM request details
		logger.DebugCF(agentComp, "LLM request",
			map[string]any{
				"agent_id":          agent.ID,
				"iteration":         iteration,
				"model":             agent.Model,
				"messages_count":    len(messages),
				"tools_count":       len(providerToolDefs),
				"max_tokens":        agent.MaxTokens,
				"temperature":       agent.Temperature,
				"system_prompt_len": len(messages[0].Content),
			})

		// Log full messages (detailed)
		if logger.IsDebug() {
			logger.DebugCF(agentComp, "Full LLM request",
				map[string]any{
					"iteration":     iteration,
					"messages_json": formatMessagesForLog(messages),
					"tools_json":    formatToolsForLog(providerToolDefs),
				})
		}

		// Call LLM with fallback chain if candidates are configured.
		var response *providers.LLMResponse
		var err error

		// Guardrail: Rate Limiting
		if al.cfg.Guardrails.RateLimiting.Enabled {
			al.rlMutex.Lock()
			now := time.Now()

			// Reset RPM counter if minute passed
			if now.After(al.rpmResetTime[agent.ID]) {
				al.rpmCounts[agent.ID] = 0
				al.rpmResetTime[agent.ID] = now.Add(time.Minute)
			}
			// Reset Token counter if hour passed
			if now.After(al.tokenResetTime[agent.ID]) {
				al.tokenCounts[agent.ID] = 0
				al.tokenResetTime[agent.ID] = now.Add(time.Hour)
			}

			// Check limits
			if maxRPM := al.cfg.Guardrails.RateLimiting.MaxRPM; maxRPM > 0 && al.rpmCounts[agent.ID] >= maxRPM {
				al.rlMutex.Unlock()
				logger.WarnCF(
					agentComp,
					"Guardrail: Rate limit exceeded (RPM)",
					map[string]any{"rpm": al.rpmCounts[agent.ID], "max": maxRPM},
				)
				logger.Audit("Rate Limit Exceeded", map[string]any{
					"agent_id": agent.ID,
					"type":     "RPM",
					"rpm":      al.rpmCounts[agent.ID],
					"max":      maxRPM,
				})
				return "Error: Agent rate limit exceeded (requests per minute). Please try again later.", iteration, errorCount, nil
			}

			estimatedTokens := al.estimateTokens(messages) // Approximate
			if maxTokens := al.cfg.Guardrails.RateLimiting.MaxTokensPerHour; maxTokens > 0 &&
				al.tokenCounts[agent.ID]+estimatedTokens > maxTokens {
				al.rlMutex.Unlock()
				logger.WarnCF(
					agentComp,
					"Guardrail: Rate limit exceeded (Tokens)",
					map[string]any{"tokens": al.tokenCounts[agent.ID], "max": maxTokens},
				)
				logger.Audit("Rate Limit Exceeded", map[string]any{
					"agent_id": agent.ID,
					"type":     "TokensPerHour",
					"tokens":   al.tokenCounts[agent.ID],
					"max":      maxTokens,
				})
				return "Error: Agent rate limit exceeded (tokens per hour). Please try again later.", iteration, errorCount, nil
			}

			al.rlMutex.Unlock()
		}

		// --- AUTO-TUNING ---
		// Dynamically adjust temperature based on task type inferred from messages
		callTemp := agent.Temperature
		// Only auto-tune if leaving at default (0.7)
		if callTemp == 0.7 {
			lastMsg := ""
			for i := len(messages) - 1; i >= 0; i-- {
				if messages[i].Role == "user" {
					lastMsg = strings.ToLower(messages[i].Content)
					break
				}
			}

			if strings.Contains(lastMsg, "code") || strings.Contains(lastMsg, "debug") ||
				strings.Contains(lastMsg, "fix") {
				callTemp = 0.2 // Lower temp for analytical/coding tasks
				logger.DebugCF(
					agentComp,
					"Auto-Tuning: lowered temperature for coding task",
					map[string]any{"temp": callTemp},
				)
			} else if strings.Contains(lastMsg, "brainstorm") || strings.Contains(lastMsg, "write") || strings.Contains(lastMsg, "creative") ||
				strings.Contains(lastMsg, "idea") {
				callTemp = 0.8 // Higher temp for creative tasks
				logger.DebugCF(
					agentComp,
					"Auto-Tuning: raised temperature for creative task",
					map[string]any{"temp": callTemp},
				)
			}
		}
		// -------------------

		// Build LLM call options, injecting thinking level if set
		effectiveMaxTokens := anthropicOutputCap(agent.MaxTokens, agent.ModelID)
		llmOpts := map[string]any{
			"max_tokens":       effectiveMaxTokens,
			"temperature":      callTemp,
			"prompt_cache_key": agent.ID,
		}
		if v, ok := al.thinkingLevel.Load(opts.SessionKey); ok {
			level := v.(ThinkingLevel)
			if budget := ThinkingBudgetTokens(level); budget > 0 {
				llmOpts["thinking"] = map[string]any{
					"type":          "enabled",
					"budget_tokens": budget,
				}
			}
		}
		if agent.ThinkingBudget > 0 {
			llmOpts["thinking_budget"] = agent.ThinkingBudget
		}
		// Consolidate: if thinking_budget not set by config, promote the slash-command value.
		// Config takes priority — only fall back to the slash-command budget if not already set.
		if _, ok := llmOpts["thinking_budget"]; !ok {
			if thinkingMap, ok := llmOpts["thinking"].(map[string]any); ok {
				if budget, ok := thinkingMap["budget_tokens"].(int); ok && budget > 0 {
					llmOpts["thinking_budget"] = budget
				}
			}
		}
		delete(llmOpts, "thinking") // clean up the old key; buildParams only reads thinking_budget

		callLLM := func() (*providers.LLMResponse, error) {
			if len(agent.Candidates) > 1 && al.fallback != nil {
				// Adaptive ranking: reorder candidates by quality scores
				candidates := agent.Candidates
				if al.providerRanker != nil {
					candidates = al.providerRanker.Rank(candidates)
				}
				fbResult, fbErr := al.fallback.Execute(ctx, candidates,
					func(ctx context.Context, provider, model string) (*providers.LLMResponse, error) {
						return agent.Provider.Chat(ctx, messages, providerToolDefs, model, llmOpts)
					},
				)
				if fbErr != nil {
					return nil, fbErr
				}
				if fbResult.Provider != "" && len(fbResult.Attempts) > 0 {
					logger.InfoCF(agentComp, fmt.Sprintf("Fallback: succeeded with %s/%s after %d attempts",
						fbResult.Provider, fbResult.Model, len(fbResult.Attempts)+1),
						map[string]any{"agent_id": agent.ID, "iteration": iteration})
				}
				return fbResult.Response, nil
			}
			return agent.Provider.Chat(ctx, messages, providerToolDefs, agent.ModelID, llmOpts)
		}

		// Retry loop for context/token errors
		statusMsg := fmt.Sprintf("Waiting for LLM (iteration %d)...", iteration)
		al.activeStatus.Store(statusMsg)
		al.broadcastPresence(agent.ID, "processing")
		al.dashboardHub.Broadcast(map[string]any{
			"type":      "agent_status",
			"agent_id":  agent.ID,
			"status":    statusMsg,
			"iteration": iteration,
		})
		maxRetries := 2
		for retry := 0; retry <= maxRetries; retry++ {
			response, err = callLLM()
			if err == nil {
				break
			}

			// Abort immediately on killswitch or real context cancellation —
			// do NOT mistake these for "context window" token errors.
			if al.killed.Load() || ctx.Err() != nil {
				return "", iteration, errorCount, context.Canceled
			}

			errMsg := strings.ToLower(err.Error())
			isContextError := strings.Contains(errMsg, "token") ||
				strings.Contains(errMsg, "invalidparameter") ||
				strings.Contains(errMsg, "length")

			if isContextError && retry < maxRetries {
				logger.WarnCF(agentComp, "Context window error detected, attempting compression", map[string]any{
					"error": err.Error(),
					"retry": retry,
				})

				if retry == 0 && !constants.IsInternalChannel(opts.Channel) {
					al.bus.PublishOutbound(bus.OutboundMessage{
						Channel: opts.Channel,
						ChatID:  opts.ChatID,
						Content: "Context window exceeded. Compressing history and retrying...",
					})
				}

				al.forceCompression(agent, opts.SessionKey)
				newHistory := agent.Sessions.GetHistory(opts.SessionKey)
				newSummary := agent.Sessions.GetSummary(opts.SessionKey)
				messages = agent.ContextBuilder.BuildMessages(
					newHistory, newSummary, "",
					nil, opts.Channel, opts.ChatID,
				)
				continue
			}
			break
		}

		if err != nil {
			logger.ErrorCF(agentComp, "LLM call failed",
				map[string]any{
					"agent_id":  agent.ID,
					"iteration": iteration,
					"error":     err.Error(),
				})
			return "", iteration, errorCount, fmt.Errorf("LLM call failed after retries: %w", err)
		}

		if response == nil {
			logger.ErrorCF(agentComp, "LLM returned nil response without error",
				map[string]any{"agent_id": agent.ID, "iteration": iteration})
			return "", iteration, errorCount, fmt.Errorf("LLM returned nil response")
		}

		// Increment rate limiter counters AFTER successful LLM call
		if al.cfg.Guardrails.RateLimiting.Enabled {
			al.rlMutex.Lock()
			al.rpmCounts[agent.ID]++
			al.tokenCounts[agent.ID] += al.estimateTokens(messages)
			al.rlMutex.Unlock()
		}

		// Record token usage
		if response.Usage != nil {
			al.usageTracker.Record(opts.SessionKey, response.Usage)

			// Record budget spend based on token usage
			if al.budgetManager != nil {
				costUSD := estimateCostUSD(response.Usage, agent.ModelID, al.cfg)
				if costUSD > 0 {
					al.budgetManager.RecordSpend(agent.ID, costUSD)
				}
			}
		}

		// Capture reasoning content for verbose mode
		if response.ReasoningContent != "" {
			lastReasoningContent = response.ReasoningContent
		}

		al.dashboardHub.Broadcast(map[string]any{
			"type":        "llm_response",
			"agent_id":    agent.ID,
			"iteration":   iteration,
			"tool_calls":  len(response.ToolCalls),
			"content_len": len(response.Content),
		})

		// Check if no tool calls - we're done (or nudge on first attempt)
		if len(response.ToolCalls) == 0 {
			isTask := looksLikeTask(opts.UserMessage)
			hasSubstantialText := len(response.Content) > 50

			// Nudge: if this is the first LLM response and it looks like a task
			// but the LLM returned only text (no tool calls), retry once with a
			// strong prompt to use tools. Allow nudge on iteration 1 or 2
			// (iteration 2 can happen if reflection checkpoint was injected).
			if iteration <= 2 && isTask && hasSubstantialText {
				logger.InfoCF(agentComp, "NUDGE: LLM returned text-only for a task, retrying with tool reminder",
					map[string]any{
						"agent_id":     agent.ID,
						"iteration":    iteration,
						"response_len": len(response.Content),
						"user_msg_len": len(opts.UserMessage),
					})

				al.dashboardHub.Broadcast(map[string]any{
					"type":      "agent_nudge",
					"agent_id":  agent.ID,
					"iteration": iteration,
					"reason":    "text_only_response",
				})

				// Append the assistant's text + a nudge as a user message
				messages = append(
					messages,
					providers.Message{Role: "assistant", Content: response.Content},
					providers.Message{
						Role: "user",
						Content: "[SYSTEM] You responded with text but made ZERO tool calls. " +
							"Nothing was actually done. You MUST call tools RIGHT NOW to execute. " +
							"Pick the first concrete step and call the appropriate tool (write_file, exec, read_file, edit_file, list_dir, spawn). " +
							"Do NOT repeat the plan. Do NOT narrate. Just call a tool.",
					},
				)
				continue // re-enter the loop for one more try
			}

			if isTask && hasSubstantialText {
				logger.WarnCF(agentComp, "LLM returned text-only for a task (nudge already used or skipped)",
					map[string]any{
						"agent_id":     agent.ID,
						"iteration":    iteration,
						"no_history":   opts.NoHistory,
						"response_len": len(response.Content),
					})
			}

			finalContent = response.Content
			logger.InfoCF(
				agentComp,
				fmt.Sprintf("SOFIA: LLM returned direct answer — %s", utils.Truncate(finalContent, 120)),
				map[string]any{
					"agent_id":         agent.ID,
					"iteration":        iteration,
					"content_len":      len(finalContent),
					"response_preview": utils.Truncate(finalContent, 120),
				},
			)
			break // no tool calls — direct answer
		}

		normalizedToolCalls := make([]providers.ToolCall, 0, len(response.ToolCalls))
		for _, tc := range response.ToolCalls {
			normalizedToolCalls = append(normalizedToolCalls, providers.NormalizeToolCall(tc))
		}

		// Doom loop: record tool calls and LLM output for this iteration
		if doomDetector != nil {
			doomDetector.RecordToolCalls(normalizedToolCalls)
			doomDetector.RecordOutput(response.Content)
		}

		// Log tool calls summary
		toolNames := make([]string, 0, len(normalizedToolCalls))
		for _, tc := range normalizedToolCalls {
			toolNames = append(toolNames, tc.Name)
		}
		logger.InfoCF(
			agentComp,
			fmt.Sprintf("TOOL: LLM requested %d tool(s): %s", len(normalizedToolCalls), strings.Join(toolNames, ", ")),
			map[string]any{
				"agent_id":  agent.ID,
				"tools":     toolNames,
				"count":     len(normalizedToolCalls),
				"iteration": iteration,
			},
		)

		// Build assistant message with tool calls
		assistantMsg := providers.Message{
			Role:               "assistant",
			Content:            response.Content,
			ReasoningContent:   response.ReasoningContent,
			ReasoningSignature: response.ReasoningSignature,
		}
		for _, tc := range normalizedToolCalls {
			argumentsJSON, _ := json.Marshal(tc.Arguments)
			// Copy ExtraContent to ensure thought_signature is persisted for Gemini 3
			extraContent := tc.ExtraContent
			thoughtSignature := ""
			if tc.Function != nil {
				thoughtSignature = tc.Function.ThoughtSignature
			}

			assistantMsg.ToolCalls = append(assistantMsg.ToolCalls, providers.ToolCall{
				ID:   tc.ID,
				Type: "function",
				Name: tc.Name,
				Function: &providers.FunctionCall{
					Name:             tc.Name,
					Arguments:        string(argumentsJSON),
					ThoughtSignature: thoughtSignature,
				},
				ExtraContent:     extraContent,
				ThoughtSignature: thoughtSignature,
			})
		}
		messages = append(messages, assistantMsg)

		// Save assistant message with tool calls to session (skipped for ephemeral calls)
		if !opts.Ephemeral {
			agent.Sessions.AddFullMessage(opts.SessionKey, assistantMsg)
		}

		// Auto-checkpoint before tool execution
		cpName := fmt.Sprintf("auto:iter-%d", iteration)
		if _, cpErr := al.checkpointMgr.Create(opts.SessionKey, agent.ID, cpName, iteration); cpErr != nil {
			logger.WarnCF(agentComp, "Failed to create auto-checkpoint", map[string]any{
				"error": cpErr.Error(), "iteration": iteration,
			})
		} else {
			logger.DebugCF(agentComp, fmt.Sprintf("CHECKPOINT: saved %s", cpName),
				map[string]any{"iteration": iteration})
		}

		// Execute tool calls — parallel or sequential based on config
		type toolCallResult struct {
			index     int
			tc        providers.ToolCall
			result    *tools.ToolResult
			durMs     int64
			resultMsg providers.Message
		}

		executeSingleTool := func(idx int, tc providers.ToolCall) toolCallResult {
			// Marshal arguments once and reuse throughout this function.
			argumentsJSON, _ := json.Marshal(tc.Arguments)
			argsPreview := utils.Truncate(string(argumentsJSON), 300)

			// Trace: create a tool_call span if tracing is active
			var toolSpan *trace.Span
			if al.tracer != nil && opts.ParentSpan != nil {
				toolSpan = al.tracer.StartSpan(opts.ParentSpan, trace.SpanToolCall, tc.Name)
				toolSpan.Attributes["args_preview"] = argsPreview
				toolSpan.Attributes["iteration"] = iteration
			}

			// Budget check: verify budget before executing tool
			if al.budgetManager != nil {
				_, allowed := al.budgetManager.CheckBudget(agent.ID)
				if !allowed {
					logger.WarnCF(agentComp, "Tool execution blocked: budget exceeded",
						map[string]any{
							"agent_id": agent.ID,
							"tool":     tc.Name,
						})
					return toolCallResult{
						index: idx,
						tc:    tc,
						result: &tools.ToolResult{
							ForLLM:  "Error: Budget exceeded. Tool execution blocked.",
							ForUser: "⚠️ Budget limit reached. Cannot execute tool.",
							IsError: true,
						},
						durMs: 0,
						resultMsg: providers.Message{
							Role:       "tool",
							Content:    "Error: Budget exceeded. Tool execution blocked.",
							ToolCallID: tc.ID,
							ToolName:   tc.Name,
						},
					}
				}
			}

			// Approval gate: check if tool requires human approval
			if al.approvalGate != nil {
				if al.approvalGate.RequiresApproval(opts.SessionKey, tc.Name, string(argumentsJSON)) {
					req := ApprovalRequest{
						ID:         fmt.Sprintf("approval-%d-%d", iteration, idx),
						ToolName:   tc.Name,
						Arguments:  string(argumentsJSON),
						AgentID:    agent.ID,
						SessionKey: opts.SessionKey,
						Channel:    opts.Channel,
						ChatID:     opts.ChatID,
					}
					approved, err := al.approvalGate.RequestApproval(ctx, req)
					if err != nil {
						return toolCallResult{
							index: idx,
							tc:    tc,
							result: &tools.ToolResult{
								ForLLM:  fmt.Sprintf("Error: Approval check failed: %v", err),
								ForUser: "⚠️ Tool approval encountered an error.",
								IsError: true,
							},
							durMs: 0,
							resultMsg: providers.Message{
								Role:       "tool",
								Content:    fmt.Sprintf("Error: Approval check failed: %v", err),
								ToolCallID: tc.ID,
								ToolName:   tc.Name,
							},
						}
					}
					if !approved {
						logger.InfoCF(agentComp, "Tool execution denied by approval gate",
							map[string]any{
								"agent_id": agent.ID,
								"tool":     tc.Name,
							})
						return toolCallResult{
							index: idx,
							tc:    tc,
							result: &tools.ToolResult{
								ForLLM:  "Error: Tool call denied by human approval gate.",
								ForUser: "⚠️ Tool execution was denied by human reviewer.",
								IsError: true,
							},
							durMs: 0,
							resultMsg: providers.Message{
								Role:       "tool",
								Content:    "Error: Tool call denied by human approval gate.",
								ToolCallID: tc.ID,
								ToolName:   tc.Name,
							},
						}
					}
				}
			}

			al.activeStatus.Store(fmt.Sprintf("Executing tool: %s", tc.Name))
			logger.InfoCF(agentComp, fmt.Sprintf("TOOL: started %s", tc.Name),
				map[string]any{
					"agent_id":     agent.ID,
					"tool":         tc.Name,
					"iteration":    iteration,
					"args_preview": argsPreview,
				})

			al.dashboardHub.Broadcast(map[string]any{
				"type":      "tool_call_start",
				"agent_id":  agent.ID,
				"tool":      tc.Name,
				"args":      argsPreview,
				"iteration": iteration,
			})

			// Broadcast tool_progress "started" event
			progressReporter := tools.NewProgressReporter(tc.Name, func(u tools.ProgressUpdate) {
				al.dashboardHub.Broadcast(map[string]any{
					"type":       "tool_progress",
					"agent_id":   agent.ID,
					"tool":       u.ToolName,
					"status":     u.Status,
					"message":    u.Message,
					"progress":   u.Progress,
					"elapsed_ms": u.Elapsed,
					"iteration":  iteration,
				})
			})
			progressReporter.Start(fmt.Sprintf("Executing %s", tc.Name))

			// Emit opencode indicator
			if tc.Name == "exec" {
				if cmd, ok := tc.Arguments["command"].(string); ok &&
					strings.Contains(strings.ToLower(cmd), "opencode") {
					logger.InfoCF(agentComp, "OPENCODE: started",
						map[string]any{"agent_id": agent.ID, "iteration": iteration})
				}
			}

			asyncCallback := func(callbackCtx context.Context, result *tools.ToolResult) {
				if !result.Silent && result.ForUser != "" {
					logger.InfoCF(agentComp, fmt.Sprintf("TOOL: async completed %s", tc.Name),
						map[string]any{
							"tool":           tc.Name,
							"for_user_len":   len(result.ForUser),
							"result_preview": utils.Truncate(result.ForUser, 160),
						})
				}
			}

			toolStart := time.Now()
			toolResult := agent.Tools.ExecuteWithContext(
				ctx, tc.Name, tc.Arguments, opts.Channel, opts.ChatID, opts.SessionKey, asyncCallback,
			)
			toolDur := time.Since(toolStart).Milliseconds()

			// Handle large responses: save to temp file if >200K chars
			toolResult = tools.HandleLargeResponse(toolResult, tc.Name)

			toolStatus := "ok"
			toolErrStr := ""
			if toolResult.Err != nil {
				toolStatus = "error"
				toolErrStr = toolResult.Err.Error()
			}

			// Broadcast tool_progress "completed" or "failed" event
			if toolResult.Err != nil || toolResult.IsError {
				msg := "Failed"
				if toolErrStr != "" {
					msg = utils.Truncate(toolErrStr, 200)
				}
				progressReporter.Fail(msg)
			} else {
				progressReporter.Complete(fmt.Sprintf("Completed in %dms", toolDur))
			}

			logger.InfoCF(agentComp, fmt.Sprintf("TOOL: finished %s in %dms — %s", tc.Name, toolDur, toolStatus),
				map[string]any{
					"agent_id":       agent.ID,
					"tool":           tc.Name,
					"duration_ms":    toolDur,
					"status":         toolStatus,
					"for_user_len":   len(toolResult.ForUser),
					"for_llm_len":    len(toolResult.ForLLM),
					"result_preview": utils.Truncate(toolResult.ForLLM, 160),
					"error":          toolErrStr,
				})

			al.dashboardHub.Broadcast(map[string]any{
				"type":        "tool_call_end",
				"agent_id":    agent.ID,
				"tool":        tc.Name,
				"duration_ms": toolDur,
				"success":     toolResult.Err == nil && !toolResult.IsError,
				"result":      utils.Truncate(toolResult.ForLLM, 300),
				"error":       toolErrStr,
				"iteration":   iteration,
			})

			if tc.Name == "exec" {
				if cmd, ok := tc.Arguments["command"].(string); ok &&
					strings.Contains(strings.ToLower(cmd), "opencode") {
					logger.InfoCF(agentComp,
						fmt.Sprintf("OPENCODE: finished in %dms — %s", toolDur, toolStatus),
						map[string]any{"agent_id": agent.ID, "duration_ms": toolDur, "status": toolStatus})
				}
			}

			// Audit log: record every tool call with timing and result
			if al.auditLogger != nil {
				_ = al.auditLogger.Log(audit.AuditEntry{
					AgentID:    agent.ID,
					SessionKey: opts.SessionKey,
					Channel:    opts.Channel,
					Action:     "tool_call",
					Detail:     tc.Name,
					Input:      utils.Truncate(string(argumentsJSON), 500),
					Output:     utils.Truncate(toolResult.ForLLM, 500),
					Duration:   toolDur,
					Success:    toolResult.Err == nil && !toolResult.IsError,
				})
			}

			// Trace: end tool span
			if al.tracer != nil && toolSpan != nil {
				spanStatus := trace.StatusOK
				if toolResult.Err != nil || toolResult.IsError {
					spanStatus = trace.StatusError
				}
				al.tracer.EndSpan(toolSpan, spanStatus, map[string]any{
					"duration_ms":    toolDur,
					"result_preview": utils.Truncate(toolResult.ForLLM, 200),
					"error":          toolErrStr,
				})
			}

			contentForLLM := toolResult.ForLLM
			if contentForLLM == "" && toolResult.Err != nil {
				contentForLLM = toolResult.Err.Error()
			}

			return toolCallResult{
				index:  idx,
				tc:     tc,
				result: toolResult,
				durMs:  toolDur,
				resultMsg: providers.Message{
					Role:       "tool",
					Content:    contentForLLM,
					ToolCallID: tc.ID,
					ToolName:   tc.Name,
					Images:     toolResult.Images,
				},
			}
		}

		var tcResults []toolCallResult

		// Killswitch check before starting tool execution.
		if al.killed.Load() || ctx.Err() != nil {
			return "", iteration, errorCount, context.Canceled
		}

		if parallelTools && len(normalizedToolCalls) > 1 && safeToParallelize(normalizedToolCalls) {
			// Parallel tool execution using errgroup (path-overlap safe)
			results := make([]toolCallResult, len(normalizedToolCalls))
			g, _ := errgroup.WithContext(ctx)

			for i, tc := range normalizedToolCalls {
				g.Go(func() error {
					results[i] = executeSingleTool(i, tc)
					return nil
				})
			}
			_ = g.Wait() // errors are always nil; tool errors are captured in results
			tcResults = results
		} else {
			// Sequential tool execution (default)
			for i, tc := range normalizedToolCalls {
				// Kill check between sequential tools.
				if al.killed.Load() || ctx.Err() != nil {
					return "", iteration, errorCount, context.Canceled
				}
				tcResults = append(tcResults, executeSingleTool(i, tc))
			}
		}

		// Process results in order and count errors
		confirmationNeeded := false
		for _, tcr := range tcResults {
			if tcr.result != nil && tcr.result.Err != nil {
				errorCount++
				if doomDetector != nil {
					doomDetector.RecordError(tcr.result.Err.Error())
				}
			}
			// Handle confirmation-required results (#5)
			if tcr.result.ConfirmationRequired {
				confirmationNeeded = true
				logger.InfoCF(agentComp, "CONFIRM: tool requires confirmation",
					map[string]any{
						"tool":   tcr.tc.Name,
						"prompt": tcr.result.ConfirmationPrompt,
					})

				// Send confirmation request to user
				if opts.SendResponse && opts.Channel != "" {
					al.bus.PublishOutbound(bus.OutboundMessage{
						Channel: opts.Channel,
						ChatID:  opts.ChatID,
						Content: fmt.Sprintf(
							"⚠️ Confirmation required: %s\n\nReply 'yes' to proceed or 'no' to cancel.",
							tcr.result.ConfirmationPrompt,
						),
					})
				}

				// Add a tool result indicating confirmation is pending
				tcr.resultMsg.Content = fmt.Sprintf("[CONFIRMATION_PENDING: %s — awaiting user response]",
					tcr.result.ConfirmationPrompt)
			}

			// Send ForUser content to user immediately if not Silent
			if !tcr.result.Silent && tcr.result.ForUser != "" && opts.SendResponse {
				outContent := tcr.result.ForUser
				// Guardrail: Output Filtering on Tool Results
				if outContent != "" {
					outContent = al.applyOutputFilter(agentComp, tcr.tc.Name, outContent)
				}
				if outContent != "" {
					if scrubbed, secretTypes := guardrails.ScrubSecrets(outContent); len(secretTypes) > 0 {
						logger.WarnCF(agentComp, "Guardrail scrubbed secrets from tool ForUser output", map[string]any{
							"tool": tcr.tc.Name, "secret_types": secretTypes,
						})
						outContent = scrubbed
					}
				}

				if outContent != "" {
					al.bus.PublishOutbound(bus.OutboundMessage{
						Channel: opts.Channel,
						ChatID:  opts.ChatID,
						Content: outContent,
					})
				}
			}

			messages = append(messages, tcr.resultMsg)
			agent.Sessions.AddFullMessage(opts.SessionKey, tcr.resultMsg)
		}

		// Budget pressure signaling: inject into last message already in the messages slice.
		// The tool results were appended to `messages` above, so we modify the last
		// tool message in-place to preserve prompt cache integrity.
		if agent.MaxIterations > 0 && len(tcResults) > 0 {
			pct := float64(iteration) / float64(agent.MaxIterations)
			var budgetNote string
			if pct >= 0.9 {
				budgetNote = "\n\n[BUDGET WARNING: 90% of iterations used. " +
					"Wrap up immediately and provide your final response.]"
			} else if pct >= 0.7 {
				budgetNote = "\n\n[BUDGET NOTE: 70% of iterations used. " +
					"Start planning to wrap up soon.]"
			}
			if budgetNote != "" {
				// Modify the last tool message already in the messages slice
				for i := len(messages) - 1; i >= 0; i-- {
					if messages[i].Role == "tool" {
						messages[i].Content += budgetNote
						break
					}
				}
			}
		}

		// Doom loop detection: check if the agent is stuck and apply graduated recovery
		if doomDetector != nil && doomDetector.Check() {
			action := doomDetector.GetRecoveryAction()
			switch action.Type {
			case DoomRecoveryRedirect:
				messages = append(messages, providers.Message{Role: "user", Content: action.Prompt})
			case DoomRecoveryModelSwitch:
				if len(agent.Candidates) > 0 {
					messages = append(messages, providers.Message{Role: "user", Content: action.Prompt})
				} else {
					// No fallback — escalate to ask-help
					helpAction := doomDetector.GetRecoveryAction()
					finalContent = helpAction.Prompt
				}
			case DoomRecoveryAskHelp, DoomRecoveryAbort:
				finalContent = action.Prompt + "\n\n" + doomDetector.FormatAttemptSummary()
			}
			if finalContent != "" {
				break // exit the for loop
			}
		}

		// Auto-rollback: if error count reaches threshold, rollback to last checkpoint
		autoRollbackThreshold := al.cfg.Agents.Defaults.AutoRollbackThreshold
		if autoRollbackThreshold <= 0 {
			autoRollbackThreshold = 3 // Default value
		}
		if errorCount >= autoRollbackThreshold {
			cp, restoredMsgs, rbErr := al.checkpointMgr.RollbackToLatest(opts.SessionKey)
			if rbErr != nil {
				logger.WarnCF(agentComp, "Auto-rollback failed", map[string]any{"error": rbErr.Error()})
			} else if cp != nil {
				logger.InfoCF(
					agentComp,
					fmt.Sprintf(
						"CHECKPOINT: auto-rollback to %q (iter %d) after %d errors",
						cp.Name,
						cp.Iteration,
						errorCount,
					),
					map[string]any{"checkpoint_id": cp.ID, "errors": errorCount},
				)

				// Rebuild in-memory messages from restored state
				messages = agent.ContextBuilder.BuildMessages(
					restoredMsgs,
					agent.Sessions.GetSummary(opts.SessionKey),
					"",
					nil, opts.Channel, opts.ChatID,
				)

				// Inject rollback notice so the LLM knows what happened
				messages = append(messages, providers.Message{
					Role: "user",
					Content: fmt.Sprintf("[SYSTEM: Auto-rollback triggered after %d consecutive tool errors. "+
						"State restored to checkpoint %q (iteration %d). "+
						"Please try a different approach.]", errorCount, cp.Name, cp.Iteration),
				})

				errorCount = 0 // Reset for the new attempt
				continue       // Restart the iteration loop from the restored state
			}
		}

		// If any tool requires confirmation, wait with a timeout before giving up
		if confirmationNeeded {
			finalContent = "Waiting for user confirmation before proceeding."
			logger.InfoCF(agentComp, "Waiting for user confirmation (5m timeout)", nil)
			timer := time.NewTimer(5 * time.Minute)
			select {
			case <-timer.C:
				finalContent = "Confirmation timed out after 5 minutes. Please retry if you still want to proceed."
				logger.WarnCF(agentComp, "Tool confirmation timed out after 5 minutes", nil)
			case <-ctx.Done():
				timer.Stop()
				// Context canceled (new message or shutdown)
			}
			break
		}
	}

	// If the loop ended without a text response, make one last LLM call
	// without tools so the model is forced to respond with plain text.
	// This handles both max-iterations exhaustion and small models that
	// fail at tool-use and return empty content.
	if finalContent == "" {
		logger.InfoCF(agentComp, "No text response after LLM loop — forcing wrap-up call without tools",
			map[string]any{"agent_id": agent.ID, "iterations": iteration})

		messages = append(messages, providers.Message{
			Role:    "user",
			Content: "[SYSTEM] Respond to the user directly with plain text. Do NOT call any tools.",
		})

		wrapMaxTokens := anthropicOutputCap(agent.MaxTokens, agent.ModelID)
		wrapResp, wrapErr := agent.Provider.Chat(ctx, messages, nil, agent.ModelID, map[string]any{
			"max_tokens":  wrapMaxTokens,
			"temperature": 0.7,
		})
		if wrapErr == nil && wrapResp != nil && wrapResp.Content != "" {
			finalContent = wrapResp.Content
			logger.InfoCF(agentComp, "Wrap-up response received",
				map[string]any{"agent_id": agent.ID, "content_len": len(finalContent)})
		}
	}

	// Verbose mode: prepend reasoning content (scrub secrets first)
	if finalContent != "" && lastReasoningContent != "" {
		if v, ok := al.verboseMode.Load(opts.SessionKey); ok && v.(bool) {
			scrubbed, _ := guardrails.ScrubSecrets(lastReasoningContent)
			finalContent = fmt.Sprintf("[Reasoning]\n%s\n\n[Response]\n%s", scrubbed, finalContent)
		}
	}

	return finalContent, iteration, errorCount, nil
}

// anthropicOutputCap returns the effective max_tokens to use for a model,
// clamping agentMaxTokens to the model's known output limit if lower.
func anthropicOutputCap(agentMaxTokens int, modelID string) int {
	mt := agentMaxTokens
	if strings.HasPrefix(strings.ToLower(modelID), "claude-") {
		if capVal := config.AnthropicOutputCap(modelID); capVal > 0 && mt > capVal {
			mt = capVal
		}
	}
	return mt
}
