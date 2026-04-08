# Implementation Plan: Agent-Skills Goal Workflow

## Overview

Enhance Sofia's autonomous goal-completion pipeline with a four-phase workflow (Specify → Plan → Tasks → Implement). The work is split into foundation types, then pipeline phases built in order, since each phase feeds the next.

## Architecture Decisions

- **Phase stored in properties JSON** — consistent with how `status` and `priority` are already stored in `parseGoalNode`
- **MaxStepRetries in AutonomyConfig** — follows existing pattern of config-driven behavior
- **Natural-language verify instructions** — subagents interpret verification using their available tools rather than fragile shell commands
- **Backward compatibility via nil checks** — goals without `Spec` or `Phase` fall through to existing behavior

---

## Task List

### Phase 1: Foundation Types

## Task 1: Add new fields to PlanStep and PlanStepDef

**Description:** Extend `PlanStep` with `AcceptanceCriteria`, `VerifyCommand`, `VerifyResult`, and `RetryCount` fields. Extend `PlanStepDef` with `AcceptanceCriteria` and `VerifyCommand`. These are pure data changes — no behavior change yet.

**Acceptance criteria:**
- [ ] `PlanStep` has all four new fields with correct JSON tags and `omitempty`
- [ ] `PlanStepDef` has `AcceptanceCriteria` and `VerifyCommand` fields
- [ ] `CreatePlanForGoal` copies the new fields from `PlanStepDef` into `PlanStep`
- [ ] Existing tests pass unchanged (new fields default to zero values)

**Verification:**
- [ ] `go build ./cmd/sofia/...`
- [ ] `go test ./pkg/tools/...`

**Dependencies:** None

**Files likely touched:**
- `pkg/tools/plan_types.go`
- `pkg/tools/plan_manager.go` (CreatePlanForGoal only)

**Estimated scope:** Small (2 files)

---

## Task 2: Add RetryStep to PlanManager and verify-aware CompleteStep

**Description:** Add `RetryStep(planID, stepIdx)` method that resets a step to `pending`, clears `AssignedTo`, and increments `RetryCount`. Modify `CompleteStep` to accept an optional `verifyResult` parameter.

