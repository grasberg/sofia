package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// JqTool provides JSON querying and transformation using jq-like expressions.
// Uses a built-in evaluator for common operations without requiring external binaries.
type JqTool struct{}

func NewJqTool() *JqTool { return &JqTool{} }

func (t *JqTool) Name() string { return "jq" }
func (t *JqTool) Description() string {
	return "Query and transform JSON data using jq expressions. Pass JSON input and a jq filter expression. Supports common jq operations: field access (.field), array indexing (.[0]), pipes (|), array iteration (.[] ), keys, values, length, select, map, type, sort_by, group_by, unique, flatten, etc."
}

func (t *JqTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"input": map[string]any{
				"type":        "string",
				"description": "JSON string to process",
			},
			"filter": map[string]any{
				"type":        "string",
				"description": "jq filter expression (e.g., '.name', '.[].id', '.[] | select(.age > 30)')",
			},
			"raw_output": map[string]any{
				"type":        "boolean",
				"description": "Output raw strings without JSON quotes (like jq -r). Default false.",
			},
			"compact": map[string]any{
				"type":        "boolean",
				"description": "Compact output (no indentation). Default false.",
			},
		},
		"required": []string{"input", "filter"},
	}
}

func (t *JqTool) Execute(ctx context.Context, args map[string]any) *ToolResult {
	inputStr, ok := args["input"].(string)
	if !ok {
		return ErrorResult("input is required (JSON string)")
	}

	filter, ok := args["filter"].(string)
	if !ok || filter == "" {
		return ErrorResult("filter is required (jq expression)")
	}

	rawOutput := false
	if raw, ok := args["raw_output"].(bool); ok {
		rawOutput = raw
	}

	compact := false
	if c, ok := args["compact"].(bool); ok {
		compact = c
	}

	// Parse input JSON
	var input any
	if err := json.Unmarshal([]byte(inputStr), &input); err != nil {
		return ErrorResult(fmt.Sprintf("invalid JSON input: %v", err))
	}

	// Execute filter via jq CLI if available, otherwise use built-in
	result, err := t.executeJqCLI(ctx, inputStr, filter, rawOutput, compact)
	if err != nil {
		// Fallback: try built-in evaluator for simple expressions
		result, err = evaluateSimpleJq(input, filter)
		if err != nil {
			return ErrorResult(fmt.Sprintf("jq filter error: %v", err))
		}
	}

	return NewToolResult(result)
}

func (t *JqTool) executeJqCLI(ctx context.Context, input, filter string, rawOutput, compact bool) (string, error) {
	jqArgs := []string{filter}
	if rawOutput {
		jqArgs = append([]string{"-r"}, jqArgs...)
	}
	if compact {
		jqArgs = append([]string{"-c"}, jqArgs...)
	}

	result := ExecuteCLICommand(CLICommandInput{
		Ctx:         ctx,
		BinaryPath:  "jq",
		Args:        jqArgs,
		Timeout:     10 * time.Second,
		ToolName:    "jq",
		InstallHint: "Install jq: brew install jq",
	})

	if result.IsError {
		return "", fmt.Errorf("jq: %s", result.ForLLM)
	}
	return result.ForLLM, nil
}

// evaluateSimpleJq handles basic jq expressions in pure Go as a fallback.
func evaluateSimpleJq(input any, filter string) (string, error) {
	filter = strings.TrimSpace(filter)

	// Identity
	if filter == "." {
		return formatJSON(input, false)
	}

	// Simple field access: .field or .field.subfield
	if strings.HasPrefix(filter, ".") && !strings.Contains(filter, "|") &&
		!strings.Contains(filter, "[") && !strings.Contains(filter, " ") {
		parts := strings.Split(filter[1:], ".")
		current := input
		for _, part := range parts {
			if part == "" {
				continue
			}
			obj, ok := current.(map[string]any)
			if !ok {
				return "", fmt.Errorf("cannot index %T with string %q", current, part)
			}
			current, ok = obj[part]
			if !ok {
				return "null", nil
			}
		}
		return formatJSON(current, false)
	}

	// keys
	if filter == "keys" || filter == ". | keys" {
		obj, ok := input.(map[string]any)
		if !ok {
			return "", fmt.Errorf("cannot get keys of %T", input)
		}
		keys := make([]string, 0, len(obj))
		for k := range obj {
			keys = append(keys, k)
		}
		return formatJSON(keys, false)
	}

	// length
	if filter == "length" || filter == ". | length" {
		switch v := input.(type) {
		case []any:
			return fmt.Sprintf("%d", len(v)), nil
		case map[string]any:
			return fmt.Sprintf("%d", len(v)), nil
		case string:
			return fmt.Sprintf("%d", len(v)), nil
		default:
			return "", fmt.Errorf("cannot get length of %T", input)
		}
	}

	// type
	if filter == "type" || filter == ". | type" {
		switch input.(type) {
		case map[string]any:
			return `"object"`, nil
		case []any:
			return `"array"`, nil
		case string:
			return `"string"`, nil
		case float64:
			return `"number"`, nil
		case bool:
			return `"boolean"`, nil
		case nil:
			return `"null"`, nil
		default:
			return fmt.Sprintf(`"%T"`, input), nil
		}
	}

	return "", fmt.Errorf("expression too complex for built-in evaluator, install jq for full support")
}

func formatJSON(v any, compact bool) (string, error) {
	var data []byte
	var err error
	if compact {
		data, err = json.Marshal(v)
	} else {
		data, err = json.MarshalIndent(v, "", "  ")
	}
	if err != nil {
		return "", err
	}
	return string(data), nil
}
