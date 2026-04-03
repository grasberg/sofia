---
name: context-engineering
description: Optimize LLM context construction, memory management, and multi-agent prompt design. Use when building system prompts, managing conversation memory, coordinating subagents, or when context is becoming too large and needs compression or restructuring.
---

# Context Engineering

Knowledge and strategies for managing LLM context effectively. Apply these principles when constructing prompts, managing memory, coordinating multi-agent work, or diagnosing degraded response quality.

## Core Principle

Context is the scarcest resource. Every token in the context window competes with every other token. A token of noise displaces a token of signal.

## Prompt Construction

### Information Placement

LLMs exhibit U-shaped attention — they attend most strongly to the beginning and end of the context window, with weaker attention in the middle.

- Place critical instructions and identity at the **start** (system prompt opening).
- Place the current task and user query at the **end** (most recent messages).
- Place reference material in the **middle** — it will be retrieved when relevant but won't dominate attention.

### Structured Formats

Use structured formats for machine-readable data passed to the model:

- **XML tags** for delineating sections (`<context>`, `<instructions>`, `<examples>`).
- **JSON** for structured data the model must parse or produce.
- **Markdown headers** for hierarchical organization of instructions.
- **Numbered lists** for sequential steps that must be followed in order.

Avoid walls of prose. The model extracts information more reliably from structured content.

### Prompt Compression Techniques

When context is approaching limits:

- Replace verbose explanations with concise examples.
- Summarize historical context instead of including raw transcripts.
- Use references to files instead of inlining file contents.
- Remove redundant information — say it once, in the right place.

## Memory Tiers

Sofia operates with three tiers of memory. Use the right tier for each type of information.

### Scratchpad (Session Memory)

- **Scope**: Current conversation only.
- **Use for**: Working state, intermediate results, current task context.
- **Behavior**: Grows with conversation, cleared on session end.
- **Cost**: Directly consumes context window tokens.

### Knowledge Graph (Persistent Memory)

- **Scope**: Persists across sessions via `memory.db`.
- **Use for**: User preferences, learned facts, correction history, relationship maps.
- **Behavior**: Loaded into system prompt on session start.
- **Cost**: Fixed overhead per session — keep entries concise.

### Workspace Files (Durable Storage)

- **Scope**: Persists on disk indefinitely.
- **Use for**: Large reference documents, project state, generated artifacts.
- **Behavior**: Read on demand, not automatically loaded.
- **Cost**: Zero context cost until read.

**Decision rule**: If information is needed in every session, put it in persistent memory. If it is needed only sometimes, put it in a workspace file and reference it from memory. If it is only needed now, keep it in the session.

## Context Degradation Detection

Watch for these signs that context quality is degrading:

- The model contradicts earlier statements in the same conversation.
- The model "forgets" instructions given earlier in the prompt.
- Responses become generic or lose specificity about the current task.
- The model hallucinates file paths, function names, or API details.

### Recovery Strategies

1. **Summarize and restart**: Compress the conversation so far into a concise summary, start a new session with the summary as context.
2. **Prune irrelevant history**: Remove messages that are no longer relevant to the current task.
3. **Externalize state**: Write working state to a file, reference the file instead of carrying it in context.

## Multi-Agent Context Design

When building prompts for subagents:

### Context Isolation

Each subagent gets only the context it needs. Do not pass the full parent context.

- **Include**: The specific task, relevant file paths, code patterns to follow, expected output format.
- **Exclude**: Unrelated conversation history, other agents' state, full system prompt.

### Subagent Prompt Template

```
Task: <specific, actionable description>
Working directory: <absolute path>
Files to read: <list of relevant files>
Patterns to follow: <code style, naming conventions, etc.>
Output format: <what to return and how>
Constraints: <what NOT to do>
```

### Coordination Patterns

- **Fan-out**: Split a task into independent subtasks, assign each to a subagent, merge results.
- **Pipeline**: Chain agents where each agent's output is the next agent's input.
- **Specialist delegation**: Route specific domains (e.g., testing, documentation) to purpose-built agents.

## Skills and Context Budget

- **Skills metadata** (name + description) is always loaded — keep descriptions precise and triggering-focused.
- **Skill bodies** are loaded on demand — they consume context only when activated.
- When multiple skills are active, monitor total context consumption.
- Prefer skills that reference external files over skills that inline large content blocks.

## Practical Guidelines

- Before adding information to context, ask: "Will this improve the model's next response?"
- Before inlining a file, ask: "Can I reference it instead and read it only if needed?"
- When context exceeds 60% of the window, proactively summarize or externalize.
- When building prompts for others: be specific. Vague prompts produce vague results.
