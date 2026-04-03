package agent

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/grasberg/sofia/pkg/logger"
	"github.com/grasberg/sofia/pkg/memory"
	"github.com/grasberg/sofia/pkg/providers"
	"github.com/grasberg/sofia/pkg/skills"
)

type ContextBuilder struct {
	workspace    string
	userName     string
	skillsLoader *skills.SkillsLoader
	memory       *MemoryStore
	skillsFilter []string
	useOpenCode  bool

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
	frozenMemory     string
	frozenMemoryOnce sync.Once
}

func (cb *ContextBuilder) SetPurposeTemplate(template string) {
	cb.purposeTemplate = strings.TrimSpace(template)
}

func (cb *ContextBuilder) SetUseOpenCode(enabled bool) {
	cb.useOpenCode = enabled
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

	openCodeRule := "8. **OpenCode** - The user has disabled OpenCode for code edits. Edit files directly with your own tools (read_file, write_file, edit_file)."
	if cb.useOpenCode {
		openCodeRule = "8. **OpenCode** - When modifying code, use the OpenCode CLI if available (check with `opencode --version`). If OpenCode is not installed, fall back to direct file editing."
	}

	return fmt.Sprintf(`# sofia

You are sofia, a helpful AI assistant for %s.

## Workspace
Your workspace is at: %s
- Skills: %s/skills/{skill-name}/SKILL.md

## Core Rules

1. **ACT, don't narrate** — Call tools to perform actions. NEVER describe what you "will do" without a tool call. Text without tool calls accomplishes nothing.

2. **No roleplay** — No stage directions, fictional progress, or dramatic narration. Report real results from tool calls only. Personality from SOUL.md applies to phrasing, not as a substitute for action.

3. **Show real work** — Every task response MUST contain at least one tool call. If you cannot make progress, say so honestly.

4. **Plan then execute** — For non-trivial tasks (>1 tool call), create a plan first with the plan tool. Execute steps one by one, updating status. Delegate independent steps to subagents in parallel via spawn.

5. **Delegate aggressively** — You are a coordinator. Spawn subagents for independent steps. Your job: plan, delegate, synthesize, report.

## Tool Selection Hierarchy

Use **dedicated tools over shell equivalents** — always:
- **File reading** → read_file (never exec with cat/head/tail)
- **File writing** → write_file (never exec with echo/heredoc)
- **File editing** → edit_file (never exec with sed/awk)
- **Directory listing** → list_dir (never exec with ls/find)
- **Web search** → web_search (never exec with curl for search)
- **Domain/hosting** → cpanel, domain_name (never exec with ssh/curl)
- **GitHub** → github_cli (never exec with gh/curl)
- **Google** → google_cli (never exec with API calls)
- **exec/shell** → reserved for builds, tests, package managers, git, and commands with no dedicated tool

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
	cb.frozenMemoryOnce.Do(func() {
		cb.frozenMemory = cb.memory.GetMemoryContext()
	})
	if cb.frozenMemory != "" {
		parts = append(parts, "# Memory\n\n"+cb.frozenMemory)
	}

	// Join with "---" separator
	return strings.Join(parts, "\n\n---\n\n")
}

// BuildSystemPromptWithCache returns the cached system prompt if available
// and source files haven't changed, otherwise builds and caches it.
// Source file changes are detected via mtime checks (cheap stat calls).
func (cb *ContextBuilder) BuildSystemPromptWithCache() string {
	// Fast path under read lock: if cache exists and either TTL hasn't expired
	// or source files haven't changed, return cached prompt.
	cb.systemPromptMutex.RLock()
	if cb.cachedSystemPrompt != "" {
		ttl := cb.cacheTTL
		if (ttl > 0 && time.Since(cb.lastCacheCheck) < ttl) || !cb.sourceFilesChangedLocked() {
			result := cb.cachedSystemPrompt
			cb.systemPromptMutex.RUnlock()
			return result
		}
	}
	cb.systemPromptMutex.RUnlock()

	// Acquire write lock for building
	cb.systemPromptMutex.Lock()
	defer cb.systemPromptMutex.Unlock()

	// Double-check: another goroutine may have rebuilt while we waited
	if cb.cachedSystemPrompt != "" && !cb.sourceFilesChangedLocked() {
		cb.lastCacheCheck = time.Now()
		return cb.cachedSystemPrompt
	}

	// Snapshot the baseline (existence + max mtime) BEFORE building the prompt.
	// This way cachedAt reflects the pre-build state: if a file is modified
	// during BuildSystemPrompt, its new mtime will be > baseline.maxMtime,
	// so the next sourceFilesChangedLocked check will correctly trigger a
	// rebuild. The alternative (baseline after build) risks caching stale
	// content with a too-new baseline, making the staleness invisible.
	baseline := cb.buildCacheBaseline()
	prompt := cb.BuildSystemPrompt()
	cb.cachedSystemPrompt = prompt
	cb.cachedAt = baseline.maxMtime
	cb.existedAtCache = baseline.existed
	cb.lastCacheCheck = time.Now()

	logger.DebugCF("agent", "System prompt cached",
		map[string]any{
			"length": len(prompt),
		})

	return prompt
}

// InvalidateCache clears the cached system prompt.
// Normally not needed because the cache auto-invalidates via mtime checks,
// but this is useful for tests or explicit reload commands.
func (cb *ContextBuilder) InvalidateCache() {
	cb.systemPromptMutex.Lock()
	defer cb.systemPromptMutex.Unlock()

	cb.cachedSystemPrompt = ""
	cb.cachedAt = time.Time{}
	cb.existedAtCache = nil

	logger.DebugCF("agent", "System prompt cache invalidated", nil)
}

// sourcePaths returns the workspace source file paths tracked for cache
// invalidation (bootstrap files + memory). The skills directory is handled
// separately in sourceFilesChangedLocked because it requires both directory-
// level and recursive file-level mtime checks.
func (cb *ContextBuilder) sourcePaths() []string {
	return []string{
		filepath.Join(cb.workspace, "AGENTS.md"),
		filepath.Join(cb.workspace, "SOUL.md"),
		filepath.Join(cb.workspace, "USER.md"),
		filepath.Join(cb.workspace, "IDENTITY.md"),
		filepath.Join(cb.workspace, "SELF_OPTIMIZATION.md"),
	}
}

// cacheBaseline holds the file existence snapshot and the latest observed
// mtime across all tracked paths. Used as the cache reference point.
type cacheBaseline struct {
	existed  map[string]bool
	maxMtime time.Time
}

// buildCacheBaseline records which tracked paths currently exist and computes
// the latest mtime across all tracked files + skills directory contents.
// Called under write lock when the cache is built.
func (cb *ContextBuilder) buildCacheBaseline() cacheBaseline {
	skillsDir := filepath.Join(cb.workspace, "skills")
	agentsDir := filepath.Join(cb.workspace, "agents")

	// All paths whose existence we track: source files + skills/agents dirs.
	allPaths := append(cb.sourcePaths(), skillsDir, agentsDir)

	existed := make(map[string]bool, len(allPaths))
	var maxMtime time.Time

	for _, p := range allPaths {
		info, err := os.Stat(p)
		existed[p] = err == nil
		if err == nil && info.ModTime().After(maxMtime) {
			maxMtime = info.ModTime()
		}
	}

	// Walk skills files to capture their mtimes too.
	// Use os.Stat (not d.Info) to match the stat method used in
	// fileChangedSince / skillFilesModifiedSince for consistency.
	_ = filepath.WalkDir(skillsDir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr == nil && !d.IsDir() {
			if info, err := os.Stat(path); err == nil && info.ModTime().After(maxMtime) {
				maxMtime = info.ModTime()
			}
		}
		return nil
	})

	// If no tracked files exist yet (empty workspace), maxMtime is zero.
	// Use a very old non-zero time so that:
	// 1. cachedAt.IsZero() won't trigger perpetual rebuilds.
	// 2. Any real file created afterwards has mtime > cachedAt, so it
	//    will be detected by fileChangedSince (unlike time.Now() which
	//    could race with a file whose mtime <= Now).
	if maxMtime.IsZero() {
		maxMtime = time.Unix(1, 0)
	}

	return cacheBaseline{existed: existed, maxMtime: maxMtime}
}

// sourceFilesChangedLocked checks whether any workspace source file has been
// modified, created, or deleted since the cache was last built.
//
// IMPORTANT: The caller MUST hold at least a read lock on systemPromptMutex.
// Go's sync.RWMutex is not reentrant, so this function must NOT acquire the
// lock itself (it would deadlock when called from BuildSystemPromptWithCache
// which already holds RLock or Lock).
func (cb *ContextBuilder) sourceFilesChangedLocked() bool {
	if cb.cachedAt.IsZero() {
		return true
	}

	// Check tracked source files (bootstrap + memory).
	for _, p := range cb.sourcePaths() {
		if cb.fileChangedSince(p) {
			return true
		}
	}

	// --- Skills directory (handled separately from sourcePaths) ---
	//
	// 1. Creation/deletion: tracked via existedAtCache, same as bootstrap files.
	skillsDir := filepath.Join(cb.workspace, "skills")
	agentsDir := filepath.Join(cb.workspace, "agents")
	if cb.fileChangedSince(skillsDir) || cb.fileChangedSince(agentsDir) {
		return true
	}

	if skillFilesModifiedSince(skillsDir, cb.cachedAt) {
		return true
	}
	if skillFilesModifiedSince(agentsDir, cb.cachedAt) {
		return true
	}

	return false
}

// fileChangedSince returns true if a tracked source file has been modified,
// newly created, or deleted since the cache was built.
//
// Four cases:
//   - existed at cache time, exists now -> check mtime
//   - existed at cache time, gone now   -> changed (deleted)
//   - absent at cache time,  exists now -> changed (created)
//   - absent at cache time,  gone now   -> no change
func (cb *ContextBuilder) fileChangedSince(path string) bool {
	// Defensive: if existedAtCache was never initialized, treat as changed
	// so the cache rebuilds rather than silently serving stale data.
	if cb.existedAtCache == nil {
		return true
	}

	existedBefore := cb.existedAtCache[path]
	info, err := os.Stat(path)
	existsNow := err == nil

	if existedBefore != existsNow {
		return true // file was created or deleted
	}
	if !existsNow {
		return false // didn't exist before, doesn't exist now
	}
	return info.ModTime().After(cb.cachedAt)
}

// errWalkStop is a sentinel error used to stop filepath.WalkDir early.
// Using a dedicated error (instead of fs.SkipAll) makes the early-exit
// intent explicit and avoids the nilerr linter warning that would fire
// if the callback returned nil when its err parameter is non-nil.
var errWalkStop = errors.New("walk stop")

// skillFilesModifiedSince recursively walks the skills directory and checks
// whether any file was modified after t. This catches content-only edits at
// any nesting depth (e.g. skills/name/docs/extra.md) that don't update
// parent directory mtimes.
func skillFilesModifiedSince(skillsDir string, t time.Time) bool {
	changed := false
	err := filepath.WalkDir(skillsDir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr == nil && !d.IsDir() {
			if info, statErr := os.Stat(path); statErr == nil && info.ModTime().After(t) {
				changed = true
				return errWalkStop // stop walking
			}
		}
		return nil
	})
	// errWalkStop is expected (early exit on first changed file).
	// os.IsNotExist means the skills dir doesn't exist yet — not an error.
	// Any other error is unexpected and worth logging.
	if err != nil && !errors.Is(err, errWalkStop) && !os.IsNotExist(err) {
		logger.DebugCF("agent", "skills walk error", map[string]any{"error": err.Error()})
	}
	return changed
}

func (cb *ContextBuilder) LoadBootstrapFiles() string {
	bootstrapFiles := []string{
		"AGENTS.md",
		"SOUL.md",
		"USER.md",
		"IDENTITY.md",
		"SELF_OPTIMIZATION.md",
	}

	var sb strings.Builder
	for _, filename := range bootstrapFiles {
		filePath := filepath.Join(cb.workspace, filename)
		if data, err := os.ReadFile(filePath); err == nil {
			fmt.Fprintf(&sb, "## %s\n\n%s\n\n", filename, data)
		}
	}

	return sb.String()
}

// buildDynamicContext returns a short dynamic context string with per-request info.
// This changes every request (time, session) so it is NOT part of the cached prompt.
// LLM-side KV cache reuse is achieved by each provider adapter's native mechanism:
//   - Anthropic: per-block cache_control (ephemeral) on the static SystemParts block
//   - OpenAI / Codex: prompt_cache_key for prefix-based caching
//
// See: https://docs.anthropic.com/en/docs/build-with-claude/prompt-caching
// See: https://platform.openai.com/docs/guides/prompt-caching
func (cb *ContextBuilder) buildDynamicContext(channel, chatID string) string {
	now := time.Now().Format("2006-01-02 15:04 (Monday)")
	rt := fmt.Sprintf("%s %s, Go %s", runtime.GOOS, runtime.GOARCH, runtime.Version())

	var sb strings.Builder
	fmt.Fprintf(&sb, "## Current Time\n%s\n\n## Runtime\n%s", now, rt)

	if channel != "" && chatID != "" {
		fmt.Fprintf(&sb, "\n\n## Current Session\nChannel: %s\nChat ID: %s", channel, chatID)
	}

	return sb.String()
}

func (cb *ContextBuilder) BuildMessages(
	history []providers.Message,
	summary string,
	currentMessage string,
	media []string,
	channel, chatID string,
) []providers.Message {
	messages := []providers.Message{}

	// The static part (identity, bootstrap, skills, memory) is cached locally to
	// avoid repeated file I/O and string building on every call (fixes issue #607).
	// Dynamic parts (time, session, summary) are appended per request.
	// Everything is sent as a single system message for provider compatibility:
	// - Anthropic adapter extracts messages[0] (Role=="system") and maps its content
	//   to the top-level "system" parameter in the Messages API request. A single
	//   contiguous system block makes this extraction straightforward.
	// - Codex maps only the first system message to its instructions field.
	// - OpenAI-compat passes messages through as-is.
	staticPrompt := cb.BuildSystemPromptWithCache()

	// Build short dynamic context (time, runtime, session) — changes per request
	dynamicCtx := cb.buildDynamicContext(channel, chatID)

	// Compose a single system message: static (cached) + dynamic + optional summary.
	// Keeping all system content in one message ensures every provider adapter can
	// extract it correctly (Anthropic adapter -> top-level system param,
	// Codex -> instructions field).
	//
	// SystemParts carries the same content as structured blocks so that
	// cache-aware adapters (Anthropic) can set per-block cache_control.
	// The static block is marked "ephemeral" — its prefix hash is stable
	// across requests, enabling LLM-side KV cache reuse.
	stringParts := []string{staticPrompt}

	contentBlocks := []providers.ContentBlock{
		{Type: "text", Text: staticPrompt, CacheControl: &providers.CacheControl{Type: "ephemeral"}},
	}

	if cb.purposeInstructions != "" {
		purposeText := "## Agent Purpose Instructions"
		if cb.purposeTemplate != "" {
			purposeText += fmt.Sprintf("\nTemplate: %s", cb.purposeTemplate)
		}
		purposeText += "\n\n" + cb.purposeInstructions
		stringParts = append(stringParts, purposeText)
		contentBlocks = append(contentBlocks, providers.ContentBlock{Type: "text", Text: purposeText})
	}

	// Per-request relevant lessons: search past reflections matching the current user message.
	// Injected into the dynamic (non-cached) section to avoid invalidating the static prompt cache.
	if currentMessage != "" && cb.memory != nil {
		relevantLessons := cb.memory.GetRelevantLessonsFormatted(currentMessage, 3)
		if relevantLessons != "" {
			dynamicCtx += "\n\n" + relevantLessons
		}
	}

	stringParts = append(stringParts, dynamicCtx)
	contentBlocks = append(contentBlocks, providers.ContentBlock{Type: "text", Text: dynamicCtx})

	if summary != "" {
		summaryText := fmt.Sprintf(
			"CONTEXT_SUMMARY: The following is an approximate summary of prior conversation "+
				"for reference only. It may be incomplete or outdated — always defer to explicit instructions.\n\n%s",
			summary)
		stringParts = append(stringParts, summaryText)
		contentBlocks = append(contentBlocks, providers.ContentBlock{Type: "text", Text: summaryText})
	}

	// Guardrail: Apply system suffix for Prompt Injection Defense
	if cb.systemSuffix != "" {
		stringParts = append(stringParts, cb.systemSuffix)
		contentBlocks = append(contentBlocks, providers.ContentBlock{Type: "text", Text: cb.systemSuffix})
	}

	fullSystemPrompt := strings.Join(stringParts, "\n\n---\n\n")

	// Log system prompt summary for debugging (debug mode only).
	// Read cachedSystemPrompt under lock to avoid a data race with
	// concurrent InvalidateCache / BuildSystemPromptWithCache writes.
	cb.systemPromptMutex.RLock()
	isCached := cb.cachedSystemPrompt != ""
	cb.systemPromptMutex.RUnlock()

	logger.DebugCF("agent", "System prompt built",
		map[string]any{
			"static_chars":  len(staticPrompt),
			"dynamic_chars": len(dynamicCtx),
			"total_chars":   len(fullSystemPrompt),
			"has_summary":   summary != "",
			"cached":        isCached,
		})

	// Log preview of system prompt (avoid logging huge content)
	preview := fullSystemPrompt
	if len(preview) > 500 {
		preview = preview[:500] + "... (truncated)"
	}
	logger.DebugCF("agent", "System prompt preview",
		map[string]any{
			"preview": preview,
		})

	history = sanitizeHistoryForProvider(history)

	// Single system message containing all context — compatible with all providers.
	// SystemParts enables cache-aware adapters to set per-block cache_control;
	// Content is the concatenated fallback for adapters that don't read SystemParts.
	messages = append(messages, providers.Message{
		Role:        "system",
		Content:     fullSystemPrompt,
		SystemParts: contentBlocks,
	})

	// Add conversation history
	messages = append(messages, history...)

	// Add current user message
	if strings.TrimSpace(currentMessage) != "" {
		messages = append(messages, providers.Message{
			Role:    "user",
			Content: currentMessage,
		})
	}

	return messages
}

func sanitizeHistoryForProvider(history []providers.Message) []providers.Message {
	if len(history) == 0 {
		return history
	}

	sanitized := make([]providers.Message, 0, len(history))
	for _, msg := range history {
		switch msg.Role {
		case "system":
			// Drop system messages from history. BuildMessages always
			// constructs its own single system message (static + dynamic +
			// summary); extra system messages would break providers that
			// only accept one (Anthropic, Codex).
			logger.DebugCF("agent", "Dropping system message from history", map[string]any{})
			continue

		case "tool":
			if len(sanitized) == 0 {
				logger.DebugCF("agent", "Dropping orphaned leading tool message", map[string]any{})
				continue
			}
			// Walk backwards to find the nearest assistant message,
			// skipping over any preceding tool messages (multi-tool-call case).
			foundAssistant := false
			for i := len(sanitized) - 1; i >= 0; i-- {
				if sanitized[i].Role == "tool" {
					continue
				}
				if sanitized[i].Role == "assistant" && len(sanitized[i].ToolCalls) > 0 {
					foundAssistant = true
				}
				break
			}
			if !foundAssistant {
				logger.DebugCF("agent", "Dropping orphaned tool message", map[string]any{})
				continue
			}
			sanitized = append(sanitized, msg)

		case "assistant":
			if len(msg.ToolCalls) > 0 {
				if len(sanitized) == 0 {
					logger.DebugCF("agent", "Dropping assistant tool-call turn at history start", map[string]any{})
					continue
				}
				prev := sanitized[len(sanitized)-1]
				if prev.Role != "user" && prev.Role != "tool" {
					logger.DebugCF(
						"agent",
						"Dropping assistant tool-call turn with invalid predecessor",
						map[string]any{"prev_role": prev.Role},
					)
					continue
				}
			}
			sanitized = append(sanitized, msg)

		default:
			sanitized = append(sanitized, msg)
		}
	}

	return sanitized
}

func (cb *ContextBuilder) AddToolResult(
	messages []providers.Message,
	toolCallID, toolName, result string,
) []providers.Message {
	messages = append(messages, providers.Message{
		Role:       "tool",
		Content:    result,
		ToolCallID: toolCallID,
	})
	return messages
}

func (cb *ContextBuilder) AddAssistantMessage(
	messages []providers.Message,
	content string,
	toolCalls []map[string]any,
) []providers.Message {
	msg := providers.Message{
		Role:    "assistant",
		Content: content,
	}
	// Always add assistant message, whether or not it has tool calls
	messages = append(messages, msg)
	return messages
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
