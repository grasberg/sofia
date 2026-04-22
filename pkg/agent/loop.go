// Sofia - Ultra-lightweight personal AI agent
// Inspired by and based on nanobot: https://github.com/HKUDS/nanobot
// License: MIT
//
// Copyright (c) 2026 Sofia contributors

package agent

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/grasberg/sofia/pkg/audit"
	"github.com/grasberg/sofia/pkg/autonomy"
	"github.com/grasberg/sofia/pkg/budget"
	"github.com/grasberg/sofia/pkg/bus"
	"github.com/grasberg/sofia/pkg/channels"
	"github.com/grasberg/sofia/pkg/checkpoint"
	"github.com/grasberg/sofia/pkg/config"
	"github.com/grasberg/sofia/pkg/dashboard"
	"github.com/grasberg/sofia/pkg/evolution"
	"github.com/grasberg/sofia/pkg/logger"
	"github.com/grasberg/sofia/pkg/memory"
	"github.com/grasberg/sofia/pkg/notifications"
	"github.com/grasberg/sofia/pkg/providers"
	"github.com/grasberg/sofia/pkg/session"
	"github.com/grasberg/sofia/pkg/state"
	"github.com/grasberg/sofia/pkg/tools"
	"github.com/grasberg/sofia/pkg/tor"
	"github.com/grasberg/sofia/pkg/trace"
)

type AgentLoop struct {
	bus             *bus.MessageBus
	cfg             *config.Config
	registry        *AgentRegistry
	registryMu      sync.RWMutex
	configMu        sync.Mutex // protects cfg.Agents.List mutations during dynamic agent creation
	state           *state.Manager
	memDB           *memory.MemoryDB
	running         atomic.Bool
	degradedMode    atomic.Bool // set when critical components fail to initialize
	summarizing     sync.Map
	fallback        *providers.FallbackChain
	channelManager  *channels.Manager
	activeAgentID   atomic.Value // string
	activeStatus    atomic.Value // string
	planManager     *tools.PlanManager
	scratchpad      *tools.SharedScratchpad
	checkpointMgr   *checkpoint.Manager
	a2aRouter       *A2ARouter
	semanticMatcher *tools.SemanticMatcher

	// Rate limiting state
	rlMutex        sync.Mutex
	rpmCounts      map[string]int       // AgentID -> requests this minute
	rpmResetTime   map[string]time.Time // AgentID -> next reset time
	tokenCounts    map[string]int       // AgentID -> tokens this hour
	tokenResetTime map[string]time.Time // AgentID -> next reset time

	autonomyMu       sync.Mutex // protects autonomyServices map
	autonomyServices map[string]*autonomy.Service
	pushService      *notifications.PushService
	dashboardHub     *dashboard.Hub
	toolTracker      *tools.ToolTracker
	budgetManager    *budget.BudgetManager
	auditLogger      *audit.AuditLogger
	evolutionEngine  *evolution.EvolutionEngine
	usageTracker     *UsageTracker
	verboseMode      sync.Map // sessionKey -> bool
	thinkingLevel    sync.Map // sessionKey -> ThinkingLevel
	elevatedMgr      *ElevatedManager
	torService       *tor.Service
	personaManager   *PersonaManager
	branchManager    *session.BranchManager
	approvalGate     *ApprovalGate

	agentModelMu sync.RWMutex // protects defaultAgent.Model writes/reads

	dispatchWg  sync.WaitGroup // tracks goroutines from dispatchPendingSteps
	subagentSem chan struct{}  // limits concurrent subagent tasks

	evolveRunning atomic.Bool // prevents duplicate /evolve run goroutines

	// Tool result deduplication cache
	toolResultCache    sync.Map // key: "toolName:argsHash" → *cacheEntry
	toolResultCacheTTL time.Duration

	tracer         *trace.Tracer             // structured execution tracing
	providerRanker *providers.ProviderRanker // adaptive provider ranking

	// Goal restart cooldown to prevent rapid repeated restarts.
	goalRestartMu    sync.Mutex
	goalRestartTimes map[int64]time.Time

	// processCancelMu protects processCancel
	processCancelMu sync.Mutex
	processCancel   context.CancelFunc // cancels the current in-flight LLM processing

	// killed is set by Reset() to immediately abort all processing.
	// Checked at the top of every processing entry point and at each
	// LLM iteration boundary.
	killed atomic.Bool

	// directCancelsMu protects directCancels — tracks cancel funcs for
	// in-flight ProcessDirect / ProcessDirectWithImages calls so Reset()
	// can cancel them.
	directCancelsMu sync.Mutex
	directCancels   map[string]context.CancelFunc

	playwrightCancel context.CancelFunc // cancels playwright install goroutine
}

