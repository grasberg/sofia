package web

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/grasberg/sofia/pkg/agent"
	"github.com/grasberg/sofia/pkg/autonomy"
	"github.com/grasberg/sofia/pkg/bus"
	"github.com/grasberg/sofia/pkg/config"
	"github.com/grasberg/sofia/pkg/dashboard"
	"github.com/grasberg/sofia/pkg/memory"
	"github.com/grasberg/sofia/pkg/providers"
	"github.com/grasberg/sofia/pkg/tools"
)

// newTestLoopServer creates a Server backed by a real AgentLoop with an
// in-memory database. This allows testing the goals API handlers end-to-end.
func newTestLoopServer(t *testing.T) (*Server, *memory.MemoryDB) {
	t.Helper()

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	cfg := &config.Config{
		MemoryDB: dbPath,
		WebUI: config.WebUIConfig{
			Enabled: true,
			Host:    "127.0.0.1",
		},
		Agents: config.AgentsConfig{
			Defaults: config.AgentDefaults{
				Workspace:         tmpDir,
				Model:             "mock",
				MaxTokens:         4096,
				MaxToolIterations: 5,
			},
			List: []config.AgentConfig{
				{ID: "main", Default: true},
			},
		},
	}

	msgBus := bus.NewMessageBus()
	provider := &mockProvider{}
	loop := agent.NewAgentLoop(cfg, msgBus, provider)

	s := &Server{
		cfg:       cfg,
		agentLoop: loop,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/api/goals", s.authMiddleware(s.handleGoals))
	mux.HandleFunc("/api/goals/completed", s.authMiddleware(s.handleGoalsCompleted))
	mux.HandleFunc("/api/goals/restart", s.authMiddleware(s.handleGoalRestart))
	mux.HandleFunc("/api/goals/", s.authMiddleware(s.handleGoalSubroute))
	mux.HandleFunc("/api/plans", s.authMiddleware(s.handlePlans))
	s.mux = mux

	return s, loop.GetMemoryDB()
}

type mockProvider struct{}

func (m *mockProvider) Chat(
	_ context.Context, _ []providers.Message, _ []providers.ToolDefinition,
	_ string, _ map[string]any,
) (*providers.LLMResponse, error) {
	return &providers.LLMResponse{Content: "mock"}, nil
}

func (m *mockProvider) GetDefaultModel() string { return "mock" }

// addTestGoal creates a goal via GoalManager and returns its ID.
func addTestGoal(t *testing.T, db *memory.MemoryDB, agentID, name, desc, priority string) int64 {
	t.Helper()
	gm := autonomy.NewGoalManager(db)
	gAny, err := gm.AddGoal(agentID, name, desc, priority)
	if err != nil {
		t.Fatalf("AddGoal failed: %v", err)
	}
	goal := gAny.(*autonomy.Goal)
	return goal.ID
}

// --- Goals GET ---

func TestHandleGoals_GET_Empty(t *testing.T) {
	s, _ := newTestLoopServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/goals", nil)
	w := httptest.NewRecorder()
	s.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var goals []autonomy.Goal
	if err := json.NewDecoder(w.Body).Decode(&goals); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if len(goals) != 0 {
		t.Errorf("expected 0 goals, got %d", len(goals))
	}
}

func TestHandleGoals_GET_WithGoals(t *testing.T) {
	s, db := newTestLoopServer(t)

	addTestGoal(t, db, "main", "Goal A", "Build something", "high")
	addTestGoal(t, db, "main", "Goal B", "Test something", "low")

	req := httptest.NewRequest(http.MethodGet, "/api/goals", nil)
	w := httptest.NewRecorder()
	s.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var goals []autonomy.Goal
	json.NewDecoder(w.Body).Decode(&goals)
	if len(goals) != 2 {
		t.Errorf("expected 2 goals, got %d", len(goals))
	}
}

func TestHandleGoals_GET_FilterByAgent(t *testing.T) {
	s, db := newTestLoopServer(t)

	addTestGoal(t, db, "agent-1", "Goal A", "desc", "high")
	addTestGoal(t, db, "agent-2", "Goal B", "desc", "low")

	req := httptest.NewRequest(http.MethodGet, "/api/goals?agent_id=agent-1", nil)
	w := httptest.NewRecorder()
	s.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var goals []autonomy.Goal
	json.NewDecoder(w.Body).Decode(&goals)
	if len(goals) != 1 {
		t.Errorf("expected 1 goal for agent-1, got %d", len(goals))
	}
}

// --- Goals PATCH (status update) ---

func TestHandleGoals_PATCH_UpdateStatus(t *testing.T) {
	s, db := newTestLoopServer(t)
	goalID := addTestGoal(t, db, "main", "Pause Me", "desc", "medium")

	body := `{"goal_id": ` + itoa(goalID) + `, "status": "paused"}`
	req := httptest.NewRequest(http.MethodPatch, "/api/goals", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Requested-With", "XMLHttpRequest")
	w := httptest.NewRecorder()
	s.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// Verify the status was updated
	gm := autonomy.NewGoalManager(db)
	goal, _ := gm.GetGoalByID(goalID)
	if goal.Status != autonomy.GoalStatusPaused {
		t.Errorf("expected status 'paused', got %q", goal.Status)
	}
}

func TestHandleGoals_PATCH_InvalidStatus(t *testing.T) {
	s, db := newTestLoopServer(t)
	goalID := addTestGoal(t, db, "main", "Test", "desc", "low")

	body := `{"goal_id": ` + itoa(goalID) + `, "status": "invalid"}`
	req := httptest.NewRequest(http.MethodPatch, "/api/goals", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Requested-With", "XMLHttpRequest")
	w := httptest.NewRecorder()
	s.mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid status, got %d", w.Code)
	}
}

func TestHandleGoals_PATCH_MissingGoalID(t *testing.T) {
	s, _ := newTestLoopServer(t)

	body := `{"status": "paused"}`
	req := httptest.NewRequest(http.MethodPatch, "/api/goals", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Requested-With", "XMLHttpRequest")
	w := httptest.NewRecorder()
	s.mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for missing goal_id, got %d", w.Code)
	}
}

// --- Goals DELETE ---

func TestHandleGoals_DELETE(t *testing.T) {
	s, db := newTestLoopServer(t)
	goalID := addTestGoal(t, db, "main", "Delete Me", "desc", "low")

	req := httptest.NewRequest(http.MethodDelete, "/api/goals?goal_id="+itoa(goalID), nil)
	req.Header.Set("X-Requested-With", "XMLHttpRequest")
	w := httptest.NewRecorder()
	s.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// Verify deletion
	gm := autonomy.NewGoalManager(db)
	_, err := gm.GetGoalByID(goalID)
	if err == nil {
		t.Error("expected goal to be deleted")
	}
}

func TestHandleGoals_DELETE_MissingID(t *testing.T) {
	s, _ := newTestLoopServer(t)

	req := httptest.NewRequest(http.MethodDelete, "/api/goals", nil)
	req.Header.Set("X-Requested-With", "XMLHttpRequest")
	w := httptest.NewRecorder()
	s.mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for missing goal_id, got %d", w.Code)
	}
}

func TestHandleGoals_DELETE_InvalidID(t *testing.T) {
	s, _ := newTestLoopServer(t)

	req := httptest.NewRequest(http.MethodDelete, "/api/goals?goal_id=abc", nil)
	req.Header.Set("X-Requested-With", "XMLHttpRequest")
	w := httptest.NewRecorder()
	s.mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid goal_id, got %d", w.Code)
	}
}

