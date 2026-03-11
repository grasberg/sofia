package constants

import "testing"

func TestIsInternalChannel(t *testing.T) {
	tests := []struct {
		channel  string
		expected bool
	}{
		{"cli", true},
		{"system", true},
		{"subagent", true},
		{"telegram", false},
		{"whatsapp", false},
		{"email", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.channel, func(t *testing.T) {
			result := IsInternalChannel(tt.channel)
			if result != tt.expected {
				t.Errorf("IsInternalChannel(%q) = %v, expected %v", tt.channel, result, tt.expected)
			}
		})
	}
}
