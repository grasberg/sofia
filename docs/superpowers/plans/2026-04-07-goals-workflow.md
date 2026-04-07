# Streamlined Goals Workflow Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace ad-hoc step-by-step goal execution with a plan-first, parallel-dispatch pipeline that generates plans from goals, spawns subagents for independent tasks, and surfaces results in new Activity and Completed web pages.

**Architecture:** Extend the existing autonomy service's `pursueGoals()` to first generate a full plan via LLM, then dispatch ready steps to subagents in parallel via `SubagentManager.Spawn()` with `AsyncCallback`. Completion callbacks cascade through dependencies. Two new HTMX pages (Activity, Completed) consume existing websocket events.

**Tech Stack:** Go, SQLite (via modernc.org/sqlite), HTMX, Tailwind CSS, WebSocket (DashboardHub)

---

## File Structure

| File | Responsibility |
|------|---------------|
| `pkg/tools/plan_types.go` | Add `DependsOn` field to `PlanStep`, add `PlanStepDef` type |
| `pkg/tools/plan_manager.go` | Add `ReadySteps()` and `CreatePlanForGoal()` methods |
| `pkg/tools/plan_manager_test.go` (new) | Unit tests for `ReadySteps` and `CreatePlanForGoal` |
| `pkg/autonomy/goals.go` | Add `GoalStatusInProgress` constant, `GoalResult` struct, `ListGoalsByStatus()`, `SetGoalResult()` methods |
| `pkg/autonomy/goals_test.go` | Tests for new goal methods |
| `pkg/autonomy/service.go` | Add `SetPlanManager()` setter on `Service` |
| `pkg/autonomy/service_goals.go` | Replace `pursueGoals()` with plan-generate-then-dispatch pipeline |
| `pkg/autonomy/service_goals_test.go` (new) | Tests for plan generation parsing and dispatch logic |
| `pkg/agent/loop_helpers.go` | Pass `PlanManager` to autonomy service during `startAutonomyServices()` |
| `pkg/agent/loop_query.go` | Add `GetPlanManager()` and `GetActiveSubagentTasks()` query methods |
| `pkg/web/handler_activity.go` (new) | `GET /api/activity` endpoint |
| `pkg/web/handler_goals.go` | Add `GET /api/goals/completed` endpoint |
| `pkg/web/server.go` | Register new routes + embed new templates |
| `pkg/web/templates/activity.html` (new) | Activity page template |
| `pkg/web/templates/completed.html` (new) | Completed page template |
| `pkg/web/templates/layout.html` | Add Activity + Completed nav links |

---

### Task 1: Add DependsOn to PlanStep and PlanStepDef type

**Files:**
- Modify: `pkg/tools/plan_types.go`

- [ ] **Step 1: Add `DependsOn` field to `PlanStep`**

In `pkg/tools/plan_types.go`, add the `DependsOn` field to the `PlanStep` struct:

```go
// PlanStep represents a single step in a plan.
type PlanStep struct {
	Index       int        `json:"index"`
	Description string     `json:"description"`
	Status      PlanStatus `json:"status"`
	Result      string     `json:"result,omitempty"`
	SubPlanID   string     `json:"sub_plan_id,omitempty"` // Links to a child plan
	AssignedTo  string     `json:"assigned_to,omitempty"` // Agent ID working on this step
	DependsOn   []int      `json:"depends_on,omitempty"`  // Indices of steps this depends on
}
```

- [ ] **Step 2: Add `PlanStepDef` type**

Add at the bottom of `pkg/tools/plan_types.go`:

```go
// PlanStepDef is the LLM-generated definition of a plan step before it becomes a PlanStep.
type PlanStepDef struct {
	Description string `json:"description"`
	DependsOn   []int  `json:"depends_on"`
}
```

- [ ] **Step 3: Verify build**

Run: `cd /Volumes/Slaven/sofia && go build -tags stdjson ./pkg/tools/...`
Expected: Clean build, no errors.

- [ ] **Step 4: Commit**

```bash
git add pkg/tools/plan_types.go
git commit -m "feat(tools): add DependsOn to PlanStep and PlanStepDef type"
```

---

### Task 2: Add ReadySteps and CreatePlanForGoal to PlanManager

**Files:**
- Modify: `pkg/tools/plan_manager.go`
- Create: `pkg/tools/plan_manager_test.go`

- [ ] **Step 1: Write failing tests for `ReadySteps`**

Create `pkg/tools/plan_manager_test.go`:

```go
package tools

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReadySteps_NoDependencies(t *testing.T) {
	pm := NewPlanManager()
	plan := pm.CreatePlanForGoal(1, "Test goal", []PlanStepDef{
		{Description: "Step A", DependsOn: nil},
		{Description: "Step B", DependsOn: nil},
		{Description: "Step C", DependsOn: nil},
	})
	ready := pm.ReadySteps(plan.ID)
	assert.Equal(t, []int{0, 1, 2}, ready)
}

func TestReadySteps_WithDependencies(t *testing.T) {
	pm := NewPlanManager()
	plan := pm.CreatePlanForGoal(1, "Test goal", []PlanStepDef{
		{Description: "Step A", DependsOn: nil},
		{Description: "Step B", DependsOn: []int{0}},
		{Description: "Step C", DependsOn: []int{0}},
		{Description: "Step D", DependsOn: []int{1, 2}},
	})

	// Initially only step 0 is ready
	ready := pm.ReadySteps(plan.ID)
	assert.Equal(t, []int{0}, ready)

	// Complete step 0 — steps 1 and 2 become ready
	pm.CompleteStep(plan.ID, 0, true, "done")
	ready = pm.ReadySteps(plan.ID)
	assert.Equal(t, []int{1, 2}, ready)

	// Complete step 1 — step 2 still ready, step 3 still blocked
	pm.CompleteStep(plan.ID, 1, true, "done")
	ready = pm.ReadySteps(plan.ID)
	assert.Equal(t, []int{2}, ready)

	// Complete step 2 — step 3 becomes ready
	pm.CompleteStep(plan.ID, 2, true, "done")
	ready = pm.ReadySteps(plan.ID)
	assert.Equal(t, []int{3}, ready)
}

func TestReadySteps_SkipsAssigned(t *testing.T) {
	pm := NewPlanManager()
	plan := pm.CreatePlanForGoal(1, "Test goal", []PlanStepDef{
		{Description: "Step A", DependsOn: nil},
		{Description: "Step B", DependsOn: nil},
	})

	// Claim step 0
	pm.ClaimPendingStep("agent-1")

	// Only step 1 should be ready (step 0 is in_progress)
	ready := pm.ReadySteps(plan.ID)
	assert.Equal(t, []int{1}, ready)
}

func TestReadySteps_NonexistentPlan(t *testing.T) {
	pm := NewPlanManager()
	ready := pm.ReadySteps("plan-999")
	assert.Empty(t, ready)
}

func TestCreatePlanForGoal(t *testing.T) {
	pm := NewPlanManager()
	plan := pm.CreatePlanForGoal(42, "Deploy monitoring", []PlanStepDef{
		{Description: "Research", DependsOn: nil},
		{Description: "Implement", DependsOn: []int{0}},
	})

	require.NotNil(t, plan)
	assert.Equal(t, int64(42), plan.GoalID)
	assert.Equal(t, "Deploy monitoring", plan.Goal)
	assert.Equal(t, PlanStatusPending, plan.Status)
	assert.Len(t, plan.Steps, 2)
	assert.Equal(t, "Research", plan.Steps[0].Description)
	assert.Equal(t, PlanStatusPending, plan.Steps[0].Status)
	assert.Empty(t, plan.Steps[0].DependsOn)
	assert.Equal(t, []int{0}, plan.Steps[1].DependsOn)
}

func TestClaimStep(t *testing.T) {
	pm := NewPlanManager()
	plan := pm.CreatePlanForGoal(1, "Test", []PlanStepDef{
		{Description: "Step A"},
		{Description: "Step B"},
	})

	// Claim step 0
	ok := pm.ClaimStep(plan.ID, 0, "agent-x")
	assert.True(t, ok)
	assert.Equal(t, PlanStatusInProgress, plan.Steps[0].Status)
	assert.Equal(t, "agent-x", plan.Steps[0].AssignedTo)

	// Can't claim again
	ok = pm.ClaimStep(plan.ID, 0, "agent-y")
	assert.False(t, ok)

	// Claim step 1 works
	ok = pm.ClaimStep(plan.ID, 1, "agent-y")
	assert.True(t, ok)
}

func TestGetPlanByGoalID(t *testing.T) {
	pm := NewPlanManager()
	pm.CreatePlanForGoal(42, "Goal A", []PlanStepDef{
		{Description: "Step 1"},
	})
	pm.CreatePlanForGoal(99, "Goal B", []PlanStepDef{
		{Description: "Step 1"},
	})

	plan := pm.GetPlanByGoalID(42)
	require.NotNil(t, plan)
	assert.Equal(t, "Goal A", plan.Goal)

	plan = pm.GetPlanByGoalID(99)
	require.NotNil(t, plan)
	assert.Equal(t, "Goal B", plan.Goal)

	plan = pm.GetPlanByGoalID(123)
	assert.Nil(t, plan)
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Volumes/Slaven/sofia && go test -tags stdjson ./pkg/tools/ -run "TestReadySteps|TestCreatePlanForGoal|TestGetPlanByGoalID" -v`
Expected: FAIL — `ReadySteps`, `CreatePlanForGoal`, `GetPlanByGoalID` not defined.

