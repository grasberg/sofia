# SPEC: Goals UI Redesign — Fullscreen Focus

## Objective

Replace Sofia's fragmented goals experience (kanban board + activity tab + completed tab) with a single **dedicated goals page** built around two concepts:

1. **A minimal goal list** — input bar at top, goal cards below showing name, progress, and status
2. **A fullscreen vertical timeline** — click any goal to see every step in real-time, transitioning to a polished result card on completion

**Target users:** Both technical operators and non-technical stakeholders. The UI must be readable without understanding agent architecture.

**Success criteria:**
- Goal creation, progress tracking, and result viewing happen on one page — no tab switching
- A non-technical user can understand what's happening by looking at the screen for 5 seconds
- Completed goals produce a stakeholder-presentable result card without reformatting
- All existing goal functionality (create, pause, restart, delete) remains accessible

---

## Architecture

### Current state (being replaced)

| File | Route | Purpose |
|------|-------|---------|
| `templates/goals.html` | `/ui/goals` | Kanban board (4 columns) + detail modal |
| `templates/activity.html` | `/ui/activity` | Live agent work snapshot |
| `templates/completed.html` | `/ui/completed` | Completed goals with expandable detail |
| `handler_activity.go` | `/api/activity` | Activity data endpoint |

3 sidebar nav items: Goals, Activity, Completed.

### New state

| File | Route | Purpose |
|------|-------|---------|
| `templates/goals.html` | `/ui/goals` | **Replaces all three** — list + timeline + results |

1 sidebar nav item: Goals. Activity and Completed nav items removed.

### Unchanged backend

| Resource | Location | Notes |
|----------|----------|-------|
| `Goal` struct | `pkg/autonomy/goals.go:45-59` | No schema changes |
| `GoalResult` struct | `pkg/autonomy/goals.go:35-43` | Already has summary, artifacts, next_steps, unmet_criteria |
| `Plan` / `PlanStep` | `pkg/tools/plan_types.go:50-84` | No changes |
| `GoalLogEntry` | `pkg/memory/db_goals.go:11-21` | No changes |
| `GoalManager` | `pkg/autonomy/goals.go:61+` | No changes |
| WebSocket events | `pkg/autonomy/service_goals.go` | All events already broadcast — no changes |

### New backend

| Change | Location | Details |
|--------|----------|---------|
| Timeline API | `pkg/web/handler_goals.go` | New `GET /api/goals/{id}/timeline` — returns goal + plan steps + log in one payload |
| `agent_count` field | `pkg/tools/manage_goals.go` | Add to tool parameters + `GoalSpec` |
| Agent count in goal creation | `pkg/autonomy/goals.go` | Store `agent_count` in goal metadata |

---

## UI Specification

### 1. Goal Input Bar

Location: top of the goals page, always visible.

```
┌─────────────────────────────────────────────────────────────────┐
│  [Describe your goal...                              ] [Start] │
│  Priority: ○ Low  ● Medium  ○ High    Agents: [Auto ▼]        │
└─────────────────────────────────────────────────────────────────┘
```

**Fields:**
- Text input (required) — goal description, placeholder: "Describe your goal..."
- Priority selector — radio group: low / medium / high. Default: medium
- Agent count — dropdown: Auto (default), 1, 2, 3, 4, 5
- Start button — submits to `/api/chat` with structured prompt (same mechanism as today)

**Behavior:**
- On submit: input clears, "Sofia is planning..." status appears, new goal appears in list within seconds
- Input disabled while submitting
- Error state: red text below input with error message

**Acceptance criteria:**
- [ ] Goal created with correct priority and description
- [ ] Agent count passed through to goal creation
- [ ] Input validation: non-empty text required
- [ ] Keyboard: Enter submits, Shift+Enter for newline

### 2. Goal List

Location: below input bar, scrollable.

```
┌─────────────────────────────────────────────────────────────────┐
│ ● Building REST API for users          ████████░░  6/8    45s  │
│ ● Setting up CI/CD pipeline            ██░░░░░░░░  1/5   1m2s │
│ ✓ Deploy staging environment           Completed · 2 min ago   │
│ ✓ Database migration script            Completed · yesterday   │
└─────────────────────────────────────────────────────────────────┘
```

