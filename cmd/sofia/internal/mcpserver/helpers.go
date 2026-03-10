package mcpserver

import (
	"context"
	"fmt"

	"github.com/grasberg/sofia/cmd/sofia/internal"
	"github.com/grasberg/sofia/pkg/agent"
	"github.com/grasberg/sofia/pkg/bus"
	"github.com/grasberg/sofia/pkg/logger"
	"github.com/grasberg/sofia/pkg/mcp"
	"github.com/grasberg/sofia/pkg/providers"
)

func runMCPServer(transport, addr string, debug bool) error {
	if debug {
		logger.SetLevel(logger.DEBUG)
	}

	// When running as stdio MCP server, suppress all non-protocol output to stderr
	// since stdout is reserved for MCP JSON-RPC messages.
	if transport == "stdio" {
		logger.SetLevel(logger.ERROR)
	}

	cfg, err := internal.LoadConfig()
	if err != nil {
		return fmt.Errorf("error loading config: %w", err)
	}

	provider, _, err := providers.CreateProvider(cfg)
	if err != nil {
		return fmt.Errorf("error creating provider: %w", err)
	}

	msgBus := bus.NewMessageBus()
	agentLoop := agent.NewAgentLoop(cfg, msgBus, provider)

	// Start agent loop in background
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = agentLoop.Run(ctx) }() //nolint:errcheck

	sofiaServer := mcp.NewSofiaServer(agentLoop)

	switch transport {
	case "stdio":
		return sofiaServer.ServeStdio()
	case "sse":
		logger.InfoC("mcp-server", fmt.Sprintf("MCP SSE server starting on %s", addr))
		return sofiaServer.ServeSSE(addr)
	default:
		return fmt.Errorf("unknown transport: %s (use 'stdio' or 'sse')", transport)
	}
}
