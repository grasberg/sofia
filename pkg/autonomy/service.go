package autonomy

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/grasberg/sofia/pkg/bus"
	"github.com/grasberg/sofia/pkg/config"
	"github.com/grasberg/sofia/pkg/dashboard"
	"github.com/grasberg/sofia/pkg/logger"
	"github.com/grasberg/sofia/pkg/memory"
	"github.com/grasberg/sofia/pkg/notifications"
	"github.com/grasberg/sofia/pkg/providers"
	"github.com/grasberg/sofia/pkg/tools"
)

// execInstaller is the default installerFunc: runs the command via
// `bash -c "<cmd>"` with combined stdout+stderr capture. Callers supply an
// already-bounded context for timeout control.
func execInstaller(ctx context.Context, command string) (bool, string, error) {
	cmd := exec.CommandContext(ctx, "bash", "-c", command)
	out, err := cmd.CombinedOutput()
	return err == nil, string(out), err
}

// TaskRunner executes a task as a specific agent, returning the result.
type TaskRunner func(ctx context.Context, agentID, sessionKey, task, originChannel, originChatID string) (string, error)

// LastChannelFunc returns "channel:chatID" for the user's last active channel.
type LastChannelFunc func() string

// proactiveSuggestionMinInterval is the minimum time between proactive suggestions.
const proactiveSuggestionMinInterval = 30 * time.Minute

// defaultAutonomyMaxCostPerDay is the default daily budget for autonomy LLM calls.
const defaultAutonomyMaxCostPerDay = 1.0

// Service configures and runs periodic autonomy operations (Proactive Suggestions, Research, Goal Pursuit).
type Service struct {
	cfg           *config.AutonomyConfig
	memDB         *memory.MemoryDB
	bus           *bus.MessageBus
	provider      providers.LLMProvider
	subMgr        *tools.SubagentManager
	modelID       string
	agentID       string
	workspace     string
	lastChannelFn LastChannelFunc
	push          *notifications.PushService
	hub           *dashboard.Hub
	taskRunner    TaskRunner
	planMgr       *tools.PlanManager
	mu            sync.Mutex
	cancelFunc    context.CancelFunc

	// Budget tracking (#20)
	budgetSpent             float64
	budgetResetDate         time.Time
	lastProactiveSuggestion time.Time

	// Auto-install of missing tools. toolInstaller is the function used to
	// actually run an install command — defaults to execInstaller, overridden
	// in tests. autoInstallAttempts tracks {goalID: {binary: attempted}} so we
	// never attempt the same install twice for the same goal, even across
	// multiple failing steps.
	toolInstaller       installerFunc
	autoInstallAttempts map[int64]map[string]bool
}

// installerFunc runs a single install shell command and returns
// (success, combined_output, error). Defined here so tests can supply a stub.
type installerFunc func(ctx context.Context, command string) (bool, string, error)

// NewService instantiates the autonomy service for a specific agent.
func NewService(
	cfg *config.AutonomyConfig,
	memDB *memory.MemoryDB,
	msgBus *bus.MessageBus,
	provider providers.LLMProvider,
	subMgr *tools.SubagentManager,
	agentID string,
	modelID string,
	workspace string,
	push *notifications.PushService,
) *Service {
	return &Service{
		cfg:                 cfg,
		memDB:               memDB,
		bus:                 msgBus,
		provider:            provider,
		subMgr:              subMgr,
		agentID:             agentID,
		modelID:             modelID,
		workspace:           workspace,
		push:                push,
		toolInstaller:       execInstaller,
		autoInstallAttempts: make(map[int64]map[string]bool),
	}
}

// SetDashboardHub sets the dashboard hub for broadcasting goal events.
func (s *Service) SetDashboardHub(hub *dashboard.Hub) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.hub = hub
}

// SetTaskRunner sets the function used to execute tasks as agents.
func (s *Service) SetTaskRunner(runner TaskRunner) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.taskRunner = runner
}

// SetLastChannelFunc sets the function to resolve the user's last active channel.
func (s *Service) SetLastChannelFunc(fn LastChannelFunc) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastChannelFn = fn
}

// SetPlanManager sets the plan manager for goal-to-plan pipeline.
func (s *Service) SetPlanManager(pm *tools.PlanManager) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.planMgr = pm
}

// GetSubagentManager returns the subagent manager for this service.
func (s *Service) GetSubagentManager() *tools.SubagentManager {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.subMgr
}

