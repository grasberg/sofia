---
name: code-review-checklist
description: "📋 Systematic code review checklists covering correctness, security, design, performance, and maintainability. Activate for any PR review, self-review, quality assessment, or when establishing review standards for a team."
---

# Code Review Checklist

Structured code review methodology that catches bugs, security issues, and design problems systematically rather than by intuition.

## Review Priority Order

Review in this order -- earlier items block later ones:

### 1. Correctness
- Does the code do what it claims to do?
- Are edge cases handled (empty input, null, overflow, concurrent access)?
- Do error paths return sensible results or propagate correctly?
- Are there off-by-one errors in loops or slices?

### 2. Security
- Is user input validated before use?
- Are SQL queries parameterized (no string concatenation)?
- Are secrets hardcoded anywhere?
- Are auth/authz checks in place for protected operations?
- Could this introduce XSS, CSRF, or injection vulnerabilities?

### 3. Design
- Does this follow existing patterns in the codebase?
- Is the abstraction level right (not too generic, not too specific)?
- Are responsibilities clearly separated?
- Would a new team member understand this code?

### 4. Performance
- Are there N+1 query patterns?
- Are expensive operations inside loops?
- Is there unnecessary memory allocation?
- Could this cause timeouts under load?

### 5. Maintainability
- Are names descriptive and consistent with the codebase?
- Is the code testable (dependencies injectable, side effects isolated)?
- Are there sufficient tests for the changed behavior?
- Is the commit message clear about what and why?

## Review Comments Guide

### Good Review Comments
- Explain **why** something is a problem, not just that it is
- Suggest a specific alternative when possible
- Distinguish between **must fix** (blocking) and **nit** (suggestion)
- Ask questions when you don't understand intent ("Is this intentional?")

### Comment Prefixes
| Prefix | Meaning |
|--------|---------|
| `blocker:` | Must fix before merge |
| `concern:` | Potential issue, needs discussion |
| `nit:` | Style/preference, non-blocking |
| `question:` | Need clarification to continue review |
| `praise:` | Something done well (important for morale) |

## Quick Checklist

```markdown
### Correctness
- [ ] Logic handles happy path correctly
- [ ] Edge cases covered (empty, null, boundary values)
- [ ] Error handling is appropriate (not swallowed, not over-broad)

### Security
- [ ] No hardcoded secrets or credentials
- [ ] User input validated and sanitized
- [ ] Auth checks present on protected paths
- [ ] No SQL injection vectors

### Design
- [ ] Follows existing codebase patterns
- [ ] Reasonable abstraction level
- [ ] No unnecessary complexity added

### Tests
- [ ] New behavior has test coverage
- [ ] Tests verify behavior, not implementation
- [ ] Edge cases tested

### Operations
- [ ] No breaking changes without migration path
- [ ] Logging sufficient for debugging
- [ ] Performance impact considered
```

## AI/LLM-Specific Review Patterns

When reviewing code that integrates AI/LLM services:
- [ ] User input is sanitized before being inserted into prompts (prompt injection prevention)
- [ ] LLM output is validated/sanitized before rendering or executing
- [ ] Structured prompts with clear role boundaries are used
- [ ] Token limits and cost controls are in place
- [ ] Fallback behavior defined for when the LLM is unavailable
- [ ] Sensitive data is not leaked into prompts or logs

## Anti-Patterns in Code Review

- Bikeshedding on style while missing logic bugs
- Reviewing only the diff without understanding the surrounding code
- Approving without actually reading the changes
- Blocking on personal preference when the code is correct
- Reviewing 1000+ line PRs in one sitting (request the author split it)

