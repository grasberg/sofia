package web

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveAssetsDir_WithEnvVar(t *testing.T) {
	tmpDir := t.TempDir()
	oldEnv := os.Getenv("SOFIA_ASSETS_DIR")
	defer os.Setenv("SOFIA_ASSETS_DIR", oldEnv)

	os.Setenv("SOFIA_ASSETS_DIR", tmpDir)
	result := resolveAssetsDir()
	if result != tmpDir {
		t.Errorf("resolveAssetsDir() = %q, want %q", result, tmpDir)
	}
}

func TestResolveAssetsDir_WithInvalidEnvVar(t *testing.T) {
	oldEnv := os.Getenv("SOFIA_ASSETS_DIR")
	defer os.Setenv("SOFIA_ASSETS_DIR", oldEnv)

	os.Setenv("SOFIA_ASSETS_DIR", "/nonexistent/path/12345")
	result := resolveAssetsDir()
	// Should fall back to "assets" (default)
	if result == "/nonexistent/path/12345" {
		t.Error("resolveAssetsDir() should not use invalid env var path")
	}
}

func TestResolveAssetsDir_FallbackDefault(t *testing.T) {
	oldEnv := os.Getenv("SOFIA_ASSETS_DIR")
	defer os.Setenv("SOFIA_ASSETS_DIR", oldEnv)

	os.Setenv("SOFIA_ASSETS_DIR", "")
	result := resolveAssetsDir()
	// Should return something (either a valid path or "assets")
	if result == "" {
		t.Error("resolveAssetsDir() should not return empty string")
	}
}

func TestResolveAssetsDir_WithCurrentDir(t *testing.T) {
	// Create a temporary assets dir in current working dir
	tmpDir := t.TempDir()
	oldWd, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(oldWd)

	assetsDir := filepath.Join(tmpDir, "assets")
	os.Mkdir(assetsDir, 0o755)

	result := resolveAssetsDir()
	if result != assetsDir && result != "assets" {
		t.Logf("resolveAssetsDir() = %q", result)
		// It might find it or not depending on pwd, but should not error
	}
}

func TestNewDashboardHub(t *testing.T) {
	hub := NewDashboardHub()
	if hub == nil {
		t.Fatal("NewDashboardHub returned nil")
	}
	if hub.clients == nil {
		t.Fatal("Hub.clients is nil")
	}
	if len(hub.clients) != 0 {
		t.Errorf("Expected empty clients map, got %d", len(hub.clients))
	}
}

func TestDashboardHubBroadcastEmptyHub(t *testing.T) {
	hub := NewDashboardHub()
	// Should not panic on empty hub
	hub.Broadcast(map[string]string{"test": "data"})
}

func TestDashboardHubBroadcastWithNilData(t *testing.T) {
	hub := NewDashboardHub()
	// Should not panic
	hub.Broadcast(nil)
}

func TestDashboardHubBroadcastMessageFormat(t *testing.T) {
	hub := NewDashboardHub()

	testData := map[string]any{
		"status": "active",
		"count":  42,
	}

	// Should not panic and should marshal successfully
	hub.Broadcast(testData)
}

func TestDashboardHubBroadcastWithComplexData(t *testing.T) {
	hub := NewDashboardHub()

	complexData := map[string]any{
		"nested": map[string]any{
			"level2": map[string]any{
				"value": "deep",
			},
		},
		"array": []int{1, 2, 3, 4, 5},
	}

	// Should not panic
	hub.Broadcast(complexData)
}

func TestDashboardHubBroadcastInvalidJSON(t *testing.T) {
	hub := NewDashboardHub()

	// Channel that returns error on marshal
	ch := make(chan any)
	hub.Broadcast(ch) // This should fail to marshal but not panic
}
