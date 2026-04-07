# Streamlined Goals Workflow

**Date:** 2026-04-07
**Status:** Approved

## Overview

Replace the ad-hoc step-by-step goal execution with a plan-first, parallel-dispatch pipeline. When a user creates a goal, Sofia automatically generates a structured plan with dependency-aware tasks, spawns subagents to execute independent tasks in parallel, and on completion produces a structured result with deployment instructions. Two new web UI pages provide real-time visibility (Activity) and permanent result archives (Completed).

## Requirements

- **Fully automatic**: goal creation triggers plan generation and execution with no user approval gate
- **Maximum parallelism**: all independent tasks dispatch simultaneously; dependencies are respected
- **Full execution logs**: completed goals show summary, deliverables, next steps, plus complete step-by-step agent logs
- **Separate Activity page**: live view of all running agents/subagents across all goals
- **Reuse existing autonomy LLM**: extend `buildGoalPlannerPrompt` for upfront plan generation
- **Permanent storage**: completed goals and logs persist forever, manual deletion only

## Architecture

### End-to-End Flow

```
User creates goal (Web UI / Telegram / CLI)
        |
        v
GoalManager.AddGoal()  ->  status: "active"
        |
        v
Autonomy service detects new active goal (next tick)
        |
        v
Phase 1: PLAN GENERATION
  - Extended buildGoalPlannerPrompt() asks for full plan with dependencies
  - LLM returns structured plan JSON
  - PlanManager.CreatePlanForGoal() stores plan with DependsOn edges
  - Goal status -> "in_progress"
  - Broadcast: goal_plan_created
        |
        v
Phase 2: PARALLEL DISPATCH
  - PlanManager.ReadySteps() finds steps with no unmet dependencies
  - For each ready step: SubagentManager.Spawn() with AsyncCallback
  - Step status -> "in_progress", AssignedTo -> subagent ID
  - Broadcast: goal_step_start per step
        |
        v
Subagents execute concurrently (existing tool loop)
        |
        v
Phase 3: COMPLETION CASCADE
  - Subagent finishes -> AsyncCallback fires
  - PlanManager.CompleteStep() marks step done + logs to goal_log
  - Broadcast: goal_step_end
  - Re-scan: PlanManager.ReadySteps() for newly unblocked steps
  - Dispatch any newly ready steps (back to Phase 2)
  - Repeat until no more steps
        |
        v
Phase 4: GOAL FINALIZATION
  - All steps completed -> one final LLM call
  - LLM generates GoalResult (summary, artifacts, next_steps)
  - GoalResult stored in goal's semantic node properties
  - Goal status -> "completed"
  - Broadcast: goal_completed
  - Notify user via push + active channel
        |
        v
Web UI: Activity page shows live progress throughout
Web UI: Completed page shows final result with full log
```

## Data Model Changes

### PlanStep -- add dependency tracking

```go
type PlanStep struct {
    Index       int        `json:"index"`
    Description string     `json:"description"`
    Status      PlanStatus `json:"status"`
    Result      string     `json:"result,omitempty"`
    SubPlanID   string     `json:"sub_plan_id,omitempty"`
    AssignedTo  string     `json:"assigned_to,omitempty"`
    DependsOn   []int      `json:"depends_on,omitempty"`  // indices of steps this depends on
}
```

### GoalResult -- new struct for completed goals

```go
type GoalResult struct {
    Summary     string   `json:"summary"`       // what was accomplished
    Artifacts   []string `json:"artifacts"`      // file paths created/modified
    NextSteps   []string `json:"next_steps"`     // deployment instructions or manual actions
    CompletedAt string   `json:"completed_at"`   // ISO timestamp
}
```

Stored in the goal's `semantic_nodes.properties` JSON alongside existing fields. No new database tables needed.

### Goal status -- add in_progress

Current: `active`, `completed`, `failed`, `paused`

Add: `in_progress` -- a plan has been generated and subagents are executing. Distinguishes "goal exists but unplanned" (`active`) from "work underway" (`in_progress`).

### PlanManager -- new methods

```go
// ReadySteps returns step indices that are pending and have all dependencies completed.
func (pm *PlanManager) ReadySteps(planID string) []int

// CreatePlanForGoal creates a plan linked to a goal from LLM-generated step data.
func (pm *PlanManager) CreatePlanForGoal(goalID int64, goal string, steps []PlanStepDef) *Plan

type PlanStepDef struct {
    Description string `json:"description"`
    DependsOn   []int  `json:"depends_on"`
}
```

## Plan Generation

The existing `buildGoalPlannerPrompt` is extended to request a full structured plan instead of a single step. The LLM response format becomes:

```json
{
  "goal_id": 42,
  "goal_name": "Deploy monitoring stack",
  "plan": {
    "steps": [
      {"description": "Research Prometheus + Grafana setup requirements", "depends_on": []},
      {"description": "Write docker-compose.yml for monitoring stack", "depends_on": [0]},
      {"description": "Create Grafana dashboard configs", "depends_on": [0]},
      {"description": "Write deployment script", "depends_on": [1, 2]},
      {"description": "Generate deployment instructions for user", "depends_on": [3]}
    ]
  }
}
```

The prompt asks for a complete plan broken into concrete, tool-executable steps with explicit dependency indices. The autonomy service parses this and creates the plan via `PlanManager.CreatePlanForGoal()`.

### Trigger: when does plan generation run?

The autonomy service's `pursueGoals()` method is replaced with a new flow:

1. List all goals with status `active` (unplanned) -- these need plan generation
2. List all goals with status `in_progress` (have a plan) -- these need dispatch checks
3. For each `active` goal: generate plan, transition to `in_progress`
4. For each `in_progress` goal: run dispatch loop (check for ready steps, spawn subagents)

