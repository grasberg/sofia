package providers

import (
	"fmt"
	"strings"

	"github.com/grasberg/sofia/pkg/auth"
	"github.com/grasberg/sofia/pkg/config"
)

var (
	getCredential    = auth.GetCredential
	setCredential    = auth.SetCredential
	refreshQwenToken = auth.RefreshQwenToken
)

type providerType int

const (
	providerTypeHTTPCompat providerType = iota
	providerTypeClaudeAuth
	providerTypeCodexAuth
	providerTypeCodexCLIToken
	providerTypeClaudeCLI
	providerTypeCodexCLI
	providerTypeQwenCLI
	providerTypeGitHubCopilot
)

type providerSelection struct {
	providerType    providerType
	apiKey          string
	apiBase         string
	proxy           string
	model           string
	workspace       string
	connectMode     string
	enableWebSearch bool
}

func resolveProviderSelection(cfg *config.Config) (providerSelection, error) {
	model := cfg.Agents.Defaults.GetModelName()
	providerName := strings.ToLower(cfg.Agents.Defaults.Provider)
	lowerModel := strings.ToLower(model)

	sel := providerSelection{
		providerType: providerTypeHTTPCompat,
		model:        model,
	}

	// First, prefer explicit provider configuration.
	if providerName != "" {
		switch providerName {
		case "ollama_cloud", "ollama-cloud":
			if cfg.Providers.Ollama.APIKey != "" {
				sel.apiKey = cfg.Providers.Ollama.APIKey
				sel.apiBase = cfg.Providers.Ollama.APIBase
				sel.proxy = cfg.Providers.Ollama.Proxy
				if sel.apiBase == "" {
					sel.apiBase = "https://ollama.com/v1"
				}
			}
		case "ollama":
			// Local Ollama: no API key required, defaults to localhost
			sel.apiKey = cfg.Providers.Ollama.APIKey
			sel.apiBase = cfg.Providers.Ollama.APIBase
			sel.proxy = cfg.Providers.Ollama.Proxy
			if sel.apiBase == "" {
				sel.apiBase = "http://localhost:11434/v1"
			}
		case "groq":
			if cfg.Providers.Groq.APIKey != "" {
				sel.apiKey = cfg.Providers.Groq.APIKey
				sel.apiBase = cfg.Providers.Groq.APIBase
				sel.proxy = cfg.Providers.Groq.Proxy
				if sel.apiBase == "" {
					sel.apiBase = DefaultGroqAPIBase
				}
			}
		case "openai", "gpt":
			if cfg.Providers.OpenAI.APIKey != "" || cfg.Providers.OpenAI.AuthMethod != "" {
				sel.enableWebSearch = cfg.Providers.OpenAI.WebSearch
				if cfg.Providers.OpenAI.AuthMethod == AuthMethodCodexCLI {
					sel.providerType = providerTypeCodexCLIToken
					return sel, nil
				}
				if cfg.Providers.OpenAI.AuthMethod == AuthMethodOAuth ||
					cfg.Providers.OpenAI.AuthMethod == AuthMethodToken {
					sel.providerType = providerTypeCodexAuth
					return sel, nil
				}
				sel.apiKey = cfg.Providers.OpenAI.APIKey
				sel.apiBase = cfg.Providers.OpenAI.APIBase
				sel.proxy = cfg.Providers.OpenAI.Proxy
				if sel.apiBase == "" {
					sel.apiBase = DefaultOpenAIAPIBase
				}
			}
		case "anthropic", "claude":
			if cfg.Providers.Anthropic.APIKey != "" || cfg.Providers.Anthropic.AuthMethod != "" {
				if cfg.Providers.Anthropic.AuthMethod == AuthMethodOAuth ||
					cfg.Providers.Anthropic.AuthMethod == AuthMethodToken {
					sel.apiBase = cfg.Providers.Anthropic.APIBase
					if sel.apiBase == "" {
						sel.apiBase = DefaultAnthropicAPIBase
					}
					sel.providerType = providerTypeClaudeAuth
					return sel, nil
				}
				sel.apiKey = cfg.Providers.Anthropic.APIKey
				sel.apiBase = cfg.Providers.Anthropic.APIBase
				sel.proxy = cfg.Providers.Anthropic.Proxy
				if sel.apiBase == "" {
					sel.apiBase = DefaultAnthropicAPIBase
				}
			}
		case "openrouter":
			if cfg.Providers.OpenRouter.APIKey != "" {
				sel.apiKey = cfg.Providers.OpenRouter.APIKey
				sel.proxy = cfg.Providers.OpenRouter.Proxy
				if cfg.Providers.OpenRouter.APIBase != "" {
					sel.apiBase = cfg.Providers.OpenRouter.APIBase
				} else {
					sel.apiBase = DefaultOpenRouterAPIBase
				}
			}
		case "zhipu", "glm":
			if cfg.Providers.Zhipu.APIKey != "" {
				sel.apiKey = cfg.Providers.Zhipu.APIKey
				sel.apiBase = cfg.Providers.Zhipu.APIBase
				sel.proxy = cfg.Providers.Zhipu.Proxy
				if sel.apiBase == "" {
					sel.apiBase = DefaultZhipuAPIBase
				}
			}
		case "gemini", "google":
			if cfg.Providers.Gemini.APIKey != "" {
				sel.apiKey = cfg.Providers.Gemini.APIKey
				sel.apiBase = cfg.Providers.Gemini.APIBase
				sel.proxy = cfg.Providers.Gemini.Proxy
				if sel.apiBase == "" || sel.apiBase == "https://generativelanguage.googleapis.com/v1beta" {
					sel.apiBase = DefaultGeminiAPIBase
				}
			}
		case "vllm":
			if cfg.Providers.VLLM.APIBase != "" {
				sel.apiKey = cfg.Providers.VLLM.APIKey
				sel.apiBase = cfg.Providers.VLLM.APIBase
				sel.proxy = cfg.Providers.VLLM.Proxy
			}
		case "shengsuanyun":
			if cfg.Providers.ShengSuanYun.APIKey != "" {
				sel.apiKey = cfg.Providers.ShengSuanYun.APIKey
				sel.apiBase = cfg.Providers.ShengSuanYun.APIBase
				sel.proxy = cfg.Providers.ShengSuanYun.Proxy
				if sel.apiBase == "" {
					sel.apiBase = DefaultShengSuanYunAPIBase
				}
			}
		case "nvidia":
			if cfg.Providers.Nvidia.APIKey != "" {
				sel.apiKey = cfg.Providers.Nvidia.APIKey
				sel.apiBase = cfg.Providers.Nvidia.APIBase
				sel.proxy = cfg.Providers.Nvidia.Proxy
				if sel.apiBase == "" {
					sel.apiBase = DefaultNvidiaAPIBase
				}
			}
		case "moonshot":
			if cfg.Providers.Moonshot.APIKey != "" {
				sel.apiKey = cfg.Providers.Moonshot.APIKey
				sel.apiBase = cfg.Providers.Moonshot.APIBase
				sel.proxy = cfg.Providers.Moonshot.Proxy
				if sel.apiBase == "" {
					sel.apiBase = DefaultMoonshotAPIBase
				}
			}
		case "qwen":
			if cfg.Providers.Qwen.APIKey != "" {
				sel.apiKey = cfg.Providers.Qwen.APIKey
				sel.apiBase = cfg.Providers.Qwen.APIBase
				sel.proxy = cfg.Providers.Qwen.Proxy
				if sel.apiBase == "" {
					sel.apiBase = DefaultQwenAPIBase
				}
			}
		case "minimax":
			if cfg.Providers.MiniMax.APIKey != "" {
				sel.apiKey = cfg.Providers.MiniMax.APIKey
				sel.apiBase = cfg.Providers.MiniMax.APIBase
				sel.proxy = cfg.Providers.MiniMax.Proxy
				if sel.apiBase == "" {
					sel.apiBase = DefaultMiniMaxAPIBase
				}
			}
		case "zai":
			if cfg.Providers.Zai.APIKey != "" {
				sel.apiKey = cfg.Providers.Zai.APIKey
				sel.apiBase = cfg.Providers.Zai.APIBase
				sel.proxy = cfg.Providers.Zai.Proxy
				if sel.apiBase == "" {
					sel.apiBase = DefaultZaiAPIBase
				}
			}
		case "grok", "xai":
			if cfg.Providers.Grok.APIKey != "" {
				sel.apiKey = cfg.Providers.Grok.APIKey
				sel.apiBase = cfg.Providers.Grok.APIBase
				sel.proxy = cfg.Providers.Grok.Proxy
				if sel.apiBase == "" {
					sel.apiBase = DefaultGrokAPIBase
				}
			}
		case "claude-cli", "claude-code", "claudecode":
			workspace := cfg.WorkspacePath()
			if workspace == "" {
				workspace = "."
			}
			sel.providerType = providerTypeClaudeCLI
			sel.workspace = workspace
			return sel, nil
		case "codex-cli", "codex-code":
			workspace := cfg.WorkspacePath()
			if workspace == "" {
				workspace = "."
			}
			sel.providerType = providerTypeCodexCLI
			sel.workspace = workspace
			return sel, nil
		case "qwen-cli", "qwen-code":
			workspace := cfg.WorkspacePath()
			if workspace == "" {
				workspace = "."
			}
			sel.providerType = providerTypeQwenCLI
			sel.workspace = workspace
			return sel, nil
		case "deepseek":
			if cfg.Providers.DeepSeek.APIKey != "" {
				sel.apiKey = cfg.Providers.DeepSeek.APIKey
				sel.apiBase = cfg.Providers.DeepSeek.APIBase
				sel.proxy = cfg.Providers.DeepSeek.Proxy
				if sel.apiBase == "" {
					sel.apiBase = DefaultDeepSeekAPIBase
				}
				if model != "deepseek-chat" && model != "deepseek-reasoner" {
					sel.model = "deepseek-chat"
				}
			}
		case "mistral":
			if cfg.Providers.Mistral.APIKey != "" {
				sel.apiKey = cfg.Providers.Mistral.APIKey
				sel.apiBase = cfg.Providers.Mistral.APIBase
				sel.proxy = cfg.Providers.Mistral.Proxy
				if sel.apiBase == "" {
					sel.apiBase = DefaultMistralAPIBase
				}
			}
		case "github_copilot", "copilot":
			sel.providerType = providerTypeGitHubCopilot
			if cfg.Providers.GitHubCopilot.APIBase != "" {
				sel.apiBase = cfg.Providers.GitHubCopilot.APIBase
			} else {
				sel.apiBase = DefaultGitHubCopilotAPIBase
			}
			sel.connectMode = cfg.Providers.GitHubCopilot.ConnectMode
			return sel, nil
		}
	}

	// Fallback: infer provider from model and configured keys.
	if sel.apiKey == "" && sel.apiBase == "" {
		switch {
		// OpenRouter model suffixes (e.g. :free, :extended) take priority
		// over provider-prefix matching so nvidia/foo:free routes to OpenRouter, not NVIDIA.
		case isOpenRouterModel(model) && cfg.Providers.OpenRouter.APIKey != "":
			sel.apiKey = cfg.Providers.OpenRouter.APIKey
			sel.proxy = cfg.Providers.OpenRouter.Proxy
			if cfg.Providers.OpenRouter.APIBase != "" {
				sel.apiBase = cfg.Providers.OpenRouter.APIBase
			} else {
				sel.apiBase = DefaultOpenRouterAPIBase
			}
		case (strings.Contains(lowerModel, "kimi") || strings.Contains(lowerModel, "moonshot") || strings.HasPrefix(model, "moonshot/")) && cfg.Providers.Moonshot.APIKey != "":
			sel.apiKey = cfg.Providers.Moonshot.APIKey
			sel.apiBase = cfg.Providers.Moonshot.APIBase
			sel.proxy = cfg.Providers.Moonshot.Proxy
			if sel.apiBase == "" {
				sel.apiBase = DefaultMoonshotAPIBase
			}
		case strings.HasPrefix(model, "openrouter/") ||
			strings.HasPrefix(model, "anthropic/") ||
			strings.HasPrefix(model, "openai/") ||
			strings.HasPrefix(model, "meta-llama/") ||
			strings.HasPrefix(model, "deepseek/") ||
			strings.HasPrefix(model, "google/"):
			sel.apiKey = cfg.Providers.OpenRouter.APIKey
			sel.proxy = cfg.Providers.OpenRouter.Proxy
			if cfg.Providers.OpenRouter.APIBase != "" {
				sel.apiBase = cfg.Providers.OpenRouter.APIBase
			} else {
				sel.apiBase = DefaultOpenRouterAPIBase
			}
		case (strings.Contains(lowerModel, "claude") || strings.HasPrefix(model, "anthropic/")) &&
			(cfg.Providers.Anthropic.APIKey != "" || cfg.Providers.Anthropic.AuthMethod != ""):
			if cfg.Providers.Anthropic.AuthMethod == AuthMethodOAuth ||
				cfg.Providers.Anthropic.AuthMethod == AuthMethodToken {
				sel.apiBase = cfg.Providers.Anthropic.APIBase
				if sel.apiBase == "" {
					sel.apiBase = DefaultAnthropicAPIBase
				}
				sel.providerType = providerTypeClaudeAuth
				return sel, nil
			}
			sel.apiKey = cfg.Providers.Anthropic.APIKey
			sel.apiBase = cfg.Providers.Anthropic.APIBase
			sel.proxy = cfg.Providers.Anthropic.Proxy
			if sel.apiBase == "" {
				sel.apiBase = DefaultAnthropicAPIBase
			}
		case (strings.Contains(lowerModel, "gpt") || strings.HasPrefix(model, "openai/")) &&
			(cfg.Providers.OpenAI.APIKey != "" || cfg.Providers.OpenAI.AuthMethod != ""):
			sel.enableWebSearch = cfg.Providers.OpenAI.WebSearch
			if cfg.Providers.OpenAI.AuthMethod == AuthMethodCodexCLI {
				sel.providerType = providerTypeCodexCLIToken
				return sel, nil
			}
			if cfg.Providers.OpenAI.AuthMethod == AuthMethodOAuth ||
				cfg.Providers.OpenAI.AuthMethod == AuthMethodToken {
				sel.providerType = providerTypeCodexAuth
				return sel, nil
			}
			sel.apiKey = cfg.Providers.OpenAI.APIKey
			sel.apiBase = cfg.Providers.OpenAI.APIBase
			sel.proxy = cfg.Providers.OpenAI.Proxy
			if sel.apiBase == "" {
				sel.apiBase = DefaultOpenAIAPIBase
			}
		case (strings.Contains(lowerModel, "gemini") || strings.HasPrefix(model, "google/")) && cfg.Providers.Gemini.APIKey != "":
			sel.apiKey = cfg.Providers.Gemini.APIKey
			sel.apiBase = cfg.Providers.Gemini.APIBase
			sel.proxy = cfg.Providers.Gemini.Proxy
			if sel.apiBase == "" || sel.apiBase == "https://generativelanguage.googleapis.com/v1beta" {
				sel.apiBase = DefaultGeminiAPIBase
			}
		case (strings.Contains(lowerModel, "glm") || strings.Contains(lowerModel, "zhipu") || strings.Contains(lowerModel, "zai")) && cfg.Providers.Zhipu.APIKey != "":
			sel.apiKey = cfg.Providers.Zhipu.APIKey
			sel.apiBase = cfg.Providers.Zhipu.APIBase
			sel.proxy = cfg.Providers.Zhipu.Proxy
			if sel.apiBase == "" {
				sel.apiBase = DefaultZhipuAPIBase
			}
		case (strings.Contains(lowerModel, "groq") || strings.HasPrefix(model, "groq/")) && cfg.Providers.Groq.APIKey != "":
			sel.apiKey = cfg.Providers.Groq.APIKey
			sel.apiBase = cfg.Providers.Groq.APIBase
			sel.proxy = cfg.Providers.Groq.Proxy
			if sel.apiBase == "" {
				sel.apiBase = DefaultGroqAPIBase
			}
		case (strings.Contains(lowerModel, "nvidia") || strings.HasPrefix(model, "nvidia/")) && cfg.Providers.Nvidia.APIKey != "":
			sel.apiKey = cfg.Providers.Nvidia.APIKey
			sel.apiBase = cfg.Providers.Nvidia.APIBase
			sel.proxy = cfg.Providers.Nvidia.Proxy
			if sel.apiBase == "" {
				sel.apiBase = DefaultNvidiaAPIBase
			}
		case strings.Contains(lowerModel, "ollama") || strings.HasPrefix(model, "ollama/"):
			// Local Ollama: API key optional, defaults to localhost
			sel.apiKey = cfg.Providers.Ollama.APIKey
			sel.apiBase = cfg.Providers.Ollama.APIBase
			sel.proxy = cfg.Providers.Ollama.Proxy
			if sel.apiBase == "" {
				sel.apiBase = "http://localhost:11434/v1"
			}
		case (strings.Contains(lowerModel, "ollama_cloud") || strings.HasPrefix(model, "ollama_cloud/") || strings.HasSuffix(lowerModel, "-cloud")) && cfg.Providers.Ollama.APIKey != "":
			sel.apiKey = cfg.Providers.Ollama.APIKey
			sel.apiBase = cfg.Providers.Ollama.APIBase
			sel.proxy = cfg.Providers.Ollama.Proxy
			if sel.apiBase == "" {
				sel.apiBase = "https://ollama.com/v1"
			}
		case (strings.Contains(lowerModel, "mistral") || strings.HasPrefix(model, "mistral/")) && cfg.Providers.Mistral.APIKey != "":
			sel.apiKey = cfg.Providers.Mistral.APIKey
			sel.apiBase = cfg.Providers.Mistral.APIBase
			sel.proxy = cfg.Providers.Mistral.Proxy
			if sel.apiBase == "" {
				sel.apiBase = DefaultMistralAPIBase
			}
		case cfg.Providers.VLLM.APIBase != "":
			sel.apiKey = cfg.Providers.VLLM.APIKey
			sel.apiBase = cfg.Providers.VLLM.APIBase
			sel.proxy = cfg.Providers.VLLM.Proxy
		default:
			if cfg.Providers.OpenRouter.APIKey != "" {
				sel.apiKey = cfg.Providers.OpenRouter.APIKey
				sel.proxy = cfg.Providers.OpenRouter.Proxy
				if cfg.Providers.OpenRouter.APIBase != "" {
					sel.apiBase = cfg.Providers.OpenRouter.APIBase
				} else {
					sel.apiBase = DefaultOpenRouterAPIBase
				}
			} else {
				return providerSelection{}, fmt.Errorf("no API key configured for model: %s", model)
			}
		}
	}

	if sel.providerType == providerTypeHTTPCompat {
		if sel.apiKey == "" && !strings.HasPrefix(model, "bedrock/") {
			return providerSelection{}, fmt.Errorf("no API key configured for provider (model: %s)", model)
		}
		if sel.apiBase == "" {
			return providerSelection{}, fmt.Errorf("no API base configured for provider (model: %s)", model)
		}
	}

	return sel, nil
}

// isOpenRouterModel returns true if the model ID uses an OpenRouter-specific
// suffix such as ":free" or ":extended".
func isOpenRouterModel(model string) bool {
	return strings.HasSuffix(model, ":free") || strings.HasSuffix(model, ":extended")
}
