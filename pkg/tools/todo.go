package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// TodoItem represents a single persistent todo entry.
type TodoItem struct {
	ID        int64  `json:"id"`
	Text      string `json:"text"`
	Status    string `json:"status"` // pending, in_progress, done
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

// TodoStore holds persistent todos backed by a JSON file.
type TodoStore struct {
	items  map[int64]*TodoItem
	nextID atomic.Int64
	path   string
	mu     sync.RWMutex
}

// NewTodoStore creates a TodoStore that persists to the given file path.
// If the file already exists, its contents are loaded.
func NewTodoStore(path string) *TodoStore {
	s := &TodoStore{
		items: make(map[int64]*TodoItem),
		path:  path,
	}
	s.nextID.Store(1)
	s.load()
	return s
}

func (s *TodoStore) load() {
	data, err := os.ReadFile(s.path)
	if err != nil {
		return // File does not exist yet — start empty.
	}

	var items []*TodoItem
	if err := json.Unmarshal(data, &items); err != nil {
		return
	}

	var maxID int64
	for _, item := range items {
		s.items[item.ID] = item
		if item.ID > maxID {
			maxID = item.ID
		}
	}
	s.nextID.Store(maxID + 1)
}

func (s *TodoStore) save() error {
	items := s.sortedItems()

	data, err := json.MarshalIndent(items, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal todos: %w", err)
	}

	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	return os.WriteFile(s.path, data, 0o644)
}

func (s *TodoStore) sortedItems() []*TodoItem {
	items := make([]*TodoItem, 0, len(s.items))
	for _, item := range s.items {
		items = append(items, item)
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].ID < items[j].ID
	})
	return items
}

// TodoTool provides a persistent todo list that survives context compaction.
// Todos are stored in ~/.sofia/todos.json.
type TodoTool struct {
	store *TodoStore
}

// NewTodoTool creates a TodoTool backed by the given file path.
func NewTodoTool(path string) *TodoTool {
	return &TodoTool{store: NewTodoStore(path)}
}

// DefaultTodoPath returns the default persistence path (~/.sofia/todos.json).
func DefaultTodoPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}
	return filepath.Join(home, ".sofia", "todos.json")
}

func (t *TodoTool) Name() string { return "todo" }

func (t *TodoTool) Description() string {
	return "Persistent todo list that survives context compaction. Actions: add (create a todo), list (show all todos as markdown checklist), update (change status), remove (delete a todo), clear (remove all done todos). Stored in ~/.sofia/todos.json."
}

func (t *TodoTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"action": map[string]any{
				"type":        "string",
				"description": "Action to perform",
				"enum":        []string{"add", "list", "update", "remove", "clear"},
			},
			"text": map[string]any{
				"type":        "string",
				"description": "Todo text (required for add)",
			},
			"id": map[string]any{
				"type":        "number",
				"description": "Todo ID (required for update/remove)",
			},
			"status": map[string]any{
				"type":        "string",
				"description": "New status (for update)",
				"enum":        []string{"pending", "in_progress", "done"},
			},
		},
		"required": []string{"action"},
	}
}

func (t *TodoTool) Execute(_ context.Context, args map[string]any) *ToolResult {
	action, ok := args["action"].(string)
	if !ok || action == "" {
		return ErrorResult("action is required")
	}

	switch action {
	case "add":
		return t.add(args)
	case "list":
		return t.list()
	case "update":
		return t.update(args)
	case "remove":
		return t.remove(args)
	case "clear":
		return t.clear()
	default:
		return ErrorResult(fmt.Sprintf("unknown action: %s", action))
	}
}

func (t *TodoTool) add(args map[string]any) *ToolResult {
	text, ok := args["text"].(string)
	if !ok || text == "" {
		return ErrorResult("text is required for add")
	}

	now := time.Now().Format(time.RFC3339)
	id := t.store.nextID.Add(1) - 1

	item := &TodoItem{
		ID:        id,
		Text:      text,
		Status:    "pending",
		CreatedAt: now,
		UpdatedAt: now,
	}

	t.store.mu.Lock()
	t.store.items[id] = item
	err := t.store.save()
	t.store.mu.Unlock()

	if err != nil {
		return ErrorResult(fmt.Sprintf("failed to save: %v", err))
	}

	return SilentResult(fmt.Sprintf("Todo #%d added: %s", id, text))
}