- [ ] **Step 3: Implement `CreatePlanForGoal`**

Add to `pkg/tools/plan_manager.go`:

```go
// CreatePlanForGoal creates a plan linked to a goal from LLM-generated step definitions.
func (pm *PlanManager) CreatePlanForGoal(goalID int64, goal string, stepDefs []PlanStepDef) *Plan {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	planID := fmt.Sprintf("plan-%d", pm.nextID)
	pm.nextID++

	steps := make([]PlanStep, len(stepDefs))
	for i, def := range stepDefs {
		steps[i] = PlanStep{
			Index:       i,
			Description: def.Description,
			Status:      PlanStatusPending,
			DependsOn:   def.DependsOn,
		}
	}

	plan := &Plan{
		ID:     planID,
		Goal:   goal,
		GoalID: goalID,
		Steps:  steps,
		Status: PlanStatusPending,
	}
	pm.plans[planID] = plan

	go pm.autoSave()
	return plan
}
```

- [ ] **Step 4: Implement `ReadySteps`**

Add to `pkg/tools/plan_manager.go`:

```go
// ReadySteps returns indices of steps that are pending, unassigned, and have all
// dependencies satisfied (all DependsOn steps are completed).
func (pm *PlanManager) ReadySteps(planID string) []int {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	plan, ok := pm.plans[planID]
	if !ok {
		return nil
	}

	var ready []int
	for i, step := range plan.Steps {
		if step.Status != PlanStatusPending || step.AssignedTo != "" {
			continue
		}
		allDepsCompleted := true
		for _, dep := range step.DependsOn {
			if dep < 0 || dep >= len(plan.Steps) || plan.Steps[dep].Status != PlanStatusCompleted {
				allDepsCompleted = false
				break
			}
		}
		if allDepsCompleted {
			ready = append(ready, i)
		}
	}
	return ready
}
```

- [ ] **Step 5: Implement `GetPlanByGoalID`**

Add to `pkg/tools/plan_manager.go`:

```go
// GetPlanByGoalID returns the plan linked to a specific goal ID, or nil.
func (pm *PlanManager) GetPlanByGoalID(goalID int64) *Plan {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	for _, plan := range pm.plans {
		if plan.GoalID == goalID {
			return plan
		}
	}
	return nil
}
```

- [ ] **Step 5b: Implement `ClaimStep`**

Add to `pkg/tools/plan_manager.go`. This claims a specific step by index (unlike `ClaimPendingStep` which claims the next available globally):

```go
// ClaimStep marks a specific step as in_progress and assigns it to the given agent.
func (pm *PlanManager) ClaimStep(planID string, stepIdx int, assignee string) bool {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	plan, ok := pm.plans[planID]
	if !ok || stepIdx < 0 || stepIdx >= len(plan.Steps) {
		return false
	}
	if plan.Steps[stepIdx].Status != PlanStatusPending {
		return false
	}

	plan.Steps[stepIdx].Status = PlanStatusInProgress
	plan.Steps[stepIdx].AssignedTo = assignee
	if plan.Status == PlanStatusPending {
		plan.Status = PlanStatusInProgress
	}

	go pm.autoSave()
	return true
}
```

- [ ] **Step 6: Run tests to verify they pass**

Run: `cd /Volumes/Slaven/sofia && go test -tags stdjson ./pkg/tools/ -run "TestReadySteps|TestCreatePlanForGoal|TestGetPlanByGoalID" -v`
Expected: All PASS.

- [ ] **Step 7: Run existing plan tests to verify no regressions**

Run: `cd /Volumes/Slaven/sofia && go test -tags stdjson ./pkg/tools/ -run "TestPlan" -v`
Expected: All existing tests PASS.

- [ ] **Step 8: Commit**

```bash
git add pkg/tools/plan_manager.go pkg/tools/plan_manager_test.go
git commit -m "feat(tools): add ReadySteps, CreatePlanForGoal, GetPlanByGoalID to PlanManager"
```

---

### Task 3: Add GoalStatusInProgress, GoalResult, and new GoalManager methods

**Files:**
- Modify: `pkg/autonomy/goals.go`
- Modify: `pkg/autonomy/goals_test.go`

- [ ] **Step 1: Write failing tests**

Append to `pkg/autonomy/goals_test.go`:

```go
func TestGoalManager_ListGoalsByStatus(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	gm := NewGoalManager(db)
	agentID := "agent-1"

	_, err := gm.AddGoal(agentID, "Goal A", "desc a", "high")
	require.NoError(t, err)
	_, err = gm.AddGoal(agentID, "Goal B", "desc b", "medium")
	require.NoError(t, err)

	// Both are active
	active, err := gm.ListGoalsByStatus(agentID, GoalStatusActive)
	require.NoError(t, err)
	assert.Len(t, active, 2)

	// None in_progress yet
	inProg, err := gm.ListGoalsByStatus(agentID, GoalStatusInProgress)
	require.NoError(t, err)
	assert.Len(t, inProg, 0)
}

func TestGoalManager_SetGoalResult(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	gm := NewGoalManager(db)
	agentID := "agent-1"

	gAny, err := gm.AddGoal(agentID, "Goal A", "desc", "high")
	require.NoError(t, err)
	goal := gAny.(*Goal)

	result := GoalResult{
		Summary:     "Deployed the stack",
		Artifacts:   []string{"/workspace/goals/goal-1/docker-compose.yml"},
		NextSteps:   []string{"Run ./deploy.sh"},
		CompletedAt: "2026-04-07T15:00:00Z",
	}
	err = gm.SetGoalResult(goal.ID, result)
	require.NoError(t, err)

	// Retrieve and verify
	updated, err := gm.GetGoalByID(goal.ID)
	require.NoError(t, err)
	assert.NotNil(t, updated.GoalResult)
	assert.Equal(t, "Deployed the stack", updated.GoalResult.Summary)
	assert.Equal(t, []string{"/workspace/goals/goal-1/docker-compose.yml"}, updated.GoalResult.Artifacts)
	assert.Equal(t, []string{"Run ./deploy.sh"}, updated.GoalResult.NextSteps)
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Volumes/Slaven/sofia && go test -tags stdjson ./pkg/autonomy/ -run "TestGoalManager_ListGoalsByStatus|TestGoalManager_SetGoalResult" -v`
Expected: FAIL — types and methods not defined.

- [ ] **Step 3: Add `GoalStatusInProgress` constant and `GoalResult` struct**

In `pkg/autonomy/goals.go`, update the constants block:

```go
const (
	GoalStatusActive     = "active"
	GoalStatusInProgress = "in_progress"
	GoalStatusCompleted  = "completed"
	GoalStatusFailed     = "failed"
	GoalStatusPaused     = "paused"
)
```

Add the `GoalResult` struct and update `Goal`:

```go
// GoalResult holds the structured outcome of a completed goal.
type GoalResult struct {
	Summary     string   `json:"summary"`
	Artifacts   []string `json:"artifacts"`
	NextSteps   []string `json:"next_steps"`
	CompletedAt string   `json:"completed_at"`
}

// Goal represents a long-term user or agent objective.
type Goal struct {
	ID          int64       `json:"id"`
	AgentID     string      `json:"agent_id"`
	Name        string      `json:"name"`
	Description string      `json:"description"`
	Status      string      `json:"status"`
	Priority    string      `json:"priority"`
	Result      string      `json:"result,omitempty"`
	GoalResult  *GoalResult `json:"goal_result,omitempty"`
	CreatedAt   time.Time   `json:"created_at"`
	UpdatedAt   time.Time   `json:"updated_at"`
}
```

- [ ] **Step 4: Implement `ListGoalsByStatus`**

Add to `pkg/autonomy/goals.go`:

