package doctor

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/grasberg/sofia/cmd/sofia/internal"
	"github.com/grasberg/sofia/pkg/config"
	"github.com/grasberg/sofia/pkg/memory"
	"github.com/grasberg/sofia/pkg/providers"
)

func printPass(check, detail string) {
	fmt.Printf("  [PASS] %s -- %s\n", check, detail)
}

func printWarn(check, detail string) {
	fmt.Printf("  [WARN] %s -- %s\n", check, detail)
}

func printFail(check, detail string) {
	fmt.Printf("  [FAIL] %s -- %s\n", check, detail)
}

func runDoctor() error {
	fmt.Println("Sofia Doctor -- checking your setup...")
	fmt.Println()

	passed := 0
	warned := 0
	failed := 0

	// 1. Check config file exists and loads
	cfg, p, w, f := checkConfig()
	passed += p
	warned += w
	failed += f

	// 2-7 only run if config loaded successfully
	if cfg != nil {
		p, w, f = checkProviderAPIKeys(cfg)
		passed += p
		warned += w
		failed += f

		p, w, f = checkProviderReachability(cfg)
		passed += p
		warned += w
		failed += f

		p, w, f = checkChannelTokens(cfg)
		passed += p
		warned += w
		failed += f

		p, w, f = checkDatabase(cfg)
		passed += p
		warned += w
		failed += f

		p, w, f = checkWorkspace(cfg)
		passed += p
		warned += w
		failed += f

		p, w, f = checkSecurity(cfg)
		passed += p
		warned += w
		failed += f
	}

	fmt.Printf("\nResults: %d passed, %d warnings, %d failed\n", passed, warned, failed)

	if failed > 0 {
		return fmt.Errorf("%d check(s) failed", failed)
	}
	return nil
}

// sofiaHome returns the Sofia home directory (~/.sofia).
func sofiaHome() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".sofia")
}

// checkConfig verifies the config file exists, loads, and validates.
// It returns the loaded config (nil on failure) and pass/warn/fail counts.
func checkConfig() (cfg *config.Config, passed, warned, failed int) {
	configPath := internal.GetConfigPath()

	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		printFail("Config file", fmt.Sprintf("%s not found", configPath))
		failed++
		return
	}

	loaded, err := internal.LoadConfig()
	if err != nil {
		printFail("Config load", err.Error())
		failed++
		return
	}

	if err := loaded.Validate(); err != nil {
		printFail("Config validation", err.Error())
		failed++
		return
	}

	printPass("Config", fmt.Sprintf("loaded and valid (%s)", configPath))
	passed++
	cfg = loaded
	return
}

// noKeyProtocols lists provider protocols that may work without an API key.
var noKeyProtocols = map[string]bool{
	"ollama":         true,
	"claude-cli":     true,
	"claude-code":    true,
	"codex-cli":      true,
	"github-copilot": true,
}

// checkProviderAPIKeys verifies that model_list entries have API keys where needed.
func checkProviderAPIKeys(cfg *config.Config) (passed, warned, failed int) {
	if len(cfg.ModelList) == 0 {
		printWarn("Provider API keys", "no models configured in model_list")
		warned++
		return
	}

	missing := 0
	for _, m := range cfg.ModelList {
		protocol, _ := providers.ExtractProtocol(m.Model)
		if noKeyProtocols[protocol] {
			continue
		}
		if m.APIKey == "" {
			missing++
		}
	}

	if missing > 0 {
		printWarn("Provider API keys",
			fmt.Sprintf("%d of %d model(s) missing API key", missing, len(cfg.ModelList)))
		warned++
	} else {
		printPass("Provider API keys",
			fmt.Sprintf("all %d model(s) have keys configured", len(cfg.ModelList)))
		passed++
	}
	return
}

// cliProtocols lists protocols that are CLI-based and have no HTTP endpoint.
var cliProtocols = map[string]bool{
	"claude-cli":     true,
	"claude-code":    true,
	"codex-cli":      true,
	"github-copilot": true,
}

// checkProviderReachability does a lightweight HTTP GET to each unique API base URL.
func checkProviderReachability(cfg *config.Config) (passed, warned, failed int) {
	// Collect unique base URLs.
	seen := make(map[string]bool)
	type endpoint struct {
		name    string
		baseURL string
	}
	var endpoints []endpoint

	for _, m := range cfg.ModelList {
		protocol, _ := providers.ExtractProtocol(m.Model)
		if cliProtocols[protocol] {
			continue
		}

		base := m.APIBase
		if base == "" {
			base = defaultAPIBase(protocol)
		}
		if base == "" || seen[base] {
			continue
		}
		seen[base] = true
		endpoints = append(endpoints, endpoint{name: m.ModelName, baseURL: base})
	}

	if len(endpoints) == 0 {
		return
	}

	client := &http.Client{Timeout: 5 * time.Second}
	for _, ep := range endpoints {
		resp, reqErr := client.Get(ep.baseURL) //nolint:gosec // intentional GET for reachability
		if reqErr != nil {
			printFail("Reachability",
				fmt.Sprintf("%s (%s) -- %s", ep.name, ep.baseURL, reqErr.Error()))
			failed++
			continue
		}
		resp.Body.Close()
		printPass("Reachability",
			fmt.Sprintf("%s (%s) -- HTTP %d", ep.name, ep.baseURL, resp.StatusCode))
		passed++
	}
	return
}

