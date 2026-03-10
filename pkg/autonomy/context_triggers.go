package autonomy

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/grasberg/sofia/pkg/memory"
)

// ContextTrigger represents an action to take when the user provides specific context.
type ContextTrigger struct {
	ID        int64     `json:"id"`
	AgentID   string    `json:"agent_id"`
	Name      string    `json:"name"`
	Condition string    `json:"condition"` // The condition required to fire
	Action    string    `json:"action"`    // The action line/prompt to inject or fire
	IsActive  bool      `json:"is_active"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// TriggerManager handles context triggers in the memory DB.
type TriggerManager struct {
	memDB *memory.MemoryDB
}

// NewTriggerManager creates a new context trigger manager.
func NewTriggerManager(memDB *memory.MemoryDB) *TriggerManager {
	return &TriggerManager{
		memDB: memDB,
	}
}

// AddTrigger creates a new context-aware trigger.
func (tm *TriggerManager) AddTrigger(agentID, name, condition, action string) (any, error) {
	props := map[string]any{
		"condition": condition,
		"action":    action,
		"is_active": true,
	}
	propsJSON, err := json.Marshal(props)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal trigger properties: %w", err)
	}

	id, err := tm.memDB.UpsertNode(agentID, "ContextTrigger", name, string(propsJSON))
	if err != nil {
		return nil, fmt.Errorf("failed to upsert trigger node: %w", err)
	}

	return tm.GetTriggerByID(id)
}

// ToggleTrigger enables or disables a trigger.
func (tm *TriggerManager) ToggleTrigger(triggerID int64, isActive bool) (any, error) {
	node, err := tm.memDB.GetNodeByID(triggerID)
	if err != nil {
		return nil, err
	}
	if node == nil || node.Label != "ContextTrigger" {
		return nil, fmt.Errorf("trigger %d not found", triggerID)
	}

	var props map[string]any
	if err := json.Unmarshal([]byte(node.Properties), &props); err != nil {
		props = make(map[string]any)
	}
	props["is_active"] = isActive

	propsJSON, _ := json.Marshal(props)
	_, err = tm.memDB.UpsertNode(node.AgentID, "ContextTrigger", node.Name, string(propsJSON))
	if err != nil {
		return nil, err
	}

	return tm.GetTriggerByID(triggerID)
}

// GetTriggerByID retrieves a trigger by ID.
func (tm *TriggerManager) GetTriggerByID(triggerID int64) (*ContextTrigger, error) {
	node, err := tm.memDB.GetNodeByID(triggerID)
	if err != nil {
		return nil, err
	}
	if node == nil || node.Label != "ContextTrigger" {
		return nil, fmt.Errorf("trigger %d not found", triggerID)
	}
	return parseTriggerNode(node), nil
}

// ListActiveTriggers returns all currently active triggers for an agent.
func (tm *TriggerManager) ListActiveTriggers(agentID string) ([]any, error) {
	nodes, err := tm.memDB.FindNodes(agentID, "ContextTrigger", "", 100)
	if err != nil {
		return nil, err
	}

	var active []any
	for _, node := range nodes {
		t := parseTriggerNode(&node)
		if t.IsActive {
			active = append(active, t)
		}
	}
	return active, nil
}

func parseTriggerNode(node *memory.SemanticNode) *ContextTrigger {
	t := &ContextTrigger{
		ID:        node.ID,
		AgentID:   node.AgentID,
		Name:      node.Name,
		CreatedAt: node.CreatedAt,
		UpdatedAt: node.UpdatedAt,
	}
	var props map[string]any
	if err := json.Unmarshal([]byte(node.Properties), &props); err == nil {
		if cond, ok := props["condition"].(string); ok {
			t.Condition = cond
		}
		if act, ok := props["action"].(string); ok {
			t.Action = act
		}
		if actv, ok := props["is_active"].(bool); ok {
			t.IsActive = actv
		}
	}
	return t
}
