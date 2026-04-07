package config

type BraveConfig struct {
	Enabled    bool   `json:"enabled"     env:"SOFIA_TOOLS_WEB_BRAVE_ENABLED"`
	APIKey     string `json:"api_key"     env:"SOFIA_TOOLS_WEB_BRAVE_API_KEY"`
	MaxResults int    `json:"max_results" env:"SOFIA_TOOLS_WEB_BRAVE_MAX_RESULTS"`
}

type TavilyConfig struct {
	Enabled    bool   `json:"enabled"     env:"SOFIA_TOOLS_WEB_TAVILY_ENABLED"`
	APIKey     string `json:"api_key"     env:"SOFIA_TOOLS_WEB_TAVILY_API_KEY"`
	BaseURL    string `json:"base_url"    env:"SOFIA_TOOLS_WEB_TAVILY_BASE_URL"`
	MaxResults int    `json:"max_results" env:"SOFIA_TOOLS_WEB_TAVILY_MAX_RESULTS"`
}

type DuckDuckGoConfig struct {
	Enabled    bool `json:"enabled"     env:"SOFIA_TOOLS_WEB_DUCKDUCKGO_ENABLED"`
	MaxResults int  `json:"max_results" env:"SOFIA_TOOLS_WEB_DUCKDUCKGO_MAX_RESULTS"`
}

type PerplexityConfig struct {
	Enabled    bool   `json:"enabled"     env:"SOFIA_TOOLS_WEB_PERPLEXITY_ENABLED"`
	APIKey     string `json:"api_key"     env:"SOFIA_TOOLS_WEB_PERPLEXITY_API_KEY"`
	MaxResults int    `json:"max_results" env:"SOFIA_TOOLS_WEB_PERPLEXITY_MAX_RESULTS"`
}

type BrowserConfig struct {
	Headless       bool   `json:"headless"        env:"SOFIA_TOOLS_WEB_BROWSER_HEADLESS"`
	TimeoutSeconds int    `json:"timeout_seconds" env:"SOFIA_TOOLS_WEB_BROWSER_TIMEOUT_SECONDS"`
	BrowserType    string `json:"browser_type"    env:"SOFIA_TOOLS_WEB_BROWSER_TYPE"` // "chromium", "firefox", "webkit"
	ScreenshotDir  string `json:"screenshot_dir"  env:"SOFIA_TOOLS_WEB_BROWSER_SCREENSHOT_DIR"`
}

type WebToolsConfig struct {
	Brave      BraveConfig      `json:"brave"`
	Tavily     TavilyConfig     `json:"tavily"`
	DuckDuckGo DuckDuckGoConfig `json:"duckduckgo"`
	Perplexity PerplexityConfig `json:"perplexity"`
	Browser    BrowserConfig    `json:"browser"`
	// Proxy is an optional proxy URL for web tools (http/https/socks5/socks5h).
	// For authenticated proxies, prefer HTTP_PROXY/HTTPS_PROXY env vars instead of embedding credentials in config.
	Proxy string `json:"proxy,omitempty" env:"SOFIA_TOOLS_WEB_PROXY"`
}

type CronToolsConfig struct {
	ExecTimeoutMinutes int `json:"exec_timeout_minutes" env:"SOFIA_TOOLS_CRON_EXEC_TIMEOUT_MINUTES"` // 0 means no timeout
}

type ExecConfig struct {
	EnableDenyPatterns bool     `json:"enable_deny_patterns" env:"SOFIA_TOOLS_EXEC_ENABLE_DENY_PATTERNS"`
	CustomDenyPatterns []string `json:"custom_deny_patterns" env:"SOFIA_TOOLS_EXEC_CUSTOM_DENY_PATTERNS"`
	ConfirmPatterns    []string `json:"confirm_patterns"     env:"SOFIA_TOOLS_EXEC_CONFIRM_PATTERNS"`
}

type GoogleToolsConfig struct {
	Enabled         bool     `json:"enabled"          env:"SOFIA_TOOLS_GOOGLE_ENABLED"`
	BinaryPath      string   `json:"binary_path"      env:"SOFIA_TOOLS_GOOGLE_BINARY_PATH"`
	TimeoutSeconds  int      `json:"timeout_seconds"  env:"SOFIA_TOOLS_GOOGLE_TIMEOUT_SECONDS"`
	AllowedCommands []string `json:"allowed_commands" env:"SOFIA_TOOLS_GOOGLE_ALLOWED_COMMANDS"`
}

