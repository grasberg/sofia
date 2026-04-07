package providers

// Provider name constants used in config resolution and factory selection.
const (
	ProviderGroq               = "groq"
	ProviderOpenAI             = "openai"
	ProviderOpenAIAlias        = "gpt"
	ProviderAnthropic          = "anthropic"
	ProviderAnthropicAlias     = "claude"
	ProviderOpenRouter         = "openrouter"
	ProviderZhipu              = "zhipu"
	ProviderZhipuAlias         = "glm"
	ProviderGemini             = "gemini"
	ProviderGeminiAlias        = "google"
	ProviderVLLM               = "vllm"
	ProviderShengSuanYun       = "shengsuanyun"
	ProviderNvidia             = "nvidia"
	ProviderMoonshot           = "moonshot"
	ProviderQwen               = "qwen"
	ProviderMiniMax            = "minimax"
	ProviderZai                = "zai"
	ProviderGrok               = "grok"
	ProviderGrokAlias          = "xai"
	ProviderClaudeCLI          = "claude-cli"
	ProviderClaudeCode         = "claude-code"
	ProviderClaudeCodeAlias    = "claudecode"
	ProviderCodexCLI           = "codex-cli"
	ProviderCodexCode          = "codex-code"
	ProviderQwenCLI            = "qwen-cli"
	ProviderQwenCode           = "qwen-code"
	ProviderDeepSeek           = "deepseek"
	ProviderMistral            = "mistral"
	ProviderGitHubCopilot      = "github_copilot"
	ProviderGitHubCopilotAlias = "copilot"
)

// Auth method constants for provider configuration.
const (
	AuthMethodOAuth     = "oauth"
	AuthMethodToken     = "token"
	AuthMethodCodexCLI  = "codex-cli"
	AuthMethodQwenOAuth = "qwen-oauth"
)

// Default API base URLs for each provider.
const (
	DefaultGroqAPIBase          = "https://api.groq.com/openai/v1"
	DefaultOpenAIAPIBase        = "https://api.openai.com/v1"
	DefaultAnthropicAPIBase     = "https://api.anthropic.com/v1"
	DefaultOpenRouterAPIBase    = "https://openrouter.ai/api/v1"
	DefaultZhipuAPIBase         = "https://open.bigmodel.cn/api/paas/v4"
	DefaultGeminiAPIBase        = "https://generativelanguage.googleapis.com/v1beta/openai"
	DefaultShengSuanYunAPIBase  = "https://router.shengsuanyun.com/api/v1"
	DefaultNvidiaAPIBase        = "https://integrate.api.nvidia.com/v1"
	DefaultMoonshotAPIBase      = "https://api.moonshot.cn/v1"
	DefaultQwenAPIBase          = "https://dashscope.aliyuncs.com/compatible-mode/v1"
	DefaultMiniMaxAPIBase       = "https://api.minimax.io/v1"
	DefaultZaiAPIBase           = "https://api.z.ai/api/paas/v4"
	DefaultGrokAPIBase          = "https://api.x.ai/v1"
	DefaultDeepSeekAPIBase      = "https://api.deepseek.com/v1"
	DefaultMistralAPIBase       = "https://api.mistral.ai/v1"
	DefaultGitHubCopilotAPIBase = "http://localhost:4321"
)
