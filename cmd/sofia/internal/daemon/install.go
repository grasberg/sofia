package daemon

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"text/template"

	"github.com/spf13/cobra"
)

const plistName = "com.sofia.gateway.plist"
const systemdUnit = "sofia-gateway.service"

var plistTemplate = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN"
  "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key>
  <string>com.sofia.gateway</string>
  <key>ProgramArguments</key>
  <array>
    <string>{{.BinaryPath}}</string>
    <string>gateway</string>
  </array>
  <key>RunAtLoad</key>
  <true/>
  <key>KeepAlive</key>
  <true/>
  <key>StandardOutPath</key>
  <string>{{.LogDir}}/sofia.log</string>
  <key>StandardErrorPath</key>
  <string>{{.LogDir}}/sofia.err.log</string>
</dict>
</plist>
`

var systemdTemplate = `[Unit]
Description=Sofia AI Gateway

[Service]
ExecStart={{.BinaryPath}} gateway
Restart=always
RestartSec=5

[Install]
WantedBy=default.target
`

type templateData struct {
	BinaryPath string
	LogDir     string
}

func generateLaunchdPlist(binaryPath, logDir string) string {
	var buf strings.Builder
	tmpl := template.Must(template.New("plist").Parse(plistTemplate))
	if err := tmpl.Execute(&buf, templateData{BinaryPath: binaryPath, LogDir: logDir}); err != nil {
		return ""
	}
	return buf.String()
}

func generateSystemdUnit(binaryPath string) string {
	var buf strings.Builder
	tmpl := template.Must(template.New("unit").Parse(systemdTemplate))
	if err := tmpl.Execute(&buf, templateData{BinaryPath: binaryPath}); err != nil {
		return ""
	}
	return buf.String()
}

func newInstallCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "install",
		Short: "Install Sofia gateway as a background service",
		RunE: func(_ *cobra.Command, _ []string) error {
			return runInstall()
		},
	}
}

func runInstall() error {
	binaryPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to resolve binary path: %w", err)
	}

	switch runtime.GOOS {
	case "darwin":
		return installDarwin(binaryPath)
	case "linux":
		return installLinux(binaryPath)
	default:
		return fmt.Errorf("unsupported platform: %s", runtime.GOOS)
	}
}

func installDarwin(binaryPath string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get home directory: %w", err)
	}

	logDir := filepath.Join(home, ".sofia", "logs")
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		return fmt.Errorf("failed to create log directory: %w", err)
	}

	plistDir := filepath.Join(home, "Library", "LaunchAgents")
	if err := os.MkdirAll(plistDir, 0o755); err != nil {
		return fmt.Errorf("failed to create LaunchAgents directory: %w", err)
	}

	plistPath := filepath.Join(plistDir, plistName)
	content := generateLaunchdPlist(binaryPath, logDir)

	if err := os.WriteFile(plistPath, []byte(content), 0o644); err != nil {
		return fmt.Errorf("failed to write plist: %w", err)
	}

	cmd := exec.Command("launchctl", "load", plistPath)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("launchctl load failed: %s: %w", strings.TrimSpace(string(out)), err)
	}

	fmt.Println("Sofia gateway installed as a launchd service.")
	fmt.Printf("  Plist: %s\n", plistPath)
	fmt.Printf("  Logs:  %s\n", logDir)
	fmt.Println()
	fmt.Println("The service will start automatically on login.")
	fmt.Println("Use 'sofia daemon status' to check its state.")

	return nil
}

func installLinux(binaryPath string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get home directory: %w", err)
	}

	unitDir := filepath.Join(home, ".config", "systemd", "user")
	if err := os.MkdirAll(unitDir, 0o755); err != nil {
		return fmt.Errorf("failed to create systemd user directory: %w", err)
	}

	unitPath := filepath.Join(unitDir, systemdUnit)
	content := generateSystemdUnit(binaryPath)

	if err := os.WriteFile(unitPath, []byte(content), 0o644); err != nil {
		return fmt.Errorf("failed to write unit file: %w", err)
	}

	cmd := exec.Command("systemctl", "--user", "enable", "--now", systemdUnit)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("systemctl enable failed: %s: %w", strings.TrimSpace(string(out)), err)
	}

	fmt.Println("Sofia gateway installed as a systemd user service.")
	fmt.Printf("  Unit: %s\n", unitPath)
	fmt.Println()
	fmt.Println("The service will start automatically on login.")
	fmt.Println("Use 'sofia daemon status' to check its state.")

	return nil
}
