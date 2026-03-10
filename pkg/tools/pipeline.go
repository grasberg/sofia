package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/grasberg/sofia/pkg/logger"
)

// CompositeTool enables chaining existing tools together into reusable pipelines.
type CompositeTool struct {
	name        string
	description string
	steps       []string // Ordered list of tool names to execute
	registry    *ToolRegistry
}

// NewCompositeTool constructs a macro-tool that executes other tools in sequence.
func NewCompositeTool(name, description string, steps []string, registry *ToolRegistry) *CompositeTool {
	return &CompositeTool{
		name:        name,
		description: description,
		steps:       steps,
		registry:    registry,
	}
}

func (c *CompositeTool) Name() string { return c.name }
func (c *CompositeTool) Description() string {
	return fmt.Sprintf("%s (Pipeline: %s)", c.description, strings.Join(c.steps, " -> "))
}

func (c *CompositeTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"initial_input": map[string]any{
				"type":        "string",
				"description": "The initial input to pass to the first tool in the pipeline",
			},
		},
	}
}

func (c *CompositeTool) Execute(ctx context.Context, args map[string]any) *ToolResult {
	if c.registry == nil {
		return ErrorResult("CompositeTool has no access to the ToolRegistry")
	}
	if len(c.steps) == 0 {
		return SilentResult("Pipeline executed: no steps defined.")
	}

	var currentOutput string
	if initial, ok := args["initial_input"].(string); ok {
		currentOutput = initial
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Pipeline '%s' Execution Trace:\n", c.name))
	sb.WriteString("=========================================\n")

	for i, stepName := range c.steps {
		logger.DebugCF("tool:pipeline", "Executing pipeline step", map[string]any{
			"pipeline": c.name,
			"step_idx": i,
			"tool":     stepName,
		})

		// Prepare arguments for the sub-tool.
		// If it's the first step, we pass whatever we mapped. Subsequent steps receive the previous output.
		// Since we don't know the exact parameter schema of the underlying tool,
		// we inject 'previous_output' generically, and 'query'/'input' as common aliases.
		stepArgs := map[string]any{}
		if currentOutput != "" {
			stepArgs["previous_output"] = currentOutput
			stepArgs["query"] = currentOutput
			stepArgs["input"] = currentOutput
			stepArgs["content"] = currentOutput
		}

		result := c.registry.ExecuteWithContext(ctx, stepName, stepArgs, "", "", nil)

		if result.IsError {
			sb.WriteString(fmt.Sprintf("\n[Step %d: %s] ❌ FAILED\n", i+1, stepName))
			sb.WriteString(fmt.Sprintf("Error: %v\n", result.Err))

			// Append the error to the trace but return immediately
			return ErrorResult(sb.String()).WithError(fmt.Errorf("pipeline step %s failed: %w", stepName, result.Err))
		}

		// Save the state for the next step
		if result.ForLLM != "" {
			currentOutput = result.ForLLM
		}

		sb.WriteString(fmt.Sprintf("\n[Step %d: %s] ✅ SUCCESS\n", i+1, stepName))

		// Truncate output in the trace if it's too long
		outStr := currentOutput
		if len(outStr) > 200 {
			outStr = outStr[:200] + "... (truncated)"
		}
		sb.WriteString(fmt.Sprintf("Output preview: %s\n", outStr))
	}

	sb.WriteString("\nPipeline completed successfully.\n")
	sb.WriteString("-----------------------------------------\n")
	sb.WriteString(fmt.Sprintf("Final Output:\n%s", currentOutput))

	return SilentResult(sb.String())
}

// CreatePipelineTool provides the LLM the ability to assemble an active pipeline.
type CreatePipelineTool struct {
	registry *ToolRegistry
}

func NewCreatePipelineTool(registry *ToolRegistry) *CreatePipelineTool {
	return &CreatePipelineTool{registry: registry}
}

func (t *CreatePipelineTool) Name() string { return "create_pipeline" }
func (t *CreatePipelineTool) Description() string {
	return "Creates a new reusable macro-tool by chaining existing tools together into a pipeline."
}

func (t *CreatePipelineTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"pipeline_name": map[string]any{
				"type":        "string",
				"description": "The unique name of the new tool (lowercase, underscore).",
			},
			"description": map[string]any{
				"type":        "string",
				"description": "What this new macro-tool accomplishes.",
			},
			"steps": map[string]any{
				"type":        "array",
				"items":       map[string]any{"type": "string"},
				"description": "The ordered list of existing tool names to chain together.",
			},
		},
		"required": []string{"pipeline_name", "description", "steps"},
	}
}

func (t *CreatePipelineTool) Execute(ctx context.Context, args map[string]any) *ToolResult {
	if t.registry == nil {
		return ErrorResult("create_pipeline tool has no access to ToolRegistry")
	}

	name, _ := args["pipeline_name"].(string)
	desc, _ := args["description"].(string)

	var steps []string
	if rawSteps, ok := args["steps"]; ok {
		// handle both []any and json unmarshaled lists
		if list, ok := rawSteps.([]any); ok {
			for _, s := range list {
				if str, ok := s.(string); ok {
					steps = append(steps, str)
				}
			}
		} else if bytes, err := json.Marshal(rawSteps); err == nil {
			_ = json.Unmarshal(bytes, &steps)
		}
	}

	if name == "" || desc == "" || len(steps) == 0 {
		return ErrorResult("pipeline_name, description, and steps (must not be empty) are required parameters")
	}

	// Validate that all steps exist
	for _, stepName := range steps {
		if _, ok := t.registry.Get(stepName); !ok {
			return ErrorResult(fmt.Sprintf("cannot create pipeline: step tool '%s' does not exist in the registry", stepName))
		}
	}

	// Construct and register the macro-tool
	macro := NewCompositeTool(name, desc, steps, t.registry)
	t.registry.Register(macro)

	logMsg := fmt.Sprintf("Successfully created and registered pipeline tool '%s' with %d steps: %s",
		name, len(steps), strings.Join(steps, " -> "))
	logger.InfoCF("tool:pipeline", "Pipeline created", map[string]any{
		"pipeline_name": name,
		"steps":         steps,
	})

	return SilentResult(logMsg)
}
