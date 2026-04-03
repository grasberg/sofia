---
name: coding-agent
description: Autonomous coding workflow for generating, reviewing, debugging, refactoring, and testing code. Use when delegating programming tasks, performing code review, fixing bugs, writing tests, or refactoring existing code in any language.
---

# Coding Agent

Behavioral instructions for executing coding tasks autonomously with high quality and reliability.

## Before Writing Any Code

1. Read the existing codebase first. Never modify code without reading it.
2. Identify the language, framework, coding style, and conventions in use.
3. Check for linters, formatters, and CI configuration (`.eslintrc`, `golangci.yml`, `Makefile`, etc.).
4. Understand the module/package structure and dependency graph.
5. Look for existing patterns that solve similar problems — reuse, don't reinvent.

## Code Generation Workflow

### Test-Driven Development (TDD)

When adding new functionality, write tests first:

1. Write a failing test that defines the expected behavior.
2. Run the test to confirm it fails for the right reason.
3. Write the minimal code to make the test pass.
4. Run all tests to confirm nothing else broke.
5. Refactor if needed, re-run tests.

### Implementation Checklist

Before reporting a coding task as done:

```
- [ ] Code compiles / interprets without errors
- [ ] All new code has corresponding tests
- [ ] All tests pass (new and existing)
- [ ] Linter runs clean (no new warnings)
- [ ] Error cases are handled (no bare panics, no swallowed errors)
- [ ] Edge cases are considered (nil, empty, zero, overflow, concurrency)
```

## Debugging Workflow

Follow this sequence strictly — do not skip steps:

1. **Reproduce**: Create a minimal reproduction case. Confirm the bug exists.
2. **Isolate**: Narrow down the failing component. Use binary search on the code path.
3. **Hypothesize**: Form a specific theory about the root cause.
4. **Test the hypothesis**: Add logging, assertions, or a targeted test to confirm or reject.
5. **Fix**: Apply the minimal change that addresses the root cause.
6. **Verify**: Run the reproduction case again. Run the full test suite.
7. **Prevent**: Add a regression test if one does not already exist.

When stuck on a bug, try these in order:
- Read the error message carefully — the answer is often in the stack trace.
- Check recent changes with `git diff` or `git log`.
- Add targeted logging at the boundary where behavior diverges from expectation.

## Refactoring Workflow

1. **Ensure test coverage**: Before refactoring, verify tests exist for the code being changed. If not, write them first.
2. **Make one change at a time**: Each refactoring step should be independently verifiable.
3. **Run tests after every change**: If tests break, undo the last change and try a smaller step.
4. **Preserve external behavior**: Refactoring changes structure, not behavior. If behavior changes, that is a feature change, not a refactor.

## Code Review Checklist

When reviewing code (own or others), evaluate each dimension:

### Correctness
- Does the code do what it claims to do?
- Are all code paths reachable and tested?
- Are return values and errors checked?

### Edge Cases
- Nil/null inputs, empty collections, zero values
- Boundary conditions (off-by-one, max int, empty string)
- Concurrent access (race conditions, deadlocks)

### Error Handling
- Are errors propagated with context, not swallowed?
- Are retries idempotent?
- Are resources cleaned up on error (defer, finally, context cancellation)?

### Performance
- Are there unnecessary allocations in hot paths?
- Are O(n^2) algorithms used where O(n) or O(n log n) would work?
- Are database queries indexed and bounded?

### Readability
- Are names descriptive and consistent with the codebase?
- Is the code self-documenting, or does it need comments?
- Are functions short and single-purpose?

## Commit Practices

- Commit after each logical unit of work — not after every file save, not after the entire task.
- Write descriptive commit messages: what changed and why.
- Format: `<type>: <summary>` (e.g., `fix: handle nil pointer in session lookup`).
- Never commit commented-out code, debug prints, or temporary files.

## When Stuck

Try three genuinely different approaches before asking for help:

1. Re-read the relevant code and documentation.
2. Search the codebase for similar patterns that work.
3. Simplify the problem — remove variables until the core issue is clear.

If all three fail, report what was tried and what was observed at each step.
