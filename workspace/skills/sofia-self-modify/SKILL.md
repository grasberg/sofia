---
name: sofia-self-modify
description: Workflow for modifying Sofia's own source code. Always use OpenCode CLI if available, then run the automated test agent to verify correctness. Use when Sofia needs to change her own Go codebase.
---

# Sofia Self-Modify

Use this skill whenever a task requires modifying Sofia's own Go source code
(`github.com/grasberg/sofia`).

## Decision: Use OpenCode if available

Run this command — it works on macOS, Linux, and Windows:

```
opencode --version
```

- If it **prints a version string**: OpenCode is installed. **Delegate all code edits to OpenCode.** Do not write files manually.
- If it **fails** (exit error, "command not found", or similar): OpenCode is not installed. Fall back to direct file editing with your own tools.

A failed check is not an error — it is a normal signal to use the fallback path.
Do not proceed with OpenCode steps if the check fails.

## Workflow

### Step 1 — Edit via OpenCode CLI

Run OpenCode non-interactively with `--print` so it applies the change and exits:

```bash
opencode --print "PROMPT" --cwd /path/to/sofia
```

- Set `--cwd` to Sofia's source root (the directory containing `go.mod`).
- The prompt must be a clear, complete description of what to change and why.
- OpenCode handles file edits, imports, and project code style automatically.
- For several distinct changes, run OpenCode once per logical change.

### Step 2 — Build to verify compilation

```bash
cd <sofia-source-root> && make build
```

If compilation fails, run OpenCode again with the compiler error output in the prompt.

### Step 3 — Run the Automated Testing Agent

After a successful build, invoke the `test-engineer` sub-agent (or `qa-automation-engineer`
for web UI / integration tests) with this task:

> "Run `make test` in the Sofia source directory, analyse any failures, and report
> which tests passed and which failed. Include the exact error messages for failures."

The test agent will run `go test ./...`, parse failures, and return a pass/fail summary
with package, test name, file, and line number for each failure.

### Step 4 — Fix failures

If tests fail, feed the exact failure output back into OpenCode:

```bash
opencode --print "The following test failed after the change. Fix it:\n\n<paste test output>" --cwd <sofia-source-root>
```

Rebuild (`make build`) and re-run the test agent. Repeat until all tests are green.

### Step 5 — Commit (only if requested)

Do not commit automatically. Only commit when the user explicitly asks.

## Notes

- The OpenCode binary is typically at `~/.opencode/bin/opencode`.
- Sofia's source root is wherever `go.mod` declares `module github.com/grasberg/sofia`.
- Never bypass OpenCode just because direct file editing feels simpler — OpenCode
  understands the full codebase context and applies changes more safely.
- If OpenCode is unavailable and you must edit directly, the test agent workflow
  (Steps 3–4) still applies.
