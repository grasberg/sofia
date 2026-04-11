# 🤖 Multi-Agent Orchestration

Coordinate specialized AI agents to tackle complex tasks.

---

## Architecture Overview

Sofia's multi-agent system lets you spawn specialized agents that work together. The **Orchestrator** pattern coordinates a team of domain experts.

```
┌──────────────────────────────────────────────────────┐
│                    User Message                       │
│              (Telegram / Discord / Web)               │
└──────────────────────┬───────────────────────────────┘
                       │
┌──────────────────────▼───────────────────────────────┐
│                  Main Agent (Sofia)                   │
│           Routes to appropriate agent                │
└──────────────────────┬───────────────────────────────┘
                       │
┌──────────────────────▼───────────────────────────────┐
│               Orchestrator Agent                      │
│     Decomposes task → Assigns to specialists          │
│                                                       │
│  ┌──────────┐ ┌──────────┐ ┌──────────┐             │
│  │Frontend  │ │Backend   │ │Security  │             │
│  │Specialist│ │Specialist│ │Auditor   │             │
│  └──────────┘ └──────────┘ └──────────┘             │
│  ┌──────────┐ ┌──────────┐ ┌──────────┐             │
│  │Test Eng. │ │DB Arch.  │ │DevOps    │             │
│  └──────────┘ └──────────┘ └──────────┘             │
└──────────────────────────────────────────────────────┘
```

### How Agents Work

1. **Main agent** receives the message and decides if it needs specialist help
2. **Orchestrator** decomposes complex tasks into subtasks
3. **Specialist agents** are spawned via `spawn` tool to handle subtasks
4. **Results** are synthesized back into a unified response
5. Each agent has its own **workspace**, **memory**, and **tool access**

---

## Agent Templates Catalog

Sofia includes 40+ specialized agent templates:

### Development

| Template | ID | Specialty |
|----------|----|-----------|
| Frontend Specialist | `frontend-specialist` | React, Next.js, Tailwind, UI components |
| Backend Specialist | `backend-specialist` | Node.js, Express, FastAPI, databases |
| Mobile Developer | `mobile-developer` | React Native, Flutter, Expo |
| Database Architect | `database-architect` | Prisma, migrations, schema optimization |
| API Architect | `api-architect` | REST, GraphQL, OpenAPI design |
| Game Developer | `game-developer` | Unity, Godot, Unreal, Phaser |

### Quality & Security

| Template | ID | Specialty |
|----------|----|-----------|
| Test Engineer | `test-engineer` | Unit tests, E2E, coverage, TDD |
| Security Auditor | `security-auditor` | Auth, vulnerabilities, OWASP |
| Penetration Tester | `penetration-tester` | Active vulnerability testing |
| QA Automation Engineer | `qa-automation-engineer` | Test automation, CI integration |
| Performance Optimizer | `performance-optimizer` | Profiling, bottlenecks, caching |

### DevOps & Infrastructure

| Template | ID | Specialty |
|----------|----|-----------|
| DevOps Engineer | `devops-engineer` | CI/CD, PM2, deployment, monitoring |
| Infrastructure Architect | `infrastructure-architect` | Cloud architecture, scaling |
| Release Engineer | `release-engineer` | Release pipelines, versioning |

### Analysis & Research

| Template | ID | Specialty |
|----------|----|-----------|
| Explorer Agent | `explorer-agent` | Codebase discovery, dependency mapping |
| Code Archaeologist | `code-archaeologist` | Legacy code analysis, refactoring |
| Research Analyst | `research-analyst` | Market research, data analysis |
| Code Reviewer | `code-reviewer` | PR reviews, code quality |

### Planning & Management

| Template | ID | Specialty |
|----------|----|-----------|
| Project Planner | `project-planner` | Task breakdown, milestones, roadmaps |
| Product Manager | `product-manager` | Feature prioritization, user stories |
| Product Owner | `product-owner` | Backlog management, sprint planning |
| Technical Lead | `technical-lead` | Architecture decisions, tech choices |
| Operations Manager | `operations-manager` | Process optimization, workflows |

### Content & Marketing

