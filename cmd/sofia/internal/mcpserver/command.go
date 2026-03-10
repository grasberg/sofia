package mcpserver

import (
	"github.com/spf13/cobra"
)

// NewMCPServerCommand creates the `sofia mcp-server` command.
func NewMCPServerCommand() *cobra.Command {
	var (
		transport string
		addr      string
		debug     bool
	)

	cmd := &cobra.Command{
		Use:   "mcp-server",
		Short: "Start Sofia as an MCP server",
		Long: `Expose Sofia as a Model Context Protocol (MCP) server.
Other AI agents and tools can connect to Sofia and use its capabilities
via the standard MCP protocol.

Supports two transport modes:
  stdio - Communicate via stdin/stdout (default, for subprocess-based clients)
  sse   - Communicate via HTTP Server-Sent Events (for network clients)`,
		Args: cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			return runMCPServer(transport, addr, debug)
		},
	}

	cmd.Flags().StringVarP(&transport, "transport", "t", "stdio", "Transport mode: stdio or sse")
	cmd.Flags().StringVarP(&addr, "addr", "a", ":9090", "Listen address for SSE transport")
	cmd.Flags().BoolVarP(&debug, "debug", "d", false, "Enable debug logging")

	return cmd
}
