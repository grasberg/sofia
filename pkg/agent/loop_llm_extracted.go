package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/grasberg/sofia/pkg/audit"
	"github.com/grasberg/sofia/pkg/bus"
	"github.com/grasberg/sofia/pkg/constants"
	"github.com/grasberg/sofia/pkg/guardrails"
	"github.com/grasberg/sofia/pkg/logger"
	"github.com/grasberg/sofia/pkg/providers"
	"github.com/grasberg/sofia/pkg/tools"
	"github.com/grasberg/sofia/pkg/trace"
	"github.com/grasberg/sofia/pkg/utils"
)

// toolCallResult holds the outcome of a single tool execution.
type toolCallResult struct {
	index     int
	tc        providers.ToolCall
	result    *tools.ToolResult
	durMs     int64
	resultMsg providers.Message
}

// injectReflectionCheckpoint adds a reflection prompt at regular intervals.
func (al *AgentLoop) injectReflectionCheckpoint(
	agentComp string,
	agent *AgentInstance,
	messages []providers.Message,
	iteration int,
	reflectionInterval int,
) []providers.Message {
	if reflectionInterval <= 0 || iteration <= 1 || (iteration-1)%reflectionInterval != 0 {
		return messages
	}

	reflectionPrompt := "[REFLECTION CHECKPOINT] " +
		"Pause and assess your progress. " +
		"What have you accomplished so far? Are you making meaningful progress? " +
		"Should you change your approach or strategy? " +
		"If stuck, consider alternative methods."

	if plan := al.planManager.GetActivePlan(); plan != nil {
		reflectionPrompt += "\n\nCurrent plan status:\n" + plan.FormatStatus()
	}

	messages = append(messages, providers.Message{
		Role:    "user",
		Content: reflectionPrompt,
	})

	logger.InfoCF(agentComp, fmt.Sprintf("REFLECT: injected reflection at iteration %d", iteration),
		map[string]any{"agent_id": agent.ID, "iteration": iteration})

	return messages
}

// injectPlanContext adds active plan status to the first user message.
func (al *AgentLoop) injectPlanContext(messages []providers.Message) {
	plan := al.planManager.GetActivePlan()
	if plan == nil {
		return
	}

	planStatus := plan.FormatStatus()
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "user" {
			messages[i].Content += "\n\n[Active Plan Status]\n" + planStatus
			break
		}
	}
}

// autoTuneTemperature adjusts temperature based on inferred task type.
func (al *AgentLoop) autoTuneTemperature(agentComp string, messages []providers.Message, baseTemp float64) float64 {
	if baseTemp != 0.7 {
		return baseTemp // Only auto-tune from default
	}

	lastMsg := ""
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "user" {
			lastMsg = strings.ToLower(messages[i].Content)
			break
		}
	}

	if strings.Contains(lastMsg, "code") || strings.Contains(lastMsg, "debug") ||
		strings.Contains(lastMsg, "fix") {
		logger.DebugCF(agentComp, "Auto-Tuning: lowered temperature for coding task",
			map[string]any{"temp": 0.2})
		return 0.2
	}

	if strings.Contains(lastMsg, "brainstorm") || strings.Contains(lastMsg, "write") ||
		strings.Contains(lastMsg, "creative") || strings.Contains(lastMsg, "idea") {
		logger.DebugCF(agentComp, "Auto-Tuning: raised temperature for creative task",
			map[string]any{"temp": 0.8})
		return 0.8
	}

	return baseTemp
}

