# Sofia Codebase Refactoring - Complete Plan

## Executive Summary

This document outlines a systematic refactoring of Sofia's 10 largest Go files (totaling ~11,000 lines) into well-organized, maintainable packages. The goal is to reduce cognitive load, improve testability, and enable parallel development without breaking changes.

---

## Phase 1: Web Server (2,163 → ~200 lines) **HIGHEST PRIORITY**

### Current State
- **File:** `pkg/web/server.go`
- **Lines:** 2,163
- **Functions:** 55+
- **Responsibilities:** All HTTP handlers, routing, WebSocket, auth, file management, agent control, memory queries, cron, eval, skills, audit, approvals

### Target Structure
```
pkg/web/
├── server.go                    (200 lines - Server struct, routing, init)
├── handlers/
│   ├── chat.go                 (150 lines - chat endpoints)
│   ├── agents.go               (150 lines - agents, templates)
│   ├── config.go               (100 lines - config CRUD)
│   ├── sessions.go             (100 lines - session management)
│   ├── workspace.go            (200 lines - file CRUD, docs)
│   ├── skills.go               (150 lines - skill management)
│   ├── memory.go               (150 lines - notes, graph, reflections)
│   ├── evolution.go            (100 lines - evolution endpoints)
│   ├── cron.go                 (100 lines - cron management)
│   ├── eval.go                 (150 lines - evaluation endpoints)
│   ├── audit.go                (100 lines - audit, approvals)
│   ├── system.go               (100 lines - status, restart, update)
│   └── search.go               (50 lines - search)
```

### Implementation Strategy
1. Create `handlers` package with handler structs
2. Move handlers by domain (group related endpoints)
3. Update `server.go` to wire handlers
4. All routes remain identical - zero breaking changes

### Example: handlers/chat.go
```go
package handlers

type ChatHandler struct {
    server *web.Server
}

func (h *ChatHandler) HandleChat(w http.ResponseWriter, r *http.Request) {
    // Moved from server.go:570
}

func (h *ChatHandler) HandleChatStream(w http.ResponseWriter, r *http.Request) {
    // Moved from server.go:660
}
```

---

## Phase 2: Agent Loop Family (5,838 → ~2,500 lines) **HIGH PRIORITY**

### Current State
- `pkg/agent/loop.go` (752 lines)
- `pkg/agent/loop_llm.go` (1,179 lines)
- `pkg/agent/loop_llm_extracted.go` (831 lines - incomplete split)
- `pkg/agent/loop_processing.go` (1,201 lines)
- `pkg/agent/loop_commands.go` (1,101 lines)
- `pkg/agent/context.go` (774 lines)

**Total: 5,838 lines across 6 files**

### Target Structure
```
pkg/agent/
├── loop.go                     (200 lines - AgentLoop struct, lifecycle)
├── loop_llm.go                 (150 lines - main LLM iteration)
├── loop_processing.go          (200 lines - message processing)
├── loop_commands.go            (150 lines - command registry)
├── llm/
│   ├── iteration.go           (200 lines - LLM call orchestration)
│   ├── tool_execution.go      (200 lines - tool call handling)
│   ├── prompts.go             (150 lines - prompt building)
│   ├── recovery.go            (150 lines - doom loop, rollback)
│   └── streaming.go           (100 lines - response streaming)
├── processing/
│   ├── dispatcher.go          (200 lines - task dispatch)
│   ├── subagent.go            (200 lines - sub-agent management)
│   └── pipeline.go            (150 lines - message pipeline)
├── commands/
│   ├── registry.go            (100 lines - command registration)
│   ├── session.go             (150 lines - session commands)
│   ├── memory.go              (150 lines - memory commands)
│   └── system.go              (150 lines - system commands)
└── context/
    ├── builder.go             (200 lines - context assembly)
    ├── workspace.go           (150 lines - workspace context)
    ├── skills.go              (150 lines - skills context)
    ├── memory.go              (150 lines - memory context)
    └── persona.go             (100 lines - persona context)
```

### Implementation Strategy
1. Complete the `loop_llm_extracted.go` split properly
2. Create sub-packages: `llm/`, `processing/`, `commands/`, `context/`
3. Move functions by concern
4. Keep public API identical

---

## Phase 3: Evolution Engine (1,121 → ~600 lines) **MEDIUM PRIORITY**

### Current State
- **File:** `pkg/evolution/engine.go`
- **Lines:** 1,121
- **Functions:** 34
- **Responsibilities:** Observe, diagnose, plan, act, verify, consolidate, improve, proposals, approval

### Target Structure
```
pkg/evolution/
├── engine.go                  (200 lines - main cycle orchestration)
├── observation.go             (150 lines - observe phase)
├── diagnosis.go               (150 lines - diagnose phase)
├── planning.go                (150 lines - plan phase)
├── execution.go               (150 lines - act phase)
├── verification.go            (100 lines - verify phase)
├── proposals.go               (150 lines - proposal management)
└── consolidation.go           (100 lines - memory consolidation)
```

