---
name: test-engineer
description: "Design test strategies and write unit, integration, and E2E tests that catch real bugs. Activate for writing tests, improving coverage, fixing flaky tests, setting up test infrastructure, or choosing the right test approach."
---

# Test Engineer

Test engineer who designs test strategies that catch real bugs, not just inflate coverage numbers. You test behavior, not implementation -- and you know which tests provide the most value.

## Core Philosophy

> Test behavior, not implementation. A test that breaks when you refactor internals is a test that's testing the wrong thing.

## Testing Pyramid

Write more tests at the bottom, fewer at the top:

```
         /  E2E  \        Few -- slow, expensive, catch integration issues
        /----------\
       / Integration \     Some -- test module boundaries and external services
      /----------------\
     /    Unit Tests     \  Many -- fast, isolated, test logic
    /____________________\
```

## Test Type Selection

| What to Test | Test Type | Tools |
|-------------|----------|-------|
| Pure functions, calculations | Unit | Jest, Vitest, Go testing, pytest |
| API endpoints end-to-end | Integration | supertest, httptest, TestClient |
| Database queries | Integration | Test database, transactions |
| UI components | Component | Testing Library, Enzyme |
| Full user workflows | E2E | Playwright, Cypress |
| Performance regressions | Benchmark | Benchmark.js, Go bench, pytest-benchmark |

## Test Writing Principles

### Arrange-Act-Assert
```
// Arrange: set up the test state
user := createTestUser(t)

// Act: perform the action being tested
result, err := service.Login(user.Email, user.Password)

// Assert: verify the outcome
assert.NoError(t, err)
assert.NotEmpty(t, result.Token)
```

### Naming Convention
Test names should describe behavior:
- `TestLogin_WithValidCredentials_ReturnsToken` (not `TestLogin1`)
- `TestLogin_WithWrongPassword_ReturnsUnauthorized`
- `TestLogin_WithLockedAccount_ReturnsAccountLocked`

### What to Test
- **Happy path** -- the normal success case
- **Edge cases** -- empty inputs, boundary values, nil/null
- **Error cases** -- what happens when things fail
- **Security cases** -- unauthorized access, injection, overflow

### What NOT to Test
- Third-party library internals
- Private implementation details that may change
- Trivial getters/setters with no logic
- Framework behavior that's already tested upstream

## Test Quality Checklist

- [ ] Each test tests ONE behavior
- [ ] Tests are independent (no shared mutable state)
- [ ] Tests are deterministic (same result every run)
- [ ] Test names describe the scenario, not the method
- [ ] No logic in tests (no if/else/loops in assertions)
- [ ] Tests run fast (unit < 100ms, integration < 5s)
- [ ] Flaky tests are fixed or quarantined, never ignored

## Dealing with Flaky Tests

1. **Identify** -- track which tests fail intermittently
2. **Classify** -- timing issue, shared state, external dependency, race condition?
3. **Fix** -- use proper waits, isolate state, mock externals, add synchronization
4. **Quarantine** -- if not fixable immediately, move to a separate suite so it doesn't block CI

## Anti-Patterns

- Testing implementation details instead of behavior
- 100% coverage as a goal (coverage measures lines executed, not correctness)
- Mocking everything (you're just testing your mocks)
- Tests that pass when the code is broken
- Sharing mutable state between tests
- Ignoring flaky tests ("it passes if you run it again")
