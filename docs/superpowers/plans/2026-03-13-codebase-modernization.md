# Sofia Codebase Modernization Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Modernize the Sofia Go codebase for improved maintainability, readability, and code quality while preserving all existing functionality.

**Architecture:** Phased refactoring starting with structural decomposition of the largest files, then improving error handling, config validation, provider patterns, and security. Each phase is independently testable and preserves behavioral equivalence.

**Tech Stack:** Go 1.25, golangci-lint v2, testify, SQLite (modernc.org/sqlite)

---

## Audit Summary

### Critical Issues (by priority)

1. **loop.go is 2,950 lines** with 40+ functions — the single biggest maintainability problem
2. **`runLLMIteration()` is 683 lines** — extreme cyclomatic complexity
3. **Inconsistent error handling** — mixed wrapping, some errors silently ignored
4. **Magic strings everywhere** — provider names, auth methods, config keys
5. **No config validation** — broken config silently proceeds
6. **Provider factory is a giant switch** — no registry/extensibility pattern
7. **Regex recompilation** — error patterns and guardrail patterns compiled on every call
8. **Skills YAML parsing** — uses regex + string split instead of YAML library
9. **Single-writer SQLite** — bottleneck under concurrency
10. **Shell guardrails bypassable** — whitespace-variant pattern evasion

### Files Requiring Attention

| File | Lines | Issue |
|------|-------|-------|
| `pkg/agent/loop.go` | 2,950 | God file — needs decomposition |
| `pkg/memory/db.go` | 1,695 | Large, many intentional error ignores |
| `pkg/tools/cpanel.go` | 987 | Large, hardcoded limits |
| `pkg/tools/bitcoin.go` | 935 | Large, mixed error handling |
| `pkg/web/server.go` | 878 | Global state, hardcoded timeouts |
| `pkg/tools/plan.go` | 877 | Large Parameters() method |
| `pkg/config/config.go` | 772 | No validation, magic strings |
| `pkg/agent/context.go` | 665 | Large context builder |

---

## Phase 1: Decompose loop.go (Lowest Risk, Highest Impact)

**Risk:** Minimal — pure file split, no logic changes, all tests must pass unchanged.

### Task 1.1: Split loop.go into focused files

**Files:**
- Modify: `pkg/agent/loop.go` (strip down to core struct + lifecycle)
- Create: `pkg/agent/loop_tools.go` (tool registration)
- Create: `pkg/agent/loop_processing.go` (message processing pipeline)
- Create: `pkg/agent/loop_llm.go` (LLM iteration loop)
- Create: `pkg/agent/loop_commands.go` (slash command handling)
- Create: `pkg/agent/loop_summarize.go` (session summarization)
- Create: `pkg/agent/loop_query.go` (read-only query methods)
- Create: `pkg/agent/loop_helpers.go` (utility functions)

Function allocation:

**loop.go** (~250 lines) — Core struct, constructor, lifecycle:
- `AgentLoop` struct definition
- `processOptions` struct
- `NewAgentLoop()`
- `Run()`
- `Stop()`
- `Reset()`
- `getRegistry()`
- `RegisterTool()`
- `SetChannelManager()`
- `ReloadAgents()`

**loop_tools.go** (~300 lines) — Tool registration:
- `registerSharedTools()`

**loop_processing.go** (~500 lines) — Message processing:
- `processMessage()`
- `processSystemMessage()`
- `ProcessDirect()`
- `ProcessDirectWithImages()`
- `ProcessDirectWithChannel()`
- `ProcessHeartbeat()`
- `runAgentLoop()`
- `runSpawnedTaskAsAgent()`

**loop_llm.go** (~700 lines) — LLM call loop:
- `runLLMIteration()`
- `applyOutputFilter()`

**loop_commands.go** (~120 lines) — Command handling:
- `handleCommand()`
- `handleStatusCommand()`

**loop_summarize.go** (~250 lines) — Summarization:
- `maybeSummarize()`
- `forceCompression()`
- `summarizeSession()`
- `summarizeBatch()`
- `safeKeepCount()`
- `safeCutPoint()`
- `estimateTokens()`

**loop_query.go** (~120 lines) — Query methods:
- `ListAgentIDs()`
- `ListAgentTools()`
- `ListSessionMetas()`
- `GetSessionHistory()`
- `GetDefaultSessionManager()`
- `DashboardHub()`
- `ListGoals()`
- `GetStartupInfo()`

**loop_helpers.go** (~120 lines) — Utilities:
- `extractPeer()`
- `extractParentPeer()`
- `looksLikeTask()`
- `formatMessagesForLog()`
- `formatToolsForLog()`
- `RecordLastChannel()`
- `RecordLastChatID()`
- `updateToolContexts()`
- `recordReputation()`
- `startAutonomyServices()`
- `stopAutonomyServices()`
- `maybLearnFromFeedback()`
- `maybeReflect()`

Steps:

- [ ] **Step 1:** Run existing tests to establish baseline
  - Run: `go test ./pkg/agent/... -count=1 -v`
  - Expected: All tests pass

- [ ] **Step 2:** Create the 7 new files by extracting functions from loop.go
  - Move each function group with its required imports
  - Keep all functions identical — no logic changes

- [ ] **Step 3:** Verify the split compiles
  - Run: `go build ./pkg/agent/...`
  - Expected: Clean build

- [ ] **Step 4:** Run all tests to verify behavioral equivalence
  - Run: `go test ./pkg/agent/... -count=1 -v`
  - Expected: Identical results to Step 1

