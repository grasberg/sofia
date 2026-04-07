package tools

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/grasberg/sofia/pkg/logger"
)

var validPipelineName = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

// PipelineStep defines a single step in a composite tool pipeline.
type PipelineStep struct {
	Tool   string            `json:"tool"`   // Tool name to execute
	Params map[string]string `json:"params,omitempty"` // Parameter mapping: {"arg": "{{input.field}}"}
	Parallel bool            `json:"parallel,omitempty"` // If true, run in parallel with next step
}

// CompositeTool enables chaining existing tools together into reusable pipelines.
type CompositeTool struct {
	name        string
	description string
	steps       []PipelineStep // Pipeline steps with parameter mapping
	registry    *ToolRegistry
}

// NewCompositeTool constructs a macro-tool that executes other tools in sequence.
// For backwards compatibility, converts string steps to PipelineStep.
func NewCompositeTool(name, description string, steps any, registry *ToolRegistry) *CompositeTool {
	var pipelineSteps []PipelineStep
	
	switch v := steps.(type) {
	case []string:
		// Backwards compatibility: convert string array to PipelineStep array
		pipelineSteps = make([]PipelineStep, len(v))
		for i, stepName := range v {
			pipelineSteps[i] = PipelineStep{Tool: stepName}
		}
	case []PipelineStep:
		pipelineSteps = v
	}
	
	return &CompositeTool{
		name:        name,
		description: description,
		steps:       pipelineSteps,
		registry:    registry,
	}
}

func (c *CompositeTool) Name() string { return c.name }
func (c *CompositeTool) Description() string {
	stepNames := make([]string, len(c.steps))
	for i, s := range c.steps {
		stepNames[i] = s.Tool
	}
	return fmt.Sprintf("%s (Pipeline: %s)", c.description, strings.Join(stepNames, " -> "))
}

