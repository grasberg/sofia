package recipe

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"

	"github.com/grasberg/sofia/pkg/logger"
)

const defaultMaxTurns = 10

// RecipeResult holds the output of a recipe execution.
type RecipeResult struct {
	Content    string         `json:"content"`
	Structured map[string]any `json:"structured,omitempty"` // populated when Response schema is set
	Iterations int            `json:"iterations"`
}

// RunFunc is the callback that the executor invokes to run the LLM tool loop.
// It receives the system instructions (may be empty), the rendered user prompt,
// the recipe settings, and the response schema (nil when no structured output
// is expected). It returns the LLM content, any structured output captured by
// a final_output tool, the number of iterations, and an error.
type RunFunc func(
	ctx context.Context,
	instructions string,
	prompt string,
	settings RecipeSettings,
	responseSchema *ResponseSchema,
) (content string, structured map[string]any, iterations int, err error)

// RecipeExecutor runs recipes by delegating the LLM loop to a RunFunc.
// This keeps the recipe package free of import cycles with pkg/tools.
type RecipeExecutor struct {
	run RunFunc
}

// NewRecipeExecutor creates an executor that delegates LLM execution to runFn.
func NewRecipeExecutor(runFn RunFunc) *RecipeExecutor {
	return &RecipeExecutor{run: runFn}
}

// Execute runs a recipe with the given parameters.
func (e *RecipeExecutor) Execute(ctx context.Context, recipe *Recipe, params map[string]string) (*RecipeResult, error) {
	prompt, err := RenderPrompt(recipe, params)
	if err != nil {
		return nil, fmt.Errorf("render prompt: %w", err)
	}

	logger.InfoCF("recipe", "Executing recipe", map[string]any{
		"title":     recipe.Title,
		"model":     recipe.Settings.Model,
		"max_turns": recipe.Settings.MaxTurns,
	})

	content, structured, iterations, err := e.run(
		ctx,
		recipe.Instructions,
		prompt,
		recipe.Settings,
		recipe.Response,
	)
	if err != nil {
		return nil, fmt.Errorf("tool loop: %w", err)
	}

	result := &RecipeResult{
		Content:    content,
		Structured: structured,
		Iterations: iterations,
	}

	// Retry logic.
	if recipe.Retry != nil && recipe.Retry.MaxRetries > 0 {
		result, err = e.retryLoop(ctx, recipe, params, result)
		if err != nil {
			return result, err
		}
	}

	return result, nil
}

// retryLoop re-executes the recipe if any check fails, up to MaxRetries times.
func (e *RecipeExecutor) retryLoop(
	ctx context.Context,
	recipe *Recipe,
	params map[string]string,
	initial *RecipeResult,
) (*RecipeResult, error) {
	current := initial
	for attempt := 0; attempt < recipe.Retry.MaxRetries; attempt++ {
		if checksPassed(ctx, recipe.Retry.Checks) {
			return current, nil
		}

		logger.WarnCF("recipe", "Retry checks failed, retrying", map[string]any{
			"attempt": attempt + 1,
			"max":     recipe.Retry.MaxRetries,
		})

		// Run on_failure command if configured.
		if recipe.Retry.OnFailure != "" {
			runShellCommand(ctx, recipe.Retry.OnFailure)
		}

		// Re-execute.
		result, err := e.Execute(ctx, recipe, params)
		if err != nil {
			return current, fmt.Errorf("retry %d: %w", attempt+1, err)
		}
		current = result
	}

	// Final check after all retries.
	if !checksPassed(ctx, recipe.Retry.Checks) {
		return current, fmt.Errorf("recipe checks still failing after %d retries", recipe.Retry.MaxRetries)
	}
	return current, nil
}

// checksPassed runs all retry checks and returns true only if every check succeeds.
func checksPassed(ctx context.Context, checks []RetryCheck) bool {
	for _, check := range checks {
		if check.Shell.Command == "" {
			continue
		}
		if !runShellCheck(ctx, check.Shell.Command) {
			return false
		}
	}
	return true
}

// runShellCheck executes a command and returns true if exit code is 0.
func runShellCheck(ctx context.Context, command string) bool {
	cmd := exec.CommandContext(ctx, "sh", "-c", command)
	err := cmd.Run()
	return err == nil
}

// runShellCommand executes a command, logging any error.
func runShellCommand(ctx context.Context, command string) {
	cmd := exec.CommandContext(ctx, "sh", "-c", command)
	if output, err := cmd.CombinedOutput(); err != nil {
		logger.WarnCF("recipe", "on_failure command failed", map[string]any{
			"command": command,
			"error":   err.Error(),
			"output":  string(output),
		})
	}
}

// FormatResult returns a human-readable summary of a recipe execution.
func FormatResult(r *RecipeResult) string {
	var sb strings.Builder
	sb.WriteString(r.Content)
	if len(r.Structured) > 0 {
		data, err := json.MarshalIndent(r.Structured, "", "  ")
		if err == nil {
			sb.WriteString("\n\nStructured output:\n")
			sb.Write(data)
		}
	}
	return sb.String()
}
