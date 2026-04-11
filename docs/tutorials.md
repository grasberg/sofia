# 🎓 Tutorials

Step-by-step guides to get the most out of Sofia.

---

## Tutorial 1: Getting Started — From Zero to First Conversation

**Goal**: Install Sofia, connect Telegram, and have your first conversation.

### Step 1: Install Sofia

```bash
# macOS (Apple Silicon)
brew tap grasberg/sofia
brew install sofia

# Or with curl
curl -fsSL https://get.sofia.ai | bash
```

### Step 2: Initialize

```bash
sofia onboard
```

Follow the interactive wizard:
1. Enter your name
2. Select your LLM provider (OpenAI, Anthropic, etc.)
3. Enter your API key
4. Choose a default model

### Step 3: Connect Telegram

1. Open Telegram and message [@BotFather](https://t.me/BotFather)
2. Send `/newbot` and follow the prompts
3. Copy the bot token
4. Find your user ID by messaging [@userinfobot](https://t.me/userinfobot)
5. Edit `~/.sofia/config.json`:

```json
{
  "channels": {
    "telegram": {
      "enabled": true,
      "token": "123456789:ABCdefGHIjklMNOpqrsTUVwxyz",
      "allow_from": ["YOUR_USER_ID"]
    }
  }
}
```

### Step 4: Start the Gateway

```bash
sofia gateway
```

### Step 5: Chat!

Open Telegram, find your bot, and send a message:

```
You: Hello Sofia! What can you do?
Sofia: Hello! I'm Sofia, your personal AI assistant. I can help with:
- Writing and editing code
- Managing files and projects
- Searching the web
- Scheduling tasks
- Coordinating multiple AI agents
- And much more! What would you like to work on?
```

### Step 6: Verify Setup

```bash
sofia doctor
```

✅ You're now running Sofia with Telegram!

---

## Tutorial 2: Building a Dev Team

**Goal**: Set up a multi-agent development team for a software project.

### Step 1: Configure Your Team

Edit `~/.sofia/config.json` and add your agents:

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
      { "id": "security-auditor", "name": "Security", "template": "security-auditor" }
    ]
  }
}
```

### Step 2: Restart the Gateway

```bash
# Stop current gateway (Ctrl+C)
sofia gateway
```

### Step 3: Assign a Complex Task

In your channel:

```
You: I need to build a REST API for a task management app with user auth.
     Use the orchestrator to coordinate the team.

Sofia: I'll coordinate the team for this project. Let me break it down:

📋 Plan:
1. [Backend] Design API endpoints and auth flow
2. [Database] Design user and task schemas
3. [Backend] Implement routes and middleware
4. [Security] Review auth implementation
5. [Test Engineer] Write comprehensive tests

Starting execution...
```

### Step 4: Monitor Progress

The orchestrator will spawn agents, track progress, and synthesize results. Each agent works in its own workspace.

✅ Your dev team is operational!

---

## Tutorial 3: Creating a Custom Skill

**Goal**: Build and test a custom skill for Sofia.

### Step 1: Create Skill Directory

```bash
mkdir -p ~/.sofia/workspace/skills/stock-checker
cd ~/.sofia/workspace/skills/stock-checker
```

### Step 2: Write SKILL.md

Create `SKILL.md`:

```markdown
---
name: stock-checker
version: 1.0.0
description: Check stock prices and portfolio performance
author: your-name
tags: [finance, stocks, portfolio]
tools:
  - name: stock_price
    description: Get the current price of a stock
    parameters:
      symbol:
        type: string
        required: true
        description: Stock ticker symbol (e.g., AAPL, GOOGL)
  - name: portfolio_summary
    description: Get a summary of your stock portfolio
    parameters:
      symbols:
        type: array
        required: true
        description: List of stock symbols to check
---

# Stock Checker Skill

Check real-time stock prices and portfolio performance.

## Usage

Ask Sofia to:
- "What's the price of AAPL?"
- "Show me my portfolio summary for AAPL, GOOGL, MSFT"
- "How is TSLA doing today?"

## Configuration

Add to `~/.sofia/config.json`:

\`\`\`json
{
  "tools": {
    "stock_checker": {
      "api_key": "your-api-key",
      "base_url": "https://api.example.com"
    }
  }
}
\`\`\`
```

### Step 3: Add Templates (Optional)

```bash
mkdir -p templates
```

Create `templates/stock_price.go.tmpl`:

```go
📊 {{.symbol}} Stock Price
━━━━━━━━━━━━━━━━━━━━
Price: ${{.price}}
Change: {{if .positive}}📈{{else}}📉{{end}} {{.changePercent}}%
Volume: {{.volume}}
Updated: {{.timestamp}}
```

