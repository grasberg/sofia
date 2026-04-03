---
name: systematic-debugging
description: "Debugging methodology and investigation techniques. Use this skill as supplementary knowledge when investigating bugs, understanding error patterns, or applying structured troubleshooting approaches."
---

# Systematic Debugging

> **Category:** methodology | **Tags:** debugging, investigation, troubleshooting, methodology, root-cause

A knowledge module for systematic debugging approaches. This supplements the debugger skill with deeper methodology and investigation frameworks.

## When to Use

- As **supplementary knowledge** during bug investigation
- When standard debugging approaches aren't working
- For **complex, multi-system bugs** that cross boundaries
- When training others on **debugging methodology**

## Investigation Frameworks

### The Scientific Method for Debugging
1. **Observe** -- gather all symptoms, logs, and error messages
2. **Hypothesize** -- form a testable theory about the cause
3. **Predict** -- if your theory is correct, what else should be true?
4. **Test** -- verify your prediction with a targeted experiment
5. **Conclude** -- if confirmed, fix it; if not, form a new hypothesis

### Fault Tree Analysis
Work backwards from the failure:
```
[Symptom: API returns 500]
  |-- [Database connection failed]
  |     |-- [Connection pool exhausted]
  |     |     |-- [Connections not being released] <-- ROOT CAUSE
  |     |     |-- [Pool size too small]
  |     |-- [Database server down]
  |-- [Unhandled exception in handler]
  |-- [Middleware error]
```

### Timeline Analysis
For intermittent or hard-to-reproduce bugs:
1. Collect timestamps of every occurrence
2. Correlate with deployments, config changes, traffic spikes
3. Look for patterns (time of day, day of week, after specific actions)
4. Check for resource exhaustion patterns (memory, connections, file handles)

## Debugging by System Layer

### Application Layer
- Add structured logging at decision points
- Check input validation and type coercion
- Verify error handling paths (are errors swallowed?)
- Look for race conditions in concurrent code

### Data Layer
- Check for N+1 queries causing slowness
- Verify data integrity (NULL values where unexpected, encoding issues)
- Inspect transaction boundaries (partial commits)
- Check for deadlocks in concurrent access patterns

### Infrastructure Layer
- Verify DNS resolution and network connectivity
- Check resource limits (memory, CPU, file descriptors, connection pools)
- Inspect container health and restart counts
- Review load balancer health checks and routing

### Integration Layer
- Check API contract changes (breaking changes in upstream services)
- Verify timeout and retry configurations
- Inspect TLS certificate expiry and trust chains
- Check for clock skew between services

## Common Bug Patterns

| Pattern | Symptoms | Usual Cause |
|---------|----------|-------------|
| Works locally, fails in CI/prod | Environment-specific behavior | Missing env vars, different versions, network restrictions |
| Fails after running for hours | Resource leak | Unclosed connections, growing caches, event listener accumulation |
| Fails under load | Concurrency bug | Race condition, deadlock, connection pool exhaustion |
| Fails intermittently | Timing-dependent | Network timeouts, garbage collection pauses, external service flakiness |
| Fails after deployment | Regression | New code, config change, dependency update |
| Fails on specific data | Edge case | Unicode, large values, NULL, special characters, timezone issues |

## Logging Strategy for Debugging

When adding debug logging, capture:
- **Request ID** -- correlate all logs for one request
- **Input values** -- what data triggered the bug
- **Decision points** -- which branch was taken and why
- **Timing** -- how long each step took
- **State transitions** -- before and after values

Remove debug logging after fixing the bug. Use structured logging (JSON) so logs are searchable.

## Capabilities

- debugging-methodology
- investigation-techniques
- fault-analysis
- timeline-correlation
- root-cause-frameworks
