# 📡 API Reference

Complete reference for Sofia's CLI, HTTP API, MCP server, and built-in tools.

---

## CLI Reference

### Global Flags

| Flag | Description |
|------|-------------|
| `-h, --help` | Show help for any command |
| `-d, --debug` | Enable debug logging |

### `sofia gateway`

Start the Sofia gateway server.

```bash
sofia gateway [flags]
# Aliases: g

Flags:
  -d, --debug   Enable debug logging
```

The gateway is the main process that handles all channels and agent interactions.

### `sofia agent`

Interact with an agent directly (non-gateway mode).

```bash
sofia agent [flags]

Flags:
  -d, --debug              Enable debug logging
  -m, --message string     Send a single message (non-interactive mode)
      --model string       Model to use (overrides config)
  -s, --session string     Session key (default: "cli:default")
```

Examples:

```bash
# Interactive mode
sofia agent

# One-shot message
sofia agent -m "What's the weather in Stockholm?"

# Use specific model
sofia agent --model gpt-4o -m "Explain quantum computing"

# Named session
sofia agent -s "project-alpha" -m "Review the code"
```

### `sofia cron`

Manage scheduled tasks.

```bash
sofia cron [command]

Subcommands:
  add       Add a new scheduled job
  disable   Disable a job
  enable    Enable a job
  list      List all scheduled jobs
  remove    Remove a job by ID

Flags:
  -h, --help   help for cron
```

Examples:

```bash
# List all cron jobs
sofia cron list

# Add a daily summary job (via agent tool)
# Use the cron tool within a conversation instead
```

### `sofia daemon`

Manage Sofia as a background service.

```bash
sofia daemon [command]

Subcommands:
  install     Install Sofia gateway as a background service
  status      Show Sofia gateway service status
  uninstall   Remove Sofia gateway background service
```

Examples:

```bash
# Install as daemon (launchd on macOS, systemd on Linux)
sofia daemon install

# Check status
sofia daemon status

# Remove daemon
sofia daemon uninstall
```

### `sofia data`

Export and import conversation data.

```bash
sofia data [command]

Subcommands:
  export    Export conversation sessions to JSON
  import    Import conversation sessions from JSON
```

### `sofia doctor`

Check Sofia's configuration and environment.

```bash
sofia doctor
```

Validates: config file, channel connections, model access, workspace permissions, tool availability.

### `sofia mcp-server`

Expose Sofia as a Model Context Protocol (MCP) server.

```bash
sofia mcp-server [flags]

Flags:
  -a, --addr string        Listen address for SSE transport (default ":9090")
  -d, --debug              Enable debug logging
  -t, --transport string   Transport mode: stdio or sse (default "stdio")
```

### `sofia onboard`

Initialize Sofia configuration and workspace.

```bash
sofia onboard
# Aliases: o
```

Interactive wizard that creates `~/.sofia/config.json` and workspace directories.

### `sofia pairing`

Manage DM pairing for unknown senders.

```bash
sofia pairing [command]

Subcommands:
  approve    Approve a pairing request by code
  list       List pending pairing requests
```

### `sofia remote`

Manage remote access via Tailscale.

```bash
sofia remote [command]

Subcommands:
  disable    Disable remote access
  enable     Enable remote access to Sofia's web UI
  status     Show remote access status
```

### `sofia version`

Show version information.

```bash
sofia version
# Output: sofia v0.0.145 (git: d981c0b2)
#         Build: 2026-04-11T12:46:37+0200
#         Go: go1.26.0
```

---

## Gateway HTTP API

The gateway exposes an HTTP API on `http://{host}:{port}` (default: `127.0.0.1:18790`).

