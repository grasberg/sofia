<div align="center">

<img src="assets/sofiamantis.png" alt="Sofia Logo" width="160" />

# Sofia

### The local, autonomous AI agent that actually gets things done

[![GitHub Stars](https://img.shields.io/github/stars/grasberg/sofia?style=social)](https://github.com/grasberg/sofia/stargazers)
[![GitHub Forks](https://img.shields.io/github/forks/grasberg/sofia?style=social)](https://github.com/grasberg/sofia/network/members)
[![CI](https://github.com/grasberg/sofia/actions/workflows/ci.yaml/badge.svg)](https://github.com/grasberg/sofia/actions/workflows/ci.yaml)
[![License: MIT](https://img.shields.io/github/license/grasberg/sofia)](https://github.com/grasberg/sofia/blob/main/LICENSE)
[![Go Version](https://img.shields.io/github/go-mod/go-version/grasberg/sofia)](https://go.dev/)
[![Last Commit](https://img.shields.io/github/last-commit/grasberg/sofia)](https://github.com/grasberg/sofia/commits/main)
[![Version](https://img.shields.io/badge/version-v0.0.145-blue)](https://github.com/grasberg/sofia/releases)
[![Platform](https://img.shields.io/badge/platform-macOS%20%7C%20Linux%20%7C%20Windows-lightgrey)](https://github.com/grasberg/sofia)
[![PRs Welcome](https://img.shields.io/badge/PRs-welcome-brightgreen.svg)](CONTRIBUTING.md)

**Single Go binary · 40+ built-in tools · 20+ LLM providers · Multi-agent orchestration · Knowledge graph memory · Self-improving**

Sofia runs **100% locally** — it reads your files, runs commands, browses the web, controls your computer, coordinates parallel agents, and **gets better at its job over time**. No cloud dependency. No data leaves your machine.

[🚀 Quick Start](#-quick-start) · [✨ Features](#-key-features) · [🤖 Multi-Agent](#-multi-agent-orchestration) · [🧠 Memory](#-advanced-memory-architecture) · [🖥️ Computer Use](#-computer-use) · [💬 Chat](#-gateway-mode--chat-platforms) · [🤝 Contributing](CONTRIBUTING.md)

</div>

---

> 💡 **One-line pitch:** Sofia is a self-improving AI orchestrator that runs entirely on your machine — think AutoGPT meets LangChain, but as a single Go binary with built-in memory, tools, and multi-agent coordination.

---

## 📑 Table of Contents

- [Why Sofia?](#-why-sofia)
- [Quick Start](#-quick-start)
- [Key Features](#-key-features)
- [Capability Scorecard](#-agentic-ai-capability-scorecard)
- [Workspace Structure](#-workspace-structure)
- [Multi-Agent Orchestration](#-multi-agent-orchestration)
- [Computer Use](#-computer-use)
- [Browser Automation](#-browser-automation-playwright)
- [Image Analysis](#-image-analysis)
- [Advanced Memory](#-advanced-memory-architecture)
- [LLM Providers](#-broad-ai-support--llm-providers)
- [Skill System](#-skill-system--self-learning)
- [Evolution Engine](#-evolution--self-improvement)
- [Guardrails and Safety](#-guardrails--safety)
- [Gateway Mode](#-gateway-mode--chat-platforms)
- [Web UI](#-web-ui)
- [Architecture](#-architecture)
- [Contributing](#-contributing)

---

## 🤔 Why Sofia?

Most AI assistants are cloud-locked chatboxes that answer questions. Sofia is a **local-first, autonomous agent** that integrates directly into your development environment and **takes action**:

| What Sofia does | How it's different |
|---|---|
| 🔧 **Uses 40+ tools autonomously** | Not just chat — reads/writes files, runs commands, browses the web, manages services |
| 🧠 **Remembers everything** | 4-layer memory with knowledge graph — not stateless, not cloud-dependent |
| 🤖 **Spawns parallel agents** | AgentArchitect creates specialized agents on-the-fly for complex tasks |
| 🔄 **Gets better over time** | EvolutionEngine evaluates performance and self-optimizes prompts, skills, and agents |
| 🖥️ **Controls your computer** | Screenshots + vision-LLM for mouse/keyboard automation on macOS and Linux |
| 🌍 **Automates browsers** | Built-in Playwright for clicks, forms, extraction, and navigation |
| 💬 **Works everywhere** | CLI, Web UI, Telegram, Discord — same agent, same memory |
| 🔒 **Runs locally** | Single Go binary. No telemetry. No data sent to cloud (except your chosen LLM provider) |

### Comparison

| | Sofia | LangChain | AutoGPT | n8n |
|---|:---:|:---:|:---:|:---:|
| Runs 100% locally | ✅ | ❌ | ❌ | Partial |
| Single binary (Go) | ✅ | ❌ (Python) | ❌ (Python) | ❌ (Node) |
| 40+ built-in tools | ✅ | Via plugins | Limited | Via nodes |
| Persistent memory and knowledge graph | ✅ | ❌ | Basic | ❌ |
| 20+ LLM providers with fallback | ✅ | ✅ | Limited | Limited |
| Self-improving reflection engine | ✅ | ❌ | ❌ | ❌ |
| Multi-agent orchestration | ✅ | Basic | Basic | ❌ |
| Browser automation (Playwright) | ✅ | ❌ | ❌ | ❌ |
| Computer use (mouse/keyboard) | ✅ | ❌ | ❌ | ❌ |
| Web UI with real-time dashboard | ✅ | ❌ | Basic | ✅ |

---

## 🚀 Quick Start

```bash
git clone https://github.com/grasberg/sofia.git
cd sofia
make deps && make build
./build/sofia onboard      # Initialize config and workspace
./build/sofia gateway      # Start the web UI
# Open http://127.0.0.1:18795 -> Models tab -> add your API key -> start chatting
```

> **Prerequisites:** Go 1.26+ ([download](https://go.dev/dl/))

<details>
<summary>Alternative: Direct binary (no Go installed)</summary>

Download the latest release binary from [GitHub Releases](https://github.com/grasberg/sofia/releases) (once available — currently building from source is required).

</details>

---

## ✨ Key Features

### 🛠️ Autonomous Tool Use
Can register domain names, publish web pages, read/edit files, run bash commands, manage Google Services (Gmail/Calendar), and 40+ more — all without human intervention.

### 🧠 Advanced Memory
Tiered memory — short-term, long-term, episodic, and semantic (knowledge graph). Automatic consolidation and strategic forgetting keep memory efficient and relevant.

### 🤖 Multi-Agent Orchestration
Delegate tasks to parallel agents. Includes **AgentArchitect** for autonomous creation and optimization of new agents "on the fly".

### 🌐 Broad AI Support
Built-in support for 20+ AI providers including OpenAI, Anthropic (Claude 4.5), Gemini, DeepSeek, Grok, MiniMax, Moonshot, Qwen, Zai, GitHub Copilot, and more.

### 📚 Skill System with Self-Learning
Antigravity Kit with expert personas, plus automatic skill creation, refinement, and knowledge distillation.

### 🔄 Evolution and Self-Improvement
5-phase `EvolutionEngine`, post-task evaluation, prompt self-optimization, and code self-modification for continuous learning and adaptation.

### 🔧 Smart Tool Management
Semantic tool matching via embeddings, performance tracking, and dynamic tool composition (pipelines) to create new macro tools.

### ⚡ Lightning-Fast Execution
Caching of tool definitions, prompts, and connection pooling for extremely low latency in the hot path.

### 🎯 Autonomy and Proactivity
Long-term goals, context-aware triggers, proactive suggestions, and self-initiated research without user interaction.

### 🛡️ Guardrails and Security
Input validation, budget management (tokens/executions), prompt injection defense, and action confirmation for high-risk operations.

### 🔌 MCP Client
Model Context Protocol support for hooking into external MCP servers and tools.

### 💬 Gateway Mode
Built-in support for chat platforms like Telegram and Discord via `sofia gateway`.

### 🖥️ Computer Use
Autonomous computer control via screenshots and vision-LLM — control mouse and keyboard on macOS and Linux.

### 🌍 Browser Automation (Playwright)
Autonomous web browsing with clicks, form filling, screenshots, and text extraction.

### 📸 Image Analysis
Analyze local images (PNG, JPEG, GIF, WebP) via vision-LLM — OCR, descriptions, and queries.

### 📋 Plan and Execute
Structured task planning with step-by-step tracking.

### 📝 Shared Scratchpad
Key-value storage for agent-to-agent communication.

### ⏰ Cron Scheduling
The agent can independently create, list, delete, and schedule recurring tasks.

### 🔄 Provider Fallback
Automatic fallback chains if an AI provider fails.

### 🎨 Modern Web UI (HTMX)
Brutalist design theme with CRT effects, real-time updates, and file uploads directly in chat.

---

## 📊 Agentic AI Capability Scorecard

> Evaluated by automated code analysis across 12 capability dimensions. Each score reflects the depth, robustness, and sophistication of the actual implementation.

| Category | Score | Highlights |
|:---------|:-----:|:-----------|
| **Multi-Agent Orchestration** | 9/10 | Dependency-aware topological scheduling, LLM-powered semantic delegation, dynamic agent auto-spawning, A2A mailbox messaging with broadcast |
| **Tool Use and Execution** | 9/10 | 40+ tools (file, shell, web, browser, hardware, MCP), embeddings-based semantic filtering, parallel execution with path-overlap safety, circuit breaker |
| **Context and Memory** | 9/10 | Knowledge graph with weighted relations, 4-layer memory (long-term, daily, graph, reflections), strategic forgetting with exponential decay, prompt cache optimization |
| **Planning and Reasoning** | 9/10 | Hierarchical plans with sub-plans, doom loop detection (4 signals, graduated recovery), evaluation loop with scored retries, auto-checkpoint/rollback |
| **Safety and Guardrails** | 9/10 | 35+ prompt injection patterns (6 languages), PII detection with Luhn/RFC1918 validation, inbound+outbound secret scrubbing, approval gates with audit trail, AES-256-GCM encryption |
| **Provider Abstraction** | 9/10 | 20+ LLM providers, fallback chains with 40+ error patterns, exponential backoff cooldown, Bayesian quality ranking, retry with jitter |
| **Channel Integration** | 8/10 | Telegram, Discord, Email, Web, CLI with shared retry logic, file sending (images/documents), voice transcription, Discord markdown formatting, DM pairing |
| **Observability and Ops** | 9/10 | SQLite audit logging, distributed tracing with span scoring, `/health` + `/ready` + `/metrics` endpoints, budget persistence, real-time WebSocket dashboard |
| **Self-Improvement** | 8/10 | 7-phase evolution loop (observe-diagnose-plan-act-verify-consolidate-improve), SafeModifier with 3 safety layers, performance-driven agent retirement, skill auto-improvement |
| **Skills and Extensibility** | 8/10 | 126 skills + 40 agent templates, 4-tier priority loading, ClawHub remote marketplace, trigram search cache, lazy loading with XML summaries |
| **Evaluation and Testing** | 8/10 | Agent execution harness with parallel runs, LLM-as-judge scoring, A/B comparison with bootstrap confidence intervals, 5 built-in benchmark suites, SQLite persistence with trend detection |
| **Scheduling and Automation** | 8/10 | 3 schedule types (at/every/cron) with context cancellation, heartbeat with active hours/days, goal-driven autonomy (multi-step), context triggers, proactive suggestions |
| | **Avg: 8.6** | |

---

## 📂 Workspace Structure

Sofia's configuration and workspace are located under `~/.sofia/`:

```text
~/.sofia/
+-- config.json            # Main configuration (models, channels, settings)
+-- memory.db              # Shared SQLite database for memory and session history
+-- workspace/             # Sofia's workspace
    +-- IDENTITY.md        # Core identity: tone, role, and how Sofia presents herself
    +-- SOUL.md            # Core principles: behavior, values, and decision style
    +-- AGENT.md           # Agent-specific system prompt
    +-- USER.md            # User context and preferences
    +-- agents/            # Specialized agents (20 base agents included)
    +-- skills/            # Local skills/expert personas (159 skills included)
    +-- cron/              # Scheduled jobs (jobs.json)
    +-- state/             # Persistent runtime state
```

---

## 🤖 Multi-Agent Orchestration

Sofia can delegate and coordinate work across multiple agents:

- **Orchestrate tool:** Define subtasks with dependencies — independent tasks run in parallel, dependent tasks in the correct order. Automatic agent selection based on scoring.
- **AgentArchitect and Sub-Agents:** Autonomous design and provisioning of specialized agents "on the fly" when problems are identified, plus 20 dedicated base agents.
- **A2A Protocol (Agent-to-Agent):** Standardized inter-agent communication with mailbox-based routing, send/receive/broadcast, and pending-polling.
- **Shared Scratchpad:** Agents can share data via a key-value store namespaced per task group.
- **Plan and Execute:** Create structured plans with steps that can be tracked and updated during execution.

---

## 🖥️ Computer Use

Sofia can control your computer autonomously via screenshots and vision-LLM:

- Takes screenshots of the desktop and analyzes them using vision-LLM
- Performs mouse clicks, keystrokes, scrolling, and text input
- Loops until the task is complete or the maximum number of steps is reached
- **Platforms:** macOS (screencapture + osascript) and Linux (scrot + xdotool)

---

## 🌍 Browser Automation (Playwright)

Sofia has built-in Playwright integration for autonomous web browsing:

- Navigate to URLs, click elements, fill out forms
- Take screenshots, extract text, and run JavaScript
- Wait for elements, handle timeouts, and scroll
- Supports Chromium, Firefox, and WebKit
- Headless and headful modes

---

## 📸 Image Analysis

Analyze local images directly in the conversation:

- Support for PNG, JPEG, GIF, and WebP
- OCR (text recognition), image description, and queries about image content
- Automatic MIME type detection and size limits
- Integrated with the vision-LLM pipeline

---

## 🧠 Advanced Memory Architecture

Sofia uses a highly modular, multi-layered SQLite-backed memory architecture, decomposed into domain-specific components for maximum scalability and targeted retrieval:

- **Sessions Context:** Manages conversational history, rolling context windows, and channel-specific session isolation.
- **Semantic Knowledge Graph:** Structured facts, entities, and relationships stored as nodes and edges. Includes `MemoryConsolidator` for deduplication and `MemoryPruner` for strategic forgetting based on access usage records. Includes the `knowledge_graph` tool.
- **Freeform Notes:** Unstructured, indexed scratchpad notes for flexible, text-based memory recall.
- **Agent Reflections:** Stores structured post-task evaluations and meta-learning matrices for continuous self-improvement.
- **Observability and State:** Distributed tracing spans, goal tracking, scheduled jobs, and context-aware trigger management.
- **Strategic Forgetting:** Exponential decay scoring based on access recency, frequency, and relevance. Automatic pruning of stale or low-value memories.
- **Memory Consolidation:** Deduplication of overlapping facts and semantic merging of related knowledge nodes.

---

## 🌐 Broad AI Support / LLM Providers

Sofia supports 20+ AI providers with automatic fallback and quality ranking:

| Provider | Model Family | Notes |
|----------|-------------|-------|
| OpenAI | GPT-4o, GPT-4.1, o3, o4-mini | Full function calling support |
| Anthropic | Claude 4.5, Claude 4, Claude 3.5 | Extended thinking, vision |
| Google | Gemini 2.5 Pro, Gemini 2.5 Flash | Multimodal |
| DeepSeek | DeepSeek V3, DeepSeek R1 | Reasoning models |
| xAI | Grok 3, Grok 3 Mini | |
| GitHub | Copilot models | Via API |
| MiniMax | MiniMax-01 | |
| Moonshot | Kimi | |
| Qwen | Qwen 3, Qwen 2.5 | |
| ZAI | Zhipu AI | |
| Local | Ollama, LM Studio, LocalAI | Fully offline capable |

...and more. All providers share a unified interface with automatic retry, fallback chains, and quality scoring.

---

## 📚 Skill System and Self-Learning

Sofia includes an **Antigravity Kit** with 159 expert personas covering:

- **Development:** Backend, frontend, DevOps, database, security
- **Analysis:** Data science, research, code review, penetration testing
- **Creative:** Writing, design, brainstorming
- **Operations:** System administration, monitoring, incident response

Skills are loaded lazily with 4-tier priority and can be created, refined, and improved automatically through the evolution engine.

---

## 🔄 Evolution and Self-Improvement

Sofia's `EvolutionEngine` implements a 7-phase continuous improvement loop:

1. **Observe** — Collect performance metrics and outcomes
2. **Diagnose** — Identify bottlenecks and failure patterns
3. **Plan** — Generate targeted improvement strategies
4. **Act** — Apply changes to prompts, skills, or agent configurations
5. **Verify** — Evaluate changes against benchmarks
6. **Consolidate** — Commit successful improvements
7. **Improve** — Refine the improvement process itself (meta-learning)

The `SafeModifier` enforces 3 layers of safety before any self-modification: validation, preview, and rollback.

---

## 🛡️ Guardrails and Safety

Sofia takes safety seriously with multiple defense layers:

- **Prompt Injection Defense:** 35+ detection patterns across 6 languages
- **PII Detection:** Luhn algorithm for credit cards, RFC1918 for private IPs
- **Secret Scrubbing:** Inbound and outbound — API keys, tokens, passwords never leak
- **Approval Gates:** High-risk operations require explicit user confirmation with audit trail
- **Budget Management:** Token and execution limits to prevent runaway costs
- **AES-256-GCM Encryption:** Sensitive data encrypted at rest

---

## 💬 Gateway Mode and Chat Platforms

Sofia connects to multiple chat platforms through a unified gateway:

```bash
./build/sofia gateway    # Starts web UI + all configured channels
```

| Platform | Features |
|----------|----------|
| **Web UI** | Real-time chat, file uploads, model switching, CRT-themed brutalist design |
| **Telegram** | Full bot with file sending, voice transcription |
| **Discord** | Markdown formatting, DM pairing, file sharing |
| **CLI** | Terminal-based interaction with all capabilities |

All platforms share the same memory, agents, and tools.

---

## 🎨 Web UI

Sofia includes a built-in web interface with a distinctive brutalist CRT-themed design:

- Real-time chat with streaming responses
- File uploads and image analysis
- Model selection and provider switching
- Memory browser and knowledge graph viewer
- Agent management and skill browsing
- Health monitoring and metrics dashboard

Access at `http://127.0.0.1:18795` after running `sofia gateway`.

---

## 🏗️ Architecture

```
+-------------------------------------------------------------+
|                      Sofia Gateway                           |
|  +-----------+  +-----------+  +-----------+  +----------+  |
|  |  Web UI   |  | Telegram  |  |  Discord  |  |   CLI    |  |
|  +-----+-----+  +-----+-----+  +-----+-----+  +-----+----+  |
|        +----------------+------------------+               |
|                  |           |                                |
|          +-------+-----------+-------+                       |
|          |      Agent Router         |                       |
|          +-----------+---------------+                       |
|                      |                                       |
|  +-------------------+--------------------+                  |
|  |              Core Engine               |                  |
|  |  +-----------+  +-----------+  +-----------+             |
|  |  | Planning  |  |  Memory   |  | Evolution  |             |
|  |  |  Engine   |  |  Manager  |  |  Engine    |             |
|  |  +-----------+  +-----------+  +-----------+             |
|  |  +-----------+  +-----------+  +-----------+             |
|  |  |   Tool    |  |   Skill   |  |  Safety &  |             |
|  |  | Registry  |  |  System   |  | Guardrails |             |
|  |  +-----------+  +-----------+  +-----------+             |
|  +----------------------------------------+                  |
|                      |                                       |
|  +-------------------+--------------------+                  |
|  |          LLM Provider Layer            |                  |
|  |  OpenAI | Anthropic | Gemini | DeepSeek | ...            |
|  |  (20+ providers with automatic fallback) |                  |
|  +-----------------------------------------+                  |
+-------------------------------------------------------------+
```

---

## 📦 Installation

### From Source (Recommended)

```bash
git clone https://github.com/grasberg/sofia.git
cd sofia
make deps && make build
./build/sofia onboard   # Initialize config and workspace
```

**Prerequisites:** Go 1.26+ ([download](https://go.dev/dl/))

### Build Options

```bash
make build          # Standard build
make build-all      # Build for all platforms
make test           # Run tests
make lint           # Run linter
make deps           # Install dependencies
```

---

## 🤝 Contributing

We welcome contributions! See [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines.

**Ways to contribute:**

- [Report bugs](https://github.com/grasberg/sofia/issues/new?template=bug_report.md)
- [Request features](https://github.com/grasberg/sofia/issues/new?template=feature_request.md)
- Submit pull requests
- Improve documentation
- Star the repo to show support

---

## 📄 License

Sofia is released under the [MIT License](LICENSE).

---

<div align="center">

**[Back to Top](#sofia)**

Built with love by [Magnus Grasberg](https://github.com/grasberg) and contributors.

</div>