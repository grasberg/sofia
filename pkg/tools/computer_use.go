// Sofia - Ultra-lightweight personal AI agent
// Inspired by and based on nanobot: https://github.com/HKUDS/nanobot
// License: MIT
//
// Copyright (c) 2026 Sofia contributors

package tools

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/grasberg/sofia/pkg/logger"
	"github.com/grasberg/sofia/pkg/providers"
)

const (
	defaultComputerUseMaxSteps      = 5
	defaultComputerUseScreenshotDir = ""
)

// computerAction is the JSON structure the LLM returns for each step.
type computerAction struct {
	Action string `json:"action"` // "click", "type", "press", "scroll", "screenshot", "done"
	X      int    `json:"x"`
	Y      int    `json:"y"`
	Text   string `json:"text"`
	Key    string `json:"key"`
	DX     int    `json:"dx"` // scroll delta x
	DY     int    `json:"dy"` // scroll delta y
}

// ComputerUseTool takes desktop screenshots, sends them to the vision LLM,
// and executes the requested mouse/keyboard actions in a loop until the task
// is complete or max_steps is reached.
//
// Platform support:
//   - macOS: screencapture + osascript
//   - Linux: scrot/gnome-screenshot + xdotool
//   - Windows/other: not supported
type ComputerUseTool struct {
	workspace     string
	screenshotDir string
	provider      providers.LLMProvider
	modelID       string
}

// ComputerUseOptions configures the ComputerUseTool.
type ComputerUseOptions struct {
	Workspace     string
	ScreenshotDir string
	Provider      providers.LLMProvider
	ModelID       string
}

// NewComputerUseTool creates a ComputerUseTool.
// provider and modelID are used for the vision LLM calls inside the loop.
func NewComputerUseTool(opts ComputerUseOptions) *ComputerUseTool {
	screenshotDir := opts.ScreenshotDir
	if screenshotDir == "" && opts.Workspace != "" {
		screenshotDir = filepath.Join(opts.Workspace, "screenshots", "computer_use")
	}
	return &ComputerUseTool{
		workspace:     opts.Workspace,
		screenshotDir: screenshotDir,
		provider:      opts.Provider,
		modelID:       opts.ModelID,
	}
}

func (t *ComputerUseTool) Name() string { return "computer_use" }

func (t *ComputerUseTool) Description() string {
	return "Control the desktop by taking screenshots and executing mouse/keyboard actions. " +
		"Provide a natural-language task and the tool will autonomously take screenshots, " +
		"interpret the screen, and click/type until the task is done. " +
		"Only available on macOS and Linux."
}

func (t *ComputerUseTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"task": map[string]any{
				"type": "string",
				"description": "Natural language description of what to do on the desktop, " +
					"e.g. \"Click the Submit button in the browser\", " +
					"\"Open Terminal and run ls -la\", " +
					"\"Take a screenshot of the current screen.\"",
			},
			"max_steps": map[string]any{
				"type": "integer",
				"description": fmt.Sprintf(
					"Maximum number of screenshot→action iterations before stopping. Default: %d.",
					defaultComputerUseMaxSteps,
				),
				"default": defaultComputerUseMaxSteps,
			},
		},
		"required": []string{"task"},
	}
}

func (t *ComputerUseTool) Execute(ctx context.Context, args map[string]any) *ToolResult {
	task, ok := args["task"].(string)
	if !ok || strings.TrimSpace(task) == "" {
		return ErrorResult("task is required")
	}
	task = strings.TrimSpace(task)

	maxSteps := defaultComputerUseMaxSteps
	if v, ok := args["max_steps"].(float64); ok && v > 0 {
		maxSteps = int(v)
	}

	if !isComputerUseSupported() {
		return ErrorResult(computerUsePlatformError())
	}

	if t.provider == nil {
		return ErrorResult("computer_use requires a vision-capable LLM provider; none is configured")
	}

	if err := os.MkdirAll(t.screenshotDir, 0o755); err != nil {
		return ErrorResult(fmt.Sprintf("failed to create screenshot directory: %v", err))
	}

	var stepLog strings.Builder
	fmt.Fprintf(&stepLog, "Task: %s\n\n", task)

	for step := 1; step <= maxSteps; step++ {
		logger.InfoCF("computer_use", fmt.Sprintf("Step %d/%d: taking screenshot", step, maxSteps), nil)
		fmt.Fprintf(&stepLog, "Step %d: ", step)

		// 1. Take screenshot
		screenshotPath, err := takeDesktopScreenshot(t.screenshotDir)
		if err != nil {
			msg := fmt.Sprintf("screenshot failed: %v", err)
			fmt.Fprintln(&stepLog, msg)
			return ErrorResult(stepLog.String()).WithError(err)
		}
		fmt.Fprintf(&stepLog, "screenshot saved to %s\n", filepath.Base(screenshotPath))

		// 2. Read screenshot as base64
		imageData, err := os.ReadFile(screenshotPath)
		if err != nil {
			return ErrorResult(fmt.Sprintf("failed to read screenshot: %v", err)).WithError(err)
		}
		dataURL := "data:image/png;base64," + base64.StdEncoding.EncodeToString(imageData)

		// 3. Ask vision LLM what action to take
		action, raw, err := t.askVisionLLM(ctx, task, dataURL, stepLog.String())
		if err != nil {
			fmt.Fprintf(&stepLog, "LLM error: %v\n", err)
			return ErrorResult(stepLog.String()).WithError(err)
		}

		logger.InfoCF("computer_use", fmt.Sprintf("Step %d: LLM action=%s", step, action.Action),
			map[string]any{"raw": raw, "action": action.Action, "x": action.X, "y": action.Y, "text": action.Text})

		// 4. Check for done
		if strings.EqualFold(action.Action, "done") {
			fmt.Fprintln(&stepLog, "done (LLM signaled task complete)")
			break
		}

		// 5. Execute action
		if err := executeDesktopAction(action); err != nil {
			fmt.Fprintf(&stepLog, "action error: %v\n", err)
			return ErrorResult(stepLog.String()).WithError(err)
		}
		fmt.Fprintf(&stepLog, "executed: %s\n", describeAction(action))

		// Short pause to let the UI settle before next screenshot
		select {
		case <-ctx.Done():
			fmt.Fprintln(&stepLog, "canceled by context")
			return ErrorResult(stepLog.String())
		case <-time.After(500 * time.Millisecond):
		}

		if step == maxSteps {
			fmt.Fprintf(&stepLog, "\nReached max_steps=%d; stopping.\n", maxSteps)
		}
	}

	return &ToolResult{
		ForLLM:  stepLog.String(),
		ForUser: fmt.Sprintf("computer_use completed %d step(s):\n%s", maxSteps, stepLog.String()),
	}
}

