package session

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync"
	"time"
)

// BranchInfo stores metadata about a conversation branch.
type BranchInfo struct {
	BranchKey    string    `json:"branch_key"`
	ParentKey    string    `json:"parent_key"`
	BranchedAt   time.Time `json:"branched_at"`
	Label        string    `json:"label,omitempty"`
	MessageCount int       `json:"message_count"`
}

// BranchManager tracks session branches.
type BranchManager struct {
	mu       sync.RWMutex
	branches map[string][]BranchInfo // parentKey -> list of branches
}

// NewBranchManager creates a new BranchManager.
func NewBranchManager() *BranchManager {
	return &BranchManager{
		branches: make(map[string][]BranchInfo),
	}
}

// Branch creates a new conversation branch from the given parent session.
// It copies all messages from the parent into a new session and records the branch metadata.
func (bm *BranchManager) Branch(sm *SessionManager, parentKey, label string) (BranchInfo, error) {
	history := sm.GetHistory(parentKey)

	id, err := shortID()
	if err != nil {
		return BranchInfo{}, fmt.Errorf("generate branch id: %w", err)
	}
	branchKey := parentKey + ":branch:" + id

	// Ensure the new session exists and copy messages over.
	sm.SetHistory(branchKey, history)

	// Also copy the summary so the branch keeps the same compression state.
	if summary := sm.GetSummary(parentKey); summary != "" {
		sm.SetSummary(branchKey, summary)
	}

	info := BranchInfo{
		BranchKey:    branchKey,
		ParentKey:    parentKey,
		BranchedAt:   time.Now(),
		Label:        label,
		MessageCount: len(history),
	}

	bm.mu.Lock()
	bm.branches[parentKey] = append(bm.branches[parentKey], info)
	bm.mu.Unlock()

	return info, nil
}

// ListBranches returns all branches created from the given parent session key.
func (bm *BranchManager) ListBranches(parentKey string) []BranchInfo {
	bm.mu.RLock()
	defer bm.mu.RUnlock()

	out := make([]BranchInfo, len(bm.branches[parentKey]))
	copy(out, bm.branches[parentKey])
	return out
}

// GetParent returns the parent session key for a branch key, if it exists.
func (bm *BranchManager) GetParent(branchKey string) (string, bool) {
	bm.mu.RLock()
	defer bm.mu.RUnlock()

	for parent, infos := range bm.branches {
		for _, info := range infos {
			if info.BranchKey == branchKey {
				return parent, true
			}
		}
	}
	return "", false
}

// shortID returns a random 4-byte hex string (8 characters).
func shortID() (string, error) {
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
