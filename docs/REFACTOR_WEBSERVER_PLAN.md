# Web Server Refactoring Plan

## Current State
- **File:** `pkg/web/server.go` (2,163 lines, 55+ functions)
- **Problem:** All HTTP handlers in one file - hard to maintain, test, and extend

## Target Structure
```
pkg/web/
├── server.go              (200 lines - Server struct, routing, middleware)
├── handlers/
│   ├── chat.go           (~150 lines - chat, chatStream)
│   ├── agents.go         (~150 lines - agents, templates)
│   ├── config.go         (~100 lines - config GET/POST)
│   ├── sessions.go       (~100 lines - sessions, session detail)
│   ├── workspace.go      (~200 lines - files, file CRUD, docs)
│   ├── skills.go         (~150 lines - skills list, add, toggle)
│   ├── memory.go         (~150 lines - notes, graph, reflections)
│   ├── evolution.go      (~100 lines - status, changelog)
│   ├── cron.go           (~100 lines - cron list, toggle)
│   ├── eval.go           (~150 lines - eval runs, detail, trend)
│   ├── audit.go          (~100 lines - audit logs, approvals)
│   ├── system.go         (~100 lines - status, restart, update, reset)
│   └── search.go         (~50 lines - search)
└── middleware/
    └── auth.go           (~50 lines - authentication)
```

## Refactoring Steps

### Step 1: Create handlers package structure
- Create `pkg/web/handlers/` directory
- Move each handler group to its own file
- Update imports and method receivers

### Step 2: Update server.go
- Keep only routing setup and middleware
- Wire up handler packages
- Reduce from 2,163 to ~200 lines

### Step 3: Test thoroughly
- Ensure all routes still work
- No breaking changes to API

## Benefits
- **Maintainability:** Each handler in focused file
- **Testability:** Easy to test individual handlers
- **Readability:** Clear domain separation
- **Extensibility:** Easy to add new handlers