**Each list item shows:**
- Status indicator: green pulsing dot (working), blue check (done), red dot (needs attention)
- Goal name (truncated to one line)
- For working: progress bar + step fraction (6/8) + elapsed time
- For done: "Completed" + relative timestamp
- For needs-attention: red "Needs attention" label

**Sorting:** Working goals first (newest on top), then done goals (most recent first).

**Actions (on hover/right-click):**
- Pause (working goals only)
- Restart (failed/needs-attention goals)
- Delete (all goals, with confirmation)

**Behavior:**
- Click any goal → transition to fullscreen timeline view
- List updates in real-time via WebSocket events
- Auto-refresh every 15 seconds as fallback

**Acceptance criteria:**
- [ ] All goals visible in one list (no separate tabs for completed)
- [ ] Working goals sort above done goals
- [ ] Real-time updates on step completion (progress bar advances)
- [ ] Click navigates to timeline view
- [ ] Context menu with pause/restart/delete actions

### 3. Fullscreen Timeline View

Entered by clicking a goal from the list. Replaces the list content (back button to return).

#### 3a. Timeline Header

```
┌─────────────────────────────────────────────────────────────────┐
│  ← Back                                                        │
│                                                                 │
│              ┌──────┐                                           │
│              │  75% │  (progress ring, large)                   │
│              └──────┘                                           │
│         Building REST API for users                             │
│     ████████████████████░░░░░  6/8 steps · 2 agents · 3m 42s   │
│                                                                 │
│     Priority: HIGH    Status: Working                           │
└─────────────────────────────────────────────────────────────────┘
```

**Shows:**
- Back button (returns to goal list)
- Large SVG progress ring (centered, ~120px diameter)
- Goal name (large text)
- Linear progress bar + "X/Y steps · N agents · elapsed time"
- Priority badge + status label
- Action buttons: Pause / Stop (for working goals)

#### 3b. Step Timeline

Below the header, scrollable. Steps appear top-to-bottom in execution order.

```
  ✅ Step 1: Design database schema
     agent-1 · 45s
     "Created users, posts, and comments tables with..."
     
  ✅ Step 2: Implement REST endpoints  
     agent-2 · 1m 12s
     "8 endpoints created: GET/POST /users, GET/POST..."

  🔄 Step 3: Add authentication middleware    ← YOU ARE HERE
     agent-1 · running 30s...
     
  ⏳ Step 4: Write integration tests
     Waiting for step 3

  ⏳ Step 5: API documentation
     Ready
```

**Each step card shows:**
- Status icon: ✅ completed (green check), 🔄 in-progress (blue spinner), ⏳ pending (gray clock), ❌ failed (red x), 🔄 retrying (amber)
- Step number + description
- Agent name + duration (completed) or "running Xs..." (in-progress) or "Waiting for step N" (blocked) or "Ready" (pending, unblocked)
- Result snippet (completed steps, truncated to ~200 chars)
- Click to expand: full result text, acceptance criteria, verify command output

**Real-time behavior:**
- New steps animate in (slide-down + fade-in)
- When a step starts: transitions from ⏳ to 🔄 with pulse animation
- When a step completes: transitions from 🔄 to ✅, result text fades in
- When a step retries: shows "Retrying (attempt N)..." in amber
- Auto-scroll to keep the active step visible

**Acceptance criteria:**
- [ ] Steps appear in real-time via WebSocket
- [ ] Step expansion shows full result, acceptance criteria, verify output
- [ ] Dependencies shown ("Waiting for step N")
- [ ] Retry attempts visible but non-alarming
- [ ] Active step auto-scrolls into view

#### 3c. Completion Transition

When all steps complete, the timeline view transitions to a result card.

