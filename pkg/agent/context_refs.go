package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/grasberg/sofia/pkg/tools"
)

var atRefRe = regexp.MustCompile(`@(\./[^\s]+|/[^\s]+|https?://[^\s]+)`)

const (
	maxAtRefs     = 5
	maxAtRefBytes = 50 * 1024 // 50 KB per reference
)

// enrichMessageContent replaces @/path and @https://url tokens in content
// with the referenced file or URL contents, inline.
// fetchURL is an injectable function for testability.
func enrichMessageContent(
	ctx context.Context,
	content string,
	workspacePath string,
	fetchURL func(ctx context.Context, url string) (string, error),
) string {
	matches := atRefRe.FindAllString(content, -1)
	if len(matches) == 0 {
		return content
	}

	count := 0
	return atRefRe.ReplaceAllStringFunc(content, func(match string) string {
		if count >= maxAtRefs {
			return match
		}
		submatches := atRefRe.FindStringSubmatch(match)
		if len(submatches) < 2 {
			return match
		}
		ref := submatches[1]
		count++

		if strings.HasPrefix(ref, "http://") || strings.HasPrefix(ref, "https://") {
			// NOTE: URL content is not re-scanned by guardrails after injection.
			body, err := fetchURL(ctx, ref)
			if err != nil {
				return fmt.Sprintf("[could not fetch %s: %v]", ref, err)
			}
			if len(body) > maxAtRefBytes {
				body = truncateBytes(body, maxAtRefBytes) + "\n... truncated"
			}
			return fmt.Sprintf("\n```\n%s\n```", body)
		}

		// File reference
		abs := ref
		if !filepath.IsAbs(ref) {
			abs = filepath.Join(workspacePath, ref)
		}
		abs = filepath.Clean(abs)
		// Security: reject paths that escape the workspace.
		// Use a trailing separator to prevent sibling-directory prefix collisions
		// (e.g. /tmp/workspace-evil must not pass a HasPrefix check on /tmp/workspace).
		if workspacePath != "" {
			wsp := workspacePath
			if !strings.HasSuffix(wsp, string(filepath.Separator)) {
				wsp += string(filepath.Separator)
			}
			if abs != workspacePath && !strings.HasPrefix(abs, wsp) {
				return match // escapes workspace
			}
		}
		data, err := os.ReadFile(abs)
		if err != nil {
			return fmt.Sprintf("[could not read %s: %v]", ref, err)
		}
		body := string(data)
		if len(body) > maxAtRefBytes {
			body = truncateBytes(body, maxAtRefBytes) + "\n... truncated"
		}
		return fmt.Sprintf("\n```\n%s\n```", body)
	})
}

// httpFetchForContext fetches a URL and returns the extracted text content.
// It reuses WebFetchTool's HTML-to-text extraction logic.
func httpFetchForContext(ctx context.Context, url string) (string, error) {
	fetcher := tools.NewWebFetchTool(maxAtRefBytes)
	result := fetcher.Execute(ctx, map[string]any{"url": url})
	if result == nil {
		return "", fmt.Errorf("no result from fetch")
	}
	if result.IsError {
		return "", fmt.Errorf("%s", result.ForLLM)
	}
	// ForUser contains the JSON result; ForLLM contains the summary.
	// Parse ForUser for the extracted text body; fall back to ForLLM summary on error or empty text.
	if text, err := extractTextFromFetchResult(result.ForUser); err == nil && text != "" {
		return text, nil
	}
	return result.ForLLM, nil
}

// extractTextFromFetchResult parses the JSON result from WebFetchTool and returns the text field.
func extractTextFromFetchResult(jsonStr string) (string, error) {
	var result struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal([]byte(jsonStr), &result); err != nil {
		return "", fmt.Errorf("failed to parse fetch result JSON: %w", err)
	}
	return result.Text, nil
}

// truncateBytes returns the string truncated to at most maxBytes, without
// cutting a multi-byte UTF-8 rune in half.
func truncateBytes(s string, maxBytes int) string {
	if len(s) <= maxBytes {
		return s
	}
	// Walk back from maxBytes to find a valid rune boundary
	for i := maxBytes; i > 0; i-- {
		if utf8.ValidString(s[:i]) {
			return s[:i]
		}
	}
	return s[:maxBytes] // fallback: raw byte slice
}
