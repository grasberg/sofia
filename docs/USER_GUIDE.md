# Sofia User Guide

This guide is for end users who want to run Sofia quickly and use it daily.

## Who this is for

- You want to chat with Sofia from terminal or chat apps.
- You want a simple setup without diving into source code.

## Prerequisites

- A Linux or macOS machine.
- An API key for at least one model provider (for example OpenAI, OpenRouter, Anthropic, Zhipu, Gemini).
- Optional: a web search API key (Brave or Tavily). DuckDuckGo fallback is available.

## Install

### Option 1: Precompiled binary

Download the binary for your platform from Releases:

- <https://github.com/sipeed/sofia/releases>

### Option 2: Build from source

```bash
git clone https://github.com/sipeed/sofia.git
cd sofia
make deps
make build
```

Binary output is placed at `build/sofia` (symlink) and `build/sofia-<os>-<arch>`.

### Option 3: Docker Compose

```bash
git clone https://github.com/sipeed/sofia.git
cd sofia

# First run creates docker/data/config.json and exits
docker compose -f docker/docker-compose.yml --profile gateway up

# Edit API keys and channel credentials
vim docker/data/config.json

# Start in background
docker compose -f docker/docker-compose.yml --profile gateway up -d
```

## 5-minute quick start

### 1) Initialize workspace and config

```bash
sofia onboard
```

This creates `~/.sofia/config.json` and default workspace data.

### 2) Configure model provider

Edit `~/.sofia/config.json` and set one model in `model_list`.

```json
{
  "agents": {
    "defaults": {
      "workspace": "~/.sofia/workspace",
      "model": "gpt-5.2"
    }
  },
  "model_list": [
    {
      "model_name": "gpt-5.2",
      "model": "openai/gpt-5.2",
      "api_key": "YOUR_API_KEY"
    }
  ]
}
```

### 3) Ask a question

```bash
sofia agent -m "What is 2+2?"
```

### 4) Start interactive mode

```bash
sofia agent
```

## Everyday usage

### One-shot and interactive chat

- One-shot: `sofia agent -m "Summarize today's top AI news"`
- Interactive: `sofia agent`
- Choose model per request: `sofia agent --model claude-sonnet-4.6 -m "Hello"`
- Use a specific session key: `sofia agent --session cli:work`

### Run as a chat gateway

```bash
sofia gateway
```

Use this when you connect Sofia to Telegram, Discord, LINE, and other channels.

### Check status

```bash
sofia status
```

### Schedule reminders and recurring jobs

```bash
# Every 10 minutes
sofia cron add --name standup --message "Remind me to post standup" --every 600

# Every day at 9:00
sofia cron add --name morning --message "Give me today's plan" --cron "0 9 * * *"

# List jobs
sofia cron list
```

### Manage skills

```bash
sofia skills list
sofia skills search "weather"
sofia skills install weather
```

### Auth helpers

```bash
sofia auth status
sofia auth models
sofia auth login --provider openai
```

## Chat app setup

Channel setup details are documented in the main README and channel docs:

- Telegram: `docs/channels/telegram/README.zh.md`
- Discord: `docs/channels/discord/README.zh.md`
- LINE: `docs/channels/line/README.zh.md`

## Config tips

- Main config file: `~/.sofia/config.json`
- Default workspace: `~/.sofia/workspace`
- For full tools config: `docs/tools_configuration.md`
- For legacy provider migration: `docs/migration/model-list-migration.md`

## Troubleshooting

### "API key configuration issue"

Usually means no valid provider key is set in `model_list`.

### Gateway not responding in Telegram/Discord

Make sure only one `sofia gateway` process is running.

### Web search has weak/empty results

Enable Brave or Tavily in config, or keep DuckDuckGo fallback enabled.

### Docker cannot reach gateway from host

Set `SOFIA_GATEWAY_HOST=0.0.0.0` or update gateway host in config.

## Useful command reference

- `sofia onboard` - initialize config and workspace.
- `sofia agent` - interactive chat.
- `sofia agent -m "..."` - one-shot chat.
- `sofia gateway` - run multi-channel gateway.
- `sofia status` - show runtime status.
- `sofia cron <subcommand>` - manage scheduled jobs.
- `sofia skills <subcommand>` - search/install/list skills.
- `sofia auth <subcommand>` - login/logout/status/models.
