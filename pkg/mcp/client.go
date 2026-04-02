package mcp

import (
	"context"
	"fmt"
	"sync"

	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"

	"github.com/grasberg/sofia/pkg/config"
	"github.com/grasberg/sofia/pkg/logger"
)

// MCPServerInstance holds an active connection to an MCP server.
type MCPServerInstance struct {
	Name    string
	Client  client.MCPClient
	Cleanup func()
	Tools   []mcp.Tool
}

// GlobalManager manages singleton connections to MCP servers based on the configuration.
// This prevents spawning a new subprocess every time an agent is initialized.
type GlobalManager struct {
	mu       sync.Mutex
	servers  map[string]*MCPServerInstance
	isClosed bool
}

// NewGlobalManager creates a new empty GlobalManager.
func NewGlobalManager() *GlobalManager {
	return &GlobalManager{
		servers: make(map[string]*MCPServerInstance),
	}
}

// EnsureServers ensures that all configured MCP servers are running and ready.
// It starts any that are not currently running and tears down any that have been removed from config.
// Call this during application initialization or when config changes.
func (m *GlobalManager) EnsureServers(ctx context.Context, cfg map[string]config.MCPServerConfig) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.isClosed {
		return fmt.Errorf("GlobalManager is closed")
	}

	// 1. Identify and shutdown servers no longer in config
	for name, srv := range m.servers {
		if _, exists := cfg[name]; !exists {
			logger.InfoCF("mcp", "Shutting down removed MCP server", map[string]any{"server": name})
			srv.Cleanup()
			delete(m.servers, name)
		}
	}

	// 2. Start missing servers
	for name, srvCfg := range cfg {
		if _, exists := m.servers[name]; exists {
			continue // Already running
		}

		logger.InfoCF("mcp", "Starting MCP server", map[string]any{
			"server":  name,
			"command": srvCfg.Command,
		})

		// Convert config env map to slice of "KEY=VALUE" strings
		var env []string
		for k, v := range srvCfg.Env {
			env = append(env, fmt.Sprintf("%s=%s", k, v))
		}

		mcpClient, err := client.NewStdioMCPClient(
			srvCfg.Command,
			env,
			srvCfg.Args...,
		)
		if err != nil {
			logger.ErrorCF("mcp", "Failed to start standard IO client for MCP server", map[string]any{
				"server": name,
				"error":  err.Error(),
			})
			continue // Keep trying others, log failure
		}

		err = mcpClient.Ping(
			ctx,
		) // In mcp-go, we might not need Connect(), but Stdio client starts implicitly. Let's ping instead to verify.
		if err != nil {
			logger.ErrorCF("mcp", "Failed to connect to MCP server", map[string]any{
				"server": name,
				"error":  err.Error(),
			})
			mcpClient.Close()
			continue
		}

		initReq := mcp.InitializeRequest{}
		initReq.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
		initReq.Params.ClientInfo.Name = "Sofia"
		initReq.Params.ClientInfo.Version = "1.0.0"

		_, err = mcpClient.Initialize(ctx, initReq)
		if err != nil {
			logger.ErrorCF("mcp", "Failed to initialize MCP server", map[string]any{
				"server": name,
				"error":  err.Error(),
			})
			mcpClient.Close()
			continue
		}

		// Fetch tools
		toolsResp, err := mcpClient.ListTools(ctx, mcp.ListToolsRequest{})
		if err != nil {
			logger.ErrorCF("mcp", "Failed to list tools from MCP server", map[string]any{
				"server": name,
				"error":  err.Error(),
			})
			mcpClient.Close()
			continue
		}

		m.servers[name] = &MCPServerInstance{
			Name:   name,
			Client: mcpClient,
			Cleanup: func() {
				if mcpClient != nil {
					_ = mcpClient.Close() // Best effort close
				}
			},
			Tools: toolsResp.Tools,
		}

		logger.InfoCF("mcp", "Successfully connected to MCP server", map[string]any{
			"server":     name,
			"tool_count": len(toolsResp.Tools),
		})
	}

	return nil
}

// GetServers returns all active MCP server instances.
func (m *GlobalManager) GetServers() []*MCPServerInstance {
	m.mu.Lock()
	defer m.mu.Unlock()

	var servers []*MCPServerInstance
	for _, srv := range m.servers {
		servers = append(servers, srv)
	}
	return servers
}

// Shutdown gracefully shuts down all managed MCP servers.
func (m *GlobalManager) Shutdown() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.isClosed = true
	for name, srv := range m.servers {
		logger.DebugCF("mcp", "Shutting down MCP server", map[string]any{"server": name})
		srv.Cleanup()
	}
	// Clear map
	m.servers = make(map[string]*MCPServerInstance)
}
