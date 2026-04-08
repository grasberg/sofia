package tools

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/grasberg/sofia/pkg/logger"
	"github.com/grasberg/sofia/pkg/providers"
)

type ToolRegistry struct {
	tools          map[string]Tool
	tracker        *ToolTracker
	circuitBreaker *CircuitBreaker
	schemaCache    map[string]providers.ToolDefinition // cached provider definitions per tool name
	mu             sync.RWMutex
	version        atomic.Int64 // incremented on Register/Unregister for cache invalidation
}

func NewToolRegistry() *ToolRegistry {
	return &ToolRegistry{
		tools:       make(map[string]Tool),
		schemaCache: make(map[string]providers.ToolDefinition),
	}
}

// SetTracker attaches a ToolTracker to the registry to observe performance metrics.
func (r *ToolRegistry) SetTracker(t *ToolTracker) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tracker = t
}

// SetCircuitBreaker attaches a CircuitBreaker to the registry. When set, tool
// execution is gated by circuit state and failures/successes are recorded.
func (r *ToolRegistry) SetCircuitBreaker(cb *CircuitBreaker) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.circuitBreaker = cb
}

func (r *ToolRegistry) Register(tool Tool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tools[tool.Name()] = tool
	delete(r.schemaCache, tool.Name()) // invalidate cached schema
	r.version.Add(1)
}

// Unregister removes a tool from the registry.
func (r *ToolRegistry) Unregister(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.tools, name)
	delete(r.schemaCache, name) // invalidate cached schema
	r.version.Add(1)
}

// GetVersion returns the current registry version.
// The version is incremented on each Register/Unregister call.
func (r *ToolRegistry) GetVersion() int64 {
	return r.version.Load()
}

func (r *ToolRegistry) Get(name string) (Tool, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	tool, ok := r.tools[name]
	return tool, ok
}

// GetProviderDefinition returns a cached providers.ToolDefinition for the named tool.
// The result is cached after first computation and invalidated on Register/Unregister.
func (r *ToolRegistry) GetProviderDefinition(name string) (providers.ToolDefinition, bool) {
	r.mu.RLock()
	if cached, ok := r.schemaCache[name]; ok {
		r.mu.RUnlock()
		return cached, true
	}
	r.mu.RUnlock()

	r.mu.Lock()
	defer r.mu.Unlock()

	// Double-check after acquiring write lock.
	if cached, ok := r.schemaCache[name]; ok {
		return cached, true
	}

	tool, ok := r.tools[name]
	if !ok {
		return providers.ToolDefinition{}, false
	}

	schema := ToolToSchema(tool)
	fn, ok := schema["function"].(map[string]any)
	if !ok {
		return providers.ToolDefinition{}, false
	}

	fnName, _ := fn["name"].(string)
	desc, _ := fn["description"].(string)
	params, _ := fn["parameters"].(map[string]any)

	def := providers.ToolDefinition{
		Type: "function",
		Function: providers.ToolFunctionDefinition{
			Name:        fnName,
			Description: desc,
			Parameters:  params,
		},
	}
	r.schemaCache[name] = def
	return def, true
}

func (r *ToolRegistry) Execute(ctx context.Context, name string, args map[string]any) *ToolResult {
	return r.ExecuteWithContext(ctx, name, args, "", "", "", nil)
}