// Start spawns the background periodic ticker.
func (s *Service) Start(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.cfg.Enabled {
		logger.InfoCF("autonomy", "Autonomy service is disabled — running goal finalization only", nil)
		// Even with autonomy off, start a lightweight tick to finalize
		// goals whose plans were completed via the chat UI.
		s.startFinalizationTicker(ctx)
		return nil
	}
	if s.cancelFunc != nil {
		return fmt.Errorf("autonomy service already running")
	}

	ctx, cancel := context.WithCancel(ctx)
	s.cancelFunc = cancel

	interval := s.cfg.IntervalMinutes
	if interval <= 0 {
		interval = 10 // Default to 10 minutes
	}
	if interval < 2 {
		interval = 2 // Minimum 2 minutes to prevent tight loops
	}

	go s.runLoop(ctx, time.Duration(interval)*time.Minute)
	logger.InfoCF("autonomy", "Autonomy service started", map[string]any{"interval_minutes": interval})
	return nil
}

// Stop shuts down the periodic tasks.
func (s *Service) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cancelFunc != nil {
		s.cancelFunc()
		s.cancelFunc = nil
		logger.InfoCF("autonomy", "Autonomy service stopped", nil)
	}
}

func (s *Service) runLoop(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	// Initial delay so we don't jump the gun on boot when models are just warming up
	select {
	case <-ctx.Done():
		return
	case <-time.After(1 * time.Minute):
		s.performAutonomyTasks(ctx)
	}

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.performAutonomyTasks(ctx)
		}
	}
}

// startFinalizationTicker runs a lightweight background loop that only finalizes
// completed goals. Used when the full autonomy service is disabled — goals
// created via the chat UI still need to transition to "completed" when their
// plan finishes.
func (s *Service) startFinalizationTicker(ctx context.Context) {
	ctx, cancel := context.WithCancel(ctx)
	s.cancelFunc = cancel

	go func() {
		// Small initial delay to let things settle on startup.
		select {
		case <-ctx.Done():
			return
		case <-time.After(30 * time.Second):
		}

		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()

		for {
			s.finalizeCompletedGoals(ctx)
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
}

// checkBudget resets the daily budget if needed and returns true if budget is available.
func (s *Service) checkBudget() bool {
	today := time.Now().Truncate(24 * time.Hour)
	if s.budgetResetDate.IsZero() || !today.Equal(s.budgetResetDate) {
		s.budgetSpent = 0
		s.budgetResetDate = today
	}

	maxCost := s.cfg.MaxCostPerDay
	if maxCost <= 0 {
		maxCost = defaultAutonomyMaxCostPerDay
	}

	if s.budgetSpent >= maxCost {
		logger.InfoCF("autonomy", "Daily budget exceeded, skipping cycle", map[string]any{
			"spent": s.budgetSpent,
			"limit": maxCost,
		})
		return false
	}
	return true
}

// trackCost adds an estimated cost based on token usage ($0.01 per 1K tokens as safe default).
func (s *Service) trackCost(totalTokens int) {
	s.budgetSpent += float64(totalTokens) / 1000.0 * 0.01
}

func (s *Service) performAutonomyTasks(ctx context.Context) {
	// Budget check before performing any work.
	if !s.checkBudget() {
		return
	}

	// 1. Goal pursuit — work toward active goals.
	// Always finalize completed plans even if autonomous goal pursuit is off,
	// because goals can be created and executed via the chat UI without the
	// autonomy flag. Without this, completed goals stay "active" forever.
	s.finalizeCompletedGoals(ctx)
	if s.cfg.Goals {
		s.pursueGoals(ctx)
	}

	// 2. Proactive Suggestions & Autonomous Research
	if s.cfg.Suggestions || s.cfg.Research {
		s.evaluateRecentActivity(ctx)
	}

	// 3. Context trigger evaluation — check recent messages against triggers
	if s.cfg.ContextTriggers {
		s.evaluateContextTriggers(ctx)
	}
}

// notifyUser sends a message to the user's last active channel (e.g. Telegram).
func (s *Service) notifyUser(message string) {
	s.mu.Lock()
	fn := s.lastChannelFn
	msgBus := s.bus
	s.mu.Unlock()

	if fn == nil || msgBus == nil {
		return
	}
	lastChannel := fn()
	if lastChannel == "" {
		return
	}
	parts := strings.SplitN(lastChannel, ":", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return
	}
	msgBus.PublishOutbound(bus.OutboundMessage{
		Channel: parts[0],
		ChatID:  parts[1],
		Content: message,
	})
}

func (s *Service) broadcast(data map[string]any) {
	s.mu.Lock()
	hub := s.hub
	s.mu.Unlock()
	if hub != nil {
		hub.Broadcast(data)
	}
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
