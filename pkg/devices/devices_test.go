package devices

import (
	"testing"

	"github.com/grasberg/sofia/pkg/state"
)

func TestParseLastChannel_Valid(t *testing.T) {
	tests := []struct {
		input    string
		platform string
		userID   string
	}{
		{"telegram:12345", "telegram", "12345"},
		{"whatsapp:user@example.com", "whatsapp", "user@example.com"},
		{"slack:C1234567", "slack", "C1234567"},
		{"email:user@domain.org", "email", "user@domain.org"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			platform, userID := parseLastChannel(tt.input)
			if platform != tt.platform {
				t.Errorf("platform = %q, want %q", platform, tt.platform)
			}
			if userID != tt.userID {
				t.Errorf("userID = %q, want %q", userID, tt.userID)
			}
		})
	}
}

func TestParseLastChannel_Invalid(t *testing.T) {
	tests := []string{
		"",
		"no-colon",
		":",
		":useronly",
		"platformonly:",
	}

	for _, tt := range tests {
		t.Run(tt, func(t *testing.T) {
			platform, userID := parseLastChannel(tt)
			if platform != "" || userID != "" {
				t.Errorf("Expected empty results for %q, got platform=%q userID=%q", tt, platform, userID)
			}
		})
	}
}

func TestNewService_Enabled(t *testing.T) {
	cfg := Config{Enabled: true, MonitorUSB: false}
	workspace := t.TempDir()
	mgr := state.NewManager(workspace)
	svc := NewService(cfg, mgr)

	if svc == nil {
		t.Fatal("NewService returned nil")
	}
	if !svc.enabled {
		t.Error("Service should be enabled")
	}
	if len(svc.sources) != 0 {
		t.Errorf("Expected 0 sources (USB disabled), got %d", len(svc.sources))
	}
}

func TestNewService_Disabled(t *testing.T) {
	cfg := Config{Enabled: false, MonitorUSB: true}
	workspace := t.TempDir()
	mgr := state.NewManager(workspace)
	svc := NewService(cfg, mgr)

	if svc == nil {
		t.Fatal("NewService returned nil")
	}
	if svc.enabled {
		t.Error("Service should be disabled")
	}
}

func TestNewService_WithUSBMonitor(t *testing.T) {
	cfg := Config{Enabled: true, MonitorUSB: true}
	workspace := t.TempDir()
	mgr := state.NewManager(workspace)
	svc := NewService(cfg, mgr)

	if svc == nil {
		t.Fatal("NewService returned nil")
	}
	if len(svc.sources) != 1 {
		t.Errorf("Expected 1 source (USB enabled), got %d", len(svc.sources))
	}
}

func TestService_Stop_NoCrash(t *testing.T) {
	cfg := Config{Enabled: true, MonitorUSB: false}
	workspace := t.TempDir()
	mgr := state.NewManager(workspace)
	svc := NewService(cfg, mgr)

	// Stop without Start should not crash
	svc.Stop()
}

func TestService_SetBus(t *testing.T) {
	cfg := Config{Enabled: true, MonitorUSB: false}
	workspace := t.TempDir()
	mgr := state.NewManager(workspace)
	svc := NewService(cfg, mgr)

	// SetBus should not panic
	svc.SetBus(nil)
}

func TestParseLastChannel_WithSpecialChars(t *testing.T) {
	tests := []struct {
		input    string
		platform string
		userID   string
	}{
		{"telegram:123456789", "telegram", "123456789"},
		{"discord:user#1234", "discord", "user#1234"},
		{"matrix:@user:example.com", "matrix", "@user:example.com"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			platform, userID := parseLastChannel(tt.input)
			if platform != tt.platform {
				t.Errorf("platform = %q, want %q", platform, tt.platform)
			}
			if userID != tt.userID {
				t.Errorf("userID = %q, want %q", userID, tt.userID)
			}
		})
	}
}
