package web

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/grasberg/sofia/pkg/autonomy"
	"github.com/grasberg/sofia/pkg/logger"
	"github.com/grasberg/sofia/pkg/memory"
	"github.com/grasberg/sofia/pkg/tools"
)

func (s *Server) handleGoals(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.handleGoalsGet(w, r)
	case http.MethodPost:
		s.handleGoalsPost(w, r)
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

// handleGoalsPost creates a goal directly (bypassing LLM chat orchestration)
// and triggers immediate autonomy plan generation.
func (s *Server) handleGoalsPost(w http.ResponseWriter, r *http.Request) {
	limitBody(r)
	var req struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		Priority    string `json:"priority"`
		AgentCount  int    `json:"agent_count"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.sendJSONError(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	if req.Name == "" {
		s.sendJSONError(w, "name is required", http.StatusBadRequest)
		return
	}
	if req.Description == "" {
		req.Description = req.Name
	}
	if req.Priority == "" {
		req.Priority = "medium"
	}

	goal, err := s.agentLoop.CreateGoal(req.Name, req.Description, req.Priority, req.AgentCount)
	if err != nil {
		logger.WarnCF("web", "Goal creation failed", map[string]any{"error": err.Error()})
		s.sendJSONError(w, "Failed to create goal", http.StatusInternalServerError)
		return
	}

	// Broadcast goal creation to connected dashboards.
	if hub := s.agentLoop.DashboardHub(); hub != nil {
		hub.Broadcast(map[string]any{
			"type":       "goal_plan_created",
			"goal_id":    goal.ID,
			"goal_name":  goal.Name,
			"step_count": 0,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(goal)
}

func (s *Server) handleGoalsPatch(w http.ResponseWriter, r *http.Request) {
	limitBody(r)
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
		logger.WarnCF("web", "Goal status update failed", map[string]any{"goal_id": req.GoalID, "error": err.Error()})
		s.sendJSONError(w, "Failed to update goal status", http.StatusInternalServerError)
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
	limitBody(r)
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
		logger.WarnCF("web", "Goal restart failed", map[string]any{"goal_id": req.GoalID, "error": err.Error()})
		s.sendJSONError(w, "Failed to restart goal", http.StatusInternalServerError)
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
		logger.WarnCF("web", "Goal delete failed", map[string]any{"goal_id": goalID, "error": err.Error()})
		s.sendJSONError(w, "Failed to delete goal", http.StatusInternalServerError)
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

// handleGoalSubroute dispatches /api/goals/{id}/log, /api/goals/{id}/timeline, and /api/goals/preview.
func (s *Server) handleGoalSubroute(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/goals/")

	// Handle /api/goals/preview (POST)
	if path == "preview" {
		if r.Method == http.MethodPost {
			s.handleGoalPreview(w, r)
		} else {
			s.sendJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
		return
	}

	if r.Method != http.MethodGet {
		s.sendJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract goal ID and action from path: /api/goals/{id}/{action}
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

	action := ""
	if len(parts) > 1 {
		action = parts[1]
	}

	switch action {
	case "timeline":
		s.handleGoalTimeline(w, goalID)
	default:
		s.handleGoalLogEntries(w, goalID)
	}
}

// handleGoalLogEntries returns step history for a goal.
func (s *Server) handleGoalLogEntries(w http.ResponseWriter, goalID int64) {
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

// handleGoalTimeline returns goal + plan + log in a single payload for the timeline view.
func (s *Server) handleGoalTimeline(w http.ResponseWriter, goalID int64) {
	memDB := s.agentLoop.GetMemoryDB()
	if memDB == nil {
		s.sendJSONError(w, "Memory database not available", http.StatusServiceUnavailable)
		return
	}

	gm := autonomy.NewGoalManager(memDB)
	goal, err := gm.GetGoalByID(goalID)
	if err != nil {
		s.sendJSONError(w, "Goal not found", http.StatusNotFound)
		return
	}

	result := map[string]any{
		"goal": goal,
	}

	// Attach plan with step details.
	pm := s.agentLoop.GetPlanManager()
	if pm != nil {
		if plan := pm.GetPlanByGoalID(goalID); plan != nil {
			steps := make([]map[string]any, len(plan.Steps))
			for i, step := range plan.Steps {
				steps[i] = map[string]any{
					"index":               step.Index,
					"description":         step.Description,
					"status":              string(step.Status),
					"result":              step.Result,
					"assigned_to":         step.AssignedTo,
					"acceptance_criteria": step.AcceptanceCriteria,
					"verify_command":      step.VerifyCommand,
					"verify_result":       step.VerifyResult,
					"depends_on":          step.DependsOn,
					"retry_count":         step.RetryCount,
				}
			}
			result["plan"] = map[string]any{
				"id":     plan.ID,
				"status": string(plan.Status),
				"steps":  steps,
			}
		}
	}

	// Attach execution log.
	entries, err := memDB.GetGoalLog(goalID)
	if err == nil && entries != nil {
		result["log"] = entries
	} else {
		result["log"] = []memory.GoalLogEntry{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
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

	// Pre-index plans by goalID to avoid N+1 lookups inside the loop.
	var plansByGoalID map[int64]*tools.Plan
	if pm != nil {
		allPlans := pm.ListAllPlans()
		plansByGoalID = make(map[int64]*tools.Plan, len(allPlans))
		for _, p := range allPlans {
			if p.GoalID != 0 {
				plansByGoalID[p.GoalID] = p
			}
		}
	}

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

		if plan := plansByGoalID[g.ID]; plan != nil {
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

// handleGoalPreview returns a lightweight LLM estimate of plan complexity,
// step count, and estimated time for a goal description.
func (s *Server) handleGoalPreview(w http.ResponseWriter, r *http.Request) {
	limitBody(r)
	var req struct {
		Description string `json:"description"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.sendJSONError(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	if req.Description == "" {
		s.sendJSONError(w, "description is required", http.StatusBadRequest)
		return
	}

	memDB := s.agentLoop.GetMemoryDB()
	if memDB == nil {
		s.sendJSONError(w, "Memory database not available", http.StatusServiceUnavailable)
		return
	}

	// Simple heuristic-based preview — no LLM call needed.
	desc := req.Description
	wordCount := len(strings.Fields(desc))
	estimatedSteps := 3 + wordCount/20
	if estimatedSteps > 10 {
		estimatedSteps = 10
	}
	if estimatedSteps < 3 {
		estimatedSteps = 3
	}

	complexity := "medium"
	if wordCount < 10 {
		complexity = "low"
		estimatedSteps = 3
	} else if wordCount > 50 {
		complexity = "high"
	}

	estimatedMinutes := estimatedSteps * 2 // rough: ~2 min per step

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"estimated_steps":   estimatedSteps,
		"complexity":        complexity,
		"estimated_minutes": estimatedMinutes,
	})
}
