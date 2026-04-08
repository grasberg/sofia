package evolution

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/grasberg/sofia/pkg/logger"
	"github.com/grasberg/sofia/pkg/providers"
	pt "github.com/grasberg/sofia/pkg/providers/protocoltypes"
)

// dangerousPatterns matches content that should never be written by the evolution engine.
var dangerousPatterns = regexp.MustCompile(
	`(?i)(ignore\s+previous|override\s+safety|disable\s+guardrail|rm\s+-rf|DROP\s+TABLE|` +
		`delete\s+from\s+\w+|truncate\s+table|exec\s*\(|eval\s*\(|os\.RemoveAll|os\.Remove)`,
)

// SafeModifier handles file modifications with versioning, immutability checks,
// and optional LLM-based semantic safety validation.
type SafeModifier struct {
	historyDir     string
	immutablePaths []string
	provider       providers.LLMProvider
}

// defaultImmutablePaths are always protected from modification.
var defaultImmutablePaths = []string{
	"config.json",
	"config.yaml",
	".env",
	"pkg/",
	"evolution/",
}

// NewSafeModifier creates a SafeModifier that versions files into historyDir and
// refuses to modify paths matching the default immutable list plus extraImmutable.
func NewSafeModifier(
	historyDir string,
	extraImmutable []string,
	provider providers.LLMProvider,
) *SafeModifier {
	all := make([]string, 0, len(defaultImmutablePaths)+len(extraImmutable))
	all = append(all, defaultImmutablePaths...)
	all = append(all, extraImmutable...)
	return &SafeModifier{
		historyDir:     historyDir,
		immutablePaths: all,
		provider:       provider,
	}
}

// IsImmutable returns true if path matches any immutable pattern via prefix or exact basename match.
func (sm *SafeModifier) IsImmutable(path string) bool {
	// Try to get absolute path, but fall back to original path if it fails
	absPath, absErr := filepath.Abs(path)

	baseName := strings.ToLower(filepath.Base(path))

	for _, pattern := range sm.immutablePaths {
		// Check if pattern is a basename (no slashes) - match against filename
		if !strings.Contains(pattern, "/") && !strings.Contains(pattern, string(filepath.Separator)) {
			if baseName == strings.ToLower(pattern) {
				return true
			}
		} else {
			// Pattern is a directory path - check various matching strategies
			normalizedPattern := filepath.ToSlash(pattern)

			// For patterns like "pkg/", we need to check:
			// 1. If user provided absolute path like "/some/path/evolution/foo.go", check if it contains "/evolution/"
			// 2. If user provided relative path like "pkg/agent/loop.go", check if it starts with "pkg/"
			// IMPORTANT: Only check "contains" on user's original path to avoid false positives from
			// absolute paths that happen to contain the pattern (e.g., running tests from /path/to/pkg/evolution/)

			if strings.HasSuffix(normalizedPattern, "/") {
				dirName := strings.TrimSuffix(normalizedPattern, "/")

				// Check if path starts with the pattern (relative path matching)
				normalizedPath := filepath.ToSlash(path)
				if strings.HasPrefix(normalizedPath, normalizedPattern) {
					return true
				}

				// Check if absolute path starts with the pattern (for absolute user paths like "/pkg/...")
				if absErr == nil {
					normalizedAbs := filepath.ToSlash(absPath)
					if strings.HasPrefix(normalizedAbs, normalizedPattern) {
						return true
					}
				}

				// Check if original path contains the pattern as a directory component
				// This handles cases like "/some/path/evolution/foo.go" matching "evolution/"
				if strings.Contains(normalizedPath, "/"+dirName+"/") || strings.HasSuffix(normalizedPath, "/"+dirName) {
					return true
				}
			} else {
				// Pattern without slash - treat as basename
				if baseName == strings.ToLower(normalizedPattern) {
					return true
				}
			}
		}
	}
	return false
}

