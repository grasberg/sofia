package hooks

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHookManager_RegisterAndFire(t *testing.T) {
	hm := NewHookManager()
	called := false

	hm.Register(BeforeToolCall, "test-hook", func(_ context.Context, hctx HookContext) HookResult {
		called = true
		assert.Equal(t, BeforeToolCall, hctx.Event)
		assert.Equal(t, "agent-1", hctx.AgentID)
		assert.Equal(t, "sess-1", hctx.SessionKey)
		assert.Equal(t, "telegram", hctx.Channel)
		assert.Equal(t, "exec", hctx.ToolName)

		return HookResult{}
	})

	result := hm.Fire(context.Background(), HookContext{
		Event:      BeforeToolCall,
		AgentID:    "agent-1",
		SessionKey: "sess-1",
		Channel:    "telegram",
		ToolName:   "exec",
	})

	assert.True(t, called, "handler should have been called")
	assert.False(t, result.Cancel)
	assert.Empty(t, result.ModifiedContent)
}

func TestHookManager_CancelOperation(t *testing.T) {
	hm := NewHookManager()
	secondCalled := false

	hm.Register(BeforeToolCall, "blocker", func(_ context.Context, _ HookContext) HookResult {
		return HookResult{Cancel: true}
	})

	hm.Register(BeforeToolCall, "after-blocker", func(_ context.Context, _ HookContext) HookResult {
		secondCalled = true
		return HookResult{}
	})

	result := hm.Fire(context.Background(), HookContext{Event: BeforeToolCall})

	assert.True(t, result.Cancel)
	assert.False(t, secondCalled, "second handler should not run after cancel")
}

func TestHookManager_ModifyContent(t *testing.T) {
	hm := NewHookManager()

	hm.Register(OnMessageReceived, "modifier", func(_ context.Context, _ HookContext) HookResult {
		return HookResult{ModifiedContent: "modified message"}
	})

	result := hm.Fire(context.Background(), HookContext{
		Event:   OnMessageReceived,
		Content: "original message",
	})

	assert.False(t, result.Cancel)
	assert.Equal(t, "modified message", result.ModifiedContent)
}

func TestHookManager_MultipleHandlers(t *testing.T) {
	hm := NewHookManager()
	order := make([]string, 0, 3)

	hm.Register(OnResponseReady, "first", func(_ context.Context, _ HookContext) HookResult {
		order = append(order, "first")
		return HookResult{
			ModifiedContent: "from-first",
			Metadata:        map[string]any{"a": 1},
		}
	})

	hm.Register(OnResponseReady, "second", func(_ context.Context, _ HookContext) HookResult {
		order = append(order, "second")
		return HookResult{
			Metadata: map[string]any{"b": 2},
		}
	})

	hm.Register(OnResponseReady, "third", func(_ context.Context, _ HookContext) HookResult {
		order = append(order, "third")
		return HookResult{
			ModifiedContent: "from-third",
			Metadata:        map[string]any{"c": 3},
		}
	})

	result := hm.Fire(context.Background(), HookContext{Event: OnResponseReady})

	assert.Equal(t, []string{"first", "second", "third"}, order)
	assert.Equal(t, "from-third", result.ModifiedContent, "last modifier wins")
	assert.Equal(t, 1, result.Metadata["a"])
	assert.Equal(t, 2, result.Metadata["b"])
	assert.Equal(t, 3, result.Metadata["c"])
}

func TestHookManager_Unregister(t *testing.T) {
	hm := NewHookManager()
	called := false

	hm.Register(OnSessionStart, "ephemeral", func(_ context.Context, _ HookContext) HookResult {
		called = true
		return HookResult{}
	})

	hm.Unregister(OnSessionStart, "ephemeral")

	hm.Fire(context.Background(), HookContext{Event: OnSessionStart})

	assert.False(t, called, "unregistered handler should not be called")

	registered := hm.ListRegistered()
	names, ok := registered[OnSessionStart]
	require.True(t, ok)
	assert.Empty(t, names)
}

func TestHookManager_NoHandlers(t *testing.T) {
	hm := NewHookManager()

	result := hm.Fire(context.Background(), HookContext{
		Event:   OnError,
		AgentID: "agent-1",
		Error:   "something failed",
	})

	assert.False(t, result.Cancel)
	assert.Empty(t, result.ModifiedContent)
	assert.Nil(t, result.Metadata)
}

func TestHookManager_ListRegistered(t *testing.T) {
	hm := NewHookManager()

	hm.Register(BeforeToolCall, "hook-a", func(_ context.Context, _ HookContext) HookResult {
		return HookResult{}
	})

	hm.Register(BeforeToolCall, "hook-b", func(_ context.Context, _ HookContext) HookResult {
		return HookResult{}
	})

	hm.Register(OnError, "error-hook", func(_ context.Context, _ HookContext) HookResult {
		return HookResult{}
	})

	registered := hm.ListRegistered()

	require.Contains(t, registered, BeforeToolCall)
	assert.Equal(t, []string{"hook-a", "hook-b"}, registered[BeforeToolCall])

	require.Contains(t, registered, OnError)
	assert.Equal(t, []string{"error-hook"}, registered[OnError])
}
