package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/grasberg/sofia/pkg/providers"
	"github.com/grasberg/sofia/pkg/recipe"
)

const finalOutputToolName = "final_output"

// RecipeTool exposes recipe discovery and execution to agents.
// Supported actions: list, show, run.
type RecipeTool struct {
	workspace string
	provider  providers.LLMProvider
	registry  *ToolRegistry
}

// NewRecipeTool creates a RecipeTool that discovers recipes from the given
// workspace path and executes them using the provided LLM provider and tools.
func NewRecipeTool(workspace string, provider providers.LLMProvider, registry *ToolRegistry) *RecipeTool {
	return &RecipeTool{
		workspace: workspace,
		provider:  provider,
		registry:  registry,
	}
}

func (t *RecipeTool) Name() string { return "recipe" }

func (t *RecipeTool) Description() string {
	return "Discover and run recipes (reusable agent workflows). Actions: " +
		"list (discover available recipes), " +
		"show <recipe_name> (display recipe details), " +
		"run <recipe_name> with params (execute a recipe)."
}

func (t *RecipeTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"action": map[string]any{
				"type":        "string",
				"enum":        []string{"list", "show", "run"},
				"description": "The action to perform",
			},
			"recipe_name": map[string]any{
				"type":        "string",
				"description": "Recipe name (required for show and run)",
			},
			"params": map[string]any{
				"type":        "object",
				"description": "Key-value parameters for running the recipe",
				"additionalProperties": map[string]any{
					"type": "string",
				},
			},
		},
		"required": []string{"action"},
	}
}

func (t *RecipeTool) Execute(ctx context.Context, args map[string]any) *ToolResult {
	action, _ := args["action"].(string)

	switch action {
	case "list":
		return t.executeList()
	case "show":
		return t.executeShow(args)
	case "run":
		return t.executeRun(ctx, args)
	default:
		return ErrorResult(fmt.Sprintf("unknown action: %q; use list, show, or run", action))
	}
}

func (t *RecipeTool) executeList() *ToolResult {
	metas, err := recipe.ListRecipes(t.workspace)
	if err != nil {
		return ErrorResult(fmt.Sprintf("failed to list recipes: %v", err))
	}

	if len(metas) == 0 {
		return SilentResult("No recipes found. Place .yaml files in workspace/recipes/ or ~/.sofia/recipes/.")
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "Available recipes (%d):\n\n", len(metas))
	for _, m := range metas {
		fmt.Fprintf(&sb, "- **%s**", m.Name)
		if m.Title != "" {
			fmt.Fprintf(&sb, " (%s)", m.Title)
		}
		if m.Description != "" {
			fmt.Fprintf(&sb, ": %s", m.Description)
		}
		fmt.Fprintf(&sb, " [%s]\n", m.Source)
	}
	return SilentResult(sb.String())
}

func (t *RecipeTool) executeShow(args map[string]any) *ToolResult {
	name, _ := args["recipe_name"].(string)
	if name == "" {
		return ErrorResult("recipe_name is required for show action")
	}

	r, path, err := t.findRecipe(name)
	if err != nil {
		return ErrorResult(err.Error())
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "Recipe: %s\n", r.Title)
	fmt.Fprintf(&sb, "Path: %s\n", path)
	if r.Description != "" {
		fmt.Fprintf(&sb, "Description: %s\n", r.Description)
	}
	if r.Author != "" {
		fmt.Fprintf(&sb, "Author: %s\n", r.Author)
	}
	if r.Settings.Model != "" {
		fmt.Fprintf(&sb, "Model: %s\n", r.Settings.Model)
	}
	if len(r.Parameters) > 0 {
		sb.WriteString("\nParameters:\n")
		for _, p := range r.Parameters {
			req := "optional"
			if p.Required {
				req = "required"
			}
			fmt.Fprintf(&sb, "  - %s (%s, %s)", p.Key, p.InputType, req)
			if p.Description != "" {
				fmt.Fprintf(&sb, ": %s", p.Description)
			}
			if p.Default != "" {
				fmt.Fprintf(&sb, " [default: %s]", p.Default)
			}
			sb.WriteString("\n")
		}
	}
	fmt.Fprintf(&sb, "\nPrompt template:\n%s", r.Prompt)
	return SilentResult(sb.String())
}

