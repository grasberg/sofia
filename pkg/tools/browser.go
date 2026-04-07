package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	"github.com/playwright-community/playwright-go"
)

// BrowseToolOptions configures the WebBrowseTool.
type BrowseToolOptions struct {
	Headless       bool
	TimeoutSeconds int
	BrowserType    string // "chromium", "firefox", "webkit"
	ScreenshotDir  string // directory for screenshots; defaults to workspace/screenshots
	Workspace      string
}

// WebBrowseTool provides autonomous browser control via Playwright.
// It opens a browser, navigates to a URL, and executes a sequence of
// actions (click, fill, screenshot, get_text, etc.) returning a step-by-step log.
type WebBrowseTool struct {
	opts BrowseToolOptions
}

// NewWebBrowseTool creates a WebBrowseTool with the given options.
// Sensible defaults are applied for any zero-value fields.
func NewWebBrowseTool(opts BrowseToolOptions) *WebBrowseTool {
	if opts.TimeoutSeconds <= 0 {
		opts.TimeoutSeconds = 30
	}
	if opts.BrowserType == "" {
		opts.BrowserType = "chromium"
	}
	if opts.ScreenshotDir == "" && opts.Workspace != "" {
		opts.ScreenshotDir = filepath.Join(opts.Workspace, "screenshots")
	}
	return &WebBrowseTool{opts: opts}
}

func (t *WebBrowseTool) Name() string { return "web_browse" }

func (t *WebBrowseTool) Description() string {
	return "Autonomously browse a website using a real browser (Playwright/Chromium). " +
		"Can navigate pages, click elements, fill forms, take screenshots, extract text/HTML, " +
		"run JavaScript, and interact with dynamic content. Use this for sites that require " +
		"JavaScript, login flows, or multi-step interactions that web_fetch cannot handle."
}

func (t *WebBrowseTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"url": map[string]any{
				"type":        "string",
				"description": "Starting URL to navigate to",
			},
			"actions": map[string]any{
				"type":        "array",
				"description": "Sequence of browser actions to execute after loading the URL",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"type": map[string]any{
							"type": "string",
							"enum": []string{
								"navigate", "click", "fill", "select",
								"wait", "wait_for", "screenshot",
								"get_text", "get_html", "scroll",
								"hover", "press", "evaluate",
							},
							"description": "Action type to perform",
						},
						"selector": map[string]any{
							"type":        "string",
							"description": "CSS selector or XPath (prefix '//' for XPath) targeting the element",
						},
						"value": map[string]any{
							"type":        "string",
							"description": "Value for fill or select actions",
						},
						"url": map[string]any{
							"type":        "string",
							"description": "URL for navigate action",
						},
						"milliseconds": map[string]any{
							"type":        "integer",
							"description": "Duration for wait action (milliseconds)",
						},
						"key": map[string]any{
							"type":        "string",
							"description": "Key name for press action (e.g. 'Enter', 'Tab', 'Escape')",
						},
						"script": map[string]any{
							"type":        "string",
							"description": "JavaScript expression to evaluate (returns serialized result)",
						},
						"name": map[string]any{
							"type":        "string",
							"description": "Optional filename (without extension) for screenshot action",
						},
					},
					"required": []string{"type"},
				},
			},
			"headless": map[string]any{
				"type":        "boolean",
				"description": "Run browser headlessly (default: true). Set false to watch the browser for debugging.",
			},
		},
		"required": []string{"url"},
	}
}

// validateBrowseURL checks that a URL is safe to browse: only http/https schemes
// are allowed, and the hostname must not resolve to a private/internal IP.
func validateBrowseURL(rawURL string) error {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}

	scheme := strings.ToLower(parsed.Scheme)
	switch scheme {
	case "http", "https":
		// allowed
	case "file", "javascript", "data":
		return fmt.Errorf("scheme %q is not allowed", scheme)
	default:
		return fmt.Errorf("only http/https URLs are allowed, got %q", scheme)
	}

	if parsed.Host == "" {
		return fmt.Errorf("missing host in URL")
	}

	// SSRF protection: reject private/internal IPs.
	if err := checkHostNotPrivate(parsed.Host); err != nil {
		return err
	}

	return nil
}

