package agent

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/grasberg/sofia/pkg/config"
	"github.com/grasberg/sofia/pkg/logger"
	"github.com/grasberg/sofia/pkg/providers"
)

// DoctorCheck represents a single diagnostic check.
type DoctorCheck struct {
	Name    string
	Status  string // "pass", "fail", "warn", "skip"
	Message string
	Fix     string // suggested fix
}

// DoctorReport contains the full diagnostic report.
type DoctorReport struct {
	Checks    []DoctorCheck
	AutoFixed []string
	Summary   string
	TotalPass int
	TotalFail int
	TotalWarn int
}

// RunDoctor performs comprehensive diagnostics and auto-healing.
func RunDoctor(cfg *config.Config) *DoctorReport {
	report := &DoctorReport{}

	// 1. Config validation
	report.addCheck(checkConfig(cfg))

	// 2. Provider connectivity
	report.addChecks(checkProviders(cfg))

	// 3. Required tools
	report.addChecks(checkRequiredTools())

	// 4. Workspace health
	report.addCheck(checkWorkspace(cfg))

	// 5. Database health
	report.addCheck(checkDatabase(cfg))

	// 6. MCP extensions
	report.addChecks(checkMCPExtensions(cfg))

	// 7. Recent logs for errors
	report.addCheck(checkRecentLogs())

	// 8. Disk space
	report.addCheck(checkDiskSpace())

	// Summarize
	for _, c := range report.Checks {
		switch c.Status {
		case "pass":
			report.TotalPass++
		case "fail":
			report.TotalFail++
		case "warn":
			report.TotalWarn++
		}
	}

	report.Summary = fmt.Sprintf("%d passed, %d failed, %d warnings",
		report.TotalPass, report.TotalFail, report.TotalWarn)

	return report
}

func (r *DoctorReport) addCheck(c DoctorCheck) {
	r.Checks = append(r.Checks, c)
}

func (r *DoctorReport) addChecks(checks []DoctorCheck) {
	r.Checks = append(r.Checks, checks...)
}

// String formats the report for display.
func (r *DoctorReport) String() string {
	var sb strings.Builder
	sb.WriteString("Sofia Doctor Report\n")
	sb.WriteString(strings.Repeat("=", 40) + "\n\n")

	for _, c := range r.Checks {
		icon := "?"
		switch c.Status {
		case "pass":
			icon = "OK"
		case "fail":
			icon = "FAIL"
		case "warn":
			icon = "WARN"
		case "skip":
			icon = "SKIP"
		}
		sb.WriteString(fmt.Sprintf("[%s] %s: %s\n", icon, c.Name, c.Message))
		if c.Fix != "" {
			sb.WriteString(fmt.Sprintf("       Fix: %s\n", c.Fix))
		}
	}

	if len(r.AutoFixed) > 0 {
		sb.WriteString("\nAuto-fixed:\n")
		for _, fix := range r.AutoFixed {
			sb.WriteString(fmt.Sprintf("  - %s\n", fix))
		}
	}

	sb.WriteString(fmt.Sprintf("\nSummary: %s\n", r.Summary))
	return sb.String()
}

func checkConfig(cfg *config.Config) DoctorCheck {
	if cfg == nil {
		return DoctorCheck{
			Name:    "Config",
			Status:  "fail",
			Message: "Config is nil",
			Fix:     "Run 'sofia onboard' to create a config",
		}
	}
	return DoctorCheck{Name: "Config", Status: "pass", Message: "Config loaded successfully"}
}

func checkProviders(cfg *config.Config) []DoctorCheck {
	var checks []DoctorCheck

	if cfg == nil {
		return checks
	}

	// Check if any provider has keys
	if cfg.Providers.IsEmpty() && len(cfg.ModelList) == 0 {
		checks = append(checks, DoctorCheck{
			Name:    "Providers",
			Status:  "fail",
			Message: "No providers configured",
			Fix:     "Add API keys to config.json or model_list entries",
		})
		return checks
	}

	// Try to reach the configured default model
	if cfg.Agents.Defaults.Model != "" {
		check := testModelConnectivity(cfg, cfg.Agents.Defaults.Model)
		checks = append(checks, check)
	}

	return checks
}

func testModelConnectivity(cfg *config.Config, model string) DoctorCheck {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	provider, resolvedModel, err := providers.CreateProvider(cfg)
	if err != nil {
		return DoctorCheck{
			Name:    fmt.Sprintf("Model: %s", model),
			Status:  "fail",
			Message: fmt.Sprintf("Failed to create provider: %v", err),
			Fix:     "Check API key and model configuration",
		}
	}

	// Try a minimal chat to verify connectivity
	resp, err := provider.Chat(ctx, []providers.Message{
		{Role: "user", Content: "Hi"},
	}, nil, resolvedModel, map[string]any{"max_tokens": 5})
	if err != nil {
		return DoctorCheck{
			Name:    fmt.Sprintf("Model: %s", model),
			Status:  "fail",
			Message: fmt.Sprintf("Connectivity test failed: %v", err),
			Fix:     "Check API key, model name, and network connectivity",
		}
	}

	if resp.Content == "" {
		return DoctorCheck{
			Name:    fmt.Sprintf("Model: %s", model),
			Status:  "warn",
			Message: "Model responded but with empty content",
		}
	}

	return DoctorCheck{
		Name:    fmt.Sprintf("Model: %s", model),
		Status:  "pass",
		Message: "Model is reachable and responding",
	}
}

