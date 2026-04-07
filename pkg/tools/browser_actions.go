package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/playwright-community/playwright-go"
)

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
			res.Error = "context canceled during wait"
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
