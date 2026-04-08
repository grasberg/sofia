# Spec: Agent-Skills Goal Workflow

## Objective

Replace Sofia's current goal-completion pipeline with a structured, phased workflow inspired by [addyosmani/agent-skills](https://github.com/addyosmani/agent-skills). The current pipeline is too simple: an LLM generates 3-10 vague steps, subagents execute them blind, and a summary is produced. There is no specification phase, no acceptance criteria, no verification per step, and no quality gates.

The new workflow introduces four phases — **Specify → Plan → Tasks → Implement** — each with verification before advancing. This ensures goals produce higher-quality, verified outcomes with incremental delivery.

### Who benefits
- Sofia's autonomy system produces more reliable goal outcomes
- Users get structured progress reporting with clear acceptance criteria
- Failed goals are easier to diagnose (each step has verification evidence)

### Success looks like
- Goals follow a phased pipeline: specify → plan → tasks → implement
- Each plan step has acceptance criteria and a verification command
- Steps are executed incrementally: implement → verify → report → next
- Failed verification triggers retry or replanning (not silent continuation)
- Goal completion includes verification evidence, not just LLM summaries

## Tech Stack

- **Language:** Go 1.25
- **Core packages:** `pkg/autonomy/`, `pkg/tools/`
- **LLM integration:** `pkg/providers/` (multi-provider, model-agnostic)
- **Storage:** SQLite via `pkg/memory/` (semantic nodes for goals)
- **Concurrency:** `sync.Mutex/RWMutex`, goroutines, async callbacks

## Commands

```
Build:  go build ./cmd/sofia/...
Test:   go test ./pkg/autonomy/... ./pkg/tools/...
Lint:   golangci-lint run ./pkg/autonomy/... ./pkg/tools/...
Run:    go run ./cmd/sofia gateway
```

## Project Structure

Files that will be modified or created:

```
pkg/autonomy/
├── service_goals.go        ← PRIMARY: rewrite plan generation + dispatch + verification
├── goals.go                ← MODIFY: add GoalPhase field, spec storage
├── service.go              ← MINOR: no structural changes expected

pkg/tools/
├── plan_types.go           ← MODIFY: add AcceptanceCriteria, VerifyCommand to PlanStep
├── plan_manager.go         ← MODIFY: add RetryStep, phase tracking helpers

cmd/sofia/internal/onboard/workspace/
├── SOUL.md                 ← MODIFY: update "Autonomous Goal Completion" section
```

## Current Flow (What Exists)

```
1. Goal created (status: active)
2. pursueGoals() picks up active goals
3. generatePlanForGoal():
   - Sends simple LLM prompt: "create 3-10 steps"
   - Parses JSON response into PlanStepDef{Description, DependsOn}
   - Creates plan via PlanManager, transitions goal to in_progress
4. dispatchReadySteps():
   - Finds steps with all dependencies met
   - Spawns subagent with prompt: "Your task: {description}. Use tools. Do it."
   - On callback: marks step completed/failed, cascades to next ready steps
5. finalizeGoal():
   - All steps done → LLM summarizes → goal marked completed
```

### Problems with current flow
- **No specification:** Steps are generated from a vague goal description with no requirements analysis
- **No acceptance criteria:** Steps have only a `Description` — no way to know when a step is "done correctly"
- **No verification:** Steps succeed if the subagent returns without error, not if the output is actually correct
- **No incremental quality:** All steps are fire-and-forget; failures don't trigger intelligent retries
- **Shallow prompts:** Both the plan-generation and task-execution prompts are minimal

## New Flow (What We're Building)

```
Phase 1: SPECIFY (new)
├── Analyze goal description
├── Identify implicit requirements
├── Define success criteria
├── Store spec in goal properties
└── Gate: spec must have ≥1 success criterion

Phase 2: PLAN (enhanced)
├── Generate plan WITH spec context
├── Each step gets: description, acceptance_criteria, verify_command
├── Dependency graph with vertical slicing preference
└── Gate: plan must parse with all required fields

Phase 3: TASKS → dispatch (enhanced)
├── Each subagent receives: goal context + step spec + acceptance criteria
├── Subagent must execute AND verify (run verify_command)
├── Verification result included in step completion
└── Gate: step only marked "completed" if verification passes

Phase 4: FINALIZE (enhanced)
├── Gather step results WITH verification evidence
├── LLM evaluates whether success criteria from spec are met
├── If not met: replan remaining work (not just fail)
└── Goal result includes: summary, artifacts, evidence, unmet_criteria
```

## Data Structure Changes

### PlanStep (plan_types.go)

Add fields to `PlanStep`:

```go
type PlanStep struct {
    Index              int        `json:"index"`
    Description        string     `json:"description"`
    AcceptanceCriteria string     `json:"acceptance_criteria,omitempty"` // NEW
    VerifyCommand      string     `json:"verify_command,omitempty"`     // NEW
    Status             PlanStatus `json:"status"`
    Result             string     `json:"result,omitempty"`
    VerifyResult       string     `json:"verify_result,omitempty"`     // NEW
    SubPlanID          string     `json:"sub_plan_id,omitempty"`
    AssignedTo         string     `json:"assigned_to,omitempty"`
    DependsOn          []int      `json:"depends_on,omitempty"`
    RetryCount         int        `json:"retry_count,omitempty"`       // NEW
}
```

