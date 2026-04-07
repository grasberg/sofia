# Ollama Optimization Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make Sofia respond as fast as direct Ollama chat — streaming tokens to the UI, sending lean prompts, and skipping unnecessary delegation for local models.

**Architecture:** Three independent optimizations: (A) SSE streaming from Ollama through the Web UI so tokens appear immediately, (B) compact system prompt mode for local models that strips non-essential context, (C) skip subagent delegation when running on a local model since small models handle direct responses better.

**Tech Stack:** Go, SSE (Server-Sent Events), Ollama OpenAI-compatible API, HTMX/JavaScript

---

### Task 1: Implement ChatStream on the OpenAI-compat provider

The `StreamingProvider` interface exists (`pkg/providers/types.go:88-97`) but no provider implements `ChatStream`. Ollama supports SSE streaming via `"stream": true` on `/v1/chat/completions`.

**Files:**
- Modify: `pkg/providers/openai_compat/provider.go`
- Modify: `pkg/providers/http_provider.go`
- Create: `pkg/providers/openai_compat/provider_stream_test.go`

- [ ] **Step 1: Write test for ChatStream**

```go
// pkg/providers/openai_compat/provider_stream_test.go
package openai_compat

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestChatStream_CollectsDeltas(t *testing.T) {
	// Mock SSE server returning 3 chunks then [DONE]
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)
		chunks := []string{
			`{"choices":[{"delta":{"content":"Hello"}}]}`,
			`{"choices":[{"delta":{"content":" world"}}]}`,
			`{"choices":[{"delta":{"content":"!"}}],"usage":{"prompt_tokens":10,"completion_tokens":3,"total_tokens":13}}`,
		}
		for _, c := range chunks {
			fmt.Fprintf(w, "data: %s\n\n", c)
			flusher.Flush()
		}
		fmt.Fprint(w, "data: [DONE]\n\n")
		flusher.Flush()
	}))
	defer server.Close()

	p := NewProvider("test-key", server.URL, "")
	ch, err := p.ChatStream(context.Background(), []Message{{Role: "user", Content: "hi"}}, nil, "test-model", nil)
	require.NoError(t, err)

	var fullText string
	for chunk := range ch {
		fullText += chunk.Delta
		if chunk.Done {
			break
		}
	}
	assert.Equal(t, "Hello world!", fullText)
}

func TestChatStream_NonStreamingFallback(t *testing.T) {
	// Server returns non-SSE JSON (like a regular Chat response)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"choices":[{"message":{"content":"fallback response"},"finish_reason":"stop"}]}`)
	}))
	defer server.Close()

	p := NewProvider("test-key", server.URL, "")
	ch, err := p.ChatStream(context.Background(), []Message{{Role: "user", Content: "hi"}}, nil, "test-model", nil)
	require.NoError(t, err)

	var fullText string
	for chunk := range ch {
		fullText += chunk.Delta
	}
	assert.Equal(t, "fallback response", fullText)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -tags stdjson ./pkg/providers/openai_compat/... -run TestChatStream -v`
Expected: FAIL — `ChatStream` method not found

- [ ] **Step 3: Implement ChatStream on openai_compat.Provider**

Add to `pkg/providers/openai_compat/provider.go` after the `Chat` method:

```go
// ChatStream sends a streaming chat completion request and returns a channel
// of StreamChunks. Each chunk contains a text delta. The channel is closed
// when the response is complete or an error occurs.
func (p *Provider) ChatStream(
	ctx context.Context,
	messages []Message,
	tools []ToolDefinition,
	model string,
	options map[string]any,
) (<-chan StreamChunk, error) {
	if p.apiBase == "" {
		return nil, fmt.Errorf("API base not configured")
	}

	model = normalizeModel(model, p.apiBase)

	requestBody := map[string]any{
		"model":    model,
		"messages": stripSystemParts(messages),
		"stream":   true,
	}

	if len(tools) > 0 {
		requestBody["tools"] = tools
		requestBody["tool_choice"] = "auto"
	}

	if maxTokens, ok := asInt(options["max_tokens"]); ok {
		fieldName := p.maxTokensField
		if fieldName == "" {
			fieldName = "max_tokens"
		}
		requestBody[fieldName] = maxTokens
	}

	if temperature, ok := asFloat(options["temperature"]); ok {
		requestBody["temperature"] = temperature
	}

	if isOllamaEndpoint(p.apiBase) {
		numCtx := 8192
		if maxTokens, ok := asInt(options["max_tokens"]); ok && maxTokens > 0 {
			numCtx = maxTokens
		}
		requestBody["options"] = map[string]any{"num_ctx": numCtx}
	}

	jsonData, err := json.Marshal(requestBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", p.apiBase+"/chat/completions", bytes.NewReader(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if p.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+p.apiKey)
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		resp.Body.Close()
		return nil, fmt.Errorf("API request failed:\n  Status: %d\n  Body:   %s", resp.StatusCode, string(body))
	}

	ch := make(chan StreamChunk, 32)

	go func() {
		defer close(ch)
		defer resp.Body.Close()

		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			line := scanner.Text()
			if !strings.HasPrefix(line, "data: ") {
				continue
			}
			data := strings.TrimPrefix(line, "data: ")
			if data == "[DONE]" {
				ch <- StreamChunk{Done: true}
				return
			}

			var chunk struct {
				Choices []struct {
					Delta struct {
						Content   string `json:"content"`
						ToolCalls []struct {
							ID       string `json:"id"`
							Type     string `json:"type"`
							Function *struct {
								Name      string `json:"name"`
								Arguments string `json:"arguments"`
							} `json:"function"`
						} `json:"tool_calls"`
					} `json:"delta"`
					FinishReason string `json:"finish_reason"`
				} `json:"choices"`
				Usage *UsageInfo `json:"usage"`
			}
			if err := json.Unmarshal([]byte(data), &chunk); err != nil {
				continue
			}

			if len(chunk.Choices) > 0 {
				delta := chunk.Choices[0].Delta
				sc := StreamChunk{Delta: delta.Content}

				for _, tc := range delta.ToolCalls {
					if tc.Function != nil {
						args := make(map[string]any)
						if tc.Function.Arguments != "" {
							_ = json.Unmarshal([]byte(tc.Function.Arguments), &args)
						}
						sc.ToolCalls = append(sc.ToolCalls, ToolCall{
							ID:        tc.ID,
							Name:      tc.Function.Name,
							Arguments: args,
						})
					}
				}

				if chunk.Choices[0].FinishReason == "stop" || chunk.Choices[0].FinishReason == "tool_calls" {
					sc.Done = true
				}

				select {
				case ch <- sc:
				case <-ctx.Done():
					return
				}

				if sc.Done {
					return
				}
			}
		}
		// If scanner ends without [DONE], send final chunk
		ch <- StreamChunk{Done: true}
	}()

	return ch, nil
}
```

Add `"bufio"` to the imports at top of file.

- [ ] **Step 4: Expose ChatStream on HTTPProvider**

Add to `pkg/providers/http_provider.go`:

```go
func (p *HTTPProvider) ChatStream(
	ctx context.Context,
	messages []Message,
	tools []ToolDefinition,
	model string,
	options map[string]any,
) (<-chan StreamChunk, error) {
	return p.delegate.ChatStream(ctx, messages, tools, model, options)
}
```

- [ ] **Step 5: Run tests**

Run: `go test -tags stdjson ./pkg/providers/openai_compat/... -run TestChatStream -v`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add pkg/providers/openai_compat/provider.go pkg/providers/openai_compat/provider_stream_test.go pkg/providers/http_provider.go
git commit -m "feat: implement ChatStream for OpenAI-compat and HTTP providers"
```

