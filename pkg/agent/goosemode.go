package agent

// GooseMode defines the agent's tool permission tier.
// Inspired by Goose's permission model, it controls how aggressively
// tools are auto-approved vs requiring human confirmation.
type GooseMode string

const (
	// GooseModeAuto approves all tool calls automatically.
	GooseModeAuto GooseMode = "auto"

	// GooseModeApprove requires human approval for every tool call.
	GooseModeApprove GooseMode = "approve"

	// GooseModeSmartApprove auto-approves read-only tools, asks for write/destructive tools.
	// Uses tool annotations (readOnlyHint, destructiveHint) and a built-in classification.
	GooseModeSmartApprove GooseMode = "smart_approve"

	// GooseModeChat disables all tool calls. The agent can only converse.
	GooseModeChat GooseMode = "chat"
)

// readOnlyTools are tools that only read data and never modify state.
// Used by SmartApprove mode to auto-approve safe operations.
var readOnlyTools = map[string]bool{
	"read_file":      true,
	"list_dir":       true,
	"glob":           true,
	"grep":           true,
	"search_history": true,
	"web_search":     true,
	"web_fetch":      true,
	"dns":            true,
	"image_analyze":  true,
	"get_tool_stats": true,
	"analyze":        true,
	"todo":           true, // reading todos is safe
	"task":           true, // session tasks are ephemeral
	"jq":             true, // JSON processing is read-only
}

// destructiveTools are tools that can cause irreversible changes.
// SmartApprove always asks for confirmation on these.
var destructiveTools = map[string]bool{
	"exec":        true,
	"shell":       true,
	"write_file":  true,
	"edit_file":   true,
	"append_file": true,
	"git":         true,
	"docker":      true,
	"kubectl":     true,
	"terraform":   true,
	"database":    true,
	"http":        true,
}

// ToolAnnotation holds MCP-compatible tool hints for permission classification.
type ToolAnnotation struct {
	ReadOnlyHint    bool `json:"readOnlyHint,omitempty"`
	DestructiveHint bool `json:"destructiveHint,omitempty"`
	Idempotent      bool `json:"idempotent,omitempty"`
	OpenWorld       bool `json:"openWorld,omitempty"`
}

// toolAnnotations stores per-tool annotations set by MCP servers or config.
var toolAnnotations = make(map[string]*ToolAnnotation)

// SetToolAnnotation registers an annotation for a tool.
func SetToolAnnotation(toolName string, ann *ToolAnnotation) {
	toolAnnotations[toolName] = ann
}

// ShouldApprove determines whether a tool call requires human approval
// based on the current GooseMode.
func ShouldApprove(mode GooseMode, toolName string) bool {
	switch mode {
	case GooseModeAuto:
		return false // never ask
	case GooseModeApprove:
		return true // always ask
	case GooseModeChat:
		return true // block all tools (they'll be denied)
	case GooseModeSmartApprove:
		return smartApproveCheck(toolName)
	default:
		return false
	}
}

// ShouldBlockTool returns true if the mode completely prevents tool execution.
func ShouldBlockTool(mode GooseMode) bool {
	return mode == GooseModeChat
}

func smartApproveCheck(toolName string) bool {
	// Check MCP annotations first
	if ann, ok := toolAnnotations[toolName]; ok {
		if ann.ReadOnlyHint {
			return false // auto-approve read-only
		}
		if ann.DestructiveHint {
			return true // always ask for destructive
		}
	}

	// Check built-in classification
	if readOnlyTools[toolName] {
		return false
	}
	if destructiveTools[toolName] {
		return true
	}

	// Default: ask for unknown tools
	return true
}
