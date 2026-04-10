package web

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/grasberg/sofia/pkg/autonomy"
	"github.com/grasberg/sofia/pkg/memory"
)

func (s *Server) handleGoals(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.handleGoalsGet(w, r)
	case http.MethodPatch:
		s.handleGoalsPatch(w, r)
	case http.MethodDelete:
		s.handleGoalsDelete(w, r)
	default:
		s.sendJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleGoalsGet(w http.ResponseWriter, r *http.Request) {
	agentID := r.URL.Query().Get("agent_id")
	goals, err := s.agentLoop.ListGoals(agentID)
	if err != nil {
		s.sendJSONError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if goals == nil {
		goals = make([]*autonomy.Goal, 0)
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(goals)
}

func (s *Server) handleGoalsPatch(w http.ResponseWriter, r *http.Request) {
	var req struct {
		GoalID int64  `json:"goal_id"`
		Status string `json:"status"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.sendJSONError(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	if req.GoalID == 0 {
		s.sendJSONError(w, "goal_id is required", http.StatusBadRequest)
		return
	}
	validStatuses := map[string]bool{
		autonomy.GoalStatusPaused:     true,
		autonomy.GoalStatusFailed:     true,
		autonomy.GoalStatusActive:     true,
		autonomy.GoalStatusInProgress: true,
	}
	if !validStatuses[req.Status] {
		s.sendJSONError(w, "status must be paused, failed, active, or in_progress", http.StatusBadRequest)
		return
	}

	if err := s.agentLoop.UpdateGoalStatus(req.GoalID, req.Status); err != nil {
		s.sendJSONError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Broadcast to connected dashboards
	if hub := s.agentLoop.DashboardHub(); hub != nil {
		hub.Broadcast(map[string]any{
			"type":    "goal_status_changed",
			"goal_id": req.GoalID,
			"status":  req.Status,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (s *Server) handleGoalRestart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.sendJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		GoalID int64 `json:"goal_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.sendJSONError(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	if req.GoalID == 0 {
		s.sendJSONError(w, "goal_id is required", http.StatusBadRequest)
		return
	}

	if err := s.agentLoop.RestartGoal(req.GoalID); err != nil {
		s.sendJSONError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if hub := s.agentLoop.DashboardHub(); hub != nil {
		hub.Broadcast(map[string]any{
			"type":    "goal_status_changed",
			"goal_id": req.GoalID,
			"status":  "active",
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (s *Server) handleGoalsDelete(w http.ResponseWriter, r *http.Request) {
	idStr := r.URL.Query().Get("goal_id")
	if idStr == "" {
		s.sendJSONError(w, "goal_id query parameter is required", http.StatusBadRequest)
		return
	}
	goalID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		s.sendJSONError(w, "Invalid goal_id", http.StatusBadRequest)
		return
	}

	if err := s.agentLoop.DeleteGoal(goalID); err != nil {
		s.sendJSONError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Broadcast to connected dashboards
	if hub := s.agentLoop.DashboardHub(); hub != nil {
		hub.Broadcast(map[string]any{
			"type":    "goal_deleted",
			"goal_id": goalID,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// handleGoalLog handles GET /api/goals/{id}/log — returns step history for a goal.
func (s *Server) handleGoalLog(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.sendJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract goal ID from path: /api/goals/{id}/log
	path := strings.TrimPrefix(r.URL.Path, "/api/goals/")
	parts := strings.SplitN(path, "/", 2)
	if len(parts) == 0 || parts[0] == "" {
		s.sendJSONError(w, "Goal ID is required", http.StatusBadRequest)
		return
	}

	goalID, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		s.sendJSONError(w, "Invalid goal ID", http.StatusBadRequest)
		return
	}

	memDB := s.agentLoop.GetMemoryDB()
	if memDB == nil {
		s.sendJSONError(w, "Memory database not available", http.StatusServiceUnavailable)
		return
	}

	entries, err := memDB.GetGoalLog(goalID)
	if err != nil {
		s.sendJSONError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if entries == nil {
		entries = []memory.GoalLogEntry{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(entries)
}

// handlePlans returns all plans (active + completed + failed) as JSON.
func (s *Server) handlePlans(w http.ResponseWriter, _ *http.Request) {
	plans := s.agentLoop.GetAllPlans()
	w.Header().Set("Content-Type", "application/json")
	if plans == nil {
		w.Write([]byte("[]"))
		return
	}
	json.NewEncoder(w).Encode(plans)
}

// handleGoalsCompleted returns completed goals with full execution logs and plan data.
func (s *Server) handleGoalsCompleted(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.sendJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	goals, err := s.agentLoop.ListGoals("")
	if err != nil {
		s.sendJSONError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	memDB := s.agentLoop.GetMemoryDB()
	pm := s.agentLoop.GetPlanManager()

	var completed []map[string]any
	for _, g := range goals {
		if g.Status != autonomy.GoalStatusCompleted {
			continue
		}

		entry := map[string]any{
			"id":          g.ID,
			"name":        g.Name,
			"description": g.Description,
			"priority":    g.Priority,
			"result":      g.Result,
			"goal_result": g.GoalResult,
			"created_at":  g.CreatedAt,
			"updated_at":  g.UpdatedAt,
		}

		if pm != nil {
			if plan := pm.GetPlanByGoalID(g.ID); plan != nil {
				steps := make([]map[string]any, len(plan.Steps))
				for i, step := range plan.Steps {
					steps[i] = map[string]any{
						"index":       step.Index,
						"description": step.Description,
						"status":      string(step.Status),
						"result":      step.Result,
						"assigned_to": step.AssignedTo,
						"depends_on":  step.DependsOn,
					}
				}
				entry["plan"] = map[string]any{
					"id":     plan.ID,
					"status": string(plan.Status),
					"steps":  steps,
				}
			}
		}

		if memDB != nil {
			entries, logErr := memDB.GetGoalLog(g.ID)
			if logErr == nil && entries != nil {
				entry["log"] = entries
			}
		}

		completed = append(completed, entry)
	}

	if completed == nil {
		completed = []map[string]any{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(completed)
}