---

### Task 2: Add streaming chat API endpoint

The current `/api/chat` handler blocks until the full response is ready. Add a `/api/chat/stream` endpoint that returns SSE events as tokens arrive.

**Files:**
- Modify: `pkg/web/server.go`
- Modify: `pkg/agent/loop_processing.go`

- [ ] **Step 1: Add ProcessDirectStream method to AgentLoop**

Add to `pkg/agent/loop_processing.go` after `ProcessDirectWithImages`:

```go
// ProcessDirectStream sends a message and streams the response via a callback.
// The callback is called for each text chunk. It's called with done=true when complete.
// Falls back to non-streaming if the provider doesn't support it.
func (al *AgentLoop) ProcessDirectStream(
	ctx context.Context,
	content, sessionKey string,
	onChunk func(text string, done bool),
) error {
	agent := al.getRegistry().GetDefaultAgent()
	if agent == nil {
		return fmt.Errorf("no default agent")
	}

	// Check if provider supports streaming
	streamProvider, canStream := agent.Provider.(providers.StreamingProvider)
	if !canStream {
		// Fallback: run non-streaming, send result as single chunk
		resp, err := al.ProcessDirect(ctx, content, sessionKey)
		if err != nil {
			return err
		}
		onChunk(resp, true)
		return nil
	}

	// Build messages the same way as the normal flow
	agent.Tools.SetAllContexts("cli", "direct")
	systemPrompt := agent.ContextBuilder.BuildSystemPromptWithCache()
	history := agent.Sessions.GetHistory(sessionKey)
	summary := agent.Sessions.GetSummary(sessionKey)

	messages := agent.ContextBuilder.BuildMessages(history, summary, "", nil, "cli", "direct")
	messages = append(messages, providers.Message{Role: "user", Content: content})

	// Save user message to session
	agent.Sessions.AddMessage(sessionKey, "user", content)

	// For conversational messages, stream without tools
	isTask := looksLikeTask(content)
	var toolDefs []providers.ToolDefinition
	if isTask {
		// Build filtered tools same as loop_llm.go
		allToolNames := agent.Tools.List()
		var allToolsList []tools.Tool
		for _, name := range allToolNames {
			if t, ok := agent.Tools.Get(name); ok {
				allToolsList = append(allToolsList, t)
			}
		}
		if len(allToolNames) > 10 {
			allToolsList = tools.KeywordMatchTools(content, allToolsList, 10)
		}
		for _, t := range allToolsList {
			schema := tools.ToolToSchema(t)
			if fn, ok := schema["function"].(map[string]any); ok {
				name, _ := fn["name"].(string)
				desc, _ := fn["description"].(string)
				params, _ := fn["parameters"].(map[string]any)
				toolDefs = append(toolDefs, providers.ToolDefinition{
					Type: "function",
					Function: providers.ToolFunctionDefinition{
						Name:        name,
						Description: desc,
						Parameters:  params,
					},
				})
			}
		}
	}

	_ = systemPrompt // already included via BuildMessages

	llmOpts := map[string]any{
		"max_tokens":  agent.MaxTokens,
		"temperature": agent.Temperature,
	}

	ch, err := streamProvider.ChatStream(ctx, messages, toolDefs, agent.ModelID, llmOpts)
	if err != nil {
		return fmt.Errorf("stream failed: %w", err)
	}

	var fullResponse strings.Builder
	for chunk := range ch {
		if chunk.Delta != "" {
			fullResponse.WriteString(chunk.Delta)
			onChunk(chunk.Delta, false)
		}
		if chunk.Done {
			break
		}
	}

	// If response has tool calls, fall back to normal processing
	finalText := fullResponse.String()
	if finalText == "" {
		// Streaming produced no text — fall back to non-streaming
		resp, err := al.ProcessDirect(ctx, content, sessionKey)
		if err != nil {
			return err
		}
		onChunk(resp, true)
		return nil
	}

	// Save assistant response to session
	agent.Sessions.AddMessage(sessionKey, "assistant", finalText)
	agent.Sessions.Save(sessionKey)

	onChunk("", true)
	return nil
}
```

