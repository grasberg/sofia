package providers

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

// QwenCliProvider implements LLMProvider by wrapping Qwen Code via acpx as a subprocess.
type QwenCliProvider struct {
	command   string
	workspace string
}

// NewQwenCliProvider creates a new Qwen CLI provider.
func NewQwenCliProvider(workspace string) *QwenCliProvider {
	return &QwenCliProvider{
		command:   "acpx",
		workspace: workspace,
	}
}

// Chat implements LLMProvider.Chat by executing Qwen Code via acpx in one-shot exec mode.
func (p *QwenCliProvider) Chat(
	ctx context.Context, messages []Message, tools []ToolDefinition, model string, options map[string]any,
) (*LLMResponse, error) {
	if p.command == "" {
		return nil, fmt.Errorf("acpx command not configured")
	}

	prompt := p.buildPrompt(messages, tools)

	args := []string{"--format", "json", "--approve-all"}
	if p.workspace != "" {
		args = append(args, "--cwd", p.workspace)
	}
	args = append(args, "qwen", "exec", "-f", "-") // read prompt from stdin

	cmd := exec.CommandContext(ctx, p.command, args...)
	cmd.Stdin = bytes.NewReader([]byte(prompt))

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()

	// Parse JSONL from stdout even if exit code is non-zero,
	// because acpx may write diagnostic info to stderr but still produce valid output.
	if stdoutStr := stdout.String(); stdoutStr != "" {
		resp, parseErr := p.parseJSONRPCEvents(stdoutStr)
		if parseErr == nil && resp != nil && (resp.Content != "" || len(resp.ToolCalls) > 0) {
			return resp, nil
		}
	}

	if err != nil {
		if ctx.Err() == context.Canceled {
			return nil, ctx.Err()
		}
		if stderrStr := stderr.String(); stderrStr != "" {
			return nil, fmt.Errorf("qwen cli error: %s", stderrStr)
		}
		return nil, fmt.Errorf("qwen cli error: %w", err)
	}

	return p.parseJSONRPCEvents(stdout.String())
}

// Embeddings implements EmbeddingProvider.
func (p *QwenCliProvider) Embeddings(
	ctx context.Context, texts []string, model string,
) ([]EmbeddingResult, error) {
	return nil, fmt.Errorf("embeddings not supported on QwenCliProvider")
}

// GetDefaultModel returns the default model identifier.
func (p *QwenCliProvider) GetDefaultModel() string {
	return "qwen-code"
}

// buildPrompt converts messages to a prompt string for Qwen Code.
func (p *QwenCliProvider) buildPrompt(messages []Message, tools []ToolDefinition) string {
	var systemParts []string
	var conversationParts []string

	for _, msg := range messages {
		switch msg.Role {
		case "system":
			systemParts = append(systemParts, msg.Content)
		case "user":
			conversationParts = append(conversationParts, msg.Content)
		case "assistant":
			conversationParts = append(conversationParts, "Assistant: "+msg.Content)
		case "tool":
			conversationParts = append(conversationParts,
				fmt.Sprintf("[Tool Result for %s]: %s", msg.ToolCallID, msg.Content))
		}
	}

	var sb strings.Builder

	if len(systemParts) > 0 {
		sb.WriteString("## System Instructions\n\n")
		sb.WriteString(strings.Join(systemParts, "\n\n"))
		sb.WriteString("\n\n## Task\n\n")
	}

	if len(tools) > 0 {
		sb.WriteString(p.buildToolsPrompt(tools))
		sb.WriteString("\n\n")
	}

	// Simplify single user message (no prefix)
	if len(conversationParts) == 1 && len(systemParts) == 0 && len(tools) == 0 {
		return conversationParts[0]
	}

	sb.WriteString(strings.Join(conversationParts, "\n"))
	return sb.String()
}

