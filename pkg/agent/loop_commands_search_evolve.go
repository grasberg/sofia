package agent

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/grasberg/sofia/pkg/memory"
	"github.com/grasberg/sofia/pkg/search"
)

// handleSearchCommand searches across all sessions for messages matching the query.
func (al *AgentLoop) handleSearchCommand(query string) string {
	if al.memDB == nil {
		return "Search unavailable: memory database not initialized."
	}

	dbRows, err := al.memDB.SearchMessages(query, 500)
	if err != nil {
		return fmt.Sprintf("Search failed: %v", err)
	}

	if len(dbRows) == 0 {
		return "No results found."
	}

	entries := dbRowsToSearchEntries(dbRows)
	results := search.KeywordSearch(query, entries, 10)
	if len(results) == 0 {
		return "No results found."
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "Search results for %q (%d matches):\n\n", query, len(results))
	for i, r := range results {
		preview := r.Content
		if len(preview) > 120 {
			preview = preview[:120] + "..."
		}
		fmt.Fprintf(&sb, "%d. [%s] %s (session: %s, score: %.0f%%)\n",
			i+1, r.Role, preview, r.SessionKey, r.Score*100)
	}
	return strings.TrimRight(sb.String(), "\n")
}

// dbRowsToSearchEntries converts memory.SearchMessageRow slices to search.MessageEntry slices.
func dbRowsToSearchEntries(rows []memory.SearchMessageRow) []search.MessageEntry {
	entries := make([]search.MessageEntry, len(rows))
	for i, r := range rows {
		entries[i] = search.MessageEntry{
			SessionKey: r.SessionKey,
			Content:    r.Content,
			Role:       r.Role,
			Timestamp:  r.CreatedAt,
		}
	}
	return entries
}

// handleEvolveCommand implements the /evolve family of session commands.
func (al *AgentLoop) handleEvolveCommand(args []string, _ string) (string, bool) {
	if al.evolutionEngine == nil {
		return "Evolution engine is not enabled. Set evolution.enabled=true in config.", true
	}
	if len(args) == 0 {
		return "Usage: /evolve status|history|run|pause|resume|revert <id>", true
	}
	switch args[0] {
	case "status":
		return al.evolutionEngine.FormatStatus(), true
	case "history":
		n := 10
		if len(args) > 1 {
			if parsed, err := strconv.Atoi(args[1]); err == nil && parsed > 0 {
				n = parsed
			}
		}
		entries, err := al.evolutionEngine.RecentHistory(n)
		if err != nil {
			return fmt.Sprintf("Error: %v", err), true
		}
		if len(entries) == 0 {
			return "No evolution history yet.", true
		}
		var sb strings.Builder
		sb.WriteString("Evolution History:\n")
		for _, e := range entries {
			outcome := ""
			if e.Outcome != "" {
				outcome = fmt.Sprintf(" [%s]", e.Outcome)
			}
			fmt.Fprintf(&sb, "  %s | %s | %s%s\n",
				e.Timestamp.Format("01-02 15:04"), e.Action, e.Summary, outcome)
		}
		return sb.String(), true
	case "run":
		if al.evolveRunning.Load() {
			return "An evolution cycle is already running.", true
		}
		al.evolveRunning.Store(true)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		go func() {
			defer cancel()
			defer al.evolveRunning.Store(false)
			al.evolutionEngine.RunNow(ctx)
		}()
		return "Evolution cycle triggered.", true
	case "pause":
		al.evolutionEngine.Pause()
		return "Evolution paused.", true
	case "resume":
		al.evolutionEngine.Resume()
		return "Evolution resumed.", true
	case "revert":
		if len(args) < 2 {
			return "Usage: /evolve revert <id>", true
		}
		if err := al.evolutionEngine.Revert(args[1]); err != nil {
			return fmt.Sprintf("Revert failed: %v", err), true
		}
		return fmt.Sprintf("Reverted action %s.", args[1]), true
	default:
		return "Unknown. Usage: /evolve status|history|run|pause|resume|revert <id>", true
	}
}

// handleOptimizePromptsCommand implements /optimize-prompts [agent_id].
func (al *AgentLoop) handleOptimizePromptsCommand(ctx context.Context, args []string) string {
	if !al.cfg.Agents.Defaults.PromptOptimization.Enabled {
		return "Prompt optimization is disabled. Enable it in config.json: agents.defaults.prompt_optimization.enabled = true"
	}

	agentID := ""
	if len(args) > 0 {
		agentID = args[0]
	}

	// Default to the default agent
	agent := al.getRegistry().GetDefaultAgent()
	if agentID != "" {
		if a, ok := al.getRegistry().GetAgent(agentID); ok {
			agent = a
		} else {
			return fmt.Sprintf("Agent %q not found.", agentID)
		}
	}
	if agent == nil {
		return "No agent available."
	}

	optimizer := NewPromptOptimizer(
		al.tracer, al.memDB, agent.Provider,
		al.cfg.Agents.Defaults.PromptOptimization,
	)

	// Step 1: Evaluate
	review, err := optimizer.Evaluate(agent.ID)
	if err != nil {
		return fmt.Sprintf("Evaluation failed: %v", err)
	}

	if !review.NeedsWork {
		return fmt.Sprintf(
			"Agent %s is performing well (avg score: %.2f). No optimization needed.",
			agent.ID,
			review.AvgScore,
		)
	}

	// Step 2: Get current system prompt
	currentPrompt := ""
	if agent.ContextBuilder != nil {
		currentPrompt = agent.ContextBuilder.BuildSystemPrompt()
	}
	if currentPrompt == "" {
		return "Could not read current system prompt for optimization."
	}

	// Step 3: Generate variants
	variants, err := optimizer.GenerateVariants(ctx, review, currentPrompt)
	if err != nil {
		return fmt.Sprintf("Variant generation failed: %v", err)
	}

	// Step 4: Create experiment
	exp, err := optimizer.CreateExperiment(agent.ID, variants)
	if err != nil {
		return fmt.Sprintf("Failed to create A/B experiment: %v", err)
	}

	return fmt.Sprintf(
		"Prompt optimization started for agent %s:\n"+
			"• Avg score: %.2f (threshold: %.2f)\n"+
			"• Low-scoring traces: %d\n"+
			"• Experiment: %s\n"+
			"• Variants: %d (including control)\n"+
			"• Trials needed: %d per variant\n\n"+
			"The experiment will run automatically. Check results with /evolve status.",
		agent.ID, review.AvgScore, al.cfg.Agents.Defaults.PromptOptimization.ScoreThreshold,
		len(review.LowTraces), exp.Name, len(variants),
		al.cfg.Agents.Defaults.PromptOptimization.TrialsPerVariant,
	)
}
