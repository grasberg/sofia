package daemon

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/spf13/cobra"
)

func newUninstallCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "uninstall",
		Short: "Remove Sofia gateway background service",
		RunE: func(_ *cobra.Command, _ []string) error {
			return runUninstall()
		},
	}
}

func runUninstall() error {
	switch runtime.GOOS {
	case "darwin":
		return uninstallDarwin()
	case "linux":
		return uninstallLinux()
	default:
		return fmt.Errorf("unsupported platform: %s", runtime.GOOS)
	}
}

func uninstallDarwin() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get home directory: %w", err)
	}

	plistPath := filepath.Join(home, "Library", "LaunchAgents", plistName)

	cmd := exec.Command("launchctl", "unload", plistPath)
	if out, err := cmd.CombinedOutput(); err != nil {
		// Log the error but continue to remove the file anyway.
		fmt.Fprintf(os.Stderr, "warning: launchctl unload: %s\n", strings.TrimSpace(string(out)))
	}

	if err := os.Remove(plistPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove plist: %w", err)
	}

	fmt.Println("Sofia gateway service uninstalled.")

	return nil
}

func uninstallLinux() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get home directory: %w", err)
	}

	cmd := exec.Command("systemctl", "--user", "disable", "--now", systemdUnit)
	if out, err := cmd.CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "warning: systemctl disable: %s\n", strings.TrimSpace(string(out)))
	}

	unitPath := filepath.Join(home, ".config", "systemd", "user", systemdUnit)
	if err := os.Remove(unitPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove unit file: %w", err)
	}

	fmt.Println("Sofia gateway service uninstalled.")

	return nil
}
