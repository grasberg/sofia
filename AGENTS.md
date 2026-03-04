# AGENTS.md — Sofia Codebase Guide

Sofia is an ultra-lightweight personal AI agent written in Go.
Module path: `github.com/grasberg/sofia`

---

## Build, Lint, and Test Commands

### Build

```sh
make build          # Build for current platform (outputs: ./sofia)
make build-all      # Cross-compile for all platforms (outputs in build/)
make generate       # Run go generate (required before manual builds)
```

The build uses `CGO_ENABLED=0` and `-tags stdjson` by default (see `GOFLAGS` in Makefile).

### Test

```sh
make test                                    # Run all tests
go test ./...                                # Equivalent to make test
go test ./pkg/agent/...                      # Run tests in a specific package
go test ./pkg/agent/ -run TestRecordLastChannel   # Run a single test by name
go test ./pkg/agent/ -run TestRecordLastChannel -v  # Verbose output
```

### Lint and Format

```sh
make lint           # Run golangci-lint
make fmt            # Format code (golangci-lint fmt — runs gofmt, gofumpt, goimports, gci, golines)
make fix            # Auto-fix lint issues
make vet            # Run go vet
make check          # deps + fmt + vet + test
```

### Other

```sh
make deps           # Download and verify dependencies
make run ARGS="..." # Build and run sofia
```

---

## Project Structure

```
cmd/sofia/          Entry point; Cobra CLI wiring only (main.go ~54 lines)
  internal/         CLI subcommands (agent, auth, cron, gateway, skills, etc.)
pkg/
  agent/            AgentInstance + AgentLoop — core orchestration
  auth/             Credential storage
  bus/              Internal message bus (pub/sub)
  channels/         Channel management (Telegram, Discord, etc.)
  config/           Config struct and JSON loading
  constants/        Shared constants
  cron/             Scheduled task runner
  devices/          Hardware device support
  fileutil/         File helpers
  health/           Health checks
  heartbeat/        Scheduled heartbeat messages
  logger/           Logging wrapper
  providers/        LLM provider adapters (Anthropic, OpenAI, Copilot, etc.)
  routing/          Agent routing and ID normalization
  session/          Session/history management
  skills/           Skill discovery and registry
  state/            Persistent state (last channel, chat ID)
  tools/            Tool implementations (file, web, exec, I2C, SPI, etc.)
  utils/            Miscellaneous utilities
  voice/            Voice support
  web/              Web UI
assets/             Static assets
third_party/        Vendored external packages (antigravity-kit)
```

---

## Code Style Guidelines

### Formatting

- Max line length: **120 characters** (enforced by `golines` and `lll`).
- Use `gofmt`/`gofumpt` style; do not manually format.
- Use `interface{}` → `any` (enforced by gofmt rewrite rule).
- Tab width: 4 spaces (for lint reporting purposes; Go always uses tabs).

### Imports

Imports are grouped in **three sections**, in this order, enforced by `gci`:

1. Standard library
2. Third-party packages
3. Local module (`github.com/grasberg/sofia/...`)

Example:
```go
import (
    "context"
    "fmt"

    "github.com/spf13/cobra"

    "github.com/grasberg/sofia/pkg/config"
    "github.com/grasberg/sofia/pkg/providers"
)
```

Never mix groups or use blank-line-free import blocks.

### Naming Conventions

- **Packages**: short, lowercase, singular (`agent`, `config`, `tools`).
- **Structs**: `PascalCase` (`AgentInstance`, `AgentLoop`, `FlexibleStringSlice`).
- **Interfaces**: `PascalCase`; prefer noun or noun+er (`LLMProvider`, `SessionManager`).
- **Functions/methods**: `PascalCase` for exported, `camelCase` for unexported.
- **Constructor functions**: `NewXxx(...)` pattern (`NewAgentLoop`, `NewAgentInstance`).
- **Constants**: `PascalCase` for exported, `camelCase` for unexported; group with `iota` for enums.
- **Variables**: short but descriptive; avoid single-letter names except in tight loops or standard idioms (`err`, `ok`, `i`).

### Types and Structs

- Prefer concrete struct types over `interface{}` / `any` unless polymorphism is needed.
- Custom JSON marshaling: implement `MarshalJSON`/`UnmarshalJSON` on the type directly (see `FlexibleStringSlice`, `AgentModelConfig`).
- Struct fields use JSON tags with `snake_case` keys and `omitempty` where the field is optional.
- Config structs carry both `json:"..."` and `env:"..."` tags for environment variable overrides.

### Error Handling

- Always check errors; do not ignore with `_` except where truly safe.
- Wrap errors with context: `fmt.Errorf("doing X: %w", err)`.
- Use sentinel errors sparingly; prefer wrapping with `%w` for unwrappable errors.
- In constructors and setup code, log and continue or return early — do not panic.
- Tests use `t.Fatalf` for fatal setup errors and `t.Errorf` for assertion failures.

### Context

- Pass `context.Context` as the **first parameter** to any function that performs I/O or long-running work.
- Do not store contexts in structs.

### Logging

- Use the internal `logger` package (`github.com/grasberg/sofia/pkg/logger`) — not `log` or `fmt.Println`.
- Log at appropriate levels; avoid noisy debug logs in production paths.

### Concurrency

- Use `sync/atomic` for simple flags and counters (`atomic.Bool`, `atomic.Value`, `atomic.Uint64`).
- Use `sync.Map` for concurrent map access (`summarizing sync.Map`).
- Protect shared state with the appropriate primitive; document ownership.

### Build Tags

- The build uses `-tags stdjson` by default. Code that imports a JSON library should be guarded accordingly if it differs from the standard library implementation.
- Hardware tools (`pkg/tools/i2c.go`, `pkg/tools/spi.go`) have special lint exclusions — they may use nolint directives.

### Code Generation

- `//go:generate` directives live in relevant packages. Run `make generate` before building.
- Generated files are excluded from some lint rules (`generated: lax` in `.golangci.yaml`).

---

## Testing Conventions

- Tests live in `*_test.go` files in the **same package** (white-box testing; `package agent` not `package agent_test`).
- Use `testing.T` from the standard library; the only test dependency is `github.com/stretchr/testify` for assertions.
- Use `os.MkdirTemp("", "prefix-*")` to create isolated temp directories; always `defer os.RemoveAll(tmpDir)`.
- Mock dependencies with local `mock*` types defined in `mock_*_test.go` files (e.g., `mockProvider`).
- Test function names follow `TestFunctionName_Scenario` or `TestFunctionName` (descriptive suffix optional).
- Integration tests that require external processes are in `*_integration_test.go` files.
- Do **not** use parallel tests (`paralleltest` linter is disabled but tests are not explicitly parallel).

---

## Linter Notes

The project uses `golangci-lint` v2 with `default: all` and a large disable list (see `.golangci.yaml`). Key active rules include:

- `govet` (all checks except `fieldalignment`)
- `misspell` (US locale)
- `nakedret` (max 3 lines)
- `gci`, `gofmt`, `gofumpt`, `goimports`, `golines` (all formatting)

Disabled linters that are **aspirationally enabled** (marked TODO) include `errcheck`, `errorlint`, `staticcheck`, `revive`, `gosec` — fix these over time rather than adding new violations.

When suppressing a lint warning inline, use `//nolint:lintername // reason`.
