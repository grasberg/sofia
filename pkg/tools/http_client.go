package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// HTTPClientTool provides structured HTTP requests for API testing and interaction.
type HTTPClientTool struct{}

func NewHTTPClientTool() *HTTPClientTool { return &HTTPClientTool{} }

func (t *HTTPClientTool) Name() string { return "http" }
func (t *HTTPClientTool) Description() string {
	return "Make HTTP requests to APIs. Supports GET, POST, PUT, PATCH, DELETE, HEAD methods with custom headers, body, and auth. More structured than web_fetch for API interaction."
}

func (t *HTTPClientTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"method": map[string]any{
				"type":        "string",
				"description": "HTTP method",
				"enum":        []string{"GET", "POST", "PUT", "PATCH", "DELETE", "HEAD"},
			},
			"url": map[string]any{
				"type":        "string",
				"description": "Request URL",
			},
			"headers": map[string]any{
				"type":        "object",
				"description": "Request headers as key-value pairs",
			},
			"body": map[string]any{
				"type":        "string",
				"description": "Request body (string or JSON)",
			},
			"timeout_seconds": map[string]any{
				"type":        "integer",
				"description": "Request timeout in seconds (default 30)",
			},
			"follow_redirects": map[string]any{
				"type":        "boolean",
				"description": "Follow redirects (default true)",
			},
		},
		"required": []string{"method", "url"},
	}
}

func (t *HTTPClientTool) Execute(ctx context.Context, args map[string]any) *ToolResult {
	method, _ := args["method"].(string)
	url, _ := args["url"].(string)

	if method == "" || url == "" {
		return ErrorResult("method and url are required")
	}

	// SSRF protection: block private/internal IPs
	if isPrivateURL(url) {
		return ErrorResult("requests to private/internal networks are blocked")
	}

	timeout := 30 * time.Second
	if raw, ok := args["timeout_seconds"]; ok {
		if n, ok := parsePositiveInt(raw); ok {
			timeout = time.Duration(n) * time.Second
		}
	}

	var bodyReader io.Reader
	if body, ok := args["body"].(string); ok && body != "" {
		bodyReader = strings.NewReader(body)
	}

	reqCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, method, url, bodyReader)
	if err != nil {
		return ErrorResult(fmt.Sprintf("failed to create request: %v", err))
	}

	// Set headers
	if headers, ok := args["headers"].(map[string]any); ok {
		for k, v := range headers {
			if s, ok := v.(string); ok {
				req.Header.Set(k, s)
			}
		}
	}

	// Default Content-Type for POST/PUT/PATCH with body
	if bodyReader != nil && req.Header.Get("Content-Type") == "" {
		req.Header.Set("Content-Type", "application/json")
	}

	client := &http.Client{Timeout: timeout}

	if raw, ok := args["follow_redirects"].(bool); ok && !raw {
		client.CheckRedirect = func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		}
	}

	resp, err := client.Do(req)
	if err != nil {
		return ErrorResult(fmt.Sprintf("request failed: %v", err))
	}
	defer resp.Body.Close()

	// Read body with size limit
	const maxBody = 100 * 1024 // 100KB
	bodyBytes, err := io.ReadAll(io.LimitReader(resp.Body, maxBody))
	if err != nil {
		return ErrorResult(fmt.Sprintf("failed to read response: %v", err))
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("HTTP %d %s\n", resp.StatusCode, resp.Status))

	// Include key response headers
	for _, h := range []string{"Content-Type", "Content-Length", "Location", "X-Request-Id"} {
		if v := resp.Header.Get(h); v != "" {
			sb.WriteString(fmt.Sprintf("%s: %s\n", h, v))
		}
	}
	sb.WriteByte('\n')

	// Try to pretty-print JSON
	body := string(bodyBytes)
	if strings.Contains(resp.Header.Get("Content-Type"), "json") {
		var prettyJSON bytes.Buffer
		if err := json.Indent(&prettyJSON, bodyBytes, "", "  "); err == nil {
			body = prettyJSON.String()
		}
	}

	// Truncate large responses
	if len(body) > 12000 {
		body = body[:12000] + fmt.Sprintf("\n... (truncated, %d more bytes)", len(body)-12000)
	}
	sb.WriteString(body)

	result := NewToolResult(sb.String())
	if resp.StatusCode >= 400 {
		result.IsError = true
	}
	return result
}

func isPrivateURL(url string) bool {
	lower := strings.ToLower(url)
	privatePatterns := []string{
		"://localhost", "://127.", "://10.", "://192.168.",
		"://172.16.", "://172.17.", "://172.18.", "://172.19.",
		"://172.20.", "://172.21.", "://172.22.", "://172.23.",
		"://172.24.", "://172.25.", "://172.26.", "://172.27.",
		"://172.28.", "://172.29.", "://172.30.", "://172.31.",
		"://[::1]", "://0.0.0.0", "://169.254.",
	}
	for _, p := range privatePatterns {
		if strings.Contains(lower, p) {
			return true
		}
	}
	return false
}
