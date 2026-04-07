# Sofia Refactoring Guide - Step by Step

## Overview

This guide provides detailed, actionable steps for refactoring Sofia's 10 largest Go files. Each section includes:
- Current state analysis
- Target structure
- Exact code moves required
- Testing strategy
- Rollback plan

---

## Part 1: Web Server (server.go - 2,163 lines)

### Why Start Here?
1. **Largest file** (2,163 lines) - immediate impact
2. **Clear domain boundaries** - easy to split by endpoint type
3. **Low risk** - HTTP handlers are independent
4. **High testability** - each handler can be unit tested

### Current Structure

All 39 handlers in `server.go`:
- Chat: `handleChat`, `handleChatStream`
- Config: `handleConfig`
- Agents: `handleAgents`, `handleAgentTemplates`, `handleAgentTemplateByName`
- Sessions: `handleSessions`, `handleSessionDetail`
- Workspace: `handleWorkspaceFiles`, `handleWorkspaceFile`, `handleWorkspaceDocs`
- Skills: `handleSkillAdd`, `handleSkillsList`, `handleSkillsToggle`
- Memory: `handleMemoryNotes`, `handleMemoryGraph`, `handleMemoryReflections`
- Evolution: `handleEvolutionStatus`, `handleEvolutionChangelog`
- Cron: `handleCron`, `handleCronToggle`
- Eval: `handleEvalRuns`, `handleEvalRunDetail`, `handleEvalTrend`
- Audit: `handleAudit`, `handleApprovals`, `handleApprovalAction`
- System: `handleStatus`, `handleRestart`, `handleUpdate`, `handleReset`
- Search: `handleSearch`
- Logs: `handleLogs`
- Goals: `handleGoals`, `handleGoalLog`
- Plan: `handlePlan`
- Presence: `handlePresence`
- Backup: `handleBackupExport`

### Refactoring Steps

#### Step 1: Create Handler Structs

For each domain, create a handler struct:

```go
// pkg/web/handlers/chat.go
package handlers

type ChatHandler struct {
    Srv *web.Server
}

func (h *ChatHandler) Register(mux *http.ServeMux) {
    mux.HandleFunc("/api/chat", h.HandleChat)
    mux.HandleFunc("/api/chat/stream", h.HandleChatStream)
}
```

#### Step 2: Move Handlers

For EACH handler:
1. Copy the method from `server.go` to the handler file
2. Change receiver from `*web.Server` to `*handlers.ChatHandler`
3. Access server via `h.Srv` instead of `s`
4. Update imports
5. Build and test

#### Step 3: Wire Up in server.go

```go
// pkg/web/server.go
import "github.com/grasberg/sofia/pkg/web/handlers"

func (s *Server) setupRoutes(mux *http.ServeMux) {
    // Create handlers
    chatHandler := &handlers.ChatHandler{Srv: s}
    agentHandler := &handlers.AgentHandler{Srv: s}
    // ... etc for each domain
    
    // Register routes
    chatHandler.Register(mux)
    agentHandler.Register(mux)
    // ... etc
}
```

#### Step 4: Test

```bash
# Build
go build ./pkg/web

# Test
go test ./pkg/web -v

# Manual test
sofia gateway
# Open http://localhost:8080
# Test all endpoints
```

### Rollback Plan

If anything breaks:
```bash
git checkout HEAD -- pkg/web/
go build ./pkg/web
```

---

## Part 2: Agent Loop Family (5,838 lines)

### Why Second?

Agent loop is the core of Sofia. It's already partially split but incomplete.

### Current Files

1. `loop.go` (752 lines) - AgentLoop struct, lifecycle
2. `loop_llm.go` (1,179 lines) - LLM iteration
3. `loop_llm_extracted.go` (831 lines) - Incomplete extraction
4. `loop_processing.go` (1,201 lines) - Message processing
5. `loop_commands.go` (1,101 lines) - Command handlers
6. `context.go` (774 lines) - Context building

### Refactoring Strategy

#### Step 1: Merge loop_llm.go + loop_llm_extracted.go

These two files are confusing because `loop_llm_extracted.go` duplicates some functions from `loop_llm.go`.

**Action:**
1. Read both files completely
2. Identify overlapping functions
3. Merge into single `loop_llm.go` with clear function boundaries
4. Delete `loop_llm_extracted.go`

#### Step 2: Extract LLM Sub-Operations

