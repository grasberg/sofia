---
name: autonomous-executor
description: Closed-loop autonomous task execution with self-verification and retry logic. Use when the user gives a complex multi-step goal and expects you to execute it end-to-end without asking clarifying questions. Triggers on phrases like "just do it", "handle this", "execute this plan", "make it happen", or when given a goal with clear success criteria.
---

# Autonomous Executor

A closed-loop execution pattern for complex goals. Freeze the intent, plan with
pass/fail criteria, execute step by step, self-verify, and retry on failure.

## When to Use

Activate this skill when:
- The user gives a complex goal with multiple steps
- The user expects autonomous execution without back-and-forth
- A task requires sequential operations where each step depends on the previous one
- The user explicitly says to "just do it" or "handle everything"

## Phase 1: Intent Freeze

Before doing anything, capture the user's intent exactly.

### Steps

1. Record the user's original request verbatim in `workspace/executor/[task-slug]/intent.md`.
2. Do NOT paraphrase, reinterpret, or "improve" the intent.
3. Extract from the intent:
   - **Goal:** What is the desired end state?
   - **Constraints:** Any explicit limitations or requirements mentioned.
   - **Implicit expectations:** What would a reasonable person expect beyond the literal words?
4. If the goal is ambiguous, make a reasonable assumption and document it. Do not ask for clarification.

### Intent Document Format

```markdown
# Intent Freeze

**Original request:** [Exact user words]
**Goal:** [Desired end state]
**Constraints:** [Any limitations]
**Assumptions:** [Decisions made where intent was ambiguous]
**Started:** [YYYY-MM-DD HH:MM]
```

## Phase 2: Planning

Break the goal into ordered steps with dependencies and verification criteria.

### Per-Step Definition

Each step must have:
- **ID:** Sequential identifier (S1, S2, S3, ...)
- **Description:** What this step does
- **Dependencies:** Which steps must complete first (e.g., "requires S1, S2")
- **Done criteria:** How to objectively verify success
- **Estimated duration:** How long this should take
- **Rollback plan:** How to undo this step if it causes problems

### Plan Document Format

Save to `workspace/executor/[task-slug]/plan.md`:

```markdown
# Execution Plan

## Steps

### S1: [Description]
- Dependencies: none
- Done criteria: [Specific, testable condition]
- Estimated duration: [time]
- Rollback: [How to undo]

### S2: [Description]
- Dependencies: S1
- Done criteria: [Specific, testable condition]
- Estimated duration: [time]
- Rollback: [How to undo]
```

### Planning Rules

- Order steps so that dependencies come first.
- Each step should be small enough to verify independently.
- If a step cannot be verified programmatically, define a manual check.
- Identify steps that can run in parallel (no mutual dependencies).

## Phase 3: Execution Loop

Execute each step in dependency order, verifying after each.

### Per-Step Execution Protocol

```
FOR each step in plan (dependency order):
  1. Log: "Starting [step ID]: [description]"
  2. Execute the step
  3. Run verification against done criteria
  4. IF verification passes:
       Log: "[step ID]: PASSED"
       Record result in execution log
       Proceed to next step
  5. IF verification fails:
       Log: "[step ID]: FAILED — [reason]"
       Analyze the failure
       Attempt alternative approach (max 3 retries per step)
       IF all retries exhausted:
         Mark step as BLOCKED
         Log what was tried
         Check if any subsequent steps can proceed without this one
         IF no path forward: halt and report
```

### Retry Strategy

On failure, try alternatives in this order:
1. **Retry with fix:** Identify what went wrong, adjust approach, retry.
2. **Alternative method:** Use a completely different approach to achieve the same outcome.
3. **Reduced scope:** Achieve a partial version of the step's goal.

After 3 failed attempts, the step is BLOCKED. Never retry more than 3 times.

### Execution Log Format

Append to `workspace/executor/[task-slug]/execution.log`:

```
[YYYY-MM-DD HH:MM:SS] S1: START — Creating project directory
[YYYY-MM-DD HH:MM:SS] S1: EXEC — mkdir -p /path/to/project
[YYYY-MM-DD HH:MM:SS] S1: VERIFY — Directory exists: YES
[YYYY-MM-DD HH:MM:SS] S1: PASSED

[YYYY-MM-DD HH:MM:SS] S2: START — Initializing git repository
[YYYY-MM-DD HH:MM:SS] S2: EXEC — git init
[YYYY-MM-DD HH:MM:SS] S2: VERIFY — .git directory exists: YES
[YYYY-MM-DD HH:MM:SS] S2: PASSED

[YYYY-MM-DD HH:MM:SS] S3: START — Installing dependencies
[YYYY-MM-DD HH:MM:SS] S3: EXEC — npm install (attempt 1)
[YYYY-MM-DD HH:MM:SS] S3: VERIFY — node_modules exists: NO
[YYYY-MM-DD HH:MM:SS] S3: FAILED — npm returned exit code 1, EACCES permission error
[YYYY-MM-DD HH:MM:SS] S3: RETRY 2 — Using --prefix flag for local install
[YYYY-MM-DD HH:MM:SS] S3: EXEC — npm install --prefix ./local
[YYYY-MM-DD HH:MM:SS] S3: VERIFY — local/node_modules exists: YES
[YYYY-MM-DD HH:MM:SS] S3: PASSED (via alternative)
```

## Phase 4: Completion Report

After all steps complete (or execution halts), produce a summary.

### Report Format

```markdown
# Execution Report

**Goal:** [From intent freeze]
**Status:** COMPLETED | PARTIAL | BLOCKED
**Duration:** [Total time]

## Results

| Step | Status | Notes |
|------|--------|-------|
| S1   | PASSED | Completed as planned |
| S2   | PASSED | Required retry — used alternative method |
| S3   | BLOCKED| Failed after 3 attempts: [reason] |

## What Was Accomplished

[Summary of what the user now has as a result of execution]

## Issues Encountered

[Any problems, with details on what was tried]

## Remaining Work

[If PARTIAL or BLOCKED: what still needs to be done and suggested approaches]
```

## Decision-Making Rules

- **Never ask for clarification mid-execution.** Use your best judgment and document assumptions.
- **Prefer reversible actions.** When two approaches are equally valid, choose the one that is easier to undo.
- **Log everything.** Every decision, every command, every result. The log is the audit trail.
- **Fail fast on hard blockers.** If a step is fundamentally impossible (not just hard), skip retries and report immediately.
- **Respect constraints.** If the user specified a constraint, never violate it even if it would make execution easier.

## Workspace File Structure

```
workspace/executor/[task-slug]/
  intent.md       # Frozen user intent
  plan.md         # Execution plan with steps
  execution.log   # Detailed execution log
  report.md       # Final completion report
```
