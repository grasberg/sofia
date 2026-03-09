package tools

import (
	"context"
	"testing"

	"github.com/grasberg/sofia/pkg/memory"
)

// helper to create a plan tool with in-memory DB for testing
func newTestPlanTool(t *testing.T) (*PlanTool, *PlanManager) {
	t.Helper()
	mgr := NewPlanManager()
	db, err := memory.Open(":memory:")
	if err != nil {
		t.Fatalf("failed to open memory DB: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	tool := NewPlanTool(mgr, db)
	return tool, mgr
}

// helper to create a plan and return the plan ID
func createTestPlan(t *testing.T, tool *PlanTool) string {
	t.Helper()
	result := tool.Execute(context.Background(), map[string]any{
		"operation": "create",
		"goal":      "Test goal",
		"steps":     []any{"Step 1", "Step 2", "Step 3"},
	})
	if result.IsError {
		t.Fatalf("create failed: %s", result.ForLLM)
	}
	return "plan-1"
}

// --- Existing functionality tests ---

func TestPlanCreate(t *testing.T) {
	tool, _ := newTestPlanTool(t)
	result := tool.Execute(context.Background(), map[string]any{
		"operation": "create",
		"goal":      "Build a website",
		"steps":     []any{"Design", "Implement", "Deploy"},
	})
	if result.IsError {
		t.Fatalf("expected success, got error: %s", result.ForLLM)
	}
	if !contains(result.ForLLM, "Build a website") {
		t.Errorf("expected goal in output, got: %s", result.ForLLM)
	}
	if !contains(result.ForLLM, "3. Deploy") {
		t.Errorf("expected step 3 in output, got: %s", result.ForLLM)
	}
}

func TestPlanCreateNoGoal(t *testing.T) {
	tool, _ := newTestPlanTool(t)
	result := tool.Execute(context.Background(), map[string]any{
		"operation": "create",
		"steps":     []any{"Step 1"},
	})
	if !result.IsError {
		t.Fatal("expected error for missing goal")
	}
}

func TestPlanCreateNoSteps(t *testing.T) {
	tool, _ := newTestPlanTool(t)
	result := tool.Execute(context.Background(), map[string]any{
		"operation": "create",
		"goal":      "Test",
	})
	if !result.IsError {
		t.Fatal("expected error for missing steps")
	}
}

func TestPlanUpdateStep(t *testing.T) {
	tool, _ := newTestPlanTool(t)
	createTestPlan(t, tool)

	result := tool.Execute(context.Background(), map[string]any{
		"operation":  "update_step",
		"plan_id":    "plan-1",
		"step_index": float64(0),
		"status":     "completed",
		"result":     "Done",
	})
	if result.IsError {
		t.Fatalf("update_step failed: %s", result.ForLLM)
	}
	if !contains(result.ForLLM, "[x]") {
		t.Errorf("expected completed icon, got: %s", result.ForLLM)
	}
}

func TestPlanGetStatus(t *testing.T) {
	tool, _ := newTestPlanTool(t)
	createTestPlan(t, tool)

	result := tool.Execute(context.Background(), map[string]any{
		"operation": "get_status",
	})
	if result.IsError {
		t.Fatalf("get_status failed: %s", result.ForLLM)
	}
	if !contains(result.ForLLM, "Test goal") {
		t.Errorf("expected goal in status output, got: %s", result.ForLLM)
	}
}

func TestPlanGetStatusNoPlans(t *testing.T) {
	tool, _ := newTestPlanTool(t)
	result := tool.Execute(context.Background(), map[string]any{
		"operation": "get_status",
	})
	if result.IsError {
		t.Fatal("get_status with no plans should not error")
	}
	if !contains(result.ForLLM, "No active plans") {
		t.Errorf("expected 'No active plans', got: %s", result.ForLLM)
	}
}

func TestPlanAutoComplete(t *testing.T) {
	tool, mgr := newTestPlanTool(t)
	createTestPlan(t, tool)

	// Complete all steps
	for i := 0; i < 3; i++ {
		tool.Execute(context.Background(), map[string]any{
			"operation":  "update_step",
			"plan_id":    "plan-1",
			"step_index": float64(i),
			"status":     "completed",
		})
	}

	plan := mgr.GetPlan("plan-1")
	if plan.Status != PlanStatusCompleted {
		t.Errorf("expected plan auto-completed, got status: %s", plan.Status)
	}
}

// --- Dynamic re-planning tests ---

func TestReplanInsert(t *testing.T) {
	tool, mgr := newTestPlanTool(t)
	createTestPlan(t, tool)

	result := tool.Execute(context.Background(), map[string]any{
		"operation":   "replan",
		"plan_id":     "plan-1",
		"action":      "insert",
		"step_index":  float64(1),
		"description": "New intermediate step",
	})
	if result.IsError {
		t.Fatalf("replan insert failed: %s", result.ForLLM)
	}

	plan := mgr.GetPlan("plan-1")
	if len(plan.Steps) != 4 {
		t.Fatalf("expected 4 steps, got %d", len(plan.Steps))
	}
	if plan.Steps[1].Description != "New intermediate step" {
		t.Errorf("expected inserted step at index 1, got: %s", plan.Steps[1].Description)
	}
	// Check reindexing
	for i, s := range plan.Steps {
		if s.Index != i {
			t.Errorf("step %d has index %d (expected %d)", i, s.Index, i)
		}
	}
}

func TestReplanInsertAtEnd(t *testing.T) {
	tool, mgr := newTestPlanTool(t)
	createTestPlan(t, tool)

	result := tool.Execute(context.Background(), map[string]any{
		"operation":   "replan",
		"plan_id":     "plan-1",
		"action":      "insert",
		"step_index":  float64(3), // at the end
		"description": "Final step",
	})
	if result.IsError {
		t.Fatalf("replan insert at end failed: %s", result.ForLLM)
	}

	plan := mgr.GetPlan("plan-1")
	if len(plan.Steps) != 4 {
		t.Fatalf("expected 4 steps, got %d", len(plan.Steps))
	}
	if plan.Steps[3].Description != "Final step" {
		t.Errorf("expected inserted step at end, got: %s", plan.Steps[3].Description)
	}
}

func TestReplanRemove(t *testing.T) {
	tool, mgr := newTestPlanTool(t)
	createTestPlan(t, tool)

	result := tool.Execute(context.Background(), map[string]any{
		"operation":  "replan",
		"plan_id":    "plan-1",
		"action":     "remove",
		"step_index": float64(1),
	})
	if result.IsError {
		t.Fatalf("replan remove failed: %s", result.ForLLM)
	}

	plan := mgr.GetPlan("plan-1")
	if len(plan.Steps) != 2 {
		t.Fatalf("expected 2 steps, got %d", len(plan.Steps))
	}
	if plan.Steps[1].Description != "Step 3" {
		t.Errorf("expected Step 3 at index 1, got: %s", plan.Steps[1].Description)
	}
}

func TestReplanRemoveLastStep(t *testing.T) {
	tool, _ := newTestPlanTool(t)

	// Create a plan with 1 step
	tool.Execute(context.Background(), map[string]any{
		"operation": "create",
		"goal":      "Tiny plan",
		"steps":     []any{"Only step"},
	})

	result := tool.Execute(context.Background(), map[string]any{
		"operation":  "replan",
		"plan_id":    "plan-1",
		"action":     "remove",
		"step_index": float64(0),
	})
	if !result.IsError {
		t.Fatal("expected error when removing last step")
	}
}

func TestReplanReorder(t *testing.T) {
	tool, mgr := newTestPlanTool(t)
	createTestPlan(t, tool)

	result := tool.Execute(context.Background(), map[string]any{
		"operation":  "replan",
		"plan_id":    "plan-1",
		"action":     "reorder",
		"step_index": float64(2),
		"new_index":  float64(0),
	})
	if result.IsError {
		t.Fatalf("replan reorder failed: %s", result.ForLLM)
	}

	plan := mgr.GetPlan("plan-1")
	if plan.Steps[0].Description != "Step 3" {
		t.Errorf("expected Step 3 moved to index 0, got: %s", plan.Steps[0].Description)
	}
	if plan.Steps[1].Description != "Step 1" {
		t.Errorf("expected Step 1 at index 1, got: %s", plan.Steps[1].Description)
	}
	if plan.Steps[2].Description != "Step 2" {
		t.Errorf("expected Step 2 at index 2, got: %s", plan.Steps[2].Description)
	}
}

func TestReplanInvalidPlan(t *testing.T) {
	tool, _ := newTestPlanTool(t)
	result := tool.Execute(context.Background(), map[string]any{
		"operation":   "replan",
		"plan_id":     "nonexistent",
		"action":      "insert",
		"step_index":  float64(0),
		"description": "whatever",
	})
	if !result.IsError {
		t.Fatal("expected error for nonexistent plan")
	}
}

// --- Hierarchical planning tests ---

func TestCreateSubplan(t *testing.T) {
	tool, mgr := newTestPlanTool(t)
	createTestPlan(t, tool)

	result := tool.Execute(context.Background(), map[string]any{
		"operation":         "create_subplan",
		"parent_plan_id":    "plan-1",
		"parent_step_index": float64(1),
		"goal":              "Detail for step 2",
		"steps":             []any{"Sub-step A", "Sub-step B"},
	})
	if result.IsError {
		t.Fatalf("create_subplan failed: %s", result.ForLLM)
	}

	// Verify parent step links to sub-plan
	parent := mgr.GetPlan("plan-1")
	if parent.Steps[1].SubPlanID == "" {
		t.Fatal("expected parent step to link to sub-plan")
	}

	// Verify sub-plan exists
	sub := mgr.GetPlan(parent.Steps[1].SubPlanID)
	if sub == nil {
		t.Fatal("sub-plan not found")
	}
	if sub.ParentPlanID != "plan-1" {
		t.Errorf("expected parent=plan-1, got: %s", sub.ParentPlanID)
	}
	if sub.ParentStepIndex != 1 {
		t.Errorf("expected parent step index=1, got: %d", sub.ParentStepIndex)
	}
	if len(sub.Steps) != 2 {
		t.Errorf("expected 2 sub-steps, got: %d", len(sub.Steps))
	}
}

func TestCreateSubplanInvalidParent(t *testing.T) {
	tool, _ := newTestPlanTool(t)
	result := tool.Execute(context.Background(), map[string]any{
		"operation":         "create_subplan",
		"parent_plan_id":    "nonexistent",
		"parent_step_index": float64(0),
		"goal":              "test",
		"steps":             []any{"a"},
	})
	if !result.IsError {
		t.Fatal("expected error for nonexistent parent")
	}
}

func TestGetStatusHierarchical(t *testing.T) {
	tool, _ := newTestPlanTool(t)
	createTestPlan(t, tool)

	// Create a sub-plan
	tool.Execute(context.Background(), map[string]any{
		"operation":         "create_subplan",
		"parent_plan_id":    "plan-1",
		"parent_step_index": float64(0),
		"goal":              "Detail for step 1",
		"steps":             []any{"Sub A", "Sub B"},
	})

	result := tool.Execute(context.Background(), map[string]any{
		"operation": "get_status",
		"plan_id":   "plan-1",
	})
	if result.IsError {
		t.Fatalf("get_status failed: %s", result.ForLLM)
	}
	if !contains(result.ForLLM, "Sub-plan for step 1") {
		t.Errorf("expected hierarchical sub-plan display, got: %s", result.ForLLM)
	}
	if !contains(result.ForLLM, "Sub A") {
		t.Errorf("expected sub-step content, got: %s", result.ForLLM)
	}
}

// --- Plan template tests ---

func TestSaveAndUseTemplate(t *testing.T) {
	tool, _ := newTestPlanTool(t)
	createTestPlan(t, tool)

	// Save as template
	result := tool.Execute(context.Background(), map[string]any{
		"operation": "save_template",
		"plan_id":   "plan-1",
		"name":      "deploy-workflow",
		"tags":      "deploy,ci",
	})
	if result.IsError {
		t.Fatalf("save_template failed: %s", result.ForLLM)
	}
	if !contains(result.ForLLM, "deploy-workflow") {
		t.Errorf("expected template name in output, got: %s", result.ForLLM)
	}

	// Use the template
	result = tool.Execute(context.Background(), map[string]any{
		"operation": "use_template",
		"name":      "deploy-workflow",
		"goal":      "Deploy v2",
	})
	if result.IsError {
		t.Fatalf("use_template failed: %s", result.ForLLM)
	}
	if !contains(result.ForLLM, "Deploy v2") {
		t.Errorf("expected overridden goal, got: %s", result.ForLLM)
	}
	if !contains(result.ForLLM, "Step 1") {
		t.Errorf("expected template steps in plan, got: %s", result.ForLLM)
	}
}

func TestFindTemplates(t *testing.T) {
	tool, _ := newTestPlanTool(t)
	createTestPlan(t, tool)

	// Save template
	tool.Execute(context.Background(), map[string]any{
		"operation": "save_template",
		"plan_id":   "plan-1",
		"name":      "web-deploy",
		"tags":      "deploy,web",
	})

	// Search
	result := tool.Execute(context.Background(), map[string]any{
		"operation": "find_templates",
		"query":     "deploy",
	})
	if result.IsError {
		t.Fatalf("find_templates failed: %s", result.ForLLM)
	}
	if !contains(result.ForLLM, "web-deploy") {
		t.Errorf("expected to find template, got: %s", result.ForLLM)
	}
}

func TestFindTemplatesNoMatch(t *testing.T) {
	tool, _ := newTestPlanTool(t)
	result := tool.Execute(context.Background(), map[string]any{
		"operation": "find_templates",
		"query":     "nonexistent",
	})
	if result.IsError {
		t.Fatalf("find_templates should not error on no match: %s", result.ForLLM)
	}
	if !contains(result.ForLLM, "No templates found") {
		t.Errorf("expected no templates message, got: %s", result.ForLLM)
	}
}

func TestUseTemplateNotFound(t *testing.T) {
	tool, _ := newTestPlanTool(t)
	result := tool.Execute(context.Background(), map[string]any{
		"operation": "use_template",
		"name":      "nonexistent",
	})
	if !result.IsError {
		t.Fatal("expected error for nonexistent template")
	}
}

func TestTemplateUsageCounter(t *testing.T) {
	tool, _ := newTestPlanTool(t)
	createTestPlan(t, tool)

	tool.Execute(context.Background(), map[string]any{
		"operation": "save_template",
		"plan_id":   "plan-1",
		"name":      "counter-test",
	})

	// Use it twice
	tool.Execute(context.Background(), map[string]any{
		"operation": "use_template",
		"name":      "counter-test",
	})
	tool.Execute(context.Background(), map[string]any{
		"operation": "use_template",
		"name":      "counter-test",
	})

	result := tool.Execute(context.Background(), map[string]any{
		"operation": "find_templates",
		"query":     "counter",
	})
	if !contains(result.ForLLM, "used 2 times") {
		t.Errorf("expected use count of 2, got: %s", result.ForLLM)
	}
}

// --- Cost/benefit evaluation tests ---

func TestEvaluate(t *testing.T) {
	tool, mgr := newTestPlanTool(t)
	createTestPlan(t, tool)

	result := tool.Execute(context.Background(), map[string]any{
		"operation":    "evaluate",
		"plan_id":      "plan-1",
		"effort":       float64(3),
		"risk":         float64(2),
		"confidence":   0.9,
		"rationale":    "Well understood domain",
		"alternatives": []any{"Alternative approach A", "Alternative approach B"},
	})
	if result.IsError {
		t.Fatalf("evaluate failed: %s", result.ForLLM)
	}

	plan := mgr.GetPlan("plan-1")
	if plan.Assessment == nil {
		t.Fatal("expected assessment to be set")
	}
	if plan.Assessment.Effort != 3 {
		t.Errorf("expected effort=3, got: %d", plan.Assessment.Effort)
	}
	if plan.Assessment.Risk != 2 {
		t.Errorf("expected risk=2, got: %d", plan.Assessment.Risk)
	}
	if plan.Assessment.Confidence != 0.9 {
		t.Errorf("expected confidence=0.9, got: %f", plan.Assessment.Confidence)
	}
	if len(plan.Assessment.Alternatives) != 2 {
		t.Errorf("expected 2 alternatives, got: %d", len(plan.Assessment.Alternatives))
	}
	if !contains(result.ForLLM, "RECOMMENDED") {
		t.Errorf("expected RECOMMENDED for good score, got: %s", result.ForLLM)
	}
}

func TestEvaluateClampValues(t *testing.T) {
	tool, mgr := newTestPlanTool(t)
	createTestPlan(t, tool)

	tool.Execute(context.Background(), map[string]any{
		"operation":  "evaluate",
		"plan_id":    "plan-1",
		"effort":     float64(15), // over max
		"risk":       float64(-1), // under min
		"confidence": 2.5,         // over max
	})

	plan := mgr.GetPlan("plan-1")
	if plan.Assessment.Effort != 10 {
		t.Errorf("expected effort clamped to 10, got: %d", plan.Assessment.Effort)
	}
	if plan.Assessment.Risk != 1 {
		t.Errorf("expected risk clamped to 1, got: %d", plan.Assessment.Risk)
	}
	if plan.Assessment.Confidence != 1.0 {
		t.Errorf("expected confidence clamped to 1.0, got: %f", plan.Assessment.Confidence)
	}
}

func TestEvaluateBadScore(t *testing.T) {
	tool, _ := newTestPlanTool(t)
	createTestPlan(t, tool)

	result := tool.Execute(context.Background(), map[string]any{
		"operation":  "evaluate",
		"plan_id":    "plan-1",
		"effort":     float64(10),
		"risk":       float64(9),
		"confidence": 0.1,
	})
	if result.IsError {
		t.Fatalf("evaluate failed: %s", result.ForLLM)
	}
	if !contains(result.ForLLM, "RECONSIDER") {
		t.Errorf("expected RECONSIDER for bad score, got: %s", result.ForLLM)
	}
}

func TestEvaluateInvalidPlan(t *testing.T) {
	tool, _ := newTestPlanTool(t)
	result := tool.Execute(context.Background(), map[string]any{
		"operation":  "evaluate",
		"plan_id":    "nonexistent",
		"effort":     float64(5),
		"risk":       float64(5),
		"confidence": 0.5,
	})
	if !result.IsError {
		t.Fatal("expected error for nonexistent plan")
	}
}

// --- Edge case tests ---

func TestUnknownOperation(t *testing.T) {
	tool, _ := newTestPlanTool(t)
	result := tool.Execute(context.Background(), map[string]any{
		"operation": "bogus",
	})
	if !result.IsError {
		t.Fatal("expected error for unknown operation")
	}
}

func TestPlanToolWithNilDB(t *testing.T) {
	mgr := NewPlanManager()
	tool := NewPlanTool(mgr, nil)

	// Template operations should fail gracefully
	result := tool.Execute(context.Background(), map[string]any{
		"operation": "find_templates",
		"query":     "test",
	})
	if !result.IsError {
		t.Fatal("expected error when memDB is nil")
	}
	if !contains(result.ForLLM, "memory database") {
		t.Errorf("expected helpful error message, got: %s", result.ForLLM)
	}
}

// --- Helper ---

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstr(s, substr))
}

func containsSubstr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