```go
// ListGoalsByStatus returns goals matching a specific status for an agent.
func (gm *GoalManager) ListGoalsByStatus(agentID, status string) ([]*Goal, error) {
	nodes, err := gm.memDB.FindNodes(agentID, "Goal", "", 100)
	if err != nil {
		return nil, err
	}

	var goals []*Goal
	for _, node := range nodes {
		g := parseGoalNode(&node)
		if g.Status == status {
			goals = append(goals, g)
		}
	}
	return goals, nil
}
```

- [ ] **Step 5: Implement `SetGoalResult`**

Add to `pkg/autonomy/goals.go`:

```go
// SetGoalResult stores a structured GoalResult in the goal's properties.
func (gm *GoalManager) SetGoalResult(goalID int64, result GoalResult) error {
	node, err := gm.memDB.GetNodeByID(goalID)
	if err != nil {
		return err
	}
	if node == nil || node.Label != "Goal" {
		return fmt.Errorf("goal %d not found", goalID)
	}

	var props map[string]any
	if err := json.Unmarshal([]byte(node.Properties), &props); err != nil {
		props = make(map[string]any)
	}

	resultJSON, err := json.Marshal(result)
	if err != nil {
		return fmt.Errorf("failed to marshal goal result: %w", err)
	}
	props["goal_result"] = json.RawMessage(resultJSON)

	propsJSON, _ := json.Marshal(props)
	_, err = gm.memDB.UpsertNode(node.AgentID, "Goal", node.Name, string(propsJSON))
	return err
}
```

- [ ] **Step 6: Update `parseGoalNode` to extract `GoalResult`**

Replace the `parseGoalNode` function in `pkg/autonomy/goals.go`:

```go
func parseGoalNode(node *memory.SemanticNode) *Goal {
	g := &Goal{
		ID:        node.ID,
		AgentID:   node.AgentID,
		Name:      node.Name,
		CreatedAt: node.CreatedAt,
		UpdatedAt: node.UpdatedAt,
	}

	var props map[string]json.RawMessage
	if err := json.Unmarshal([]byte(node.Properties), &props); err == nil {
		if v, ok := props["description"]; ok {
			json.Unmarshal(v, &g.Description)
		}
		if v, ok := props["status"]; ok {
			json.Unmarshal(v, &g.Status)
		}
		if v, ok := props["priority"]; ok {
			json.Unmarshal(v, &g.Priority)
		}
		if v, ok := props["result"]; ok {
			json.Unmarshal(v, &g.Result)
		}
		if v, ok := props["goal_result"]; ok {
			var gr GoalResult
			if json.Unmarshal(v, &gr) == nil {
				g.GoalResult = &gr
			}
		}
	}
	return g
}
```

Note: The properties JSON used `map[string]string` before. The existing `AddGoal` and `UpdateGoalStatus` methods write `map[string]string`. We change the parser to use `map[string]json.RawMessage` which handles both string values (they unmarshal as quoted JSON strings) and the nested `goal_result` object. The write paths (`AddGoal`, `UpdateGoalStatus`, `UpdateGoalResult`) continue to use `map[string]string` for simple fields, which is compatible. `SetGoalResult` writes `goal_result` as a JSON object via `map[string]any`.

- [ ] **Step 7: Run tests to verify they pass**

Run: `cd /Volumes/Slaven/sofia && go test -tags stdjson ./pkg/autonomy/ -run "TestGoalManager" -v`
Expected: All PASS, including existing tests.

- [ ] **Step 8: Commit**

```bash
git add pkg/autonomy/goals.go pkg/autonomy/goals_test.go
git commit -m "feat(autonomy): add GoalStatusInProgress, GoalResult, ListGoalsByStatus, SetGoalResult"
```

---

### Task 4: Add SetPlanManager to autonomy Service

**Files:**
- Modify: `pkg/autonomy/service.go`
- Modify: `pkg/agent/loop_helpers.go`

- [ ] **Step 1: Add `planManager` field and setter to `Service`**

In `pkg/autonomy/service.go`, add to the `Service` struct:

```go
planManager interface {
	CreatePlanForGoal(goalID int64, goal string, steps []tools.PlanStepDef) *tools.Plan
	ReadySteps(planID string) []int
	CompleteStep(planID string, stepIdx int, success bool, result string)
	GetPlanByGoalID(goalID int64) *tools.Plan
	ClaimPendingStep(agentID string) (string, int, string, bool)
}
```

Note: Use an interface rather than importing `*tools.PlanManager` directly to avoid a circular dependency (tools imports nothing from autonomy, and we keep it that way). The `Service` doesn't need the full `PlanManager` — only these methods.

Add the setter:

```go
// PlanManagerAPI is the subset of PlanManager methods needed by the autonomy service.
type PlanManagerAPI interface {
	CreatePlanForGoal(goalID int64, goal string, steps []PlanStepDef) *Plan
	ReadySteps(planID string) []int
	CompleteStep(planID string, stepIdx int, success bool, result string)
	GetPlanByGoalID(goalID int64) *Plan
}
```

Wait — `PlanStepDef` and `Plan` live in `pkg/tools`. The autonomy package already imports `pkg/tools` (for `SubagentManager`). So we can reference the concrete types directly.

In `pkg/autonomy/service.go`, add to the `Service` struct, after `taskRunner`:

```go
planMgr *tools.PlanManager
```

Add the setter below the other `Set*` methods:

```go
// SetPlanManager sets the plan manager for goal-to-plan pipeline.
func (s *Service) SetPlanManager(pm *tools.PlanManager) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.planMgr = pm
}
```

- [ ] **Step 2: Wire PlanManager in `startAutonomyServices`**

In `pkg/agent/loop_helpers.go`, after `svc.SetTaskRunner(al.runSpawnedTaskAsAgent)` (line 49), add:

```go
svc.SetPlanManager(al.planManager)
```

- [ ] **Step 3: Verify build**

Run: `cd /Volumes/Slaven/sofia && go build -tags stdjson ./pkg/...`
Expected: Clean build.

- [ ] **Step 4: Commit**

```bash
git add pkg/autonomy/service.go pkg/agent/loop_helpers.go
git commit -m "feat(autonomy): add SetPlanManager to wire PlanManager into autonomy service"
```

---

### Task 5: Replace pursueGoals with plan-then-dispatch pipeline

