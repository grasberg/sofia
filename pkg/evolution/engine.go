package evolution

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/grasberg/sofia/pkg/bus"
	"github.com/grasberg/sofia/pkg/config"
	"github.com/grasberg/sofia/pkg/logger"
	"github.com/grasberg/sofia/pkg/memory"
	"github.com/grasberg/sofia/pkg/providers"
	pt "github.com/grasberg/sofia/pkg/providers/protocoltypes"
	"github.com/grasberg/sofia/pkg/reputation"
	"github.com/grasberg/sofia/pkg/utils"
)

// EvolutionEngine implements the 5-phase observe-diagnose-plan-act-verify loop
// that continuously evolves the agent system.
type EvolutionEngine struct {
	provider   providers.LLMProvider
	memDB      *memory.MemoryDB
	registrar  AgentRegistrar
	a2a        A2ARegistrar
	toolStats  ToolStatsProvider
	reputation *reputation.Manager
	store      *AgentStore
	changelog  *ChangelogWriter
	tracker    *PerformanceTracker
	architect  *AgentArchitect
	modifier   *SafeModifier
	cfg        *config.EvolutionConfig
	bus        *bus.MessageBus

	mu          sync.Mutex
	cancelFunc  context.CancelFunc
	running     atomic.Bool
	budgetSpent float64
	lastRun     time.Time
	paused      atomic.Bool
}

// NewEvolutionEngine creates a new EvolutionEngine wired to all required dependencies.
func NewEvolutionEngine(
	provider providers.LLMProvider,
	memDB *memory.MemoryDB,
	registrar AgentRegistrar,
	a2a A2ARegistrar,
	toolStats ToolStatsProvider,
	rep *reputation.Manager,
	store *AgentStore,
	changelog *ChangelogWriter,
	tracker *PerformanceTracker,
	architect *AgentArchitect,
	modifier *SafeModifier,
	cfg *config.EvolutionConfig,
	msgBus *bus.MessageBus,
) *EvolutionEngine {
	return &EvolutionEngine{
		provider:   provider,
		memDB:      memDB,
		registrar:  registrar,
		a2a:        a2a,
		toolStats:  toolStats,
		reputation: rep,
		store:      store,
		changelog:  changelog,
		tracker:    tracker,
		architect:  architect,
		modifier:   modifier,
		cfg:        cfg,
		bus:        msgBus,
	}
}

// Start begins the background evolution loop. Mirrors autonomy.Service.Start.
func (e *EvolutionEngine) Start(ctx context.Context) error {
	if !e.cfg.Enabled {
		logger.InfoCF("evolution", "Evolution engine is disabled in config", nil)
		return nil
	}

	if e.running.Load() {
		return fmt.Errorf("evolution engine already running")
	}

	ctx, cancel := context.WithCancel(ctx)
	e.cancelFunc = cancel
	e.running.Store(true)

	interval := e.cfg.IntervalMinutes
	if interval <= 0 {
		interval = 30
	}
	if interval < 5 {
		interval = 5
	}

	go e.runLoop(ctx, time.Duration(interval)*time.Minute)
	logger.InfoCF("evolution", "Evolution engine started", map[string]any{
		"interval_minutes": interval,
	})
	return nil
}

// Stop shuts down the evolution engine gracefully.
func (e *EvolutionEngine) Stop() {
	if e.cancelFunc != nil {
		e.cancelFunc()
		e.cancelFunc = nil
	}
	e.running.Store(false)
	logger.InfoCF("evolution", "Evolution engine stopped", nil)
}

// runLoop is the background goroutine that drives periodic evolution cycles.
func (e *EvolutionEngine) runLoop(ctx context.Context, interval time.Duration) {
	defer e.running.Store(false)

	// 2-minute initial delay to let the system warm up.
	select {
	case <-ctx.Done():
		return
	case <-time.After(2 * time.Minute):
	}

	e.runCycle(ctx)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			e.runCycle(ctx)
			e.checkDailySummary(ctx)
		}
	}
}