Create `pkg/agent/llm/` package:

```go
// pkg/agent/llm/prompts.go
func BuildSystemPrompt(agent *AgentInstance, opts ProcessOptions) string
func BuildUserMessage(opts ProcessOptions) providers.Message
func InjectReflection(messages []providers.Message, iteration int) []providers.Message

// pkg/agent/llm/execution.go
func CallLLM(ctx context.Context, provider LLMProvider, messages []providers.Message, opts LLMOpts) (*LLMResponse, error)
func RetryOnContextError(fn LLMCallFn, maxRetries int) (*LLMResponse, error)

// pkg/agent/llm/tools.go
func ExtractToolCalls(response *LLMResponse) []ToolCall
func ExecuteToolCalls(ctx context.Context, calls []ToolCall, registry *ToolRegistry) []ToolResult
```

#### Step 3: Extract Processing Sub-Operations

Create `pkg/agent/processing/` package:

```go
// pkg/agent/processing/dispatcher.go
func DispatchMessage(ctx context.Context, msg Message, loop *AgentLoop) error
func CreateSubAgentTask(ctx context.Context, agentID string, task string) error

// pkg/agent/processing/subagent.go
func SpawnSubAgent(ctx context.Context, agentID string, config *Config) (*SubAgent, error)
func ManageSubAgentLifecycle(agent *SubAgent) error
```

### Testing Strategy

```bash
# Unit tests for each sub-package
go test ./pkg/agent/llm -v
go test ./pkg/agent/processing -v

# Integration tests
go test ./pkg/agent -v

# Manual test
sofia agent "test message"
```

---

## Part 3: Evolution Engine (1,121 lines)

### Current Responsibilities

1. Observe - gather metrics
2. Diagnose - analyze metrics with LLM
3. Plan - propose actions with LLM
4. Act - execute actions
5. Verify - check results
6. Consolidate - memory maintenance
7. Proposals - human approval workflow
8. Learning - improve from experience

### Target Structure

```
pkg/evolution/
├── engine.go          - Main cycle orchestration (200 lines)
├── observe.go         - Metrics gathering (150 lines)
├── diagnose.go        - LLM-based analysis (150 lines)
├── plan.go            - Action planning (150 lines)
├── act.go             - Action execution (150 lines)
├── verify.go          - Results verification (100 lines)
├── proposals.go       - Human approval (150 lines)
└── learning.go        - Experience learning (100 lines)
```

### Refactoring Steps

#### Step 1: Extract observe()

```go
// pkg/evolution/observe.go
package evolution

func (e *EvolutionEngine) observe(ctx context.Context) ObservationReport {
    // Move from engine.go:256-313
}
```

#### Step 2: Extract diagnose()

```go
// pkg/evolution/diagnose.go
package evolution

func (e *EvolutionEngine) diagnose(ctx context.Context, report ObservationReport) (Diagnosis, error) {
    // Move from engine.go:314-368
}
```

#### Step 3: Repeat for each phase

Each phase becomes its own file.

### Testing

```bash
go test ./pkg/evolution -v -run TestEvolutionEngine
```

---

## Part 4: Plan Manager (1,087 lines)

### Current Structure

All plan operations in `pkg/tools/plan.go`:
- Plan struct and lifecycle
- Step management
- Status transitions
- Hierarchical plans
- Execution tracking

### Target Structure

```
pkg/tools/plan/
├── plan.go        - Plan type and lifecycle (200 lines)
├── step.go        - Step management (200 lines)
├── status.go      - Status transitions (150 lines)
├── hierarchy.go   - Sub-plans (200 lines)
└── tracker.go     - Execution tracking (200 lines)
```

### Refactoring Steps

Same pattern as evolution engine - extract each concern to its own file.

---

## Part 5: Semantic Memory (980 lines)

### Current Structure

All knowledge graph operations in `pkg/memory/db_semantic.go`:
- Node CRUD
- Edge management
- Semantic search
- Graph traversal
- Deduplication
- Consolidation

### Target Structure

```
pkg/memory/semantic/
├── nodes.go      - Node operations (200 lines)
├── edges.go      - Edge operations (150 lines)
├── search.go     - Semantic search (200 lines)
├── traversal.go  - Graph queries (200 lines)
└── maintenance.go - Dedup, pruning (150 lines)
```

---

## Part 6: Config (956 lines)

