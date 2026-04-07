package tools

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/grasberg/sofia/pkg/logger"
)

// SelfModifyTool allows the agent to modify its own source code or configuration.
// It enforces confirmation and prevents modifying golden configs.
type SelfModifyTool struct {
	workspace string // The root directory or workspace allowing modification
}

// NewSelfModifyTool creates a new SelfModifyTool.
func NewSelfModifyTool(workspace string) *SelfModifyTool {
	return &SelfModifyTool{
		workspace: workspace,
	}
}

func (t *SelfModifyTool) Name() string {
	return "self_modify"
}

func (t *SelfModifyTool) Description() string {
	return "Safely modify the agent's own source code or configuration. Requires user confirmation before execution."
}

func (t *SelfModifyTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{
				"type":        "string",
				"description": "Absolute or relative path to the file to modify.",
			},
			"content": map[string]any{
				"type":        "string",
				"description": "The complete new content for the file.",
			},
			"confirm_hash": map[string]any{
				"type":        "string",
				"description": "Hash required to confirm the modification. Do not provide this on the first call.",
			},
		},
		"required": []string{"path", "content"},
	}
}

// Execute performs the side-effect if confirmation hash matches and the path is valid.
func (t *SelfModifyTool) Execute(ctx context.Context, args map[string]any) *ToolResult {
	pathStr, ok := args["path"].(string)
	if !ok || pathStr == "" {
		return ErrorResult("Missing or invalid 'path' parameter")
	}

	contentStr, ok := args["content"].(string)
	if !ok {
		return ErrorResult("Missing or invalid 'content' parameter")
	}

	// Resolve absolute path
	absPath := pathStr
	if !filepath.IsAbs(absPath) {
		absPath = filepath.Join(t.workspace, pathStr)
	}

	// Guardrail: Prevent modifying golden configs (check this FIRST before workspace boundary)
	baseName := strings.ToLower(filepath.Base(absPath))
	if baseName == "config.json" || baseName == "config.yaml" || baseName == ".env" {
		logger.WarnCF("self_modify", "Blocked attempt to modify golden config", map[string]any{"path": absPath})
		return ErrorResult(fmt.Sprintf("Guardrail blocked modification of golden config file: %s", baseName))
	}

	// Guardrail: Enforce workspace boundary - prevent writes outside workspace
	if t.workspace != "" {
		cleanWorkspace := filepath.Clean(t.workspace)
		cleanPath := filepath.Clean(absPath)
		if !strings.HasPrefix(cleanPath, cleanWorkspace) {
			logger.WarnCF("self_modify", "Blocked attempt to write outside workspace", map[string]any{
				"path":      absPath,
				"workspace": t.workspace,
			})
			return ErrorResult(fmt.Sprintf("Guardrail blocked: path '%s' is outside workspace '%s'", absPath, t.workspace))
		}
	}

	// Verify confirmation hash
	expectedHash := fmt.Sprintf("%x", sha256.Sum256([]byte(absPath+contentStr)))[:16]
	providedHash, _ := args["confirm_hash"].(string)

	if providedHash != expectedHash {
		prompt := fmt.Sprintf("Allow self-modification of file `%s` (size: %d bytes)?", absPath, len(contentStr))
		res := ConfirmationResult(prompt)
		res.ForLLM = fmt.Sprintf("Modification requires confirmation.\n"+
			"To proceed, call this tool again with identical arguments PLUS "+
			"the parameter 'confirm_hash': %q", expectedHash)
		return res
	}

	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(absPath), 0o755); err != nil {
		return ErrorResult(fmt.Sprintf("Failed to create directory: %v", err)).WithError(err)
	}

	// Write the file
	if err := os.WriteFile(absPath, []byte(contentStr), 0o644); err != nil {
		return ErrorResult(fmt.Sprintf("Failed to write file: %v", err)).WithError(err)
	}

	// Audit the modification
	auditLogPath := filepath.Join(t.workspace, "self_modifications.log")
	auditEntry := fmt.Sprintf("[%s] Modified: %s\n", time.Now().Format(time.RFC3339), absPath)

	f, err := os.OpenFile(auditLogPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err == nil {
		f.WriteString(auditEntry)
		f.Close()
	} else {
		logger.WarnCF("self_modify", "Failed to write to audit log", map[string]any{"error": err.Error()})
	}

	logger.InfoCF("self_modify", "Successfully modified file", map[string]any{"path": absPath})

	return UserResult(fmt.Sprintf("Successfully modified %s and logged to audit trail.", absPath))
}
