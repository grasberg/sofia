package tools

import (
	"encoding/json"
	"fmt"
	"os"
)

// autoSave saves to persistPath if set. It collects state under RLock
// and writes to disk outside the lock to avoid deadlock.
func (pm *PlanManager) autoSave() {
	if pm.persistPath == "" {
		return
	}
	// Collect the data under RLock.
	pm.mu.RLock()
	state := planPersistState{Plans: pm.plans, NextID: pm.nextID}
	pm.mu.RUnlock()

	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return
	}
	_ = os.WriteFile(pm.persistPath, data, 0o600)
}

// planPersistState is the JSON-serializable snapshot of the PlanManager.
type planPersistState struct {
	Plans  map[string]*Plan `json:"plans"`
	NextID int              `json:"next_id"`
}

// Save persists all plans to the given file path.
func (pm *PlanManager) Save(path string) error {
	pm.mu.RLock()
	state := planPersistState{Plans: pm.plans, NextID: pm.nextID}
	pm.mu.RUnlock()

	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("plan: marshal: %w", err)
	}
	return os.WriteFile(path, data, 0o600)
}

// Load restores plans from the given file path. Missing file is not an error.
func (pm *PlanManager) Load(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("plan: read: %w", err)
	}
	var state planPersistState
	if err := json.Unmarshal(data, &state); err != nil {
		return fmt.Errorf("plan: unmarshal: %w", err)
	}

	pm.mu.Lock()
	defer pm.mu.Unlock()
	if state.Plans != nil {
		pm.plans = state.Plans
		// Rebuild the goal index from loaded plans.
		pm.goalIndex = make(map[int64]string, len(pm.plans))
		for _, p := range pm.plans {
			if p.GoalID != 0 {
				pm.goalIndex[p.GoalID] = p.ID
			}
		}
	}
	if state.NextID > pm.nextID {
		pm.nextID = state.NextID
	}
	return nil
}
