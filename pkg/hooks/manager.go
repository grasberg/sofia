package hooks

import (
	"context"
	"sync"
)

// HookEvent identifies a lifecycle event in the agent loop.
type HookEvent string

const (
	BeforeToolCall    HookEvent = "before_tool_call"
	AfterToolCall     HookEvent = "after_tool_call"
	OnMessageReceived HookEvent = "on_message_received"
	OnResponseReady   HookEvent = "on_response_ready"
	OnSessionStart    HookEvent = "on_session_start"
	OnSessionEnd      HookEvent = "on_session_end"
	OnError           HookEvent = "on_error"
)

// HookContext provides context to hook handlers.
type HookContext struct {
	Event      HookEvent      `json:"event"`
	AgentID    string         `json:"agent_id"`
	SessionKey string         `json:"session_key"`
	Channel    string         `json:"channel"`
	ToolName   string         `json:"tool_name,omitempty"`
	Content    string         `json:"content,omitempty"`
	Error      string         `json:"error,omitempty"`
	Metadata   map[string]any `json:"metadata,omitempty"`
}

// HookResult can modify agent behavior.
type HookResult struct {
	Cancel          bool           `json:"cancel"`
	ModifiedContent string         `json:"modified_content,omitempty"`
	Metadata        map[string]any `json:"metadata,omitempty"`
}

// HookHandler is a function called when a hook event fires.
type HookHandler func(ctx context.Context, hctx HookContext) HookResult

// HookManager manages lifecycle hooks.
type HookManager struct {
	mu       sync.RWMutex
	handlers map[HookEvent][]namedHandler
}

type namedHandler struct {
	Name    string
	Handler HookHandler
}

// NewHookManager creates a new HookManager ready for use.
func NewHookManager() *HookManager {
	return &HookManager{
		handlers: make(map[HookEvent][]namedHandler),
	}
}

// Register adds a named handler for the given event. Handlers fire in registration order.
func (hm *HookManager) Register(event HookEvent, name string, handler HookHandler) {
	hm.mu.Lock()
	defer hm.mu.Unlock()

	hm.handlers[event] = append(hm.handlers[event], namedHandler{
		Name:    name,
		Handler: handler,
	})
}

// Unregister removes a handler by name from the given event.
func (hm *HookManager) Unregister(event HookEvent, name string) {
	hm.mu.Lock()
	defer hm.mu.Unlock()

	handlers := hm.handlers[event]
	filtered := make([]namedHandler, 0, len(handlers))

	for _, h := range handlers {
		if h.Name != name {
			filtered = append(filtered, h)
		}
	}

	hm.handlers[event] = filtered
}

// Fire calls all handlers for the event in registration order. If any handler sets Cancel to true,
// execution stops and the result is returned immediately. The ModifiedContent from the last handler
// that sets it is used in the final result.
func (hm *HookManager) Fire(ctx context.Context, hctx HookContext) HookResult {
	hm.mu.RLock()
	handlers := make([]namedHandler, len(hm.handlers[hctx.Event]))
	copy(handlers, hm.handlers[hctx.Event])
	hm.mu.RUnlock()

	var result HookResult

	for _, h := range handlers {
		r := h.Handler(ctx, hctx)

		if r.Cancel {
			r.ModifiedContent = lastNonEmpty(r.ModifiedContent, result.ModifiedContent)
			r.Metadata = mergeMeta(result.Metadata, r.Metadata)
			return r
		}

		if r.ModifiedContent != "" {
			result.ModifiedContent = r.ModifiedContent
		}

		result.Metadata = mergeMeta(result.Metadata, r.Metadata)
	}

	return result
}

// ListRegistered returns a map of event to handler names for all registered hooks.
func (hm *HookManager) ListRegistered() map[HookEvent][]string {
	hm.mu.RLock()
	defer hm.mu.RUnlock()

	out := make(map[HookEvent][]string, len(hm.handlers))

	for event, handlers := range hm.handlers {
		names := make([]string, len(handlers))
		for i, h := range handlers {
			names[i] = h.Name
		}

		out[event] = names
	}

	return out
}

func lastNonEmpty(a, b string) string {
	if a != "" {
		return a
	}

	return b
}

func mergeMeta(base, overlay map[string]any) map[string]any {
	if len(base) == 0 && len(overlay) == 0 {
		return nil
	}

	merged := make(map[string]any, len(base)+len(overlay))

	for k, v := range base {
		merged[k] = v
	}

	for k, v := range overlay {
		merged[k] = v
	}

	return merged
}
