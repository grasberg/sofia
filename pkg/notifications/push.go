package notifications

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/gen2brain/beeep"
	"github.com/grasberg/sofia/pkg/logger"
)

// PushService provides a way to send OS-level desktop push notifications.
type PushService struct {
	appName  string
	iconPath string
}

// NewPushService creates a new PushService.
func NewPushService(appName string) *PushService {
	// Try to find an icon path, or leave empty for default OS icon
	icon := "assets/icon.png"
	if _, err := os.Stat(icon); os.IsNotExist(err) {
		icon = ""
	} else {
		if abs, err := filepath.Abs(icon); err == nil {
			icon = abs
		}
	}

	return &PushService{
		appName:  appName,
		iconPath: icon,
	}
}

// Send sends a desktop notification.
func (s *PushService) Send(title, message string) error {
	fullTitle := title
	if s.appName != "" && title != s.appName {
		fullTitle = fmt.Sprintf("%s: %s", s.appName, title)
	}

	err := beeep.Notify(fullTitle, message, s.iconPath)
	if err != nil {
		logger.WarnCF("push_notifications", "Failed to send desktop notification", map[string]any{
			"error": err.Error(),
			"title": title,
		})
		return err
	}

	logger.DebugCF("push_notifications", "Sent desktop notification", map[string]any{
		"title": title,
	})
	return nil
}

// Alert sends an alert (often stays on screen until dismissed, depending on OS).
func (s *PushService) Alert(title, message string) error {
	fullTitle := title
	if s.appName != "" && title != s.appName {
		fullTitle = fmt.Sprintf("%s: %s", s.appName, title)
	}

	err := beeep.Alert(fullTitle, message, s.iconPath)
	if err != nil {
		logger.WarnCF("push_notifications", "Failed to send desktop alert", map[string]any{
			"error": err.Error(),
			"title": title,
		})
		return err
	}

	logger.DebugCF("push_notifications", "Sent desktop alert", map[string]any{
		"title": title,
	})
	return nil
}
