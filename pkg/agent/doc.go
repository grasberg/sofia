// Package agent implements the Sofia agent execution system.
//
// # Architecture Overview
//
// The agent package follows a flat-file organisation with namespaced prefixes.
// Files are grouped by domain:
//
// ## Core Loop (loop_*.go)
//   - loop.go            — AgentLoop struct, NewAgentLoop, Run/Stop lifecycle
//   - loop_commands.go   — Slash-command parsing and dispatch (/session, /persona, /role…)
//   - loop_llm.go        — LLM iteration: sending messages, receiving tool calls
//   - loop_processing.go — Direct/spawned task processing, presence, reputation
//   - loop_tools.go      — Tool registration for shared + per-agent tools
//   - loop_helpers.go    — Autonomy services, context updates, feedback learning
//   - loop_guardrails.go — Input validation, output filtering, prompt injection defence
//   - loop_summarize.go  — Context window compression and summarisation
//   - loop_query.go      — Read-only AgentLoop accessors (GetStartupInfo, DashboardHub…)
//
// ## Agent Instance (instance.go)
//   - instance.go        — AgentInstance struct: per-conversation state
//   - context.go         — Context window management and message truncation
//
// ## Memory System (memory_*.go)
//   - memory.go              — AgentLoop memory accessors (notes, semantic nodes)
//   - memory_consolidation.go — Merging duplicate nodes, resolving conflicts
//   - memory_forgetting.go   — Stale node pruning and forgetting strategies
//   - memory_quality.go      — Node quality scoring and ranking
//   - semantic_memory.go     — High-level semantic graph operations
//
// ## Reflection & Learning (reflection_*.go)
//   - reflection.go         — Post-task self-evaluation
//   - reflection_scoring.go — Score calculation for reflections
//   - prompt_optimizer.go   — Prompt self-optimisation from past reflections
//
// ## Multi-Agent (orchestration.go, delegation.go, a2a.go, elevated.go)
//   - orchestration.go  — Task spawning and multi-agent orchestration
//   - delegation.go     — Agent-to-agent task delegation
//   - a2a.go            — Agent-to-agent (A2A) protocol support
//   - elevated.go       — Elevated (privileged) agent execution
//
// ## Evaluation & Monitoring
//   - evaluation_loop.go    — Continuous evaluation loop
//   - doom_loop.go          — Doom loop (infinite repetition) detection and recovery
//   - capabilities.go       — Dynamic agent capability discovery
//
// ## Configuration
//   - registry.go       — Agent registry and configuration lookup
//   - roles.go          — Role-based agent behaviour
//   - persona.go        — Persona configuration
//   - templates.go      — Agent prompt templates
//   - thinking.go       — Extended thinking / reasoning configuration
//   - usage.go          — Token usage tracking and budget enforcement
//   - pricing.go        — LLM pricing tables
//   - suggestions.go    — Follow-up question suggestions
//   - approval.go       — Human-in-the-loop approval gate
package agent