// --- Goals POST (create goal directly) ---

func TestHandleGoals_POST_CreateGoal(t *testing.T) {
	s, db := newTestLoopServer(t)

	body := `{"name":"Build REST API","description":"Build a REST API with CRUD endpoints","priority":"high","agent_count":3}`
	req := httptest.NewRequest(http.MethodPost, "/api/goals", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Requested-With", "fetch")
	w := httptest.NewRecorder()
	s.mux.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var goal autonomy.Goal
	if err := json.NewDecoder(w.Body).Decode(&goal); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if goal.Name != "Build REST API" {
		t.Errorf("expected name 'Build REST API', got %q", goal.Name)
	}
	if goal.Priority != "high" {
		t.Errorf("expected priority 'high', got %q", goal.Priority)
	}
	if goal.Status != autonomy.GoalStatusActive {
		t.Errorf("expected status 'active', got %q", goal.Status)
	}

	// Verify persisted in DB
	gm := autonomy.NewGoalManager(db)
	stored, err := gm.GetGoalByID(goal.ID)
	if err != nil {
		t.Fatalf("goal not found in DB: %v", err)
	}
	if stored.Name != "Build REST API" {
		t.Errorf("stored name mismatch: %q", stored.Name)
	}
}

func TestHandleGoals_POST_DefaultsPriorityAndDescription(t *testing.T) {
	s, _ := newTestLoopServer(t)

	body := `{"name":"Simple goal"}`
	req := httptest.NewRequest(http.MethodPost, "/api/goals", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Requested-With", "fetch")
	w := httptest.NewRecorder()
	s.mux.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var goal autonomy.Goal
	json.NewDecoder(w.Body).Decode(&goal)
	if goal.Priority != "medium" {
		t.Errorf("expected default priority 'medium', got %q", goal.Priority)
	}
	if goal.Description != "Simple goal" {
		t.Errorf("expected description to default to name, got %q", goal.Description)
	}
}

func TestHandleGoals_POST_MissingName(t *testing.T) {
	s, _ := newTestLoopServer(t)

	body := `{"description":"No name"}`
	req := httptest.NewRequest(http.MethodPost, "/api/goals", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Requested-With", "fetch")
	w := httptest.NewRecorder()
	s.mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for missing name, got %d", w.Code)
	}
}

func TestHandleGoals_POST_InvalidJSON(t *testing.T) {
	s, _ := newTestLoopServer(t)

	req := httptest.NewRequest(http.MethodPost, "/api/goals", strings.NewReader("{bad json"))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Requested-With", "fetch")
	w := httptest.NewRecorder()
	s.mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid JSON, got %d", w.Code)
	}
}

func TestHandleGoals_POST_AgentCountPersisted(t *testing.T) {
	s, db := newTestLoopServer(t)

	body := `{"name":"Parallel goal","description":"Run with 5 agents","priority":"medium","agent_count":5}`
	req := httptest.NewRequest(http.MethodPost, "/api/goals", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Requested-With", "fetch")
	w := httptest.NewRecorder()
	s.mux.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var goal autonomy.Goal
	json.NewDecoder(w.Body).Decode(&goal)

	gm := autonomy.NewGoalManager(db)
	stored, _ := gm.GetGoalByID(goal.ID)
	if stored.AgentCount != 5 {
		t.Errorf("expected agent_count=5, got %d", stored.AgentCount)
	}
}

// --- Goals preview ---

func TestHandleGoalPreview_ShortDescription(t *testing.T) {
	s, _ := newTestLoopServer(t)

	body := `{"description":"Write tests"}`
	req := httptest.NewRequest(http.MethodPost, "/api/goals/preview", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Requested-With", "fetch")
	w := httptest.NewRecorder()
	s.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var result map[string]any
	json.NewDecoder(w.Body).Decode(&result)
	if result["complexity"] != "low" {
		t.Errorf("expected complexity 'low' for short description, got %v", result["complexity"])
	}
	if result["estimated_steps"] != float64(3) {
		t.Errorf("expected 3 steps for short description, got %v", result["estimated_steps"])
	}
}

func TestHandleGoalPreview_LongDescription(t *testing.T) {
	s, _ := newTestLoopServer(t)

	words := make([]string, 60)
	for i := range words {
		words[i] = "word"
	}
	desc := strings.Join(words, " ")

	body := `{"description":"` + desc + `"}`
	req := httptest.NewRequest(http.MethodPost, "/api/goals/preview", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Requested-With", "fetch")
	w := httptest.NewRecorder()
	s.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var result map[string]any
	json.NewDecoder(w.Body).Decode(&result)
	if result["complexity"] != "high" {
		t.Errorf("expected complexity 'high' for long description, got %v", result["complexity"])
	}
}

func TestHandleGoalPreview_MissingDescription(t *testing.T) {
	s, _ := newTestLoopServer(t)

	body := `{}`
	req := httptest.NewRequest(http.MethodPost, "/api/goals/preview", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Requested-With", "fetch")
	w := httptest.NewRecorder()
	s.mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for missing description, got %d", w.Code)
	}
}

func TestHandleGoalPreview_InvalidJSON(t *testing.T) {
	s, _ := newTestLoopServer(t)

	req := httptest.NewRequest(http.MethodPost, "/api/goals/preview", strings.NewReader("{bad"))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Requested-With", "fetch")
	w := httptest.NewRecorder()
	s.mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid JSON, got %d", w.Code)
	}
}

// --- Goals timeline ---

func TestHandleGoalTimeline_ExistingGoal(t *testing.T) {
	s, db := newTestLoopServer(t)
	goalID := addTestGoal(t, db, "main", "Timeline Goal", "desc", "high")

	req := httptest.NewRequest(http.MethodGet, "/api/goals/"+itoa(goalID)+"/timeline", nil)
	req.Header.Set("X-Requested-With", "fetch")
	w := httptest.NewRecorder()
	s.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var result map[string]any
	json.NewDecoder(w.Body).Decode(&result)
	goalData, ok := result["goal"].(map[string]any)
	if !ok {
		t.Fatal("expected 'goal' key in timeline response")
	}
	if goalData["name"] != "Timeline Goal" {
		t.Errorf("expected goal name 'Timeline Goal', got %v", goalData["name"])
	}
}

func TestHandleGoalTimeline_NonexistentGoal(t *testing.T) {
	s, _ := newTestLoopServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/goals/99999/timeline", nil)
	req.Header.Set("X-Requested-With", "fetch")
	w := httptest.NewRecorder()
	s.mux.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404 for nonexistent goal, got %d", w.Code)
	}
}

// --- Goals completed ---

func TestHandleGoalsCompleted_Empty(t *testing.T) {
	s, _ := newTestLoopServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/goals/completed", nil)
	w := httptest.NewRecorder()
	s.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var completed []map[string]any
	json.NewDecoder(w.Body).Decode(&completed)
	if len(completed) != 0 {
		t.Errorf("expected 0 completed, got %d", len(completed))
	}
}

func TestHandleGoalsCompleted_WithCompletedGoal(t *testing.T) {
	s, db := newTestLoopServer(t)
	goalID := addTestGoal(t, db, "main", "Done Goal", "finished work", "high")

	gm := autonomy.NewGoalManager(db)
	gm.UpdateGoalStatus(goalID, autonomy.GoalStatusCompleted)

	req := httptest.NewRequest(http.MethodGet, "/api/goals/completed", nil)
	w := httptest.NewRecorder()
	s.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var completed []map[string]any
	json.NewDecoder(w.Body).Decode(&completed)
	if len(completed) != 1 {
		t.Fatalf("expected 1 completed, got %d", len(completed))
	}
	if completed[0]["name"] != "Done Goal" {
		t.Errorf("expected name 'Done Goal', got %v", completed[0]["name"])
	}
}

// --- Plans ---

func TestHandlePlans_Empty(t *testing.T) {
	s, _ := newTestLoopServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/plans", nil)
	w := httptest.NewRecorder()
	s.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

// --- Method not allowed ---

func TestHandleGoals_MethodNotAllowed(t *testing.T) {
	s, _ := newTestLoopServer(t)

	req := httptest.NewRequest(http.MethodPut, "/api/goals", nil)
	req.Header.Set("X-Requested-With", "XMLHttpRequest")
	w := httptest.NewRecorder()
	s.mux.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", w.Code)
	}
}

// --- Helpers ---

func itoa(id int64) string {
	return fmt.Sprintf("%d", id)
}

// Ensure tools and dashboard are importable for type references.
var _ *tools.PlanManager
var _ *dashboard.Hub
