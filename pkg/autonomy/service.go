package autonomy

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/grasberg/sofia/pkg/bus"
	"github.com/grasberg/sofia/pkg/config"
	"github.com/grasberg/sofia/pkg/logger"
	"github.com/grasberg/sofia/pkg/memory"
	"github.com/grasberg/sofia/pkg/providers"
	"github.com/grasberg/sofia/pkg/tools"
)

// Service configures and runs periodic autonomy operations (Proactive Suggestions, Research).
type Service struct {
	cfg        *config.AutonomyConfig
	memDB      *memory.MemoryDB
	bus        *bus.MessageBus
	provider   providers.LLMProvider
	subMgr     *tools.SubagentManager
	modelID    string
	agentID    string
	workspace  string
	mu         sync.Mutex
	cancelFunc context.CancelFunc
}

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
) *Service {
	return &Service{
		cfg:       cfg,
		memDB:     memDB,
		bus:       msgBus,
		provider:  provider,
		subMgr:    subMgr,
		agentID:   agentID,
		modelID:   modelID,
		workspace: workspace,
	}
}

// Start spawns the background periodic ticker.
func (s *Service) Start(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.cfg.Enabled {
		logger.InfoCF("autonomy", "Autonomy service is disabled in config", nil)
		return nil
	}
	if s.cancelFunc != nil {
		return fmt.Errorf("autonomy service already running")
	}

	ctx, cancel := context.WithCancel(ctx)
	s.cancelFunc = cancel

	interval := s.cfg.IntervalMinutes
	if interval < 5 {
		interval = 60 // Default to 60 if set too low to prevent spam
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

func (s *Service) performAutonomyTasks(ctx context.Context) {
	// 1. Proactive Suggestions & Autonomous Research
	if s.cfg.Suggestions || s.cfg.Research {
		s.evaluateRecentActivity(ctx)
	}
}

func (s *Service) evaluateRecentActivity(ctx context.Context) {
	// Look at recent sessions to see if there is an open question or idle state where a proactive tip helps.
	sessions, err := s.memDB.ListSessions()
	if err != nil || len(sessions) == 0 {
		return
	}

	// For simplicity, just pick the top session (most recently active)
	topSession := sessions[0]
	// If it was updated less than 10 minutes ago, the user might still be actively chatting. Give them space.
	if time.Since(topSession.UpdatedAt) < 10*time.Minute {
		return
	}
	// If it's been idle for over 2 days, they might not care right now unless it's a critical goal.
	if time.Since(topSession.UpdatedAt) > 48*time.Hour {
		return
	}

	messages, err := s.memDB.GetMessages(topSession.Key)
	if err != nil || len(messages) == 0 {
		return
	}

	// Just grab the last 5 messages for context
	startIdx := len(messages) - 5
	if startIdx < 0 {
		startIdx = 0
	}
	recentMsgs := messages[startIdx:]

	var sb strings.Builder
	for _, m := range recentMsgs {
		sb.WriteString(fmt.Sprintf("%s: %s\n", m.Role, m.Content))
	}

	prompt := fmt.Sprintf(`Analyze the recent conversation context and determine if a proactive suggestion or autonomous research would be highly beneficial to the user right now.

Recent Context:
%s

Instructions:
1. If the user was struggling with a concept, you might suggest doing background research.
2. If the user was building something, suggest the next logical component.
3. If there is no obvious high-value proactive action to take, reply ONLY with "NO_SUGGESTION".
4. If you DO have a proactive plan, outline it briefly.
`, sb.String())

	messagesToLLM := []providers.Message{
		{Role: "user", Content: prompt},
	}

	options := map[string]any{
		"max_tokens":  500,
		"temperature": 0.7,
	}

	resp, err := s.provider.Chat(ctx, messagesToLLM, nil, s.modelID, options)
	if err != nil || len(resp.Content) == 0 {
		return
	}

	content := resp.Content
	if content == "NO_SUGGESTION" || strings.TrimSpace(content) == "" {
		return
	}

	logger.InfoCF("autonomy", "Proactive generation triggered", map[string]any{"response": content})

	// Inject this thought into the memory bus so the agent wakes up and messages the user
	s.bus.PublishInbound(bus.InboundMessage{
		Channel:    "cli", // Assuming we route to cli/default or similar system channel
		ChatID:     "proactive",
		SenderID:   "autonomy_service",
		Content:    fmt.Sprintf("[PROACTIVE THOUGHT] Based on recent activity, I've had an idea:\n%s\n\nTake action on this using your tools, or simply send the suggestion to the user.", content),
		SessionKey: topSession.Key, // inject directly into their ongoing session
	})
}