### Step 4: Test Your Skill

Restart the gateway and test:

```
You: Check the price of AAPL using stock-checker

Sofia: Let me check that for you using the stock-checker skill...
📊 AAPL Stock Price
━━━━━━━━━━━━━━━━━━━━
Price: $178.72
Change: 📈 +1.23%
Volume: 52,341,200
Updated: 2026-04-11 12:00:00 UTC
```

✅ Your custom skill is working!

---

## Tutorial 4: Setting Up Remote Access

**Goal**: Access Sofia's web UI remotely via Tailscale.

### Step 1: Install Tailscale

```bash
# macOS
brew install tailscale

# Linux
curl -fsSL https://tailscale.com/install.sh | sh
```

### Step 2: Enable Remote Access

```bash
sofia remote enable
```

This configures Tailscale to proxy the web UI.

### Step 3: Verify

```bash
sofia remote status
```

### Step 4: Access from Any Device

On any device connected to your Tailscale network:

```
https://your-machine-name.tailnet-name.ts.net:18795
```

✅ You can now access Sofia from anywhere!

---

## Tutorial 5: Automation with Cron

**Goal**: Schedule recurring tasks for daily summaries and code reviews.

### Step 1: Schedule a Daily Summary

In a conversation with Sofia:

```
You: Schedule a daily summary every morning at 8am

Sofia: I'll set up a daily summary cron job for you.

cron(
  action: "add",
  message: "Generate a daily summary of yesterday's activities and present it",
  cron_expr: "0 8 * * *",
  deliver: true
)
```

### Step 2: Schedule a Weekly Code Review

```
You: Every Friday at 5pm, review the code changes this week

Sofia: Setting up a weekly code review.

cron(
  action: "add",
  message: "Review all code changes from this week and provide a summary",
  cron_expr: "0 17 * * 5",
  deliver: true
)
```

### Step 3: List All Scheduled Jobs

```bash
sofia cron list
```

### Step 4: Manage Jobs

```bash
# Disable a job
sofia cron disable <job_id>

# Enable a job
sofia cron enable <job_id>

# Remove a job
sofia cron remove <job_id>
```

✅ Your automations are set up!

---

## Tutorial 6: Security Hardening

**Goal**: Configure guardrails for a production deployment.

### Step 1: Enable Input Validation

Edit `~/.sofia/config.json`:

```json
{
  "guardrails": {
    "input_validation": {
      "enabled": true,
      "max_message_length": 10000,
      "deny_patterns": [
        "ignore previous instructions",
        "system prompt",
        "you are now"
      ]
    }
  }
}
```

### Step 2: Enable Output Filtering

```json
{
  "guardrails": {
    "output_filtering": {
      "enabled": true,
      "redact_patterns": [
        "\\b\\d{3}-\\d{2}-\\d{4}\\b",
        "\\b[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\\.[A-Z|a-z]{2,}\\b"
      ],
      "action": "redact"
    }
  }
}
```

### Step 3: Enable Rate Limiting

```json
{
  "guardrails": {
    "rate_limiting": {
      "enabled": true,
      "max_rpm": 30,
      "max_tokens_per_hour": 100000
    }
  }
}
```

### Step 4: Enable PII Detection

```json
{
  "guardrails": {
    "pii_detection": {
      "enabled": true
    }
  }
}
```

### Step 5: Enable Sandboxed Execution

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

### Step 6: Restrict Command Execution

```json
{
  "tools": {
    "exec": {
      "enable_deny_patterns": true,
      "custom_deny_patterns": [
        "rm -rf",
        "dd if=",
        ":(){ :|:& };:",
        "mkfs",
        "format"
      ],
      "confirm_patterns": [
        "git push",
        "npm publish",
        "docker rm"
      ]
    }
  }
}
```

### Step 7: Verify

```bash
sofia doctor
```

✅ Your Sofia instance is hardened for production!

---

## Tutorial 7: MCP Integration

**Goal**: Connect Sofia to Claude Desktop as an MCP server.

### Step 1: Start Sofia MCP Server

```bash
sofia mcp-server -t stdio
```

### Step 2: Configure Claude Desktop

Edit `~/Library/Application Support/Claude/claude_desktop_config.json` (macOS) or `%APPDATA%\Claude\claude_desktop_config.json` (Windows):

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

### Step 3: Restart Claude Desktop

Close and reopen Claude Desktop. You should see Sofia's tools available.

### Step 4: Use Sofia's Tools in Claude

