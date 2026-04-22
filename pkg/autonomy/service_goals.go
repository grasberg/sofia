package autonomy

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/grasberg/sofia/pkg/logger"
	"github.com/grasberg/sofia/pkg/tools"
)

// goalPriorityOrder maps priority strings to sort order.
var goalPriorityOrder = map[string]int{"high": 0, "medium": 1, "low": 2, "": 1}

// sortGoalsByPriority sorts goals so high-priority goals are processed first.
func sortGoalsByPriority(goals []*Goal) {
	sort.Slice(goals, func(i, j int) bool {
		pi := goalPriorityOrder[goals[i].Priority]
		pj := goalPriorityOrder[goals[j].Priority]
		return pi < pj
	})
}

// finalizeCompletedGoals scans active/in-progress goals and finalizes any
// whose plans are fully completed. This runs even when the autonomy goals
// flag is off, because goals created via the chat UI still need finalization.
func (s *Service) finalizeCompletedGoals(ctx context.Context) {
	s.mu.Lock()
	pm := s.planMgr
	s.mu.Unlock()
	if pm == nil {
		return
	}

	gm := NewGoalManager(s.memDB)
	allGoals, err := gm.ListAllGoals(s.agentID)
	if err != nil {
		return
	}

	for _, goal := range allGoals {
		if goal.Status != GoalStatusActive && goal.Status != GoalStatusInProgress {
			continue
		}
		plan := pm.GetPlanByGoalID(goal.ID)
		if plan != nil && plan.Status == tools.PlanStatusCompleted {
			logger.InfoCF("autonomy", "Finalizing completed goal", map[string]any{
				"goal_id":   goal.ID,
				"goal_name": goal.Name,
			})
			s.finalizeGoal(ctx, gm, goal, plan)
		}
	}
}

// pursueGoals is the phased pipeline entry point: plan → implement.
// Goals are processed in priority order (high → medium → low).
func (s *Service) pursueGoals(ctx context.Context) {
	gm := NewGoalManager(s.memDB)

	allGoals, err := gm.ListAllGoals(s.agentID)
	if err != nil {
		logger.WarnCF("autonomy", "Failed to list goals", map[string]any{"error": err.Error()})
		return
	}

	sortGoalsByPriority(allGoals)

	for _, goal := range allGoals {
		if goal.Status != GoalStatusActive && goal.Status != GoalStatusInProgress {
			continue
		}
		select {
		case <-ctx.Done():
			return
		default:
		}

		phase := goal.Phase
		if phase == "" || phase == "specify" {
			phase = GoalPhasePlan
		}

		switch phase {
		case GoalPhasePlan:
			s.generatePlanForGoal(ctx, gm, goal)
		case GoalPhaseImplement:
			s.dispatchReadySteps(ctx, gm, goal)
		}
	}
}

// maxStepRetries returns the configured max retries, defaulting to 2.
func (s *Service) maxStepRetries() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cfg.MaxStepRetries > 0 {
		return s.cfg.MaxStepRetries
	}
	return 2
}

// maxAutoFixAttempts returns the configured max auto-fix attempts, defaulting to 2.
func (s *Service) maxAutoFixAttempts() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cfg.MaxAutoFixAttempts > 0 {
		return s.cfg.MaxAutoFixAttempts
	}
	return maxGoalAutoFixes
}

// NotifyGoalCreated triggers immediate plan generation for a newly created goal,
// bypassing the tick interval wait.
func (s *Service) NotifyGoalCreated(goalID int64) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				logger.ErrorCF("autonomy", "Panic in NotifyGoalCreated", map[string]any{
					"goal_id": goalID,
					"panic":   fmt.Sprintf("%v", r),
				})
			}
		}()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()

		gm := NewGoalManager(s.memDB)
		goal, err := gm.GetGoalByID(goalID)
		if err != nil || goal == nil {
			logger.WarnCF("autonomy", "NotifyGoalCreated: goal not found", map[string]any{
				"goal_id": goalID, "error": fmt.Sprintf("%v", err),
			})
			return
		}
		if goal.Status != GoalStatusActive {
			return
		}
		s.generatePlanForGoal(ctx, gm, goal)
	}()
}

// extractVerifyResult extracts the verification section from subagent output.
// Returns the verification text and whether verification passed.
func extractVerifyResult(output string) (verifyText string, passed bool) {
	const startMarker = "---VERIFICATION---"
	const endMarker = "---END VERIFICATION---"

	startIdx := strings.LastIndex(output, startMarker)
	if startIdx == -1 {
		return "", false
	}

	section := output[startIdx+len(startMarker):]
	endIdx := strings.Index(section, endMarker)
	if endIdx != -1 {
		section = section[:endIdx]
	}

	section = strings.TrimSpace(section)
	passed = strings.Contains(strings.ToUpper(section), "RESULT: PASS")
	return section, passed
}

func (s *Service) defaultGoalConcurrency() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cfg.DefaultGoalConcurrency > 0 {
		if s.cfg.DefaultGoalConcurrency > 10 {
			return 10
		}
		return s.cfg.DefaultGoalConcurrency
	}
	return 3
}

func (s *Service) stepBackoffBaseSec() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cfg.StepBackoffBaseSec > 0 {
		return s.cfg.StepBackoffBaseSec
	}
	return 10
}

func (s *Service) stepBackoffMaxSec() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cfg.StepBackoffMaxSec > 0 {
		return s.cfg.StepBackoffMaxSec
	}
	return 120
}

// completeGoal marks a goal as completed with phase update.
func (s *Service) completeGoal(gm *GoalManager, goal *Goal) {
	if _, err := gm.UpdateGoalStatus(goal.ID, GoalStatusCompleted); err != nil {
		logger.WarnCF("autonomy", "Failed to mark goal completed",
			map[string]any{"goal_id": goal.ID, "error": err.Error()})
	}
	_ = gm.UpdateGoalPhase(goal.ID, GoalPhaseCompleted)
}

// goalFolderPath returns the absolute path for a goal's working directory.
func (s *Service) goalFolderPath(goalID int64, goalName string) string {
	return filepath.Join(s.workspace, "goals", tools.GoalFolderName(goalID, goalName))
}

// ensureGoalFolder creates the goal folder if it doesn't exist.
func (s *Service) ensureGoalFolder(goalID int64, goalName string) string {
	dir := s.goalFolderPath(goalID, goalName)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		logger.WarnCF("autonomy", "Failed to create goal folder", map[string]any{
			"path":  dir,
			"error": err.Error(),
		})
	}
	return dir
}
