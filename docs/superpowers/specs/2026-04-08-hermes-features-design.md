# Hermes-Agent Feature Ports — Design Spec

**Date:** 2026-04-08  
**Source:** NousResearch/hermes-agent v0.2.0–v0.7.0 release notes  
**Scope:** 7 low-to-medium effort features worth porting into Sofia

---

## Context

After reviewing all hermes-agent releases, several features stood out as high-value and not yet present in Sofia. Features already implemented in Sofia (stale file detection, pre-exec scanning, approval gate) were excluded. Features requiring major architectural changes (pluggable memory providers, profile system) were deferred. The 7 features below are focused, bounded, and independently implementable.

---

## Feature 1: Inline Diff Previews

### Problem
`EditFileTool` and `WriteFileTool` return bare confirmation strings like `"File edited: /path"`. Neither the user nor the LLM can see what actually changed without re-reading the file.

### Design
- `go-difflib` is already an indirect dependency via testify — promote to direct use.
- In `EditFileTool.Run()` (`pkg/tools/edit.go`): after applying the replacement, generate a unified diff of original→modified content and append it to the tool result.
- In `WriteFileTool.Run()` (`pkg/tools/filesystem.go`): read existing file content before overwriting (if file exists), generate diff, append to result. If file is new, return a `+++ (new file)` header.
- Diff format: standard unified diff with 3 lines of context, capped at 100 lines total (truncated with a `... N more lines omitted` trailer if longer).
- The diff is returned as part of the tool result string, visible to both the LLM and the user.

### Key files
- `pkg/tools/edit.go` — `EditFileTool.Run()`
- `pkg/tools/filesystem.go` — `WriteFileTool.Run()`
- `go.mod` — add `github.com/pmezard/go-difflib` as direct dep

---

## Feature 2: Session Compression Config

### Problem
All summarization thresholds in `loop_summarize.go` are hardcoded (75%/90% context triggers, protected head/tail sizes, tool result truncation limit). There is no way to tune these per-agent or globally without editing source.

### Design
Add a `SummarizationConfig` struct to `pkg/config/config.go`, nested under the agent defaults:

```go
type SummarizationConfig struct {
    ContextTriggerPct      int `json:"context_trigger_pct,omitempty"`       // default 75
    ForceTriggerPct        int `json:"force_trigger_pct,omitempty"`          // default 90
    ProtectHead            int `json:"protect_head,omitempty"`               // default 2
    ProtectTailPct         int `json:"protect_tail_pct,omitempty"`           // default 30
    MinTail                int `json:"min_tail,omitempty"`                   // default 4
    ToolResultTruncateChars int `json:"tool_result_truncate_chars,omitempty"` // default 200
}
```

- Add `Summarization SummarizationConfig` to `AgentDefaults` and `AgentConfig` (per-agent override).
- In `loop_summarize.go`, replace each hardcoded constant with a lookup: `agent.Summarization.ContextTriggerPct` falling back to the default if zero.
- `AgentInstance` already has access to agent config — no new wiring needed.

### Key files
- `pkg/config/config.go` — add `SummarizationConfig`, embed in defaults + per-agent
- `pkg/agent/loop_summarize.go` — replace all hardcoded thresholds

---

## Feature 3: `/yolo` Approval Toggle

### Problem
`ApprovalGate` has a rich config structure but there is no runtime way to bypass approval mid-session without restarting. Hermes uses `/yolo` for this.

### Design
- Add `approvalBypass sync.Map` to `ApprovalGate` in `pkg/agent/approval.go`.
- At the top of `RequiresApproval()`, check: if `approvalBypass.Load(sessionKey)` is true, return `false`.
- Expose `SetBypass(sessionKey string, on bool)` method on `ApprovalGate`.
- Add `/yolo [on|off]` to `loop_commands.go` following the same pattern as `/verbose`:
  - No args: report current state.
  - `on`: call `al.approvalGate.SetBypass(sessionKey, true)`, return confirmation.
  - `off`: call `al.approvalGate.SetBypass(sessionKey, false)`, return confirmation.
