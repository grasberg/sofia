---
name: brainstorm
description: "Structured ideation — generates 3+ options with honest trade-offs, effort estimates, and a clear recommendation. Use when comparing approaches, making architecture decisions, or any 'how should I...' / 'X vs Y' question."
---

# Brainstorm

Structured ideation mode that explores multiple approaches before committing to one. Present options with honest trade-offs and let the user decide.

## Core Philosophy

> Present options, let them decide. No code in brainstorm mode -- ideas first, implementation second.

## Process

### Step 1: Clarify the Goal
Before generating options, understand:
- What problem are you solving?
- Who are the users / consumers?
- What are the hard constraints (budget, timeline, team size, existing stack)?

### Step 2: Generate Options
Present at least **3 approaches**, each with:
- One-paragraph description
- Pros (concrete advantages)
- Cons (honest drawbacks)
- Effort estimate (Low / Medium / High)
- Best suited for (when this is the right choice)

### Step 3: Recommend
- Summarize the comparison in a table
- Give a clear recommendation with reasoning
- Acknowledge what you'd lose by not choosing the alternatives

## Output Format

```markdown
## Brainstorm: [Topic]

### Goal
[1-2 sentences: what we're trying to achieve]

### Constraints
- [constraint 1]
- [constraint 2]

---

### Option A: [Name]
[Description]

**Pros:**
- [advantage 1]
- [advantage 2]

**Cons:**
- [drawback 1]
- [drawback 2]

**Effort:** [Low/Medium/High]
**Best for:** [scenario]

---

### Option B: [Name]
[...]

---

### Comparison

| Criteria | Option A | Option B | Option C |
|----------|----------|----------|----------|
| Complexity | Low | Medium | High |
| Scalability | Medium | High | High |
| Time to implement | 1 day | 3 days | 1 week |

### Recommendation
[Which option and why, given the stated constraints]
```

## Brainstorm Topics This Works Well For

- Authentication strategies (JWT vs sessions vs OAuth)
- State management approaches
- Database selection
- API design (REST vs GraphQL vs gRPC)
- Deployment strategies
- Caching strategies
- Monolith vs microservices
- Build vs buy decisions

## Anti-Patterns

- Presenting only one option (that's a recommendation, not a brainstorm)
- Hiding the downsides of your preferred approach
- Writing code during brainstorm (ideas first, code later)
- Analysis paralysis -- 3-4 options is enough, not 10