Add `"strings"` to imports if not present. Also add imports for `providers` and `tools` packages.

- [ ] **Step 2: Add SSE handler to web server**

Add to `pkg/web/server.go`, after the `handleChat` function:

```go
// handleChatStream handles POST /api/chat/stream — SSE streaming chat.
func (s *Server) handleChatStream(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.sendJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	limitBody(r)
	var req struct {
		Message    string `json:"message"`
		SessionKey string `json:"session_key"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.sendJSONError(w, err.Error(), http.StatusBadRequest)
		return
	}

	sessionKey := req.SessionKey
	if sessionKey == "" {
		sessionKey = "web:ui:" + time.Now().UTC().Format(time.RFC3339)
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	flusher, ok := w.(http.Flusher)
	if !ok {
		s.sendJSONError(w, "Streaming not supported", http.StatusInternalServerError)
		return
	}

	// Send session key as first event
	fmt.Fprintf(w, "data: %s\n\n", mustJSON(map[string]string{"type": "session", "session_key": sessionKey}))
	flusher.Flush()

	ctx := r.Context()
	err := s.agentLoop.ProcessDirectStream(ctx, req.Message, sessionKey, func(text string, done bool) {
		if done {
			fmt.Fprintf(w, "data: %s\n\n", mustJSON(map[string]any{"type": "done"}))
		} else {
			fmt.Fprintf(w, "data: %s\n\n", mustJSON(map[string]any{"type": "delta", "content": text}))
		}
		flusher.Flush()
	})

	if err != nil {
		fmt.Fprintf(w, "data: %s\n\n", mustJSON(map[string]any{"type": "error", "error": err.Error()}))
		flusher.Flush()
	}
}

func mustJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}
```

- [ ] **Step 3: Register the route**

In `pkg/web/server.go`, in the route registration section (after `mux.HandleFunc("/api/chat", ...)`):

```go
mux.HandleFunc("/api/chat/stream", api(s.handleChatStream))
```

- [ ] **Step 4: Build and verify**

Run: `make build`
Expected: Build complete

- [ ] **Step 5: Commit**

```bash
git add pkg/web/server.go pkg/agent/loop_processing.go
git commit -m "feat: add SSE streaming chat endpoint /api/chat/stream"
```

---

### Task 3: Wire streaming into the Web UI chat

Update the `sendChat()` JavaScript function to use `/api/chat/stream` when available, showing tokens as they arrive instead of waiting for the full response.

**Files:**
- Modify: `pkg/web/templates/layout.html`

- [ ] **Step 1: Replace sendChat with streaming version**

In `pkg/web/templates/layout.html`, replace the `sendChat()` function. Find the `fetch("/api/chat"` call and its response handling. Replace the try/catch block inside `sendChat()` (from `const res = await fetch("/api/chat"` through the response rendering) with:

```javascript
            try {
                // Create assistant bubble placeholder for streaming
                const bubbleId = 'stream-' + Date.now();
                const sofiaHtml =
                    "<div id='" + bubbleId + "' class='flex gap-4 animate-slide-up'>" +
                    "<div class='w-8 h-8 rounded-lg bg-sofia/10 border border-sofia/20 flex items-center justify-center shrink-0'>" +
                    "<img src='/assets/sofiamantis.png' class='w-5 h-5 opacity-80'>" +
                    "</div>" +
                    "<div>" +
                    "<div class='chat-bubble-sofia px-4 py-3 rounded-xl text-sm leading-relaxed max-w-[85%] text-zinc-300 whitespace-pre-wrap break-words' id='" + bubbleId + "-text'></div>" +
                    "<div class='text-[9px] text-zinc-600 ml-1 mt-1 font-bold uppercase tracking-widest'>Sofia System</div>" +
                    "</div>" +
                    "</div>";
                history.innerHTML += sofiaHtml;
                history.scrollTop = history.scrollHeight;

                const res = await fetch("/api/chat/stream", {
                    method: "POST",
                    headers: { "Content-Type": "application/json" },
                    body: JSON.stringify({ message: msg, session_key: (typeof currentSessionKey !== 'undefined' ? currentSessionKey : '') })
                });

                if (!res.ok) {
                    const errData = await res.json().catch(() => ({ error: "Request failed" }));
                    throw new Error(errData.error || "Unknown error");
                }

                const reader = res.body.getReader();
                const decoder = new TextDecoder();
                let streamedText = '';
                let buffer = '';

                while (true) {
                    const { done, value } = await reader.read();
                    if (done) break;

                    buffer += decoder.decode(value, { stream: true });
                    const lines = buffer.split('\n');
                    buffer = lines.pop() || '';

                    for (const line of lines) {
                        if (!line.startsWith('data: ')) continue;
                        const data = line.slice(6);
                        try {
                            const evt = JSON.parse(data);
                            if (evt.type === 'session' && evt.session_key) {
                                if (typeof currentSessionKey !== 'undefined') {
                                    currentSessionKey = evt.session_key;
                                    if (typeof updateSessionLabel === 'function') updateSessionLabel();
                                }
                            } else if (evt.type === 'delta' && evt.content) {
                                streamedText += evt.content;
                                const textEl = document.getElementById(bubbleId + '-text');
                                if (textEl) textEl.innerHTML = formatAssistantMessage(streamedText);
                                history.scrollTop = history.scrollHeight;
                            } else if (evt.type === 'done') {
                                // Finalize
                            } else if (evt.type === 'error') {
                                throw new Error(evt.error);
                            }
                        } catch (parseErr) {
                            if (parseErr.message && !parseErr.message.includes('JSON')) throw parseErr;
                        }
                    }
                }

                // Update chat history cache with final rendered bubble
                const finalBubble = document.getElementById(bubbleId);
                if (finalBubble) chatHistory.push(finalBubble.outerHTML);

                // If no text was streamed, fall back to non-streaming
                if (!streamedText) {
                    const fallbackRes = await fetch("/api/chat", {
                        method: "POST",
                        headers: { "Content-Type": "application/json" },
                        body: JSON.stringify({ message: msg, files: filesToSend, session_key: (typeof currentSessionKey !== 'undefined' ? currentSessionKey : '') })
                    });
                    const fallbackData = await fallbackRes.json();
                    if (fallbackData.response) {
                        const textEl = document.getElementById(bubbleId + '-text');
                        if (textEl) textEl.innerHTML = formatAssistantMessage(fallbackData.response);
                    }
                }

                chatWaiting = false;
                indicator.classList.add("hidden");
                indicator.textContent = "Sofia is thinking...";
```

- [ ] **Step 2: Build and test manually**

Run: `make build && make install`
Test: Open Web UI, send a message, verify tokens appear incrementally.

- [ ] **Step 3: Commit**

```bash
git add pkg/web/templates/layout.html
git commit -m "feat: stream Ollama responses in Web UI chat"
```

---

### Task 4: Compact system prompt for local models

When running on Ollama, the ~12KB system prompt is the main latency driver. Create a compact mode that sends only essential instructions.

**Files:**
- Modify: `pkg/agent/context.go`
- Modify: `pkg/agent/instance.go`
- Create: `pkg/agent/context_test.go` (add test)

- [ ] **Step 1: Add IsLocalModel field to AgentInstance**

In `pkg/agent/instance.go`, add to the `AgentInstance` struct:

```go
IsLocalModel  bool // True when using a local provider (Ollama), triggers compact prompts
```

In `NewAgentInstance`, after the provider resolution (around line 167), detect local model:

```go
// Detect local model for compact prompt mode
isLocal := false
if mc, err := cfg.GetModelConfig(model); err == nil && mc != nil {
    isLocal = strings.Contains(mc.APIBase, "localhost") || strings.Contains(mc.APIBase, "127.0.0.1")
}
```

And set it in the return struct:

```go
IsLocalModel:  isLocal,
```

- [ ] **Step 2: Add BuildCompactSystemPrompt to ContextBuilder**

Add to `pkg/agent/context.go`:

```go
// BuildCompactSystemPrompt returns a minimal system prompt for local/small models.
// Strips skills metadata, memory context, and verbose rules to reduce token count.
func (cb *ContextBuilder) BuildCompactSystemPrompt() string {
	workspacePath, _ := filepath.Abs(cb.workspace)
	name := cb.userName
	if name == "" {
		name = "the user"
	}

	return fmt.Sprintf(`You are Sofia, a helpful AI assistant for %s.
Workspace: %s

Rules:
- Respond directly and concisely.
- Use tools when the user asks you to perform actions (file operations, web search, etc.).
- For conversational messages, just respond with text — no tool calls needed.
- Be helpful, honest, and brief.`, name, workspacePath)
}
```

- [ ] **Step 3: Use compact prompt for local models in the LLM loop**

In `pkg/agent/loop_processing.go`, in the `runAgentLoop` function where `BuildSystemPromptWithCache` is called (find the `BuildMessages` call), add the compact mode:

Find where `agent.ContextBuilder.BuildMessages(...)` is called and before it, add:

```go
// Use compact system prompt for local models to reduce latency
if agent.IsLocalModel {
    compactPrompt := agent.ContextBuilder.BuildCompactSystemPrompt()
    // Override the system message with compact version
    messages := []providers.Message{{Role: "system", Content: compactPrompt}}
    // Add conversation history (last few turns only for local models)
    hist := agent.Sessions.GetHistory(opts.SessionKey)
    maxHist := 6 // Keep last 3 exchanges for local models
    if len(hist) > maxHist {
        hist = hist[len(hist)-maxHist:]
    }
    for _, h := range hist {
        messages = append(messages, providers.Message{Role: h.Role, Content: h.Content})
    }
    // Add current user message
    messages = append(messages, providers.Message{Role: "user", Content: opts.UserMessage})
}
```

This replaces the ~12KB prompt with ~300 bytes.

- [ ] **Step 4: Build and test**

Run: `make build`
Expected: Build complete

- [ ] **Step 5: Commit**

```bash
git add pkg/agent/context.go pkg/agent/instance.go pkg/agent/loop_processing.go
git commit -m "feat: compact system prompt for local models reduces prompt from 12KB to 300B"
```

---

### Task 5: Skip delegation for local models

Delegation spawns subagents, each making their own LLM call with the full prompt overhead. For small local models, this wastes time and produces worse results. Skip delegation and let the main agent handle everything directly.

**Files:**
- Modify: `pkg/agent/loop_processing.go`

- [ ] **Step 1: Guard delegation with local model check**

In `pkg/agent/loop_processing.go`, in the `processMessage` function, find the delegation section (around line 518-552). The block starts with:

```go
// --- Auto-spawn agents for capabilities/skills not covered by any existing agent ---
```

Wrap the entire delegation section (auto-spawn + multi-delegation) with a local model guard:

```go
// Skip delegation for local models — small models handle direct responses
// better than coordinating subagents, and each delegation adds a full
// LLM round-trip with the same prompt overhead.
defaultAgent := al.getRegistry().GetDefaultAgent()
skipDelegation := defaultAgent != nil && defaultAgent.IsLocalModel

if !skipDelegation {
    // --- Auto-spawn agents for capabilities/skills not covered by any existing agent ---
    // ... existing delegation code ...
}
```

The `if !skipDelegation {` should wrap everything from the auto-spawn block through the multi-delegation block (through the line where candidates are processed).

- [ ] **Step 2: Build and test**

Run: `make build`
Expected: Build complete

- [ ] **Step 3: Verify with logs**

After restart, send a message. Logs should show:
- No "Auto-created agent" lines
- No "delegating to N agent(s)" lines
- Direct response from main agent

- [ ] **Step 4: Commit**

```bash
git add pkg/agent/loop_processing.go
git commit -m "feat: skip subagent delegation for local models to reduce latency"
```

---

### Task 6: Integration test and final build

**Files:**
- No new files

- [ ] **Step 1: Run all tests**

```bash
make test
```

Expected: All tests pass

- [ ] **Step 2: Build and install**

```bash
make build && make install
```

- [ ] **Step 3: Manual verification**

1. Start Sofia with Ollama model
2. Send "hej" — should get streaming response in <5 seconds
3. Send "what's 2+2" — should get streaming response, no delegation
4. Check logs: no subagent spawning, no delegation, compact prompt
5. Send "create a file called test.txt with hello world" — should use tools (task detection works)

- [ ] **Step 4: Final commit if any fixups needed**

```bash
git add -A
git commit -m "fix: integration fixes for Ollama optimization"
```