// makeSubagentSem creates a buffered channel used as a semaphore to limit
// concurrent subagent tasks. A value <= 0 means unlimited.
func makeSubagentSem(max int) chan struct{} {
	if max <= 0 {
		return nil // no limit
	}
	return make(chan struct{}, max)
}

// processOptions configures how a message is processed
type processOptions struct {
	SessionKey      string      // Session identifier for history/context
	Channel         string      // Target channel for tool execution
	ChatID          string      // Target chat ID for tool execution
	UserMessage     string      // User message content (may include prefix)
	UserImages      []string    // Optional base64 data URLs for vision (e.g. "data:image/png;base64,...")
	DefaultResponse string      // Response when LLM returns empty
	EnableSummary   bool        // Whether to trigger summarization
	SendResponse    bool        // Whether to send response via bus
	NoHistory       bool        // If true, don't load session history (for heartbeat)
	ModelOverride   string      // If set, use this model alias instead of the agent's default
	ParentSpan      *trace.Span // Parent trace span for hierarchical tracing
	Ephemeral       bool        // If true, the exchange is not stored in session history
	// OnTextDelta, if set, receives streamed text fragments as they arrive
	// from the LLM. The agent loop will switch to ChatStream for any
	// iteration where the provider implements StreamingProvider; iterations
	// that produce tool_calls still return a full LLMResponse so the normal
	// tool-execution path is unchanged. Callers are responsible for thread-
	// safety — the callback fires from the goroutine driving the stream.
	OnTextDelta func(string)
}

const defaultResponse = "I've completed processing but have no response to give. Increase `max_tool_iterations` in config.json."

func (al *AgentLoop) Run(ctx context.Context) error {
	al.running.Store(true)

	if al.evolutionEngine != nil {
		if err := al.evolutionEngine.Start(ctx); err != nil {
			logger.WarnCF("agent", "Failed to start evolution engine",
				map[string]any{"error": err.Error()})
		}
	}

	// Start the plan task dispatcher — auto-assigns pending plan steps to subagents
	go al.runPlanDispatcher(ctx)

	for al.running.Load() {
		select {
		case <-ctx.Done():
			return nil
		default:
			msg, ok := al.bus.ConsumeInbound(ctx)
			if !ok {
				continue
			}

			procCtx, procCancel := context.WithCancel(ctx)
			al.processCancelMu.Lock()
			al.processCancel = procCancel
			al.processCancelMu.Unlock()

			response, err := al.processMessage(procCtx, msg)

			al.processCancelMu.Lock()
			al.processCancel = nil
			al.processCancelMu.Unlock()
			procCancel() // ensure cleanup
			if err != nil {
				response = fmt.Sprintf("Error processing message: %v", err)
			}

			if response != "" {
				// Check if the message tool already sent a response during this round.
				// If so, skip publishing to avoid duplicate messages to the user.
				// Use default agent's tools to check (message tool is shared).
				alreadySent := false
				defaultAgent := al.getRegistry().GetDefaultAgent()
				if defaultAgent != nil {
					if tool, ok := defaultAgent.Tools.Get("message"); ok {
						if mt, ok := tool.(*tools.MessageTool); ok {
							alreadySent = mt.HasSentInRound()
						}
					}
				}

				if !alreadySent {
					al.bus.PublishOutbound(bus.OutboundMessage{
						Channel: msg.Channel,
						ChatID:  msg.ChatID,
						Content: response,
					})
				}
			}
		}
	}

	return nil
}

func (al *AgentLoop) Stop() {
	al.running.Store(false)
	if al.playwrightCancel != nil {
		al.playwrightCancel()
	}
	if al.evolutionEngine != nil {
		al.evolutionEngine.Stop()
	}
	al.stopAutonomyServices()
	if al.tracer != nil {
		al.tracer.Close()
	}

	// Wait for dispatched goroutines with a timeout
	done := make(chan struct{})
	go func() {
		al.dispatchWg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		logger.WarnCF("agent", "Timed out waiting for dispatched goroutines to finish", nil)
	}
}

// getRegistry returns the current agent registry with proper synchronization.
func (al *AgentLoop) getRegistry() *AgentRegistry {
	al.registryMu.RLock()
	defer al.registryMu.RUnlock()
	return al.registry
}

func (al *AgentLoop) RegisterTool(tool tools.Tool) {
	for _, agentID := range al.getRegistry().ListAgentIDs() {
		if agent, ok := al.getRegistry().GetAgent(agentID); ok {
			agent.Tools.Register(tool)
		}
	}
}

func (al *AgentLoop) SetChannelManager(cm *channels.Manager) {
	al.channelManager = cm
}

// TorService returns the Tor anonymity service used by agent web tools.
func (al *AgentLoop) TorService() *tor.Service {
	return al.torService
}