### PlanStepDef (plan_types.go)

Add fields to the LLM-generated step definition:

```go
type PlanStepDef struct {
    Description        string `json:"description"`
    AcceptanceCriteria string `json:"acceptance_criteria"`
    VerifyCommand      string `json:"verify_command"`
    DependsOn          []int  `json:"depends_on"`
}
```

### Goal (goals.go)

Add spec storage:

```go
type Goal struct {
    // ... existing fields ...
    Phase    string    `json:"phase,omitempty"`    // NEW: specify, plan, tasks, implement, completed
    Spec     *GoalSpec `json:"spec,omitempty"`     // NEW
}

type GoalSpec struct {
    Requirements    []string `json:"requirements"`
    SuccessCriteria []string `json:"success_criteria"`
    Constraints     []string `json:"constraints,omitempty"`
    Context         string   `json:"context,omitempty"`
}
```

## Code Style

Follow existing patterns in the codebase:

```go
// Function comment follows godoc style — one line starting with the function name.
// buildSpecificationPrompt creates the LLM prompt for the specification phase.
func buildSpecificationPrompt(goal *Goal) string {
    return fmt.Sprintf(`You are an autonomous AI agent. Analyze this goal and produce a specification.

Goal: %s
Description: %s

Respond in this exact JSON format (no markdown, no code fences):
{"requirements": [...], "success_criteria": [...], "constraints": [...]}`, goal.Name, goal.Description)
}
```

- Functions prefixed with `build*Prompt` for LLM prompts
- Functions prefixed with `parse*Response` for LLM response parsing
- Error handling: `logger.WarnCF` + early return (no panics)
- JSON responses: always instruct "no markdown, no code fences"
- Use `utils.CleanJSONFences()` defensively when parsing LLM JSON

## Testing Strategy

- **Framework:** `go test` with standard `testing` package
- **Location:** Same package (`pkg/autonomy/`, `pkg/tools/`)
- **Existing tests:** `pkg/tools/plan_test.go` — extend with new fields

### Test coverage needed

| Area | Test | Type |
|------|------|------|
| Prompt generation | `buildSpecificationPrompt` produces valid prompt | Unit |
| Prompt generation | `buildEnhancedPlanPrompt` includes spec context | Unit |
| Prompt generation | `buildVerifyingTaskPrompt` includes acceptance criteria | Unit |
| Response parsing | `parseSpecResponse` handles valid JSON | Unit |
| Response parsing | `parseSpecResponse` handles malformed JSON | Unit |
| Response parsing | `parseGoalPlanResponse` parses new fields (acceptance_criteria, verify_command) | Unit |
| Plan types | `PlanStepDef` marshals/unmarshals with new fields | Unit |
| Plan manager | `RetryStep` resets status and increments retry count | Unit |
| Plan manager | `CompleteStep` stores verify_result | Unit |
| Goal phases | Phase transitions: specify → plan → tasks → implement → completed | Unit |
| Integration | Full pipeline with mock LLM provider | Integration |

## Boundaries

### Always do
- Run `go build ./cmd/sofia/...` after every change to verify compilation
- Run `go test ./pkg/autonomy/... ./pkg/tools/...` before committing
- Preserve backward compatibility: existing goals without specs should still work
- Use `utils.CleanJSONFences()` when parsing any LLM JSON response
- Keep LLM prompts instructing "no markdown, no code fences" for JSON responses
- Log phase transitions with `logger.InfoCF`

### Ask first
- Changing the `GoalManager` interface in `pkg/tools/manage_goals.go` (breaks the interface contract)
- Adding new tool parameters to `manage_goals` (affects user-facing API)
- Modifying `PlanManager` methods that other packages depend on
- Changing the `Plan.FormatStatus()` output format (affects dashboard)

### Never do
- Remove existing goal statuses or plan statuses
- Break the `GoalManager` interface that tools package depends on
- Add external dependencies
- Change the autonomy budget tracking logic
- Modify the subagent spawn mechanism (`SubagentManager.Spawn`)

## Success Criteria

1. **Spec phase works:** Active goals get a `GoalSpec` generated before planning begins
2. **Enhanced plans:** Plan steps include `acceptance_criteria` and `verify_command` fields
3. **Verification in dispatch:** Subagent task prompts include verification instructions; step results include verification output
4. **Retry on failure:** Failed verification triggers retries up to `MaxStepRetries` (configurable, default 2) before marking step as failed
5. **Phase tracking:** Goals track their current phase (specify → plan → tasks → implement → completed)
6. **Backward compatible:** Goals created before this change (no spec) still execute via the existing flow
7. **Tests pass:** All new code has unit tests; existing tests still pass
8. **Builds clean:** `go build ./cmd/sofia/...` succeeds with no errors

## Resolved Questions

1. **Max retries per step:** Configurable via `AutonomyConfig.MaxStepRetries` (default: 2).
2. **Spec approval gate:** No approval gate — fully autonomous. No behavior change.
3. **Verify command scope:** Natural-language verification instructions. The subagent interprets and executes verification using its available tools (exec, read_file, etc.). This is more flexible than shell commands and avoids fragile LLM-generated command strings. The verify instruction tells the subagent *what to check*, not *how to check it*.
4. **Phase persistence:** Stored in semantic node properties JSON (alongside status, priority, etc.). Consistent with existing patterns.
