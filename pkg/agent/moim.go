package agent

import (
	"os"
	"path/filepath"
	"strings"
)

// MOIM (Messages of Immediate Memos) provides a simple mechanism to inject
// persistent user context into every agent turn. Content is loaded from:
// 1. SOFIA_MOIM_TEXT environment variable (inline text)
// 2. SOFIA_MOIM_FILE environment variable (path to a file)
// 3. workspace/.moim file (project-level persistent instructions)
// 4. ~/.sofia/.moim file (global persistent instructions)
//
// Inspired by Goose's "Top of Mind" (TOM) feature.

// LoadMOIM loads all MOIM (Messages of Immediate Memos) content and returns
// a combined string for injection into the agent's context.
func LoadMOIM(workspace string) string {
	var parts []string

	// 1. Environment variable: inline text
	if text := os.Getenv("SOFIA_MOIM_TEXT"); text != "" {
		parts = append(parts, text)
	}

	// 2. Environment variable: file path
	if filePath := os.Getenv("SOFIA_MOIM_FILE"); filePath != "" {
		if content, err := os.ReadFile(filePath); err == nil {
			trimmed := strings.TrimSpace(string(content))
			if trimmed != "" {
				parts = append(parts, trimmed)
			}
		}
	}

	// 3. Workspace .moim file
	if workspace != "" {
		workspaceMoim := filepath.Join(workspace, ".moim")
		if content, err := os.ReadFile(workspaceMoim); err == nil {
			trimmed := strings.TrimSpace(string(content))
			if trimmed != "" {
				parts = append(parts, trimmed)
			}
		}
	}

	// 4. Global .moim file
	home, err := os.UserHomeDir()
	if err == nil {
		globalMoim := filepath.Join(home, ".sofia", ".moim")
		if content, err := os.ReadFile(globalMoim); err == nil {
			trimmed := strings.TrimSpace(string(content))
			if trimmed != "" {
				parts = append(parts, trimmed)
			}
		}
	}

	if len(parts) == 0 {
		return ""
	}

	return strings.Join(parts, "\n\n")
}
