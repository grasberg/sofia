# 📚 Sofia Documentation

> **Your personal AI assistant — local, private, extensible.**

Sofia is an open-source AI assistant that runs on your machine, connects to your favorite messaging apps, and orchestrates 40+ specialized agents to get things done.

---

## 🚀 Quick Links

| Section | Description |
|---------|-------------|
| [📦 Installation](./installation.md) | Get Sofia running in 5 minutes |
| [⚙️ Configuration](./configuration.md) | Full config reference for all features |
| [🧩 Skills System](./skills.md) | Extend Sofia with community & custom skills |
| [🤖 Multi-Agent Orchestration](./multi-agent.md) | Coordinate specialized AI agents |
| [📡 API Reference](./api-reference.md) | CLI, HTTP, MCP, and tool reference |
| [🎓 Tutorials](./tutorials.md) | Step-by-step guides for common workflows |

---

## What is Sofia?

Sofia is a **self-hosted AI assistant** built in Go that:

- **Runs locally** — your data stays on your machine
- **Connects everywhere** — Telegram, Discord, Email, Web UI
- **Orchestrates agents** — 40+ specialized templates for any task
- **Extends with skills** — plugin system via ClawHub registry
- **Speaks MCP** — integrate with Claude Desktop, Cursor, and other AI tools
- **Automates workflows** — cron jobs, triggers, autonomous goals

```
┌─────────────────────────────────────────────┐
│              Your Channels                   │
│   Telegram  ·  Discord  ·  Email  ·  Web   │
└──────────────────┬──────────────────────────┘
                   │
┌──────────────────▼──────────────────────────┐
│           Sofia Gateway (:18790)             │
│         Router · Auth · Session Mgmt         │
└──────────────────┬──────────────────────────┘
                   │
┌──────────────────▼──────────────────────────┐
│            Agent Orchestrator                │
│    ┌──────┐ ┌──────┐ ┌──────┐ ┌──────┐      │
│    │Front.│ │Back. │ │Test  │ │Secur.│ ...  │
│    └──────┘ └──────┘ └──────┘ └──────┘      │
└──────────────────┬──────────────────────────┘
                   │
┌──────────────────▼──────────────────────────┐
│           Tools & Skills Layer               │
│  Web · Exec · Files · Google · GitHub · ... │
└─────────────────────────────────────────────┘
```

---

## ✨ Key Features

- **🔌 Multi-Channel** — Chat via Telegram, Discord, Email, or Web UI
- **🤖 40+ Agent Templates** — From frontend specialist to security auditor
- **🧩 Skills Marketplace** — Discover and install extensions from ClawHub
- **🔄 Multi-Agent Orchestration** — Spawn, coordinate, and synthesize agent teams
- **⏰ Automation** — Cron jobs, triggers, and autonomous goal pursuit
- **🔒 Privacy-First** — Runs locally, no cloud dependency
- **🌐 MCP Server** — Expose Sofia's tools to any MCP-compatible client
- **🧠 Evolution** — Agents that self-improve and auto-scale based on usage
- **🛡️ Guardrails** — Input validation, rate limiting, PII detection, sandboxed execution

---

## 🏁 Quick Start

```bash
# Install
brew install sofia
# or: curl -fsSL https://get.sofia.ai | bash

# Initialize
sofia onboard

# Start
sofia gateway

# Verify
sofia doctor
```

👉 **Full installation guide**: [Installation](./installation.md)

---

## 📖 Documentation Sections

### For New Users
1. [Installation](./installation.md) — Set up Sofia on your machine
2. [Configuration](./configuration.md) — Connect channels and customize behavior
3. [Tutorials](./tutorials.md) — Follow step-by-step guides

### For Developers
4. [Skills System](./skills.md) — Build and publish custom skills
5. [Multi-Agent Orchestration](./multi-agent.md) — Coordinate agent teams
6. [API Reference](./api-reference.md) — CLI, HTTP, MCP, and tool docs

---

## 🫂 Community

- **GitHub**: [github.com/grasberg/sofia](https://github.com/grasberg/sofia)
- **Issues**: [Report a bug](https://github.com/grasberg/sofia/issues)
- **Discussions**: [Join the conversation](https://github.com/grasberg/sofia/discussions)

---

## 📄 License

Sofia is open source. See the [repository](https://github.com/grasberg/sofia) for license details.