func (t *WebBrowseTool) Execute(ctx context.Context, args map[string]any) *ToolResult {
	startURL, ok := args["url"].(string)
	if !ok || startURL == "" {
		return ErrorResult("url is required")
	}

	// Validate the starting URL for safe schemes and non-private hosts.
	if err := validateBrowseURL(startURL); err != nil {
		return ErrorResult(fmt.Sprintf("URL blocked: %v", err))
	}

	// Determine headless mode; args can override the tool default.
	headless := t.opts.Headless
	if h, ok := args["headless"].(bool); ok {
		headless = h
	}

	// Parse actions list (optional — navigating to url alone is valid).
	var actions []browseAction
	if raw, ok := args["actions"].([]any); ok {
		for i, item := range raw {
			m, ok := item.(map[string]any)
			if !ok {
				return ErrorResult(fmt.Sprintf("action[%d] is not an object", i))
			}
			a, err := parseAction(m)
			if err != nil {
				return ErrorResult(fmt.Sprintf("action[%d]: %v", i, err))
			}
			actions = append(actions, a)
		}
	}

	// Enforce overall timeout via context.
	timeout := time.Duration(t.opts.TimeoutSeconds) * time.Second
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Launch Playwright + browser.
	pw, err := playwright.Run()
	if err != nil {
		return ErrorResult(fmt.Sprintf("failed to start playwright: %v", err))
	}
	defer pw.Stop() //nolint:errcheck // best-effort cleanup

	browser, err := launchBrowser(pw, t.opts.BrowserType, headless)
	if err != nil {
		return ErrorResult(fmt.Sprintf("failed to launch browser (%s): %v", t.opts.BrowserType, err))
	}
	defer browser.Close() //nolint:errcheck // best-effort cleanup

	browserCtx, err := browser.NewContext()
	if err != nil {
		return ErrorResult(fmt.Sprintf("failed to create browser context: %v", err))
	}
	defer browserCtx.Close() //nolint:errcheck // best-effort cleanup

	page, err := browserCtx.NewPage()
	if err != nil {
		return ErrorResult(fmt.Sprintf("failed to open page: %v", err))
	}

	// Navigate to starting URL.
	actionTimeout := float64(t.opts.TimeoutSeconds * 1000) // milliseconds for Playwright
	if _, err := page.Goto(startURL, playwright.PageGotoOptions{
		Timeout:   playwright.Float(actionTimeout),
		WaitUntil: playwright.WaitUntilStateDomcontentloaded,
	}); err != nil {
		return ErrorResult(fmt.Sprintf("failed to navigate to %s: %v", startURL, err))
	}

	results := []actionResult{}
	screenshots := []string{}

	// Execute each action in sequence.
	for i, action := range actions {
		// Check context cancellation before each action.
		select {
		case <-ctx.Done():
			return ErrorResult(fmt.Sprintf("browser browse timed out after %d actions", i))
		default:
		}

		res := t.executeAction(ctx, page, action, i+1, actionTimeout)
		results = append(results, res)

		if res.Error != "" {
			// Stop sequence on first error, return partial results.
			break
		}

		// Collect screenshot paths for the summary.
		if action.Type == "screenshot" && res.Result != "" {
			screenshots = append(screenshots, res.Result)
		}
	}

	// Build LLM-facing log.
	currentURL := page.URL()
	llmLog := buildLLMLog(startURL, currentURL, results, screenshots)

	// Build user-facing JSON report.
	report := map[string]any{
		"start_url":   startURL,
		"current_url": currentURL,
		"steps":       results,
		"screenshots": screenshots,
	}
	reportJSON, _ := json.MarshalIndent(report, "", "  ")

	return &ToolResult{
		ForLLM:  llmLog,
		ForUser: string(reportJSON),
	}
}

// buildLLMLog constructs a concise step-by-step log for the LLM.
func buildLLMLog(startURL, currentURL string, results []actionResult, screenshots []string) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "Browser session: started at %s\n", startURL)
	if currentURL != startURL {
		fmt.Fprintf(&sb, "Current URL: %s\n", currentURL)
	}
	if len(results) == 0 {
		sb.WriteString("No actions performed.\n")
	} else {
		sb.WriteString("\nSteps:\n")
		for _, r := range results {
			if r.Error != "" {
				fmt.Fprintf(&sb, "  [%d] %s => ERROR: %s\n", r.Step, r.Action, r.Error)
			} else {
				fmt.Fprintf(&sb, "  [%d] %s => %s\n", r.Step, r.Action, r.Result)
			}
		}
	}
	if len(screenshots) > 0 {
		sb.WriteString("\nScreenshots saved:\n")
		for _, s := range screenshots {
			fmt.Fprintf(&sb, "  %s\n", s)
		}
	}
	return sb.String()
}

// urlSlug returns a filesystem-safe slug derived from a URL.
func urlSlug(rawURL string) string {
	// Strip scheme and common prefixes.
	s := strings.TrimPrefix(rawURL, "https://")
	s = strings.TrimPrefix(s, "http://")
	// Replace unsafe characters.
	replacer := strings.NewReplacer(
		"/", "_",
		"?", "_",
		"&", "_",
		"=", "_",
		"#", "_",
		":", "_",
		".", "-",
	)
	s = replacer.Replace(s)
	if len(s) > 60 {
		s = s[:60]
	}
	return s
}

// truncateString truncates s to at most max runes.
func truncateString(s string, max int) string {
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max]) + "... [truncated]"
}
