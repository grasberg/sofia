package mcp

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"

	"github.com/grasberg/sofia/pkg/logger"
)

// AgentBackend is the interface the MCP server needs from the agent loop.
// This avoids importing the agent package directly and prevents circular deps.
type AgentBackend interface {
	// ProcessDirect sends a message to the default agent and returns the response.
	ProcessDirect(ctx context.Context, content, sessionKey string) (string, error)
	// ProcessDirectWithChannel sends a message with channel context.
	ProcessDirectWithChannel(
		ctx context.Context, content, sessionKey, channel, chatID string,
	) (string, error)
	// ListAgentIDs returns all registered agent IDs.
	ListAgentIDs() []string
	// ListAgentTools returns tool names for a given agent.
	ListAgentTools(agentID string) []string
	// ListSessionMetas returns session metadata.
	ListSessionMetas() []SessionMeta
	// GetSessionHistory returns messages for a session key.
	GetSessionHistory(sessionKey string) []MessageInfo
}

// SessionMeta is a lightweight session summary.
type SessionMeta struct {
	Key          string `json:"key"`
	Channel      string `json:"channel"`
	Preview      string `json:"preview"`
	MessageCount int    `json:"message_count"`
}

// MessageInfo is a simplified message representation.
type MessageInfo struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// SofiaServer wraps an mcp-go MCPServer configured with Sofia's capabilities.
type SofiaServer struct {
	mcpServer *mcpserver.MCPServer
	backend   AgentBackend
}

// NewSofiaServer creates a new MCP server exposing Sofia as a tool provider.
func NewSofiaServer(backend AgentBackend) *SofiaServer {
	s := &SofiaServer{backend: backend}

	instructions := "Sofia is a personal AI agent gateway. " +
		"Use the 'chat' tool to send messages and get responses."

	s.mcpServer = mcpserver.NewMCPServer(
		"Sofia",
		"1.0.0",
		mcpserver.WithToolCapabilities(false),
		mcpserver.WithInstructions(instructions),
	)

	s.registerTools()
	return s
}

// MCPServer returns the underlying mcp-go server instance.
func (s *SofiaServer) MCPServer() *mcpserver.MCPServer {
	return s.mcpServer
}

func (s *SofiaServer) registerTools() {
	s.mcpServer.AddTool(
		mcp.Tool{
			Name: "chat",
			Description: "Send a message to Sofia and receive a response. " +
				"Use session_key to maintain conversation context across calls.",
			InputSchema: mcp.ToolInputSchema{
				Type: "object",
				Properties: map[string]any{
					"message": map[string]any{
						"type":        "string",
						"description": "The message to send to Sofia",
					},
					"session_key": map[string]any{
						"type":        "string",
						"description": "Session key for conversation continuity",
					},
					"agent_id": map[string]any{
						"type":        "string",
						"description": "Target agent ID (optional)",
					},
				},
				Required: []string{"message"},
			},
		},
		s.handleChat,
	)

	s.mcpServer.AddTool(
		mcp.Tool{
			Name:        "list_agents",
			Description: "List all available Sofia agents with their IDs.",
			InputSchema: mcp.ToolInputSchema{
				Type:       "object",
				Properties: map[string]any{},
			},
		},
		s.handleListAgents,
	)

	s.mcpServer.AddTool(
		mcp.Tool{
			Name:        "list_tools",
			Description: "List tools available to a specific Sofia agent.",
			InputSchema: mcp.ToolInputSchema{
				Type: "object",
				Properties: map[string]any{
					"agent_id": map[string]any{
						"type":        "string",
						"description": "Agent ID to list tools for",
					},
				},
			},
		},
		s.handleListTools,
	)

	s.mcpServer.AddTool(
		mcp.Tool{
			Name:        "list_sessions",
			Description: "List all conversation sessions with metadata.",
			InputSchema: mcp.ToolInputSchema{
				Type:       "object",
				Properties: map[string]any{},
			},
		},
		s.handleListSessions,
	)

	s.mcpServer.AddTool(
		mcp.Tool{
			Name: "get_session_history",
			Description: "Get the message history for a specific " +
				"conversation session.",
			InputSchema: mcp.ToolInputSchema{
				Type: "object",
				Properties: map[string]any{
					"session_key": map[string]any{
						"type":        "string",
						"description": "The session key to retrieve history for",
					},
				},
				Required: []string{"session_key"},
			},
		},
		s.handleGetSessionHistory,
	)
}

