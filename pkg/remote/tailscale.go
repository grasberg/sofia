package remote

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// TailscaleStatus represents the JSON output of "tailscale status --json".
type TailscaleStatus struct {
	BackendState string `json:"BackendState"`
	Self         struct {
		DNSName      string   `json:"DNSName"`
		TailscaleIPs []string `json:"TailscaleIPs"`
	} `json:"Self"`
}

// TailscaleManager wraps the Tailscale CLI to manage remote access.
type TailscaleManager struct{}

// NewTailscaleManager returns a new TailscaleManager.
func NewTailscaleManager() *TailscaleManager {
	return &TailscaleManager{}
}

// IsAvailable checks if the tailscale CLI is in PATH.
func (tm *TailscaleManager) IsAvailable() bool {
	_, err := exec.LookPath("tailscale")
	return err == nil
}

// Status returns the current Tailscale status.
func (tm *TailscaleManager) Status() (*TailscaleStatus, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	out, err := exec.CommandContext(ctx, "tailscale", "status", "--json").Output()
	if err != nil {
		return nil, fmt.Errorf("tailscale status: %w", err)
	}

	var status TailscaleStatus
	if err := json.Unmarshal(out, &status); err != nil {
		return nil, fmt.Errorf("parse tailscale status: %w", err)
	}

	return &status, nil
}

// EnableServe exposes a local port via Tailscale Serve (tailnet-only HTTPS).
func (tm *TailscaleManager) EnableServe(port int) error {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	target := fmt.Sprintf("https+insecure://localhost:%d", port)
	cmd := exec.CommandContext(ctx, "tailscale", "serve", "--bg", target)

	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("tailscale serve: %s: %w", strings.TrimSpace(string(out)), err)
	}

	return nil
}

// EnableFunnel exposes a local port via Tailscale Funnel (public HTTPS).
func (tm *TailscaleManager) EnableFunnel(port int) error {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	target := fmt.Sprintf("https+insecure://localhost:%d", port)
	cmd := exec.CommandContext(ctx, "tailscale", "funnel", "--bg", target)

	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("tailscale funnel: %s: %w", strings.TrimSpace(string(out)), err)
	}

	return nil
}

// Disable turns off Tailscale Serve/Funnel.
func (tm *TailscaleManager) Disable() error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	out, err := exec.CommandContext(ctx, "tailscale", "serve", "off").CombinedOutput()
	if err != nil {
		return fmt.Errorf("tailscale serve off: %s: %w", strings.TrimSpace(string(out)), err)
	}

	return nil
}