```
┌─────────────────────────────────────────────────────────────────┐
│  ← Back                                                [Share] │
│                                                                 │
│              ┌──────┐                                           │
│              │ 100% │  (progress ring, filled, blue)            │
│              └──────┘                                           │
│         Building REST API for users                             │
│         Completed in 4m 23s · 8 steps · 2 agents               │
│                                                                 │
│  ── Summary ─────────────────────────────────────────────────── │
│  Built a complete REST API with user CRUD, JWT authentication,  │
│  input validation, and comprehensive test suite. All endpoints  │
│  pass integration tests.                                        │
│                                                                 │
│  ── Files Generated ─────────────────────────────────────────── │
│  📄 goals/goal-1-rest-api/schema.sql                            │
│  📄 goals/goal-1-rest-api/main.go                               │
│  📄 goals/goal-1-rest-api/handlers.go                           │
│  📄 goals/goal-1-rest-api/auth.go                               │
│  📄 goals/goal-1-rest-api/main_test.go                          │
│                                                                 │
│  ── Unmet Criteria (if any) ─────────────────────────────────── │
│  ⚠ Rate limiting not implemented (deferred)                     │
│                                                                 │
│  ── Next Steps ──────────────────────────────────────────────── │
│  • Add rate limiting middleware                                  │
│  • Set up production deployment                                  │
│                                                                 │
│  [Show step timeline ▼]                                         │
└─────────────────────────────────────────────────────────────────┘
```

**Shows:**
- Progress ring at 100% (blue, not green — signals "done" not "active")
- Goal name + completion stats (time, steps, agents)
- Summary paragraph (from `GoalResult.Summary`)
- Files generated (from `GoalResult.Artifacts`) — clickable paths
- Unmet criteria (from `GoalResult.UnmetCriteria`) — amber warning list
- Next steps (from `GoalResult.NextSteps`)
- "Show step timeline" toggle — collapses/expands the step timeline below
- Share button — copies markdown-formatted summary to clipboard

**Acceptance criteria:**
- [ ] Transition from timeline to result card is smooth (no page reload)
- [ ] All GoalResult fields rendered (summary, artifacts, unmet_criteria, next_steps)
- [ ] Share button copies clean markdown
- [ ] Step timeline still accessible via toggle
- [ ] File paths are displayed clearly (not as raw strings)

### 4. Needs Attention State

When auto-retry exhausts (after N failures), the goal enters "needs attention" state.

**In list:** Red dot + "Needs attention" label replaces progress bar.

**In timeline:** Banner appears above the failed step:

```
┌─────────────────────────────────────────────────────────────────┐
│ ⚠ This goal needs your help                                    │
│ Step 3 failed after 3 attempts: "npm not found in PATH"         │
│                                                                 │
│ [Retry Step]  [Restart Goal]  [Mark Failed]                     │
└─────────────────────────────────────────────────────────────────┘
```

**Acceptance criteria:**
- [ ] Banner appears only after auto-retry exhausts
- [ ] Clear explanation of what failed and why
- [ ] Actionable buttons (retry step, restart goal, mark failed)
- [ ] Red dot visible in goal list for at-a-glance awareness

---

## API Specification

### New: `GET /api/goals/{id}/timeline`

Returns all data needed to render the timeline view in a single request.

**Response:**
```json
{
  "goal": {
    "id": 1,
    "name": "Build REST API",
    "description": "...",
    "status": "in_progress",
    "priority": "high",
    "phase": "implement",
    "created_at": "2026-04-11T10:00:00Z",
    "updated_at": "2026-04-11T10:04:23Z",
    "goal_result": null
  },
  "plan": {
    "id": "plan-uuid",
    "status": "in_progress",
    "steps": [
      {
        "index": 0,
        "description": "Design database schema",
        "status": "completed",
        "result": "Created users table with...",
        "assigned_to": "agent-1",
        "acceptance_criteria": "Schema file exists and is valid SQL",
        "verify_command": "sqlite3 test.db < schema.sql",
        "depends_on": [],
        "retry_count": 0
      }
    ]
  },
  "log": [
    {
      "id": 1,
      "goal_id": 1,
      "agent_id": "agent-1",
      "step": "Design database schema",
      "result": "Created users table...",
      "success": true,
      "duration_ms": 45000,
      "created_at": "2026-04-11T10:00:45Z"
    }
  ],
  "agents": {
    "agent-1": "Backend Specialist",
    "agent-2": "Code Reviewer"
  }
}
```

### Modified: Goal creation prompt

The chat prompt for goal creation (in `goals.html` JavaScript) adds `agent_count`:

```
1. Create this as a {PRIORITY} priority goal using manage_goals (action: "add", agent_count: {N}).
2. Create a detailed plan...
```

### Modified: `manage_goals` tool