- [ ] **Step 5:** Run full test suite
  - Run: `go test ./... -count=1 -timeout 5m`
  - Expected: All tests pass

- [ ] **Step 6:** Run linter
  - Run: `golangci-lint run ./pkg/agent/...`
  - Expected: No new warnings

- [ ] **Step 7:** Commit
  - `git commit -m "refactor: decompose agent/loop.go into focused files"`

---

## Phase 2: Extract Constants and Add Config Validation (Low Risk)

### Task 2.1: Extract magic strings to constants

**Files:**
- Create: `pkg/providers/provider_names.go`
- Modify: `pkg/providers/factory.go`
- Modify: `pkg/config/config.go`

- [ ] Define provider name constants (ProviderOpenAI, ProviderAnthropic, etc.)
- [ ] Define auth method constants
- [ ] Replace all string literals in factory.go with constants
- [ ] Replace string literals in config.go with constants
- [ ] Run tests, lint, commit

### Task 2.2: Add config validation

**Files:**
- Create: `pkg/config/validate.go`
- Create: `pkg/config/validate_test.go`
- Modify: `pkg/config/config.go` (call Validate after load)

- [ ] Write tests for validation (missing required fields, invalid values)
- [ ] Implement `Config.Validate() error` method
- [ ] Call Validate() after config load
- [ ] Run tests, lint, commit

---

## Phase 3: Pre-compile Regexes (Low Risk)

### Task 3.1: Pre-compile guardrail patterns

**Files:**
- Modify: `pkg/agent/loop_processing.go` (or wherever guardrails end up)
- Modify: `pkg/agent/loop_llm.go`

- [ ] Move prompt injection patterns to package-level compiled regexes
- [ ] Pre-compile output filter patterns at config load time
- [ ] Pre-compile input validation deny patterns at config load time
- [ ] Run tests, lint, commit

### Task 3.2: Pre-compile error classifier patterns

**Files:**
- Modify: `pkg/providers/error_classifier.go`

- [ ] Move regex patterns to package-level compiled regexes
- [ ] Run tests, lint, commit

---

## Phase 4: Improve Error Handling (Medium Risk)

### Task 4.1: Consistent error wrapping

**Files:**
- Modify: across `pkg/agent/`, `pkg/providers/`, `pkg/tools/`

- [ ] Audit all `fmt.Errorf` calls — ensure `%w` is used for wrappable errors
- [ ] Replace bare `errors.New` where context should be added
- [ ] Add error context to currently bare returns
- [ ] Fix intentionally-ignored errors (add comments or handle)
- [ ] Run tests, lint, commit

### Task 4.2: Fix unchecked errors in tests

**Files:**
- Modify: test files in `pkg/tools/`, `pkg/agent/`, `pkg/heartbeat/`

- [ ] Add error checks to `os.WriteFile()` calls in tests
- [ ] Add error checks to `Close()` calls
- [ ] Use `require.NoError(t, err)` pattern
- [ ] Run tests, lint, commit

---

## Phase 5: Provider Registry Pattern (Medium Risk)

### Task 5.1: Replace factory switch with registry

**Files:**
- Create: `pkg/providers/registry.go`
- Create: `pkg/providers/registry_test.go`
- Modify: `pkg/providers/factory.go`

- [ ] Define `ProviderFactory` interface
- [ ] Create `ProviderRegistry` with `Register()` and `Create()` methods
- [ ] Migrate each provider case from switch to registered factory
- [ ] Update factory.go to use registry
- [ ] Run tests, lint, commit

---

## Phase 6: Skills YAML Parsing (Low Risk)

### Task 6.1: Replace regex YAML with proper parser

**Files:**
- Modify: `pkg/skills/loader.go`
- Modify: `pkg/skills/loader_test.go`

- [ ] Replace regex frontmatter extraction with `gopkg.in/yaml.v3`
- [ ] Replace `parseSimpleYAML()` with proper YAML unmarshal
- [ ] Add test cases for edge cases (colons in values, quotes, multiline)
- [ ] Run tests, lint, commit

---

## Phase 7: Session/Memory Optimization (Medium-High Risk)

### Task 7.1: Add database indexes

**Files:**
- Modify: `pkg/memory/db.go`

- [ ] Add indexes for common query patterns (agent_id, session_key, created_at)
- [ ] Add composite indexes where needed
- [ ] Run tests, lint, commit

### Task 7.2: Optimize message serialization

**Files:**
- Modify: `pkg/session/manager.go`

- [ ] Add pagination to GetHistory() (limit + offset)
- [ ] Consider lazy loading for large sessions
- [ ] Run tests, lint, commit

---

## Phase 8: Security Hardening (Medium Risk)

### Task 8.1: Improve shell guardrails

**Files:**
- Modify: `pkg/tools/shell_guardrails.go`
- Modify: `pkg/tools/shell_guardrails_test.go`

- [ ] Normalize whitespace before pattern matching
- [ ] Add tests for whitespace bypass attempts
- [ ] Pre-compile deny patterns
- [ ] Run tests, lint, commit

---

## Remaining Technical Debt (Not Addressed in This Plan)

- `memory/db.go` (1,695 lines) needs decomposition in a future pass
- `web/server.go` global state consolidation
- `tools/cpanel.go` and `tools/bitcoin.go` could be split
- Connection pooling for SQLite under high concurrency
- Tool parameter validation framework
- Semantic tool matching integration into core loop
- Concurrency strategy documentation
- Architecture Decision Records (ADRs)