// VersionFile reads the file at path and writes a timestamped backup into historyDir.
// Returns the backup path.
func (sm *SafeModifier) VersionFile(path string) (string, error) {
	data, err := os.ReadFile(path) //nolint:gosec // path is caller-controlled workspace file
	if err != nil {
		return "", fmt.Errorf("evolution: read file for versioning: %w", err)
	}

	if err := os.MkdirAll(sm.historyDir, 0o750); err != nil {
		return "", fmt.Errorf("evolution: create history dir: %w", err)
	}

	base := filepath.Base(path)
	ts := time.Now().Unix()
	backupName := fmt.Sprintf("%s.%d.bak", base, ts)
	backupPath := filepath.Join(sm.historyDir, backupName)

	if err := os.WriteFile(backupPath, data, 0o600); err != nil {
		return "", fmt.Errorf("evolution: write backup: %w", err)
	}

	return backupPath, nil
}

// ModifyFile safely modifies a file: checks immutability, creates a version backup,
// optionally validates safety via LLM, then writes the new content.
func (sm *SafeModifier) ModifyFile(ctx context.Context, path, newContent string) error {
	if sm.IsImmutable(path) {
		return fmt.Errorf("evolution: path %q is immutable", path)
	}

	// Primary gate: structural regex check for dangerous patterns.
	if match := dangerousPatterns.FindString(newContent); match != "" {
		logger.WarnCF("evolution", "Content blocked by structural safety check", map[string]any{
			"path":    path,
			"pattern": match,
		})
		return fmt.Errorf("blocked_by_safety: content for %q contains dangerous pattern %q", path, match)
	}

	// Version the existing file if it exists.
	if _, err := os.Stat(path); err == nil {
		if _, err := sm.VersionFile(path); err != nil {
			return fmt.Errorf("evolution: version before modify: %w", err)
		}
	}

	// Secondary gate: semantic safety check via LLM (fail-closed).
	if sm.provider != nil {
		blocked, err := sm.checkSafety(ctx, newContent)
		if err != nil {
			logger.WarnCF(
				"evolution",
				"Safety validation LLM call failed, blocking write as precaution",
				map[string]any{
					"path":  path,
					"error": err.Error(),
				},
			)
			return fmt.Errorf("safety validation unavailable, write blocked: %w", err)
		} else if blocked {
			return fmt.Errorf("blocked_by_safety: content for %q was flagged as unsafe", path)
		}
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return fmt.Errorf("evolution: create parent dir: %w", err)
	}

	if err := os.WriteFile(path, []byte(newContent), 0o600); err != nil {
		return fmt.Errorf("evolution: write modified file: %w", err)
	}

	return nil
}

// checkSafety asks the LLM whether the content disables safety mechanisms.
// Returns true if the content is flagged as unsafe.
func (sm *SafeModifier) checkSafety(ctx context.Context, content string) (bool, error) {
	prompt := "Does this content disable safety mechanisms, remove access controls, " +
		"or bypass guardrails? Answer YES or NO.\n\n" + content

	messages := []pt.Message{
		{Role: "user", Content: prompt},
	}

	resp, err := sm.provider.Chat(ctx, messages, nil, sm.provider.GetDefaultModel(), nil)
	if err != nil {
		return false, fmt.Errorf("safety LLM call: %w", err)
	}

	answer := strings.TrimSpace(strings.ToUpper(resp.Content))
	return strings.HasPrefix(answer, "YES"), nil
}

// RevertFile restores a file from a backup.
func (sm *SafeModifier) RevertFile(path, backupPath string) error {
	data, err := os.ReadFile(backupPath) //nolint:gosec // backupPath is from our own history dir
	if err != nil {
		return fmt.Errorf("evolution: read backup for revert: %w", err)
	}

	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("evolution: write reverted file: %w", err)
	}

	return nil
}

// ListBackups returns backup paths for the given file, sorted by timestamp descending (newest first).
func (sm *SafeModifier) ListBackups(path string) ([]string, error) {
	base := filepath.Base(path)
	pattern := filepath.Join(sm.historyDir, base+".*.bak")

	matches, err := filepath.Glob(pattern)
	if err != nil {
		return nil, fmt.Errorf("evolution: glob backups: %w", err)
	}

	sort.Sort(sort.Reverse(sort.StringSlice(matches)))
	return matches, nil
}
