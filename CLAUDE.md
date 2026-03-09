# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Commands

```bash
# Build
go build ./...
go build -o sofia ./cmd/sofia

# Run
go run ./cmd/sofia

# Test all
go test ./...

# Test a single package
go test ./pkg/providers/...

# Test a single test
go test ./pkg/providers/... -run TestFallback

# Lint (uses golangci-lint v2)
golangci-lint run

# Format code (run via golangci-lint or directly)
gofmt -w .
gofumpt -w .
goimports -w .
```

Config is stored at `~/.sofia/config.json`. The binary is `sofia`.

## Architecture

Sofia is a personal AI agent gateway written in Go. It routes messages from multiple channels (Telegram, Discord, CLI, Web) to one or more configured LLM agents, then streams responses back.

### Top-level structure

- `cmd/sofia/` — CLI entrypoint using Cobra. Subcommands: `gateway`, `agent`, `cron`, `onboard`, `version`
- `pkg/` — All core logic as importable packages
- `workspace/` — Runtime workspace: `AGENT.md` (system prompt), `USER.md` (user profile), `skills/` (local skills)

### Core data flow

1. **Gateway** (`pkg/agent/loop.go` — `AgentLoop`) is the central coordinator. It owns a `MessageBus` and dispatches inbound messages from channels to agents.
2. **Channels** (`pkg/channels/`) — Telegram, Discord, CLI, Web. Each sends `InboundMessage` to the bus and subscribes to `OutboundMessage`.
3. **AgentRegistry** (`pkg/agent/registry.go`) — Holds all configured `AgentInstance` objects. Routes messages using `routing.RouteResolver` (based on bindings in config).
4. **AgentInstance** (`pkg/agent/instance.go`) — Per-agent state: model config, `ToolRegistry`, `SessionManager`, `ContextBuilder`, fallback candidates.
5. **AgentLoop.Process()** calls the LLM in a tool-use loop (max iterations configurable), dispatching tool calls and appending results until the model stops calling tools.

### Provider abstraction

`pkg/providers/` implements `LLMProvider` (single `Chat()` method). Supported backends:
- HTTP-compatible OpenAI API (OpenAI, Anthropic, Groq, Gemini, OpenRouter, etc.)
- Claude CLI (`claude-cli` / `claude-code` provider)
- Codex CLI (`codex-cli` provider)
- GitHub Copilot

Provider selection is resolved from `config.json` in `factory.go`. Models can be specified with a protocol prefix (e.g. `openai/gpt-4o`) or as aliases from `model_list`. Fallback chains (`pkg/providers/fallback.go`) try candidates in order on retriable errors.

### Config structure (`pkg/config/config.go`)

Key fields in `~/.sofia/config.json`:
- `agents.defaults` — workspace path, default model, max tokens, max tool iterations
- `agents.list` — per-agent overrides (id, name, model, workspace, template, skills filter, subagents)
- `bindings` — routes channel/peer patterns to specific agent IDs
- `channels` — Telegram/Discord token config
- `providers` — API keys per provider
- `model_list` — named model aliases with provider config
- `session` — DM scope, identity links across channels

### Skills system (`pkg/skills/`)

Skills are Markdown-based capability packages loaded from `workspace/skills/` (local) and `~/.sofia/skills/` (global). Each skill is a directory with a `SKILL.md` file containing YAML frontmatter (`name`, `description`) and Markdown instructions. The `ContextBuilder` loads skill metadata into the system prompt; full skill bodies are only loaded when triggered.

The `RegistryManager` supports fetching skills from remote registries (e.g. ClawHub) for install/search.

### Tools (`pkg/tools/`)

Built-in tools registered per agent: `read_file`, `write_file`, `list_dir`, `exec`, `edit_file`, `append_file`, `image_analyze`, plus channel-specific tools (web browsing via Playwright, hardware I2C/SPI on Linux, spawn subagents). All file tools are workspace-scoped and can be restricted to the workspace directory.

### Session & Memory (`pkg/session/`, `pkg/memory/`)

Conversation history is persisted in SQLite (`~/.sofia/memory.db`). `SessionManager` loads/saves per-session message history. `MemoryDB` also stores agent memory notes that appear in the system prompt.

### Cron (`pkg/cron/`, `cmd/sofia/internal/cron/`)

A cron service persists scheduled jobs to `~/.sofia/cron/jobs.json`. Jobs fire messages into the agent loop on schedule. CLI subcommands: `cron add`, `cron list`, `cron remove`, `cron enable`, `cron disable`.

## Code conventions

- Line length limit: 120 characters
- Import order (enforced by `gci`): standard library → third-party → local (`github.com/grasberg/sofia/...`)
- `gofumpt` and `goimports` for formatting
- Tests use `testing` stdlib and `github.com/stretchr/testify`
- Integration tests are in `_integration_test.go` files and may require external credentials
