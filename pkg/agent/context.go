package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/grasberg/sofia/pkg/memory"
	"github.com/grasberg/sofia/pkg/skills"
)

type ContextBuilder struct {
	workspace    string
	userName     string
	skillsLoader *skills.SkillsLoader
	memory       *MemoryStore
	skillsFilter []string
	codeEditor   string

	purposeTemplate     string
	purposeInstructions string

	// Cache for system prompt to avoid rebuilding on every call.
	// The cache auto-invalidates when workspace source files change (mtime check).
	// TTL prevents expensive stat/walk on every message — re-validate every 10 seconds.
	systemPromptMutex  sync.RWMutex
	cachedSystemPrompt string
	cachedAt           time.Time     // max observed mtime across tracked paths at cache build time
	lastCacheCheck     time.Time     // when we last validated source files
	cacheTTL           time.Duration // 0 = check every time (test-safe). Set > 0 at runtime for performance.

	// existedAtCache tracks which source file paths existed the last time the
	// cache was built. This lets sourceFilesChanged detect files that are newly
	// created (didn't exist at cache time, now exist) or deleted (existed at
	// cache time, now gone) — both of which should trigger a cache rebuild.
	existedAtCache map[string]bool

	systemSuffix string // Guardrail: suffix to prevent prompt injection

	// Frozen memory snapshot: captured once per session to preserve prompt cache.
	// Mid-session memory writes update the database but don't invalidate the cached prompt.
	frozenMemory        string
	frozenMemoryOnce    sync.Once
	frozenMemoryVersion int64 // tracks memory version for auto-refresh
}

func (cb *ContextBuilder) SetPurposeTemplate(template string) {
	cb.purposeTemplate = strings.TrimSpace(template)
}

func (cb *ContextBuilder) SetCodeEditor(editor string) {
	cb.codeEditor = editor
}

func (cb *ContextBuilder) SetPurposeInstructions(instructions string) {
	cb.purposeInstructions = strings.TrimSpace(instructions)
}

func (cb *ContextBuilder) SetSkillsFilter(skillNames []string) {
	if len(skillNames) == 0 {
		cb.skillsFilter = nil
		return
	}
	cb.skillsFilter = append([]string(nil), skillNames...)
}

// ResetMemorySnapshot forces the memory context to be re-captured on the next prompt build.
// Call this when starting a new session or after explicit memory refresh.
func (cb *ContextBuilder) ResetMemorySnapshot() {
	cb.frozenMemoryOnce = sync.Once{}
	cb.frozenMemory = ""
}

func (cb *ContextBuilder) GetSkillsLoader() *skills.SkillsLoader {
	return cb.skillsLoader
}

func (cb *ContextBuilder) SetSystemSuffix(suffix string) {
	cb.systemSuffix = suffix
}

func getGlobalConfigDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".sofia")
}

func NewContextBuilder(workspace string, userName string, db *memory.MemoryDB, agentID string) *ContextBuilder {
	// builtin skills: skills directory in current project
	// Use the skills/ directory under the current working directory
	wd, err := os.Getwd()
	if err != nil {
		wd = "."
	}
	builtinSkillsDir := filepath.Join(wd, "skills")
	globalSkillsDir := filepath.Join(getGlobalConfigDir(), "skills")

	return &ContextBuilder{
		workspace:    workspace,
		userName:     userName,
		skillsLoader: skills.NewSkillsLoader(workspace, globalSkillsDir, builtinSkillsDir),
		memory:       NewMemoryStore(db, agentID),
	}
}