func (c *CompositeTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"input": map[string]any{
				"type":        "object",
				"description": "Input parameters for the pipeline. Structure depends on pipeline definition.",
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

	// Extract initial input
	initialInput := make(map[string]any)
	if input, ok := args["input"].(map[string]any); ok {
		initialInput = input
	} else if initial, ok := args["initial_input"].(string); ok {
		// Backwards compatibility
		initialInput["content"] = initial
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "Pipeline '%s' Execution Trace:\n", c.name)
	sb.WriteString("=========================================\n")

	// Track step results for parameter mapping
	stepResults := make(map[string]*ToolResult)
	stepOutputs := make(map[string]string)
	var currentOutput string

	for i, step := range c.steps {
		logger.DebugCF("tool:pipeline", "Executing pipeline step", map[string]any{
			"pipeline": c.name,
			"step_idx": i,
			"tool":     step.Tool,
		})

		// Prepare arguments for the sub-tool using parameter mapping
		stepArgs := c.resolveStepParams(step.Params, initialInput, stepResults, stepOutputs, currentOutput)

		result := c.registry.ExecuteWithContext(ctx, step.Tool, stepArgs, "", "", nil)

		stepKey := fmt.Sprintf("step%d", i+1)
		stepResults[stepKey] = result

		if result.IsError {
			fmt.Fprintf(&sb, "\n[Step %d: %s] ❌ FAILED\n", i+1, step.Tool)
			fmt.Fprintf(&sb, "Error: %v\n", result.Err)

			return ErrorResult(sb.String()).WithError(fmt.Errorf("pipeline step %s failed: %w", step.Tool, result.Err))
		}

		// Save the output for next step
		if result.ForLLM != "" {
			currentOutput = result.ForLLM
			stepOutputs[stepKey] = currentOutput
		}

		fmt.Fprintf(&sb, "\n[Step %d: %s] ✅ SUCCESS\n", i+1, step.Tool)

		// Truncate output in the trace if it's too long
		outStr := currentOutput
		if len(outStr) > 200 {
			outStr = outStr[:200] + "... (truncated)"
		}
		fmt.Fprintf(&sb, "Output preview: %s\n", outStr)

		// Check if this step should run in parallel with the next
		if step.Parallel && i+1 < len(c.steps) {
			// Execute next step in parallel
			nextStep := c.steps[i+1]
			nextStepArgs := c.resolveStepParams(nextStep.Params, initialInput, stepResults, stepOutputs, currentOutput)
			
			// Use goroutine for parallel execution
			resultChan := make(chan *ToolResult, 1)
			go func() {
				resultChan <- c.registry.ExecuteWithContext(ctx, nextStep.Tool, nextStepArgs, "", "", nil)
			}()

			// Wait for parallel step to complete
			result = <-resultChan
			nextStepKey := fmt.Sprintf("step%d", i+2)
			stepResults[nextStepKey] = result

			if result.IsError {
				fmt.Fprintf(&sb, "\n[Step %d: %s (parallel)] ❌ FAILED\n", i+2, nextStep.Tool)
				fmt.Fprintf(&sb, "Error: %v\n", result.Err)
				return ErrorResult(sb.String()).WithError(fmt.Errorf("parallel pipeline step %s failed: %w", nextStep.Tool, result.Err))
			}

			if result.ForLLM != "" {
				currentOutput = result.ForLLM
				stepOutputs[nextStepKey] = currentOutput
			}

			fmt.Fprintf(&sb, "\n[Step %d: %s (parallel)] ✅ SUCCESS\n", i+2, nextStep.Tool)
			i++ // Skip next step in main loop
		}
	}

	sb.WriteString("\nPipeline completed successfully.\n")
	sb.WriteString("-----------------------------------------\n")
	fmt.Fprintf(&sb, "Final Output:\n%s", currentOutput)

	return SilentResult(sb.String())
}

// resolveStepParams resolves parameter templates for a pipeline step.
// Supports templates like: "{{input.field}}", "{{step1.ForLLM}}", "{{previous_output}}"
func (c *CompositeTool) resolveStepParams(
	params map[string]string,
	initialInput map[string]any,
	stepResults map[string]*ToolResult,
	stepOutputs map[string]string,
	previousOutput string,
) map[string]any {
	stepArgs := make(map[string]any)

	// If no explicit params, use default behavior (inject previous_output)
	if len(params) == 0 {
		if previousOutput != "" {
			stepArgs["previous_output"] = previousOutput
			stepArgs["query"] = previousOutput
			stepArgs["input"] = previousOutput
			stepArgs["content"] = previousOutput
		}
		return stepArgs
	}

	// Resolve each parameter template
	for key, template := range params {
		resolved := c.resolveTemplate(template, initialInput, stepResults, stepOutputs, previousOutput)
		stepArgs[key] = resolved
	}

	return stepArgs
}

// resolveTemplate resolves a template string with placeholders.
func (c *CompositeTool) resolveTemplate(
	template string,
	initialInput map[string]any,
	stepResults map[string]*ToolResult,
	stepOutputs map[string]string,
	previousOutput string,
) any {
	// Handle {{input.field}} templates
	if strings.HasPrefix(template, "{{input.") && strings.HasSuffix(template, "}}") {
		field := template[8 : len(template)-2]
		if value, ok := initialInput[field]; ok {
			return value
		}
	}

	// Handle {{stepN.ForLLM}} or {{stepN}} templates
	if strings.HasPrefix(template, "{{step") && strings.HasSuffix(template, "}}") {
		inner := template[2 : len(template)-2]
		parts := strings.SplitN(inner, ".", 2)
		stepKey := parts[0]
		
		if result, ok := stepResults[stepKey]; ok {
			if len(parts) == 2 && parts[1] == "ForLLM" {
				return result.ForLLM
			} else if len(parts) == 1 {
				// Default to ForLLM
				return result.ForLLM
			}
		}
		
		// Try stepOutputs
		if output, ok := stepOutputs[stepKey]; ok {
			return output
		}
	}

	// Handle {{previous_output}} template
	if template == "{{previous_output}}" || template == "{{output}}" {
		return previousOutput
	}

	// No template match, return as-is
	return template
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
	return "Create a reusable pipeline by chaining existing tools together. Define the steps and parameter mappings."
}

func (t *CreatePipelineTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"name": map[string]any{
				"type":        "string",
				"description": "Name for the pipeline (lowercase, alphanumeric, underscores)",
			},
			"description": map[string]any{
				"type":        "string",
				"description": "Description of what the pipeline does",
			},
			"steps": map[string]any{
				"type": "array",
				"description": "Ordered list of pipeline steps. Each step can be a string (tool name) or object with tool, params, and parallel fields.",
				"items": map[string]any{
					"oneOf": []map[string]any{
						{"type": "string"},
						{
							"type": "object",
							"properties": map[string]any{
								"tool":     map[string]any{"type": "string", "description": "Tool name to execute"},
								"params":   map[string]any{"type": "object", "description": "Parameter mapping (optional)"},
								"parallel": map[string]any{"type": "boolean", "description": "Run in parallel with next step (optional)"},
							},
							"required": []string{"tool"},
						},
					},
				},
			},
		},
		"required": []string{"name", "description", "steps"},
	}
}

