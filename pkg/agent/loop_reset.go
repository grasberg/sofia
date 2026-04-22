package agent

import (
	"path/filepath"
	"time"

	"github.com/grasberg/sofia/pkg/autonomy"
	"github.com/grasberg/sofia/pkg/evolution"
	"github.com/grasberg/sofia/pkg/logger"
	"github.com/grasberg/sofia/pkg/providers"
	"github.com/grasberg/sofia/pkg/tools"
)

// Reset cancels any in-flight processing, clears all sessions, and resets all goals.
// The agent loop continues running and is ready for new messages.
func (al *AgentLoop) Reset() map[string]any {
	result := map[string]any{}

	// ── KILLSWITCH: set killed flag so every processing path aborts immediately ──
	al.killed.Store(true)
	logger.InfoCF("agent", "Reset: KILLSWITCH activated — aborting all work", nil)

	// 1. Cancel in-flight bus-driven processing
	al.processCancelMu.Lock()
	if al.processCancel != nil {
		al.processCancel()
		al.processCancel = nil
	}
	al.processCancelMu.Unlock()

	// 2. Cancel all in-flight ProcessDirect calls (Web UI, cron, heartbeat)
	al.directCancelsMu.Lock()
	directCancelled := len(al.directCancels)
	for key, cancel := range al.directCancels {
		cancel()
		delete(al.directCancels, key)
	}
	al.directCancelsMu.Unlock()
	result["direct_calls_canceled"] = directCancelled

	// 3. Stop all autonomy services (background goal pursuit, proactive suggestions)
	al.stopAutonomyServices()
	result["autonomy_stopped"] = true

	// 4. Stop the evolution engine
	if al.evolutionEngine != nil {
		al.evolutionEngine.Stop()
	}
	result["evolution_stopped"] = true

	// 5. Wait for dispatched plan goroutines to finish (with timeout)
	done := make(chan struct{})
	go func() {
		al.dispatchWg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		logger.WarnCF("agent", "Reset: timed out waiting for plan dispatchers", nil)
	}

	// 6. Drain queued inbound messages so they don't fire after reset
	drained := 0
	for {
		select {
		case <-al.bus.InboundChan():
			drained++
		default:
			goto busDrained
		}
	}
busDrained:
	result["messages_drained"] = drained

	// 7. Clear all sessions for all agents
	sessionsCleared := 0
	for _, agentID := range al.getRegistry().ListAgentIDs() {
		if agent, ok := al.getRegistry().GetAgent(agentID); ok && agent.Sessions != nil {
			for _, meta := range agent.Sessions.ListSessions() {
				if err := agent.Sessions.DeleteSession(meta.Key); err == nil {
					sessionsCleared++
				}
			}
		}
	}
	result["sessions_cleared"] = sessionsCleared

	// 8. Reset all active goals
	goalsReset := 0
	if al.memDB != nil {
		gm := autonomy.NewGoalManager(al.memDB)
		for _, agentID := range al.getRegistry().ListAgentIDs() {
			goals, err := gm.ListAllGoals(agentID)
			if err != nil {
				continue
			}
			for _, g := range goals {
				if g.Status == autonomy.GoalStatusActive || g.Status == autonomy.GoalStatusPaused {
					if _, err := gm.UpdateGoalStatus(g.ID, autonomy.GoalStatusCompleted); err == nil {
						goalsReset++
					}
				}
			}
		}
	}
	result["goals_reset"] = goalsReset

	// 9. Clear active plan
	if al.planManager != nil {
		al.planManager.ClearPlan()
	}
	result["plan_cleared"] = true

	// 10. Reset status
	al.activeStatus.Store("Idle")
	al.activeAgentID.Store("")

	// ── Lift the killswitch so the system can accept new work ──
	al.killed.Store(false)

	logger.InfoCF("agent", "Reset: KILLSWITCH complete — system ready", result)
	return result
}

// ReloadAgents reloads the agent registry and shared tools from the current config.
func (al *AgentLoop) ReloadAgents() {
	logger.InfoCF("agent", "Reloading agents from config", nil)

	// Create a new provider from the updated config every time.
	// This ensures changes to the default model or provider keys take effect immediately
	// without requiring a full process restart.
	provider, _, err := providers.CreateProvider(al.cfg)
	if err != nil {
		logger.ErrorCF("agent", "Cannot reload agents: provider creation failed", map[string]any{"error": err.Error()})
		// Fallback to existing provider if creation fails, so we don't crash
		if defaultAgent := al.getRegistry().GetDefaultAgent(); defaultAgent != nil {
			provider = defaultAgent.Provider
		}
	} else if provider == nil {
		logger.WarnCF("agent", "Cannot reload agents: no model configured", nil)
		// Fallback to existing
		if defaultAgent := al.getRegistry().GetDefaultAgent(); defaultAgent != nil {
			provider = defaultAgent.Provider
		}
	} else {
		logger.InfoCF("agent", "Created provider from updated config",
			map[string]any{"model": al.cfg.Agents.Defaults.GetModelName()})
	}

	newRegistry := NewAgentRegistry(al.cfg, provider, al.memDB)

	// Re-register new agents with the A2A router
	for _, id := range newRegistry.ListAgentIDs() {
		al.a2aRouter.Register(id)
	}

	toolStatsPath := filepath.Join(filepath.Dir(al.memDB.Path()), "tool_stats.json")
	var newToolTracker *tools.ToolTracker
	if al.registry != nil {
		newToolTracker = tools.NewToolTracker(toolStatsPath)
	}

	registerSharedTools(
		al.cfg,
		al.bus,
		newRegistry,
		al.runSpawnedTaskAsAgent,
		al.planManager,
		al.scratchpad,
		al.checkpointMgr,
		al.memDB,
		al.a2aRouter,
		newToolTracker,
		al.torService,
	)

	al.registryMu.Lock()
	al.registry = newRegistry
	al.toolTracker = newToolTracker
	al.registryMu.Unlock()

	al.stopAutonomyServices()
	al.startAutonomyServices(provider, al.pushService)

	// Hot-swap the model on the evolution engine so background evolution
	// loops use the new model without tearing down their running state.
	if al.evolutionEngine != nil && provider != nil {
		evoModel := al.cfg.Agents.Defaults.GetModelName()
		if mainAgent, ok := newRegistry.GetAgent("main"); ok && mainAgent.Model != "" {
			evoModel = mainAgent.Model
		}
		if al.cfg.Evolution.Model != "" {
			evoModel = al.cfg.Evolution.Model
		}
		al.evolutionEngine.SetProvider(provider, evoModel)
		logger.InfoCF("agent", "Evolution engine model updated", map[string]any{"model": evoModel})
	}

	logger.InfoCF("agent", "Agents reloaded successfully", nil)
}

// GetEvolutionEngine returns the evolution engine instance (may be nil).
func (al *AgentLoop) GetEvolutionEngine() *evolution.EvolutionEngine {
	return al.evolutionEngine
}