### Endpoints

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/health` | Health check |
| `POST` | `/api/chat` | Send a chat message |
| `GET` | `/api/sessions` | List conversation sessions |
| `GET` | `/api/sessions/{id}` | Get session details |
| `POST` | `/api/agents/{id}/chat` | Chat with a specific agent |
| `GET` | `/api/agents` | List available agents |
| `GET` | `/api/tools` | List available tools |
| `POST` | `/api/tools/{name}` | Execute a tool |

### Chat Message Format

```json
POST /api/chat
{
  "message": "Hello, Sofia!",
  "session": "cli:default",
  "agent_id": "main",
  "model": ""
}
```

Response:

```json
{
  "response": "Hello! How can I help you today?",
  "session": "cli:default",
  "agent_id": "main",
  "tool_calls": 0,
  "tokens_used": 42
}
```

---

## MCP Server Protocol

Sofia can operate as an MCP (Model Context Protocol) server, allowing other AI tools to use Sofia's capabilities.

### Transport Modes

| Mode | Use Case | Command |
|------|----------|---------|
| `stdio` | Subprocess communication (default) | `sofia mcp-server -t stdio` |
| `sse` | Network communication | `sofia mcp-server -t sse -a :9090` |

### Connecting from Claude Desktop

Add to `claude_desktop_config.json`:

```json
{
  "mcpServers": {
    "sofia": {
      "command": "sofia",
      "args": ["mcp-server", "-t", "stdio"]
    }
  }
}
```

### Connecting from Cursor

Configure in Cursor's MCP settings:

```json
{
  "mcpServers": {
    "sofia": {
      "command": "sofia",
      "args": ["mcp-server", "-t", "stdio"]
    }
  }
}
```

### SSE Mode (Network)

For network-based clients:

```bash
sofia mcp-server -t sse -a :9090
```

Then connect your client to `http://localhost:9090/sse`.

---

## Channel Integration APIs

### Telegram Setup

