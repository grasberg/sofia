# 🧩 Skills System

Extend Sofia's capabilities with community and custom skills.

---

## What Are Skills?

Skills are **plugins for Sofia** that add new tools, knowledge, and behaviors. Think of them as packages that extend what Sofia can do — from querying databases to generating diagrams to managing cloud resources.

```
┌─────────────────────────────────────┐
│           Sofia Core                │
│   ┌───────────────────────────┐     │
│   │      Built-in Tools       │     │
│   │  web · exec · files · ... │     │
│   └───────────────────────────┘     │
│   ┌───────────────────────────┐     │
│   │      Skills Layer         │     │
│   │  ┌─────┐ ┌─────┐ ┌────┐  │     │
│   │  │ DB  │ │ K8s │ │AWS │  │     │
│   │  └─────┘ └─────┘ └────┘  │     │
│   └───────────────────────────┘     │
└─────────────────────────────────────┘
```

---

## Browsing & Discovering Skills

### Using the `find_skills` Tool

Search the ClawHub registry for available skills:

```
find_skills(query: "database management", limit: 5)
```

Parameters:

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `query` | string | required | Search query describing desired capability |
| `limit` | int | `5` | Max results to return (1-20) |

Returns: skill slugs, descriptions, versions, and relevance scores.

### ClawHub Registry