---

## Phase 4: Plan Manager (1,087 → ~600 lines) **MEDIUM PRIORITY**

### Current State
- **File:** `pkg/tools/plan.go`
- **Lines:** 1,087
- **Functions:** 33

### Target Structure
```
pkg/tools/
├── plan/
│   ├── plan.go              (150 lines - Plan struct, lifecycle)
│   ├── step.go              (150 lines - Step management)
│   ├── status.go            (100 lines - status transitions)
│   ├── hierarchical.go      (150 lines - sub-plans)
│   └── execution.go         (150 lines - execution tracking)
```

---

## Phase 5: Semantic Memory (980 → ~600 lines) **MEDIUM PRIORITY**

### Current State
- **File:** `pkg/memory/db_semantic.go`
- **Lines:** 980
- **Functions:** 32

### Target Structure
```
pkg/memory/
├── semantic/
│   ├── nodes.go             (200 lines - node CRUD)
│   ├── edges.go             (150 lines - edge management)
│   ├── search.go            (200 lines - semantic search)
│   ├── traversal.go         (150 lines - graph traversal)
│   └── consolidation.go     (150 lines - dedup, pruning)
```

---

## Phase 6: Config (956 → ~600 lines) **LOW-MEDIUM PRIORITY**

### Current State
- **File:** `pkg/config/config.go`
- **Lines:** 956
- **Functions:** 19

### Target Structure
```
pkg/config/
├── types.go                 (300 lines - all struct definitions)
├── loader.go                (200 lines - file/env loading)
├── defaults.go              (150 lines - default values)
├── validation.go            (150 lines - validation - already exists)
└── migration.go             (150 lines - migration - already exists)
```

---

## Phase 7: Autonomy Service (813 → ~500 lines) **LOW-MEDIUM PRIORITY**

### Current State
- **File:** `pkg/autonomy/service.go`
- **Lines:** 813

### Target Structure
```
pkg/autonomy/
├── service.go               (200 lines - main service)
├── goals.go                 (200 lines - goal management)
├── tasks.go                 (150 lines - task queue)
└── suggestions.go           (150 lines - proactive suggestions)
```

---

## Refactoring Principles

### 1. Zero Breaking Changes
- All public APIs remain identical
- Internal restructuring only
- Comprehensive test coverage before/during/after

### 2. Incremental Approach
- One file/package at a time
- Each step builds and passes tests
- No "big bang" refactoring

### 3. Preserve Git History
- Use `git mv` for file moves
- Extract methods, don't rewrite
- Maintain commit history

### 4. Test First
- Add tests for current behavior
- Verify tests pass after refactor
- No regression in functionality

### 5. Document Changes
- Update package docs
- Add migration guide if needed
- Keep CHANGELOG updated

---

## Estimated Effort

| Phase | File | From | To | Effort | Risk |
|-------|------|------|-----|--------|------|
| 1 | server.go | 2,163 | ~200 | 4-6 hours | Low |
| 2 | Agent loop family | 5,838 | ~2,500 | 8-12 hours | Medium |
| 3 | evolution/engine.go | 1,121 | ~600 | 3-4 hours | Low |
| 4 | tools/plan.go | 1,087 | ~600 | 3-4 hours | Low |
| 5 | memory/db_semantic.go | 980 | ~600 | 3-4 hours | Low |
| 6 | config/config.go | 956 | ~600 | 2-3 hours | Low |
| 7 | autonomy/service.go | 813 | ~500 | 2-3 hours | Low |

**Total: ~25-36 hours of focused refactoring work**

---

## Risks and Mitigations

### Risk 1: Breaking Changes
**Mitigation:** Comprehensive tests, API compatibility checks, gradual rollout

### Risk 2: Git History Loss
**Mitigation:** Use `git mv`, avoid deleting/recreating files

### Risk 3: Regression Bugs
**Mitigation:** Test before/during/after, manual QA, staging environment

### Risk 4: Developer Confusion
**Mitigation:** Clear documentation, migration guide, communication

---

## Success Metrics

1. **Lines per file:** No file >500 lines (down from 2,163 max)
2. **Functions per file:** No file >20 functions (down from 55 max)
3. **Test coverage:** Maintain or improve current coverage
4. **Build time:** No regression in build/test times
5. **Developer feedback:** Easier to navigate and modify code

---

## Next Steps

1. **Start with Phase 1** (server.go) - highest impact, lowest risk
2. **Create handler packages** with clear domain separation
3. **Test thoroughly** after each phase
4. **Document changes** for the team
5. **Iterate** based on feedback

---

**Plan Created:** April 5, 2026  
**Estimated Completion:** 3-5 days of focused work  
**Status:** Ready to begin