func checkRequiredTools() []DoctorCheck {
	tools := map[string]string{
		"git":    "Version control",
		"rg":     "Code search (ripgrep)",
		"jq":     "JSON processing",
		"docker": "Container management",
	}

	var checks []DoctorCheck
	for tool, desc := range tools {
		_, err := exec.LookPath(tool)
		if err != nil {
			checks = append(checks, DoctorCheck{
				Name:    fmt.Sprintf("Tool: %s", tool),
				Status:  "warn",
				Message: fmt.Sprintf("%s not found in PATH", desc),
				Fix:     fmt.Sprintf("Install %s for full functionality", tool),
			})
		} else {
			checks = append(checks, DoctorCheck{
				Name:    fmt.Sprintf("Tool: %s", tool),
				Status:  "pass",
				Message: fmt.Sprintf("%s is available", desc),
			})
		}
	}

	return checks
}

func checkWorkspace(cfg *config.Config) DoctorCheck {
	if cfg == nil {
		return DoctorCheck{Name: "Workspace", Status: "skip", Message: "No config"}
	}

	workspace := cfg.Agents.Defaults.Workspace
	if workspace == "" {
		return DoctorCheck{
			Name:    "Workspace",
			Status:  "warn",
			Message: "No default workspace configured",
		}
	}

	if _, err := os.Stat(workspace); os.IsNotExist(err) {
		return DoctorCheck{
			Name:    "Workspace",
			Status:  "fail",
			Message: fmt.Sprintf("Workspace directory does not exist: %s", workspace),
			Fix:     "Create the workspace directory or update config",
		}
	}

	return DoctorCheck{
		Name:    "Workspace",
		Status:  "pass",
		Message: fmt.Sprintf("Workspace exists: %s", workspace),
	}
}

func checkDatabase(cfg *config.Config) DoctorCheck {
	if cfg == nil {
		return DoctorCheck{Name: "Database", Status: "skip", Message: "No config"}
	}

	dbPath := cfg.MemoryDB
	if dbPath == "" {
		home, _ := os.UserHomeDir()
		dbPath = filepath.Join(home, ".sofia", "memory.db")
	}

	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		return DoctorCheck{
			Name:    "Database",
			Status:  "warn",
			Message: "Database file not found (will be created on first use)",
		}
	}

	info, err := os.Stat(dbPath)
	if err != nil {
		return DoctorCheck{
			Name:    "Database",
			Status:  "fail",
			Message: fmt.Sprintf("Cannot access database: %v", err),
		}
	}

	sizeMB := info.Size() / (1024 * 1024)
	return DoctorCheck{
		Name:    "Database",
		Status:  "pass",
		Message: fmt.Sprintf("Database OK (%d MB)", sizeMB),
	}
}

func checkMCPExtensions(cfg *config.Config) []DoctorCheck {
	if cfg == nil || len(cfg.Tools.MCP) == 0 {
		return []DoctorCheck{{
			Name:    "MCP Extensions",
			Status:  "skip",
			Message: "No MCP extensions configured",
		}}
	}

	var checks []DoctorCheck
	for name, mcpCfg := range cfg.Tools.MCP {
		_, err := exec.LookPath(mcpCfg.Command)
		if err != nil {
			checks = append(checks, DoctorCheck{
				Name:    fmt.Sprintf("MCP: %s", name),
				Status:  "fail",
				Message: fmt.Sprintf("Command %q not found", mcpCfg.Command),
				Fix:     fmt.Sprintf("Install %s or update MCP config", mcpCfg.Command),
			})
		} else {
			checks = append(checks, DoctorCheck{
				Name:    fmt.Sprintf("MCP: %s", name),
				Status:  "pass",
				Message: fmt.Sprintf("Command %q available", mcpCfg.Command),
			})
		}
	}

	return checks
}

func checkRecentLogs() DoctorCheck {
	// Check if recent log has errors
	history := logger.GetHistory()
	if len(history) == 0 {
		return DoctorCheck{
			Name:    "Recent Logs",
			Status:  "pass",
			Message: "No recent log entries",
		}
	}

	errorCount := 0
	for _, entry := range history {
		if strings.Contains(entry, "[ERROR]") || strings.Contains(entry, "[FATAL]") {
			errorCount++
		}
	}

	if errorCount > 0 {
		return DoctorCheck{
			Name:    "Recent Logs",
			Status:  "warn",
			Message: fmt.Sprintf("Found %d error(s) in recent logs", errorCount),
			Fix:     "Check logs for details: ~/.sofia/sofia.log",
		}
	}

	return DoctorCheck{
		Name:    "Recent Logs",
		Status:  "pass",
		Message: fmt.Sprintf("No errors in %d recent log entries", len(history)),
	}
}

func checkDiskSpace() DoctorCheck {
	home, err := os.UserHomeDir()
	if err != nil {
		return DoctorCheck{Name: "Disk Space", Status: "skip", Message: "Cannot determine home dir"}
	}

	sofiaDir := filepath.Join(home, ".sofia")
	var totalSize int64
	_ = filepath.WalkDir(sofiaDir, func(_ string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if info, err := d.Info(); err == nil {
			totalSize += info.Size()
		}
		return nil
	})

	sizeMB := totalSize / (1024 * 1024)
	if sizeMB > 500 {
		return DoctorCheck{
			Name:    "Disk Space",
			Status:  "warn",
			Message: fmt.Sprintf("Sofia data directory is %d MB", sizeMB),
			Fix:     "Consider cleaning old sessions or running data export/import",
		}
	}

	return DoctorCheck{
		Name:    "Disk Space",
		Status:  "pass",
		Message: fmt.Sprintf("Sofia data: %d MB", sizeMB),
	}
}