**Acceptance criteria:**
- [ ] `RetryStep` resets step status to `pending`, clears `AssignedTo`, increments `RetryCount`
- [ ] `RetryStep` returns false if plan/step not found or step is not `failed`/`completed`
- [ ] `CompleteStepWithVerify(planID, stepIdx, success, result, verifyResult)` stores `VerifyResult`
- [ ] Existing `CompleteStep` callers continue to work (add new method, don't break old one)
- [ ] Auto-save triggers on both methods

**Verification:**
- [ ] Unit tests for `RetryStep` (happy path, not-found, wrong status)
- [ ] Unit test for `CompleteStepWithVerify` storing verify result
- [ ] `go test ./pkg/tools/...`

**Dependencies:** Task 1

**Files likely touched:**
- `pkg/tools/plan_manager.go`
- `pkg/tools/plan_test.go`

**Estimated scope:** Small (2 files)

---

## Task 3: Add GoalSpec, Goal.Phase, and MaxStepRetries config

**Description:** Add `GoalSpec` struct and `Phase`/`Spec` fields to `Goal`. Update `parseGoalNode` to extract the new fields from properties JSON. Add `MaxStepRetries` to `AutonomyConfig`. Add helper methods for phase transitions on goals.

**Acceptance criteria:**
- [ ] `GoalSpec` struct defined with `Requirements`, `SuccessCriteria`, `Constraints`, `Context`
- [ ] `Goal` has `Phase` (string) and `Spec` (*GoalSpec) fields
- [ ] `parseGoalNode` populates `Phase` and `Spec` from properties JSON
- [ ] `GoalManager` has `UpdateGoalPhase(goalID, phase)` and `SetGoalSpec(goalID, spec)` methods
- [ ] `AutonomyConfig` has `MaxStepRetries int` with JSON/env tags
- [ ] Goals without `phase`/`spec` in properties parse without error (backward compat)

**Verification:**
- [ ] Unit test: `parseGoalNode` with and without new fields
- [ ] Unit test: `UpdateGoalPhase` + `SetGoalSpec` round-trip
- [ ] `go build ./cmd/sofia/...`

**Dependencies:** None (parallel with Task 1)

**Files likely touched:**
- `pkg/autonomy/goals.go`
- `pkg/config/config_channels.go`
- `pkg/autonomy/goals_test.go` (new)

**Estimated scope:** Small (3 files)

---

### Checkpoint: Foundation
- [ ] `go build ./cmd/sofia/...` passes
- [ ] `go test ./pkg/tools/... ./pkg/autonomy/...` passes
- [ ] No behavior changes — existing goal pipeline works identically
- [ ] Review before proceeding to pipeline phases

---

### Phase 2: Specification Pipeline

## Task 4: Build specification phase (prompt, parser, orchestration)

**Description:** Create `buildSpecificationPrompt`, `parseSpecResponse`, and `specifyGoal` functions in `service_goals.go`. The spec phase analyzes a goal's description, produces requirements and success criteria, and stores them in the goal's properties.

**Acceptance criteria:**
- [ ] `buildSpecificationPrompt(goal)` returns a prompt that asks for `{requirements, success_criteria, constraints}`
- [ ] `parseSpecResponse(content)` parses JSON into `GoalSpec`, handles malformed input gracefully
- [ ] `specifyGoal(ctx, gm, goal)` calls LLM, parses response, calls `SetGoalSpec`, transitions phase to `"plan"`
- [ ] Budget is checked before LLM call; tokens tracked after
- [ ] Phase transition logged with `logger.InfoCF`
- [ ] Broadcast event `goal_spec_created` with goal_id

**Verification:**
- [ ] Unit test: `buildSpecificationPrompt` output contains goal name and description
- [ ] Unit test: `parseSpecResponse` with valid JSON
- [ ] Unit test: `parseSpecResponse` with malformed JSON returns error
- [ ] `go build ./cmd/sofia/...`

**Dependencies:** Task 3

**Files likely touched:**
- `pkg/autonomy/service_goals.go`
- `pkg/autonomy/service_goals_test.go` (new)

**Estimated scope:** Small (2 files)

---

## Task 5: Wire specification phase into pursueGoals

**Description:** Modify `pursueGoals` to add a new phase loop: active goals without a spec (phase empty or `"specify"`) go through `specifyGoal` before `generatePlanForGoal`. Goals with a spec skip directly to plan generation.

**Acceptance criteria:**
- [ ] `pursueGoals` processes active goals in order: specify → plan → dispatch
- [ ] Goals with `Phase == ""` or `Phase == "specify"` enter spec generation
- [ ] Goals with `Phase == "plan"` (spec already done) enter plan generation
- [ ] Goals with `Phase == "implement"` enter dispatch
- [ ] Backward compat: goals without phase field get a spec generated (no crash, no skip)

**Verification:**
- [ ] Manual: add a goal via `manage_goals`, observe spec created in logs
- [ ] `go build ./cmd/sofia/...`
- [ ] `go test ./pkg/autonomy/...`

**Dependencies:** Task 4

**Files likely touched:**
- `pkg/autonomy/service_goals.go` (pursueGoals only)

**Estimated scope:** XS (1 file, ~20 lines)

---

### Checkpoint: Specification Phase
- [ ] `go build ./cmd/sofia/...` passes
- [ ] Active goals get a GoalSpec before planning begins
- [ ] Existing in_progress goals continue dispatching (no regression)
- [ ] Review before proceeding

---

### Phase 3: Enhanced Planning

## Task 6: Replace plan generation prompt with spec-aware version

**Description:** Replace `buildPlanGenerationPrompt` with `buildEnhancedPlanPrompt` that includes the goal's spec (requirements, success criteria) and instructs the LLM to produce steps with `acceptance_criteria` and `verify_command` fields. Update `parseGoalPlanResponse` to validate the new fields are present.

**Acceptance criteria:**
- [ ] New prompt includes spec requirements and success criteria
- [ ] LLM is instructed to produce steps with `description`, `acceptance_criteria`, `verify_command`, `depends_on`
- [ ] Prompt instructs vertical slicing preference (complete feature paths)
- [ ] `parseGoalPlanResponse` validates `acceptance_criteria` is non-empty on each step (warn if missing, don't reject)
- [ ] `generatePlanForGoal` transitions goal phase to `"implement"` after plan creation
- [ ] Falls back to old prompt format if goal has no spec (backward compat)

**Verification:**
- [ ] Unit test: `buildEnhancedPlanPrompt` includes spec fields
- [ ] Unit test: `parseGoalPlanResponse` with new fields
- [ ] Unit test: `parseGoalPlanResponse` with old format (no acceptance_criteria) still works
- [ ] `go build ./cmd/sofia/...`

**Dependencies:** Task 5

**Files likely touched:**
- `pkg/autonomy/service_goals.go`
- `pkg/autonomy/service_goals_test.go`

**Estimated scope:** Small (2 files)

---

### Checkpoint: Enhanced Planning
- [ ] Plans generated from new goals contain acceptance criteria per step
- [ ] Plans generated from old goals (no spec) still work
- [ ] `go test ./pkg/autonomy/... ./pkg/tools/...` passes
- [ ] Review before proceeding

---

### Phase 4: Verified Dispatch

## Task 7: Enhance dispatch with verification and retry logic

**Description:** Modify `dispatchReadySteps` to use an enhanced task prompt that includes acceptance criteria and verification instructions. Add retry logic: when a step's subagent reports back, evaluate whether verification passed. If not, and `RetryCount < MaxStepRetries`, call `RetryStep` to reschedule. Use `CompleteStepWithVerify` to store verification output.

**Acceptance criteria:**
- [ ] Task prompt includes: goal name, step description, acceptance criteria, and verification instruction
- [ ] Subagent prompt instructs: "After completing the task, verify your work by: {verify_command}. Report what you verified and whether it passed."
- [ ] Callback parses result for verification evidence (looks for verification section in output)
- [ ] Failed verification → `RetryStep` if `RetryCount < MaxStepRetries`, else mark failed
- [ ] Successful verification → `CompleteStepWithVerify` with verify result stored
- [ ] Retry and failure events broadcast to dashboard
- [ ] `MaxStepRetries` read from config (default 2)

**Verification:**
- [ ] Unit test: `buildVerifyingTaskPrompt` includes acceptance criteria and verify instruction
- [ ] `go build ./cmd/sofia/...`
- [ ] Manual: trigger a goal, observe verification output in step results

**Dependencies:** Task 2, Task 6

**Files likely touched:**
- `pkg/autonomy/service_goals.go` (dispatchReadySteps, new prompt builder)
- `pkg/autonomy/service_goals_test.go`

**Estimated scope:** Medium (2 files, ~80 lines changed)

---

### Checkpoint: Verified Dispatch
- [ ] Steps include verification evidence in their results
- [ ] Failed verification triggers retry (visible in logs)
- [ ] Retries exhaust → step marked failed
- [ ] `go test ./pkg/autonomy/... ./pkg/tools/...` passes
- [ ] Review before proceeding

---

### Phase 5: Finalization and Polish

## Task 8: Enhanced finalization with spec evaluation

**Description:** Modify `finalizeGoal` to include the goal's spec success criteria in the finalization prompt. The LLM evaluates each success criterion as met/unmet. If any criteria are unmet, log a warning but still complete (don't block — replanning is a future enhancement). Include `unmet_criteria` in `GoalResult`.

**Acceptance criteria:**
- [ ] Finalization prompt includes spec success criteria and asks LLM to evaluate each
- [ ] LLM response parsed for `unmet_criteria` field (array of strings)
- [ ] `GoalResult` extended with `UnmetCriteria []string` and `Evidence []string`
- [ ] Verification evidence from step results included in finalization prompt
- [ ] Notification to user mentions unmet criteria if any
- [ ] Goals without specs use the existing finalization flow (backward compat)

**Verification:**
- [ ] Unit test: finalization prompt includes success criteria
- [ ] `go build ./cmd/sofia/...`
- [ ] `go test ./pkg/autonomy/...`

**Dependencies:** Task 7

**Files likely touched:**
- `pkg/autonomy/service_goals.go` (finalizeGoal)
- `pkg/autonomy/goals.go` (GoalResult)
- `pkg/autonomy/service_goals_test.go`

**Estimated scope:** Small (3 files)

---

## Task 9: Update SOUL.md goal completion instructions

**Description:** Update the "Autonomous Goal Completion" section in the onboard workspace SOUL.md to reflect the new phased workflow. Keep the same tone and brevity.

**Acceptance criteria:**
- [ ] "Autonomous Goal Completion" section describes: specify → plan → implement → verify → finalize
- [ ] Mentions acceptance criteria and verification
- [ ] Preserves existing personality/tone
- [ ] No changes to other sections

**Verification:**
- [ ] Read the file and confirm only the goal section changed
- [ ] `go build ./cmd/sofia/...` (ensures embed still works)

**Dependencies:** None (can run parallel to any task)

**Files likely touched:**
- `cmd/sofia/internal/onboard/workspace/SOUL.md`

**Estimated scope:** XS (1 file)

---

## Task 10: Backward compatibility guard + integration test

**Description:** Write an integration-style test that exercises the full pipeline with a mock LLM provider. Test both new-style goals (with spec) and old-style goals (no spec/phase). Verify old goals don't crash and still complete.

**Acceptance criteria:**
- [ ] Test creates a goal with no phase/spec → pipeline generates spec → generates plan → dispatches
- [ ] Test creates a goal with pre-existing spec → pipeline skips specify, goes to plan
- [ ] Test verifies retry on failed verification (mock subagent returns failure)
- [ ] Test verifies old-style `PlanStepDef` (no acceptance_criteria) still parses
- [ ] All assertions pass

**Verification:**
- [ ] `go test ./pkg/autonomy/... -run TestGoalPipeline -v`
- [ ] `go test ./pkg/tools/... -run TestPlanStepDef -v`

**Dependencies:** Task 8

**Files likely touched:**
- `pkg/autonomy/service_goals_test.go`
- `pkg/tools/plan_test.go`

**Estimated scope:** Medium (2 files)

---

### Checkpoint: Complete
- [ ] `go build ./cmd/sofia/...` passes
- [ ] `go test ./pkg/autonomy/... ./pkg/tools/...` passes — all green
- [ ] All 8 spec success criteria verified
- [ ] Ready for review and merge

---

## Risks and Mitigations

| Risk | Impact | Mitigation |
|------|--------|------------|
| LLM produces specs/plans that don't parse | Medium | `utils.CleanJSONFences()` + warn-and-skip (don't block pipeline) |
| Retry loop burns budget on bad steps | Medium | `MaxStepRetries` cap + budget check before each retry |
| Backward compat break for existing goals | High | Nil checks everywhere; test both old and new goal paths |
| `parseGoalNode` breaks on new properties | High | Test round-trip with and without new fields |
| `GoalManager` interface break (tools pkg) | High | Don't touch the interface; add methods to the concrete type only |

## Parallelization Opportunities

- **Task 1 and Task 3** are independent — can run in parallel
- **Task 9** (SOUL.md) is independent — can run any time
- Tasks 4-8 are sequential (each phase feeds the next)