// buildLLMCallFunc creates a function that calls the LLM with fallback chain.
func (al *AgentLoop) buildLLMCallFunc(
	ctx context.Context,
	agent *AgentInstance,
	messages []providers.Message,
	providerToolDefs []providers.ToolDefinition,
	llmOpts map[string]any,
	agentComp string,
	iteration int,
) func() (*providers.LLMResponse, error) {
	return func() (*providers.LLMResponse, error) {
		if len(agent.Candidates) > 1 && al.fallback != nil {
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
}

// handleLLMRetryLoop manages retries for context/token errors.
func (al *AgentLoop) handleLLMRetryLoop(
	ctx context.Context,
	callLLM func() (*providers.LLMResponse, error),
	agentComp string,
	agent *AgentInstance,
	opts processOptions,
	iteration int,
	messages []providers.Message,
) (*providers.LLMResponse, []providers.Message, error) {
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
		response, err := callLLM()
		if err == nil {
			return response, messages, nil
		}

		if al.killed.Load() || ctx.Err() != nil {
			return nil, messages, context.Canceled
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

		return nil, messages, fmt.Errorf("LLM call failed after retries: %w", err)
	}

	return nil, messages, fmt.Errorf("LLM call failed after retries")
}

// handleNoToolCallsResponse processes responses without tool calls.
// Returns (shouldContinue, shouldNudge, finalContent).
func (al *AgentLoop) handleNoToolCallsResponse(
	agentComp string,
	agent *AgentInstance,
	opts processOptions,
	messages []providers.Message,
	response *providers.LLMResponse,
	iteration int,
) (bool, bool, string) {
	isTask := looksLikeTask(opts.UserMessage)
	hasSubstantialText := len(response.Content) > 50

	// Nudge logic
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
		return true, true, "" // continue loop, nudge triggered
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

	logger.InfoCF(
		agentComp,
		fmt.Sprintf("SOFIA: LLM returned direct answer — %s", utils.Truncate(response.Content, 120)),
		map[string]any{
			"agent_id":         agent.ID,
			"iteration":        iteration,
			"content_len":      len(response.Content),
			"response_preview": utils.Truncate(response.Content, 120),
		},
	)

	return false, false, response.Content // exit loop, final content
}

// injectBudgetPressureNote adds iteration budget warnings to messages.
func (al *AgentLoop) injectBudgetPressureNote(messages []providers.Message, iteration, maxIterations int) {
	if maxIterations <= 0 || len(messages) == 0 {
		return
	}

	pct := float64(iteration) / float64(maxIterations)
	var budgetNote string
	if pct >= 0.9 {
		budgetNote = "\n\n[BUDGET WARNING: 90% of iterations used. " +
			"Wrap up immediately and provide your final response.]"
	} else if pct >= 0.7 {
		budgetNote = "\n\n[BUDGET NOTE: 70% of iterations used. " +
			"Start planning to wrap up soon.]"
	}

	if budgetNote == "" {
		return
	}

	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "tool" {
			messages[i].Content += budgetNote
			break
		}
	}
}

// handleDoomLoopRecovery processes doom loop detection and applies graduated recovery.
// Returns true if loop should be exited with finalContent set.
func (al *AgentLoop) handleDoomLoopRecovery(
	doomDetector *DoomLoopDetector,
	messages *[]providers.Message,
	finalContent *string,
) bool {
	if doomDetector == nil || !doomDetector.Check() {
		return false
	}

	action := doomDetector.GetRecoveryAction()
	switch action.Type {
	case DoomRecoveryRedirect:
		*messages = append(*messages, providers.Message{Role: "user", Content: action.Prompt})
		return false
	case DoomRecoveryModelSwitch:
		*messages = append(*messages, providers.Message{Role: "user", Content: action.Prompt})
		return false
	case DoomRecoveryAskHelp, DoomRecoveryAbort:
		*finalContent = action.Prompt + "\n\n" + doomDetector.FormatAttemptSummary()
		return true
	}
	return false
}

// handleAutoRollback attempts to rollback to last checkpoint on repeated errors.
// Returns (shouldContinue, errorCount, messages).
func (al *AgentLoop) handleAutoRollback(
	agentComp string,
	agent *AgentInstance,
	opts processOptions,
	errorCount int,
	messages []providers.Message,
) (bool, int, []providers.Message) {
	autoRollbackThreshold := al.cfg.Agents.Defaults.AutoRollbackThreshold
	if autoRollbackThreshold <= 0 {
		autoRollbackThreshold = 3 // Default value
	}
	if errorCount < autoRollbackThreshold {
		return true, errorCount, messages
	}

	cp, restoredMsgs, rbErr := al.checkpointMgr.RollbackToLatest(opts.SessionKey)
	if rbErr != nil {
		logger.WarnCF(agentComp, "Auto-rollback failed", map[string]any{"error": rbErr.Error()})
		return true, errorCount, messages
	}
	if cp == nil {
		return true, errorCount, messages
	}

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

	messages = agent.ContextBuilder.BuildMessages(
		restoredMsgs,
		agent.Sessions.GetSummary(opts.SessionKey),
		"",
		nil, opts.Channel, opts.ChatID,
	)

	messages = append(messages, providers.Message{
		Role: "user",
		Content: fmt.Sprintf("[SYSTEM: Auto-rollback triggered after %d consecutive tool errors. "+
			"State restored to checkpoint %q (iteration %d). "+
			"Please try a different approach.]", errorCount, cp.Name, cp.Iteration),
	})

	return true, 0, messages // Reset error count, continue
}

// executeToolCall runs a single tool with full tracing, budget checks, and approval gates.
func (al *AgentLoop) executeToolCall(
	ctx context.Context,
	idx int,
	tc providers.ToolCall,
	agent *AgentInstance,
	opts processOptions,
	agentComp string,
	iteration int,
) toolCallResult {
	// Trace: create a tool_call span if tracing is active
	var toolSpan *trace.Span
	if al.tracer != nil && opts.ParentSpan != nil {
		toolSpan = al.tracer.StartSpan(opts.ParentSpan, trace.SpanToolCall, tc.Name)
		argsJSON, _ := json.Marshal(tc.Arguments)
		toolSpan.Attributes["args_preview"] = utils.Truncate(string(argsJSON), 300)
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
		argsJSON, _ := json.Marshal(tc.Arguments)
		if al.approvalGate.RequiresApproval(opts.SessionKey, tc.Name, string(argsJSON)) {
			return al.handleApprovalGate(ctx, tc, idx, iteration, agent, opts, agentComp)
		}
	}

	return al.executeToolWithTracking(ctx, tc, agent, opts, agentComp, iteration, toolSpan)
}

// handleApprovalGate manages the human approval workflow for a tool call.
func (al *AgentLoop) handleApprovalGate(
	ctx context.Context,
	tc providers.ToolCall,
	idx, iteration int,
	agent *AgentInstance,
	opts processOptions,
	agentComp string,
) toolCallResult {
	argsJSON, _ := json.Marshal(tc.Arguments)
	req := ApprovalRequest{
		ID:         fmt.Sprintf("approval-%d-%d", iteration, idx),
		ToolName:   tc.Name,
		Arguments:  string(argsJSON),
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
	return toolCallResult{} // approved, continue
}

// executeToolWithTracking runs a tool with progress tracking, logging, and tracing.
func (al *AgentLoop) executeToolWithTracking(
	ctx context.Context,
	tc providers.ToolCall,
	agent *AgentInstance,
	opts processOptions,
	agentComp string,
	iteration int,
	toolSpan *trace.Span,
) toolCallResult {
	al.activeStatus.Store(fmt.Sprintf("Executing tool: %s", tc.Name))
	argsJSON, _ := json.Marshal(tc.Arguments)
	argsPreview := utils.Truncate(string(argsJSON), 200)
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

	// Check tool result cache for deduplication
	if cachedResult, hit := al.checkToolResultCache(tc.Name, tc.Arguments); hit {
		logger.InfoCF(agentComp, "Tool result cache hit", map[string]any{
			"agent_id": agent.ID,
			"tool":     tc.Name,
		})
		return toolCallResult{
			index:     -1,
			tc:        tc,
			result:    cachedResult,
			durMs:     0,
			resultMsg: buildToolResultMessage(tc, cachedResult),
		}
	}

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

	toolStatus := "ok"
	toolErrStr := ""
	if toolResult.Err != nil {
		toolStatus = "error"
		toolErrStr = toolResult.Err.Error()
	}

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

	if al.auditLogger != nil {
		_ = al.auditLogger.Log(audit.AuditEntry{
			AgentID:    agent.ID,
			SessionKey: opts.SessionKey,
			Channel:    opts.Channel,
			Action:     "tool_call",
			Detail:     tc.Name,
			Input:      utils.Truncate(string(argsJSON), 500),
			Output:     utils.Truncate(toolResult.ForLLM, 500),
			Duration:   toolDur,
			Success:    toolResult.Err == nil && !toolResult.IsError,
		})
	}

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

	// Store result in cache for deduplication
	al.storeToolResultCache(tc.Name, tc.Arguments, toolResult)

	return toolCallResult{
		index:     -1, // will be set by caller
		tc:        tc,
		result:    toolResult,
		durMs:     toolDur,
		resultMsg: buildToolResultMessage(tc, toolResult),
	}
}

// buildToolResultMessage creates a tool result message from a tool call and result.
func buildToolResultMessage(tc providers.ToolCall, result *tools.ToolResult) providers.Message {
	contentForLLM := result.ForLLM
	if contentForLLM == "" && result.Err != nil {
		contentForLLM = result.Err.Error()
	}
	return providers.Message{
		Role:       "tool",
		Content:    contentForLLM,
		ToolCallID: tc.ID,
		ToolName:   tc.Name,
		Images:     result.Images,
	}
}

// processToolResults handles tool execution results in order.
// Returns (confirmationNeeded, errorCount).
func (al *AgentLoop) processToolResults(
	tcResults []toolCallResult,
	messages *[]providers.Message,
	agent *AgentInstance,
	opts processOptions,
	doomDetector *DoomLoopDetector,
	agentComp string,
) (bool, int) {
	confirmationNeeded := false
	errorCount := 0

	for _, tcr := range tcResults {
		if tcr.result != nil && tcr.result.Err != nil {
			errorCount++
			if doomDetector != nil {
				doomDetector.RecordError(tcr.result.Err.Error())
			}
		}

		if tcr.result.ConfirmationRequired {
			confirmationNeeded = true
			logger.InfoCF(agentComp, "CONFIRM: tool requires confirmation",
				map[string]any{
					"tool":   tcr.tc.Name,
					"prompt": tcr.result.ConfirmationPrompt,
				})

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

			tcr.resultMsg.Content = fmt.Sprintf("[CONFIRMATION_PENDING: %s — awaiting user response]",
				tcr.result.ConfirmationPrompt)
		}

		if !tcr.result.Silent && tcr.result.ForUser != "" && opts.SendResponse {
			outContent := tcr.result.ForUser
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

		*messages = append(*messages, tcr.resultMsg)
		agent.Sessions.AddFullMessage(opts.SessionKey, tcr.resultMsg)
	}

	return confirmationNeeded, errorCount
}

// handleConfirmationWait waits for user confirmation with timeout.
func (al *AgentLoop) handleConfirmationWait(ctx context.Context, agentComp string) string {
	logger.InfoCF(agentComp, "Waiting for user confirmation (5m timeout)", nil)
	timer := time.NewTimer(5 * time.Minute)
	defer timer.Stop()

	select {
	case <-timer.C:
		logger.WarnCF(agentComp, "Tool confirmation timed out after 5 minutes", nil)
		return "Confirmation timed out after 5 minutes. Please retry if you still want to proceed."
	case <-ctx.Done():
		return ""
	}
}

// applyVerboseReasoning prepends reasoning content to final response in verbose mode.
func (al *AgentLoop) applyVerboseReasoning(finalContent, lastReasoningContent string, opts processOptions) string {
	if finalContent == "" || lastReasoningContent == "" {
		return finalContent
	}

	if v, ok := al.verboseMode.Load(opts.SessionKey); ok && v.(bool) {
		scrubbed, _ := guardrails.ScrubSecrets(lastReasoningContent)
		return fmt.Sprintf("[Reasoning]\n%s\n\n[Response]\n%s", scrubbed, finalContent)
	}

	return finalContent
}
