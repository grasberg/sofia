package mcp

import (
	"context"
	"encoding/json"
	"testing"

	gomcp "github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockBackend implements AgentBackend for testing.
type mockBackend struct {
	lastMessage    string
	lastSessionKey string
	lastChannel    string
	lastChatID     string
	response       string
	err            error
	agentIDs       []string
	toolNames      map[string][]string
	sessions       []SessionMeta
	history        map[string][]MessageInfo
}

func (m *mockBackend) ProcessDirect(
	_ context.Context, content, sessionKey string,
) (string, error) {
	m.lastMessage = content
	m.lastSessionKey = sessionKey
	return m.response, m.err
}

func (m *mockBackend) ProcessDirectWithChannel(
	_ context.Context,
	content, sessionKey, channel, chatID string,
) (string, error) {
	m.lastMessage = content
	m.lastSessionKey = sessionKey
	m.lastChannel = channel
	m.lastChatID = chatID
	return m.response, m.err
}

func (m *mockBackend) ListAgentIDs() []string {
	return m.agentIDs
}

func (m *mockBackend) ListAgentTools(agentID string) []string {
	if m.toolNames != nil {
		return m.toolNames[agentID]
	}
	return nil
}

func (m *mockBackend) ListSessionMetas() []SessionMeta {
	return m.sessions
}

func (m *mockBackend) GetSessionHistory(
	sessionKey string,
) []MessageInfo {
	if m.history != nil {
		return m.history[sessionKey]
	}
	return nil
}

func newTestRequest(args map[string]any) gomcp.CallToolRequest {
	return gomcp.CallToolRequest{
		Params: gomcp.CallToolParams{
			Arguments: args,
		},
	}
}

func TestNewSofiaServer(t *testing.T) {
	backend := &mockBackend{}
	s := NewSofiaServer(backend)
	assert.NotNil(t, s)
	assert.NotNil(t, s.mcpServer)
	assert.Equal(t, backend, s.backend)
	assert.NotNil(t, s.MCPServer())
}

func TestHandleChat(t *testing.T) {
	backend := &mockBackend{response: "Hello from Sofia!"}
	s := NewSofiaServer(backend)

	req := newTestRequest(map[string]any{
		"message":     "Hi there",
		"session_key": "test-session",
	})
	result, err := s.handleChat(context.Background(), req)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.False(t, result.IsError)
	assert.Equal(t, "Hi there", backend.lastMessage)
	assert.Equal(t, "test-session", backend.lastSessionKey)
	assert.Equal(t, "mcp", backend.lastChannel)
	assert.Equal(t, "server", backend.lastChatID)
}

func TestHandleChatDefaultSession(t *testing.T) {
	backend := &mockBackend{response: "OK"}
	s := NewSofiaServer(backend)

	req := newTestRequest(map[string]any{"message": "test"})
	_, err := s.handleChat(context.Background(), req)
	require.NoError(t, err)
	assert.Equal(t, "mcp:default", backend.lastSessionKey)
}

func TestHandleChatEmptyMessage(t *testing.T) {
	backend := &mockBackend{}
	s := NewSofiaServer(backend)

	req := newTestRequest(map[string]any{})
	result, err := s.handleChat(context.Background(), req)
	require.NoError(t, err)
	assert.True(t, result.IsError)
}

func TestHandleChatError(t *testing.T) {
	backend := &mockBackend{err: assert.AnError}
	s := NewSofiaServer(backend)

	req := newTestRequest(map[string]any{"message": "test"})
	result, err := s.handleChat(context.Background(), req)
	require.NoError(t, err)
	assert.True(t, result.IsError)
}

func TestHandleListAgents(t *testing.T) {
	backend := &mockBackend{
		agentIDs: []string{"default", "coder", "writer"},
	}
	s := NewSofiaServer(backend)

	result, err := s.handleListAgents(
		context.Background(), gomcp.CallToolRequest{},
	)
	require.NoError(t, err)
	assert.False(t, result.IsError)

	var data map[string]any
	tc, ok := result.Content[0].(gomcp.TextContent)
	require.True(t, ok)
	err = json.Unmarshal([]byte(tc.Text), &data)
	require.NoError(t, err)
	assert.Equal(t, float64(3), data["count"])
}

func TestHandleListTools(t *testing.T) {
	backend := &mockBackend{
		toolNames: map[string][]string{
			"default": {"read_file", "write_file", "exec"},
		},
	}
	s := NewSofiaServer(backend)

	req := newTestRequest(map[string]any{"agent_id": "default"})
	result, err := s.handleListTools(context.Background(), req)
	require.NoError(t, err)
	assert.False(t, result.IsError)

	var data map[string]any
	tc, ok := result.Content[0].(gomcp.TextContent)
	require.True(t, ok)
	err = json.Unmarshal([]byte(tc.Text), &data)
	require.NoError(t, err)
	assert.Equal(t, float64(3), data["count"])
	assert.Equal(t, "default", data["agent_id"])
}

func TestHandleListSessions(t *testing.T) {
	backend := &mockBackend{
		sessions: []SessionMeta{
			{
				Key: "s1", Channel: "telegram",
				Preview: "Hello", MessageCount: 5,
			},
			{
				Key: "s2", Channel: "cli",
				Preview: "World", MessageCount: 3,
			},
		},
	}
	s := NewSofiaServer(backend)

	result, err := s.handleListSessions(
		context.Background(), gomcp.CallToolRequest{},
	)
	require.NoError(t, err)
	assert.False(t, result.IsError)

	var data map[string]any
	tc, ok := result.Content[0].(gomcp.TextContent)
	require.True(t, ok)
	err = json.Unmarshal([]byte(tc.Text), &data)
	require.NoError(t, err)
	assert.Equal(t, float64(2), data["count"])
}

func TestHandleGetSessionHistory(t *testing.T) {
	backend := &mockBackend{
		history: map[string][]MessageInfo{
			"s1": {
				{Role: "user", Content: "Hello"},
				{Role: "assistant", Content: "Hi!"},
			},
		},
	}
	s := NewSofiaServer(backend)

	req := newTestRequest(map[string]any{"session_key": "s1"})
	result, err := s.handleGetSessionHistory(
		context.Background(), req,
	)
	require.NoError(t, err)
	assert.False(t, result.IsError)

	var data map[string]any
	tc, ok := result.Content[0].(gomcp.TextContent)
	require.True(t, ok)
	err = json.Unmarshal([]byte(tc.Text), &data)
	require.NoError(t, err)
	assert.Equal(t, float64(2), data["count"])
	assert.Equal(t, "s1", data["session_key"])
}

func TestHandleGetSessionHistoryMissingKey(t *testing.T) {
	backend := &mockBackend{}
	s := NewSofiaServer(backend)

	req := newTestRequest(map[string]any{})
	result, err := s.handleGetSessionHistory(
		context.Background(), req,
	)
	require.NoError(t, err)
	assert.True(t, result.IsError)
}
