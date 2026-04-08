package session

import (
	"fmt"
	"log"
	"time"

	"github.com/grasberg/sofia/pkg/providers"
)

// ForkSession creates a new session by cloning the history of an existing session.
// The new session gets a unique key derived from the source session key.
// This enables branching conversations — exploring different approaches from the same point.
func (sm *SessionManager) ForkSession(sourceKey string) (string, error) {
	// Load source messages
	msgs, err := sm.db.GetMessages(sourceKey)
	if err != nil {
		return "", fmt.Errorf("failed to load source session %q: %w", sourceKey, err)
	}
	if len(msgs) == 0 {
		return "", fmt.Errorf("source session %q has no messages", sourceKey)
	}

	return sm.createFork(sourceKey, msgs)
}

// ForkSessionAt creates a new session from the first N messages of an existing session.
// This allows branching from a specific point in the conversation.
func (sm *SessionManager) ForkSessionAt(sourceKey string, messageCount int) (string, error) {
	if messageCount <= 0 {
		return "", fmt.Errorf("messageCount must be positive")
	}

	msgs, err := sm.db.GetMessages(sourceKey)
	if err != nil {
		return "", fmt.Errorf("failed to load source session %q: %w", sourceKey, err)
	}

	if messageCount > len(msgs) {
		messageCount = len(msgs)
	}

	return sm.createFork(sourceKey, msgs[:messageCount])
}

func (sm *SessionManager) createFork(sourceKey string, msgs []providers.Message) (string, error) {
	forkKey := fmt.Sprintf("%s-fork-%d", sourceKey, time.Now().UnixNano())

	// Create the new session
	if _, err := sm.db.GetOrCreateSession(forkKey, sm.agentID); err != nil {
		return "", fmt.Errorf("failed to create fork session: %w", err)
	}

	// Copy all messages
	for _, msg := range msgs {
		if err := sm.db.AppendMessage(forkKey, msg); err != nil {
			log.Printf("session: fork copy message: %v", err)
		}
	}

	return forkKey, nil
}