1. Create a bot via [@BotFather](https://t.me/BotFather)
2. Get the bot token
3. Find your Telegram user ID (message [@userinfobot](https://t.me/userinfobot))
4. Configure in `config.json`:

```json
{
  "channels": {
    "telegram": {
      "enabled": true,
      "token": "BOT_TOKEN_HERE",
      "allow_from": ["YOUR_USER_ID"]
    }
  }
}
```

### Discord Setup

1. Create an application at [Discord Developer Portal](https://discord.com/developers/applications)
2. Create a bot and get the token
3. Enable Message Content Intent in the bot settings
4. Invite the bot to your server with appropriate permissions
5. Configure:

```json
{
  "channels": {
    "discord": {
      "enabled": true,
      "token": "BOT_TOKEN_HERE",
      "allow_from": [],
      "mention_only": true
    }
  }
}
```

### Email Setup

For Gmail:

```json
{
  "channels": {
    "email": {
      "enabled": true,
      "use_gmail_api": true,
      "username": "you@gmail.com",
      "password": "app-password",
      "poll_interval_sec": 60
    }
  }
}
```

For IMAP/SMTP:

```json
{
  "channels": {
    "email": {
      "enabled": true,
      "use_gmail_api": false,
      "imap_server": "imap.example.com",
      "smtp_server": "smtp.example.com",
      "username": "you@example.com",
      "password": "your-password",
      "poll_interval_sec": 60
    }
  }
}
```

---

## Built-in Tool Reference

### Web Tools

#### `web_search`

Search the web for information.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `query` | string | ✅ | Search query |

```
web_search(query: "Go 1.22 release notes")
```

#### `web_fetch`

Fetch and parse a URL.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `url` | string | ✅ | URL to fetch |
| `format` | string | ❌ | Output format: `"text"`, `"html"`, `"markdown"` |

#### `web_browse`

Browse websites with a real browser (Playwright/Chromium).

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `url` | string | ✅ | Starting URL |
| `actions` | array | ❌ | Sequence of browser actions |
| `headless` | bool | ❌ | Run headless (default: true) |

Actions: `navigate`, `click`, `fill`, `select`, `wait`, `screenshot`, `get_text`, `get_html`, `scroll`, `hover`, `press`, `evaluate`

### File Tools

#### `read_file`

Read file contents.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `path` | string | ✅ | File path |
| `offset` | int | ❌ | Start line (0-based) |
| `limit` | int | ❌ | Max lines to read |

#### `write_file`

Write content to a file.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `path` | string | ✅ | File path |
| `content` | string | ✅ | Content to write |

#### `edit_file`

Make targeted edits to a file.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `path` | string | ✅ | File path |
| `edits` | array | ✅ | List of edit operations |

#### `list_dir`

List directory contents.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `path` | string | ✅ | Directory path |

### Execution Tools

#### `exec`

Execute shell commands.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `command` | string | ✅ | Shell command to execute |
| `working_dir` | string | ❌ | Working directory |

#### `screenshot`

Take a screenshot of the desktop.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `filename` | string | ❌ | Output filename |

### Planning & Task Tools

#### `plan`

Create and manage structured plans.

| Operation | Description |
|-----------|-------------|
| `create` | Create a new plan with steps |
| `update_step` | Update a step's status |
| `get_status` | View plan progress |
| `replan` | Insert/remove/reorder steps |
| `create_subplan` | Create hierarchical sub-plans |
| `evaluate` | Cost/benefit analysis |

#### `task`

Track tasks within a session.

| Action | Description |
|--------|-------------|
| `create` | Add a task |
| `list` | Show all tasks |
| `update` | Change status/description |
| `delete` | Remove a task |

#### `manage_goals`

Manage long-term agent goals.

| Action | Description |
|--------|-------------|
| `add` | Create a new goal |
| `update_status` | Update goal status |
| `list` | View active goals |

#### `manage_triggers`

Manage context-aware triggers.

| Action | Description |
|--------|-------------|
| `add` | Create a trigger |
| `toggle` | Enable/disable a trigger |
| `list` | View active triggers |

### Agent Tools

#### `spawn`

Spawn a subagent for a task.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `task` | string | ✅ | Task description |
| `agent_id` | string | ❌ | Target agent ID |
| `label` | string | ❌ | Short label |
| `skills` | string[] | ❌ | Skills to equip |

#### `dynamic_tool`

Create, list, or remove tools at runtime.

| Operation | Description |
|-----------|-------------|
| `create` | Create a new tool |
| `list` | List dynamic tools |
| `remove` | Remove a tool |
| `get` | Get tool details |

### Memory Tools

#### `scratchpad`

Shared key-value storage between agents.

| Operation | Description |
|-----------|-------------|
| `write` | Store a value |
| `read` | Retrieve a value |
| `list` | List all keys |

### Document Tools

#### `create_document`

Create formatted documents.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `format` | string | ✅ | `"html"`, `"csv"`, or `"md"` |
| `content` | string | ✅ | Document content |
| `filename` | string | ❌ | Output filename |
| `title` | string | ❌ | Document title |

### Integration Tools

#### `google_cli`

Run Google service commands (Gmail, Drive, Calendar).

```bash
gmail search "is:unread" --max 10
gmail get <msgId>
gmail send --to user@example.com --subject "Hello" --body "Hi there"
drive list
calendar events list
```

#### `github_cli`

Run GitHub CLI commands.

#### `cron`

Schedule reminders and tasks.

| Action | Description |
|--------|-------------|
| `add` | Schedule a new job |
| `list` | List all jobs |
| `remove` | Remove a job |
| `enable` | Enable a job |
| `disable` | Disable a job |

#### `find_skills`

Search for installable skills.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `query` | string | ✅ | Search query |
| `limit` | int | ❌ | Max results (1-20) |

#### `bitcoin`

Bitcoin wallet operations.

#### `cpanel`

cPanel hosting management.

#### `domain_name`

Domain name management.

#### `vercel`

Vercel deployment operations.

---

## Web UI

Sofia includes a built-in web interface accessible at `http://{host}:{port}` (default: `http://0.0.0.0:18795`).

Configuration:

```json
{
  "webui": {
    "enabled": true,
    "host": "0.0.0.0",
    "port": 18795
  }
}
```

---

## Data Export/Import

### Export

```bash
sofia data export --output sessions.json
```

Exports all conversation sessions to JSON format.

### Import

```bash
sofia data import --input sessions.json
```

Imports conversation sessions from a JSON file.

---

## Error Codes

| Code | Meaning | Resolution |
|------|---------|------------|
| `CONFIG_NOT_FOUND` | Config file missing | Run `sofia onboard` |
| `INVALID_CONFIG` | Config JSON malformed | Check JSON syntax |
| `CHANNEL_AUTH` | Channel authentication failed | Verify token/credentials |
| `MODEL_UNAVAILABLE` | LLM model not reachable | Check API key and endpoint |
| `WORKSPACE_ERROR` | Workspace access issue | Check file permissions |
| `TOOL_DENIED` | Tool blocked by safety guard | Review `exec.deny_patterns` |
| `RATE_LIMITED` | Rate limit exceeded | Wait or increase limits |
| `AGENT_NOT_FOUND` | Agent ID doesn't exist | Check agent config |
| `SKILL_NOT_FOUND` | Skill not installed | Use `find_skills` to search |

---

## Next Steps

- [Configuration](./configuration.md) — Configure all these features
- [Multi-Agent](./multi-agent.md) — Agent orchestration details
- [Tutorials](./tutorials.md) — Hands-on guides