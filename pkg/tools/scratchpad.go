package tools

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
)

// SharedScratchpad provides a concurrent key-value store for agent-to-agent communication.
// Entries are namespaced by task group to support orchestration scenarios.
type SharedScratchpad struct {
	data map[string]map[string]string // taskGroup -> key -> value
	mu   sync.RWMutex
}

// NewSharedScratchpad creates a new SharedScratchpad.
func NewSharedScratchpad() *SharedScratchpad {
	return &SharedScratchpad{
		data: make(map[string]map[string]string),
	}
}

// Write stores a value in the scratchpad under the given task group and key.
func (s *SharedScratchpad) Write(taskGroup, key, value string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.data[taskGroup]; !ok {
		s.data[taskGroup] = make(map[string]string)
	}
	s.data[taskGroup][key] = value
}

// Read retrieves a value from the scratchpad.
func (s *SharedScratchpad) Read(taskGroup, key string) (string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if group, ok := s.data[taskGroup]; ok {
		val, exists := group[key]
		return val, exists
	}
	return "", false
}

// List returns all keys in a task group.
func (s *SharedScratchpad) List(taskGroup string) []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	group, ok := s.data[taskGroup]
	if !ok {
		return nil
	}
	keys := make([]string, 0, len(group))
	for k := range group {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// ScratchpadTool provides LLM access to the shared scratchpad.
type ScratchpadTool struct {
	scratchpad   *SharedScratchpad
	defaultGroup string
}

// NewScratchpadTool creates a new ScratchpadTool.
func NewScratchpadTool(scratchpad *SharedScratchpad, defaultGroup string) *ScratchpadTool {
	if defaultGroup == "" {
		defaultGroup = "default"
	}
	return &ScratchpadTool{
		scratchpad:   scratchpad,
		defaultGroup: defaultGroup,
	}
}

func (t *ScratchpadTool) Name() string { return "scratchpad" }
func (t *ScratchpadTool) Description() string {
	return "Shared key-value scratchpad for storing and retrieving data between agents and across tool iterations. Operations: write (store a value), read (retrieve a value), list (list all keys)."
}

func (t *ScratchpadTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"operation": map[string]any{
				"type":        "string",
				"enum":        []string{"write", "read", "list"},
				"description": "The operation to perform",
			},
			"key": map[string]any{
				"type":        "string",
				"description": "The key to read or write (required for read/write)",
			},
			"value": map[string]any{
				"type":        "string",
				"description": "The value to store (required for write)",
			},
			"group": map[string]any{
				"type":        "string",
				"description": "Task group namespace (optional, defaults to 'default')",
			},
		},
		"required": []string{"operation"},
	}
}

func (t *ScratchpadTool) Execute(ctx context.Context, args map[string]any) *ToolResult {
	op, _ := args["operation"].(string)
	group, _ := args["group"].(string)
	if group == "" {
		group = t.defaultGroup
	}

	switch op {
	case "write":
		key, _ := args["key"].(string)
		value, _ := args["value"].(string)
		if key == "" {
			return ErrorResult("key is required for write operation")
		}
		t.scratchpad.Write(group, key, value)
		return SilentResult(fmt.Sprintf("Stored key %q in group %q", key, group))

	case "read":
		key, _ := args["key"].(string)
		if key == "" {
			return ErrorResult("key is required for read operation")
		}
		val, ok := t.scratchpad.Read(group, key)
		if !ok {
			return SilentResult(fmt.Sprintf("Key %q not found in group %q", key, group))
		}
		return SilentResult(fmt.Sprintf("Key %q in group %q: %s", key, group, val))

	case "list":
		keys := t.scratchpad.List(group)
		if len(keys) == 0 {
			return SilentResult(fmt.Sprintf("No keys in group %q", group))
		}
		return SilentResult(fmt.Sprintf("Keys in group %q:\n- %s", group, strings.Join(keys, "\n- ")))

	default:
		return ErrorResult(fmt.Sprintf("unknown operation: %s", op))
	}
}