**Files:**
- Modify: `pkg/autonomy/service_goals.go`
- Create: `pkg/autonomy/service_goals_test.go` (we'll add to the existing test file)

This is the core task. The new `pursueGoals` does two things:
1. For `active` goals (no plan yet): generate a plan via LLM, create it in PlanManager, transition goal to `in_progress`
2. For `in_progress` goals: find ready steps, spawn subagents, wire callbacks

- [ ] **Step 1: Write test for plan generation prompt parsing**

Append to `pkg/autonomy/service_goal_step_test.go`:

```go
func TestParseGoalPlanResponse_Valid(t *testing.T) {
	input := `{"goal_id": 42, "goal_name": "Deploy stack", "plan": {"steps": [{"description": "Research", "depends_on": []}, {"description": "Build", "depends_on": [0]}]}}`
	resp, err := parseGoalPlanResponse(input)
	require.NoError(t, err)
	assert.Equal(t, int64(42), resp.GoalID)
	assert.Equal(t, "Deploy stack", resp.GoalName)
	require.Len(t, resp.Steps, 2)
	assert.Equal(t, "Research", resp.Steps[0].Description)
	assert.Empty(t, resp.Steps[0].DependsOn)
	assert.Equal(t, "Build", resp.Steps[1].Description)
	assert.Equal(t, []int{0}, resp.Steps[1].DependsOn)
}

func TestParseGoalPlanResponse_CodeFenced(t *testing.T) {
	input := "```json\n{\"goal_id\": 1, \"goal_name\": \"Test\", \"plan\": {\"steps\": [{\"description\": \"Do it\", \"depends_on\": []}]}}\n```"
	resp, err := parseGoalPlanResponse(input)
	require.NoError(t, err)
	assert.Equal(t, int64(1), resp.GoalID)
	require.Len(t, resp.Steps, 1)
}

func TestParseGoalPlanResponse_Invalid(t *testing.T) {
	_, err := parseGoalPlanResponse("{bad json}")
	require.Error(t, err)
}

func TestParseGoalPlanResponse_NoSteps(t *testing.T) {
	input := `{"goal_id": 1, "goal_name": "Test", "plan": {"steps": []}}`
	_, err := parseGoalPlanResponse(input)
	require.Error(t, err)
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Volumes/Slaven/sofia && go test -tags stdjson ./pkg/autonomy/ -run "TestParseGoalPlanResponse" -v`
Expected: FAIL — `parseGoalPlanResponse` not defined.

- [ ] **Step 3: Write test for finalization result parsing**

Append to `pkg/autonomy/service_goal_step_test.go`:

```go
func TestParseGoalResultResponse_Valid(t *testing.T) {
	input := `{"summary": "Done", "artifacts": ["/a.txt"], "next_steps": ["Deploy it"]}`
	result, err := parseGoalResultResponse(input)
	require.NoError(t, err)
	assert.Equal(t, "Done", result.Summary)
	assert.Equal(t, []string{"/a.txt"}, result.Artifacts)
	assert.Equal(t, []string{"Deploy it"}, result.NextSteps)
}

func TestParseGoalResultResponse_Invalid(t *testing.T) {
	_, err := parseGoalResultResponse("{bad}")
	require.Error(t, err)
}
```

- [ ] **Step 4: Run tests to verify they fail**

Run: `cd /Volumes/Slaven/sofia && go test -tags stdjson ./pkg/autonomy/ -run "TestParseGoalResultResponse" -v`
Expected: FAIL — `parseGoalResultResponse` not defined.

- [ ] **Step 5: Rewrite `service_goals.go`**

Replace the entire contents of `pkg/autonomy/service_goals.go` with the new plan-then-dispatch pipeline. The file is self-contained — it only modifies methods on `*Service`:

```go
package autonomy

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/grasberg/sofia/pkg/logger"
	"github.com/grasberg/sofia/pkg/providers"
	"github.com/grasberg/sofia/pkg/tools"
)

var goalSlugRe = regexp.MustCompile(`[^a-zA-Z0-9]+`)

// pursueGoals implements the plan-first, parallel-dispatch pipeline.
// Phase 1: For each active (unplanned) goal — generate a plan via LLM.
// Phase 2: For each in_progress goal — dispatch ready steps to subagents.
func (s *Service) pursueGoals(ctx context.Context) {
	gm := NewGoalManager(s.memDB)

	// Phase 1: Generate plans for active (unplanned) goals
	activeGoals, err := gm.ListGoalsByStatus(s.agentID, GoalStatusActive)
	if err != nil {
		logger.WarnCF("autonomy", "Failed to list active goals", map[string]any{"error": err.Error()})
		return
	}
	for _, goal := range activeGoals {
		select {
		case <-ctx.Done():
			return
		default:
		}
		s.generatePlanForGoal(ctx, gm, goal)
	}

	// Phase 2: Dispatch ready steps for in_progress goals
	inProgressGoals, err := gm.ListGoalsByStatus(s.agentID, GoalStatusInProgress)
	if err != nil {
		logger.WarnCF("autonomy", "Failed to list in_progress goals", map[string]any{"error": err.Error()})
		return
	}
	for _, goal := range inProgressGoals {
		select {
		case <-ctx.Done():
			return
		default:
		}
		s.dispatchReadySteps(ctx, gm, goal)
	}
}

// goalPlanResponse is the expected LLM response for plan generation.
type goalPlanResponse struct {
	GoalID   int64  `json:"goal_id"`
	GoalName string `json:"goal_name"`
	Plan     struct {
		Steps []tools.PlanStepDef `json:"steps"`
	} `json:"plan"`
	Steps []tools.PlanStepDef `json:"steps"` // alias — accept steps at top level too
}

func parseGoalPlanResponse(content string) (*goalPlanResponse, error) {
	trimmed := strings.TrimSpace(content)
	cleaned := strings.TrimSpace(
		strings.TrimSuffix(strings.TrimPrefix(strings.TrimPrefix(trimmed, "```json"), "```"), "```"),
	)

	var resp goalPlanResponse
	if err := json.Unmarshal([]byte(cleaned), &resp); err != nil {
		return nil, fmt.Errorf("failed to parse plan response: %w", err)
	}

	// Accept steps at top level as fallback
	if len(resp.Plan.Steps) == 0 && len(resp.Steps) > 0 {
		resp.Plan.Steps = resp.Steps
	}

	if len(resp.Plan.Steps) == 0 {
		return nil, fmt.Errorf("plan has no steps")
	}
	return &resp, nil
}

// goalResultResponse is the expected LLM response for goal finalization.
type goalResultResponse struct {
	Summary   string   `json:"summary"`
	Artifacts []string `json:"artifacts"`
	NextSteps []string `json:"next_steps"`
}

func parseGoalResultResponse(content string) (*goalResultResponse, error) {
	trimmed := strings.TrimSpace(content)
	cleaned := strings.TrimSpace(
		strings.TrimSuffix(strings.TrimPrefix(strings.TrimPrefix(trimmed, "```json"), "```"), "```"),
	)

	var resp goalResultResponse
	if err := json.Unmarshal([]byte(cleaned), &resp); err != nil {
		return nil, fmt.Errorf("failed to parse goal result: %w", err)
	}
	return &resp, nil
}

func (s *Service) buildPlanGenerationPrompt(goal *Goal) string {
	goalDir := s.ensureGoalFolder(goal.ID, goal.Name)
	return fmt.Sprintf(`You are an autonomous AI agent. Generate a complete execution plan for the following goal.

Goal [ID:%d]: %s
Description: %s
Priority: %s

Create a structured plan with concrete, actionable steps. Each step must be achievable with available tools (read_file, write_file, exec, edit_file, list_dir, append_file).

Rules:
- Break the goal into 3-10 concrete steps
- Each step must be specific and self-contained
- Include dependency indices — which steps must complete before this step can start
- Steps with no dependencies can run in parallel
- All file operations must use absolute paths under the goal folder: %s
- The final step should always produce a summary or deployment instructions

Respond in this exact JSON format (no markdown, no code fences):
{"goal_id": %d, "goal_name": "%s", "plan": {"steps": [{"description": "step text", "depends_on": [0]}, ...]}}

depends_on is an array of 0-based step indices that must complete first. Use [] for steps with no dependencies.`,
		goal.ID, goal.Name, goal.Description, goal.Priority, goalDir, goal.ID, goal.Name)
}

func (s *Service) generatePlanForGoal(ctx context.Context, gm *GoalManager, goal *Goal) {
	s.mu.Lock()
	pm := s.planMgr
	s.mu.Unlock()

	if pm == nil {
		logger.WarnCF("autonomy", "PlanManager not configured, skipping plan generation", nil)
		return
	}

	// Check if plan already exists for this goal
	if existing := pm.GetPlanByGoalID(goal.ID); existing != nil {
		// Already has a plan — transition to in_progress
		gm.UpdateGoalStatus(goal.ID, GoalStatusInProgress)
		return
	}

	if !s.checkBudget() {
		return
	}

	logger.InfoCF("autonomy", "Generating plan for goal", map[string]any{
		"goal_id":   goal.ID,
		"goal_name": goal.Name,
	})

	s.broadcast(map[string]any{
		"type":      "goal_evaluation_start",
		"agent_id":  s.agentID,
		"goal_id":   goal.ID,
		"goal_name": goal.Name,
	})

	prompt := s.buildPlanGenerationPrompt(goal)
	messages := []providers.Message{{Role: "user", Content: prompt}}

	resp, err := s.provider.Chat(ctx, messages, nil, s.modelID, map[string]any{
		"max_tokens":  1000,
		"temperature": 0.3,
	})
	if err != nil {
		logger.WarnCF("autonomy", "Plan generation LLM call failed", map[string]any{
			"goal_id": goal.ID,
			"error":   err.Error(),
		})
		return
	}
	if resp.Usage != nil {
		s.trackCost(resp.Usage.TotalTokens)
	}

	planResp, err := parseGoalPlanResponse(resp.Content)
	if err != nil {
		logger.WarnCF("autonomy", "Failed to parse plan response", map[string]any{
			"goal_id": goal.ID,
			"error":   err.Error(),
			"content": truncate(resp.Content, 300),
		})
		return
	}

	plan := pm.CreatePlanForGoal(goal.ID, goal.Name, planResp.Plan.Steps)

	// Transition goal to in_progress
	gm.UpdateGoalStatus(goal.ID, GoalStatusInProgress)

	logger.InfoCF("autonomy", "Plan generated for goal", map[string]any{
		"goal_id":    goal.ID,
		"goal_name":  goal.Name,
		"plan_id":    plan.ID,
		"step_count": len(plan.Steps),
	})

	s.broadcast(map[string]any{
		"type":       "goal_plan_created",
		"agent_id":   s.agentID,
		"goal_id":    goal.ID,
		"goal_name":  goal.Name,
		"plan_id":    plan.ID,
		"step_count": len(plan.Steps),
	})

	s.notifyUser(fmt.Sprintf("📋 Plan created for *%s* — %d steps", goal.Name, len(plan.Steps)))
}

func (s *Service) dispatchReadySteps(ctx context.Context, gm *GoalManager, goal *Goal) {
	s.mu.Lock()
	pm := s.planMgr
	subMgr := s.subMgr
	s.mu.Unlock()

	if pm == nil || subMgr == nil {
		return
	}

	plan := pm.GetPlanByGoalID(goal.ID)
	if plan == nil {
		return
	}

	// Check if plan is already completed or failed
	if plan.Status == tools.PlanStatusCompleted {
		s.finalizeGoal(ctx, gm, goal, plan)
		return
	}
	if plan.Status == tools.PlanStatusFailed {
		return
	}

	readyIndices := pm.ReadySteps(plan.ID)
	if len(readyIndices) == 0 {
		return
	}

	for _, stepIdx := range readyIndices {
		select {
		case <-ctx.Done():
			return
		default:
		}

		if !s.checkBudget() {
			return
		}

		step := plan.Steps[stepIdx]
		goalDir := s.ensureGoalFolder(goal.ID, goal.Name)

		taskPrompt := fmt.Sprintf(`You are working toward goal: "%s"

Your task: %s

CRITICAL RULES:
- You MUST use tool calls (read_file, write_file, exec, list_dir, etc.) to do real work.
- All file operations for this goal MUST use absolute paths under the goal folder: %s
- This folder has already been created for you. Save all output files there.
- Do NOT just describe what you would do. Actually do it with tools.
- Every response must contain at least one tool call unless the step is purely informational.
- When done, summarize what you actually accomplished (files created, commands run, results).`,
			goal.Name, step.Description, goalDir)

		// Claim the specific step
		label := fmt.Sprintf("goal-%d-step-%d", goal.ID, stepIdx)
		if !pm.ClaimStep(plan.ID, stepIdx, label) {
			continue // Already claimed by another dispatch
		}

		stepIdxCopy := stepIdx
		goalCopy := *goal
		planID := plan.ID

		s.broadcast(map[string]any{
			"type":       "goal_step_start",
			"agent_id":   s.agentID,
			"goal_id":    goal.ID,
			"goal_name":  goal.Name,
			"step":       step.Description,
			"step_index": stepIdx,
		})

		callback := func(cbCtx context.Context, result *tools.ToolResult) {
			success := result != nil && !result.IsError
			resultText := ""
			if result != nil {
				resultText = result.ForLLM
			}

			start := time.Now()
			pm.CompleteStep(planID, stepIdxCopy, success, truncate(resultText, 2000))

			dur := time.Since(start).Milliseconds()
			if s.memDB != nil {
				_ = s.memDB.InsertGoalLog(goalCopy.ID, s.agentID, step.Description, resultText, success, dur)
			}

			s.broadcast(map[string]any{
				"type":        "goal_step_end",
				"agent_id":    s.agentID,
				"goal_id":     goalCopy.ID,
				"goal_name":   goalCopy.Name,
				"step":        step.Description,
				"step_index":  stepIdxCopy,
				"success":     success,
				"result":      truncate(resultText, 500),
				"duration_ms": dur,
			})

			if success {
				s.notifyUser(fmt.Sprintf("✅ *%s* step %d done: %s", goalCopy.Name, stepIdxCopy+1, truncate(step.Description, 100)))
			} else {
				s.notifyUser(fmt.Sprintf("❌ *%s* step %d failed: %s", goalCopy.Name, stepIdxCopy+1, truncate(step.Description, 100)))
			}

			// Re-scan for newly unblocked steps
			updatedPlan := pm.GetPlanByGoalID(goalCopy.ID)
			if updatedPlan != nil && updatedPlan.Status == tools.PlanStatusCompleted {
				s.finalizeGoal(cbCtx, gm, &goalCopy, updatedPlan)
			} else {
				s.dispatchReadySteps(cbCtx, gm, &goalCopy)
			}
		}

		_, err := subMgr.Spawn(ctx, taskPrompt, label, "", nil, "system", "autonomy", callback)
		if err != nil {
			logger.WarnCF("autonomy", "Failed to spawn subagent for goal step", map[string]any{
				"goal_id":    goal.ID,
				"step_index": stepIdx,
				"error":      err.Error(),
			})
		}
	}
}

func (s *Service) finalizeGoal(ctx context.Context, gm *GoalManager, goal *Goal, plan *tools.Plan) {
	// Gather all step results for the finalization prompt
	var stepResults strings.Builder
	for _, step := range plan.Steps {
		status := "completed"
		if step.Status == tools.PlanStatusFailed {
			status = "failed"
		}
		fmt.Fprintf(&stepResults, "- Step %d [%s]: %s\n  Result: %s\n",
			step.Index+1, status, step.Description, truncate(step.Result, 300))
	}

	goalDir := s.goalFolderPath(goal.ID, goal.Name)

	prompt := fmt.Sprintf(`A goal has been completed. Summarize the results for the user.

Goal: %s
Description: %s
Goal folder: %s

Step results:
%s

Respond in this exact JSON format (no markdown, no code fences):
{"summary": "what was accomplished", "artifacts": ["list of file paths created"], "next_steps": ["actionable instructions for the user to deploy or use the results"]}`,
		goal.Name, goal.Description, goalDir, stepResults.String())

	if !s.checkBudget() {
		// Still complete the goal even if we can't generate a fancy result
		gm.UpdateGoalStatus(goal.ID, GoalStatusCompleted)
		return
	}

	messages := []providers.Message{{Role: "user", Content: prompt}}
	resp, err := s.provider.Chat(ctx, messages, nil, s.modelID, map[string]any{
		"max_tokens":  500,
		"temperature": 0.3,
	})

	if resp != nil && resp.Usage != nil {
		s.trackCost(resp.Usage.TotalTokens)
	}

	if err == nil {
		parsed, parseErr := parseGoalResultResponse(resp.Content)
		if parseErr == nil {
			result := GoalResult{
				Summary:     parsed.Summary,
				Artifacts:   parsed.Artifacts,
				NextSteps:   parsed.NextSteps,
				CompletedAt: time.Now().UTC().Format(time.RFC3339),
			}
			_ = gm.SetGoalResult(goal.ID, result)
		}
	}

	gm.UpdateGoalStatus(goal.ID, GoalStatusCompleted)

	logger.InfoCF("autonomy", "Goal completed", map[string]any{
		"goal_id":   goal.ID,
		"goal_name": goal.Name,
	})

	s.broadcast(map[string]any{
		"type":      "goal_completed",
		"agent_id":  s.agentID,
		"goal_id":   goal.ID,
		"goal_name": goal.Name,
	})

	s.notifyUser(fmt.Sprintf("🏁 Goal completed: *%s*", goal.Name))

	if s.push != nil {
		_ = s.push.Send(
			fmt.Sprintf("Sofia: Goal Completed — %s", goal.Name),
			"All steps finished. Check the Completed page for results.",
		)
	}
}

// goalFolderName returns a filesystem-safe folder name for a goal.
func goalFolderName(goalID int64, goalName string) string {
	slug := strings.ToLower(strings.TrimSpace(goalSlugRe.ReplaceAllString(goalName, "-")))
	slug = strings.Trim(slug, "-")
	if len(slug) > 50 {
		slug = slug[:50]
	}
	if slug == "" {
		slug = "goal"
	}
	return fmt.Sprintf("goal-%d-%s", goalID, slug)
}

// goalFolderPath returns the absolute path for a goal's working directory.
func (s *Service) goalFolderPath(goalID int64, goalName string) string {
	return filepath.Join(s.workspace, "goals", goalFolderName(goalID, goalName))
}

// ensureGoalFolder creates the goal folder if it doesn't exist.
func (s *Service) ensureGoalFolder(goalID int64, goalName string) string {
	dir := s.goalFolderPath(goalID, goalName)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		logger.WarnCF("autonomy", "Failed to create goal folder", map[string]any{
			"path":  dir,
			"error": err.Error(),
		})
	}
	return dir
}
```

- [ ] **Step 6: Run new parsing tests**

Run: `cd /Volumes/Slaven/sofia && go test -tags stdjson ./pkg/autonomy/ -run "TestParseGoalPlanResponse|TestParseGoalResultResponse" -v`
Expected: All PASS.

- [ ] **Step 7: Run all autonomy tests**

Run: `cd /Volumes/Slaven/sofia && go test -tags stdjson ./pkg/autonomy/ -v`
Expected: All PASS. Note: `TestExecuteOneGoalStep_GoalComplete` and `TestExecuteOneGoalStep_UsesTaskRunner` will fail because they test the old `executeOneGoalStep` method which no longer exists. Update those tests:

Remove or update the tests in `service_goal_step_test.go` that reference `executeOneGoalStep`. Keep `TestBuildGoalsSummary` and `TestParseGoalPlannerResponse` (the old parser is gone but we have new parsers). The old `TestParseGoalPlannerResponse` should be removed since `parseGoalPlannerResponse` no longer exists.

Replace the old tests with updated versions that test the new functions. Remove `TestBuildGoalsSummary`, `TestParseGoalPlannerResponse`, `TestExecuteOneGoalStep_GoalComplete`, `TestExecuteOneGoalStep_UsesTaskRunner`, and the `maxInt64` and `decodeGoal` helpers since they tested the old API.

- [ ] **Step 8: Verify build**

Run: `cd /Volumes/Slaven/sofia && go build -tags stdjson ./pkg/...`
Expected: Clean build.

- [ ] **Step 9: Commit**

```bash
git add pkg/autonomy/service_goals.go pkg/autonomy/service_goal_step_test.go
git commit -m "feat(autonomy): replace step-by-step goal execution with plan-then-dispatch pipeline"
```

---

### Task 6: Add query methods for Activity page data

**Files:**
- Modify: `pkg/agent/loop_query.go`

- [ ] **Step 1: Add `GetPlanManager` and `GetActiveSubagentTasks` methods**

Append to `pkg/agent/loop_query.go`:

```go
// GetPlanManager returns the plan manager. Used by web handlers for activity/completed data.
func (al *AgentLoop) GetPlanManager() *tools.PlanManager {
	return al.planManager
}

