package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"

	"github.com/grasberg/sofia/pkg/logger"
)

// MCPToolAdapter wraps an external MCP tool to conform to the Sofia Tool interface.
type MCPToolAdapter struct {
	serverName string
	mcpTool    mcp.Tool
	client     client.MCPClient
}

// NewMCPToolAdapter creates a new bridge for an MCP tool.
func NewMCPToolAdapter(serverName string, tool mcp.Tool, client client.MCPClient) *MCPToolAdapter {
	return &MCPToolAdapter{
		serverName: serverName,
		mcpTool:    tool,
		client:     client,
	}
}

// Name returns the namespaced tool name (e.g. "serverName_toolName" to prevent collisions).
// If the server configured name is strictly unique, we can prefix it.
// To avoid messy names, we'll use `mcp_servername__toolname`.
func (t *MCPToolAdapter) Name() string {
	// Replacing dashes with underscores to keep LLM happy, and namespacing it.
	safeServer := strings.ReplaceAll(t.serverName, "-", "_")
	safeServer = strings.ReplaceAll(safeServer, " ", "_")
	return fmt.Sprintf("mcp_%s__%s", safeServer, t.mcpTool.Name)
}

func (t *MCPToolAdapter) Description() string {
	desc := t.mcpTool.Description
	if desc == "" {
		desc = fmt.Sprintf("MCP Tool %s from %s", t.mcpTool.Name, t.serverName)
	}
	// Add attribution so the LLM knows it is external.
	return fmt.Sprintf("[MCP: %s] %s", t.serverName, desc)
}

func (t *MCPToolAdapter) Parameters() map[string]any {
	// mcp.Tool.InputSchema is already the JSON schema.
	// We might need to marshal-unmarshal to type `map[string]any` if it's strongly typed,
	// but in mcp-go it is usually a struct.

	// Fast conversion via JSON:
	var schema map[string]any
	b, err := json.Marshal(t.mcpTool.InputSchema)
	if err == nil {
		_ = json.Unmarshal(b, &schema)
	} else {
		logger.ErrorCF("mcp_tool", "Failed to parse MCP tool input schema", map[string]any{
			"tool":  t.mcpTool.Name,
			"error": err.Error(),
		})
		schema = map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		}
	}

	// Convert top level `properties` field if it exists to match expected Sofia Tool Parameters().
	// Normally Sofia parameters are the top-level schema properties map.
	if props, ok := schema["properties"]; ok && props != nil {
		if _, ok2 := props.(map[string]any); ok2 {
			// If we need missing fields, we add them. But `properties` alone usually
			// doesn't cut it. However Sofia's ToolToSchema wraps Parameters() inside
			// `{"type": "object", "properties": <Parameters()>}`
			// Wait, let's look at `ToolToSchema` in base.go:
			// "parameters": tool.Parameters() directly.
			// Which means `tool.Parameters()` must return the FULL schema:
			// `{"type": "object", "properties": {...}, "required": [...]}`.
			// mcpTool.InputSchema IS the full JSON schema.
			return schema
		}
	}

	return schema
}

func (t *MCPToolAdapter) Execute(ctx context.Context, args map[string]any) *ToolResult {
	req := mcp.CallToolRequest{}
	// mcp-go CallToolRequest has Params.Name and Params.Arguments
	req.Params.Name = t.mcpTool.Name
	req.Params.Arguments = args

	resp, err := t.client.CallTool(ctx, req)
	if err != nil {
		logger.ErrorCF("mcp_tool", "MCP tool execution failed", map[string]any{
			"server": t.serverName,
			"tool":   t.mcpTool.Name,
			"error":  err.Error(),
		})
		return ErrorResult(fmt.Sprintf("Failed to execute MCP tool %q: %v", t.mcpTool.Name, err)).WithError(err)
	}

	if resp.IsError {
		// Try to extract error message from content
		var errMsg string
		for _, content := range resp.Content {
			// Need to check specific content types. In mcp-go it's often interface{}
			// representing TextContent, ImageContent etc.
			if textContent, ok := content.(mcp.TextContent); ok {
				errMsg += textContent.Text + "\n"
			} else {
				// fallback serialization
				b, _ := json.Marshal(content)
				errMsg += string(b) + "\n"
			}
		}
		if errMsg == "" {
			errMsg = "Unknown MCP tool error"
		}
		return ErrorResult(strings.TrimSpace(errMsg))
	}

	// Success parsing
	var out string
	images := []string{}

	for _, content := range resp.Content {
		switch c := content.(type) {
		case mcp.TextContent:
			out += c.Text + "\n"
		case mcp.ImageContent:
			out += fmt.Sprintf("[Image data: %s]\n", c.MIMEType)
			// Can pass to LLM if Sofia supports it (ToolResult has Images field)
			images = append(images, fmt.Sprintf("data:%s;base64,%s", c.MIMEType, c.Data))
		case mcp.EmbeddedResource:
			resBytes, _ := json.Marshal(c.Resource)
			out += fmt.Sprintf("[Embedded Resource: %s]\n", string(resBytes))
		default:
			b, _ := json.Marshal(c)
			out += string(b) + "\n"
		}
	}

	res := NewToolResult(strings.TrimSpace(out))
	if len(images) > 0 {
		res.Images = images
	}
	return res
}