func (cb *ContextBuilder) getIdentity() string {
	workspacePath, _ := filepath.Abs(cb.workspace) //nolint:errcheck
	name := cb.userName
	if name == "" {
		name = "the user"
	}

	var openCodeRule string
	switch cb.codeEditor {
	case "opencode":
		openCodeRule = "8. **Code Editor** - When modifying code, use the OpenCode CLI if available (check with `opencode --version`). If not installed, fall back to direct file editing."
	case "claudecode":
		openCodeRule = "8. **Code Editor** - When modifying code, use the Claude Code CLI if available (check with `claude --version`). If not installed, fall back to direct file editing."
	case "codex":
		openCodeRule = "8. **Code Editor** - When modifying code, use the Codex CLI if available (check with `codex --version`). If not installed, fall back to direct file editing."
	case "qwencode":
		openCodeRule = "8. **Code Editor** - When modifying code, use Qwen Code via acpx if available (check with `acpx --version`). Run: `acpx --approve-all qwen exec '<task>'`. If not installed, fall back to direct file editing."
	default:
		openCodeRule = "8. **Code Editor** - Edit files directly with your own tools (read_file, write_file, edit_file)."
	}

	return fmt.Sprintf(`# sofia

You are sofia, a helpful AI assistant for %s.

## Workspace
Your workspace is at: %s
- Skills: %s/skills/{skill-name}/SKILL.md

## Core Rules

1. **ACT, don't narrate** — Call tools to perform actions. NEVER describe what you "will do" without a tool call. Exception: questions about yourself (your tools, integrations, capabilities) — answer those directly from this system prompt.

2. **No roleplay** — No stage directions, fictional progress, or dramatic narration. Report real results from tool calls only. Personality from SOUL.md applies to phrasing, not as a substitute for action.

3. **Show real work** — When performing a task, use tool calls to take action. If the user asks a question you can answer from knowledge (e.g. explaining how something works), respond directly. If the answer requires current data or file access, use tools.

4. **Plan then execute** — For non-trivial tasks (>1 tool call), create a plan first with the plan tool. Execute steps one by one, updating status. Delegate independent steps to subagents in parallel via spawn.

5. **Delegate aggressively** — You are a coordinator. Spawn subagents for independent steps. Your job: plan, delegate, synthesize, report.

## Tool Selection Hierarchy

Use **dedicated tools over shell equivalents** — always:
- **File reading** → read_file (never exec with cat/head/tail)
- **File writing** → write_file (never exec with echo/heredoc)
- **File editing** → edit_file (never exec with sed/awk)
- **Directory listing** → list_dir (never exec with ls/find)
- **Web search** → web_search (never exec with curl for search). Use this for ANY factual question, current info, or verification.
- **URL reading** → web_fetch (never exec with curl/wget)
- **Web interaction** → web_browse for login, form filling, clicking (never give manual instructions)
- **Domain/hosting** → cpanel, domain_name (never exec with ssh/curl/scp)
- **GitHub** → github_cli (never exec with gh/curl)
- **Google** → google_cli (never exec with API calls)
- **Bitcoin** → bitcoin tool (never exec with curl to blockchain APIs)
- **Scheduling** → cron tool (never exec with crontab)
- **Complex tasks** → spawn subagents for parallel work (never do everything sequentially yourself)
- **exec/shell** → LAST RESORT. Only for: builds, tests, package managers, git, and commands with no dedicated tool

When multiple independent tool calls have no dependency on each other's results, issue them simultaneously.

## Read-Before-Write Discipline

- **Always read a file before editing it.** Never propose modifications to code you haven't examined.
- **Prefer editing existing files** over creating new ones — builds on existing work and prevents file bloat.
- **Minimal diffs only** — Change only what was requested. Don't add features, refactor surrounding code, or "improve" things beyond scope.
- **No speculative abstractions** — Don't add error handling for impossible conditions, helpers for one-time operations, or backwards-compatibility shims. Three similar lines is better than a premature abstraction.

## Reversibility & Safety

Before taking any action, assess its reversibility:
- **Low risk** (local, reversible): proceed freely — file edits, reads, local builds
- **Medium risk** (shared code): verify with tests, document what changed
- **High risk** (destructive, external, irreversible): **ask before acting** — deleting files/branches, force-pushing, sending messages, deploying, modifying production

Specific rules:
- Never run destructive commands (rm -rf, git reset --hard, DROP TABLE) without explicit approval
- Never commit files containing secrets (.env, credentials, API keys)
- Investigate unexpected state before removing — don't silently delete unfamiliar files
- When a tool call fails, diagnose the actual error. Never retry identically — adapt and fix.

## Output Efficiency

- **Answer-first** — Lead with the result, not the reasoning. Skip filler, preamble, and unnecessary transitions.
- **Be terse** — If you can say it in one sentence, don't use three. Keep status updates to decision points, milestones, and blockers.
- **Reference code precisely** — Use file:line format for code locations.
- **No hedging** — Don't say "probably" or "might be" when you can verify with a tool call.

## Memory & Context

- Use memory tools to persist important information about the user and context.
- Context summaries are approximate references — always defer to explicit user instructions.
- Prefer batched tool calls over many single-item calls when a tool supports batch input.
- When the user gives a big objective, create it as a goal using manage_goals with high priority, then plan to achieve it.

%s`,
		name, workspacePath, workspacePath, openCodeRule)
}