// GetActiveSubagentTasks returns running subagent tasks across all autonomy services.
func (al *AgentLoop) GetActiveSubagentTasks() []map[string]any {
	al.autonomyMu.Lock()
	defer al.autonomyMu.Unlock()

	var tasks []map[string]any
	for agentID, svc := range al.autonomyServices {
		if svc == nil {
			continue
		}
		subMgr := svc.GetSubagentManager()
		if subMgr == nil {
			continue
		}
		for _, task := range subMgr.ListTasks() {
			tasks = append(tasks, map[string]any{
				"agent_id":    agentID,
				"subagent_id": task.ID,
				"task":        task.Task,
				"label":       task.Label,
				"status":      task.Status,
				"created":     task.Created,
			})
		}
	}
	return tasks
}
```

- [ ] **Step 2: Add `GetSubagentManager` to autonomy Service**

In `pkg/autonomy/service.go`, add:

```go
// GetSubagentManager returns the subagent manager for this service.
func (s *Service) GetSubagentManager() *tools.SubagentManager {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.subMgr
}
```

- [ ] **Step 3: Verify build**

Run: `cd /Volumes/Slaven/sofia && go build -tags stdjson ./pkg/...`
Expected: Clean build.

- [ ] **Step 4: Commit**

```bash
git add pkg/agent/loop_query.go pkg/autonomy/service.go
git commit -m "feat(agent): add GetPlanManager and GetActiveSubagentTasks query methods"
```

---

### Task 7: Add GET /api/goals/completed endpoint

**Files:**
- Modify: `pkg/web/handler_goals.go`

- [ ] **Step 1: Add `handleGoalsCompleted` handler**

Append to `pkg/web/handler_goals.go`:

```go
// handleGoalsCompleted returns completed goals with full execution logs and plan data.
func (s *Server) handleGoalsCompleted(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.sendJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	goals, err := s.agentLoop.ListGoals("")
	if err != nil {
		s.sendJSONError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	memDB := s.agentLoop.GetMemoryDB()
	pm := s.agentLoop.GetPlanManager()

	var completed []map[string]any
	for _, g := range goals {
		if g.Status != "completed" {
			continue
		}

		entry := map[string]any{
			"id":          g.ID,
			"name":        g.Name,
			"description": g.Description,
			"priority":    g.Priority,
			"result":      g.Result,
			"goal_result": g.GoalResult,
			"created_at":  g.CreatedAt,
			"updated_at":  g.UpdatedAt,
		}

		// Attach plan data if available
		if pm != nil {
			if plan := pm.GetPlanByGoalID(g.ID); plan != nil {
				steps := make([]map[string]any, len(plan.Steps))
				for i, s := range plan.Steps {
					steps[i] = map[string]any{
						"index":       s.Index,
						"description": s.Description,
						"status":      string(s.Status),
						"result":      s.Result,
						"assigned_to": s.AssignedTo,
						"depends_on":  s.DependsOn,
					}
				}
				entry["plan"] = map[string]any{
					"id":     plan.ID,
					"status": string(plan.Status),
					"steps":  steps,
				}
			}
		}

		// Attach goal log
		if memDB != nil {
			entries, err := memDB.GetGoalLog(g.ID)
			if err == nil && entries != nil {
				entry["log"] = entries
			}
		}

		completed = append(completed, entry)
	}

	if completed == nil {
		completed = []map[string]any{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(completed)
}
```

- [ ] **Step 2: Verify build**

Run: `cd /Volumes/Slaven/sofia && go build -tags stdjson ./pkg/web/...`
Expected: Clean build.

- [ ] **Step 3: Commit**

```bash
git add pkg/web/handler_goals.go
git commit -m "feat(web): add GET /api/goals/completed endpoint"
```

---

### Task 8: Add GET /api/activity endpoint

**Files:**
- Create: `pkg/web/handler_activity.go`

- [ ] **Step 1: Create `handler_activity.go`**

Create `pkg/web/handler_activity.go`:

```go
package web

import (
	"encoding/json"
	"net/http"

	"github.com/grasberg/sofia/pkg/autonomy"
	"github.com/grasberg/sofia/pkg/tools"
)

// handleActivity returns a snapshot of all active goal work across agents.
func (s *Server) handleActivity(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.sendJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	pm := s.agentLoop.GetPlanManager()
	subagentTasks := s.agentLoop.GetActiveSubagentTasks()

	// Build a map of subagent label -> task info for quick lookup
	taskByLabel := make(map[string]map[string]any)
	for _, t := range subagentTasks {
		if label, ok := t["label"].(string); ok {
			taskByLabel[label] = t
		}
	}

	// Get all in-progress goals
	goals, err := s.agentLoop.ListGoals("")
	if err != nil {
		s.sendJSONError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var agents []map[string]any
	for _, g := range goals {
		if g.Status != autonomy.GoalStatusInProgress {
			continue
		}

		entry := map[string]any{
			"agent_id":  g.AgentID,
			"goal_id":   g.ID,
			"goal_name": g.Name,
		}

		if pm != nil {
			if plan := pm.GetPlanByGoalID(g.ID); plan != nil {
				entry["plan_id"] = plan.ID

				var activeTasks []map[string]any
				pending, completed, total := 0, 0, len(plan.Steps)

				for _, step := range plan.Steps {
					switch step.Status {
					case tools.PlanStatusPending:
						pending++
					case tools.PlanStatusCompleted:
						completed++
					case tools.PlanStatusInProgress:
						taskInfo := map[string]any{
							"step_index":  step.Index,
							"description": step.Description,
							"status":      "running",
							"assigned_to": step.AssignedTo,
						}
						// Enrich with subagent data if available
						label := step.AssignedTo
						if t, ok := taskByLabel[label]; ok {
							taskInfo["subagent_id"] = t["subagent_id"]
							taskInfo["created"] = t["created"]
						}
						activeTasks = append(activeTasks, taskInfo)
					case tools.PlanStatusFailed:
						// Count failed as not-pending for progress
					}
				}

				entry["active_tasks"] = activeTasks
				entry["pending_tasks"] = pending
				entry["completed_tasks"] = completed
				entry["total_tasks"] = total
			}
		}

		agents = append(agents, entry)
	}

	if agents == nil {
		agents = []map[string]any{}
	}

	result := map[string]any{"agents": agents}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}
```

- [ ] **Step 2: Verify build**

Run: `cd /Volumes/Slaven/sofia && go build -tags stdjson ./pkg/web/...`
Expected: Clean build.

- [ ] **Step 3: Commit**

```bash
git add pkg/web/handler_activity.go
git commit -m "feat(web): add GET /api/activity endpoint for live agent task monitoring"
```

---

### Task 9: Register new routes and embed templates in server.go

**Files:**
- Modify: `pkg/web/server.go`

- [ ] **Step 1: Add embed directives for new templates**

In `pkg/web/server.go`, after the `goalsHTML` embed (around line 102), add:

```go
//go:embed templates/activity.html
var activityHTML []byte

//go:embed templates/completed.html
var completedHTML []byte
```

- [ ] **Step 2: Add HTMX partial routes**

After the line `mux.HandleFunc("/ui/goals", servePartial(goalsHTML))` (around line 207), add:

```go
mux.HandleFunc("/ui/activity", servePartial(activityHTML))
mux.HandleFunc("/ui/completed", servePartial(completedHTML))
```

- [ ] **Step 3: Add API routes**

After the line `mux.HandleFunc("/api/goals/", api(s.handleGoalLog))` (around line 237), add:

```go
mux.HandleFunc("GET /api/goals/completed", api(s.handleGoalsCompleted))
mux.HandleFunc("GET /api/activity", api(s.handleActivity))
```

Note: The `GET /api/goals/completed` route must be registered BEFORE the catch-all `/api/goals/` to ensure proper matching. Move the new route before the existing `/api/goals/` line.

- [ ] **Step 4: Verify build**

Run: `cd /Volumes/Slaven/sofia && go build -tags stdjson ./pkg/web/...`
Expected: Will fail because the template files don't exist yet. That's fine — we'll create them in the next tasks.

- [ ] **Step 5: Commit**

```bash
git add pkg/web/server.go
git commit -m "feat(web): register activity and completed routes and embeds"
```

---

### Task 10: Create Activity page template

**Files:**
- Create: `pkg/web/templates/activity.html`

- [ ] **Step 1: Create `activity.html`**

Create `pkg/web/templates/activity.html`. Follow the same structure as `goals.html` — HTMX partial with Tailwind CSS, Material Symbols icons, websocket updates:

```html
<!-- ACTIVITY TAB (HTMX Partial) -->
<div id="tab-activity" class="flex flex-col flex-grow min-h-0 animate-fade-in">
    <!-- Header -->
    <div class="px-6 py-5 border-b border-[var(--border-color)] bg-[var(--bg-main)]/50 backdrop-blur shrink-0 flex items-center justify-between">
        <div>
            <h2 class="text-xl font-bold tracking-tight text-[var(--text-main)] flex items-center gap-2">
                <i data-lucide="activity" class="w-5 h-5 text-sofia"></i>
                Agent Activity
            </h2>
            <p class="text-xs text-zinc-500 mt-1">Live view of all running agents and subagents across goals.</p>
        </div>
        <button onclick="activityRefresh()" class="px-3 py-1.5 rounded-lg text-xs font-semibold hover:bg-zinc-800 text-zinc-400 hover:text-[var(--text-main)] transition-colors border border-transparent hover:border-zinc-700 flex items-center gap-1">
            <i data-lucide="refresh-cw" class="w-3 h-3"></i> Refresh
        </button>
    </div>

    <div class="flex-grow overflow-y-auto px-6 py-4">
        <div id="activity-empty" class="hidden text-center py-16 text-zinc-600">
            <i data-lucide="coffee" class="w-12 h-12 mx-auto mb-3 text-zinc-700"></i>
            <div class="text-sm font-medium">No active work</div>
            <div class="text-xs mt-1">When goals are being worked on, you'll see agent activity here.</div>
        </div>

        <div id="activity-list" class="space-y-4"></div>
    </div>
</div>

<script>
function activityRefresh() {
    fetch('/api/activity')
        .then(r => r.json())
        .then(data => renderActivity(data))
        .catch(() => {});
}

function renderActivity(data) {
    const list = document.getElementById('activity-list');
    const empty = document.getElementById('activity-empty');
    if (!data.agents || data.agents.length === 0) {
        list.innerHTML = '';
        empty.classList.remove('hidden');
        return;
    }
    empty.classList.add('hidden');

    list.innerHTML = data.agents.map(agent => {
        const total = agent.total_tasks || 0;
        const completed = agent.completed_tasks || 0;
        const pct = total > 0 ? Math.round((completed / total) * 100) : 0;
        const activeTasks = agent.active_tasks || [];
        const pending = agent.pending_tasks || 0;

        return `
        <div class="bg-surface-container-high/50 border border-outline-variant/20 rounded-2xl p-5">
            <div class="flex items-center justify-between mb-3">
                <div class="flex items-center gap-3">
                    <span class="w-2.5 h-2.5 rounded-full bg-green-500 animate-pulse"></span>
                    <span class="text-sm font-bold text-[var(--text-main)]">${agent.goal_name}</span>
                    <span class="text-[10px] font-mono text-zinc-500">goal:${agent.goal_id}</span>
                </div>
                <span class="text-xs font-mono text-zinc-500">${completed}/${total} steps</span>
            </div>
            <!-- Progress bar -->
            <div class="w-full bg-zinc-800 rounded-full h-1.5 mb-4">
                <div class="bg-sofia rounded-full h-1.5 transition-all" style="width:${pct}%"></div>
            </div>
            <!-- Active tasks -->
            ${activeTasks.length > 0 ? `
            <div class="space-y-2">
                <div class="text-[10px] font-bold uppercase tracking-widest text-zinc-500 mb-1">Running</div>
                ${activeTasks.map(task => `
                <div class="flex items-center gap-3 bg-surface-container/50 rounded-xl px-4 py-3">
                    <span class="w-2 h-2 rounded-full bg-blue-500 animate-pulse shrink-0"></span>
                    <div class="flex-grow min-w-0">
                        <div class="text-xs font-medium text-[var(--text-main)] truncate">${task.description}</div>
                        <div class="text-[10px] text-zinc-500 font-mono mt-0.5">${task.assigned_to || 'subagent'} · step ${task.step_index + 1}</div>
                    </div>
                    <span class="text-[10px] font-mono text-zinc-600 shrink-0">running</span>
                </div>
                `).join('')}
            </div>
            ` : ''}
            ${pending > 0 ? `<div class="text-[10px] text-zinc-600 mt-2">${pending} step(s) waiting</div>` : ''}
        </div>`;
    }).join('');
}

// Auto-refresh on load
activityRefresh();

// WebSocket live updates
if (window.dashboardWS) {
    const origHandler = window.dashboardWS.onmessage;
    window.dashboardWS.onmessage = function(event) {
        if (origHandler) origHandler(event);
        try {
            const msg = JSON.parse(event.data);
            if (['goal_step_start', 'goal_step_end', 'goal_plan_created', 'goal_completed'].includes(msg.type)) {
                activityRefresh();
            }
        } catch(e) {}
    };
}
</script>
```

- [ ] **Step 2: Verify build**

Run: `cd /Volumes/Slaven/sofia && go build -tags stdjson ./pkg/web/...`
Expected: Clean build (the embed directive now finds the file).

- [ ] **Step 3: Commit**

```bash
git add pkg/web/templates/activity.html
git commit -m "feat(web): add Activity page template with live agent task monitoring"
```

---

### Task 11: Create Completed page template

**Files:**
- Create: `pkg/web/templates/completed.html`

- [ ] **Step 1: Create `completed.html`**

Create `pkg/web/templates/completed.html`:

```html
<!-- COMPLETED GOALS TAB (HTMX Partial) -->
<div id="tab-completed" class="flex flex-col flex-grow min-h-0 animate-fade-in">
    <!-- Header -->
    <div class="px-6 py-5 border-b border-[var(--border-color)] bg-[var(--bg-main)]/50 backdrop-blur shrink-0 flex items-center justify-between">
        <div>
            <h2 class="text-xl font-bold tracking-tight text-[var(--text-main)] flex items-center gap-2">
                <i data-lucide="check-circle-2" class="w-5 h-5 text-sofia"></i>
                Completed Goals
            </h2>
            <p class="text-xs text-zinc-500 mt-1">Results, artifacts, and deployment instructions from finished goals.</p>
        </div>
        <div class="flex items-center gap-2">
            <input type="text" id="completed-search" placeholder="Search..."
                class="bg-surface-container border border-outline-variant/20 rounded-lg px-3 py-1.5 text-xs text-[var(--text-main)] w-48 focus:outline-none focus:border-sofia/50"
                oninput="completedFilter()">
            <button onclick="completedRefresh()" class="px-3 py-1.5 rounded-lg text-xs font-semibold hover:bg-zinc-800 text-zinc-400 hover:text-[var(--text-main)] transition-colors border border-transparent hover:border-zinc-700 flex items-center gap-1">
                <i data-lucide="refresh-cw" class="w-3 h-3"></i> Refresh
            </button>
        </div>
    </div>

    <div class="flex-grow overflow-y-auto px-6 py-4">
        <div id="completed-empty" class="hidden text-center py-16 text-zinc-600">
            <i data-lucide="inbox" class="w-12 h-12 mx-auto mb-3 text-zinc-700"></i>
            <div class="text-sm font-medium">No completed goals yet</div>
            <div class="text-xs mt-1">When goals finish, their results will appear here.</div>
        </div>

        <div id="completed-list" class="space-y-4"></div>
    </div>
</div>

<script>
let completedGoals = [];

function completedRefresh() {
    fetch('/api/goals/completed')
        .then(r => r.json())
        .then(data => { completedGoals = data; renderCompleted(data); })
        .catch(() => {});
}

function completedFilter() {
    const q = (document.getElementById('completed-search').value || '').toLowerCase();
    if (!q) { renderCompleted(completedGoals); return; }
    renderCompleted(completedGoals.filter(g => g.name.toLowerCase().includes(q) || (g.description || '').toLowerCase().includes(q)));
}

function renderCompleted(goals) {
    const list = document.getElementById('completed-list');
    const empty = document.getElementById('completed-empty');
    if (!goals || goals.length === 0) {
        list.innerHTML = '';
        empty.classList.remove('hidden');
        return;
    }
    empty.classList.add('hidden');

    list.innerHTML = goals.map((g, idx) => {
        const gr = g.goal_result || {};
        const plan = g.plan || {};
        const steps = plan.steps || [];
        const log = g.log || [];
        const priorityColor = g.priority === 'high' ? 'text-red-400' : g.priority === 'medium' ? 'text-amber-400' : 'text-zinc-400';
        const completedAt = gr.completed_at ? new Date(gr.completed_at).toLocaleDateString() : (g.updated_at ? new Date(g.updated_at).toLocaleDateString() : '');

        return `
        <div class="bg-surface-container-high/50 border border-outline-variant/20 rounded-2xl overflow-hidden">
            <!-- Goal header (click to expand) -->
            <div class="px-5 py-4 cursor-pointer hover:bg-surface-container-highest/30 transition-colors" onclick="toggleCompletedDetail(${idx})">
                <div class="flex items-center justify-between">
                    <div class="flex items-center gap-3">
                        <span class="w-2.5 h-2.5 rounded-full bg-sofia"></span>
                        <span class="text-sm font-bold text-[var(--text-main)]">${g.name}</span>
                        <span class="text-[10px] font-semibold uppercase ${priorityColor}">${g.priority || 'medium'}</span>
                    </div>
                    <div class="flex items-center gap-3">
                        <span class="text-[10px] font-mono text-zinc-500">${completedAt}</span>
                        <i data-lucide="chevron-down" class="w-4 h-4 text-zinc-600 transition-transform completed-chevron-${idx}"></i>
                    </div>
                </div>
                ${gr.summary ? `<p class="text-xs text-zinc-400 mt-2 line-clamp-2">${gr.summary}</p>` : ''}
            </div>

            <!-- Expandable detail -->
            <div id="completed-detail-${idx}" class="hidden border-t border-outline-variant/10">
                ${gr.summary ? `
                <div class="px-5 py-3 border-b border-outline-variant/10">
                    <div class="text-[10px] font-bold uppercase tracking-widest text-zinc-500 mb-1">Summary</div>
                    <p class="text-xs text-zinc-300">${gr.summary}</p>
                </div>` : ''}

                ${gr.artifacts && gr.artifacts.length > 0 ? `
                <div class="px-5 py-3 border-b border-outline-variant/10">
                    <div class="text-[10px] font-bold uppercase tracking-widest text-zinc-500 mb-1">Artifacts</div>
                    <div class="space-y-1">
                        ${gr.artifacts.map(a => `<div class="text-xs font-mono text-sofia/80">${a}</div>`).join('')}
                    </div>
                </div>` : ''}

                ${gr.next_steps && gr.next_steps.length > 0 ? `
                <div class="px-5 py-3 border-b border-outline-variant/10 bg-sofia/[0.03]">
                    <div class="text-[10px] font-bold uppercase tracking-widest text-sofia mb-2">Next Steps</div>
                    <div class="space-y-2">
                        ${gr.next_steps.map(s => `
                        <div class="flex items-start gap-2">
                            <span class="text-sofia mt-0.5">&#8250;</span>
                            <span class="text-xs text-zinc-300">${s}</span>
                        </div>`).join('')}
                    </div>
                </div>` : ''}

                ${steps.length > 0 ? `
                <div class="px-5 py-3">
                    <div class="flex items-center justify-between mb-2 cursor-pointer" onclick="toggleCompletedTimeline(${idx})">
                        <div class="text-[10px] font-bold uppercase tracking-widest text-zinc-500">Execution Timeline (${steps.length} steps)</div>
                        <i data-lucide="chevron-down" class="w-3 h-3 text-zinc-600"></i>
                    </div>
                    <div id="completed-timeline-${idx}" class="hidden space-y-2">
                        ${steps.map(s => {
                            const icon = s.status === 'completed' ? '<span class="text-green-400 text-xs">&#10003;</span>' : '<span class="text-red-400 text-xs">&#10007;</span>';
                            const logEntry = log.find(l => l.step === s.description);
                            const duration = logEntry ? `${(logEntry.duration_ms / 1000).toFixed(1)}s` : '';
                            return `
                            <div class="bg-surface-container/50 rounded-lg px-3 py-2">
                                <div class="flex items-center gap-2">
                                    ${icon}
                                    <span class="text-xs text-[var(--text-main)] flex-grow">${s.index + 1}. ${s.description}</span>
                                    <span class="text-[10px] font-mono text-zinc-600">${s.assigned_to || ''}</span>
                                    <span class="text-[10px] font-mono text-zinc-600">${duration}</span>
                                </div>
                                ${s.result ? `<div class="text-[10px] text-zinc-500 mt-1 pl-5 line-clamp-3 font-mono">${s.result}</div>` : ''}
                            </div>`;
                        }).join('')}
                    </div>
                </div>` : ''}
            </div>
        </div>`;
    }).join('');

    if (typeof lucide !== 'undefined') lucide.createIcons();
}

function toggleCompletedDetail(idx) {
    const el = document.getElementById('completed-detail-' + idx);
    el.classList.toggle('hidden');
}

function toggleCompletedTimeline(idx) {
    const el = document.getElementById('completed-timeline-' + idx);
    el.classList.toggle('hidden');
}

completedRefresh();
</script>
```

- [ ] **Step 2: Verify build**

Run: `cd /Volumes/Slaven/sofia && go build -tags stdjson ./pkg/web/...`
Expected: Clean build.

- [ ] **Step 3: Commit**

```bash
git add pkg/web/templates/completed.html
git commit -m "feat(web): add Completed page template with goal results and execution timeline"
```

---

### Task 12: Add Activity and Completed nav links to layout.html

**Files:**
- Modify: `pkg/web/templates/layout.html`

- [ ] **Step 1: Add nav links**

In `pkg/web/templates/layout.html`, after the Goals nav item (line 135), add:

```html
            <a href="#" hx-get="/ui/activity" hx-target="#main-content" onclick="setActiveNav(this, event);" id="nav-activity"
                class="nav-item">
                <span class="material-symbols-outlined" style="font-size:20px">sprint</span>
                <span class="hidden lg:inline">Activity</span>
            </a>
            <a href="#" hx-get="/ui/completed" hx-target="#main-content" onclick="setActiveNav(this, event);" id="nav-completed"
                class="nav-item">
                <span class="material-symbols-outlined" style="font-size:20px">task_alt</span>
                <span class="hidden lg:inline">Completed</span>
            </a>
```

Insert these two links right after the Goals nav link and before the Agents nav link.

- [ ] **Step 2: Verify build**

Run: `cd /Volumes/Slaven/sofia && make build`
Expected: Clean build with embedded templates.

- [ ] **Step 3: Commit**

```bash
git add pkg/web/templates/layout.html
git commit -m "feat(web): add Activity and Completed nav links to sidebar"
```

---

### Task 13: Update handleGoalsPatch to accept in_progress status

**Files:**
- Modify: `pkg/web/handler_goals.go`

- [ ] **Step 1: Allow `in_progress` in PATCH validation**

In `pkg/web/handler_goals.go`, in `handleGoalsPatch`, update the status validation (line 53):

```go
if req.Status != "paused" && req.Status != "failed" && req.Status != "active" && req.Status != "in_progress" {
	s.sendJSONError(w, "status must be paused, failed, active, or in_progress", http.StatusBadRequest)
	return
}
```

- [ ] **Step 2: Verify build**

Run: `cd /Volumes/Slaven/sofia && go build -tags stdjson ./pkg/web/...`
Expected: Clean build.

- [ ] **Step 3: Commit**

```bash
git add pkg/web/handler_goals.go
git commit -m "feat(web): allow in_progress status in goal PATCH endpoint"
```

---

### Task 14: Full integration build and lint

**Files:** None new — validation only.

- [ ] **Step 1: Full build**

Run: `cd /Volumes/Slaven/sofia && make build`
Expected: Clean build with all embedded templates.

- [ ] **Step 2: Run all tests**

Run: `cd /Volumes/Slaven/sofia && make test`
Expected: All tests pass.

- [ ] **Step 3: Run linter**

Run: `cd /Volumes/Slaven/sofia && make lint`
Expected: No new lint errors from our changes.

- [ ] **Step 4: Fix any issues and commit**

If lint or tests fail, fix the issues and commit:

```bash
git add -A
git commit -m "fix: address lint and test issues from goals workflow implementation"
```