type MCPServerConfig struct {
	Command string            `json:"command"`
	Args    []string          `json:"args,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
}

// BraveSearchConfig configures the Brave Search web search tool.
type BraveSearchConfig struct {
	Enabled bool   `json:"enabled" env:"SOFIA_TOOLS_BRAVE_SEARCH_ENABLED"`
	APIKey  string `json:"api_key" env:"SOFIA_TOOLS_BRAVE_SEARCH_API_KEY"`
}

// GitHubCLIConfig configures the GitHub CLI (gh) tool.
type GitHubCLIConfig struct {
	Enabled         bool     `json:"enabled"          env:"SOFIA_TOOLS_GITHUB_ENABLED"`
	BinaryPath      string   `json:"binary_path"      env:"SOFIA_TOOLS_GITHUB_BINARY_PATH"`
	TimeoutSeconds  int      `json:"timeout_seconds"  env:"SOFIA_TOOLS_GITHUB_TIMEOUT_SECONDS"`
	AllowedCommands []string `json:"allowed_commands" env:"SOFIA_TOOLS_GITHUB_ALLOWED_COMMANDS"`
}

// CpanelConfig configures the cPanel hosting management tool.
type CpanelConfig struct {
	Enabled  bool   `json:"enabled"   env:"SOFIA_TOOLS_CPANEL_ENABLED"`
	Host     string `json:"host"      env:"SOFIA_TOOLS_CPANEL_HOST"`
	Port     int    `json:"port"      env:"SOFIA_TOOLS_CPANEL_PORT"`
	Username string `json:"username"  env:"SOFIA_TOOLS_CPANEL_USERNAME"`
	APIToken string `json:"api_token" env:"SOFIA_TOOLS_CPANEL_API_TOKEN"`
}

// BitcoinConfig configures the Bitcoin wallet and blockchain tool.
type BitcoinConfig struct {
	Enabled    bool   `json:"enabled"     env:"SOFIA_TOOLS_BITCOIN_ENABLED"`
	Network    string `json:"network"     env:"SOFIA_TOOLS_BITCOIN_NETWORK"`     // mainnet, testnet, signet
	WalletPath string `json:"wallet_path" env:"SOFIA_TOOLS_BITCOIN_WALLET_PATH"` // path to encrypted wallet file
	Passphrase string `json:"passphrase"  env:"SOFIA_TOOLS_BITCOIN_PASSPHRASE"`  // wallet encryption passphrase
}

// PorkbunConfig configures the Porkbun domain management tool.
type PorkbunConfig struct {
	Enabled      bool   `json:"enabled"        env:"SOFIA_TOOLS_PORKBUN_ENABLED"`
	APIKey       string `json:"api_key"        env:"SOFIA_TOOLS_PORKBUN_API_KEY"`
	SecretAPIKey string `json:"secret_api_key" env:"SOFIA_TOOLS_PORKBUN_SECRET_API_KEY"`
}

// VercelConfig configures the Vercel CLI deployment tool.
type VercelConfig struct {
	Enabled         bool     `json:"enabled"          env:"SOFIA_TOOLS_VERCEL_ENABLED"`
	BinaryPath      string   `json:"binary_path"      env:"SOFIA_TOOLS_VERCEL_BINARY_PATH"`
	TimeoutSeconds  int      `json:"timeout_seconds"  env:"SOFIA_TOOLS_VERCEL_TIMEOUT_SECONDS"`
	AllowedCommands []string `json:"allowed_commands" env:"SOFIA_TOOLS_VERCEL_ALLOWED_COMMANDS"`
}

type ToolsConfig struct {
	Web         WebToolsConfig             `json:"web"`
	Cron        CronToolsConfig            `json:"cron"`
	Exec        ExecConfig                 `json:"exec"`
	Google      GoogleToolsConfig          `json:"google"`
	GitHub      GitHubCLIConfig            `json:"github"`
	BraveSearch BraveSearchConfig          `json:"brave_search"`
	Porkbun     PorkbunConfig              `json:"porkbun"`
	Cpanel      CpanelConfig               `json:"cpanel"`
	Bitcoin     BitcoinConfig              `json:"bitcoin"`
	Vercel      VercelConfig               `json:"vercel"`
	Skills      SkillsToolsConfig          `json:"skills"`
	MCP         map[string]MCPServerConfig `json:"mcp,omitempty"`
}

type SkillsToolsConfig struct {
	Registries            SkillsRegistriesConfig `json:"registries"`
	MaxConcurrentSearches int                    `json:"max_concurrent_searches" env:"SOFIA_SKILLS_MAX_CONCURRENT_SEARCHES"`
	SearchCache           SearchCacheConfig      `json:"search_cache"`
}

type SearchCacheConfig struct {
	MaxSize    int `json:"max_size"    env:"SOFIA_SKILLS_SEARCH_CACHE_MAX_SIZE"`
	TTLSeconds int `json:"ttl_seconds" env:"SOFIA_SKILLS_SEARCH_CACHE_TTL_SECONDS"`
}

type SkillsRegistriesConfig struct {
	ClawHub ClawHubRegistryConfig `json:"clawhub"`
}

type ClawHubRegistryConfig struct {
	Enabled         bool   `json:"enabled"           env:"SOFIA_SKILLS_REGISTRIES_CLAWHUB_ENABLED"`
	BaseURL         string `json:"base_url"          env:"SOFIA_SKILLS_REGISTRIES_CLAWHUB_BASE_URL"`
	AuthToken       string `json:"auth_token"        env:"SOFIA_SKILLS_REGISTRIES_CLAWHUB_AUTH_TOKEN"`
	SearchPath      string `json:"search_path"       env:"SOFIA_SKILLS_REGISTRIES_CLAWHUB_SEARCH_PATH"`
	SkillsPath      string `json:"skills_path"       env:"SOFIA_SKILLS_REGISTRIES_CLAWHUB_SKILLS_PATH"`
	DownloadPath    string `json:"download_path"     env:"SOFIA_SKILLS_REGISTRIES_CLAWHUB_DOWNLOAD_PATH"`
	Timeout         int    `json:"timeout"           env:"SOFIA_SKILLS_REGISTRIES_CLAWHUB_TIMEOUT"`
	MaxZipSize      int    `json:"max_zip_size"      env:"SOFIA_SKILLS_REGISTRIES_CLAWHUB_MAX_ZIP_SIZE"`
	MaxResponseSize int    `json:"max_response_size" env:"SOFIA_SKILLS_REGISTRIES_CLAWHUB_MAX_RESPONSE_SIZE"`
}