| Template | ID | Specialty |
|----------|----|-----------|
| Documentation Writer | `documentation-writer` | README, API docs, guides |
| Content Creator | `content-creator` | Blog posts, copywriting |
| SEO Specialist | `seo-specialist` | SEO optimization, meta tags |
| Brand Marketing Lead | `brand-marketing-lead` | Brand strategy, campaigns |
| Growth Strategist | `growth-strategist` | Growth hacking, analytics |

### Business & Strategy

| Template | ID | Specialty |
|----------|----|-----------|
| Startup Founder | `startup-founder` | MVP planning, lean methodology |
| Fintech Specialist | `fintech-specialist` | Financial tech, compliance |

### Specialized

| Template | ID | Specialty |
|----------|----|-----------|
| Debugger | `debugger` | Root cause analysis, systematic debugging |
| AI Architect | `ai-architect` | ML/AI system design |
| AI Ethics Advisor | `ai-ethics-advisor` | Responsible AI, bias detection |
| Embedded IoT Engineer | `embedded-iot-engineer` | Embedded systems, IoT protocols |
| Accessibility Auditor | `accessibility-auditor` | WCAG compliance, a11y |
| Lifestyle Concierge | `lifestyle-concierge` | Personal recommendations |
| Wellness Coach | `wellness-coach` | Health, fitness, mindfulness |
| Talent Manager | `talent-manager` | Hiring, team building |

### Orchestration

| Template | ID | Specialty |
|----------|----|-----------|
| Orchestrator | `orchestrator` | Multi-agent coordination, synthesis |
| Personal Assistant | `personal-assistant` | General tasks, scheduling |

---

## Configuring Agents

### Adding Agents to config.json

```json
{
  "agents": {
    "defaults": {
      "workspace": "~/.sofia/workspace",
      "model_name": "gpt-4o",
      "max_tokens": 32768,
      "max_tool_iterations": 50,
      "max_concurrent_subagents": 2
    },
    "list": [
      {
        "id": "main",
        "default": true,
        "name": "Sofia",
        "subagents": { "allow_agents": ["*"] }
      },
      {
        "id": "orchestrator",
        "name": "Orchestrator",
        "template": "orchestrator",
        "subagents": { "allow_agents": ["*"] }
      },
      {
        "id": "frontend-specialist",
        "name": "Frontend Specialist",
        "template": "frontend-specialist",
        "subagents": { "allow_agents": ["*"] }
      },
      {
        "id": "backend-specialist",
        "name": "Backend Specialist",
        "template": "backend-specialist",
        "subagents": { "allow_agents": ["*"] }
      },
      {
        "id": "test-engineer",
        "name": "Test Engineer",
        "template": "test-engineer",
        "subagents": { "allow_agents": ["*"] }
      }
    ]
  }
}
```

### Per-Agent Overrides

Each agent can override defaults:

```json
{
  "id": "security-auditor",
  "name": "Security Auditor",
  "template": "security-auditor",
  "model_name": "gpt-4o",
  "max_tokens": 16384,
  "subagents": { "allow_agents": ["*"] },
  "summarization": { "enabled": true }
}
```

| Field | Type | Description |
|-------|------|-------------|
| `id` | string | Unique identifier (used for workspace naming) |
| `name` | string | Display name |
| `template` | string | Agent template to use |
| `default` | bool | Whether this is the primary agent |
| `model_name` | string | Override default model for this agent |
| `max_tokens` | int | Override default token limit |
| `subagents.allow_agents` | string[] | Which agents this one can spawn (`["*"]` = all) |
| `summarization` | object | Conversation summarization settings |

---

## The Orchestrator Pattern

The orchestrator is the coordinator. It:

1. **Receives** a complex task
2. **Decomposes** it into domain-specific subtasks
3. **Spawns** specialist agents for each subtask
4. **Monitors** progress and handles failures
5. **Synthesizes** results into a unified response

### How Spawning Works

When the orchestrator spawns a subagent:

```
┌─────────────┐     spawn      ┌──────────────────┐
│ Orchestrator │ ──────────────►│ Frontend Specialist│
│             │                 │ (own workspace)    │
│             │ ◄────────────── │ (own memory)       │
│             │     result     │ (own tools)        │
└─────────────┘                 └──────────────────┘
```

- Each subagent gets its own **workspace** at `~/.sofia/workspace-{agent-id}/`
- Each subagent has its own **conversation memory**
- Subagents can use all tools available to the parent
- Results are returned to the orchestrator for synthesis