This means a goal transitions through: `active` (created) -> `in_progress` (planned, executing) -> `completed` / `failed`.

## Parallel Dispatch

Execution uses a dispatch loop:

1. `PlanManager.ReadySteps()` finds all steps where status is `pending` and all `DependsOn` entries point to completed steps
2. For each ready step, `SubagentManager.Spawn()` is called with an `AsyncCallback`
3. Step's `AssignedTo` is set to the subagent task ID, status transitions to `in_progress`
4. On completion, the callback:
   - Calls `PlanManager.CompleteStep()` with success/failure and result
   - Logs to `goal_log` via `InsertGoalLog()`
   - Broadcasts `goal_step_end` websocket event
   - Triggers an immediate re-scan for newly unblocked steps

No concurrency limit -- all independent tasks run simultaneously. Budget checks still apply per LLM call.

### Failure handling

If a step fails, it is marked `failed` on the plan. Steps depending on it remain `pending` (blocked). The plan status becomes `failed`. The user can retry failed steps from the UI.

## Goal Finalization

When all plan steps are completed, the callback makes one final LLM call to generate a `GoalResult`:

- **Summary**: what was accomplished across all steps
- **Artifacts**: file paths created or modified (gathered from step results)
- **Next steps**: deployment instructions, manual actions, URLs -- actionable items for the user

The GoalResult is stored in the goal's semantic node properties JSON and the goal status transitions to `completed`.

## Web UI: Activity Page

New page at `/activity` showing all running agents and subagents in real-time.

### Backend

`GET /api/activity` -- new endpoint in `handler_activity.go`. Returns a snapshot combining `SubagentManager.ListTasks()` with `PlanManager` state:

```json
{
  "agents": [
    {
      "agent_id": "sofia",
      "goal_id": 42,
      "goal_name": "Deploy monitoring stack",
      "plan_id": "plan-3",
      "active_tasks": [
        {
          "subagent_id": "subagent-7",
          "step_index": 1,
          "description": "Write docker-compose.yml",
          "status": "running",
          "started_at": "2026-04-07T14:32:00Z",
          "elapsed_ms": 45000
        }
      ],
      "pending_tasks": 2,
      "completed_tasks": 1,
      "total_tasks": 5
    }
  ]
}
```

### Frontend

Template at `templates/activity.html`:
- Per-goal sections with progress bar (completed/total steps)
- Task rows per active subagent: step description, agent ID, status indicator, elapsed time
- Completed tasks collapsed by default, expandable
- Real-time updates via existing `DashboardHub` websocket events

## Web UI: Completed Page

New page at `/completed` showing finished goals with full execution history.

### Backend

`GET /api/goals/completed` -- new endpoint in `handler_goals.go`. Returns completed goals joined with their plan steps and full goal logs:

```json
[
  {
    "id": 42,
    "name": "Deploy monitoring stack",
    "description": "Set up Prometheus + Grafana monitoring",
    "priority": "high",
    "completed_at": "2026-04-07T15:10:00Z",
    "result": {
      "summary": "Created a complete Docker-based monitoring stack.",
      "artifacts": ["goal-42-.../docker-compose.yml", "goal-42-.../deploy.sh"],
      "next_steps": ["Run ./deploy.sh to start the stack", "Access Grafana at localhost:3000"]
    },
    "plan": {
      "id": "plan-3",
      "steps": [
        {"index": 0, "description": "Research requirements", "status": "completed", "assigned_to": "subagent-6", "result": "..."}
      ]
    },
    "log": [
      {"step": "Research requirements", "result": "...", "success": true, "duration_ms": 12000, "created_at": "..."}
    ]
  }
]
```

### Frontend

Template at `templates/completed.html`:
- Expandable goal cards with: name, priority badge, completion date
- Result summary section
- Artifacts section with file paths
- Next Steps section with highlighted action items and copy-to-clipboard
- Execution timeline (collapsible): every step with subagent ID, duration, success/fail, full result
- Search/filter by goal name and date range

## Files Modified

| File | Change |
|------|--------|
| `pkg/autonomy/service_goals.go` | Replace step-by-step loop with plan-then-dispatch pipeline |
| `pkg/autonomy/goals.go` | Add `GoalStatusInProgress`, `GoalResult` struct |
| `pkg/tools/plan_types.go` | Add `DependsOn` to `PlanStep`, add `PlanStepDef` |
| `pkg/tools/plan_manager.go` | Add `ReadySteps()`, `CreatePlanForGoal()` |
| `pkg/web/handler_activity.go` | New: Activity page API endpoint |
| `pkg/web/handler_goals.go` | Add `GET /api/goals/completed` endpoint |
| `pkg/web/server.go` | Register new routes |
| `pkg/web/templates/activity.html` | New: Activity page template |
| `pkg/web/templates/completed.html` | New: Completed page template |
| `pkg/web/templates/` (nav/layout) | Add Activity + Completed nav links |

## Files Unchanged

| File | Reason |
|------|--------|
| `pkg/tools/subagent.go` | Used as-is; AsyncCallback is the only new integration point |
| `pkg/memory/` | No schema changes; GoalResult fits in existing properties JSON |
| `pkg/agent/loop.go` | Pass `PlanManager` reference to autonomy `Service` during `startAutonomyServices()` |

## New Websocket Event

`goal_plan_created` -- broadcast when a plan is generated for a goal. Carries plan ID, goal ID, step count. Used by the Activity page to show newly planned goals.

All other events (`goal_step_start`, `goal_step_end`, `goal_completed`, `goal_status_changed`) already exist.
