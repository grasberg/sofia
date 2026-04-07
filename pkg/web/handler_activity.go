package web

import (
	"encoding/json"
	"net/http"

	"github.com/grasberg/sofia/pkg/autonomy"
	"github.com/grasberg/sofia/pkg/tools"
)

// handleActivity returns a snapshot of all active goal work across agents.
func (s *Server) handleActivity(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.sendJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	pm := s.agentLoop.GetPlanManager()
	subagentTasks := s.agentLoop.GetActiveSubagentTasks()

	taskByLabel := make(map[string]map[string]any)
	for _, t := range subagentTasks {
		if label, ok := t["label"].(string); ok {
			taskByLabel[label] = t
		}
	}

	goals, err := s.agentLoop.ListGoals("")
	if err != nil {
		s.sendJSONError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var agents []map[string]any
	for _, g := range goals {
		if g.Status != autonomy.GoalStatusInProgress {
			continue
		}

		entry := map[string]any{
			"agent_id":  g.AgentID,
			"goal_id":   g.ID,
			"goal_name": g.Name,
		}

		if pm != nil {
			if plan := pm.GetPlanByGoalID(g.ID); plan != nil {
				entry["plan_id"] = plan.ID

				var activeTasks []map[string]any
				pending, completed, total := 0, 0, len(plan.Steps)

				for _, step := range plan.Steps {
					switch step.Status {
					case tools.PlanStatusPending:
						pending++
					case tools.PlanStatusCompleted:
						completed++
					case tools.PlanStatusInProgress:
						taskInfo := map[string]any{
							"step_index":  step.Index,
							"description": step.Description,
							"status":      "running",
							"assigned_to": step.AssignedTo,
						}
						label := step.AssignedTo
						if t, ok := taskByLabel[label]; ok {
							taskInfo["subagent_id"] = t["subagent_id"]
							taskInfo["created"] = t["created"]
						}
						activeTasks = append(activeTasks, taskInfo)
					case tools.PlanStatusFailed:
						// counted but not shown as active
					}
				}

				entry["active_tasks"] = activeTasks
				entry["pending_tasks"] = pending
				entry["completed_tasks"] = completed
				entry["total_tasks"] = total
			}
		}

		agents = append(agents, entry)
	}

	if agents == nil {
		agents = []map[string]any{}
	}

	result := map[string]any{"agents": agents}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}