// runCycle executes one full observe-diagnose-plan-act-verify cycle.
func (e *EvolutionEngine) runCycle(ctx context.Context) {
	if !e.mu.TryLock() {
		logger.InfoCF("evolution", "Cycle already in progress, skipping", nil)
		return
	}
	defer e.mu.Unlock()

	if e.paused.Load() {
		logger.DebugCF("evolution", "Evolution is paused, skipping cycle", nil)
		return
	}

	// Budget check: reset at midnight, skip if exceeded.
	if e.cfg.MaxCostPerDay > 0 && e.budgetSpent >= e.cfg.MaxCostPerDay {
		logger.InfoCF("evolution", "Daily budget exceeded, skipping cycle", map[string]any{
			"spent": e.budgetSpent,
			"limit": e.cfg.MaxCostPerDay,
		})
		return
	}

	logger.InfoCF("evolution", "Starting evolution cycle", nil)

	// Phase 1: Observe
	report := e.observe(ctx)

	// Phase 2: Diagnose
	diagnosis, err := e.diagnose(ctx, report)
	if err != nil {
		logger.WarnCF("evolution", "Diagnosis failed", map[string]any{"error": err.Error()})
		return
	}

	// Phase 3: Plan
	actions, err := e.plan(ctx, diagnosis)
	if err != nil {
		logger.WarnCF("evolution", "Planning failed", map[string]any{"error": err.Error()})
		return
	}

	// Phase 4: Act
	e.act(ctx, actions)

	// Phase 5: Verify
	e.verify(ctx)

	e.lastRun = time.Now()
	logger.InfoCF("evolution", "Evolution cycle complete", map[string]any{
		"actions_planned": len(actions),
	})
}

// observe gathers runtime metrics from all agents and tools.
func (e *EvolutionEngine) observe(_ context.Context) ObservationReport {
	report := ObservationReport{
		AgentStats:   make(map[string]*AgentPerfSnapshot),
		ToolFailures: make(map[string]int),
	}

	agentIDs := e.registrar.ListAgentIDs()
	for _, id := range agentIDs {
		perf, err := e.tracker.GetAgentPerformance(id)
		if err != nil {
			logger.DebugCF("evolution", "Failed to get performance for agent", map[string]any{
				"agent_id": id,
				"error":    err.Error(),
			})
			continue
		}
		report.AgentStats[id] = &AgentPerfSnapshot{
			AgentID:     id,
			SuccessRate: perf.SuccessRate24h,
			TaskCount:   perf.TaskCount24h,
			AvgScore:    perf.AvgScore24h,
			Trend:       perf.Trend,
		}
		report.TotalTasks += perf.TaskCount24h
	}

	// Gather tool failure stats.
	if e.toolStats != nil {
		stats := e.toolStats.GetStats()
		for tool, v := range stats {
			if count, ok := v.(int); ok {
				report.ToolFailures[tool] = count
			}
		}
	}

	// Compute overall error rate.
	if report.TotalTasks > 0 {
		totalFailures := 0
		for _, snap := range report.AgentStats {
			if snap.TaskCount > 0 {
				totalFailures += int(float64(snap.TaskCount) * (1 - snap.SuccessRate))
			}
		}
		report.ErrorRate = float64(totalFailures) / float64(report.TotalTasks)
	}

	logger.InfoCF("evolution", "Observation complete", map[string]any{
		"agents":      len(report.AgentStats),
		"total_tasks": report.TotalTasks,
		"error_rate":  report.ErrorRate,
	})

	return report
}

// diagnose sends the observation report to the LLM for analysis.
func (e *EvolutionEngine) diagnose(ctx context.Context, report ObservationReport) (Diagnosis, error) {
	reportJSON, err := json.Marshal(report)
	if err != nil {
		return Diagnosis{}, fmt.Errorf("marshal observation report: %w", err)
	}

	messages := []pt.Message{
		{
			Role: "system",
			Content: "You are an AI system analyst. Analyze the provided metrics and " +
				"identify issues. Respond with valid JSON only, no markdown fences.",
		},
		{
			Role: "user",
			Content: fmt.Sprintf(
				"Analyze these agent system metrics and identify issues.\n\n"+
					"Metrics:\n%s\n\n"+
					"Respond in JSON: "+
					"{\"capability_gaps\": [\"...\"], "+
					"\"underperformers\": [\"agent_id\", ...], "+
					"\"success_patterns\": [\"...\"], "+
					"\"prompt_suggestions\": [\"...\"]}",
				string(reportJSON),
			),
		},
	}

	resp, err := e.provider.Chat(ctx, messages, nil, "", nil)
	if err != nil {
		return Diagnosis{}, fmt.Errorf("diagnosis LLM call: %w", err)
	}

	// Estimate cost from response tokens (rough: $0.003 per 1K tokens for typical models)
	if resp.Usage != nil {
		estimatedCost := float64(resp.Usage.TotalTokens) / 1000.0 * 0.003
		e.budgetSpent += estimatedCost
	}

	content := utils.CleanJSONFences(resp.Content)

	var diagnosis Diagnosis
	if err := json.Unmarshal([]byte(content), &diagnosis); err != nil {
		return Diagnosis{}, fmt.Errorf("parse diagnosis response: %w", err)
	}

	logger.InfoCF("evolution", "Diagnosis complete", map[string]any{
		"capability_gaps":  len(diagnosis.CapabilityGaps),
		"underperformers":  len(diagnosis.Underperformers),
		"success_patterns": len(diagnosis.SuccessPatterns),
	})

	return diagnosis, nil
}

