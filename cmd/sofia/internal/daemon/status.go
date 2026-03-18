package daemon

import (
	"fmt"
	"os/exec"
	"runtime"
	"strings"

	"github.com/spf13/cobra"
)

func newStatusCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show Sofia gateway service status",
		RunE: func(_ *cobra.Command, _ []string) error {
			return runStatus()
		},
	}
}

func runStatus() error {
	switch runtime.GOOS {
	case "darwin":
		return statusDarwin()
	case "linux":
		return statusLinux()
	default:
		return fmt.Errorf("unsupported platform: %s", runtime.GOOS)
	}
}

func statusDarwin() error {
	cmd := exec.Command("launchctl", "list", "com.sofia.gateway")
	out, err := cmd.CombinedOutput()
	if err != nil {
		fmt.Println("Sofia gateway: stopped (not loaded)")
		return nil
	}

	output := strings.TrimSpace(string(out))
	if output == "" {
		fmt.Println("Sofia gateway: stopped (not loaded)")
		return nil
	}

	fmt.Println("Sofia gateway: running")
	fmt.Println()
	fmt.Println(output)

	return nil
}

func statusLinux() error {
	cmd := exec.Command("systemctl", "--user", "is-active", systemdUnit)
	out, err := cmd.CombinedOutput()
	state := strings.TrimSpace(string(out))

	if err != nil || state != "active" {
		fmt.Printf("Sofia gateway: stopped (%s)\n", state)
		return nil
	}

	fmt.Println("Sofia gateway: running")

	return nil
}