func (s *SofiaServer) handleChat(
	ctx context.Context,
	req mcp.CallToolRequest,
) (*mcp.CallToolResult, error) {
	args := req.GetArguments()
	message, ok := args["message"].(string)
	if !ok || message == "" {
		return mcp.NewToolResultError("message is required"), nil
	}

	sessionKey, ok := args["session_key"].(string)
	if !ok || sessionKey == "" {
		sessionKey = "mcp:default"
	}

	logger.InfoCF("mcp-server", "Processing chat request", map[string]any{
		"session_key": sessionKey,
		"message_len": len(message),
	})

	response, err := s.backend.ProcessDirectWithChannel(
		ctx, message, sessionKey, "mcp", "server",
	)
	if err != nil {
		logger.ErrorCF("mcp-server", "Chat request failed", map[string]any{
			"error": err.Error(),
		})
		return mcp.NewToolResultError(
			fmt.Sprintf("Sofia error: %v", err),
		), nil
	}

	return mcp.NewToolResultText(response), nil
}

func (s *SofiaServer) handleListAgents(
	_ context.Context,
	_ mcp.CallToolRequest,
) (*mcp.CallToolResult, error) {
	agentIDs := s.backend.ListAgentIDs()
	data, err := json.Marshal(map[string]any{
		"agents": agentIDs,
		"count":  len(agentIDs),
	})
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return mcp.NewToolResultText(string(data)), nil
}

func (s *SofiaServer) handleListTools(
	_ context.Context,
	req mcp.CallToolRequest,
) (*mcp.CallToolResult, error) {
	args := req.GetArguments()
	agentID, _ := args["agent_id"].(string) //nolint:errcheck
	tools := s.backend.ListAgentTools(agentID)
	data, err := json.Marshal(map[string]any{
		"agent_id": agentID,
		"tools":    tools,
		"count":    len(tools),
	})
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return mcp.NewToolResultText(string(data)), nil
}

func (s *SofiaServer) handleListSessions(
	_ context.Context,
	_ mcp.CallToolRequest,
) (*mcp.CallToolResult, error) {
	sessions := s.backend.ListSessionMetas()
	data, err := json.Marshal(map[string]any{
		"sessions": sessions,
		"count":    len(sessions),
	})
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return mcp.NewToolResultText(string(data)), nil
}

func (s *SofiaServer) handleGetSessionHistory(
	_ context.Context,
	req mcp.CallToolRequest,
) (*mcp.CallToolResult, error) {
	args := req.GetArguments()
	sessionKey, ok := args["session_key"].(string)
	if !ok || sessionKey == "" {
		return mcp.NewToolResultError("session_key is required"), nil
	}
	messages := s.backend.GetSessionHistory(sessionKey)
	data, err := json.Marshal(map[string]any{
		"session_key": sessionKey,
		"messages":    messages,
		"count":       len(messages),
	})
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return mcp.NewToolResultText(string(data)), nil
}

// ServeStdio starts the MCP server listening on stdin/stdout.
func (s *SofiaServer) ServeStdio() error {
	return mcpserver.ServeStdio(s.mcpServer)
}

// ServeSSE starts the MCP server as an SSE HTTP server on the given address.
func (s *SofiaServer) ServeSSE(addr string) error {
	sseServer := mcpserver.NewSSEServer(s.mcpServer)
	logger.InfoCF("mcp-server",
		fmt.Sprintf("MCP SSE server listening on %s", addr), nil)
	return sseServer.Start(addr)
}
