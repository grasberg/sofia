# Todo: Agent-Skills Goal Workflow

## Phase 1: Foundation Types
- [ ] Task 1: Add AcceptanceCriteria, VerifyCommand, VerifyResult, RetryCount to PlanStep/PlanStepDef
- [ ] Task 2: Add RetryStep + CompleteStepWithVerify to PlanManager
- [ ] Task 3: Add GoalSpec, Goal.Phase, AutonomyConfig.MaxStepRetries

**Checkpoint:** build + tests pass, no behavior change

## Phase 2: Specification Pipeline
- [ ] Task 4: Build spec phase (buildSpecificationPrompt, parseSpecResponse, specifyGoal)
- [ ] Task 5: Wire spec phase into pursueGoals

**Checkpoint:** active goals get specs before planning

## Phase 3: Enhanced Planning
- [ ] Task 6: Replace plan generation prompt with spec-aware version

**Checkpoint:** plans contain acceptance criteria per step

## Phase 4: Verified Dispatch
- [ ] Task 7: Enhance dispatch with verification + retry logic

**Checkpoint:** steps include verification evidence, retries work

## Phase 5: Finalization and Polish
- [ ] Task 8: Enhanced finalization with spec success criteria evaluation
- [ ] Task 9: Update SOUL.md goal completion instructions
- [ ] Task 10: Backward compatibility guard + integration test

**Final checkpoint:** all green, all 8 success criteria met