In Claude Desktop, you can now use Sofia's tools:

```
You: Search the web for "Go 1.22 features"

Claude: [Uses Sofia's web_search tool]
Here are the key features in Go 1.22...
```

### SSE Mode (for Network Access)

```bash
sofia mcp-server -t sse -a :9090
```

Connect from any MCP client to `http://localhost:9090/sse`.

✅ Sofia is now an MCP tool server!

---

## Tutorial 8: Email Assistant

**Goal**: Configure Sofia to process and respond to emails.

### Step 1: Set Up Gmail API

1. Go to [Google Cloud Console](https://console.cloud.google.com/)
2. Create a project and enable Gmail API
3. Create OAuth credentials
4. Install `gog` (Google CLI for Sofia):

```bash
# Install gog
brew install gog
# Or follow: https://github.com/grasberg/gog

# Authenticate
gog auth
```

### Step 2: Configure Email Channel

Edit `~/.sofia/config.json`:

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
  },
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

### Step 3: Set Up Email Processing Triggers

In a conversation:

```
You: When I get an email with "urgent" in the subject, notify me on Telegram

Sofia: I'll set up a trigger for urgent emails.

manage_triggers(
  action: "add",
  name: "urgent-email",
  condition: "Email subject contains 'urgent'",
  trigger_action: "Send Telegram notification with email summary"
)
```

### Step 4: Test

Send yourself a test email and verify Sofia processes it.

✅ Your email assistant is running!

---

## Tutorial 9: Custom Agent Template

**Goal**: Create a specialized agent for your workflow.

### Step 1: Add Agent to Config

Edit `~/.sofia/config.json`:

```json
{
  "agents": {
    "list": [
      {
        "id": "main",
        "default": true,
        "name": "Sofia",
        "subagents": { "allow_agents": ["*"] }
      },
      {
        "id": "code-reviewer",
        "name": "Code Reviewer",
        "template": "code-reviewer",
        "model_name": "gpt-4o",
        "max_tokens": 16384,
        "subagents": { "allow_agents": ["security-auditor", "test-engineer"] },
        "summarization": { "enabled": true }
      }
    ]
  }
}
```

### Step 2: Create Agent Workspace

```bash
mkdir -p ~/.sofia/workspace-code-reviewer
```

### Step 3: Use Your Custom Agent

```
You: Use the code-reviewer agent to review the auth module

Sofia: Spawning code-reviewer agent...

[Code Reviewer]: I've reviewed the auth module. Here are my findings:
1. JWT secret should use environment variables
2. Token refresh logic has a race condition
3. Missing rate limiting on login endpoint
4. Password hashing uses appropriate bcrypt cost factor

Overall: 7/10 security score. Recommendations applied.
```

✅ Your custom agent is ready!

---

## Tutorial 10: Monitoring & Debugging

**Goal**: Use Sofia's diagnostic tools to monitor and debug.

### Step 1: Run Doctor

```bash
sofia doctor
```

This checks:
- ✅ Config file exists and is valid
- ✅ Model API connectivity
- ✅ Channel connections
- ✅ Workspace permissions
- ✅ Tool availability

### Step 2: Check Gateway Logs

Run the gateway with debug logging:

```bash
sofia gateway -d
```

### Step 3: Check Daemon Status

```bash
sofia daemon status
```

### Step 4: Review Audit Trail

Sofia logs all actions to `~/.sofia/audit.db`:

```bash
# View recent audit entries
sqlite3 ~/.sofia/audit.db "SELECT * FROM audit ORDER BY timestamp DESC LIMIT 20;"
```

### Step 5: Monitor Memory Usage

```bash
# Check memory database size
ls -lh ~/.sofia/memory.db
```

### Step 6: Export Session Data

```bash
sofia data export --output sessions-backup.json
```

### Step 7: Common Issues

| Issue | Solution |
|-------|----------|
| Gateway won't start | Check port availability: `lsof -i :18790` |
| Model API errors | Verify API key and model name |
| Telegram not responding | Check bot token and `allow_from` |
| High memory usage | Reduce `max_concurrent_subagents` |
| Slow responses | Check `max_tool_iterations` and model speed |
| Agent not spawning | Verify agent ID in config and `allow_agents` |

✅ You can now monitor and debug Sofia effectively!

---

## Next Steps

- [Installation](./installation.md) — Set up Sofia
- [Configuration](./configuration.md) — Customize all settings
- [Skills System](./skills.md) — Extend with plugins
- [Multi-Agent](./multi-agent.md) — Coordinate agent teams
- [API Reference](./api-reference.md) — Full technical reference