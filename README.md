# Sofia - AI Workspace Assistant 🧠✨

![Version](https://img.shields.io/badge/version-v0.0.145-blue)
Sofia is an advanced, context-aware AI assistant and multi-agent orchestrator written in Go. Designed to function as a full-stack developer, system architect, and project manager. By integrating directly into the local development environment, Sofia can read/write files, execute terminal commands, schedule tasks, and delegate work to specialized sub-agents.

## ✨ Key Features

*   🛠️ **Autonomous Tool Use:** Can register domain names, publish web pages, read/edit files, run bash commands, and manage Google Services (Gmail/Calendar).
*   🧠 **Advanced Memory:** Tiered memory — short-term, long-term, episodic, and semantic (knowledge graph). Automatic consolidation and strategic forgetting keep memory efficient.
*   🤖 **Multi-Agent Orchestration:** Delegate tasks to parallel agents. Includes **AgentArchitect** for autonomous creation and optimization of new agents "on the fly".
*   🌐 **Broad AI Support:** Built-in support for 20+ AI providers including OpenAI, Anthropic (Claude 4.5), Gemini, DeepSeek, Grok, MiniMax, Moonshot, Qwen, Zai, GitHub Copilot, and more.
*   📚 **Skill System with Self-Learning:** Antigravity Kit with expert personas, plus automatic skill creation, refinement, and knowledge distillation.
*   🔄 **Evolution & Self-Improvement:** 5-phase `EvolutionEngine`, post-task evaluation, prompt self-optimization, and code self-modification for continuous learning and adaptation.
*   🔧 **Smart Tool Management:** Semantic tool matching via embeddings, performance tracking, and dynamic tool composition (pipelines) to create new macro tools.
*   ⚡ **Lightning-Fast Execution:** Caching of tool definitions, prompts, and connection pooling for extremely low latency in the hot path.
*   🎯 **Autonomy & Proactivity:** Long-term goals, context-aware triggers, proactive suggestions, and self-initiated research without user interaction.
*   🛡️ **Guardrails & Security:** Input validation, budget management (tokens/executions), prompt injection defense, and action confirmation for high-risk operations.
*   🔌 **MCP Client:** Model Context Protocol support for hooking into external MCP servers and tools.
*   💬 **Gateway Mode:** Built-in support for chat platforms like Telegram and Discord via `sofia gateway`.
*   🖥️ **Computer Use:** Autonomous computer control via screenshots and vision-LLM — control mouse and keyboard on macOS and Linux.
*   🌍 **Browser Automation (Playwright):** Autonomous web browsing with clicks, form filling, screenshots, and text extraction.
*   📸 **Image Analysis:** Analyze local images (PNG, JPEG, GIF, WebP) via vision-LLM — OCR, descriptions, and queries.
*   📋 **Plan & Execute:** Structured task planning with step-by-step tracking.
*   📝 **Shared Scratchpad:** Key-value storage for agent-to-agent communication.
*   ⏰ **Cron Scheduling:** The agent can independently create, list, delete, and schedule recurring tasks.
*   🔄 **Provider Fallback:** Automatic fallback chains if an AI provider fails.
*   🎨 **Modern Web UI (HTMX):** Brutalist design theme with CRT effects, real-time updates, and file uploads directly in chat.

## 📊 Agentic AI Capability Scorecard

> Evaluated by automated code analysis across 12 capability dimensions. Each score reflects the depth, robustness, and sophistication of the actual implementation.

| Category | Score | Highlights |
|:---------|:-----:|:-----------|
| **Multi-Agent Orchestration** | 9/10 | Dependency-aware topological scheduling, LLM-powered semantic delegation, dynamic agent auto-spawning, A2A mailbox messaging with broadcast |
| **Tool Use & Execution** | 9/10 | 40+ tools (file, shell, web, browser, hardware, MCP), embeddings-based semantic filtering, parallel execution with path-overlap safety, circuit breaker |
| **Context & Memory** | 9/10 | Knowledge graph with weighted relations, 4-layer memory (long-term, daily, graph, reflections), strategic forgetting with exponential decay, prompt cache optimization |
| **Planning & Reasoning** | 9/10 | Hierarchical plans with sub-plans, doom loop detection (4 signals, graduated recovery), evaluation loop with scored retries, auto-checkpoint/rollback |
| **Safety & Guardrails** | 9/10 | 35+ prompt injection patterns (6 languages), PII detection with Luhn/RFC1918 validation, inbound+outbound secret scrubbing, approval gates with audit trail, AES-256-GCM encryption |
| **Provider Abstraction** | 9/10 | 20+ LLM providers, fallback chains with 40+ error patterns, exponential backoff cooldown, Bayesian quality ranking, retry with jitter |
| **Channel Integration** | 8/10 | Telegram, Discord, Email, Web, CLI with shared retry logic, file sending (images/documents), voice transcription, Discord markdown formatting, DM pairing |
| **Observability & Ops** | 9/10 | SQLite audit logging, distributed tracing with span scoring, `/health` + `/ready` + `/metrics` endpoints, budget persistence, real-time WebSocket dashboard |
| **Self-Improvement** | 8/10 | 7-phase evolution loop (observe-diagnose-plan-act-verify-consolidate-improve), SafeModifier with 3 safety layers, performance-driven agent retirement, skill auto-improvement |
| **Skills & Extensibility** | 8/10 | 126 skills + 40 agent templates, 4-tier priority loading, ClawHub remote marketplace, trigram search cache, lazy loading with XML summaries |
| **Evaluation & Testing** | 8/10 | Agent execution harness with parallel runs, LLM-as-judge scoring, A/B comparison with bootstrap confidence intervals, 5 built-in benchmark suites, SQLite persistence with trend detection |
| **Scheduling & Automation** | 8/10 | 3 schedule types (at/every/cron) with context cancellation, heartbeat with active hours/days, goal-driven autonomy (multi-step), context triggers, proactive suggestions |
| | **Avg: 8.6** | |

## 📂 Workspace Structure

Sofia's configuration and workspace are located under `~/.sofia/`:

```text
~/.sofia/
├── config.json            # Main configuration (models, channels, settings)
├── memory.db              # Shared SQLite database for memory and session history
└── workspace/             # Sofia's workspace
    ├── IDENTITY.md        # Core identity: tone, role, and how Sofia presents herself
    ├── SOUL.md            # Core principles: behavior, values, and decision style
    ├── AGENT.md           # Agent-specific system prompt
    ├── USER.md            # User context and preferences
    ├── agents/            # Specialized agents (20 base agents included, Pixel Agents, etc.)
    │   ├── backend-specialist.md
    │   ├── frontend-specialist.md
    │   └── ...
    ├── skills/            # Local skills/expert personas (159 skills included)
    │   ├── github/
    │   ├── hardware/
    │   ├── skill-creator/
    │   └── ...
    ├── cron/              # Scheduled jobs (jobs.json)
    └── state/             # Persistent runtime state
```

## 🚀 Installation & Getting Started

### Prerequisites

Before building from source, you need to have **Go installed** (recommended: Go 1.26 or later). You can download Go from [go.dev/dl](https://go.dev/dl/).

### Install from Source

```bash
git clone https://github.com/grasberg/sofia.git
cd sofia
make deps
make build
```

The compiled binary will be placed directly in the project root directory as `./build/sofia`.

### Quick Start

1. **Initialize configuration and workspace:**
```bash
./build/sofia onboard
```

2. **Start Gateway (for chat/web interface):**
```bash
./build/sofia gateway
```

3. **Open Sofia's Control Panel:**
Navigate to `http://127.0.0.1:18795` in your browser. Go to the **Models** tab to add your provider and API key.

## 🤖 Multi-Agent Orchestration

Sofia can delegate and coordinate work across multiple agents:

*   **Orchestrate tool:** Define a set of subtasks with dependencies — independent tasks run in parallel, dependent tasks in the correct order. Automatic agent selection based on scoring.
*   **AgentArchitect & Sub-Agents:** Autonomous design and provisioning of specialized agents "on the fly" when problems are identified, as well as dedicated background and synchronous agents (comes with 20 base agents).
*   **A2A Protocol (Agent-to-Agent):** Standardized inter-agent communication with mailbox-based routing, send/receive/broadcast, and pending-polling.
*   **Shared Scratchpad:** Agents can share data via a key-value store namespaced per task group.
*   **Plan & Execute:** Create structured plans with steps that can be tracked and updated during execution.

## 🖥️ Computer Use

Sofia can control your computer autonomously via screenshots and vision-LLM:

*   Takes screenshots of the desktop and analyzes them using vision-LLM.
*   Performs mouse clicks, keystrokes, scrolling, and text input.
*   Loops until the task is complete or the maximum number of steps is reached.
*   **Platforms:** macOS (screencapture + osascript) and Linux (scrot + xdotool).

## 🌍 Browser Automation (Playwright)

Sofia has built-in Playwright integration for autonomous web browsing:

*   Navigate to URLs, click elements, fill out forms.
*   Take screenshots, extract text, and run JavaScript.
*   Wait for elements, handle timeouts, and scroll.
*   Supports Chromium, Firefox, and WebKit.
*   Headless and headful modes.

## 📸 Image Analysis

Analyze local images directly in the conversation:

*   Support for PNG, JPEG, GIF, and WebP.
*   OCR (text recognition), image description, and queries about image content.
*   Automatic MIME type detection and size limits.
*   Integrated with the vision-LLM pipeline.

## 🧠 Advanced Memory Architecture

Sofia uses a highly modular, multi-layered SQLite-backed memory architecture, decomposed into domain-specific components for maximum scalability and targeted retrieval:

*   **Sessions Context (`db_sessions.go`):** Manages conversational history, rolling context windows, and channel-specific session isolation.
*   **Semantic Knowledge Graph (`db_semantic_*.go`):** Structured facts, entities, and relationships stored as nodes and edges. Includes `MemoryConsolidator` for deduplication and `MemoryPruner` for strategic forgetting based on access usage records (`RecordStat`). Includes the `knowledge_graph` tool.
*   **Freeform Notes (`db_notes.go`):** Unstructured, indexed scratchpad notes for flexible, text-based memory recall.
*   **Agent Reflections (`db_reflections.go`):** Stores structured post-task evaluations and meta-learning matrices for continuous self-improvement.
*   **Observability & State (`db_traces.go`, `db_checkpoints.go`):** Records execution traces (LLM spans, tool usage) for distributed observability, alongside agent loop checkpoints for safe rollback capabilities.
*   **Autonomous Goals (`db_goals.go`):** Dedicated persistence layer for long-term objectives across sessions.

## 🔄 Self-Reflection & Self-Improvement

Sofia evaluates herself after every task and improves continuously:

*   **Post-Task Reflection:** `ReflectionEngine` runs an LLM-driven evaluation after each task: what worked, what failed, lessons learned, and meta-learning.
*   **Performance Score:** `PerformanceScorer` calculates a 0.0–1.0 rating based on error rates, tool efficiency, and completion.
*   **Trend Analysis:** `GetPerformanceTrend` compares recent vs older reflections to detect improvement or decline.
*   **Prompt Self-Optimization:** `optimizePrompt` automatically adjusts system instructions based on poor performance results.
*   **Meta-Learning:** Each reflection includes a `meta_learning` field that stores insights about the learning process itself.
*   **Code Self-Modification:** The `self_modify` tool allows Sofia to safely modify her own code with confirmation hashes and an audit trail.
*   **EvolutionEngine:** An advanced, 5-phase engine for autonomous self-development that analyzes performance, updates agents, and generates changelogs (triggered via the `/evolve` command).

## 🎯 Autonomy & Proactivity

Sofia can act independently without user initiation:

*   **Long-Term Goals System:** Sofia autonomously pursues complex, multi-step objectives across sessions using a robust Goal Engine via the `manage_goals` tool.
    *   **Phased Lifecycle:** Goals transition through structured phases: `Specify` (defining parameters), `Plan` (breaking down tasks), `Implement` (executing work), and `Completed`.
    *   **Rigorous Specifications:** Active goals retain a structural `GoalSpec`, outlining strict requirements, success criteria, context, and operational constraints.
    *   **Structured Results:** Upon completion, a `GoalResult` captures the outcome summary, produced artifacts, actionable next steps, unmet criteria, and evaluation evidence.
    *   **Automatic Injection:** Active, high-priority goals are dynamically inserted into the agent's runtime context for immediate focus.
*   **Context-Aware Triggers:** The `manage_triggers` tool creates conditional actions that trigger based on user conversational context.
*   **Proactive Suggestions:** `AutonomyService` periodically analyzes recent activity and generates unsolicited suggestions when deemed valuable.
*   **Autonomous Research:** Identifies knowledge gaps and independently initiates research on relevant topics.

## 🔧 Tool Use & Discovery

Sofia has advanced logic to manage and optimize her tool usage:

*   **Semantic Tool Matching:** Uses embeddings to filter out the most relevant tools based on user intent. This reduces token usage and increases the LLM's focus.
*   **Tool Performance Tracking:** `ToolTracker` automatically measures success rates and execution times for all tools. Sofia can use `get_tool_stats` to see which tools perform best for specific tasks.
*   **Tool Composition (Pipelines):** With `create_pipeline`, Sofia can chain multiple tools into a new, reusable macro tool. Data flows automatically between pipeline steps.
*   **MCP Support:** Dynamic discovery of tools via Model Context Protocol servers.

## 📚 Skill System with Self-Learning

Sofia can create and improve her own skills:

*   **Extensive Autonomous Library:** 159 pre-installed autonomous "self-directed" skills where Sofia executes large-scale tasks from start to finish.
*   **Auto-Skill Creation:** `create_skill` generates new skills automatically from successful approaches.
*   **Skill Refinement:** `update_skill` improves existing skills based on usage feedback.
*   **Knowledge Distillation:** `distill_knowledge` compresses learned experiences into reusable knowledge.

## 🔌 MCP Support (Model Context Protocol)

Sofia features a built-in MCP client to connect to external MCP servers:

*   Connect to external tool and data sources via a standardized protocol.
*   MCP tools are dynamically exposed in the agent's tool registry.
*   Configure MCP servers via `config.json`.

## 🔒 Guardrails & Security Model

Sofia utilizes a comprehensive, multi-layered security system:

*   **Workspace Restriction:** File and command tools are strictly sandboxed to the configured workspace path.
*   **Input Validation:** Configurable maximum length and deny patterns to block malicious prompt injections.
*   **Output Filtering:** Filters sensitive data (PII, secrets) from responses before they are returned.
*   **Prompt Injection Defense:** LLM-based detection and blocking of prompt injection attempts with configurable actions (block/warn).
*   **Action Confirmation:** The `self_modify` tool requires hash confirmation before executing high-risk changes.
*   **Audit Trail:** All self-modifications are logged with timestamps in `self_modifications.log`.
*   **Budget Management:** A strict policy for token and execution budgets is applied to stop agents that would otherwise drain resources.
*   **Daemons:** Isolated background processes are managed securely and respect system restrictions.

**Via Web UI:**
1.  Open Sofia's Web UI → **System**.
2.  Click on the **Security** tab.
3.  Enable **Restrict to Workspace** and configure guardrails.
4.  Settings are saved automatically.

## 💓 Heartbeat (Background Agent)

Sofia can automatically perform tasks in the background based on a schedule.

**Via Web UI:**
1.  Open Sofia's Web UI → **System**.
2.  Click on the **Heartbeat** tab.
3.  Enable **Enable Heartbeat** and specify how often the agent should run (in minutes).
4.  Enter **Active Hours** in the format `09:00-17:00` — leave blank for 24/7.
5.  Select **Active Days** — leave blank to run every day.
6.  Settings are saved automatically.

## 🧭 Customizing Sofia's Personality

Sofia's behavior, tone, and personality are controlled by two files: **IDENTITY.md** and **SOUL.md**. You can easily edit them directly in the web interface:

1.  **Start Sofia:** `sofia gateway`
2.  **Open browser:** Navigate to `http://127.0.0.1:18795`
3.  **Go to System** in the left menu.
4.  Edit **IDENTITY.md** (who Sofia is) and **SOUL.md** (how Sofia behaves) directly in the text boxes under the **Prompts** tab.
5.  Click **Save prompt files** — the changes take effect immediately without restarting.

### `IDENTITY.md` — Who is Sofia?
Defines Sofia's role, name, and foundational context. Example:
```md
# Identity
- Name: Sofia
- Role: Personal AI assistant
- Running: 24/7 on the user's own hardware
```

### `SOUL.md` — How does Sofia behave?
Defines personality, language, values, and decision logic. Example:
```md
# Soul
- Svara alltid på svenska (Always respond in Swedish)
- Var proaktiv och självgående (Be proactive and self-driven)
- Använd torr humor och driv (Use dry humor and drive)
- Prioritera handling framför att fråga om lov (Prioritize action over asking for permission)
```

> 💡 **Tip:** You can give Sofia any personality you want — formal, relaxed, sarcastic, educational, or completely tailored to your workflow.

## 🎨 Web UI

Sofia's web interface is built with **HTMX** and **Go Templates**, featuring a unique brutalist design theme with CRT effects:

*   **Chat:** Real-time conversation with streaming, markdown rendering, and file uploads (including image uploads for vision models).
*   **Chat History:** Search, browse, and resume previous conversations with full session management.
*   **Memory Explorer:** Interactive, graphical exploration of Sofia's semantic memory and knowledge nodes.
*   **Goals Kanban:** An integrated Kanban board to track system goals with drag-and-drop.
*   **Agents & Pixel Agents:** Visual live overview and performance management of all your agents and the entirely new "Pixel Agents".
*   **Monitor:** Real-time monitoring of agent activity, tool calls, system status, and ongoing goals (Activity Monitor).
*   **System (Settings Hub):** A comprehensive settings section to manage every aspect of Sofia's behavior, guardrails, and integrations:

    | Configuration | Description |
    | :--- | :--- |
    | **🎭 Identity & Personas** | Fine-tune Sofia's baseline characteristics and seamlessly manage all **20 distinct agent personas**. |
    | **🧬 Evolution & Autonomy** | Control the `EvolutionEngine`. Set long-term goals and instantly dial proactive autonomy levels up or down. |
    | **💰 Budget limits** | Enforce strict real-time execution limits and token thresholds to guarantee zero runaway costs. |
    | **⚡ Triggers & Webhooks** | Design sophisticated, context-aware external webhook endpoints and conditional event listeners. |
    | **⏱️ Cron & Heartbeat** | Access a visual timeline of scheduled recurring background jobs alongside the core heartbeat interval. |
    | **🧠 Models & Intelligence** | Connect 20+ AI providers, set default models, and configure smart routing for different agent logic. |
    | **🔊 Channels & TTS** | Configure chat links to Telegram, Discord, and activate advanced human-like Text-to-Speech (TTS) options. |
    | **🔌 Integrations** | Extend Sofia's reach outward: GitHub, Google, Porkbun, cPanel, local HD Bitcoin Wallets, and more. |
    | **🛠️ Tools & Skills** | Oversee and configure her expansive arsenal: the **159 autonomous skills** and dozens of system tools. |
    | **🔐 Remote & Security** | Institute iron-clad workspace restrictions, input/output guardrails, and strict remote API access controls. |
    | **📋 Logs** | Observe raw, real-time application and network logs to monitor all background logic. |

## 🔄 AI Providers

Sofia supports all providers via an OpenAI-compatible API interface:

| Provider | Support |
|---|---|
| OpenAI | ✅ |
| Anthropic | ✅ |
| Google Gemini  | ✅ |
| DeepSeek | ✅ |
| Grok  | ✅ |
| MiniMax | ✅ |
| Moonshot | ✅ |
| Qwen | ✅ |
| Zai | ✅ |
| GitHub Copilot | ✅ |
| Groq | ✅ |
| OpenRouter | ✅ |
| Mistral AI | ✅ |

**Provider Fallback:** Configure fallback chains so that Sofia automatically switches to the next provider if the primary one fails.

## 🔌 Integrations

To give Sofia full power, you can connect her with external services.

### 📧 Google (Gmail & Calendar)

Sofia uses `gogcli` to interact with Google Services.

1.  **Install gogcli:** Make sure `gog` is in your PATH.
2.  **Authenticate:** Run the following in the terminal and follow the instructions:
    ```bash
    gog login your.email@gmail.com
    ```
3.  **Enable in Sofia:**
    -   Open Sofia's Web UI -> **System** -> **Integrations**.
    -   Enable **Google CLI** and specify the path to `gog`.
    -   Configure allowed commands (gmail, calendar, drive).
    -   Save the settings.

### 🐙 GitHub

Sofia uses GitHub CLI (`gh`) to manage repos, PRs, and code.

1.  **Install GitHub CLI:** `brew install gh` (macOS) or visit [cli.github.com](https://cli.github.com).
2.  **Authenticate:** Run the following in the terminal and follow the instructions:
    ```bash
    gh auth login
    ```
3.  **Enable in Sofia:**
    -   Open Sofia's Web UI -> **System** -> **Integrations**.
    -   Toggle the **GitHub CLI** switch and click **Save settings**.
    -   **Restart Sofia** after saving.

Sofia can now manage PRs, issues, repos, workflows, and more via the `github_cli` tool.

4.  **Git Identity:** Ensure your local git is configured so Sofia can commit in your name:
    ```bash
    git config --global user.name "Your Name"
    git config --global user.email "your.email@example.com"
    ```

### 💬 Telegram

Sofia can be linked to Telegram and answer messages directly in chat.

**Via Web UI (recommended):**
1.  Create a bot via [BotFather](https://t.me/BotFather) on Telegram. Run `/newbot` and follow the instructions.
2.  Copy the bot token provided by BotFather.
3.  Open Sofia's Web UI → **Channels**.
4.  Enable **Telegram**, paste your bot token.
5.  Under **Allow From**, you can restrict which Telegram users are allowed to talk to Sofia (optional, leave blank for everyone).
6.  Click **Save Settings** and restart Sofia.

> 💡 **Tip:** If you are running Sofia behind a firewall or VPN, you can enter a proxy under the **Proxy** field on the Channels page.

### 🎮 Discord

Sofia can also be active in Discord servers and DMs.

**Via Web UI (recommended):**
1.  Go to the [Discord Developer Portal](https://discord.com/developers/applications) and create a new application.
2.  Under **Bot** → click **Add Bot** → copy your **Bot Token**.
3.  Under **OAuth2 → URL Generator** — select the `bot` scope and give it permissions to read/send messages. Invite the bot to your server via the generated link.
4.  Open Sofia's Web UI → **Channels**.
5.  Enable **Discord**, paste your bot token.
6.  **Allow From** — specify Discord usernames permitted to interact with Sofia (optional).
7.  **Mention Only** — if enabled, Sofia only answers when @-mentioned; otherwise, she answers all messages in channels she has access to.
8.  Click **Save Settings** and restart Sofia.

> 💡 **Tip:** Set `mention_only` to `true` if Sofia is in an active channel with many users — otherwise, she will reply to everything.

### 🐷 Porkbun (Domain Management)

Sofia can check availability, register domains, and manage DNS records via the Porkbun API.

1.  **Get API Keys:** Log in to [Porkbun](https://porkbun.com/account/api) and generate an "API Key" and "Secret API Key".
2.  **Configure in Sofia:**
    -   Open Sofia's Web UI -> **System** -> **Integrations**.
    -   Enable **Porkbun** and paste your `API Key` and `Secret API Key`.
    -   Save the settings.

### 📦 cPanel (Web Hosting)

Sofia can manage your web hosting account via cPanel UAPI: upload files, create databases, and manage domains.

1.  **Create API Token:** Log in to cPanel -> **Security** -> **Manage API Tokens**. Create a new token with the privileges you want Sofia to have.
2.  **Configure in Sofia:**
    -   Open Sofia's Web UI -> **System** -> **Integrations**.
    -   Enable **cPanel** and enter the host, username, and your API token.
    -   Save the settings.

### ₿ Bitcoin Wallet

Sofia integrates directly with the Bitcoin blockchain, allowing for both public queries and full HD wallet management without relying on a local daemon.

1.  **Public Queries:** Query balances, transactions, and UTXOs seamlessly using the Mempool.space API.
2.  **Wallet Operations:** Create, import, and manage a local BIP84 HD wallet to safely generate addresses and send transactions locally. The wallet is encrypted via a passphrase.
3.  **Configure in Sofia:**
    -   Open Sofia's Web UI -> **System** -> **Integrations**.
    -   Enable **Bitcoin** and configure the passphrase and network (Mainnet, Testnet, Signet).
    -   Save the settings.

## 🛠️ Complete Tool List

| Tool | Description |
|---|---|
| `file_read` / `file_write` / `file_edit` | Read, write, and edit files |
| `shell` | Run terminal commands |
| `web_browse` | Autonomous web browsing via Playwright |
| `computer_use` | Control the computer's screen, mouse, and keyboard |
| `image_analyze` | Analyze local images via vision-LLM |
| `orchestrate` | Multi-agent orchestration with dependency graphs |
| `spawn` / `subagent` | Launch asynchronous/synchronous sub-agents |
| `a2a` | Agent-to-Agent communication (send/receive/broadcast) |
| `plan` | Structured task planning |
| `scratchpad` | Shared key-value store between agents |
| `cron` | Create and manage scheduled jobs |
| `message` | Send messages to chat channels |
| `gogcli` | Google Gmail, Calendar, and Drive |
| `knowledge_graph` | Knowledge graph — add, search, and delete facts and relations |
| `manage_goals` | Create, update, and track long-term goals |
| `manage_triggers` | Create context-aware triggers for conditional actions |
| `create_skill` | Automatically create new skills from successful approaches |
| `update_skill` | Refine existing skills based on feedback |
| `distill_knowledge` | Distill experiences into reusable knowledge |
| `self_modify` | Code/configuration self-modification with safety guardrails |
| `notify_user` | Push notifications to the user's desktop |
| `get_tool_stats` | Fetch performance data and success rates for tools |
| `create_pipeline` | Create a new macro tool by chaining existing tools |
| `mcp` | Connect to external MCP servers for dynamic tools |
| `domain_name` | Manage domains via Porkbun (check, register, dns, nameservers) |
| `cpanel` | Manage cPanel web hosting (files, domains, databases, SSL) |
| `bitcoin` | Bitcoin integration: local HD wallet management, UTXOs, send transactions, and public queries |


---
*Built to accelerate development. Your local AI colleague.*
