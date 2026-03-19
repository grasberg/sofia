# [Project Name] Implementation Plan

<!-- Optional YAML frontmatter
goal: "Build [feature/system] with [key capabilities]"
tech_stack: "Go, React, SQLite"
priority: "high"
-->

## Overview

Brief description of what will be built and why.

## Tech Stack

| Technology | Version | Purpose | Rationale |
|------------|---------|---------|-----------|
| Go | 1.26 | Backend logic | Performance, concurrency, existing expertise |
| SQLite | 3.45 | Database | Simplicity, embedded, no external dependency |
| React | 19 | Frontend UI | Component reuse, ecosystem |

## File Structure

```
project/
├── src/
│   ├── componentA/
│   │   ├── logic.go
│   │   └── test.go
│   └── componentB/
│       └── ui.jsx
├── docs/
│   └── design.md
└── README.md
```

## Task Breakdown

### Task 1: [Short task name]
**Agent:** [agent-id]  
**Skills:** [skill1, skill2]  
**Priority:** P0 | P1 | P2  
**Dependencies:** none | Task X  
**INPUT:** [What inputs does the agent need?]  
**OUTPUT:** [Concrete deliverable]  
**VERIFY:** [How to verify success?]

### Task 2: [Another task]
**Agent:** [agent-id]  
**Skills:** [skill1]  
**Priority:** P1  
**Dependencies:** Task 1  
**INPUT:** [Description]  
**OUTPUT:** [Description]  
**VERIFY:** [Description]

*(Add as many tasks as needed, grouped by phase or component)*

## Phase X: Verification

1. **Build verification:** `npm run build` / `go build` passes
2. **Test verification:** All tests pass (`npm test` / `go test`)
3. **Lint verification:** Code style meets standards
4. **Security scan:** No critical vulnerabilities
5. **User acceptance:** Feature works as expected

## Rollback Plan

If something goes wrong:

1. Revert code changes via git
2. Restore database backup
3. Re-deploy previous version

## Notes

Any additional context, assumptions, or decisions.

---

*This document follows the [Implementation Plan Template](IMPLEMENTATION_PLAN.md).*