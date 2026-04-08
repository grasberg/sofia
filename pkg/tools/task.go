package tools

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// TaskItem represents a tracked task within a session.
type TaskItem struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Status      string `json:"status"` // pending, in_progress, completed, cancelled
	Description string `json:"description,omitempty"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}

// TaskStore holds session tasks in memory.
type TaskStore struct {
	tasks  map[string]*TaskItem
	mu     sync.RWMutex
	nextID atomic.Int64
}

func NewTaskStore() *TaskStore {
	s := &TaskStore{tasks: make(map[string]*TaskItem)}
	s.nextID.Store(1)
	return s
}

// TaskTool provides session task tracking: create, list, update, complete.
type TaskTool struct {
	store *TaskStore
}

func NewTaskTool() *TaskTool {
	return &TaskTool{store: NewTaskStore()}
}

func (t *TaskTool) Name() string { return "task" }
func (t *TaskTool) Description() string {
	return "Track tasks within the current session. Actions: create (add a task), list (show all tasks), update (change status/description), delete (remove a task). Helps break down complex work and report progress."
}

func (t *TaskTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"action": map[string]any{
				"type":        "string",
				"description": "Action to perform",
				"enum":        []string{"create", "list", "update", "delete"},
			},
			"id": map[string]any{
				"type":        "string",
				"description": "Task ID (required for update/delete)",
			},
			"title": map[string]any{
				"type":        "string",
				"description": "Task title (required for create)",
			},
			"description": map[string]any{
				"type":        "string",
				"description": "Task description (optional)",
			},
			"status": map[string]any{
				"type":        "string",
				"description": "Task status for update action",
				"enum":        []string{"pending", "in_progress", "completed", "cancelled"},
			},
		},
		"required": []string{"action"},
	}
}

func (t *TaskTool) Execute(_ context.Context, args map[string]any) *ToolResult {
	action, ok := args["action"].(string)
	if !ok {
		return ErrorResult("action is required")
	}

	switch action {
	case "create":
		return t.create(args)
	case "list":
		return t.list()
	case "update":
		return t.update(args)
	case "delete":
		return t.remove(args)
	default:
		return ErrorResult(fmt.Sprintf("unknown action: %s", action))
	}
}

func (t *TaskTool) create(args map[string]any) *ToolResult {
	title, ok := args["title"].(string)
	if !ok || title == "" {
		return ErrorResult("title is required for create")
	}

	now := time.Now().Format(time.RFC3339)
	id := fmt.Sprintf("%d", t.store.nextID.Add(1)-1)

	task := &TaskItem{
		ID:        id,
		Title:     title,
		Status:    "pending",
		CreatedAt: now,
		UpdatedAt: now,
	}
	if desc, ok := args["description"].(string); ok {
		task.Description = desc
	}

	t.store.mu.Lock()
	t.store.tasks[id] = task
	t.store.mu.Unlock()

	return SilentResult(fmt.Sprintf("Task #%s created: %s", id, title))
}

func (t *TaskTool) list() *ToolResult {
	t.store.mu.RLock()
	defer t.store.mu.RUnlock()

	if len(t.store.tasks) == 0 {
		return NewToolResult("No tasks.")
	}

	var sb strings.Builder
	counts := map[string]int{}
	for _, task := range t.store.tasks {
		icon := statusIcon(task.Status)
		sb.WriteString(fmt.Sprintf("%s #%s [%s] %s", icon, task.ID, task.Status, task.Title))
		if task.Description != "" {
			sb.WriteString(fmt.Sprintf(" — %s", task.Description))
		}
		sb.WriteByte('\n')
		counts[task.Status]++
	}
	sb.WriteString(fmt.Sprintf("\nTotal: %d", len(t.store.tasks)))
	for status, count := range counts {
		sb.WriteString(fmt.Sprintf(" | %s: %d", status, count))
	}
	return NewToolResult(sb.String())
}

func (t *TaskTool) update(args map[string]any) *ToolResult {
	id, ok := args["id"].(string)
	if !ok || id == "" {
		return ErrorResult("id is required for update")
	}

	t.store.mu.Lock()
	defer t.store.mu.Unlock()

	task, ok := t.store.tasks[id]
	if !ok {
		return ErrorResult(fmt.Sprintf("task #%s not found", id))
	}

	if status, ok := args["status"].(string); ok {
		task.Status = status
	}
	if title, ok := args["title"].(string); ok && title != "" {
		task.Title = title
	}
	if desc, ok := args["description"].(string); ok {
		task.Description = desc
	}
	task.UpdatedAt = time.Now().Format(time.RFC3339)

	return SilentResult(fmt.Sprintf("Task #%s updated: [%s] %s", id, task.Status, task.Title))
}

func (t *TaskTool) remove(args map[string]any) *ToolResult {
	id, ok := args["id"].(string)
	if !ok || id == "" {
		return ErrorResult("id is required for delete")
	}

	t.store.mu.Lock()
	defer t.store.mu.Unlock()

	if _, ok := t.store.tasks[id]; !ok {
		return ErrorResult(fmt.Sprintf("task #%s not found", id))
	}

	delete(t.store.tasks, id)
	return SilentResult(fmt.Sprintf("Task #%s deleted", id))
}

func statusIcon(status string) string {
	switch status {
	case "pending":
		return "[ ]"
	case "in_progress":
		return "[~]"
	case "completed":
		return "[x]"
	case "cancelled":
		return "[-]"
	default:
		return "[?]"
	}
}
