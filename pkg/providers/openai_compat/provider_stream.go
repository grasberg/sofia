package openai_compat

import (
	"encoding/json"
	"strings"
)

// streamChunkPayload is the SSE frame shape for /chat/completions streaming.
// Broken out so both the main loop and the accumulator type can refer to the
// same struct without duplicating the nested literal.
type streamChunkPayload struct {
	Choices []struct {
		Delta struct {
			Content          string               `json:"content"`
			ReasoningContent string               `json:"reasoning_content"`
			ToolCalls        []streamToolCallPart `json:"tool_calls"`
		} `json:"delta"`
		FinishReason *string `json:"finish_reason"`
	} `json:"choices"`
	Usage *UsageInfo `json:"usage"`
}

// streamToolCallPart is a single tool_call entry inside a delta. OpenAI
// streams tool calls by index: the first chunk for an index carries the id
// and function name, subsequent chunks extend function.arguments one token
// at a time until the call is complete.
type streamToolCallPart struct {
	Index    int    `json:"index"`
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function *struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

// streamAccumulator assembles a streaming response into a final LLMResponse.
// Tool calls arrive keyed by "index" and need arguments concatenated across
// chunks before the JSON can be parsed, so we can't build ToolCall values
// eagerly — we stash raw fragments and materialise them in finalize().
type streamAccumulator struct {
	text         strings.Builder
	reasoning    strings.Builder
	finishReason string
	usage        *UsageInfo
	toolCalls    map[int]*streamToolCallState
	toolOrder    []int
}

type streamToolCallState struct {
	id       string
	name     string
	argsText strings.Builder
}

func newStreamAccumulator() *streamAccumulator {
	return &streamAccumulator{
		toolCalls: make(map[int]*streamToolCallState),
	}
}

func (a *streamAccumulator) applyToolCallDelta(part streamToolCallPart) {
	state, ok := a.toolCalls[part.Index]
	if !ok {
		state = &streamToolCallState{}
		a.toolCalls[part.Index] = state
		a.toolOrder = append(a.toolOrder, part.Index)
	}
	if part.ID != "" {
		state.id = part.ID
	}
	if part.Function != nil {
		if part.Function.Name != "" {
			state.name = part.Function.Name
		}
		if part.Function.Arguments != "" {
			state.argsText.WriteString(part.Function.Arguments)
		}
	}
}

func (a *streamAccumulator) finalize() *LLMResponse {
	resp := &LLMResponse{
		Content:          a.text.String(),
		ReasoningContent: a.reasoning.String(),
		FinishReason:     a.finishReason,
		Usage:            a.usage,
	}
	if resp.FinishReason == "" {
		resp.FinishReason = "stop"
	}
	if len(a.toolOrder) == 0 {
		return resp
	}
	calls := make([]ToolCall, 0, len(a.toolOrder))
	for _, idx := range a.toolOrder {
		state := a.toolCalls[idx]
		args := make(map[string]any)
		if raw := strings.TrimSpace(state.argsText.String()); raw != "" {
			if err := json.Unmarshal([]byte(raw), &args); err != nil {
				// Preserve the raw string when the model produced
				// malformed JSON — the agent loop surfaces a clearer
				// error upstream than a silent drop would.
				args["raw"] = raw
			}
		}
		calls = append(calls, ToolCall{
			ID:        state.id,
			Name:      state.name,
			Arguments: args,
		})
	}
	resp.ToolCalls = calls
	return resp
}
