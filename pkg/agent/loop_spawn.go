package agent

import (
	"context"
	"fmt"
	"time"

	"github.com/grasberg/sofia/pkg/logger"
	"github.com/grasberg/sofia/pkg/reputation"
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