func (t *TodoTool) list() *ToolResult {
	t.store.mu.RLock()
	defer t.store.mu.RUnlock()

	if len(t.store.items) == 0 {
		return NewToolResult("No todos.")
	}

	items := t.store.sortedItems()
	return NewToolResult(formatTodoList(items))
}

func formatTodoList(items []*TodoItem) string {
	var sb strings.Builder
	sb.WriteString("## Todos\n\n")

	counts := map[string]int{}
	for _, item := range items {
		checkbox := todoCheckbox(item.Status)
		label := ""
		if item.Status == "in_progress" {
			label = " *(in progress)*"
		}
		sb.WriteString(fmt.Sprintf("%s #%d: %s%s\n", checkbox, item.ID, item.Text, label))
		counts[item.Status]++
	}

	sb.WriteString(fmt.Sprintf("\nTotal: %d", len(items)))
	for _, status := range []string{"pending", "in_progress", "done"} {
		if c, ok := counts[status]; ok {
			sb.WriteString(fmt.Sprintf(" | %s: %d", status, c))
		}
	}
	sb.WriteByte('\n')

	return sb.String()
}

func todoCheckbox(status string) string {
	switch status {
	case "done":
		return "- [x]"
	case "in_progress":
		return "- [~]"
	default:
		return "- [ ]"
	}
}

func (t *TodoTool) update(args map[string]any) *ToolResult {
	id, ok := extractTodoID(args)
	if !ok {
		return ErrorResult("id is required for update")
	}

	status, _ := args["status"].(string)
	if status == "" {
		return ErrorResult("status is required for update")
	}

	switch status {
	case "pending", "in_progress", "done":
		// valid
	default:
		return ErrorResult(fmt.Sprintf("invalid status: %s (must be pending, in_progress, or done)", status))
	}

	t.store.mu.Lock()
	item, exists := t.store.items[id]
	if !exists {
		t.store.mu.Unlock()
		return ErrorResult(fmt.Sprintf("todo #%d not found", id))
	}

	item.Status = status
	item.UpdatedAt = time.Now().Format(time.RFC3339)
	err := t.store.save()
	t.store.mu.Unlock()

	if err != nil {
		return ErrorResult(fmt.Sprintf("failed to save: %v", err))
	}

	return SilentResult(fmt.Sprintf("Todo #%d updated to %s: %s", id, status, item.Text))
}

func (t *TodoTool) remove(args map[string]any) *ToolResult {
	id, ok := extractTodoID(args)
	if !ok {
		return ErrorResult("id is required for remove")
	}

	t.store.mu.Lock()
	if _, exists := t.store.items[id]; !exists {
		t.store.mu.Unlock()
		return ErrorResult(fmt.Sprintf("todo #%d not found", id))
	}

	delete(t.store.items, id)
	err := t.store.save()
	t.store.mu.Unlock()

	if err != nil {
		return ErrorResult(fmt.Sprintf("failed to save: %v", err))
	}

	return SilentResult(fmt.Sprintf("Todo #%d removed", id))
}

func (t *TodoTool) clear() *ToolResult {
	t.store.mu.Lock()
	removed := 0
	for id, item := range t.store.items {
		if item.Status == "done" {
			delete(t.store.items, id)
			removed++
		}
	}
	err := t.store.save()
	t.store.mu.Unlock()

	if err != nil {
		return ErrorResult(fmt.Sprintf("failed to save: %v", err))
	}

	if removed == 0 {
		return NewToolResult("No completed todos to clear.")
	}
	return SilentResult(fmt.Sprintf("Cleared %d completed todo(s)", removed))
}

// extractTodoID handles both float64 (from JSON) and string ID representations.
func extractTodoID(args map[string]any) (int64, bool) {
	switch v := args["id"].(type) {
	case float64:
		return int64(v), true
	case int64:
		return v, true
	case int:
		return int64(v), true
	case string:
		// Try parsing "1", "2", etc.
		var id int64
		if _, err := fmt.Sscanf(v, "%d", &id); err == nil {
			return id, true
		}
	}
	return 0, false
}