### Orchestrator Workflow Example

```
User: "Build me a REST API for a todo app"

Orchestrator:
  1. Plan → Decompose into subtasks
  2. Spawn backend-specialist → Design API + implement routes
  3. Spawn database-architect → Design schema + migrations
  4. Spawn test-engineer → Write tests
  5. Spawn security-auditor → Review auth implementation
  6. Synthesize → Combine all outputs into final result
```

---

## Subagent Spawning & Communication

### Spawning a Subagent

The `spawn` tool creates a subagent:

```
spawn(
  task: "Review the authentication middleware for security vulnerabilities",
  agent_id: "security-auditor",
  label: "security-review"
)
```

| Parameter | Type | Description |
|-----------|------|-------------|
| `task` | string | The task description for the subagent |
| `agent_id` | string | Optional: Target agent ID to delegate to |
| `label` | string | Optional: Short label for display |
| `skills` | string[] | Optional: Skills to equip the subagent with |

### Scratchpad for Inter-Agent Communication

Agents share data via the `scratchpad` tool:

```
# Agent A writes
scratchpad(operation: "write", key: "api-design", value: "{...}", group: "todo-project")

# Agent B reads
scratchpad(operation: "read", key: "api-design", group: "todo-project")
```

| Parameter | Type | Description |
|-----------|------|-------------|
| `operation` | string | `"write"`, `"read"`, or `"list"` |
| `key` | string | Key to read/write |
| `value` | string | Value to store (for write) |
| `group` | string | Namespace for the data |

### Task Tracking

Track subagent progress with the `task` tool:

```
task(action: "create", title: "Implement auth middleware", description: "JWT-based auth")
task(action: "update", id: "task-1", status: "in_progress")
task(action: "list")
```

---

## Agent Isolation

Each agent operates in isolation:

| Aspect | Isolation Level |
|--------|----------------|
| **Workspace** | `~/.sofia/workspace-{agent-id}/` — separate directories |
| **Memory** | Separate conversation history per agent |
| **Tools** | Inherits parent's tools, can be restricted via `subagents.allow_agents` |
| **Files** | Can be restricted to workspace with `restrict_to_workspace: true` |

### Workspace Structure

```
~/.sofia/
├── config.json                    # Global config
├── memory.db                      # Global memory
├── audit.db                       # Audit log
├── workspace/                     # Main agent workspace
│   └── skills/                    # Installed skills
├── workspace-orchestrator/        # Orchestrator workspace
├── workspace-frontend-specialist/ # Frontend agent workspace
├── workspace-backend-specialist/  # Backend agent workspace
└── workspace-test-engineer/       # Test agent workspace
```

---

## The Evolution System

Sofia can **self-improve** by creating, modifying, and retiring agents based on usage patterns.

### How Evolution Works

```
┌─────────────────────────────────────────────┐
│              Evolution Cycle                 │
│                                             │
│  1. Analyze usage patterns                  │
│  2. Identify gaps (no agent for task X)     │
│  3. Create new agents for common tasks      │
│  4. Retire unused agents                    │
│  5. Optimize prompts based on feedback      │
│  6. Consolidate memories                    │
└─────────────────────────────────────────────┘
```

### Configuration

```json
{
  "evolution": {
    "enabled": true,
    "model": "gemma4:31b-cloud",
    "interval_minutes": 30,
    "max_cost_per_day": 5,
    "daily_summary": true,
    "daily_summary_time": "08:00",
    "self_modify_enabled": true,
    "max_agents": 20,
    "require_approval": false,
    "retirement_threshold": 0.3,
    "retirement_min_tasks": 5,
    "retirement_inactive_days": 7,
    "memory_consolidation": false,
    "consolidation_interval_h": 6,
    "skill_auto_improve": false
  }
}
```

| Setting | Description |
|---------|-------------|
| `self_modify_enabled` | Allow agents to modify their own prompts |
| `require_approval` | Require human approval before changes |
| `retirement_threshold` | Usage ratio below which agents are retired (0.3 = 30%) |
| `retirement_inactive_days` | Days without use before retirement |
| `max_agents` | Maximum number of agents (prevents runaway creation) |

---

## Autonomy Features

### Goals