// plan asks the LLM to propose evolution actions based on the diagnosis.
func (e *EvolutionEngine) plan(ctx context.Context, diagnosis Diagnosis) ([]EvolutionAction, error) {
	diagJSON, err := json.Marshal(diagnosis)
	if err != nil {
		return nil, fmt.Errorf("marshal diagnosis: %w", err)
	}

	messages := []pt.Message{
		{
			Role: "system",
			Content: "You are an AI system architect. Propose evolution actions. " +
				"Available types: create_agent, retire_agent, tune_agent, " +
				"create_skill, modify_workspace, no_action. " +
				"Be conservative — prefer no_action when metrics are acceptable. " +
				"Respond with valid JSON only, no markdown fences.",
		},
		{
			Role: "user",
			Content: fmt.Sprintf(
				"Based on this diagnosis, propose evolution actions.\n\n"+
					"Diagnosis:\n%s\n\n"+
					"Respond as a JSON array: "+
					"[{\"type\": \"...\", \"agent_id\": \"...\", "+
					"\"params\": {...}, \"reason\": \"...\"}]",
				string(diagJSON),
			),
		},
	}

	resp, err := e.provider.Chat(ctx, messages, nil, "", nil)
	if err != nil {
		return nil, fmt.Errorf("planning LLM call: %w", err)
	}

	// Estimate cost from response tokens (rough: $0.003 per 1K tokens for typical models)
	if resp.Usage != nil {
		estimatedCost := float64(resp.Usage.TotalTokens) / 1000.0 * 0.003
		e.budgetSpent += estimatedCost
	}

	content := utils.CleanJSONFences(resp.Content)

	var actions []EvolutionAction
	if err := json.Unmarshal([]byte(content), &actions); err != nil {
		return nil, fmt.Errorf("parse planning response: %w", err)
	}

	logger.InfoCF("evolution", "Planning complete", map[string]any{
		"actions": len(actions),
	})

	return actions, nil
}

// act executes each planned action and logs results to the changelog.
func (e *EvolutionEngine) act(ctx context.Context, actions []EvolutionAction) {
	for _, action := range actions {
		switch action.Type {
		case ActionCreateAgent:
			e.actCreateAgent(ctx, action)
		case ActionRetireAgent:
			e.actRetireAgent(action)
		case ActionTuneAgent:
			e.actTuneAgent(action)
		case ActionCreateSkill:
			e.actCreateSkill(ctx, action)
		case ActionModifyWorkspace:
			e.actModifyWorkspace(ctx, action)
		case ActionNoAction:
			logger.DebugCF("evolution", "No action required", map[string]any{
				"reason": action.Reason,
			})
		default:
			logger.WarnCF("evolution", "Unknown action type", map[string]any{
				"type": string(action.Type),
			})
		}
	}
}

func (e *EvolutionEngine) actCreateAgent(ctx context.Context, action EvolutionAction) {
	gap, _ := action.Params["gap"].(string)
	if gap == "" {
		gap = action.Reason
	}

	cfg, err := e.architect.DesignAgent(ctx, gap)
	if err != nil {
		logger.WarnCF("evolution", "Failed to design agent", map[string]any{
			"error": err.Error(),
		})
		return
	}

	if err := e.architect.CreateAgent(ctx, *cfg); err != nil {
		logger.WarnCF("evolution", "Failed to create agent", map[string]any{
			"agent_id": cfg.ID,
			"error":    err.Error(),
		})
		return
	}

	e.logAction(action, fmt.Sprintf("Created agent %s (%s)", cfg.ID, cfg.Name))
}

