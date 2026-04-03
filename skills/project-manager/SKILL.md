---
name: project-manager
description: "🗂️ Agile sprints, risk registers, velocity tracking, and status reporting. Use this skill whenever the user's task involves project-management, agile, scrum, planning, risk, or any related topic, even if they don't explicitly mention 'Project Manager'."
---

# 🗂️ Project Manager

> **Category:** business | **Tags:** project-management, agile, scrum, planning, risk

Delivery is a system, not a heroic effort. The PM's job is to make the path to "shipped" visible, remove obstacles before the team hits them, and say the hard truths about scope and timeline early enough to act on them.

## When to Use

- Planning sprints, estimating work, or breaking down epics into stories
- A project is behind schedule and needs a recovery plan
- Building or maintaining a risk register
- Writing status reports for stakeholders
- Negotiating scope when constraints change (budget, timeline, team)
- Running retrospectives or improving team velocity

## Core Principles

- **Surface bad news early.** A missed deadline reported two weeks before the date is a planning problem. Reported the day before, it is a trust problem. Status reports exist to prevent surprises.
- **Scope is the only lever you control.** You cannot add hours to the day or make developers work faster. When timeline pressure increases, negotiate scope using MoSCoW (see below). Every "must-have" added means another item moves to "won't-have."
- **Velocity is descriptive, not prescriptive.** Velocity tells you how fast the team actually moves, not how fast you wish they moved. Use it for forecasting, never as a performance metric. Gaming velocity destroys its predictive value.
- **Risks are cheap to manage, expensive to react to.** A risk register takes 30 minutes per week to maintain. An unmanaged risk that fires costs days or weeks. Every risk needs an owner, a trigger, and a mitigation plan.
- **Done means deployed and verified, not "code complete."** If it is not in production and working, it is not done. Include QA, deployment, and monitoring in every estimate.

## Workflow

1. **Break down work.** Epic -> stories -> tasks. Each story has acceptance criteria, an estimate, and no external dependencies (or those dependencies are tracked).
2. **Estimate with the team.** Use story points or t-shirt sizes. The team estimates, not the PM. Include buffer for unknowns (rule of thumb: 20% for known domain, 40% for new territory).
3. **Plan the sprint.** Pull from the prioritized backlog up to the team's average velocity. Do not over-commit. Leave 15% capacity for bugs and support work.
4. **Track daily.** Short standups: what shipped, what is blocked. The PM's job is to chase blockers, not to listen to status recitals.
5. **Report weekly.** Status report to stakeholders using the template below. Red/amber/green with actions, not just colors.
6. **Retro every sprint.** What went well, what did not, what to change. Pick one action item and actually do it. A retro without follow-through is theater.

## Project Recovery Framework

When a project is behind schedule, follow these steps in order:

```
1. ASSESS (Day 1)
   - How far behind are we? (days/sprints, not vibes)
   - What caused the slip? (scope creep, underestimation, blockers, attrition)
   - What is the remaining work? (re-estimate from scratch, do not trust old numbers)

2. NEGOTIATE SCOPE (Day 2-3)
   Use MoSCoW to categorize remaining work:
   - Must Have: Ship is broken without this. Non-negotiable.
   - Should Have: Important but the product works without it. Defer to fast-follow.
   - Could Have: Nice-to-have. Cut first.
   - Won't Have: Explicitly out of scope for this release. Write it down.

   Example:
   | Feature              | Original | Recovery  | Rationale                         |
   |----------------------|----------|-----------|-----------------------------------|
   | User authentication  | Must     | Must      | Cannot launch without login       |
   | Admin dashboard      | Must     | Should    | Can use direct DB queries for v1  |
   | Email notifications  | Should   | Could     | Manual process acceptable at scale|
   | Dark mode            | Could    | Won't     | Zero impact on core value prop    |

3. REPLAN (Day 3-5)
   - New timeline based on Must Haves only
   - Identify the critical path (longest chain of dependent tasks)
   - Add 20% buffer to the new estimate
   - Get team buy-in on the new plan

4. COMMUNICATE (Day 5)
   - Stakeholder briefing: what changed, why, new timeline, what was cut
   - Frame cuts as "deferred to phase 2," not "removed"
   - Get explicit sign-off on the revised scope
```

## Output Templates

### Sprint Planning Summary

```
## Sprint [X] Plan | [Start Date] - [End Date]

**Sprint goal:** [One sentence -- what does "success" look like?]
**Capacity:** [X] story points (team velocity avg: [Y])

### Committed Stories
| # | Story | Points | Owner | Dependencies |
|---|-------|--------|-------|-------------|
| 1 | ...   | 5      | @dev  | None        |

### Carried Over from Last Sprint
| # | Story | Points | Reason for carryover |

**Risks this sprint:**
- [Risk]: [Mitigation]

**Not planned (next sprint candidates):**
- [Story] -- deferred because [reason]
```

### Status Report

```
## Weekly Status | [Date] | Project: [Name]

**Overall:** 🟢 Green / 🟡 Amber / 🔴 Red
**Sprint progress:** [X/Y] stories complete ([Z] points)
**Timeline:** On track / [N days] behind -- [action to recover]

### Completed This Week
- [Deliverable]: [Impact]

### In Progress
- [Item]: [Expected completion] | [Blocker if any]

### Risks & Issues
| Risk/Issue | Impact | Owner | Action | Due |
|-----------|--------|-------|--------|-----|

### Decisions Needed
- [Decision]: [Context] -- need answer by [date]
```

### Risk Register

```
| ID | Risk | Likelihood (1-5) | Impact (1-5) | Score | Owner | Trigger | Mitigation | Status |
|----|------|-----------------|-------------|-------|-------|---------|------------|--------|
| R1 | Key developer leaves mid-sprint | 2 | 5 | 10 | PM | Resignation notice | Cross-train on critical modules, document architecture decisions | Open |
```

## Common Patterns

- **Timeboxed spikes for unknowns.** When nobody knows how long something will take, allocate a fixed timebox (e.g., 2 days) to investigate. After the spike, estimate the real work. This prevents unbounded research.
- **"Two pizza" standups.** If standup takes more than 15 minutes, the team is too large or people are problem-solving in standup. Take detailed discussions offline. Standup is for surfacing blockers, not resolving them.
- **Rolling wave planning.** Plan the next 2 sprints in detail, the next 2-4 at epic level, beyond that at theme level. Detailed plans for Q3 in Q1 are fiction.
- **Dependency boards.** If multiple teams are involved, maintain a visual dependency map. Untracked cross-team dependencies are the #1 cause of "surprise" delays.

## Anti-Patterns

- **Using velocity as a performance metric.** Teams will inflate estimates to hit the number. Velocity becomes meaningless for forecasting, which was its only purpose.
- **"Green" status reports until the last week.** If the project was green for 8 weeks and suddenly red, the status reports were lies. Amber exists for a reason -- use it when risks are materializing, not after they have hit.
- **Skipping retrospectives when busy.** Busy sprints are exactly when retros matter most. "We don't have time to improve" guarantees the next sprint is equally painful.
- **Planning at 100% capacity.** Bugs happen. Production incidents happen. People get sick. Plan at 80-85% capacity or every sprint will carry over work.
- **Scope negotiation without stakeholder sign-off.** Cutting scope unilaterally and hoping nobody notices always backfires. Get explicit agreement in writing.

## Capabilities

- project-management
- agile
- risk-management
- planning
