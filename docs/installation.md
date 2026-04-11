# 📦 Installation Guide

Complete guide to installing and setting up Sofia on your machine.

---

## Prerequisites

| Requirement | Details |
|-------------|---------|
| **OS** | macOS (Apple Silicon), Linux (x86_64/ARM64), Windows (x86_64) |
| **Go** | 1.21+ (only needed for building from source) |
| **LLM Access** | An OpenAI-compatible API endpoint, or local model server |
| **Disk** | ~50MB for the binary, ~200MB with workspaces |

---

## Installation Methods

### Option 1: Homebrew (macOS / Linux)

```bash
brew tap grasberg/sofia
brew install sofia
```

### Option 2: Install Script

```bash
curl -fsSL https://get.sofia.ai | bash
```

This detects your OS and architecture, downloads the latest release, and installs to `/usr/local/bin/sofia`.

### Option 3: Docker

```bash
docker pull ghcr.io/grasberg/sofia:latest

# Run with config mounted
docker run -d \
  --name sofia \
  -v ~/.sofia:/root/.sofia \
  -p 18790:18790 \
  -p 18795:18795 \
  ghcr.io/grasberg/sofia:latest \
  sofia gateway
```

### Option 4: Build from Source

```bash
git clone https://github.com/grasberg/sofia.git
cd sofia
go build -o sofia ./cmd/sofia
sudo mv sofia /usr/local/bin/
```

### Option 5: Download Binary

Download the latest release for your platform from [GitHub Releases](https://github.com/grasberg/sofia/releases).

```bash
# macOS (Apple Silicon)
curl -L -o sofia https://github.com/grasberg/sofia/releases/latest/download/sofia-darwin-arm64
chmod +x sofia
sudo mv sofia /usr/local/bin/

# Linux (x86_64)
curl -L -o sofia https://github.com/grasberg/sofia/releases/latest/download/sofia-linux-amd64
chmod +x sofia
sudo mv sofia /usr/local/bin/

# Windows (PowerShell)
Invoke-WebRequest -Uri "https://github.com/grasberg/sofia/releases/latest/download/sofia-windows-amd64.exe" -OutFile "sofia.exe"
```

---

## Verify Installation

```bash
sofia version
# Output: sofia v0.0.145 (git: d981c0b2)
#         Build: 2026-04-11T12:46:37+0200
#         Go: go1.26.0
```

---

## First-Time Setup

### Step 1: Initialize Configuration

```bash
sofia onboard
```

This interactive wizard will:
1. Create `~/.sofia/config.json` with default settings
2. Create your workspace directory at `~/.sofia/workspace/`
3. Prompt for your preferred LLM provider and API key
4. Optionally configure a messaging channel (Telegram, Discord, or Email)

### Step 2: Configure Your LLM Provider

Edit `~/.sofia/config.json` and set your model:

```json
{
  "agents": {
    "defaults": {
      "model_name": "gpt-4o",
      "model_fallbacks": ["gpt-4o-mini", "claude-3-haiku"]
    }
  }
}
```

Supported model formats:
- `gpt-4o`, `gpt-4o-mini` — OpenAI
- `claude-3-opus`, `claude-3-sonnet` — Anthropic
- `glm-5.1:cloud` — GLM
- `gemma4:31b-cloud` — Gemma
- Any OpenAI-compatible endpoint (set `provider` and base URL)

### Step 3: Start the Gateway

```bash
sofia gateway
```

The gateway starts on `127.0.0.1:18790` by default. You'll see:

```
🪲 Sofia Gateway v0.0.145
   Listening on 127.0.0.1:18790
   Web UI: http://127.0.0.1:18795
   Channels: telegram ✓
```

### Step 4: Verify with Doctor

```bash
sofia doctor
```

This checks your configuration, connectivity, and channel status.

---

## Running as a Daemon

For persistent operation, install Sofia as a background service:

```bash
# Install as daemon
sofia daemon install

# Check status
sofia daemon status

# Remove daemon
sofia daemon uninstall
```

On macOS, this creates a `launchd` plist. On Linux, a `systemd` service.

---

## Connecting a Channel

### Telegram

1. Create a bot via [@BotFather](https://t.me/BotFather) and get the token
2. Edit `~/.sofia/config.json`:

```json
{
  "channels": {
    "telegram": {
      "enabled": true,
      "token": "123456:ABC-DEF...",
      "allow_from": ["your_telegram_user_id"]
    }
  }
}
```

3. Restart the gateway: `sofia gateway`

### Discord

1. Create a bot in the [Discord Developer Portal](https://discord.com/developers/applications)
2. Configure:

```json
{
  "channels": {
    "discord": {
      "enabled": true,
      "token": "your-bot-token",
      "allow_from": ["your_discord_user_id"],
      "mention_only": true
    }
  }
}
```

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

---

## Updating

```bash
# Homebrew
brew upgrade sofia

# Install script
curl -fsSL https://get.sofia.ai | bash

# Docker
docker pull ghcr.io/grasberg/sofia:latest
```

Your configuration and workspaces are preserved across updates.

---

## Uninstalling

```bash
# Remove daemon first
sofia daemon uninstall

# Remove binary
sudo rm /usr/local/bin/sofia

# Remove config and data (optional)
rm -rf ~/.sofia
```

---

## Troubleshooting

### "Command not found: sofia"

Ensure `/usr/local/bin` (or your install path) is in your `$PATH`:

```bash
echo $PATH
# If missing:
export PATH="/usr/local/bin:$PATH"
```

### Gateway won't start

```bash
# Check if port is in use
lsof -i :18790

# Use a different port
# Edit config.json: "gateway": { "port": 18791 }
```

### Telegram bot not responding

1. Verify the bot token is correct
2. Ensure `allow_from` includes your Telegram user ID (not username)
3. Check gateway logs for connection errors

### Model API errors

1. Verify your API key is valid
2. Check the model name matches your provider's format
3. Test with `sofia agent -m "gpt-4o" -m "hello"`

### Permission denied on workspace

```bash
# Fix ownership
sudo chown -R $(whoami) ~/.sofia
```

---

## Next Steps

- [Configuration Reference](./configuration.md) — Customize all settings
- [Tutorials](./tutorials.md) — Step-by-step guides
- [Multi-Agent Orchestration](./multi-agent.md) — Set up agent teams