// buildToolsPrompt creates a tool definitions section for the prompt.
func (p *QwenCliProvider) buildToolsPrompt(tools []ToolDefinition) string {
	var sb strings.Builder

	sb.WriteString("## Available Tools\n\n")
	sb.WriteString("When you need to use a tool, respond with ONLY a JSON object:\n\n")
	sb.WriteString("```json\n")
	sb.WriteString(
		`{"tool_calls":[{"id":"call_xxx","type":"function","function":{"name":"tool_name","arguments":"{...}"}}]}`,
	)
	sb.WriteString("\n```\n\n")
	sb.WriteString("CRITICAL: The 'arguments' field MUST be a JSON-encoded STRING.\n\n")
	sb.WriteString("### Tool Definitions:\n\n")

	for _, tool := range tools {
		if tool.Type != "function" {
			continue
		}
		fmt.Fprintf(&sb, "#### %s\n", tool.Function.Name)
		if tool.Function.Description != "" {
			fmt.Fprintf(&sb, "Description: %s\n", tool.Function.Description)
		}
		if len(tool.Function.Parameters) > 0 {
			paramsJSON, _ := json.Marshal(tool.Function.Parameters)
			fmt.Fprintf(&sb, "Parameters:\n```json\n%s\n```\n", string(paramsJSON))
		}
		sb.WriteString("\n")
	}

	return sb.String()
}

// acpxEvent represents a single JSON-RPC message from acpx --format json.
type acpxEvent struct {
	JSONRPC string         `json:"jsonrpc"`
	ID      any            `json:"id,omitempty"`
	Method  string         `json:"method,omitempty"`
	Params  *acpxParams    `json:"params,omitempty"`
	Result  map[string]any `json:"result,omitempty"`
}

type acpxParams struct {
	SessionID string       `json:"sessionId,omitempty"`
	Update    *acpxUpdate  `json:"update,omitempty"`
}

type acpxUpdate struct {
	SessionUpdate string        `json:"sessionUpdate"`
	Content       *acpxContent  `json:"content,omitempty"`
	Meta          *acpxMeta     `json:"_meta,omitempty"`
}

type acpxContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type acpxMeta struct {
	Usage *acpxUsage `json:"usage,omitempty"`
}

type acpxUsage struct {
	InputTokens      int `json:"inputTokens"`
	OutputTokens     int `json:"outputTokens"`
	TotalTokens      int `json:"totalTokens"`
	ThoughtTokens    int `json:"thoughtTokens"`
	CachedReadTokens int `json:"cachedReadTokens"`
}

// parseJSONRPCEvents processes the JSON-RPC JSONL output from acpx --format json.
func (p *QwenCliProvider) parseJSONRPCEvents(output string) (*LLMResponse, error) {
	var contentParts []string
	var usage *UsageInfo
	var stopReason string

	scanner := bufio.NewScanner(strings.NewReader(output))
	// Increase scanner buffer for large responses
	scanner.Buffer(make([]byte, 0, 1024*1024), 1024*1024)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		var event acpxEvent
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			continue // skip malformed lines
		}

		// Handle session/update events
		if event.Method == "session/update" && event.Params != nil && event.Params.Update != nil {
			update := event.Params.Update

			switch update.SessionUpdate {
			case "agent_message_chunk":
				if update.Content != nil && update.Content.Text != "" {
					contentParts = append(contentParts, update.Content.Text)
				}
				// Final message chunk carries usage info in _meta
				if update.Meta != nil && update.Meta.Usage != nil {
					u := update.Meta.Usage
					usage = &UsageInfo{
						PromptTokens:     u.InputTokens,
						CompletionTokens: u.OutputTokens,
						TotalTokens:      u.TotalTokens,
					}
				}
			}
		}

		// Handle prompt result (stop reason)
		if event.ID != nil && event.Result != nil {
			if sr, ok := event.Result["stopReason"].(string); ok {
				stopReason = sr
			}
		}
	}

	if len(contentParts) == 0 {
		return nil, fmt.Errorf("qwen cli: no response content received")
	}

	content := strings.Join(contentParts, "")

	// Extract tool calls from response text (same pattern as other CLI providers)
	toolCalls := extractToolCallsFromText(content)

	finishReason := "stop"
	if len(toolCalls) > 0 {
		finishReason = "tool_calls"
		content = stripToolCallsFromText(content)
	} else if stopReason == "end_turn" {
		finishReason = "stop"
	}

	return &LLMResponse{
		Content:      strings.TrimSpace(content),
		ToolCalls:    toolCalls,
		FinishReason: finishReason,
		Usage:        usage,
	}, nil
}