Sofia can pursue long-term goals autonomously:

```
manage_goals(
  action: "add",
  name: "Improve test coverage",
  description: "Get test coverage above 80%",
  priority: "high"
)
```

### Suggestions

When enabled, Sofia proactively suggests actions based on context.

### Research

Autonomous research mode lets Sofia investigate topics and report findings.

### Context Triggers

Fire actions when specific conditions are met in conversation:

```
manage_triggers(
  action: "add",
  name: "security-review-trigger",
  condition: "When authentication code is discussed",
  trigger_action: "Spawn security-auditor to review"
)
```

---

## Best Practices

### 1. Start Small, Scale Up

Begin with 2-3 agents and add more as needed:

```json
"list": [
  { "id": "main", "default": true, "name": "Sofia" },
  { "id": "orchestrator", "name": "Orchestrator", "template": "orchestrator" },
  { "id": "backend-specialist", "name": "Backend", "template": "backend-specialist" }
]
```

### 2. Use the Orchestrator for Complex Tasks

Don't try to make one agent do everything. Let the orchestrator decompose and delegate.

### 3. Restrict Subagent Permissions

Limit which agents can spawn which others:

```json
{
  "id": "frontend-specialist",
  "subagents": { "allow_agents": ["test-engineer"] }
}
```

### 4. Set Cost Limits

Prevent runaway API costs:

```json
{
  "autonomy": { "max_cost_per_day": 5 },
  "evolution": { "max_cost_per_day": 5 }
}
```

### 5. Use Workspace Isolation

For production, restrict agents to their workspaces:

```json
{
  "agents": {
    "defaults": { "restrict_to_workspace": true }
  }
}
```

---

## Example: Development Team Workflow

Here's a complete setup for a software development team:

```json
{
  "agents": {
    "defaults": {
      "workspace": "~/.sofia/workspace",
      "model_name": "gpt-4o",
      "model_fallbacks": ["gpt-4o-mini"],
      "max_tokens": 32768,
      "max_tool_iterations": 50,
      "max_concurrent_subagents": 3
    },
    "list": [
      { "id": "main", "default": true, "name": "Sofia", "subagents": { "allow_agents": ["*"] } },
      { "id": "orchestrator", "name": "Orchestrator", "template": "orchestrator", "subagents": { "allow_agents": ["*"] } },
      { "id": "frontend-specialist", "name": "Frontend", "template": "frontend-specialist", "subagents": { "allow_agents": ["test-engineer"] } },
      { "id": "backend-specialist", "name": "Backend", "template": "backend-specialist", "subagents": { "allow_agents": ["database-architect", "test-engineer"] } },
      { "id": "database-architect", "name": "Database", "template": "database-architect" },
      { "id": "test-engineer", "name": "QA", "template": "test-engineer" },
      { "id": "security-auditor", "name": "Security", "template": "security-auditor" },
      { "id": "devops-engineer", "name": "DevOps", "template": "devops-engineer" }
    ]
  }
}
```

### Workflow in Action

```
User: "Build a user authentication system"

Orchestrator:
  📋 Plan created:
  1. [backend-specialist] Design auth API endpoints
  2. [database-architect] Design user schema
  3. [backend-specialist] Implement auth routes
  4. [security-auditor] Review security
  5. [test-engineer] Write auth tests
  6. [devops-engineer] Set up deployment

  🔄 Step 1: Spawning backend-specialist...
  ✅ Auth API design complete

  🔄 Step 2: Spawning database-architect...
  ✅ User schema designed

  🔄 Step 3: Spawning backend-specialist...
  ✅ Auth routes implemented

  🔄 Step 4: Spawning security-auditor...
  ✅ Security review passed (2 suggestions)

  🔄 Step 5: Spawning test-engineer...
  ✅ 12 tests written, all passing

  📊 Synthesis: Authentication system complete.
     - 5 API endpoints
     - JWT + refresh token support
     - 2 security improvements applied
     - 12 tests (100% passing)
```

---

## Next Steps

- [Configuration](./configuration.md) — Full agent configuration reference
- [Skills System](./skills.md) — Extend agents with skills
- [API Reference](./api-reference.md) — Spawn, task, and scratchpad API docs
- [Tutorials](./tutorials.md) — Hands-on multi-agent tutorials