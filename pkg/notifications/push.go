package notifications

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/gen2brain/beeep"

	"github.com/grasberg/sofia/pkg/logger"
)

// PushService provides a way to send OS-level desktop push notifications.
type PushService struct {
	appName  string
	iconPath string
	openURL  string // URL to open when notification is clicked (macOS)
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

// SetOpenURL configures the URL to open when a notification is clicked.
func (s *PushService) SetOpenURL(url string) {
	s.openURL = url
}

// Send sends a desktop notification. On macOS, clicking the notification
// opens the configured URL (typically the Sofia web UI).
func (s *PushService) Send(title, message string) error {
	fullTitle := title
	if s.appName != "" && title != s.appName {
		fullTitle = fmt.Sprintf("%s: %s", s.appName, title)
	}

	var err error
	if runtime.GOOS == "darwin" && s.openURL != "" {
		err = s.sendDarwin(fullTitle, message)
	} else {
		err = beeep.Notify(fullTitle, message, s.iconPath)
	}

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

// sendDarwin sends a macOS notification that opens a URL when clicked.
// Tries terminal-notifier first (supports -open), falls back to osascript.
func (s *PushService) sendDarwin(title, message string) error {
	// Try terminal-notifier first — it supports click-to-open.
	if tn, lookErr := exec.LookPath("terminal-notifier"); lookErr == nil {
		args := []string{
			"-title", title,
			"-message", message,
			"-group", "sofia",
		}
		if s.openURL != "" {
			args = append(args, "-open", s.openURL)
		}
		if s.iconPath != "" {
			args = append(args, "-appIcon", s.iconPath)
		}
		cmd := exec.Command(tn, args...)
		if err := cmd.Run(); err == nil {
			return nil
		}
		// Fall through to osascript on failure.
	}

	// Fallback: osascript. display notification cannot handle clicks, so we
	// show the notification and hint the URL in the message.
	script := fmt.Sprintf(
		`display notification %q with title %q`,
		message+"\nClick to open: "+s.openURL, title,
	)
	return exec.Command("osascript", "-e", script).Run()
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
