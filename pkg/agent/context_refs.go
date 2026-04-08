package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

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
			body, err := fetchURL(ctx, ref)
			if err != nil {
				return fmt.Sprintf("[could not fetch %s: %v]", ref, err)
			}
			if len(body) > maxAtRefBytes {
				body = body[:maxAtRefBytes] + "\n... truncated"
			}
			return fmt.Sprintf("\n```\n%s\n```", body)
		}

		// File reference
		abs := ref
		if !filepath.IsAbs(ref) {
			abs = filepath.Join(workspacePath, ref)
		}
		abs = filepath.Clean(abs)
		// Security: reject paths that escape the workspace
		if workspacePath != "" && !strings.HasPrefix(abs, workspacePath) {
			return match
		}
		data, err := os.ReadFile(abs)
		if err != nil {
			return fmt.Sprintf("[could not read %s: %v]", ref, err)
		}
		body := string(data)
		if len(body) > maxAtRefBytes {
			body = body[:maxAtRefBytes] + "\n... truncated"
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
	// Parse ForUser for the extracted text body; fall back to ForLLM summary on error.
	if text, err := extractTextFromFetchResult(result.ForUser); err == nil {
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
