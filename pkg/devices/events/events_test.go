package events

import (
	"testing"
)

func TestActionConstants(t *testing.T) {
	tests := []struct {
		action   Action
		expected string
	}{
		{ActionAdd, "add"},
		{ActionRemove, "remove"},
		{ActionChange, "change"},
	}

	for _, tt := range tests {
		if string(tt.action) != tt.expected {
			t.Errorf("expected %q, got %q", tt.expected, string(tt.action))
		}
	}
}

func TestKindConstants(t *testing.T) {
	tests := []struct {
		kind     Kind
		expected string
	}{
		{KindUSB, "usb"},
		{KindBluetooth, "bluetooth"},
		{KindPCI, "pci"},
		{KindGeneric, "generic"},
	}

	for _, tt := range tests {
		if string(tt.kind) != tt.expected {
			t.Errorf("expected %q, got %q", tt.expected, string(tt.kind))
		}
	}
}

func TestDeviceEventFormatMessage(t *testing.T) {
	event := &DeviceEvent{
		Action:       ActionAdd,
		Kind:         KindUSB,
		DeviceID:     "1-2",
		Vendor:       "Apple",
		Product:      "iPhone",
		Serial:       "ABC123",
		Capabilities: "USB 3.0",
	}

	msg := event.FormatMessage()

	// Check that the message contains expected parts
	if msg == "" {
		t.Fatal("expected non-empty message")
	}

	if !contains(msg, "Device Connected") {
		t.Errorf("expected message to contain 'Device Connected', got: %s", msg)
	}

	if !contains(msg, "usb") {
		t.Errorf("expected message to contain 'usb'")
	}

	if !contains(msg, "Apple") {
		t.Errorf("expected message to contain 'Apple'")
	}

	if !contains(msg, "iPhone") {
		t.Errorf("expected message to contain 'iPhone'")
	}

	if !contains(msg, "ABC123") {
		t.Errorf("expected message to contain serial number")
	}

	if !contains(msg, "USB 3.0") {
		t.Errorf("expected message to contain capabilities")
	}
}

func TestDeviceEventFormatMessageDisconnect(t *testing.T) {
	event := &DeviceEvent{
		Action:  ActionRemove,
		Kind:    KindBluetooth,
		Vendor:  "Sony",
		Product: "Headphones",
	}

	msg := event.FormatMessage()

	if msg == "" {
		t.Fatal("expected non-empty message")
	}

	if !contains(msg, "Device Disconnected") {
		t.Errorf("expected message to contain 'Device Disconnected'")
	}

	if !contains(msg, "bluetooth") {
		t.Errorf("expected message to contain 'bluetooth'")
	}
}

func TestDeviceEventFormatMessageMinimal(t *testing.T) {
	event := &DeviceEvent{
		Action:  ActionAdd,
		Kind:    KindGeneric,
		Vendor:  "Unknown",
		Product: "Device",
	}

	msg := event.FormatMessage()

	if msg == "" {
		t.Fatal("expected non-empty message")
	}

	// Should not panic and should contain basic info
	if !contains(msg, "Device Connected") {
		t.Errorf("expected message to contain 'Device Connected'")
	}
}

// Helper function to check if string contains substring
func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
