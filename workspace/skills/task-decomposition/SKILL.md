---
name: task-decomposition
description: Structured method for breaking complex goals into executable micro-tasks with dependency tracking and progress reporting. Use when the user gives a large or vague objective that needs to be broken down before execution, when planning a multi-step project, or when asked to create a task plan. Triggers on phrases like "break this down", "plan this out", "what are the steps", "create a task list", or when faced with any goal too large to execute in a single action.
---

# Task Decomposition

A systematic method for breaking complex goals into small, executable tasks with
clear dependencies, verification methods, and progress tracking.

## When to Use

Activate this skill when:
- The user gives a goal that requires more than 3 steps
- The user asks you to plan or break down a project
- A goal is too vague to execute directly
- You need to coordinate multiple parallel workstreams
- The user asks for a task list, project plan, or breakdown

## Phase 1: Define the End State

Before decomposing, define what "done" looks like.

### Steps

1. Ask yourself: if this goal were perfectly achieved, what would exist that does
   not exist now?
2. Define concrete, observable indicators of completion.
3. Write these as testable statements (can answer yes/no).

### End State Document

```markdown
# End State Definition

**Goal:** [User's stated goal]
**Done when:**
- [ ] [Observable indicator 1]
- [ ] [Observable indicator 2]
- [ ] [Observable indicator 3]

**Out of scope:**
- [Things explicitly NOT part of this goal]
```

Save to `workspace/tasks/[goal-slug]/end-state.md`.

## Phase 2: Work Backwards

Starting from the end state, work backwards to identify all required tasks.

### Backward Chaining Process

1. Look at each "done when" indicator.
2. Ask: "What is the last thing I need to do to make this true?"
3. For that last step, ask: "What must be true before I can do this?"
4. Continue until you reach steps that can be done right now with no prerequisites.
5. This produces a natural dependency chain.

### Example

Goal: "Deploy a new API endpoint"

Working backwards:
- Done: endpoint is live and responding -> deploy to production
- Before deploy: tests pass -> write and run tests
- Before tests: endpoint code exists -> implement the endpoint
- Before implementation: API spec is defined -> define the API spec
- Before spec: requirements are clear -> clarify requirements

Forward order: clarify requirements -> define API spec -> implement endpoint ->
write tests -> deploy to production.

## Phase 3: Size and Refine Tasks

Each task should take 2-10 minutes to execute. Split or merge as needed.

### Per-Task Definition

```markdown
### T[N]: [Short description]

- **Description:** [What to do, specifically]
- **Dependencies:** [T1, T3, or "none"]
- **Verification:** [How to confirm this is done correctly]
- **Duration estimate:** [2-10 minutes]
- **Status:** pending
- **Parallelizable:** [yes/no — can this run alongside other tasks?]
```

### Sizing Rules

- **Too big (> 10 min):** Split into sub-tasks. A task like "build the frontend"
  should become "create component A", "create component B", "wire up routing", etc.
- **Too small (< 2 min):** Merge with an adjacent task. "Create a file" and
  "write one line to it" should be a single task.
- **Just right (2-10 min):** Can be started, completed, and verified in one focused
  effort.

## Phase 4: Build the Dependency Graph

Map out which tasks block which other tasks.

### Dependency Rules

- A task can only start when all its dependencies are `done`.
- Tasks with no mutual dependencies can run in parallel.
- Circular dependencies indicate a planning error; resolve by splitting tasks.

### Graph Format

Represent as a textual graph in the plan document:

```
T1 (no deps)
T2 (no deps)
  T3 (requires T1)
  T4 (requires T1, T2)
    T5 (requires T3, T4)
      T6 (requires T5)
```

### Identifying Parallel Opportunities

Tasks at the same depth level with no mutual dependencies can execute in parallel.
Mark these explicitly:

```
PARALLEL GROUP A: T1, T2 (no dependencies, can start immediately)
PARALLEL GROUP B: T3, T4 (both depend only on group A tasks)
SEQUENTIAL: T5 -> T6 (strict order required)
```

## Phase 5: Track Progress

Maintain a live status document as tasks are executed.

### Status Values

- `pending` — Not yet started, dependencies may not be met.
- `ready` — All dependencies are done; this task can start.
- `in_progress` — Currently being worked on.
- `done` — Completed and verified.
- `blocked` — Cannot proceed; reason documented.
- `skipped` — Determined to be unnecessary during execution.

### Progress Document

Save to `workspace/tasks/[goal-slug]/progress.md`:

```markdown
# Task Progress

**Goal:** [Goal description]
**Started:** [YYYY-MM-DD HH:MM]
**Last updated:** [YYYY-MM-DD HH:MM]

## Overview

Total: [N] | Done: [N] | In Progress: [N] | Blocked: [N] | Pending: [N]

## Task Status

| ID  | Description         | Status      | Notes          |
|-----|---------------------|-------------|----------------|
| T1  | Clarify requirements| done        | Completed 10:15|
| T2  | Define API spec     | in_progress |                |
| T3  | Implement endpoint  | pending     | Blocked by T2  |
```

### Progress Reports

When asked for a progress update, generate:

```markdown
## Progress Report — [YYYY-MM-DD HH:MM]

**Overall:** [N]% complete ([done]/[total] tasks)
**On track:** [yes/no]
**Blockers:** [list or "none"]

### Completed since last report
- T1: [description]

### Currently in progress
- T2: [description] — [what's happening]

### Next up
- T3: [description] — ready to start when T2 completes

### Risks
- [Any identified risks to timeline]
```

## Phase 6: Re-Planning

When a blocked task requires changing the plan, re-plan explicitly.

### Re-Plan Triggers

- A task is blocked and no workaround exists.
- New information changes the goal or constraints.
- A completed task reveals that subsequent tasks need adjustment.
- The user changes requirements mid-execution.

### Re-Plan Process

1. Document why re-planning is needed.
2. Identify which tasks are affected.
3. Revise or replace affected tasks.
4. Update the dependency graph.
5. Update the progress document.
6. Log the re-plan event:

```markdown
## Re-Plan [YYYY-MM-DD HH:MM]

**Trigger:** T4 blocked — external API does not support required feature
**Changes:**
- T4: removed
- T4a: added — use alternative API
- T5: updated dependencies (now depends on T4a instead of T4)
```

## Workspace File Structure

```
workspace/tasks/[goal-slug]/
  end-state.md     # Definition of done
  plan.md          # Full task list with dependencies
  progress.md      # Live progress tracking
```

## Important Rules

- **Every task must be verifiable.** If you cannot define how to check that a task
  is done, the task is too vague.
- **Dependencies must be explicit.** Never assume task order from position in the list.
- **Update progress in real time.** Stale progress documents are worse than none.
- **Re-plan, do not patch.** When the plan breaks, update the plan formally instead
  of making ad-hoc adjustments.
- **Estimate honestly.** Underestimates compound; pad by 50% if uncertain.