### Current Structure

All in `pkg/config/config.go`:
- Struct definitions (~400 lines)
- Loading logic (~200 lines)
- Defaults (~150 lines)
- Validation (~100 lines) - already in validate.go
- Migration (~100 lines) - already in migration.go

### Target Structure

```
pkg/config/
├── types.go       - All struct definitions (300 lines)
├── loader.go      - File/env loading (200 lines)
├── defaults.go    - Default values (150 lines) - exists
├── validate.go    - Validation (150 lines) - exists
└── migration.go   - Migration (150 lines) - exists
```

### Refactoring Steps

1. Move all struct definitions to `types.go`
2. Move loading logic to `loader.go`
3. Keep imports and cross-references working

---

## Part 7: Autonomy Service (813 lines)

### Current Structure

All in `pkg/autonomy/service.go`:
- Goal management
- Task queue
- Proactive suggestions
- Budget enforcement
- Service lifecycle

### Target Structure

```
pkg/autonomy/
├── service.go     - Main service (200 lines)
├── goals.go       - Goal management (200 lines)
├── tasks.go       - Task queue (200 lines)
└── suggestions.go - Proactive suggestions (150 lines)
```

---

## General Refactoring Pattern

For EACH file being split:

### Step 1: Analyze
```bash
# Count lines and functions
wc -l file.go
grep "^func " file.go | wc -l
```

### Step 2: Plan
- Identify logical groupings
- Define target file structure
- List dependencies between groups

### Step 3: Extract (one group at a time)
1. Create target file
2. Copy function(s)
3. Update receiver/type if needed
4. Add necessary imports
5. Build: `go build ./pkg/...`
6. Test: `go test ./pkg/...`
7. Commit: `git add -A && git commit -m "refactor: extract X"`

### Step 4: Verify
```bash
# Full build
go build ./...

# All tests
go test ./... -count=1

# Manual smoke test
sofia gateway
# Open browser, test key features
```

### Step 5: Clean Up
- Remove old code
- Update documentation
- Add package-level godoc

---

## Testing Strategy

### Before Refactoring
```bash
# Establish baseline
go test ./... -count=1 > /tmp/test-baseline.txt
go build ./... > /tmp/build-baseline.txt
```

### During Refactoring
```bash
# After each extraction
go build ./pkg/affected
go test ./pkg/affected -v
```

### After Refactoring
```bash
# Compare to baseline
go test ./... -count=1 > /tmp/test-after.txt
diff /tmp/test-baseline.txt /tmp/test-after.txt
```

---

## Risk Mitigation

### 1. Small Commits
- Commit after each successful extraction
- Include build/test verification in commit message
- Easy to rollback if needed

### 2. Feature Flags
- No feature changes during refactoring
- Pure code movement
- API-compatible changes only

### 3. Parallel Testing
- Run old and new code in parallel initially
- Compare outputs
- Switch when confident

### 4. Documentation
- Update package docs
- Add migration guide
- Clear commit messages

---

## Timeline

| Phase | File | Lines | Est. Time | Day |
|-------|------|-------|-----------|-----|
| 1 | server.go | 2,163 | 4-6h | Day 1-2 |
| 2 | loop_llm.go family | 5,838 | 8-12h | Day 3-5 |
| 3 | evolution/engine.go | 1,121 | 3-4h | Day 6 |
| 4 | tools/plan.go | 1,087 | 3-4h | Day 7 |
| 5 | memory/db_semantic.go | 980 | 3-4h | Day 8 |
| 6 | config/config.go | 956 | 2-3h | Day 9 |
| 7 | autonomy/service.go | 813 | 2-3h | Day 10 |

**Total: ~10 days of focused work**

---

## Success Criteria

1. ✅ No file >500 lines
2. ✅ All tests pass
3. ✅ No breaking changes
4. ✅ Improved code navigation
5. ✅ Clear package boundaries
6. ✅ Better documentation

---

## Getting Started

Start with the web server (`server.go`) because:
1. **Largest impact** - 2,163 → ~200 lines
2. **Clearest boundaries** - HTTP handlers by domain
3. **Lowest risk** - Independent endpoints
4. **Best practice** - Demonstrates pattern for rest

Then move to agent loop family, then the rest.

---

**Guide Created:** April 5, 2026  
**Status:** Ready to execute  
**First Target:** `pkg/web/server.go`
