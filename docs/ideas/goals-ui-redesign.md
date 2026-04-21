# Goals UI Redesign: Fullscreen Focus

## Problem Statement

How might we redesign Sofia's goal UI so that any user — technical or not — can set a goal, watch every step happen in real-time, and get a polished, demo-ready result when it's done?

The current UI fragments the experience across three views (goals kanban, activity, completed), uses four status columns that require domain knowledge, and produces results buried in modals and technical logs.

## Recommended Direction

**"Fullscreen Focus"** — replace the entire goals surface (kanban + activity tab + completed tab) with a single dedicated page built around two concepts:

1. **A minimal goal list** with input at the top. Each goal shows name, a progress indicator, and a status dot (working/done). Nothing else. This is the overview.

2. **A fullscreen vertical timeline** that opens when you click any goal. While working, steps appear in real-time — each step shows an agent avatar, description, status icon (spinner/check/cross), duration, and result snippet. A large progress ring anchors the top. When the goal completes, the timeline smoothly transitions into a **polished result card** — summary, generated files, time taken — designed to be stakeholder-presentable without editing.

There are only **two visible states**: Working and Done. Failed steps are retried automatically by Sofia. Only if a goal is truly stuck does it surface an "needs attention" indicator — keeping the interface calm and confident by default.

Goal creation includes a **per-goal agent slider** (1-5 agents) so the user controls parallelism. The default is "auto" (Sofia decides based on complexity).

### Why this direction

- **One surface, not three.** Eliminates context-switching between goals/activity/completed tabs.
- **Two states, not four.** Working and Done is the only mental model needed. Paused/failed are internal concerns.
- **Demo-ready by design.** The completion view IS the presentation — no reformatting needed.
- **Scales to both audiences.** Non-technical users see progress ring + step names. Technical users click a step to see full output.

## Key Assumptions to Validate

- [ ] **Kanban removal is safe** — no workflows depend on manually moving goals between columns. Validate by checking if pause/resume is used frequently. If yes, add a subtle pause button to the list item rather than a column.
- [ ] **Auto-retry is reliable enough** — Sofia's error classification and retry logic can handle most failures without user intervention. Validate against the last 20 goal executions: how many required manual intervention?
- [ ] **Per-goal agent count is meaningful** — users can make an informed choice. Validate by offering "auto" as default and tracking whether users ever change it.
- [ ] **Single-goal focus is sufficient** — users rarely need to compare two working goals side-by-side. Validate by observing usage: do users run 1 goal at a time or 5?

## MVP Scope

### In scope

- **Goal input bar** with text field, agent count slider (1-5 + auto), and priority selector (low/medium/high)
- **Goal list** — minimal cards: name, progress fraction (3/8 steps), status dot (green pulse = working, blue check = done, red = needs attention)
- **Fullscreen timeline view** — vertical timeline with step cards showing: agent name, description, status icon, duration, result snippet (truncated). Steps appear in real-time via WebSocket.
- **Progress header** — goal name, large progress ring, agent count, elapsed time
- **Completion transition** — when all steps done, timeline collapses and result card slides in: summary paragraph, file list with paths, total duration, "Share" button (copies markdown summary)
- **Step detail expand** — click any step in timeline to see full result text, verify command output, acceptance criteria
- **Auto-retry visual** — if a step retries, show a subtle "retrying..." label on the step. No user action needed unless escalated.
- **"Needs attention" state** — after N failed retries, goal shows red dot in list + banner in timeline with explanation and manual retry button

### Backend changes

- New API endpoint `GET /api/goals/{id}/timeline` — returns goal + plan steps + log entries in a single timeline-optimized payload
- WebSocket events already exist (`goal_step_start`, `goal_step_end`, etc.) — no changes needed
- Add `agent_count` field to goal creation (manage_goals tool + GoalSpec)
- Completion card data already exists in `GoalResult` struct — just needs consistent population

### Migration

- Replace `goals.html` template entirely
- Remove `activity.html` template (absorbed into timeline)
- Remove `completed.html` template (absorbed into done-state goal cards)
- Keep API endpoints for backwards compatibility; add new timeline endpoint

## Not Doing (and Why)

- **Kanban board** — adds complexity without proportional value. A list with status dots conveys the same information in less space.
- **Separate activity tab** — the timeline IS the activity view. No need for a separate surface.
- **Separate completed tab** — done goals live in the same list, just with a different visual state. Filter by status if needed.
- **Paused state in UI** — if pause/resume is needed, it becomes a subtle action button, not a visible state column.
- **Manual step control** — users don't reorder, skip, or manually trigger steps. Sofia owns execution. The UI is read-only during work.
- **Step dependency graph** — while plans have `depends_on`, visualizing this as a DAG adds complexity. The vertical timeline with "waiting" status is sufficient for MVP.
- **Multi-goal comparison** — no split-screen or diff view between goals. One goal in focus at a time.
- **Export/PDF** — the "Share" button copies markdown. PDF generation is a nice-to-have, not MVP.

## Open Questions

- What happens to the existing chat-based goal creation flow? Should it redirect to the new goals page, or coexist?
- Should done goals auto-archive after N days, or stay in the list indefinitely?
- How does the agent slider map to actual subagent spawning? Is it `max_concurrent` or `total_agents`?
- Should the completion card include cost/token usage? (Useful for technical users, noise for non-technical.)

## Visual Direction

- Clean, minimal, high contrast
- Progress ring as the centerpiece of the timeline header
- Step cards with generous whitespace and clear iconography
- Smooth animations for step appearance and completion transitions
- "Calm technology" — the UI should feel confident, not anxious. No excessive pulsing or spinners.
