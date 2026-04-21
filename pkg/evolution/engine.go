package evolution

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/grasberg/sofia/pkg/bus"
	"github.com/grasberg/sofia/pkg/config"
	"github.com/grasberg/sofia/pkg/logger"
	"github.com/grasberg/sofia/pkg/memory"
	"github.com/grasberg/sofia/pkg/providers"
	"github.com/grasberg/sofia/pkg/reputation"
)

// Proposal represents a pending evolution action that requires human approval.
type Proposal struct {
	ID        string          `json:"id"`
	Action    EvolutionAction `json:"action"`
	CreatedAt time.Time       `json:"created_at"`
	Status    string          `json:"status"` // "pending", "approved", "rejected"
}

// EvolutionEngine implements the 5-phase observe-diagnose-plan-act-verify loop
// that continuously evolves the agent system.
type EvolutionEngine struct {
	// provider and model are mutated under providerMu so users can hot-swap
	// them via SetProvider when the global model changes — without having
	// to tear the engine down and lose its running state / budget tracking.
	providerMu sync.RWMutex
	provider   providers.LLMProvider
	model      string

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

	mu                sync.Mutex
	cancelFunc        context.CancelFunc
	running           bool
	budgetSpent       float64
	budgetResetDate   time.Time
	lastRun           time.Time
	lastConsolidation time.Time
	paused            atomic.Bool
	pendingProposals  []Proposal
}

// llm returns the currently configured (provider, model) pair. Used by the
// phase handlers so they pick up mid-flight SetProvider swaps.
func (e *EvolutionEngine) llm() (providers.LLMProvider, string) {
	e.providerMu.RLock()
	defer e.providerMu.RUnlock()
	return e.provider, e.model
}

// SetProvider atomically swaps the engine's LLM provider and model, and
// propagates the change to the attached AgentArchitect and SafeModifier so
// every code path that was caching the old model picks up the new one on
// its next call. Safe to invoke while the engine is running.
func (e *EvolutionEngine) SetProvider(provider providers.LLMProvider, model string) {
	e.providerMu.Lock()
	e.provider = provider
	e.model = model
	arch := e.architect
	mod := e.modifier
	e.providerMu.Unlock()

	if arch != nil {
		arch.SetProvider(provider, model)
	}
	if mod != nil {
		mod.SetProvider(provider)
	}
}

// NewEvolutionEngine creates a new EvolutionEngine wired to all required dependencies.
func NewEvolutionEngine(
	provider providers.LLMProvider,
	model string,
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
		model:      model,
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

	e.mu.Lock()
	defer e.mu.Unlock()

	if e.running {
		return fmt.Errorf("evolution engine already running")
	}

	ctx, cancel := context.WithCancel(ctx)
	e.cancelFunc = cancel
	e.running = true

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
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.cancelFunc != nil {
		e.cancelFunc()
		e.cancelFunc = nil
	}
	e.running = false
	logger.InfoCF("evolution", "Evolution engine stopped", nil)
}

// IsRunning returns whether the evolution engine is currently running.
func (e *EvolutionEngine) IsRunning() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.running
}

// runLoop is the background goroutine that drives periodic evolution cycles.
func (e *EvolutionEngine) runLoop(ctx context.Context, interval time.Duration) {
	defer func() {
		e.mu.Lock()
		e.running = false
		e.mu.Unlock()
	}()

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

	// Reset budget at the start of a new day.
	today := time.Now().Truncate(24 * time.Hour)
	if e.budgetResetDate.IsZero() || !today.Equal(e.budgetResetDate) {
		e.budgetSpent = 0
		e.budgetResetDate = today
	}

	// Budget check: skip if exceeded.
	if e.cfg.MaxCostPerDay > 0 && e.budgetSpent >= e.cfg.MaxCostPerDay {
		logger.InfoCF("evolution", "Daily budget exceeded, skipping cycle", map[string]any{
			"spent": e.budgetSpent,
			"limit": e.cfg.MaxCostPerDay,
		})
		return
	}

	logger.InfoCF("evolution", "Starting evolution cycle", nil)

	if e.provider == nil {
		logger.WarnCF("evolution", "No LLM provider available, skipping cycle", nil)
		return
	}

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

	// Phase 6: Periodic memory consolidation
	if e.cfg.MemoryConsolidation {
		e.maybeConsolidate()
	}

	// Phase 7: Skill auto-improvement
	if e.cfg.SkillAutoImprove && e.cfg.SelfModifyEnabled {
		e.improveSkills(ctx)
	}

	e.lastRun = time.Now()
	logger.InfoCF("evolution", "Evolution cycle complete", map[string]any{
		"actions_planned": len(actions),
	})
}

// --- Public methods for /evolve commands ---

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

// RunNow triggers an immediate evolution cycle (for /evolve run).
func (e *EvolutionEngine) RunNow(ctx context.Context) {
	e.runCycle(ctx)
}

// FormatStatus returns a human-readable status summary.
func (e *EvolutionEngine) FormatStatus() string {
	e.mu.Lock()
	running := e.running
	spent := e.budgetSpent
	lastRun := e.lastRun
	pendingCount := 0
	for _, p := range e.pendingProposals {
		if p.Status == "pending" {
			pendingCount++
		}
	}
	e.mu.Unlock()

	var sb strings.Builder
	sb.WriteString("Evolution Engine Status\n")
	sb.WriteString("======================\n")

	if running {
		sb.WriteString("State: running\n")
	} else {
		sb.WriteString("State: stopped\n")
	}

	if e.paused.Load() {
		sb.WriteString("Paused: yes\n")
	} else {
		sb.WriteString("Paused: no\n")
	}

	if !lastRun.IsZero() {
		fmt.Fprintf(&sb, "Last run: %s\n", lastRun.Format(time.RFC3339))
	} else {
		sb.WriteString("Last run: never\n")
	}

	if e.cfg.MaxCostPerDay > 0 {
		fmt.Fprintf(&sb, "Budget: $%.2f / $%.2f\n", spent, e.cfg.MaxCostPerDay)
	}

	fmt.Fprintf(&sb, "Interval: %d minutes\n", e.cfg.IntervalMinutes)

	if pendingCount > 0 {
		fmt.Fprintf(&sb, "Pending proposals: %d\n", pendingCount)
	}

	return sb.String()
}

// RecentHistory returns the most recent changelog entries.
func (e *EvolutionEngine) RecentHistory(n int) ([]ChangelogEntry, error) {
	since := time.Now().Add(-30 * 24 * time.Hour)
	return e.changelog.Query(since, n)
}
