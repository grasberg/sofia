package agent

import (
	"strings"

	"github.com/grasberg/sofia/pkg/guardrails"
	"github.com/grasberg/sofia/pkg/logger"
	"github.com/grasberg/sofia/pkg/providers"
)

// applyOutputFilter checks a string against OutputFiltering redact patterns,
// optionally blocking or redacting the content, and logging audits.
func (al *AgentLoop) applyOutputFilter(agentComp, source, content string) string {
	if !al.cfg.Guardrails.OutputFiltering.Enabled || len(al.cfg.Guardrails.OutputFiltering.RedactPatterns) == 0 {
		return content
	}

	filteredContent := content
	for _, pattern := range al.cfg.Guardrails.OutputFiltering.RedactPatterns {
		re := getCachedRegex(pattern)
		if re == nil {
			continue
		}

		if re.MatchString(filteredContent) {
			if al.cfg.Guardrails.OutputFiltering.Action == "block" {
				logger.WarnCF(agentComp, "Guardrail blocked output", map[string]any{
					"source":  source,
					"pattern": pattern,
				})
				logger.Audit("Output Blocked", map[string]any{
					"source":  source,
					"pattern": pattern,
				})
				return "[OUTPUT BLOCKED BY FILTER]"
			}

			// Redact action
			filteredContent = re.ReplaceAllString(filteredContent, "[REDACTED]")
			logger.WarnCF(agentComp, "Guardrail redacted output", map[string]any{
				"source":  source,
				"pattern": pattern,
			})
			logger.Audit("Output Redacted", map[string]any{
				"source":  source,
				"pattern": pattern,
			})
		}
	}

	scrubbed, secretTypes := guardrails.ScrubSecrets(filteredContent)
	if len(secretTypes) > 0 {
		logger.WarnCF(agentComp, "Guardrail scrubbed secrets from output", map[string]any{
			"source":       source,
			"secret_types": secretTypes,
		})
		logger.Audit("Secrets Scrubbed", map[string]any{
			"source":       source,
			"secret_types": secretTypes,
		})
		filteredContent = scrubbed
	}

	return filteredContent
}

// safeToParallelize checks whether a batch of tool calls can be safely
// executed in parallel. If any two calls reference overlapping file paths
// (via "path", "file", or "file_path" arguments), they must run sequentially.
func safeToParallelize(calls []providers.ToolCall) bool {
	// File-mutating tools that should be checked for path overlap
	writingTools := map[string]bool{
		"write_file": true, "edit_file": true, "append_file": true,
		"shell": true, "exec": true,
	}

	var paths []string
	hasWriter := false
	for _, tc := range calls {
		p := extractFilePath(tc.Arguments)
		if p != "" {
			paths = append(paths, p)
			if writingTools[tc.Name] {
				hasWriter = true
			}
		}
	}

	// If no writing tools involved, parallel is safe
	if !hasWriter || len(paths) < 2 {
		return true
	}

	// Check for any overlapping paths
	for i := 0; i < len(paths); i++ {
		for j := i + 1; j < len(paths); j++ {
			if pathsOverlap(paths[i], paths[j]) {
				return false
			}
		}
	}
	return true
}

func extractFilePath(args map[string]any) string {
	for _, key := range []string{"path", "file", "file_path", "filename"} {
		if v, ok := args[key].(string); ok && v != "" {
			return v
		}
	}
	return ""
}

func pathsOverlap(a, b string) bool {
	if a == b {
		return true
	}
	// Check if one is a parent directory of the other
	if strings.HasPrefix(a, b+"/") || strings.HasPrefix(b, a+"/") {
		return true
	}
	return false
}