func (e *EvolutionEngine) actRetireAgent(action EvolutionAction) {
	agentID := action.AgentID
	if agentID == "" {
		logger.WarnCF("evolution", "retire_agent action missing agent_id", nil)
		return
	}

	if err := e.registrar.RemoveAgent(agentID); err != nil {
		logger.WarnCF("evolution", "Failed to remove agent from registry", map[string]any{
			"agent_id": agentID,
			"error":    err.Error(),
		})
	}

	reason := action.Reason
	if reason == "" {
		reason = "retired by evolution engine"
	}
	if err := e.store.MarkRetired(agentID, reason); err != nil {
		logger.WarnCF("evolution", "Failed to mark agent retired in store", map[string]any{
			"agent_id": agentID,
			"error":    err.Error(),
		})
	}

	e.logAction(action, fmt.Sprintf("Retired agent %s: %s", agentID, reason))
}

func (e *EvolutionEngine) actTuneAgent(action EvolutionAction) {
	agentID := action.AgentID
	if agentID == "" {
		logger.WarnCF("evolution", "tune_agent action missing agent_id", nil)
		return
	}

	existing, _, err := e.store.Get(agentID)
	if err != nil || existing == nil {
		logger.WarnCF("evolution", "Cannot tune agent: not found in store", map[string]any{
			"agent_id": agentID,
		})
		return
	}

	// Apply tuning parameters from the action.
	if newPrompt, ok := action.Params["purpose_prompt"].(string); ok && newPrompt != "" {
		existing.PurposePrompt = newPrompt
	}
	if newModel, ok := action.Params["model"].(string); ok && newModel != "" {
		existing.ModelID = newModel
	}

	if err := e.store.Save(agentID, *existing); err != nil {
		logger.WarnCF("evolution", "Failed to save tuned agent config", map[string]any{
			"agent_id": agentID,
			"error":    err.Error(),
		})
		return
	}

	e.logAction(action, fmt.Sprintf("Tuned agent %s", agentID))
}

func (e *EvolutionEngine) actCreateSkill(ctx context.Context, action EvolutionAction) {
	skillID, _ := action.Params["skill_id"].(string)
	skillName, _ := action.Params["name"].(string)
	skillContent, _ := action.Params["content"].(string)

	if skillID == "" || skillName == "" {
		logger.WarnCF("evolution", "create_skill action missing required params", nil)
		return
	}

	// Validate skill ID is a safe slug (no path traversal)
	if strings.Contains(skillID, "/") || strings.Contains(skillID, "\\") || strings.Contains(skillID, "..") {
		logger.WarnCF("evolution", "Invalid skill ID blocked", map[string]any{
			"skill_id": skillID,
		})
		return
	}

	if skillContent == "" {
		skillContent = action.Reason
	}

	content := fmt.Sprintf("---\nname: %s\ndescription: %s\n---\n\n%s\n",
		skillName, skillName, skillContent)

	skillDir := filepath.Join(e.architect.workspace, "skills", skillID)
	skillPath := filepath.Join(skillDir, "SKILL.md")

	if err := e.modifier.ModifyFile(ctx, skillPath, content); err != nil {
		logger.WarnCF("evolution", "Failed to create skill file", map[string]any{
			"skill_id": skillID,
			"error":    err.Error(),
		})
		return
	}

	e.logAction(action, fmt.Sprintf("Created skill %s (%s)", skillID, skillName))
}

func (e *EvolutionEngine) actModifyWorkspace(ctx context.Context, action EvolutionAction) {
	filePath, _ := action.Params["path"].(string)
	newContent, _ := action.Params["content"].(string)

	if filePath == "" || newContent == "" {
		logger.WarnCF("evolution", "modify_workspace action missing path or content", nil)
		return
	}

	// Validate path is within workspace
	absPath, _ := filepath.Abs(filePath)
	absWorkspace, _ := filepath.Abs(e.architect.workspace)
	if !strings.HasPrefix(absPath, absWorkspace) {
		logger.WarnCF("evolution", "Path traversal blocked", map[string]any{
			"path":      filePath,
			"workspace": e.architect.workspace,
		})
		return
	}

	if err := e.modifier.ModifyFile(ctx, filePath, newContent); err != nil {
		logger.WarnCF("evolution", "Failed to modify workspace file", map[string]any{
			"path":  filePath,
			"error": err.Error(),
		})
		return
	}

	e.logAction(action, fmt.Sprintf("Modified workspace file %s", filePath))
}

