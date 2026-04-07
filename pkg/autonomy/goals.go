package autonomy

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/grasberg/sofia/pkg/memory"
)

// Goal Statuses
const (
	GoalStatusActive    = "active"
	GoalStatusCompleted = "completed"
	GoalStatusFailed    = "failed"
	GoalStatusPaused    = "paused"
)

// Goal represents a long-term user or agent objective.
type Goal struct {
	ID          int64     `json:"id"`
	AgentID     string    `json:"agent_id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Status      string    `json:"status"`
	Priority    string    `json:"priority"` // low, medium, high
	Result      string    `json:"result,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// GoalManager handles querying and storing goals in the memory DB.
type GoalManager struct {
	memDB *memory.MemoryDB
}

// NewGoalManager creates a new GoalManager instance.
func NewGoalManager(memDB *memory.MemoryDB) *GoalManager {
	return &GoalManager{
		memDB: memDB,
	}
}

// AddGoal creates a new active goal for the agent.
func (gm *GoalManager) AddGoal(agentID, name, description, priority string) (any, error) {
	if priority == "" {
		priority = "medium"
	}

	props := map[string]string{
		"description": description,
		"status":      GoalStatusActive,
		"priority":    priority,
	}
	propsJSON, err := json.Marshal(props)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal goal properties: %w", err)
	}

	id, err := gm.memDB.UpsertNode(agentID, "Goal", name, string(propsJSON))
	if err != nil {
		return nil, fmt.Errorf("failed to upsert goal node: %w", err)
	}

	return gm.GetGoalByID(id)
}

// UpdateGoalStatus updates an existing goal's status.
func (gm *GoalManager) UpdateGoalStatus(goalID int64, newStatus string) (any, error) {
	node, err := gm.memDB.GetNodeByID(goalID)
	if err != nil {
		return nil, err
	}
	if node == nil || node.Label != "Goal" {
		return nil, fmt.Errorf("goal %d not found", goalID)
	}

	var props map[string]string
	if err := json.Unmarshal([]byte(node.Properties), &props); err != nil {
		props = make(map[string]string)
	}
	props["status"] = newStatus

	propsJSON, _ := json.Marshal(props)
	_, err = gm.memDB.UpsertNode(node.AgentID, "Goal", node.Name, string(propsJSON))
	if err != nil {
		return nil, err
	}

	return gm.GetGoalByID(goalID)
}

// GetGoalByID retrieves a specific goal by its semantic node ID.
func (gm *GoalManager) GetGoalByID(goalID int64) (*Goal, error) {
	node, err := gm.memDB.GetNodeByID(goalID)
	if err != nil {
		return nil, err
	}
	if node == nil || node.Label != "Goal" {
		return nil, fmt.Errorf("goal %d not found", goalID)
	}
	return parseGoalNode(node), nil
}

// ListActiveGoals returns all currently active goals for an agent.
func (gm *GoalManager) ListActiveGoals(agentID string) ([]any, error) {
	// We use QueryGraph or FindNodes. FindNodes is exact match.
	// But since properties holds status, we fetch all Goal nodes and filter in memory.
	// Optimally, we could store status in the semantic index but for now this works.
	nodes, err := gm.memDB.FindNodes(agentID, "Goal", "", 100)
	if err != nil {
		return nil, err
	}

	var activeGoals []any
	for _, node := range nodes {
		g := parseGoalNode(&node)
		if g.Status == GoalStatusActive {
			activeGoals = append(activeGoals, g)
		}
	}
	return activeGoals, nil
}

// ListAllGoals returns all goals for an agent regardless of status.
func (gm *GoalManager) ListAllGoals(agentID string) ([]*Goal, error) {
	nodes, err := gm.memDB.FindNodes(agentID, "Goal", "", 100)
	if err != nil {
		return nil, err
	}

	goals := make([]*Goal, 0, len(nodes))
	for _, node := range nodes {
		goals = append(goals, parseGoalNode(&node))
	}
	return goals, nil
}

// UpdateGoalResult updates an existing goal's result text.
func (gm *GoalManager) UpdateGoalResult(goalID int64, result string) error {
	node, err := gm.memDB.GetNodeByID(goalID)
	if err != nil {
		return err
	}
	if node == nil || node.Label != "Goal" {
		return fmt.Errorf("goal %d not found", goalID)
	}

	var props map[string]string
	if err := json.Unmarshal([]byte(node.Properties), &props); err != nil {
		props = make(map[string]string)
	}
	props["result"] = result

	propsJSON, _ := json.Marshal(props)
	_, err = gm.memDB.UpsertNode(node.AgentID, "Goal", node.Name, string(propsJSON))
	return err
}

// DeleteGoal removes a goal and its log entries from the database.
func (gm *GoalManager) DeleteGoal(goalID int64) error {
	// Delete log entries first (child records).
	if err := gm.memDB.DeleteGoalLog(goalID); err != nil {
		return fmt.Errorf("failed to delete goal log: %w", err)
	}
	return gm.memDB.DeleteNode(goalID)
}

// DeleteAllGoals removes all goals (and their logs) for an agent.
func (gm *GoalManager) DeleteAllGoals(agentID string) (int, error) {
	goals, err := gm.ListAllGoals(agentID)
	if err != nil {
		return 0, err
	}
	deleted := 0
	for _, g := range goals {
		if err := gm.DeleteGoal(g.ID); err == nil {
			deleted++
		}
	}
	return deleted, nil
}

func parseGoalNode(node *memory.SemanticNode) *Goal {
	g := &Goal{
		ID:        node.ID,
		AgentID:   node.AgentID,
		Name:      node.Name,
		CreatedAt: node.CreatedAt,
		UpdatedAt: node.UpdatedAt,
	}
	var props map[string]string
	if err := json.Unmarshal([]byte(node.Properties), &props); err == nil {
		g.Description = props["description"]
		g.Status = props["status"]
		g.Priority = props["priority"]
		g.Result = props["result"]
	}
	return g
}