func (t *CreatePipelineTool) Execute(ctx context.Context, args map[string]any) *ToolResult {
	name, _ := args["name"].(string)
	desc, _ := args["description"].(string)
	
	if !validPipelineName.MatchString(name) {
		return ErrorResult("Invalid pipeline name. Must start with lowercase letter and contain only lowercase letters, numbers, and underscores.")
	}

	// Parse steps - can be array of strings or array of objects
	stepsRaw, ok := args["steps"].([]any)
	if !ok {
		return ErrorResult("steps must be an array")
	}

	var pipelineSteps []PipelineStep
	for _, stepRaw := range stepsRaw {
		switch v := stepRaw.(type) {
		case string:
			// Simple string step
			pipelineSteps = append(pipelineSteps, PipelineStep{Tool: v})
		case map[string]any:
			// Object step with params
			toolName, _ := v["tool"].(string)
			if toolName == "" {
				return ErrorResult("Each step object must have a 'tool' field")
			}
			
			step := PipelineStep{Tool: toolName}
			
			// Parse params if present
			if paramsRaw, ok := v["params"].(map[string]any); ok {
				step.Params = make(map[string]string)
				for k, v := range paramsRaw {
					if strVal, ok := v.(string); ok {
						step.Params[k] = strVal
					}
				}
			}
			
			// Parse parallel flag if present
			if parallel, ok := v["parallel"].(bool); ok {
				step.Parallel = parallel
			}
			
			pipelineSteps = append(pipelineSteps, step)
		default:
			return ErrorResult(fmt.Sprintf("Invalid step format at index %d", len(pipelineSteps)))
		}
	}

	if len(pipelineSteps) == 0 {
		return ErrorResult("Pipeline must have at least one step")
	}

	// Validate all tool names exist
	for _, step := range pipelineSteps {
		if _, exists := t.registry.Get(step.Tool); !exists {
			return ErrorResult(fmt.Sprintf("Unknown tool in pipeline: %s", step.Tool))
		}
	}

	// Create the pipeline
	pipeline := NewCompositeTool(name, desc, pipelineSteps, t.registry)
	_ = pipeline // Store in registry or return ID for later use

	logger.InfoCF("tool:pipeline", "Pipeline created", map[string]any{
		"name":  name,
		"steps": len(pipelineSteps),
	})

	return UserResult(fmt.Sprintf("Pipeline '%s' created successfully with %d steps: %s",
		name, len(pipelineSteps), formatPipelineSteps(pipelineSteps)))
}

func formatPipelineSteps(steps []PipelineStep) string {
	stepNames := make([]string, len(steps))
	for i, s := range steps {
		stepNames[i] = s.Tool
		if s.Parallel {
			stepNames[i] += " (parallel)"
		}
	}
	return strings.Join(stepNames, " -> ")
}