// ExecuteWithContext executes a tool with channel/chatID/sessionKey context and optional async callback.
// If the tool implements AsyncTool and a non-nil callback is provided,
// the callback will be set on the tool before execution.
func (r *ToolRegistry) ExecuteWithContext(
	ctx context.Context,
	name string,
	args map[string]any,
	channel, chatID, sessionKey string,
	asyncCallback AsyncCallback,
) *ToolResult {
	if sessionKey != "" {
		ctx = WithSessionKey(ctx, sessionKey)
	}
	logger.InfoCF("tool", "Tool execution started",
		map[string]any{
			"tool": name,
			"args": args,
		})

	tool, ok := r.Get(name)
	if !ok {
		logger.ErrorCF("tool", "Tool not found",
			map[string]any{
				"tool": name,
			})
		return ErrorResult(fmt.Sprintf("tool %q not found", name)).WithError(fmt.Errorf("tool not found"))
	}

	// Circuit breaker gate: reject execution if the circuit is open.
	r.mu.RLock()
	cb := r.circuitBreaker
	r.mu.RUnlock()
	if cb != nil && !cb.AllowExecution(name) {
		msg := circuitBreakerError(name)
		return ErrorResult(msg).
			WithRetryHint("The tool is temporarily disabled. Try again later or use an alternative.").
			WithError(fmt.Errorf("circuit breaker open for tool %q", name))
	}

	// If tool implements ContextualTool, set context
	if contextualTool, ok := tool.(ContextualTool); ok && channel != "" && chatID != "" {
		contextualTool.SetContext(channel, chatID)
	}

	// If tool implements AsyncTool and callback is provided, set callback
	if asyncTool, ok := tool.(AsyncTool); ok && asyncCallback != nil {
		asyncTool.SetCallback(asyncCallback)
		logger.DebugCF("tool", "Async callback injected",
			map[string]any{
				"tool": name,
			})
	}

	start := time.Now()
	result := tool.Execute(ctx, args)
	duration := time.Since(start)

	// Record execution metrics if tracker is active
	r.mu.RLock()
	tracker := r.tracker
	r.mu.RUnlock()
	if tracker != nil {
		tracker.Record(name, duration, result.IsError)
	}

	// Record circuit breaker outcome
	if cb != nil {
		if result.IsError {
			cb.RecordFailure(name)
		} else {
			cb.RecordSuccess(name)
		}
	}

	// Log based on result type
	if result.IsError {
		logger.ErrorCF("tool", "Tool execution failed",
			map[string]any{
				"tool":     name,
				"duration": duration.Milliseconds(),
				"error":    result.ForLLM,
			})
	} else if result.Async {
		logger.InfoCF("tool", "Tool started (async)",
			map[string]any{
				"tool":     name,
				"duration": duration.Milliseconds(),
			})
	} else {
		logger.InfoCF("tool", "Tool execution completed",
			map[string]any{
				"tool":          name,
				"duration_ms":   duration.Milliseconds(),
				"result_length": len(result.ForLLM),
			})
	}

	return result
}

// sortedToolNames returns tool names in sorted order for deterministic iteration.
// This is critical for KV cache stability: non-deterministic map iteration would
// produce different system prompts and tool definitions on each call, invalidating
// the LLM's prefix cache even when no tools have changed.
func (r *ToolRegistry) sortedToolNames() []string {
	names := make([]string, 0, len(r.tools))
	for name := range r.tools {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func (r *ToolRegistry) GetDefinitions() []map[string]any {
	r.mu.RLock()
	defer r.mu.RUnlock()

	sorted := r.sortedToolNames()
	definitions := make([]map[string]any, 0, len(sorted))
	for _, name := range sorted {
		definitions = append(definitions, ToolToSchema(r.tools[name]))
	}
	return definitions
}

// ToProviderDefs converts tool definitions to provider-compatible format.
// This is the format expected by LLM provider APIs.
func (r *ToolRegistry) ToProviderDefs() []providers.ToolDefinition {
	r.mu.RLock()
	defer r.mu.RUnlock()

	sorted := r.sortedToolNames()
	definitions := make([]providers.ToolDefinition, 0, len(sorted))
	for _, name := range sorted {
		tool := r.tools[name]
		schema := ToolToSchema(tool)

		// Safely extract nested values with type checks
		fn, ok := schema["function"].(map[string]any)
		if !ok {
			continue
		}

		name, _ := fn["name"].(string)
		desc, _ := fn["description"].(string)
		params, _ := fn["parameters"].(map[string]any)

		definitions = append(definitions, providers.ToolDefinition{
			Type: "function",
			Function: providers.ToolFunctionDefinition{
				Name:        name,
				Description: desc,
				Parameters:  params,
			},
		})
	}
	return definitions
}

// List returns a list of all registered tool names.
func (r *ToolRegistry) List() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.sortedToolNames()
}

// Count returns the number of registered tools.
func (r *ToolRegistry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.tools)
}

// GetSummaries returns human-readable summaries of all registered tools.
// Returns a slice of "name - description" strings.
func (r *ToolRegistry) GetSummaries() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	sorted := r.sortedToolNames()
	summaries := make([]string, 0, len(sorted))
	for _, name := range sorted {
		tool := r.tools[name]
		summaries = append(summaries, fmt.Sprintf("- `%s` - %s", tool.Name(), tool.Description()))
	}
	return summaries
}
