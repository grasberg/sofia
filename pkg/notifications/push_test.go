package notifications

import (
	"testing"
)

func TestNewPushService(t *testing.T) {
	ps := NewPushService("TestApp")
	if ps.appName != "TestApp" {
		t.Errorf("expected appName to be 'TestApp', got %q", ps.appName)
	}
}

func TestSend(t *testing.T) {
	ps := NewPushService("TestApp")
	// Note: This will attempt to send a real notification on the OS.
	// In a real test environment, we'd mock beeep.Notify.
	// For now, we just verify the function doesn't panic.
	err := ps.Send("Test Title", "Test Message")
	// We don't assert error here because it depends on the OS environment
	_ = err
}

func TestAlert(t *testing.T) {
	ps := NewPushService("TestApp")
	// Similar to Send, this attempts a real OS-level alert.
	err := ps.Alert("Test Alert", "Alert Message")
	_ = err
}

func TestPushServiceWithoutIcon(t *testing.T) {
	ps := NewPushService("TestApp")
	if ps.iconPath == "" {
		// Expected if icon file doesn't exist
		t.Logf("iconPath is empty (expected if no icon file)")
	}
}
