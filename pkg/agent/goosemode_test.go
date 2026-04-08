package agent

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestShouldApprove_AutoMode(t *testing.T) {
	assert.False(t, ShouldApprove(GooseModeAuto, "exec"))
	assert.False(t, ShouldApprove(GooseModeAuto, "read_file"))
}

func TestShouldApprove_ApproveMode(t *testing.T) {
	assert.True(t, ShouldApprove(GooseModeApprove, "read_file"))
	assert.True(t, ShouldApprove(GooseModeApprove, "exec"))
}

func TestShouldApprove_ChatMode(t *testing.T) {
	assert.True(t, ShouldApprove(GooseModeChat, "read_file"))
	assert.True(t, ShouldBlockTool(GooseModeChat))
}

func TestShouldApprove_SmartApprove(t *testing.T) {
	// Read-only tools should be auto-approved
	assert.False(t, ShouldApprove(GooseModeSmartApprove, "read_file"))
	assert.False(t, ShouldApprove(GooseModeSmartApprove, "glob"))
	assert.False(t, ShouldApprove(GooseModeSmartApprove, "grep"))
	assert.False(t, ShouldApprove(GooseModeSmartApprove, "web_search"))

	// Destructive tools should require approval
	assert.True(t, ShouldApprove(GooseModeSmartApprove, "exec"))
	assert.True(t, ShouldApprove(GooseModeSmartApprove, "write_file"))
	assert.True(t, ShouldApprove(GooseModeSmartApprove, "docker"))

	// Unknown tools should require approval
	assert.True(t, ShouldApprove(GooseModeSmartApprove, "unknown_tool"))
}

func TestShouldApprove_ToolAnnotations(t *testing.T) {
	// Set a custom annotation
	SetToolAnnotation("my_custom_tool", &ToolAnnotation{ReadOnlyHint: true})
	assert.False(t, ShouldApprove(GooseModeSmartApprove, "my_custom_tool"))

	SetToolAnnotation("my_danger_tool", &ToolAnnotation{DestructiveHint: true})
	assert.True(t, ShouldApprove(GooseModeSmartApprove, "my_danger_tool"))

	// Cleanup
	delete(toolAnnotations, "my_custom_tool")
	delete(toolAnnotations, "my_danger_tool")
}
