# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Commands

Prefer Makefile targets over raw `go` commands — they set required flags (`CGO_ENABLED=0`, `-tags stdjson`) and embed version info via ldflags.

```bash
# Build (runs go:generate first, then compiles with correct flags)
make build

# Run tests
make test

# Lint (golangci-lint v2, config in .golangci.yaml)
make lint

# Auto-fix lint issues
make fix

# Format (gci, gofmt, gofumpt, goimports, golines)
make fmt

# Install to ~/.local/bin
make install

# Run a single package's tests
go test ./pkg/providers/...

# Run a single test
go test ./pkg/providers/... -run TestFallback
```

Config is stored at `~/.sofia/config.json`. Runtime data in `~/.sofia/` (memory.db, cron/jobs.json, skills/). The binary is `sofia`.

## Architecture

Sofia is a personal AI agent gateway written in Go. It routes messages from multiple channels (Telegram, Discord, CLI, Web) to one or more configured LLM agents, then streams responses back.

### Top-level structure

- `cmd/sofia/` — CLI entrypoint using Cobra. Subcommands: `gateway`, `agent`, `cron`, `onboard`, `version`
- `pkg/` — All core logic as importable packages
- `workspace/` — Runtime workspace: `AGENT.md` (system prompt), `USER.md` (user profile), `skills/` (local skills)

### Core data flow

```
Channels → MessageBus → AgentLoop → AgentRegistry → AgentInstance → LLMProvider
                ↑                                         ↓
          OutboundMessage                           ToolRegistry
```

1. **Channels** (`pkg/channels/`) — Telegram, Discord, CLI, Web. Each implements the `Channel` interface, publishes `InboundMessage` to the bus and subscribes to `OutboundMessage`.
2. **MessageBus** (`pkg/bus/`) — Publish-subscribe hub with buffered channels (500 capacity). Ephemeral messages (thinking, stream_delta) drop silently under backpressure; content messages block up to 10s.
3. **AgentLoop** (`pkg/agent/loop.go`) — Central coordinator. Consumes inbound messages, routes to agents via `RouteResolver`, runs the LLM tool-use loop, publishes responses.
4. **AgentRegistry** (`pkg/agent/registry.go`) — Holds all configured `AgentInstance` objects. Routes messages using `routing.RouteResolver` (based on bindings in config).
5. **AgentInstance** (`pkg/agent/instance.go`) — Per-agent state: model config, `ToolRegistry`, `SessionManager`, `ContextBuilder`, fallback candidates.

### Agent loop internals

The loop is split across files by concern:

- `loop.go` — Core `Run()` loop, `AgentLoop` struct with all orchestration state (rate limiting, A2A router, plan manager, shared scratchpad, checkpoint manager, tool tracker)
- `loop_processing.go` — Message processing pipeline: guardrail checks → context building → session history → LLM iteration loop → summarization when history exceeds 20 messages or 75% of context window
- `loop_llm.go` — Single LLM iteration: semantic tool filtering (embeddings-based, when >10 tools), reflection checkpoints every N iterations, active plan injection, parallel tool execution
- `loop_tools.go` — Tool registration, sharing across agents, tool list construction
- `loop_commands.go` — Slash command handling (`/status`, `/show`, `/list`, `/switch`)
- `loop_guardrails.go` — Input validation, prompt injection detection via pre-compiled regex patterns
- `loop_query.go` — Query interface for session history, goals, agent monitoring metadata
- `loop_summarize.go` — Context summarization and forced compression (drops oldest 50% on overflow)
- `loop_helpers.go` — Tool context propagation, autonomy service lifecycle, subagent execution with reputation tracking

### Provider abstraction

`pkg/providers/` implements `LLMProvider` (single `Chat()` method). Optional interfaces: `StreamingProvider` (real-time output), `EmbeddingProvider` (semantic tool matching), `StatefulProvider` (cleanup).

Supported backends: OpenAI-compatible HTTP API (OpenAI, Anthropic, Groq, Gemini, OpenRouter, DeepSeek, Grok, etc.), Claude CLI, Codex CLI, GitHub Copilot.

