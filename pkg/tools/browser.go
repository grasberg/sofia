package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
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
							"description": "JavaScript expression to evaluate (returns serialised result)",
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

// browseAction represents a single parsed action from the LLM.
type browseAction struct {
	Type         string
	Selector     string
	Value        string
	URL          string
	Milliseconds int
	Key          string
	Script       string
	Name         string
}

// actionResult records the outcome of one action.
type actionResult struct {
	Step   int    `json:"step"`
	Action string `json:"action"`
	Result string `json:"result,omitempty"`
	Error  string `json:"error,omitempty"`
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

// executeAction performs a single browse action on the page.
func (t *WebBrowseTool) executeAction(
	ctx context.Context,
	page playwright.Page,
	action browseAction,
	step int,
	actionTimeoutMs float64,
) actionResult {
	res := actionResult{
		Step:   step,
		Action: action.Type,
	}
	if action.Selector != "" {
		res.Action = fmt.Sprintf("%s(%q)", action.Type, action.Selector)
	}

	switch action.Type {
	case "navigate":
		if action.URL == "" {
			res.Error = "navigate requires url"
			return res
		}
		if err := validateBrowseURL(action.URL); err != nil {
			res.Error = fmt.Sprintf("URL blocked: %v", err)
			return res
		}
		if _, err := page.Goto(action.URL, playwright.PageGotoOptions{
			Timeout:   playwright.Float(actionTimeoutMs),
			WaitUntil: playwright.WaitUntilStateDomcontentloaded,
		}); err != nil {
			res.Error = err.Error()
			return res
		}
		res.Result = fmt.Sprintf("navigated to %s", action.URL)

	case "click":
		if action.Selector == "" {
			res.Error = "click requires selector"
			return res
		}
		if err := page.Locator(action.Selector).Click(playwright.LocatorClickOptions{
			Timeout: playwright.Float(actionTimeoutMs),
		}); err != nil {
			res.Error = err.Error()
			return res
		}
		res.Result = "clicked"

	case "fill":
		if action.Selector == "" {
			res.Error = "fill requires selector"
			return res
		}
		if err := page.Locator(action.Selector).Fill(action.Value, playwright.LocatorFillOptions{
			Timeout: playwright.Float(actionTimeoutMs),
		}); err != nil {
			res.Error = err.Error()
			return res
		}
		res.Result = fmt.Sprintf("filled with %q", action.Value)

	case "select":
		if action.Selector == "" {
			res.Error = "select requires selector"
			return res
		}
		if _, err := page.Locator(action.Selector).SelectOption(playwright.SelectOptionValues{
			Values: &[]string{action.Value},
		}, playwright.LocatorSelectOptionOptions{
			Timeout: playwright.Float(actionTimeoutMs),
		}); err != nil {
			res.Error = err.Error()
			return res
		}
		res.Result = fmt.Sprintf("selected %q", action.Value)

	case "wait":
		ms := action.Milliseconds
		if ms <= 0 {
			ms = 500
		}
		select {
		case <-ctx.Done():
			res.Error = "context cancelled during wait"
			return res
		case <-time.After(time.Duration(ms) * time.Millisecond):
		}
		res.Result = fmt.Sprintf("waited %dms", ms)

	case "wait_for":
		if action.Selector == "" {
			res.Error = "wait_for requires selector"
			return res
		}
		if err := page.Locator(action.Selector).WaitFor(playwright.LocatorWaitForOptions{
			Timeout: playwright.Float(actionTimeoutMs),
		}); err != nil {
			res.Error = err.Error()
			return res
		}
		res.Result = fmt.Sprintf("element %q is visible", action.Selector)

	case "screenshot":
		path, err := t.takeScreenshot(page, action.Name)
		if err != nil {
			res.Error = err.Error()
			return res
		}
		res.Result = path

	case "get_text":
		if action.Selector == "" {
			res.Error = "get_text requires selector"
			return res
		}
		text, err := page.Locator(action.Selector).InnerText(playwright.LocatorInnerTextOptions{
			Timeout: playwright.Float(actionTimeoutMs),
		})
		if err != nil {
			res.Error = err.Error()
			return res
		}
		res.Result = text

	case "get_html":
		if action.Selector == "" {
			// Full page HTML.
			html, err := page.Content()
			if err != nil {
				res.Error = err.Error()
				return res
			}
			res.Result = truncateString(html, 20000)
		} else {
			html, err := page.Locator(action.Selector).InnerHTML(playwright.LocatorInnerHTMLOptions{
				Timeout: playwright.Float(actionTimeoutMs),
			})
			if err != nil {
				res.Error = err.Error()
				return res
			}
			res.Result = truncateString(html, 20000)
		}

	case "scroll":
		if action.Selector != "" {
			if err := page.Locator(action.Selector).ScrollIntoViewIfNeeded(
				playwright.LocatorScrollIntoViewIfNeededOptions{
					Timeout: playwright.Float(actionTimeoutMs),
				},
			); err != nil {
				res.Error = err.Error()
				return res
			}
			res.Result = fmt.Sprintf("scrolled to %q", action.Selector)
		} else {
			if _, err := page.Evaluate("window.scrollBy(0, window.innerHeight)"); err != nil {
				res.Error = err.Error()
				return res
			}
			res.Result = "scrolled down one viewport"
		}

	case "hover":
		if action.Selector == "" {
			res.Error = "hover requires selector"
			return res
		}
		if err := page.Locator(action.Selector).Hover(playwright.LocatorHoverOptions{
			Timeout: playwright.Float(actionTimeoutMs),
		}); err != nil {
			res.Error = err.Error()
			return res
		}
		res.Result = "hovered"

	case "press":
		if action.Selector == "" {
			res.Error = "press requires selector"
			return res
		}
		key := action.Key
		if key == "" {
			key = "Enter"
		}
		if err := page.Locator(action.Selector).Press(key, playwright.LocatorPressOptions{
			Timeout: playwright.Float(actionTimeoutMs),
		}); err != nil {
			res.Error = err.Error()
			return res
		}
		res.Result = fmt.Sprintf("pressed %q", key)

	case "evaluate":
		if action.Script == "" {
			res.Error = "evaluate requires script"
			return res
		}
		val, err := page.Evaluate(action.Script)
		if err != nil {
			res.Error = err.Error()
			return res
		}
		out, _ := json.Marshal(val)
		res.Result = string(out)

	default:
		res.Error = fmt.Sprintf("unknown action type %q", action.Type)
	}

	return res
}

// takeScreenshot saves a PNG screenshot and returns its path.
func (t *WebBrowseTool) takeScreenshot(page playwright.Page, name string) (string, error) {
	dir := t.opts.ScreenshotDir
	if dir == "" {
		dir = "screenshots"
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("failed to create screenshot directory: %w", err)
	}

	if name == "" {
		slug := urlSlug(page.URL())
		name = fmt.Sprintf("%s-%s", time.Now().Format("20060102-150405"), slug)
	}
	path := filepath.Join(dir, name+".png")

	if _, err := page.Screenshot(playwright.PageScreenshotOptions{
		Path:     playwright.String(path),
		FullPage: playwright.Bool(false),
	}); err != nil {
		return "", fmt.Errorf("screenshot failed: %w", err)
	}
	return path, nil
}

// launchBrowser launches the requested browser type.
func launchBrowser(pw *playwright.Playwright, browserType string, headless bool) (playwright.Browser, error) {
	opts := playwright.BrowserTypeLaunchOptions{
		Headless: playwright.Bool(headless),
	}
	switch strings.ToLower(browserType) {
	case "firefox":
		return pw.Firefox.Launch(opts)
	case "webkit":
		return pw.WebKit.Launch(opts)
	default:
		return pw.Chromium.Launch(opts)
	}
}

// parseAction converts a raw map[string]any action into a browseAction.
func parseAction(m map[string]any) (browseAction, error) {
	a := browseAction{}

	t, ok := m["type"].(string)
	if !ok || t == "" {
		return a, fmt.Errorf("action missing required field 'type'")
	}
	a.Type = t

	if v, ok := m["selector"].(string); ok {
		a.Selector = v
	}
	if v, ok := m["value"].(string); ok {
		a.Value = v
	}
	if v, ok := m["url"].(string); ok {
		a.URL = v
	}
	if v, ok := m["key"].(string); ok {
		a.Key = v
	}
	if v, ok := m["script"].(string); ok {
		a.Script = v
	}
	if v, ok := m["name"].(string); ok {
		a.Name = v
	}
	if v, ok := m["milliseconds"].(float64); ok {
		a.Milliseconds = int(v)
	}

	return a, nil
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
