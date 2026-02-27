# Sofia

Sofia is a lightweight AI assistant written in Go.

It provides a CLI-first agent workflow (`sofia agent`), a gateway mode for chat platforms (`sofia gateway`), scheduled tasks (`sofia cron`), pluggable skills, and configurable model providers via `model_list`.

## Features

- Lightweight Go implementation with a single-binary runtime.
- Interactive and one-shot agent usage from terminal.
- Gateway channel support: Telegram and Discord.
- Model-centric provider configuration (`model_list`) with vendor-style model references (for example `openai/gpt-5.2`, `anthropic/...`, `zhipu/...`).
- Scheduled jobs and reminders with `cron` (`every` interval and cron expression support).
- Skill management from CLI (`skills list/search/install/remove/show`).
- Workspace-based sandboxing (`restrict_to_workspace`) to limit file and command access.
- Built-in status/auth/version commands for operations and debugging.

## Install

### Install with precompiled binary

Download the binary for your platform from this repository's Releases page.

### Install from source

```bash
git clone https://github.com/<your-account>/sofia.git
cd sofia
make deps
make build
```

The built binary is available at `build/sofia`.

## Quick Start

1. Initialize config and workspace:

```bash
sofia onboard
```

2. Edit `~/.sofia/config.json` and set at least one model with an API key:

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

3. Run a one-shot message:

```bash
sofia agent -m "What is 2+2?"
```

4. Or start interactive mode:

```bash
sofia agent
```

## Common Commands

```bash
sofia onboard
sofia agent -m "hello"
sofia agent
sofia gateway
sofia status
sofia cron list
sofia skills list
sofia auth status
sofia version
```

## Gateway and Channels

Run gateway mode:

```bash
sofia gateway
```

Then enable and configure channels in `~/.sofia/config.json` under `channels`.

## Scheduling

Add recurring and cron-based jobs:

```bash
# Every 10 minutes
sofia cron add --name followup --message "Check pending tasks" --every 600

# Every day at 09:00
sofia cron add --name morning --message "Summarize today's priorities" --cron "0 9 * * *"
```

## Security Model

Sofia supports workspace restriction via:

```json
{
  "agents": {
    "defaults": {
      "restrict_to_workspace": true
    }
  }
}
```

When enabled, file and command tools are constrained to the configured workspace path.

## Documentation

- End users: `docs/USER_GUIDE.md`
- Developers: `docs/DEVELOPER_SETUP.md`
- Tools configuration: `docs/tools_configuration.md`
