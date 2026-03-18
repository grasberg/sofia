package channels

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// PairingRequest represents a pending DM pairing request from an unknown sender.
type PairingRequest struct {
	Channel  string    `json:"channel"`
	SenderID string    `json:"sender_id"`
	Code     string    `json:"code"`
	Expires  time.Time `json:"expires"`
}

// PairingManager manages DM pairing codes and approved senders.
// It generates short-lived codes for unknown senders and persists
// approved sender lists to disk.
type PairingManager struct {
	mu              sync.RWMutex
	pendingCodes    map[string]*PairingRequest // code -> request
	approvedSenders map[string]map[string]bool // channel -> set of senderIDs
	filePath        string
}

// NewPairingManager creates a PairingManager that persists approved senders
// to sofiaHome/pairing.json.
func NewPairingManager(sofiaHome string) *PairingManager {
	pm := &PairingManager{
		pendingCodes:    make(map[string]*PairingRequest),
		approvedSenders: make(map[string]map[string]bool),
		filePath:        filepath.Join(sofiaHome, "pairing.json"),
	}
	pm.load()
	return pm
}

// GenerateCode creates a random 6-character hex code for the given channel
// and sender. The code expires after 10 minutes.
func (pm *PairingManager) GenerateCode(channel, senderID string) string {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	pm.cleanExpired()

	buf := make([]byte, 3)
	_, _ = rand.Read(buf)
	code := hex.EncodeToString(buf)

	pm.pendingCodes[code] = &PairingRequest{
		Channel:  channel,
		SenderID: senderID,
		Code:     code,
		Expires:  time.Now().Add(10 * time.Minute),
	}

	return code
}

// Approve validates a pairing code, moves the sender to the approved set,
// and persists the change to disk. Returns the channel and senderID on
// success, or an error if the code is invalid or expired.
func (pm *PairingManager) Approve(code string) (string, string, error) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	// Look up the code before cleaning so we can distinguish
	// "expired" from "never existed".
	req, ok := pm.pendingCodes[code]
	if !ok {
		pm.cleanExpired()
		return "", "", fmt.Errorf("invalid pairing code %q", code)
	}

	if time.Now().After(req.Expires) {
		delete(pm.pendingCodes, code)
		pm.cleanExpired()
		return "", "", fmt.Errorf("pairing code %q has expired", code)
	}

	pm.cleanExpired()

	if pm.approvedSenders[req.Channel] == nil {
		pm.approvedSenders[req.Channel] = make(map[string]bool)
	}
	pm.approvedSenders[req.Channel][req.SenderID] = true

	delete(pm.pendingCodes, code)
	pm.save()

	return req.Channel, req.SenderID, nil
}

// IsApproved reports whether the sender has been approved for the given channel.
func (pm *PairingManager) IsApproved(channel, senderID string) bool {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	return pm.approvedSenders[channel][senderID]
}

// ListPending returns all non-expired pending pairing requests.
func (pm *PairingManager) ListPending() []*PairingRequest {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	now := time.Now()
	var result []*PairingRequest
	for _, req := range pm.pendingCodes {
		if now.Before(req.Expires) {
			result = append(result, req)
		}
	}
	return result
}

// save marshals the approved senders map to disk as JSON.
func (pm *PairingManager) save() {
	data, err := json.MarshalIndent(pm.approvedSenders, "", "  ")
	if err != nil {
		return
	}

	dir := filepath.Dir(pm.filePath)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return
	}

	_ = os.WriteFile(pm.filePath, data, 0o600)
}

// load reads the approved senders map from disk.
func (pm *PairingManager) load() {
	data, err := os.ReadFile(pm.filePath)
	if err != nil {
		return
	}

	var approved map[string]map[string]bool
	if err := json.Unmarshal(data, &approved); err != nil {
		return
	}

	pm.approvedSenders = approved
}

// cleanExpired removes expired pending codes.
func (pm *PairingManager) cleanExpired() {
	now := time.Now()
	for code, req := range pm.pendingCodes {
		if now.After(req.Expires) {
			delete(pm.pendingCodes, code)
		}
	}
}
