<div align="center">

# 🤖 Sofia

### Your Local, Autonomous AI Colleague

[![GitHub stars](https://img.shields.io/github/stars/grasberg/sofia?style=social)](https://github.com/grasberg/sofia/stargazers)
[![GitHub forks](https://img.shields.io/github/forks/grasberg/sofia?style=social)](https://github.com/grasberg/sofia/network/members)
[![License](https://img.shields.io/github/license/grasberg/sofia)](https://github.com/grasberg/sofia/blob/main/LICENSE)
[![Go Version](https://img.shields.io/github/go-mod/go-version/grasberg/sofia)](https://go.dev/)
[![Last Commit](https://img.shields.io/github/last-commit/grasberg/sofia)](https://github.com/grasberg/sofia/commits/main)
[![Version](https://img.shields.io/badge/version-v0.0.145-blue)](https://github.com/grasberg/sofia/releases)
[![PRs Welcome](https://img.shields.io/badge/PRs-welcome-brightgreen.svg)](CONTRIBUTING.md)

**A self-improving AI orchestrator that runs 100% locally.**  
Single Go binary. 40+ tools. 20+ LLM providers. Multi-agent coordination.  
Persistent memory with a knowledge graph. Browser automation. Computer control.  
**And it gets better at its job over time.**

[🚀 Quick Start](#-quick-start) · [✨ Features](#-key-features) · [📊 Scorecard](#-agentic-ai-capability-scorecard) · [🏗️ Architecture](#-architecture) · [🤝 Contributing](CONTRIBUTING.md)

</div>

---

## Why Sofia?

Most AI assistants are cloud-locked chatbots that wait for you to type something. **Sofia is different.**

- 🏠 **Local-first** — No cloud dependency. Your data stays on your machine.
- ⚡ **Single binary** — Written in Go. No Python, no Docker, no Node.js. Just `make build`.
- 🧠 **Self-improving** — Evolution engine analyzes past performance and optimizes itself.
- 🤖 **Multi-agent** — Spawn, coordinate, and retire agents dynamically. A2A protocol built-in.
- 🔐 **Security-first** — 35+ prompt injection defenses, PII detection, AES-256-GCM encryption.
- 🌐 **20+ LLM providers** — OpenAI, Claude, Gemini, DeepSeek, Grok, and more. Automatic fallback.

---

## 🚀 Quick Start

```bash
git clone https://github.com/grasberg/sofia.git
cd sofia
make deps && make build
./build/sofia onboard      # Initialize config & workspace
./build/sofia gateway      # Start the web UI
# Open http://127.0.0.1:18795 → Models tab → add your API key → start chatting
```

> **Prerequisites:** Go 1.26+ ([download](https://go.dev/dl/))

---

## ✨ Key Features

### 🛠️ Autonomous Tool Use
Register domain names, publish web pages, read/edit files, run bash commands, manage Google Services (Gmail/Calendar) — all without human intervention.

### 🧠 Advanced Memory
Tiered memory system — short-term, long-term, episodic, and semantic (knowledge graph). Automatic consolidation and strategic forgetting keep memory efficient.

### 🤖 Multi-Agent Orchestration
Delegate tasks to parallel agents. **AgentArchitect** creates and optimizes new agents on the fly. A2A mailbox messaging with broadcast.

### 🌐 20+ LLM Providers
OpenAI, Anthropic (Claude 4.5), Gemini, DeepSeek, Grok, MiniMax, Moonshot, Qwen, GitHub Copilot, and more. Automatic fallback chains with exponential backoff.

### 📚 Skill System with Self-Learning
126+ built-in skills with expert personas. Automatic skill creation, refinement, and knowledge distillation. ClawHub marketplace for community skills.

### 🔄 Evolution & Self-Improvement
5-phase `EvolutionEngine`: observe → diagnose → plan → act → verify. Post-task evaluation, prompt self-optimization, and code self-modification.

### 🖥️ Computer Use
Autonomous computer control via screenshots and vision-LLM. Control mouse and keyboard on macOS and Linux.

### 🌍 Browser Automation
Playwright-based web browsing. Click, fill forms, take screenshots, extract text — all autonomously.

### 🛡️ Guardrails & Security
Input validation, budget management, prompt injection defense (35+ patterns in 6 languages), PII detection, and action confirmation for high-risk operations.

<details>
<summary><b>📋 More Features</b></summary>

- **Smart Tool Management** — Semantic tool matching via embeddings, performance tracking, dynamic tool composition
- **Lightning-Fast Execution** — Caching of tool definitions, prompts, and connection pooling for low latency
- **Autonomy & Proactivity** — Long-term goals, context-aware triggers, proactive suggestions, self-initiated research
- **MCP Client** — Model Context Protocol support for external MCP servers and tools
- **Gateway Mode** — Built-in support for Telegram, Discord, Email, Web, and CLI
- **Plan & Execute** — Structured task planning with step-by-step tracking and sub-plans
- **Shared Scratchpad** — Key-value storage for agent-to-agent communication
- **Cron Scheduling** — Independent task scheduling with at/every/cron patterns
- **Modern Web UI** — Brutalist design with CRT effects, real-time updates, and file uploads

</details>

---

## 📊 Agentic AI Capability Scorecard

> Evaluated by automated code analysis across 12 capability dimensions.

| Category | Score | Highlights |
|:---------|:-----:|:-----------|
| **Multi-Agent Orchestration** | 9/10 | Dependency-aware scheduling, semantic delegation, dynamic agent spawning, A2A messaging |
| **Tool Use & Execution** | 9/10 | 40+ tools, embeddings-based filtering, parallel execution, circuit breaker |
| **Context & Memory** | 9/10 | Knowledge graph, 4-layer memory, strategic forgetting, prompt cache |
| **Planning & Reasoning** | 9/10 | Hierarchical plans, doom loop detection, evaluation loop, auto-checkpoint |
| **Safety & Guardrails** | 9/10 | 35+ injection patterns, PII detection, secret scrubbing, AES-256-GCM |
| **Provider Abstraction** | 9/10 | 20+ providers, fallback chains, Bayesian quality ranking, retry with jitter |
| **Channel Integration** | 8/10 | Telegram, Discord, Email, Web, CLI with shared retry logic |
| **Observability & Ops** | 9/10 | SQLite audit logging, distributed tracing, health/metrics endpoints |
| **Self-Improvement** | 8/10 | 7-phase evolution loop, SafeModifier, performance-driven agent retirement |
| **Skills & Extensibility** | 8/10 | 126 skills, 40 agent templates, ClawHub marketplace, lazy loading |
| **Evaluation & Testing** | 8/10 | LLM-as-judge scoring, A/B comparison, 5 benchmark suites |
| **Scheduling & Automation** | 8/10 | 3 schedule types, heartbeat, goal-driven autonomy, context triggers |
| | **Avg: 8.6** | |

---

## 🏗️ Architecture

```
┌─────────────────────────────────────────────────────────┐
│                    Channels (Input)                      │
│   Telegram  ·  Discord  ·  Email  ·  Web UI  ·  CLI    │
└──────────────────────────┬──────────────────────────────┘
                           │
                    ┌──────▼──────┐
                    │   Gateway    │  ← HTTP/WebSocket/REST
                    └──────┬──────┘
                           │
              ┌────────────┼────────────────┐
              │         Orchestrator          │
              │  ┌─────────┴──────────┐      │
              │  │   Evolution Engine  │      │
              │  │  (Self-Improvement) │      │
              │  └────────────────────┘      │
              │                               │
              │  ┌──────────┐  ┌──────────┐  │
              │  │  Agent 1  │  │  Agent 2  │  │
              │  │  (A2A)    │  │  (A2A)    │  │
              │  └─────┬────┘  └─────┬────┘  │
              └────────┼─────────────┼────────┘
                       │             │
              ┌────────┴─────────────┴────────┐
              │         Tool Layer             │
              │  File · Shell · Web · Browser  │
              │  GitHub · Google · Bitcoin     │
              │  Cron · Memory · Plan · MCP    │
              └───────────────┬────────────────┘
                              │
                    ┌─────────▼─────────┐
                    │   LLM Providers    │
                    │  20+ with fallback │
                    └───────────────────┘
```

---

## ⚔️ Comparison

| | **Sofia** | **AutoGPT** | **CrewAI** | **LangChain** | **OpenDevin** |
|---|:---:|:---:|:---:|:---:|:---:|
| **Runs 100% locally** | ✅ | ❌ | ❌ | ❌ | ❌ |
| **Single binary** | ✅ | ❌ | ❌ | ❌ | ❌ |
| **40+ built-in tools** | ✅ | Limited | Via plugins | Via plugins | Limited |
| **Knowledge graph** | ✅ | Basic | ❌ | ❌ | ❌ |
| **Self-improving** | ✅ | ❌ | ❌ | ❌ | ❌ |
| **Multi-agent orchestration** | ✅ | Basic | ✅ | Basic | ❌ |
| **Browser automation** | ✅ | ❌ | ❌ | ❌ | ❌ |
| **Computer use** | ✅ | ❌ | ❌ | ❌ | ❌ |
| **20+ LLM providers** | ✅ | Limited | Limited | ✅ | Limited |
| **Security guardrails** | ✅ | ❌ | ❌ | ❌ | ❌ |
| **Language** | Go | Python | Python | Python | Python |

---

## 📂 Workspace Structure

```
~/.sofia/
├── config.json          # LLM providers, model selection, preferences
├── workspace/
│   ├── IDENTITY.md      # Who Sofia is
│   ├── SOUL.md          # Personality & behavior rules
│   ├── USER.md          # User preferences
│   ├── skills/          # 126+ skill definitions
│   └── agents/          # 40+ agent templates
├── db/                  # SQLite databases (memory, audit, goals)
└── logs/                # Execution logs
```

---

## 🗺️ Roadmap

- [ ] **Plugin SDK** — Third-party tool development kit
- [ ] **Voice Interface** — Speech-to-text and text-to-speech
- [ ] **Mobile Companion** — iOS/Android control app
- [ ] **Team Mode** — Multi-user collaboration
- [ ] **Cloud Sync** — Optional encrypted cloud backup
- [ ] **Marketplace** — Community skills and agents marketplace

---

## 🤝 Contributing

We love contributions! See [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines.

Whether you are fixing a bug, adding a feature, improving docs, or sharing a skill — every contribution matters.

## 🛡️ Security

Found a vulnerability? Please see [SECURITY.md](SECURITY.md) for responsible disclosure.

## 📄 License

Sofia is released under the [MIT License](LICENSE).

---

<div align="center">

**[⭐ Star us on GitHub](https://github.com/grasberg/sofia)** · **[🐛 Report a Bug](https://github.com/grasberg/sofia/issues)** · **[💬 Join the Discussion](https://github.com/grasberg/sofia/discussions)**

Made with ❤️ by the Sofia community

</div>