// askVisionLLM sends the current screenshot to the vision LLM and asks it what
// single action to take next to make progress on the task.
func (t *ComputerUseTool) askVisionLLM(
	ctx context.Context,
	task, dataURL, previousSteps string,
) (*computerAction, string, error) {
	prompt := buildVisionPrompt(task, previousSteps)

	msg := providers.Message{
		Role:    "user",
		Content: prompt,
		Images:  []string{dataURL},
	}

	resp, err := t.provider.Chat(
		ctx,
		[]providers.Message{msg},
		nil,
		t.modelID,
		map[string]any{
			"max_tokens":  256,
			"temperature": 0.1,
		},
	)
	if err != nil {
		return nil, "", fmt.Errorf("vision LLM call failed: %w", err)
	}

	action, err := parseActionJSON(resp.Content)
	if err != nil {
		return nil, resp.Content, fmt.Errorf("could not parse LLM action JSON (%q): %w", resp.Content, err)
	}
	return action, resp.Content, nil
}

// buildVisionPrompt constructs the prompt sent to the vision LLM at each step.
func buildVisionPrompt(task, previousSteps string) string {
	var sb strings.Builder
	sb.WriteString("You are controlling a desktop computer to complete a task.\n\n")
	sb.WriteString("TASK: ")
	sb.WriteString(task)
	sb.WriteString("\n\n")

	if previousSteps != "" {
		sb.WriteString("PREVIOUS STEPS:\n")
		sb.WriteString(previousSteps)
		sb.WriteString("\n\n")
	}

	sb.WriteString("Look at the screenshot above. What single action should be taken next?\n\n")
	sb.WriteString("Reply ONLY with a JSON object in this exact format (no other text):\n")
	sb.WriteString(`{"action":"<action>","x":<int>,"y":<int>,"text":"<string>","key":"<string>","dx":<int>,"dy":<int>}`)
	sb.WriteString("\n\n")
	sb.WriteString("action must be one of:\n")
	sb.WriteString("  click      - left-click at (x,y)\n")
	sb.WriteString("  right_click - right-click at (x,y)\n")
	sb.WriteString("  double_click - double-click at (x,y)\n")
	sb.WriteString("  type       - type the text string (no coordinates needed)\n")
	sb.WriteString("  press      - press a key (e.g. Return, Tab, Escape, ctrl+c)\n")
	sb.WriteString("  scroll     - scroll at (x,y) by (dx,dy) pixels\n")
	sb.WriteString("  screenshot - take a fresh screenshot to reassess (no other fields needed)\n")
	sb.WriteString("  done       - task is complete, stop the loop\n")
	sb.WriteString("\nOmit fields that are not relevant to the action (use 0 or empty string).")

	return sb.String()
}

// parseActionJSON extracts a computerAction from the LLM response.
// The LLM is instructed to return only JSON, but may include markdown fences.
func parseActionJSON(raw string) (*computerAction, error) {
	s := strings.TrimSpace(raw)

	// Strip markdown code fences if present
	if strings.HasPrefix(s, "```") {
		lines := strings.Split(s, "\n")
		var inner []string
		for i, line := range lines {
			if i == 0 {
				continue // skip opening ```json or ```
			}
			if strings.HasPrefix(strings.TrimSpace(line), "```") {
				break
			}
			inner = append(inner, line)
		}
		s = strings.Join(inner, "\n")
	}

	// Find the JSON object boundaries
	start := strings.Index(s, "{")
	end := strings.LastIndex(s, "}")
	if start == -1 || end == -1 || end <= start {
		return nil, fmt.Errorf("no JSON object found in response")
	}
	s = s[start : end+1]

	var action computerAction
	if err := json.Unmarshal([]byte(s), &action); err != nil {
		return nil, err
	}
	return &action, nil
}

// describeAction returns a human-readable string for a computer action.
func describeAction(a *computerAction) string {
	switch strings.ToLower(a.Action) {
	case "click":
		return fmt.Sprintf("click at (%d, %d)", a.X, a.Y)
	case "right_click":
		return fmt.Sprintf("right-click at (%d, %d)", a.X, a.Y)
	case "double_click":
		return fmt.Sprintf("double-click at (%d, %d)", a.X, a.Y)
	case "type":
		preview := a.Text
		if len(preview) > 40 {
			preview = preview[:40] + "..."
		}
		return fmt.Sprintf("type %q", preview)
	case "press":
		return fmt.Sprintf("press key %q", a.Key)
	case "scroll":
		return fmt.Sprintf("scroll at (%d, %d) by dx=%d dy=%d", a.X, a.Y, a.DX, a.DY)
	case "screenshot":
		return "take screenshot"
	case "done":
		return "done"
	default:
		return a.Action
	}
}