- Bypass is session-scoped and non-persistent (resets on session restart).

### Key files
- `pkg/agent/approval.go` — `approvalBypass sync.Map`, `SetBypass()`, check in `RequiresApproval()`
- `pkg/agent/loop_commands.go` — `/yolo` handler
- `pkg/agent/loop.go` — confirm `AgentLoop` holds a reference to `ApprovalGate`

---

## Feature 4: `/btw` Ephemeral Questions

### Problem
Users sometimes want a quick side answer without polluting the session history (e.g., "what does this function do?" mid-task). Currently any message extends the history.

### Design
- Supported in CLI and Web channels only. In gateway mode, `/btw` is treated as a regular message.
- Detection: in `handleCommand()` in `loop_commands.go`, intercept `/btw <question>`. Extract the trailing text as the question.
- Pass an `ephemeral bool` field through `processMessageOpts` (or equivalent options struct) to `runLLMIteration` in `loop_llm.go`.
- In `runLLMIteration`, when `opts.Ephemeral` is true:
  - Build messages normally (using full session history as context).
  - After the LLM responds, **skip** `agent.Sessions.AddFullMessage()` for both the assistant message and any tool results.
  - Skip the summarization trigger check.
  - Prepend `[btw] ` to the response so the user can distinguish it visually.
- The session history is unchanged after the exchange.

### Key files
- `pkg/agent/loop_commands.go` — `/btw` detection and dispatch
- `pkg/agent/loop_llm.go` — `opts.Ephemeral` flag, conditional `AddFullMessage()` skip
- `pkg/agent/loop.go` — `processMessageOpts` struct update (or wherever opts are threaded)

---

## Feature 5: `@file` / `@url` Context Injection

### Problem
Users have no way to inline file contents or URLs into a message without using a separate tool call. Hermes supports `@/path` and `@https://url` references anywhere in message text.

### Design
- New function `enrichMessageContent(content string, workspacePath string) string` in a new file `pkg/agent/context_refs.go`.
- Called in `processMessage()` in `loop_processing.go` after guardrails (secret scrubbing + PII redaction) but before routing — so the enriched content reaches the LLM.
- **Parsing:** regex `@(\./[^\s]+|/[^\s]+|https?://[^\s]+)` finds all references inline.
- **File references** (`@/abs/path` or `@./rel/path`):
  - Resolve relative paths against the agent's workspace root.
  - Reject any path that escapes the workspace (path traversal guard) — leave the token as-is.
  - Read up to 50 KB; if larger, truncate with a trailer.
  - Replace the token with a fenced block: `` `\n```\n<content>\n```\n` ``
- **URL references** (`@https://...`):
  - Reuse the existing `WebFetchTool` fetch/markdown conversion logic.
  - Respect the same SSRF blocklist already enforced by the web tools.
  - Cap at 50 KB.
- **Limits:** max 5 references per message. Extras are left as-is.
- Errors (file not found, fetch failure) are noted inline: `[could not read @/path: file not found]`.

### Key files
- `pkg/agent/context_refs.go` — new file, `enrichMessageContent()`
- `pkg/agent/loop_processing.go` — call `enrichMessageContent()` after guardrails
- `pkg/tools/web_fetch.go` — reuse fetch logic (or extract a shared helper)

---

## Feature 6: Per-Model Output Limits