The default skill registry is [ClawHub](https://clawhub.ai). Browse it directly to discover skills, or use `find_skills` from within Sofia.

---

## Installing Skills

### Via `install_skill`

```
install_skill(slug: "database-manager")
```

This will:
1. Download the skill from ClawHub
2. Extract it to `~/.sofia/workspace-{agent}/skills/{skill-name}/`
3. Register the skill with the agent
4. Make the skill's tools available immediately

### Manual Installation

1. Download or create the skill directory
2. Place it in your workspace: `~/.sofia/workspace/skills/{skill-name}/`
3. Ensure it contains a `SKILL.md` file
4. Restart the gateway or the skill will be picked up on next tool iteration

---

## Skill Structure

Every skill follows this directory structure:

```
skills/
└── my-skill/
    ├── SKILL.md          # Required: Skill manifest and documentation
    ├── README.md          # Optional: Extended documentation
    ├── templates/         # Optional: Go templates for tool output
    │   └── query.go.tmpl
    ├── scripts/           # Optional: Shell scripts
    │   └── setup.sh
    └── config.json        # Optional: Skill-specific configuration
```

---

## SKILL.md Format

The `SKILL.md` file is the heart of every skill. It defines what the skill does and how the agent should use it.

```markdown
---
name: database-manager
version: 1.2.0
description: Query and manage SQL and NoSQL databases
author: community
tags: [database, sql, postgres, mysql, mongodb]
tools:
  - name: db_query
    description: Execute a SQL query against a configured database
    parameters:
      query:
        type: string
        required: true
        description: SQL query to execute
      database:
        type: string
        required: false
        description: Database name override
  - name: db_list_tables
    description: List all tables in the configured database
    parameters: {}
---

# Database Manager Skill

This skill provides database query and management capabilities.

## Usage

Ask Sofia to:
- "Query the users table for active accounts"
- "List all tables in the production database"
- "Run a migration on the analytics schema"

## Configuration

Set in `~/.sofia/config.json` under `tools.database_manager`:

\`\`\`json
{
  "tools": {
    "database_manager": {
      "connection_string": "postgres://user:pass@localhost:5432/mydb",
      "max_rows": 1000,
      "read_only": true
    }
  }
}
\`\`\`
```

### SKILL.md Frontmatter Fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | ✅ | Unique skill identifier (kebab-case) |
| `version` | string | ✅ | Semantic version |
| `description` | string | ✅ | One-line description |
| `author` | string | ❌ | Author name or handle |
| `tags` | string[] | ❌ | Search tags |
| `tools` | array | ❌ | Tool definitions (see below) |

### Tool Definition Fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | ✅ | Tool name (snake_case) |
| `description` | string | ✅ | What the tool does |
| `parameters` | object | ❌ | Parameter definitions |

### Parameter Definition Fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `type` | string | ✅ | Parameter type: `string`, `number`, `boolean`, `array` |
| `required` | bool | ✅ | Whether the parameter is required |
| `description` | string | ❌ | Parameter description |
| `default` | any | ❌ | Default value |

---

## Skill Configuration

Skills are configured in `~/.sofia/config.json` under `tools.skills`:

```json
{
  "tools": {
    "skills": {
      "registries": {
        "clawhub": {
          "enabled": true,
          "base_url": "https://clawhub.ai",
          "auth_token": "",
          "search_path": "",
          "skills_path": "",
          "download_path": "",
          "timeout": 0,
          "max_zip_size": 0,
          "max_response_size": 0
        }
      },
      "max_concurrent": 5
    }
  }
}
```

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `registries.clawhub.enabled` | bool | `true` | Enable ClawHub registry |
| `registries.clawhub.base_url` | string | `"https://clawhub.ai"` | Registry URL |
| `registries.clawhub.auth_token` | string | `""` | Authentication token for private registries |
| `max_concurrent` | int | `5` | Max concurrent skill operations |

---

## Built-in vs Community Skills

### Built-in Tools (Always Available)

These are core Sofia tools, not skills:

| Category | Tools |
|----------|-------|
| **Web** | `web_search`, `web_fetch`, `web_browse` |
| **Files** | `read_file`, `write_file`, `edit_file`, `list_dir` |
| **Execution** | `exec`, `screenshot` |
| **Planning** | `plan`, `task`, `manage_goals`, `manage_triggers` |
| **Agents** | `spawn`, `dynamic_tool` |
| **Memory** | `scratchpad` |
| **Documents** | `create_document` |
| **Google** | `google_cli` (Gmail, Drive, Calendar) |
| **GitHub** | `github_cli` |
| **Scheduling** | `cron` |
| **Skills** | `find_skills`, `install_skill` |
| **Bitcoin** | `bitcoin` |
| **Hosting** | `cpanel`, `domain_name`, `vercel` |

### Community Skills (Install from ClawHub)

Skills extend beyond built-in tools. Examples:
- Database connectors (PostgreSQL, MySQL, MongoDB)
- Cloud providers (AWS, GCP, Azure)
- Monitoring (Prometheus, Grafana)
- Communication (Slack, Teams)
- And more...

---

## Creating a Custom Skill

### Step 1: Create the Skill Directory

```bash
mkdir -p ~/.sofia/workspace/skills/my-custom-skill
cd ~/.sofia/workspace/skills/my-custom-skill
```

### Step 2: Write SKILL.md

```markdown
---
name: my-custom-skill
version: 0.1.0
description: Does something custom and awesome
author: your-name
tags: [custom, example]
tools:
  - name: custom_greet
    description: Generate a custom greeting
    parameters:
      name:
        type: string
        required: true
        description: Name to greet
      style:
        type: string
        required: false
        description: Greeting style (formal, casual, fun)
---

# My Custom Skill

Generates custom greetings in various styles.

## Usage

Ask Sofia to "greet Alice formally" or "say hi to Bob in a fun way".
```

### Step 3: Add Templates (Optional)

Create Go templates for structured tool output:

```
templates/
└── greet.go.tmpl
```

```go
{{- if eq .style "formal" -}}
Good day, {{.name}}. I hope this message finds you well.
{{- else if eq .style "fun" -}}
Hey {{.name}}! 🎉 What's popping?!
{{- else -}}
Hey {{.name}}! 👋
{{- end -}}
```

### Step 4: Add Scripts (Optional)

```
scripts/
└── setup.sh
```

```bash
#!/bin/bash
# Called when the skill is installed
echo "Setting up my-custom-skill..."
```

### Step 5: Test Your Skill

Restart the gateway and test:

```bash
sofia gateway
# Then in a conversation:
# "Use my-custom-skill to greet Alice formally"
```

### Step 6: Publish to ClawHub

Package your skill and submit to the ClawHub registry:

```bash
# Package
cd ~/.sofia/workspace/skills/my-custom-skill
tar -czf my-custom-skill.tar.gz .

# Upload via ClawHub web interface or API
```

---

## Skill Security Considerations

### Trust Model

- **Built-in tools** are vetted and safe
- **Community skills** are reviewed but use at your own risk
- **Custom skills** are your responsibility

### Best Practices

1. **Review SKILL.md** before installing — check what tools it adds
2. **Check permissions** — skills may request access to files, network, or system commands
3. **Use `restrict_to_workspace`** — set `"restrict_to_workspace": true` in agent defaults to sandbox skill file access
4. **Audit regularly** — review installed skills in your workspace
5. **Keep updated** — skills may receive security patches

### Sandboxed Execution

For untrusted skills, enable sandboxed execution:

```json
{
  "guardrails": {
    "sandboxed_exec": {
      "enabled": true,
      "docker_image": "sofia-sandbox:latest"
    }
  }
}
```

---

## SKILL.md Template

Copy this template to start a new skill:

```markdown
---
name: {{SKILL_NAME}}
version: 0.1.0
description: {{ONE_LINE_DESCRIPTION}}
author: {{YOUR_NAME}}
tags: [{{TAG1}}, {{TAG2}}]
tools:
  - name: {{tool_name}}
    description: {{What this tool does}}
    parameters:
      {{param_name}}:
        type: {{string|number|boolean|array}}
        required: {{true|false}}
        description: {{What this parameter does}}
---

# {{SKILL_NAME}}

{{Detailed description of what the skill does and how to use it.}}

## Usage

{{Example prompts that trigger the skill.}}

## Configuration

{{Any configuration needed in config.json.}}

## Examples

{{Example inputs and outputs.}}
```

---

## Next Steps

- [Multi-Agent Orchestration](./multi-agent.md) — Coordinate agents that use skills
- [API Reference](./api-reference.md) — Full tool reference including skill tools
- [Tutorials](./tutorials.md) — Step-by-step skill creation tutorial