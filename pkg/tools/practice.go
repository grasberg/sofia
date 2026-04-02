package tools

import (
	"context"
	"fmt"

	"github.com/grasberg/sofia/pkg/logger"
	"github.com/grasberg/sofia/pkg/memory"
)

// PracticeTool retrieves failed tasks from memory and spawns a subagent to practice them.
type PracticeTool struct {
	db       *memory.MemoryDB
	manager  *SubagentManager
	agentID  string
	callback AsyncCallback
}

func NewPracticeTool(db *memory.MemoryDB, manager *SubagentManager, agentID string) *PracticeTool {
	return &PracticeTool{
		db:      db,
		manager: manager,
		agentID: agentID,
	}
}

// SetCallback implements AsyncTool interface for async completion notification
func (t *PracticeTool) SetCallback(cb AsyncCallback) {
	t.callback = cb
}

func (t *PracticeTool) Name() string {
	return "practice_past_failures"
}

func (t *PracticeTool) Description() string {
	return "Retrieve past failed tasks from memory and spawn a sandbox subagent to practice and find a better approach. Run this when you have idle time to self-improve."
}

func (t *PracticeTool) Parameters() map[string]any {
	return map[string]any{
		"type":       "object",
		"properties": map[string]any{},
	}
}

func (t *PracticeTool) Execute(ctx context.Context, args map[string]any) *ToolResult {
	if t.db == nil || t.manager == nil {
		return ErrorResult("Practice tool missing dependencies")
	}

	records, err := t.db.GetFailedReflections(t.agentID, 1)
	if err != nil || len(records) == 0 {
		return UserResult("No recent failed tasks found to practice.")
	}

	record := records[0]

	practiceTask := fmt.Sprintf(`[SANDBOX PRACTICE SCENARIO]
You are in a sandbox practice environment. You must solve this task: 
%s

In a previous attempt, you failed because:
%s

Lessons learned from that failure:
%s

Find a robust solution to this problem, applying the lessons learned. Report your findings and any new system instructions that should be adopted.`,
		record.TaskSummary, record.WhatFailed, record.Lessons)

	logger.InfoCF("practice", "Spawning subagent for practice scenario", map[string]any{"task": record.TaskSummary})

	// Wrap the callback to process the practice result
	var customCallback AsyncCallback
	if t.callback != nil {
		customCallback = func(ctx context.Context, res *ToolResult) {
			msg := fmt.Sprintf("Practice session for '%s' completed.\nResult: %s", record.TaskSummary, res.ForLLM)
			logger.InfoCF("practice", "Practice subagent finished", nil)
			t.callback(ctx, UserResult(msg))
		}
	}

	resultMsg, err := t.manager.Spawn(
		ctx,
		practiceTask,
		"practice-sandbox",
		"",
		nil,
		"internal",
		"practice",
		customCallback,
	)
	if err != nil {
		return ErrorResult(fmt.Sprintf("Failed to spawn practice subagent: %v", err))
	}

	return AsyncResult(fmt.Sprintf("Spawned practice subagent for failed task: %s\n%s", record.TaskSummary, resultMsg))
}