// logAction writes a changelog entry for the given action.
func (e *EvolutionEngine) logAction(action EvolutionAction, summary string) {
	var metricBefore float64
	if action.AgentID != "" {
		perf, err := e.tracker.GetAgentPerformance(action.AgentID)
		if err == nil {
			metricBefore = perf.SuccessRate24h
		}
	}

	entry := &ChangelogEntry{
		Timestamp:    time.Now().UTC(),
		Action:       string(action.Type),
		Summary:      summary,
		MetricBefore: metricBefore,
		Details: map[string]any{
			"agent_id": action.AgentID,
			"params":   action.Params,
			"reason":   action.Reason,
		},
	}
	if err := e.changelog.Write(entry); err != nil {
		logger.WarnCF("evolution", "Failed to write changelog entry", map[string]any{
			"error": err.Error(),
		})
	}
}

// verify checks recent unverified changelog entries and evaluates outcomes.
func (e *EvolutionEngine) verify(_ context.Context) {
	entries, err := e.changelog.QueryUnverified(10)
	if err != nil {
		logger.WarnCF("evolution", "Failed to query unverified entries", map[string]any{
			"error": err.Error(),
		})
		return
	}

	for _, entry := range entries {
		outcome := e.evaluateOutcome(entry)
		if err := e.changelog.UpdateOutcome(entry.ID, outcome); err != nil {
			logger.WarnCF("evolution", "Failed to update outcome", map[string]any{
				"entry_id": entry.ID,
				"error":    err.Error(),
			})
		}

		if outcome.Result == "degraded" {
			logger.WarnCF("evolution", "Action resulted in degradation, consider revert", map[string]any{
				"entry_id": entry.ID,
				"action":   entry.Action,
				"summary":  entry.Summary,
			})
		}
	}
}

// evaluateOutcome compares current metrics vs baseline for a changelog entry.
func (e *EvolutionEngine) evaluateOutcome(entry ChangelogEntry) ActionOutcome {
	// Extract agent_id from entry details for metric comparison.
	agentID, _ := entry.Details["agent_id"].(string)
	if agentID == "" {
		return ActionOutcome{Result: "no_change"}
	}

	perf, err := e.tracker.GetAgentPerformance(agentID)
	if err != nil {
		return ActionOutcome{Result: "no_change"}
	}

	metricAfter := perf.SuccessRate24h
	metricBefore := entry.MetricBefore

	outcome := ActionOutcome{
		MetricBefore: metricBefore,
		MetricAfter:  metricAfter,
	}

	delta := metricAfter - metricBefore
	switch {
	case delta > 0.05:
		outcome.Result = "improved"
	case delta < -0.05:
		outcome.Result = "degraded"
	default:
		outcome.Result = "no_change"
	}

	return outcome
}

// --- Public methods for /evolve commands ---

// RunNow triggers an immediate evolution cycle (for /evolve run).
func (e *EvolutionEngine) RunNow(ctx context.Context) {
	e.runCycle(ctx)
}

// Pause stops the evolution engine from running cycles.
func (e *EvolutionEngine) Pause() {
	e.paused.Store(true)
	logger.InfoCF("evolution", "Evolution engine paused", nil)
}

// Resume allows the evolution engine to run cycles again.
func (e *EvolutionEngine) Resume() {
	e.paused.Store(false)
	logger.InfoCF("evolution", "Evolution engine resumed", nil)
}

// FormatStatus returns a human-readable status summary.
func (e *EvolutionEngine) FormatStatus() string {
	var sb strings.Builder
	sb.WriteString("Evolution Engine Status\n")
	sb.WriteString("======================\n")

	if e.running.Load() {
		sb.WriteString("State: running\n")
	} else {
		sb.WriteString("State: stopped\n")
	}

	if e.paused.Load() {
		sb.WriteString("Paused: yes\n")
	} else {
		sb.WriteString("Paused: no\n")
	}

	if !e.lastRun.IsZero() {
		sb.WriteString(fmt.Sprintf("Last run: %s\n", e.lastRun.Format(time.RFC3339)))
	} else {
		sb.WriteString("Last run: never\n")
	}

	if e.cfg.MaxCostPerDay > 0 {
		sb.WriteString(fmt.Sprintf("Budget: $%.2f / $%.2f\n", e.budgetSpent, e.cfg.MaxCostPerDay))
	}

	sb.WriteString(fmt.Sprintf("Interval: %d minutes\n", e.cfg.IntervalMinutes))

	return sb.String()
}

