---
name: self-improving-agent
description: Structured learning capture and self-improvement system. Use after completing a significant task, when the user corrects you, when you discover a knowledge gap, or when you find a better way to do something. Also use proactively to review past learnings before starting similar tasks. Triggers on phrases like "learn from this", "remember this for next time", "review your learnings", or automatically after corrections and task completions.
---

# Self-Improving Agent

A structured system for capturing learnings, tracking mistakes, and systematically
improving over time. After every significant task, log what worked, what failed,
and what was learned.

## When to Use

Activate this skill:
- After completing any significant task (success or failure)
- When the user corrects you on something
- When you discover you did not know something you should have
- When you find a more effective pattern than what you used before
- Before starting a task similar to one where you previously had issues
- When the user asks you to review or improve your learnings

## Learning Entry Format

Each learning gets a unique ID and structured fields.

### Entry Template

```markdown
## [LRN-YYYYMMDD-NNN]

- **Category:** [correction | knowledge_gap | best_practice | optimization]
- **Date:** [YYYY-MM-DD]
- **Context:** [What task were you performing?]
- **Description:** [What happened? What did you learn?]
- **Resolution:** [How was it resolved? What is the correct approach?]
- **Applicable to:** [What types of future tasks does this apply to?]
- **Severity:** [low | medium | high]
- **Promoted:** [no | yes — if promoted to AGENT.md or SOUL.md]
```

### Entry ID Convention

- Format: `LRN-YYYYMMDD-NNN` where NNN is a zero-padded sequence number for that day.
- Example: `LRN-20260318-001` is the first learning on March 18, 2026.
- Check existing entries to determine the next sequence number.

## Categories

### Correction

The user corrected something you did wrong.

**When to log:** Any time the user says "no", "that's wrong", "I meant...",
"actually...", or otherwise indicates you made an error.

**What to capture:**
- What you did wrong
- What the user expected
- Why you made the mistake (misunderstanding, assumption, missing context)
- The correct approach going forward

**Example:**
```markdown
## [LRN-20260318-001]

- **Category:** correction
- **Date:** 2026-03-18
- **Context:** User asked to deploy to staging
- **Description:** I deployed to production instead of staging because I assumed
  the default environment. User corrected me immediately.
- **Resolution:** Always confirm the target environment explicitly. Default to
  staging unless the user says "production" or "prod".
- **Applicable to:** All deployment tasks
- **Severity:** high
- **Promoted:** no
```

### Knowledge Gap

You discovered something you did not know.

**When to log:** When you have to look something up, when you give an incorrect
answer due to missing knowledge, or when you learn a new fact relevant to your work.

**What to capture:**
- What you did not know
- How you discovered the gap
- The correct information
- Where to find this information in the future

### Best Practice

You discovered an effective pattern worth remembering.

**When to log:** When a particular approach works especially well, when the user
teaches you a preferred workflow, or when you find a method that should become standard.

**What to capture:**
- The practice and why it works
- When to apply it
- Any prerequisites or constraints

### Optimization

You found a better way to do something you were already doing.

**When to log:** When you find a faster, cleaner, or more reliable approach to
a task you have done before.

**What to capture:**
- The old approach and its downsides
- The new approach and why it is better
- Measurable improvement if applicable

## Storage

All learnings are stored in `workspace/LEARNINGS.md`.

### File Structure

```markdown
# Learnings Log

## Summary Statistics

- Total entries: [N]
- Corrections: [N]
- Knowledge gaps: [N]
- Best practices: [N]
- Optimizations: [N]
- Last updated: [YYYY-MM-DD]

---

## [LRN-YYYYMMDD-NNN]
[Entry content]

---

## [LRN-YYYYMMDD-NNN]
[Entry content]
```

New entries are appended at the bottom. Update the summary statistics after each
new entry.

## Pre-Task Review

Before starting a task, check `workspace/LEARNINGS.md` for relevant entries.

### Review Process

1. Read `workspace/LEARNINGS.md` if it exists.
2. Search for entries related to the current task type.
3. Pay special attention to `correction` and `knowledge_gap` entries.
4. Apply any relevant learnings to the current task plan.
5. If a relevant learning exists, explicitly acknowledge it:
   "Based on a previous learning [LRN-ID], I will [adjusted approach]."

## Weekly Review and Promotion

Periodically review all learnings and identify those worth promoting to the
system prompt.

### Promotion Criteria

A learning should be promoted to `AGENT.md` or `SOUL.md` when:
- It applies broadly across many task types (not just one specific case).
- It has been validated through multiple occurrences.
- It represents a fundamental principle, not a one-off fix.
- It would prevent recurring mistakes.

### Promotion Process

1. Review all entries since last review.
2. Identify candidates meeting promotion criteria.
3. Draft a concise rule or guideline for the system prompt.
4. Add the rule to the appropriate section of `AGENT.md` or `SOUL.md`.
5. Mark the source entries as `Promoted: yes`.
6. Do not delete promoted entries; they serve as the detailed record.

### Weekly Review Checklist

- [ ] Review all new entries since last review
- [ ] Identify patterns: are there repeated mistakes in the same category?
- [ ] Check if any corrections have occurred more than once (systematic issue)
- [ ] Promote broadly applicable learnings
- [ ] Update summary statistics

## Error Frequency Tracking

Track how often errors occur by category to identify systematic weaknesses.

### Tracking Method

Maintain a section in `workspace/LEARNINGS.md`:

```markdown
## Error Frequency

| Category      | This Week | Last Week | Trend |
|---------------|-----------|-----------|-------|
| correction    | 2         | 4         | DOWN  |
| knowledge_gap | 1         | 1         | FLAT  |
| best_practice | 3         | 2         | UP    |
| optimization  | 1         | 0         | UP    |
```

### Pattern Detection

If corrections in the same area occur 3 or more times:
1. Flag it as a systematic weakness.
2. Create a dedicated rule in `AGENT.md` to address it.
3. Log the pattern detection itself as a learning.

## Important Rules

- **Log immediately.** Do not wait; capture the learning right after it happens.
- **Be specific.** Vague learnings like "be more careful" are useless. State exactly
  what to do differently.
- **Never delete learnings.** They are an append-only log. Mark outdated ones as
  superseded instead.
- **Check before repeating.** Before any significant task, scan for related learnings.
  Never make the same mistake twice.
- **Honesty over ego.** Log mistakes accurately. The goal is improvement, not a
  perfect record.