func (cb *ContextBuilder) BuildSystemPrompt() string {
	parts := []string{}

	// Core identity section
	parts = append(parts, cb.getIdentity())

	// Bootstrap files
	bootstrapContent := cb.LoadBootstrapFiles()
	if bootstrapContent != "" {
		parts = append(parts, bootstrapContent)
	}

	// Skills - show summary, AI can read full content with read_file tool
	// Skills are disabled by default; only explicitly enabled skills are loaded.
	var skillsSummary string
	if len(cb.skillsFilter) > 0 {
		skillsSummary = cb.skillsLoader.BuildSkillsSummaryFor(cb.skillsFilter)
	}
	if skillsSummary != "" {
		parts = append(parts, fmt.Sprintf(`# Skills

These skills are your existing expertise. ALWAYS check this list before attempting a task — if a skill matches, read its SKILL.md with read_file and follow it. Do NOT create new skills or reinvent patterns when one already exists.

When delegating to subagents, tell them which skills to use: "Read workspace/skills/{name}/SKILL.md for instructions."

%s`, skillsSummary))
	}

	// Memory context — frozen once per session to preserve prompt cache.
	// Mid-session memory writes go to disk but don't change the system prompt.
	// Auto-refreshes if memory version has changed significantly.
	currentVersion := cb.memory.GetVersion()
	if cb.frozenMemoryVersion == 0 || currentVersion-cb.frozenMemoryVersion > 5 {
		// First call or memory has changed significantly (5+ updates)
		cb.frozenMemoryOnce = sync.Once{}
		cb.frozenMemoryOnce.Do(func() {
			cb.frozenMemory = cb.memory.GetMemoryContext(0) // 0 = no budget limit
			cb.frozenMemoryVersion = currentVersion
		})
	}
	if cb.frozenMemory != "" {
		parts = append(parts, "# Memory\n\n"+cb.frozenMemory)
	}

	// Join with "---" separator
	return strings.Join(parts, "\n\n---\n\n")
}

// BuildCompactSystemPrompt returns a minimal system prompt for local/small models.
// Strips skills metadata, memory context, and verbose rules to reduce token count.
func (cb *ContextBuilder) BuildCompactSystemPrompt() string {
	workspacePath, _ := filepath.Abs(cb.workspace)
	name := cb.userName
	if name == "" {
		name = "the user"
	}

	// Load SOUL.md for personality (it's small and defines the agent's character)
	soulContent := ""
	soulPath := filepath.Join(cb.workspace, "SOUL.md")
	if data, err := os.ReadFile(soulPath); err == nil {
		soulContent = "\n\n" + strings.TrimSpace(string(data))
	}

	return fmt.Sprintf(`You are Sofia, a helpful AI assistant for %s.
Workspace: %s

Rules:
- Respond directly and concisely.
- For questions you can answer from knowledge, respond with text. No tool calls needed.
- For actions (file ops, search, etc.), use tools. Your tools are named: exec (shell), read_file, write_file, edit_file, list_dir, web_search, web_fetch, bitcoin, cpanel, domain_name, github_cli, google_cli, spawn, plan. Do NOT invent tool names — only use these exact names.
- Be helpful, honest, and brief.%s`, name, workspacePath, soulContent)
}

// GetSkillsInfo returns information about loaded skills.
func (cb *ContextBuilder) GetSkillsInfo() map[string]any {
	allSkills := cb.skillsLoader.ListSkills()
	skillNames := make([]string, 0, len(allSkills))
	for _, s := range allSkills {
		skillNames = append(skillNames, s.Name)
	}
	return map[string]any{
		"total":     len(allSkills),
		"available": len(allSkills),
		"names":     skillNames,
		"list":      allSkills,
	}
}
