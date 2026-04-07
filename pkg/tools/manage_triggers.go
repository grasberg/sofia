package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// TriggerManager interface breaks the import cycle between tools and autonomy.
type TriggerManager interface {
	AddTrigger(agentID, name, condition, action string) (any, error)
	ToggleTrigger(triggerID int64, isActive bool) (any, error)
	ListActiveTriggers(agentID string) ([]any, error)
}

// ManageTriggersOptions contains the TriggerManager.
type ManageTriggersOptions struct {
	TriggerManager TriggerManager
	AgentID        string
}

// ManageTriggersTool allows Sofia to autonomously set conditional triggers based on conversation context.
type ManageTriggersTool struct {
	mgr     TriggerManager
	agentID string
}

// NewManageTriggersTool creates a new coordinate tool.
func NewManageTriggersTool(opts ManageTriggersOptions) *ManageTriggersTool {
	return &ManageTriggersTool{
		mgr:     opts.TriggerManager,
		agentID: opts.AgentID,
	}
}

func (t *ManageTriggersTool) Name() string {
	return "manage_triggers"
}

func (t *ManageTriggersTool) Description() string {
	return `Manage context-aware triggers. These act as passive listeners and fire 'actions' when the 'condition' is met in conversation.
Actions: 
 - "add": create a new trigger (requires name, condition, action).
 - "toggle": enable/disable a trigger (requires trigger_id, is_active).
 - "list": view all active triggers.`
}

func (t *ManageTriggersTool) Parameters() map[string]any {
	var schema map[string]any
	json.Unmarshal([]byte(`{"type":"object","properties":{
		"action":{"type":"string","description":"add, toggle, or list"},
		"trigger_id":{"type":"integer","description":"The numeric ID of the trigger (for toggle)"},
		"name":{"type":"string","description":"Name of the trigger (for add)"},
		"condition":{"type":"string","description":"Context under which this trigger fires (for add)"},
		"trigger_action":{"type":"string","description":"What to do when fired (for add)"},
		"is_active":{"type":"boolean","description":"True/False state of the trigger (for toggle)"}
	},"required":["action"]}`), &schema)
	return schema
}

// Execute performs trigger manipulations
func (t *ManageTriggersTool) Execute(ctx context.Context, args map[string]any) *ToolResult {
	if t.mgr == nil {
		return ErrorResult("manage_triggers not configured: TriggerManager is nil")
	}

	bArgs, _ := json.Marshal(args)
	var parsedArgs struct {
		Action        string `json:"action"`
		TriggerID     int64  `json:"trigger_id"`
		Name          string `json:"name"`
		Condition     string `json:"condition"`
		TriggerAction string `json:"trigger_action"`
		IsActive      bool   `json:"is_active"`
	}

	if err := json.Unmarshal(bArgs, &parsedArgs); err != nil {
		return ErrorResult(fmt.Sprintf("invalid arguments: %v", err))
	}

	switch strings.ToLower(parsedArgs.Action) {
	case "add":
		if parsedArgs.Name == "" || parsedArgs.Condition == "" || parsedArgs.TriggerAction == "" {
			return ErrorResult("name, condition, and trigger_action required for 'add' action")
		}
		trigAny, err := t.mgr.AddTrigger(t.agentID, parsedArgs.Name, parsedArgs.Condition, parsedArgs.TriggerAction)
		if err != nil {
			return ErrorResult(fmt.Sprintf("failed to add trigger: %v", err))
		}
		b, _ := json.Marshal(trigAny)
		var trig map[string]any
		json.Unmarshal(b, &trig)
		if id, ok := trig["id"].(float64); ok {
			return NewToolResult(fmt.Sprintf("Trigger successfully added. ID: %.0f", id))
		}
		return NewToolResult("Trigger successfully added.")

	case "toggle":
		if parsedArgs.TriggerID == 0 {
			return ErrorResult("trigger_id required for 'toggle' action")
		}
		trigAny, err := t.mgr.ToggleTrigger(parsedArgs.TriggerID, parsedArgs.IsActive)
		if err != nil {
			return ErrorResult(fmt.Sprintf("failed to toggle trigger: %v", err))
		}
		b, _ := json.Marshal(trigAny)
		var trig map[string]any
		json.Unmarshal(b, &trig)
		id, _ := trig["id"].(float64)
		isActive, _ := trig["is_active"].(bool)
		return NewToolResult(
			fmt.Sprintf("Trigger %.0f active state set to %v", id, isActive),
		)

	case "list":
		triggersAny, err := t.mgr.ListActiveTriggers(t.agentID)
		if err != nil {
			return ErrorResult(fmt.Sprintf("failed to list triggers: %v", err))
		}
		if len(triggersAny) == 0 {
			return NewToolResult("No active context triggers found.")
		}
		var out strings.Builder
		for _, trigAny := range triggersAny {
			b, _ := json.Marshal(trigAny)
			var trig map[string]any
			json.Unmarshal(b, &trig)
			id, _ := trig["id"].(float64)
			name, _ := trig["name"].(string)
			condition, _ := trig["condition"].(string)
			action, _ := trig["action"].(string)
			fmt.Fprintf(&out, "- ID: %.0f | Name: %s\n  Condition: %s\n  Action: %s\n",
				id,
				name,
				condition,
				action)
		}
		return NewToolResult(out.String())

	default:
		return ErrorResult(fmt.Sprintf("unknown action: %s", parsedArgs.Action))
	}
}
