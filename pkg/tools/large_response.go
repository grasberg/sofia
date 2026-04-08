package tools

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// LargeResponseThreshold is the character count above which tool output is saved to a temp file.
const LargeResponseThreshold = 200_000

// HandleLargeResponse checks if a tool result exceeds the threshold and, if so,
// saves the full content to a temp file and replaces ForLLM with a reference.
// This prevents context pollution from very large tool outputs.
func HandleLargeResponse(result *ToolResult, toolName string) *ToolResult {
	if result == nil || result.IsError || result.Async || len(result.ForLLM) <= LargeResponseThreshold {
		return result
	}

	// Save full content to temp file
	tmpDir := os.TempDir()
	fileName := fmt.Sprintf("sofia-tool-output-%s-%d.txt", toolName, time.Now().UnixNano())
	tmpPath := filepath.Join(tmpDir, fileName)

	if err := os.WriteFile(tmpPath, []byte(result.ForLLM), 0o644); err != nil {
		// If we can't write the temp file, truncate in place
		truncated := result.ForLLM[:LargeResponseThreshold]
		result.ForLLM = truncated + fmt.Sprintf(
			"\n\n[OUTPUT TRUNCATED: %d chars total. Full output could not be saved to file: %v]",
			len(result.ForLLM), err)
		return result
	}

	originalLen := len(result.ForLLM)

	// Keep a preview (first 2000 + last 1000 chars) for LLM context
	preview := result.ForLLM[:2000]
	if originalLen > 3000 {
		preview += "\n\n... [middle omitted] ...\n\n" + result.ForLLM[originalLen-1000:]
	}

	result.ForLLM = fmt.Sprintf(
		"%s\n\n[LARGE OUTPUT: %d chars total, saved to %s. Use read_file to access the full content if needed.]",
		preview, originalLen, tmpPath)

	return result
}