// defaultAPIBase mirrors the provider factory defaults for reachability checks.
func defaultAPIBase(protocol string) string {
	defaults := map[string]string{
		"openai":     "https://api.openai.com/v1",
		"anthropic":  "https://api.anthropic.com/v1",
		"openrouter": "https://openrouter.ai/api/v1",
		"groq":       "https://api.groq.com/openai/v1",
		"gemini":     "https://generativelanguage.googleapis.com/v1beta/openai",
		"nvidia":     "https://integrate.api.nvidia.com/v1",
		"ollama":     "http://localhost:11434/v1",
		"moonshot":   "https://api.moonshot.cn/v1",
		"deepseek":   "https://api.deepseek.com/v1",
		"cerebras":   "https://api.cerebras.ai/v1",
		"volcengine": "https://ark.cn-beijing.volces.com/api/v3",
		"qwen":       "https://dashscope.aliyuncs.com/compatible-mode/v1",
		"mistral":    "https://api.mistral.ai/v1",
		"grok":       "https://api.x.ai/v1",
		"zai":        "https://api.z.ai/api/paas/v4",
		"minimax":    "https://api.minimax.io/v1",
	}
	return defaults[protocol]
}

// checkChannelTokens verifies tokens for enabled channels.
func checkChannelTokens(cfg *config.Config) (passed, warned, failed int) {
	tg := cfg.Channels.Telegram
	dc := cfg.Channels.Discord

	if !tg.Enabled && !dc.Enabled {
		printWarn("Channel tokens", "no channels enabled")
		warned++
		return
	}

	if tg.Enabled {
		if tg.Token == "" {
			printFail("Telegram token", "enabled but token is empty")
			failed++
		} else {
			printPass("Telegram token", "configured")
			passed++
		}
	}

	if dc.Enabled {
		if dc.Token == "" {
			printFail("Discord token", "enabled but token is empty")
			failed++
		} else {
			printPass("Discord token", "configured")
			passed++
		}
	}
	return
}

// checkDatabase tries to open the SQLite database.
func checkDatabase(cfg *config.Config) (passed, warned, failed int) {
	dbPath := cfg.MemoryDB
	if dbPath == "" {
		dbPath = filepath.Join(sofiaHome(), "memory.db")
	}

	db, err := memory.Open(dbPath)
	if err != nil {
		printFail("Database", fmt.Sprintf("cannot open %s -- %s", dbPath, err.Error()))
		failed++
		return
	}
	_ = db.Close()

	printPass("Database", fmt.Sprintf("opened successfully (%s)", dbPath))
	passed++
	return
}

// checkWorkspace verifies workspace directory and key files exist.
func checkWorkspace(cfg *config.Config) (passed, warned, failed int) {
	wsPath := cfg.WorkspacePath()
	if wsPath == "" {
		wsPath = filepath.Join(sofiaHome(), "workspace")
	}

	info, err := os.Stat(wsPath)
	if err != nil || !info.IsDir() {
		printWarn("Workspace", fmt.Sprintf("directory not found: %s", wsPath))
		warned++
		return
	}

	var missingFiles []string
	for _, name := range []string{"AGENT.md", "USER.md"} {
		p := filepath.Join(wsPath, name)
		if _, statErr := os.Stat(p); os.IsNotExist(statErr) {
			missingFiles = append(missingFiles, name)
		}
	}

	if len(missingFiles) > 0 {
		printWarn("Workspace", fmt.Sprintf("missing files: %s", strings.Join(missingFiles, ", ")))
		warned++
	} else {
		printPass("Workspace", fmt.Sprintf("AGENT.md and USER.md present (%s)", wsPath))
		passed++
	}
	return
}

// checkSecurity warns about enabled channels with no AllowFrom restrictions.
func checkSecurity(cfg *config.Config) (passed, warned, failed int) {
	var open []string

	if cfg.Channels.Telegram.Enabled && len(cfg.Channels.Telegram.AllowFrom) == 0 {
		open = append(open, "Telegram")
	}
	if cfg.Channels.Discord.Enabled && len(cfg.Channels.Discord.AllowFrom) == 0 {
		open = append(open, "Discord")
	}

	if len(open) > 0 {
		printWarn("Security",
			fmt.Sprintf("%s has no allow_from restriction -- anyone can message your agent",
				strings.Join(open, ", ")))
		warned++
	} else if cfg.Channels.Telegram.Enabled || cfg.Channels.Discord.Enabled {
		printPass("Security", "all enabled channels have allow_from configured")
		passed++
	}
	return
}
