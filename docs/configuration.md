# ⚙️ Configuration Reference

Complete reference for Sofia's configuration file at `~/.sofia/config.json`.

---

## Overview

Sofia uses a single JSON configuration file. All settings can be managed through:
- Direct editing of `~/.sofia/config.json`
- The `sofia onboard` wizard (initial setup)
- Environment variables (prefixed with `SOFIA_`)

---

## Configuration Sections

| Section | Purpose |
|---------|---------|
| [`session`](#session) | Session and DM behavior |
| [`agents`](#agents) | Agent defaults and templates |
| [`channels`](#channels) | Messaging integrations |
| [`gateway`](#gateway) | HTTP gateway settings |
| [`tools`](#tools) | Tool configurations |
| [`triggers`](#triggers) | Context-aware triggers |
| [`heartbeat`](#heartbeat) | Periodic check-in settings |
| [`autonomy`](#autonomy) | Autonomous behavior settings |
| [`evolution`](#evolution) | Agent self-improvement |
| [`webui`](#webui) | Web interface settings |
| [`tts`](#tts) | Text-to-speech |
| [`remote_access`](#remote_access) | Tailscale remote access |
| [`guardrails`](#guardrails) | Security and safety |
| [`user_name`](#user_name) | User identity |
| [`memory_db`](#memory_db) | Memory storage |

---

## `session`

Controls how Sofia handles conversation sessions and DMs.

```json
{
  "session": {
    "dm_scope": "per-channel-peer"
  }
}
```

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `dm_scope` | string | `"per-channel-peer"` | Session scoping: `"per-channel-peer"` (separate per contact), `"global"` (shared) |

---

## `agents`

Configures agent defaults and the list of available agents.

### Agent Defaults

```json
{
  "agents": {
    "defaults": {
      "workspace": "~/.sofia/workspace",
      "restrict_to_workspace": false,
      "provider": "",
      "model_name": "glm-5.1:cloud",
      "model_fallbacks": ["gemma4:31b-cloud", "MiniMax-M2.7"],
      "max_tokens": 32768,
      "max_tool_iterations": 50,
      "max_concurrent_subagents": 2,
      "evaluation_loop": {
        "enabled": false,
        "threshold": 0,
        "max_retries": 0
      },
      "doom_loop_detection": {
        "enabled": false,
        "repetition_threshold": 0
      },
      "auto_escalation": {
        "enabled": false,
        "smart_model_routing": false
      },
      "prompt_optimization": {
        "enabled": false,
        "score_threshold": 0,
        "min_traces": 0,
        "max_variants": 0,
        "trials_per_variant": 0
      },
      "summarization": {}
    }
  }
}
```

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `workspace` | string | `"~/.sofia/workspace"` | Base path for agent workspaces |
| `restrict_to_workspace` | bool | `false` | If true, agents can only write to their workspace |
| `provider` | string | `""` | LLM provider override (empty = auto-detect) |
| `model_name` | string | `"glm-5.1:cloud"` | Default model for all agents |
| `model_fallbacks` | string[] | `[]` | Fallback models if primary fails |
| `max_tokens` | int | `32768` | Maximum response tokens |
| `max_tool_iterations` | int | `50` | Max tool calls per conversation turn |
| `max_concurrent_subagents` | int | `2` | Max parallel subagent spawns |

### Evaluation Loop

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `enabled` | bool | `false` | Enable self-evaluation of responses |
| `threshold` | float | `0` | Quality threshold (0-1) to accept response |
| `max_retries` | int | `0` | Max re-evaluation attempts |

### Doom Loop Detection

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `enabled` | bool | `false` | Detect repetitive tool calls |
| `repetition_threshold` | int | `0` | Number of repeated calls before breaking |

### Auto Escalation

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `enabled` | bool | `false` | Auto-escalate to stronger model |
| `smart_model_routing` | bool | `false` | Route tasks to optimal model |

### Prompt Optimization

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `enabled` | bool | `false` | Enable automatic prompt optimization |
| `score_threshold` | float | `0` | Minimum quality score to keep |
| `min_traces` | int | `0` | Minimum traces before optimizing |
| `max_variants` | int | `0` | Max prompt variants to test |
| `trials_per_variant` | int | `0` | Trials per variant |

### Agent List

Each agent in the list inherits defaults and can override them:

```json
{
  "list": [
    {
      "id": "main",
      "default": true,
      "name": "Sofia",
      "subagents": { "allow_agents": ["*"] },
      "summarization": {}
    },
    {
      "id": "security-auditor",
      "name": "Security Auditor",
      "template": "security-auditor",
      "subagents": { "allow_agents": ["*"] },
      "summarization": {}
    }
  ]
}
```

| Field | Type | Description |
|-------|------|-------------|
| `id` | string | Unique agent identifier (used for workspace naming) |
| `name` | string | Display name |
| `default` | bool | Whether this is the default agent (only one should be true) |
| `template` | string | Agent template to use (see [Multi-Agent](./multi-agent.md)) |
| `subagents.allow_agents` | string[] | Which agents this one can spawn (`["*"]` = all) |
| `summarization` | object | Conversation summarization settings |

---

## `channels`

Configure messaging platform integrations.

### Telegram

```json
{
  "channels": {
    "telegram": {
      "enabled": true,
      "token": "123456789:ABCdefGHIjklMNOpqrsTUVwxyz",
      "proxy": "",
      "allow_from": ["397244034"]
    }
  }
}
```

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `enabled` | bool | `false` | Enable Telegram channel |
| `token` | string | `""` | Bot token from @BotFather |
| `proxy` | string | `""` | SOCKS5 proxy URL (e.g., `socks5://127.0.0.1:1080`) |
| `allow_from` | string[] | `[]` | Allowed Telegram user IDs (empty = allow all) |

### Discord

```json
{
  "channels": {
    "discord": {
      "enabled": true,
      "token": "your-bot-token",
      "allow_from": ["123456789"],
      "mention_only": true
    }
  }
}
```

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `enabled` | bool | `false` | Enable Discord channel |
| `token` | string | `""` | Discord bot token |
| `allow_from` | string[] | `[]` | Allowed Discord user IDs |
| `mention_only` | bool | `false` | Only respond when bot is @mentioned |

### Email

```json
{
  "channels": {
    "email": {
      "enabled": true,
      "use_gmail_api": true,
      "imap_server": "imap.gmail.com",
      "smtp_server": "smtp.gmail.com",
      "username": "you@gmail.com",
      "password": "app-password",
      "poll_interval_sec": 60
    }
  }
}
```

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `enabled` | bool | `false` | Enable email channel |
| `use_gmail_api` | bool | `false` | Use Gmail API instead of IMAP |
| `imap_server` | string | `""` | IMAP server address |
| `smtp_server` | string | `""` | SMTP server address |
| `username` | string | `""` | Email account username |
| `password` | string | `""` | Email account password or app password |
| `poll_interval_sec` | int | `0` | How often to check for new emails |

---

## `gateway`

```json
{
  "gateway": {
    "host": "127.0.0.1",
    "port": 18790
  }
}
```

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `host` | string | `"127.0.0.1"` | Gateway listen address |
| `port` | int | `18790` | Gateway listen port |

---

## `tools`

### Web Search

```json
{
  "tools": {
    "web": {
      "brave": {
        "enabled": false,
        "api_key": "",
        "max_results": 5
      },
      "tavily": {
        "enabled": false,
        "api_key": "",
        "base_url": "",
        "max_results": 5
      },
      "duckduckgo": {
        "enabled": true,
        "max_results": 5
      },
      "perplexity": {
        "enabled": false,
        "api_key": "",
        "max_results": 5
      },
      "browser": {
        "headless": true,
        "timeout_seconds": 30,
        "browser_type": "chromium"
      }
    }
  }
}
```

### Google Integration

```json
{
  "tools": {
    "google": {
      "enabled": true,
      "binary_path": "gog",
      "timeout_seconds": 90,
      "allowed_commands": ["gmail", "drive", "calendar"]
    }
  }
}
```

### GitHub Integration

```json
{
  "tools": {
    "github": {
      "enabled": true,
      "binary_path": "gh",
      "timeout_seconds": 60,
      "allowed_commands": ["repo", "issue", "pr", "release"]
    }
  }
}
```

### Command Execution

```json
{
  "tools": {
    "exec": {
      "enable_deny_patterns": true,
      "custom_deny_patterns": null,
      "confirm_patterns": null
    }
  }
}
```

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `enable_deny_patterns` | bool | `true` | Block dangerous commands |
| `custom_deny_patterns` | string[] | `null` | Additional patterns to block |
| `confirm_patterns` | string[] | `null` | Patterns requiring user confirmation |

### Skills Registry

```json
{
  "tools": {
    "skills": {
      "registries": {
        "clawhub": {
          "enabled": true,
          "base_url": "https://clawhub.ai",
          "auth_token": ""
        }
      }
    }
  }
}
```

### Other Tools

| Tool | Key | Description |
|------|-----|-------------|
| Cron | `tools.cron` | Scheduled task execution timeout |
| Brave Search | `tools.brave_search` | Brave Search API |
| Porkbun | `tools.porkbun` | Domain management |
| cPanel | `tools.cpanel` | Hosting control panel |
| Bitcoin | `tools.bitcoin` | Bitcoin wallet operations |
| Vercel | `tools.vercel` | Vercel deployment |

---

## `triggers`

Context-aware triggers that fire actions when conditions are met.

```json
{
  "triggers": {}
}
```

Triggers are managed via the `manage_triggers` tool at runtime. See [API Reference](./api-reference.md) for trigger management.

---

## `heartbeat`

```json
{
  "heartbeat": {
    "enabled": true,
    "interval": 30,
    "model": "",
    "active_hours": "",
    "active_days": []
  }
}
```

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `enabled` | bool | `true` | Enable periodic heartbeat |
| `interval` | int | `30` | Seconds between heartbeats |
| `model` | string | `""` | Model override for heartbeat tasks |
| `active_hours` | string | `""` | Active hours (e.g., `"09:00-22:00"`) |
| `active_days` | string[] | `[]` | Active days (e.g., `["mon","tue","wed","thu","fri"]`) |

---

## `autonomy`

```json
{
  "autonomy": {
    "enabled": true,
    "suggestions": true,
    "goals": true,
    "research": true,
    "context_triggers": true,
    "interval_minutes": 60,
    "max_cost_per_day": 0
  }
}
```

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `enabled` | bool | `true` | Enable autonomous behavior |
| `suggestions` | bool | `true` | Allow Sofia to suggest actions |
| `goals` | bool | `true` | Enable goal pursuit |
| `research` | bool | `true` | Allow autonomous research |
| `context_triggers` | bool | `true` | Enable context-aware triggers |
| `interval_minutes` | int | `60` | How often to check for autonomous tasks |
| `max_cost_per_day` | float | `0` | Maximum API cost per day (0 = unlimited) |

---

## `evolution`

```json
{
  "evolution": {
    "enabled": false,
    "model": "gemma4:31b-cloud",
    "interval_minutes": 30,
    "max_cost_per_day": 5,
    "daily_summary": true,
    "daily_summary_time": "08:00",
    "daily_summary_channel": "",
    "daily_summary_chat_id": "",
    "retirement_threshold": 0.3,
    "retirement_min_tasks": 5,
    "retirement_inactive_days": 7,
    "self_modify_enabled": true,
    "max_agents": 20,
    "require_approval": false,
    "memory_consolidation": false,
    "consolidation_interval_h": 6,
    "skill_auto_improve": false
  }
}
```

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `enabled` | bool | `false` | Enable agent evolution system |
| `model` | string | `"gemma4:31b-cloud"` | Model for evolution decisions |
| `interval_minutes` | int | `30` | Evolution check interval |
| `max_cost_per_day` | float | `5` | Max API cost for evolution |
| `daily_summary` | bool | `true` | Send daily activity summaries |
| `daily_summary_time` | string | `"08:00"` | Time for daily summary |
| `daily_summary_channel` | string | `""` | Channel for summaries |
| `self_modify_enabled` | bool | `true` | Allow agents to modify their own prompts |
| `max_agents` | int | `20` | Maximum number of agents |
| `require_approval` | bool | `false` | Require human approval for changes |
| `retirement_threshold` | float | `0.3` | Usage ratio below which agents are retired |
| `retirement_min_tasks` | int | `5` | Minimum tasks before retirement evaluation |
| `retirement_inactive_days` | int | `7` | Days of inactivity before retirement |
| `memory_consolidation` | bool | `false` | Enable memory consolidation |
| `consolidation_interval_h` | int | `6` | Hours between consolidation runs |
| `skill_auto_improve` | bool | `false` | Auto-improve skills based on usage |

---

## `webui`

```json
{
  "webui": {
    "enabled": true,
    "host": "0.0.0.0",
    "port": 18795
  }
}
```

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `enabled` | bool | `true` | Enable the web UI |
| `host` | string | `"0.0.0.0"` | Web UI listen address |
| `port` | int | `18795` | Web UI port |

---

## `tts`

```json
{
  "tts": {
    "enabled": false,
    "provider": ""
  }
}
```

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `enabled` | bool | `false` | Enable text-to-speech |
| `provider` | string | `""` | TTS provider (e.g., `"openai"`, `"elevenlabs"`) |

---

## `remote_access`

```json
{
  "remote_access": {
    "enabled": false
  }
}
```

Enable via `sofia remote enable`. Uses Tailscale for secure remote access to the web UI.

---

## `guardrails`

Security and safety configuration.

```json
{
  "guardrails": {
    "input_validation": {
      "enabled": false,
      "max_message_length": 0,
      "deny_patterns": []
    },
    "output_filtering": {
      "enabled": false,
      "redact_patterns": [],
      "action": ""
    },
    "rate_limiting": {
      "enabled": false,
      "max_rpm": 0,
      "max_tokens_per_hour": 0
    },
    "sandboxed_exec": {
      "enabled": false,
      "docker_image": ""
    },
    "prompt_injection": {
      "enabled": false,
      "action": "block",
      "system_suffix": ""
    },
    "pii_detection": {
      "enabled": false
    }
  }
}
```

### Input Validation

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `enabled` | bool | `false` | Enable input validation |
| `max_message_length` | int | `0` | Max characters per message (0 = unlimited) |
| `deny_patterns` | string[] | `[]` | Regex patterns to block in input |

### Output Filtering

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `enabled` | bool | `false` | Enable output filtering |
| `redact_patterns` | string[] | `[]` | Patterns to redact from output |
| `action` | string | `""` | Action on match: `"block"`, `"redact"`, `"warn"` |

### Rate Limiting

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `enabled` | bool | `false` | Enable rate limiting |
| `max_rpm` | int | `0` | Max requests per minute |
| `max_tokens_per_hour` | int | `0` | Max tokens per hour |

### Sandboxed Execution

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `enabled` | bool | `false` | Run commands in Docker sandbox |
| `docker_image` | string | `""` | Docker image for sandbox |

### Prompt Injection Protection

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `enabled` | bool | `false` | Enable injection detection |
| `action` | string | `"block"` | Action: `"block"`, `"warn"`, `"log"` |
| `system_suffix` | string | `""` | Additional system prompt for protection |

### PII Detection

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `enabled` | bool | `false` | Enable PII detection in messages |

---

## `user_name`

```json
{
  "user_name": "YourName"
}
```

Your display name, used by Sofia to personalize interactions.

---

## `memory_db`

```json
{
  "memory_db": ""
}
```

Path to the SQLite memory database. Default: `~/.sofia/memory.db`.

---

## Example Configurations

### Minimal (Local Chat Only)

```json
{
  "session": { "dm_scope": "per-channel-peer" },
  "agents": {
    "defaults": {
      "model_name": "gpt-4o",
      "max_tokens": 16384
    },
    "list": [
      { "id": "main", "default": true, "name": "Sofia" }
    ]
  },
  "channels": {},
  "gateway": { "host": "127.0.0.1", "port": 18790 },
  "tools": { "web": { "duckduckgo": { "enabled": true } } },
  "webui": { "enabled": true, "host": "127.0.0.1", "port": 18795 }
}
```

### Full Production Setup

```json
{
  "session": { "dm_scope": "per-channel-peer" },
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
      { "id": "orchestrator", "name": "Orchestrator", "template": "orchestrator" },
      { "id": "frontend-specialist", "name": "Frontend Specialist", "template": "frontend-specialist" },
      { "id": "backend-specialist", "name": "Backend Specialist", "template": "backend-specialist" },
      { "id": "test-engineer", "name": "Test Engineer", "template": "test-engineer" },
      { "id": "security-auditor", "name": "Security Auditor", "template": "security-auditor" }
    ]
  },
  "channels": {
    "telegram": { "enabled": true, "token": "YOUR_TOKEN", "allow_from": ["YOUR_ID"] },
    "discord": { "enabled": true, "token": "YOUR_TOKEN", "allow_from": [], "mention_only": true }
  },
  "gateway": { "host": "127.0.0.1", "port": 18790 },
  "tools": {
    "web": { "duckduckgo": { "enabled": true, "max_results": 5 } },
    "google": { "enabled": true, "binary_path": "gog", "allowed_commands": ["gmail", "drive", "calendar"] },
    "github": { "enabled": true, "binary_path": "gh", "allowed_commands": ["repo", "issue", "pr"] },
    "exec": { "enable_deny_patterns": true }
  },
  "heartbeat": { "enabled": true, "interval": 30 },
  "autonomy": { "enabled": true, "suggestions": true, "goals": true, "interval_minutes": 60 },
  "webui": { "enabled": true, "host": "0.0.0.0", "port": 18795 },
  "guardrails": {
    "input_validation": { "enabled": true, "max_message_length": 10000 },
    "rate_limiting": { "enabled": true, "max_rpm": 30 }
  }
}
```

---

## Environment Variables

Sofia supports environment variables with the `SOFIA_` prefix:

| Variable | Description |
|----------|-------------|
| `SOFIA_CONFIG` | Path to config file (default: `~/.sofia/config.json`) |
| `SOFIA_WORKSPACE` | Base workspace path |
| `SOFIA_MODEL` | Override default model |
| `SOFIA_PORT` | Override gateway port |
| `SOFIA_DEBUG` | Enable debug logging (`1` or `true`) |

---

## Next Steps

- [Skills System](./skills.md) — Extend Sofia with plugins
- [Multi-Agent Orchestration](./multi-agent.md) — Configure agent teams
- [API Reference](./api-reference.md) — Full tool and command reference