Add `agent_count` parameter (integer, optional, default 0 = auto):
- Stored in goal metadata (new field on GoalSpec or as a separate property)
- Passed to autonomy service for step dispatch concurrency control

---

## Navigation Changes

### layout.html sidebar

**Remove:** `nav-activity` and `nav-completed` links.

**Keep:** `nav-goals` link (unchanged route `/ui/goals`).

**Update:** `nav-goals` label from "Goals" to "Goals" (no change needed).

### WebSocket handler in layout.html

**Update:** Remove forwarding to `_activityWsHandler`. Keep forwarding to `_goalsPlanEvent` (renamed appropriately in new template).

---

## File Changes Summary

| Action | File | Notes |
|--------|------|-------|
| **Rewrite** | `pkg/web/templates/goals.html` | New template: input + list + timeline + result card |
| **Delete** | `pkg/web/templates/activity.html` | Absorbed into goals.html timeline |
| **Delete** | `pkg/web/templates/completed.html` | Absorbed into goals.html done state |
| **Delete** | `pkg/web/handler_activity.go` | `/api/activity` endpoint no longer needed |
| **Edit** | `pkg/web/handler_goals.go` | Add `GET /api/goals/{id}/timeline` handler |
| **Edit** | `pkg/web/server.go` | Remove activity/completed embeds + routes; add timeline route |
| **Edit** | `pkg/web/templates/layout.html` | Remove nav-activity, nav-completed; update WS handler |
| **Edit** | `pkg/tools/manage_goals.go` | Add `agent_count` parameter |

---

## Code Style

- **Templates:** Vanilla JS (no framework), Tailwind CSS classes, `var` declarations for browser compat
- **HTML:** Use `escapeHtml()` (defined in layout.html) for all dynamic content
- **Animations:** CSS transitions + `animate-fade-in` / `animate-slide-up` classes from layout.html
- **Icons:** Lucide icons via `data-lucide` attributes + `refreshIcons()` calls
- **WebSocket:** Use existing `window._goalsPlanEvent` pattern for real-time updates
- **API calls:** `fetch()` with JSON parsing, error handling with status display
- **Go handlers:** Follow existing patterns in `handler_goals.go` — JSON response, auth middleware
- **Progress ring:** SVG `<circle>` with `stroke-dasharray` / `stroke-dashoffset` animation

---

## Testing Strategy

### Backend
- Unit test for new timeline handler: verify it returns goal + plan + log in one payload
- Unit test for `agent_count` parameter in manage_goals tool
- Existing tests must continue to pass (goal creation, status updates, plan operations)

### Frontend (manual verification)
- Create a goal → verify it appears in list with progress
- Watch steps execute → verify timeline updates in real-time
- Wait for completion → verify result card renders with all fields
- Test needs-attention state → verify banner and action buttons
- Test pause/restart/delete from list context menu
- Test share button → verify clipboard content
- Test responsive behavior at 768px and 1440px breakpoints

### Regression
- Existing `/api/goals` endpoint unchanged
- Existing `/api/plans` endpoint unchanged
- Chat-based goal creation still works
- WebSocket events still broadcast correctly

---

## Boundaries

### Always do
- Use `escapeHtml()` for all user-provided content in templates
- Maintain existing API backwards compatibility
- Keep WebSocket event format unchanged
- Follow existing Go handler patterns (auth middleware, JSON responses)
- Use Tailwind utility classes (no inline styles)

### Ask first
- Before removing any API endpoint (even if UI no longer uses it)
- Before changing WebSocket event payload format
- Before modifying Goal/Plan/GoalResult structs

### Never do
- Add a JS framework (React, Vue, etc.) — vanilla JS only
- Remove the chat-based goal creation flow — it coexists with the new input bar
- Break existing goal/plan data in MemoryDB
- Add external dependencies (CDN scripts, new npm packages)

---

## Not Doing

- **Kanban board** — replaced by simple list
- **Separate activity/completed tabs** — merged into one page
- **Step dependency graph visualization** — timeline with "Waiting for step N" is sufficient
- **Export/PDF** — Share button copies markdown only
- **Multi-goal comparison** — one goal in focus at a time
- **Manual step reordering** — Sofia owns execution
- **Cost/token usage display** — not in MVP
