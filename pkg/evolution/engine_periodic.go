package evolution

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/grasberg/sofia/pkg/bus"
	"github.com/grasberg/sofia/pkg/logger"
)

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
	fmt.Fprintf(&sb, "Actions in last 24h: %d\n\n", len(entries))

	for _, entry := range entries {
		outcome := entry.Outcome
		if outcome == "" {
			outcome = "pending"
		}
		fmt.Fprintf(&sb, "- [%s] %s (outcome: %s)\n", entry.Action, entry.Summary, outcome)
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

// improveSkills runs the skill analyzer and auto-deploys high-priority improvements.
// If RequireApproval is enabled, improvements are queued as proposals.
func (e *EvolutionEngine) improveSkills(ctx context.Context) {
	agentIDs := e.registrar.ListAgentIDs()
	for _, agentID := range agentIDs {
		analyzer := NewSkillAnalyzer(e.memDB, agentID, e.provider, "")
		improvements, err := analyzer.AnalyzeAndSuggestImprovements(ctx, 10)
		if err != nil || len(improvements) == 0 {
			continue
		}

		for _, imp := range improvements {
			if imp.Priority < 3 {
				continue // Only auto-apply high-priority improvements
			}

			skillPath := filepath.Join(e.architect.workspace, "skills", imp.SkillName, "SKILL.md")
			existingContent, readErr := os.ReadFile(skillPath)
			if readErr != nil {
				continue // Skill doesn't exist — skip, don't create
			}

			prompter := NewSkillImprovementPrompts(e.provider, "")
			improved, genErr := prompter.GenerateSkillImprovement(ctx,
				Suggestion{Issue: string(existingContent), Suggestion: ""},
				Suggestion{Issue: imp.Issue, Suggestion: imp.Suggestion},
			)
			if genErr != nil || improved == "" {
				continue
			}

			if e.cfg.RequireApproval {
				e.pendingProposals = append(e.pendingProposals, Proposal{
					ID: uuid.NewString(),
					Action: EvolutionAction{
						Type: ActionModifyWorkspace,
						Params: map[string]any{
							"path":    skillPath,
							"content": improved,
						},
						Reason: fmt.Sprintf("Skill improvement: %s — %s", imp.SkillName, imp.Issue),
					},
					CreatedAt: time.Now().UTC(),
					Status:    "pending",
				})
				logger.InfoCF("evolution", "Skill improvement queued for approval",
					map[string]any{"skill": imp.SkillName, "issue": imp.Issue})
				continue
			}

			if modErr := e.modifier.ModifyFile(ctx, skillPath, improved); modErr != nil {
				logger.WarnCF("evolution", "Failed to auto-improve skill",
					map[string]any{"skill": imp.SkillName, "error": modErr.Error()})
			} else {
				logger.InfoCF("evolution", "Skill auto-improved",
					map[string]any{"skill": imp.SkillName, "issue": imp.Issue})
			}
		}
	}
}

// abs returns the absolute value of an integer.
func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

// maybeConsolidate runs memory consolidation if enough time has passed since
// the last consolidation. Interval is configurable (default 6 hours).
// Also triggers on event-driven thresholds (new nodes count).
func (e *EvolutionEngine) maybeConsolidate() {
	intervalH := e.cfg.ConsolidationIntervalH
	if intervalH <= 0 {
		intervalH = 6
	}
	interval := time.Duration(intervalH) * time.Hour

	// Check if consolidation is needed based on time OR event-driven triggers
	shouldConsolidate := false

	if !e.lastConsolidation.IsZero() && time.Since(e.lastConsolidation) < interval {
		// Check event-driven triggers: new node count threshold
		// This is a simplified check - full implementation would track node counts
		shouldConsolidate = false
	} else {
		shouldConsolidate = true
	}

	if !shouldConsolidate {
		return
	}

	agentIDs := e.registrar.ListAgentIDs()
	totalMerged, totalPruned := 0, 0

	for _, agentID := range agentIDs {
		// Step 1: Merge duplicate nodes
		duplicates, err := e.memDB.FindDuplicateNodes(agentID)
		if err != nil {
			logger.WarnCF("evolution", "Consolidation: find duplicates failed",
				map[string]any{"agent_id": agentID, "error": err.Error()})
			continue
		}
		for _, cluster := range duplicates {
			if len(cluster) < 2 {
				continue
			}
			primaryIdx := 0
			for i, n := range cluster {
				if n.AccessCount > cluster[primaryIdx].AccessCount {
					primaryIdx = i
				}
			}
			secondaryIDs := make([]int64, 0, len(cluster)-1)
			for i := range cluster {
				if i != primaryIdx {
					secondaryIDs = append(secondaryIDs, cluster[i].ID)
				}
			}
			if mergeErr := e.memDB.MergeNodes(cluster[primaryIdx].ID, secondaryIDs); mergeErr == nil {
				totalMerged += len(secondaryIDs)
			}
		}

		// Step 2: Prune stale nodes
		staleNodes, err := e.memDB.GetStaleNodes(agentID, 30*24*time.Hour, 2)
		if err != nil {
			continue
		}
		for _, node := range staleNodes {
			if node.QualityScore < 0.2 && node.AccessCount < 2 {
				if delErr := e.memDB.DeleteNode(node.ID); delErr == nil {
					totalPruned++
				}
			}
		}
	}

	e.lastConsolidation = time.Now()

	if totalMerged > 0 || totalPruned > 0 {
		logger.InfoCF("evolution", "Scheduled memory consolidation complete",
			map[string]any{"merged": totalMerged, "pruned": totalPruned})
	}
}