func (t *RecipeTool) executeRun(ctx context.Context, args map[string]any) *ToolResult {
	name, _ := args["recipe_name"].(string)
	if name == "" {
		return ErrorResult("recipe_name is required for run action")
	}

	r, _, err := t.findRecipe(name)
	if err != nil {
		return ErrorResult(err.Error())
	}

	// Extract params.
	params := make(map[string]string)
	if rawParams, ok := args["params"]; ok {
		switch v := rawParams.(type) {
		case map[string]any:
			for k, val := range v {
				params[k] = fmt.Sprintf("%v", val)
			}
		case map[string]string:
			params = v
		}
	}

	// Build the RunFunc that bridges recipe execution into the tools layer.
	runFn := t.buildRunFunc()

	executor := recipe.NewRecipeExecutor(runFn)
	result, err := executor.Execute(ctx, r, params)
	if err != nil {
		return ErrorResult(fmt.Sprintf("recipe execution failed: %v", err))
	}

	output := recipe.FormatResult(result)
	if len(result.Structured) > 0 {
		data, jsonErr := json.Marshal(result.Structured)
		if jsonErr == nil {
			return SilentResult(output).WithStructuredData(result.Structured, string(data))
		}
	}
	return SilentResult(output)
}

// buildRunFunc creates a recipe.RunFunc that delegates to RunToolLoop with the
// tool's provider and registry.
func (t *RecipeTool) buildRunFunc() recipe.RunFunc {
	return func(
		ctx context.Context,
		instructions string,
		prompt string,
		settings recipe.RecipeSettings,
		responseSchema *recipe.ResponseSchema,
	) (string, map[string]any, int, error) {
		// Determine model.
		model := settings.Model
		if model == "" && t.provider != nil {
			model = t.provider.GetDefaultModel()
		}

		maxTurns := settings.MaxTurns
		if maxTurns <= 0 {
			maxTurns = 10
		}

		llmOpts := map[string]any{}
		if settings.Temperature > 0 {
			llmOpts["temperature"] = settings.Temperature
		}

		// Clone registry and optionally add final_output tool.
		execRegistry := t.registry
		var capture *finalOutputCapture
		if responseSchema != nil && len(responseSchema.JSONSchema) > 0 {
			execRegistry = cloneRegistryWithFinalOutput(t.registry, responseSchema.JSONSchema)
			if tool, ok := execRegistry.Get(finalOutputToolName); ok {
				capture, _ = tool.(*finalOutputCapture)
			}
		}

		// Build messages.
		var messages []providers.Message
		if instructions != "" {
			messages = append(messages, providers.Message{
				Role:    "system",
				Content: instructions,
			})
		}
		messages = append(messages, providers.Message{
			Role:    "user",
			Content: prompt,
		})

		loopResult, err := RunToolLoop(ctx, ToolLoopConfig{
			Provider:      t.provider,
			Model:         model,
			Tools:         execRegistry,
			MaxIterations: maxTurns,
			LLMOptions:    llmOpts,
		}, messages, "", "")
		if err != nil {
			return "", nil, 0, err
		}

		var structured map[string]any
		if capture != nil {
			structured = capture.captured
		}

		return loopResult.Content, structured, loopResult.Iterations, nil
	}
}

// findRecipe locates a recipe by name across all discovery paths.
func (t *RecipeTool) findRecipe(name string) (*recipe.Recipe, string, error) {
	metas, err := recipe.ListRecipes(t.workspace)
	if err != nil {
		return nil, "", fmt.Errorf("failed to list recipes: %w", err)
	}

	for _, m := range metas {
		if m.Name == name {
			r, loadErr := recipe.LoadRecipe(m.Path)
			if loadErr != nil {
				return nil, "", fmt.Errorf("failed to load recipe %q: %w", name, loadErr)
			}
			return r, m.Path, nil
		}
	}

	return nil, "", fmt.Errorf("recipe %q not found", name)
}

// cloneRegistryWithFinalOutput copies tools from src into a new registry and
// adds a final_output tool that captures structured output.
func cloneRegistryWithFinalOutput(src *ToolRegistry, schema map[string]any) *ToolRegistry {
	dst := NewToolRegistry()
	for _, name := range src.List() {
		if t, ok := src.Get(name); ok {
			dst.Register(t)
		}
	}
	dst.Register(&finalOutputCapture{schema: schema})
	return dst
}

// finalOutputCapture is a synthetic tool registered when a recipe defines a
// response schema. The LLM calls it to produce structured output.
type finalOutputCapture struct {
	schema   map[string]any
	captured map[string]any
}

func (f *finalOutputCapture) Name() string { return finalOutputToolName }

func (f *finalOutputCapture) Description() string {
	return "Submit the final structured output for this recipe. " +
		"The output must conform to the recipe's response schema."
}

func (f *finalOutputCapture) Parameters() map[string]any {
	if len(f.schema) > 0 {
		return f.schema
	}
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"output": map[string]any{
				"type":        "object",
				"description": "The structured output object",
			},
		},
		"required": []string{"output"},
	}
}

func (f *finalOutputCapture) Execute(_ context.Context, args map[string]any) *ToolResult {
	f.captured = args

	summary, err := json.Marshal(args)
	if err != nil {
		return ErrorResult(fmt.Sprintf("failed to marshal final output: %v", err))
	}
	return SilentResult(fmt.Sprintf("Final output recorded: %s", string(summary)))
}
