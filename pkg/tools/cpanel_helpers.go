package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// ── UAPI helpers ─────────────────────────────────────────────────────

func (t *CpanelTool) baseURL() string {
	return fmt.Sprintf("https://%s:%d", t.host, t.port)
}

func (t *CpanelTool) uapiURL(module, function string) string {
	return fmt.Sprintf("%s/execute/%s/%s", t.baseURL(), module, function)
}

func (t *CpanelTool) doGet(ctx context.Context, module, function string, params url.Values) (map[string]any, error) {
	reqURL := t.uapiURL(module, function)
	if len(params) > 0 {
		reqURL += "?" + params.Encode()
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", fmt.Sprintf("cpanel %s:%s", t.username, t.apiToken))

	resp, err := t.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1024*1024))
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, truncateStr(string(body), 300))
	}

	var result map[string]any
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	return result, nil
}

func (t *CpanelTool) doPost(ctx context.Context, module, function string, params url.Values) (map[string]any, error) {
	reqURL := t.uapiURL(module, function)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, reqURL, strings.NewReader(params.Encode()))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", fmt.Sprintf("cpanel %s:%s", t.username, t.apiToken))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := t.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1024*1024))
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, truncateStr(string(body), 300))
	}

	var result map[string]any
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	return result, nil
}

func uapiOK(result map[string]any) (any, error) {
	status, _ := result["status"].(float64)
	if status != 1 {
		errs, _ := result["errors"].([]any)
		if len(errs) > 0 {
			var msgs []string
			for _, e := range errs {
				if s, ok := e.(string); ok {
					msgs = append(msgs, s)
				}
			}
			return nil, fmt.Errorf("%s", strings.Join(msgs, "; "))
		}
		return nil, fmt.Errorf("cPanel API error (status %v)", result["status"])
	}
	return result["data"], nil
}

func getStr(args map[string]any, key string) string {
	s, _ := args[key].(string)
	return strings.TrimSpace(s)
}

func validateRemotePath(path string) error {
	if strings.Contains(path, "..") {
		return fmt.Errorf("path must not contain '..'")
	}
	if !strings.HasPrefix(path, "/") {
		return fmt.Errorf("path must be absolute (start with /)")
	}
	return nil
}
