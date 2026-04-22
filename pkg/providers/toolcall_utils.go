// Sofia - Ultra-lightweight personal AI agent
// License: MIT
//
// Copyright (c) 2026 Sofia contributors

package providers

import "encoding/json"

// NormalizeToolCall normalizes a ToolCall to ensure all fields are properly populated.
// It handles cases where Name/Arguments might be in different locations (top-level vs Function)
// and ensures both are populated consistently.
func NormalizeToolCall(tc ToolCall) ToolCall {
	normalized := tc

	if normalized.Name == "" && normalized.Function != nil {
		normalized.Name = normalized.Function.Name
	}

	if normalized.Arguments == nil {
		normalized.Arguments = map[string]any{}
	}

	if len(normalized.Arguments) == 0 && normalized.Function != nil && normalized.Function.Arguments != "" {
		var parsed map[string]any
		if err := json.Unmarshal([]byte(normalized.Function.Arguments), &parsed); err == nil && parsed != nil {
			normalized.Arguments = parsed
		}
	}

	if normalized.Function == nil {
		normalized.Function = &FunctionCall{
			Name:      normalized.Name,
			Arguments: marshalArgs(normalized.Arguments),
		}
	} else {
		if normalized.Function.Name == "" {
			normalized.Function.Name = normalized.Name
		}
		if normalized.Name == "" {
			normalized.Name = normalized.Function.Name
		}
		if normalized.Function.Arguments == "" {
			normalized.Function.Arguments = marshalArgs(normalized.Arguments)
		}
	}

	return normalized
}

// ToolCallArgumentsJSON returns the tool call's Arguments as a JSON string,
// reusing Function.Arguments when the normalizer already produced it so the
// hot tool-dispatch path avoids a redundant map→string marshal.
func ToolCallArgumentsJSON(tc ToolCall) string {
	if tc.Function != nil && tc.Function.Arguments != "" {
		return tc.Function.Arguments
	}
	return marshalArgs(tc.Arguments)
}

func marshalArgs(args map[string]any) string {
	b, _ := json.Marshal(args)
	return string(b)
}