// RecentHistory returns the most recent changelog entries.
func (e *EvolutionEngine) RecentHistory(n int) ([]ChangelogEntry, error) {
	since := time.Now().Add(-30 * 24 * time.Hour)
	return e.changelog.Query(since, n)
}

// Revert reverts a specific changelog entry by ID.
func (e *EvolutionEngine) Revert(id string) error {
	entry, err := e.changelog.Get(id)
	if err != nil {
		return fmt.Errorf("get changelog entry: %w", err)
	}
	if entry == nil {
		return fmt.Errorf("changelog entry %s not found", id)
	}

	switch entry.Action {
	case string(ActionRetireAgent):
		agentID, _ := entry.Details["agent_id"].(string)
		if agentID != "" {
			// Re-activate in store.
			existing, _, err := e.store.Get(agentID)
			if err == nil && existing != nil {
				_ = e.store.Save(agentID, *existing)
				e.a2a.Register(agentID)
			}
		}
	case string(ActionModifyWorkspace):
		// Revert would need the backup path. Log a warning for now.
		logger.WarnCF("evolution", "Workspace modification revert requires manual backup restore",
			map[string]any{"entry_id": id})
	default:
		logger.InfoCF("evolution", "Revert not supported for action type", map[string]any{
			"action": entry.Action,
		})
	}

	// Mark the entry as reverted.
	return e.changelog.UpdateOutcome(id, ActionOutcome{Result: "reverted"})
}

// checkDailySummary checks if it is time to send the daily evolution summary.
func (e *EvolutionEngine) checkDailySummary(ctx context.Context) {
	if !e.cfg.DailySummary || e.cfg.DailySummaryTime == "" {
		return
	}

	now := time.Now()
	targetTime, err := time.Parse("15:04", e.cfg.DailySummaryTime)
	if err != nil {
		return
	}

	// Check if current time matches the target hour:minute within a 5-minute window.
	currentMinutes := now.Hour()*60 + now.Minute()
	targetMinutes := targetTime.Hour()*60 + targetTime.Minute()
	if abs(currentMinutes-targetMinutes) > 5 {
		return
	}

	// Avoid sending more than once per day.
	if !e.lastRun.IsZero() && now.Sub(e.lastRun) < 23*time.Hour {
		return
	}

	e.sendDailySummary(ctx)
}

func (e *EvolutionEngine) sendDailySummary(_ context.Context) {
	since := time.Now().Add(-24 * time.Hour)
	entries, err := e.changelog.Query(since, 50)
	if err != nil {
		logger.WarnCF("evolution", "Failed to query changelog for daily summary", map[string]any{
			"error": err.Error(),
		})
		return
	}

	if len(entries) == 0 {
		return
	}

	var sb strings.Builder
	sb.WriteString("Evolution Daily Summary\n")
	sb.WriteString("=======================\n\n")
	sb.WriteString(fmt.Sprintf("Actions in last 24h: %d\n\n", len(entries)))

	for _, entry := range entries {
		outcome := entry.Outcome
		if outcome == "" {
			outcome = "pending"
		}
		sb.WriteString(fmt.Sprintf("- [%s] %s (outcome: %s)\n", entry.Action, entry.Summary, outcome))
	}

	channel := e.cfg.DailySummaryChannel
	chatID := e.cfg.DailySummaryChatID
	if channel == "" || chatID == "" || e.bus == nil {
		logger.InfoCF("evolution", "Daily summary generated but no delivery channel configured", nil)
		return
	}

	e.bus.PublishOutbound(bus.OutboundMessage{
		Channel: channel,
		ChatID:  chatID,
		Content: sb.String(),
	})

	logger.InfoCF("evolution", "Daily summary sent", map[string]any{
		"channel": channel,
		"entries": len(entries),
	})
}

// abs returns the absolute value of an integer.
func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}