Provider selection is resolved in `factory.go`: models can use protocol prefix (`openai/gpt-4o`), `model_list` aliases, or round-robin selection. `FallbackChain` (`fallback.go`) tries candidates in order with `CooldownTracker` and error classification (`error_classifier.go`) — format errors are non-retriable; auth/rate-limit/overloaded errors trigger fallback.

### Context builder (`pkg/agent/context.go`)

Builds the system prompt by assembling (in order): IDENTITY.md → SOUL.md → USER.md → AGENT.md → skill metadata → memory notes → purpose template → guardrail suffix. Files are cached with mtime-based invalidation.

Skills are loaded from three locations: `workspace/skills/`, `~/.sofia/skills/`, and builtin `./skills/`. Only metadata is included by default; full skill body loads on trigger.

### Skills system (`pkg/skills/`, `skills/`)

Skills are domain-expert personas loaded as `skills/{name}/SKILL.md` with YAML frontmatter (`name`, `description`). The loader (`pkg/skills/loader.go`) auto-discovers them from three locations in priority order: `workspace/skills/` > `~/.sofia/skills/` > builtin `./skills/`. Only metadata (name + description) goes into the system prompt; the full body loads on trigger. Skill names must match `^[a-zA-Z0-9]+(-[a-zA-Z0-9]+)*$`, descriptions max 1024 chars. The simple YAML parser only handles single-line `key: value` — no multiline block scalars.

### Config structure (`pkg/config/config.go`)

Key fields in `~/.sofia/config.json`:
- `agents.defaults` — workspace path, default model, max tokens, max tool iterations
- `agents.list` — per-agent overrides (id, name, model, workspace, template, skills filter, subagents)
- `bindings` — routes channel/peer patterns to specific agent IDs
- `channels` — Telegram/Discord token config
- `providers` — API keys per provider
- `model_list` — named model aliases with provider config
- `session` — DM scope, identity links across channels

Validation (`pkg/config/validate.go`) checks: max_tokens, temperature bounds (0–2), duplicate agent IDs, enabled-but-missing-token channels, binding consistency.

### Multi-agent coordination

- **A2A Router** — Mailbox-based agent-to-agent messaging with send/receive/broadcast
- **SharedScratchpad** — Key-value store for inter-agent data sharing
- **Subagent spawning** — Tool-based spawning with reputation tracking
- **AutonomyService** — Per-agent background goal tracking and proactive suggestions

### Tools (`pkg/tools/`)

Core tools: `read_file`, `write_file`, `list_dir`, `edit_file`, `append_file`, `shell`/`exec`, `image_analyze`, `web_search`, `web_fetch`, `web_browse` (Playwright), `spawn`/`subagent`, `a2a`, `plan`, `scratchpad`, `cron`, `message`, `mcp` (dynamic MCP tools), hardware I2C/SPI on Linux.

Tool interfaces: `Tool` (base), `ContextualTool` (channel/chatID aware), `AsyncTool` (background with callback). All file tools are workspace-scoped. `ToolTracker` monitors execution metrics (success rate, duration).

### Web server (`pkg/web/server.go`)

HTMX-based UI with embedded Go templates. Serves chat with streaming/markdown/image upload, agent management, real-time activity feed via WebSocket (`DashboardHub`), settings panels, session history with search.

### Session & Memory (`pkg/session/`, `pkg/memory/`)

Conversation history persisted in SQLite (`~/.sofia/memory.db`). `SessionManager` loads/saves per-session message history. `MemoryDB` also stores agent memory notes, goals, and triggers. All agents share the same database.

## Code conventions

- Build: `CGO_ENABLED=0`, `-tags stdjson` (set by Makefile)
- Line length: 120 characters (enforced by `golines`)
- Import order (enforced by `gci`): standard library → third-party → local (`github.com/grasberg/sofia/...`)
- Formatting: `gofumpt`, `goimports`, `golines` — run `make fmt`
- `interface{}` → `any`, `a[b:len(a)]` → `a[b:]` (auto-rewritten by gofmt)
- Tests use `testing` stdlib and `github.com/stretchr/testify`
- Integration tests are in `_integration_test.go` files and may require external credentials
- SQLite: uses `modernc.org/sqlite` (pure Go, no CGO)
