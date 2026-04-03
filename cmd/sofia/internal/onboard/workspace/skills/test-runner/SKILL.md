---
name: test-runner
description: Automated testing workflow for Sofia's Go codebase. Runs go test, parses failures with file and line references, and distinguishes regressions from pre-existing failures. Use after any code change to verify correctness.
---

# Automated Testing Agent

Use this skill to verify that Sofia's Go codebase compiles and all tests pass
after a code change.

## Quick Run

```bash
cd <sofia-source-root> && make test 2>&1
```

Or without Make:

```bash
go test ./... 2>&1
```

## Interpreting Output

Go test output format:

```
--- FAIL: TestFunctionName (0.00s)
    file_test.go:42: expected X, got Y
FAIL    github.com/grasberg/sofia/pkg/agent
```

For each failure, extract and report:
- Package path
- Test function name
- Source file + line number
- Exact assertion message

## Structured Report Format

Return results in this format:

```
BUILD: ok | FAILED
TESTS: N passed, M failed

FAILURES:
1. pkg/agent — TestFooBar (agent_test.go:42)
   expected "foo", got "bar"
```

## Regression Isolation

To distinguish new failures from pre-existing ones:

```bash
git stash && go test ./... 2>&1 > /tmp/before.txt
git stash pop && go test ./... 2>&1 > /tmp/after.txt
diff /tmp/before.txt /tmp/after.txt
```

Failures appearing only in `after.txt` are regressions introduced by the change.
Pre-existing failures must be noted but not treated as regressions.

## Known Pre-Existing Failures in This Repo

These tests fail on every run regardless of recent changes — do not flag as regressions:

- `cmd/sofia — TestNewSofiaCommand`: hardcoded subcommand count
- `cmd/sofia/internal — TestGetVersion`: expects `"dev"`, gets actual version tag
- `pkg/providers — TestCreateProvider_ClaudeCli` and related: missing protocol handlers
- `pkg/skills — TestListSkills*`: picks up real skills from `~/.sofia/` (environment bleed)