### Problem
`max_tokens` is set globally per agent. Model-specific limits (e.g., Anthropic's 128K cap on Opus 4.6, 64K on Sonnet 4.6) are not enforced. A misconfigured `max_tokens` can cause API errors.

### Design
- `ModelConfig.MaxTokens` already exists in `pkg/config/config_providers.go` but is not wired into the LLM call.
- In `loop_llm.go`, after building `llmOpts["max_tokens"] = agent.MaxTokens`, check if the resolved model config has a non-zero `MaxTokens` and use it as an override.
- Add a `provider_defaults.go` (already untracked in git) for Anthropic-specific defaults:
  - `claude-opus-4` family → 128K output cap
  - `claude-sonnet-4` family → 64K output cap
  - Applied only when `ModelConfig.MaxTokens` is zero (user-set value wins).
- **Anthropic 429 long-context handling:** when the Anthropic provider receives a 429 with `"long-context-tier"` in the error body, retry the request with `context_window` reduced to 200K. This goes in `pkg/providers/anthropic/provider.go`.

### Key files
- `pkg/agent/loop_llm.go` — wire `ModelConfig.MaxTokens` into `llmOpts`
- `pkg/config/provider_defaults.go` — already exists (untracked), add Anthropic model caps
- `pkg/providers/anthropic/provider.go` — 429 long-context retry

---

## Feature 7: Reasoning Block Preservation (Anthropic)

### Problem
The Anthropic provider's `parseResponse()` silently drops `thinking` content blocks — they are never parsed into `ReasoningContent`. Conversely, `buildParams()` never sends thinking blocks back in subsequent assistant messages. Extended thinking is therefore broken for multi-turn tool-use conversations.

### Design

**In `parseResponse()` (`pkg/providers/anthropic/provider.go`):**
- Add `case "thinking":` to the content block switch.
- Use `block.AsThinking()` to get the thinking block.
- Concatenate into `ReasoningContent` (same as how `content` accumulates text blocks).

**In `buildParams()` (`pkg/providers/anthropic/provider.go`):**
- For assistant messages where `msg.ReasoningContent != ""`, prepend a `ThinkingBlockParam` content block before the text/tool_use blocks.
- The Anthropic SDK provides `anthropic.ThinkingBlockParam{Type: "thinking", ThinkingText: msg.ReasoningContent}` (verify exact API from SDK).

**Enabling extended thinking:**
- Check `options["thinking_budget"]` (int). If non-zero, set `params.Thinking = anthropic.ThinkingParam{Type: "enabled", BudgetTokens: budget}` on the request params.
- Wire `thinking_budget` from agent config into `llmOpts` in `loop_llm.go` (alongside `max_tokens`).
- Add `ThinkingBudget int` to agent config (defaults to 0 = disabled).

**Note:** Requires the `anthropic-sdk-go` to expose `ThinkingBlockParam` — verify this exists in the current SDK version before implementing.

### Key files
- `pkg/providers/anthropic/provider.go` — `parseResponse()` and `buildParams()`
- `pkg/config/config.go` — `ThinkingBudget int` in agent config
- `pkg/agent/loop_llm.go` — pass `thinking_budget` in `llmOpts`

---

## Verification

For each feature, test as follows:

1. **Inline diffs** — Edit a file via the `edit_file` tool. Confirm the tool result contains a `---`/`+++` unified diff. Test with a new file (write_file) — confirm `+++ (new file)` header.

2. **Compression config** — Set `context_trigger_pct: 50` in agent config. Send messages until history grows. Confirm summarization triggers earlier than the default 75%.

3. **`/yolo`** — Call `/yolo on`. Perform a tool call that normally requires approval. Confirm it runs without prompting. Call `/yolo off` and confirm approval returns.

4. **`/btw`** — Send `/btw what is 2+2?`. Confirm a response is returned. Check session history — confirm the exchange was not stored.

5. **`@file`/`@url`** — Send `summarize @/path/to/file`. Confirm the file contents are inlined in the system message sent to the LLM. Test with a non-workspace path — confirm it's left as-is.

6. **Per-model limits** — Set `max_tokens: 200000` for an Opus agent. Add a unit test in `pkg/providers/anthropic/` that confirms the resolved `max_tokens` in `buildParams()` is capped at 128K regardless of the input value.

7. **Reasoning preservation** — Enable `thinking_budget: 8000` for an Anthropic agent. Run a multi-turn conversation with tool calls. Confirm thinking blocks appear in `/verbose` output on each turn, not just the first.
