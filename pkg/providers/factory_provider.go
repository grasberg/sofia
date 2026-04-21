// Sofia - Ultra-lightweight personal AI agent
// License: MIT
//
// Copyright (c) 2026 Sofia contributors

package providers

import (
	"fmt"
	"strings"
	"time"

	"github.com/grasberg/sofia/pkg/config"
	"github.com/grasberg/sofia/pkg/providers/openai_compat"
)

// createClaudeAuthProvider creates a Claude provider using OAuth credentials from auth store.
func createClaudeAuthProvider() (LLMProvider, error) {
	cred, err := getCredential("anthropic")
	if err != nil {
		return nil, fmt.Errorf("loading auth credentials: %w", err)
	}
	if cred == nil {
		return nil, fmt.Errorf("no credentials for anthropic. Run: sofia auth login --provider anthropic")
	}
	return NewClaudeProviderWithTokenSource(cred.AccessToken, createClaudeTokenSource()), nil
}

// createCodexAuthProvider creates a Codex provider using OAuth credentials from auth store.
func createCodexAuthProvider() (LLMProvider, error) {
	cred, err := getCredential("openai")
	if err != nil {
		return nil, fmt.Errorf("loading auth credentials: %w", err)
	}
	if cred == nil {
		return nil, fmt.Errorf("no credentials for openai. Run: sofia auth login --provider openai")
	}
	return NewCodexProviderWithTokenSource(cred.AccessToken, cred.AccountID, createCodexTokenSource()), nil
}

// createQwenAuthProvider creates a Qwen provider using OAuth credentials from auth store.
// The token is used as a Bearer token against portal.qwen.ai/v1 (the OAuth endpoint).
func createQwenAuthProvider(requestTimeout int, opts ...openai_compat.Option) (LLMProvider, error) {
	cred, err := getCredential("qwen")
	if err != nil {
		return nil, fmt.Errorf("loading qwen auth credentials: %w", err)
	}
	if cred == nil {
		return nil, fmt.Errorf(
			"no credentials for qwen. Configure Qwen OAuth in Settings or import ~/.qwen/oauth_creds.json",
		)
	}

	apiBase := "https://portal.qwen.ai/v1"

	allOpts := append(opts, openai_compat.WithTokenSource(createQwenTokenSource()))
	return NewHTTPProviderWithMaxTokensFieldAndRequestTimeout(
		cred.AccessToken,
		apiBase,
		"",
		"",
		requestTimeout,
		allOpts...,
	), nil
}

// createQwenTokenSource returns a function that loads and refreshes Qwen OAuth tokens.
func createQwenTokenSource() func() (string, error) {
	return func() (string, error) {
		cred, err := getCredential("qwen")
		if err != nil {
			return "", fmt.Errorf("loading qwen auth credentials: %w", err)
		}
		if cred == nil {
			return "", fmt.Errorf("no credentials for qwen")
		}

		if cred.AuthMethod == "oauth" && cred.NeedsRefresh() && cred.RefreshToken != "" {
			refreshed, rErr := refreshQwenToken(cred)
			if rErr != nil {
				return "", fmt.Errorf("refreshing qwen token: %w", rErr)
			}
			if err := setCredential("qwen", refreshed); err != nil {
				return "", fmt.Errorf("saving refreshed qwen token: %w", err)
			}
			return refreshed.AccessToken, nil
		}

		return cred.AccessToken, nil
	}
}

// ExtractProtocol extracts the protocol prefix and model identifier from a model string.
// If no prefix is specified, it defaults to "openai".
// Examples:
//   - "openai/gpt-4o" -> ("openai", "gpt-4o")
//   - "anthropic/claude-sonnet-4.6" -> ("anthropic", "claude-sonnet-4.6")
//   - "gpt-4o" -> ("openai", "gpt-4o")  // default protocol
func ExtractProtocol(model string) (protocol, modelID string) {
	model = strings.TrimSpace(model)
	protocol, modelID, found := strings.Cut(model, "/")
	if !found {
		return "openai", model
	}
	return protocol, modelID
}

// CreateProviderFromConfig creates a provider based on the ModelConfig.
// It uses the protocol prefix in the Model field to determine which provider to create.
// Supported protocols: openai, anthropic, gemini, deepseek, groq, openrouter, mistral, ollama, nvidia, cerebras, qwen, moonshot, volcengine, grok, zai, minimax
// Returns the provider, the model ID (without protocol prefix), and any error.
func delayOpts(cfg *config.ModelConfig) []openai_compat.Option {
	if cfg.RequestDelay > 0 {
		return []openai_compat.Option{openai_compat.WithRequestDelay(time.Duration(cfg.RequestDelay) * time.Second)}
	}
	return nil
}

// collectAPIKeys returns the deduplicated list of API keys from a ModelConfig.
// APIKey (single) and APIKeys (pool) are merged, preserving order.
func collectAPIKeys(cfg *config.ModelConfig) []string {
	seen := make(map[string]bool)
	var keys []string
	if cfg.APIKey != "" {
		seen[cfg.APIKey] = true
		keys = append(keys, cfg.APIKey)
	}
	for _, k := range cfg.APIKeys {
		if k != "" && !seen[k] {
			seen[k] = true
			keys = append(keys, k)
		}
	}
	return keys
}

// createSingleKeyProvider creates a provider from cfg, forcing the given API key.
// Used by the key-rotation path to build one provider instance per key.
func createSingleKeyProvider(cfg *config.ModelConfig, key string) (LLMProvider, error) {
	single := *cfg
	single.APIKey = key
	single.APIKeys = nil // prevent recursive rotation
	p, _, err := CreateProviderFromConfig(&single)
	return p, err
}

func CreateProviderFromConfig(cfg *config.ModelConfig) (LLMProvider, string, error) {
	if cfg == nil {
		return nil, "", fmt.Errorf("config is nil")
	}

	if cfg.Model == "" {
		return nil, "", fmt.Errorf("model is required")
	}

	protocol, modelID := ExtractProtocol(cfg.Model)

	// Key rotation: when multiple API keys are configured, build one provider
	// per key and wrap them in a KeyRotatingProvider.
	if allKeys := collectAPIKeys(cfg); len(allKeys) > 1 {
		kp := make([]keyedProvider, 0, len(allKeys))
		for _, key := range allKeys {
			p, err := createSingleKeyProvider(cfg, key)
			if err != nil {
				return nil, "", fmt.Errorf("key_pool: creating provider for key %s…: %w",
					key[:min(8, len(key))], err)
			}
			kp = append(kp, keyedProvider{key: key, provider: p})
		}
		pool := NewKeyPool(allKeys, cfg.PoolStrategy)
		return NewKeyRotatingProvider(pool, kp), modelID, nil
	}

	switch protocol {
	case "openai":
		// OpenAI with OAuth/token auth (Codex-style)
		if cfg.AuthMethod == "oauth" || cfg.AuthMethod == "token" {
			provider, err := createCodexAuthProvider()
			if err != nil {
				return nil, "", err
			}
			return provider, modelID, nil
		}
		// OpenAI with API key
		if cfg.APIKey == "" && cfg.APIBase == "" {
			return nil, "", fmt.Errorf("api_key or api_base is required for HTTP-based protocol %q", protocol)
		}
		apiBase := cfg.APIBase
		if apiBase == "" {
			apiBase = getDefaultAPIBase(protocol)
		}
		return NewHTTPProviderWithMaxTokensFieldAndRequestTimeout(
			cfg.APIKey,
			apiBase,
			cfg.Proxy,
			cfg.MaxTokensField,
			cfg.RequestTimeout,
			delayOpts(cfg)...,
		), modelID, nil

	case "qwen":
		// Qwen supports OAuth (qwen-oauth) or standard API key.
		if cfg.AuthMethod == AuthMethodQwenOAuth || cfg.AuthMethod == AuthMethodOAuth {
			provider, err := createQwenAuthProvider(cfg.RequestTimeout, delayOpts(cfg)...)
			if err != nil {
				return nil, "", err
			}
			return provider, modelID, nil
		}
		// Fall through to standard API key path.
		if cfg.APIKey == "" && cfg.APIBase == "" {
			return nil, "", fmt.Errorf("api_key or api_base is required for qwen protocol (model: %s)", cfg.Model)
		}
		apiBase := cfg.APIBase
		if apiBase == "" {
			apiBase = getDefaultAPIBase(protocol)
		}
		return NewHTTPProviderWithMaxTokensFieldAndRequestTimeout(
			cfg.APIKey,
			apiBase,
			cfg.Proxy,
			cfg.MaxTokensField,
			cfg.RequestTimeout,
			delayOpts(cfg)...,
		), modelID, nil

	case "openrouter", "groq", "gemini", "nvidia",
		"ollama", "moonshot", "deepseek", "cerebras",
		"volcengine", "mistral", "grok", "zai", "minimax", "vllm", "shengsuanyun":
		// All other OpenAI-compatible HTTP providers
		if cfg.APIKey == "" && cfg.APIBase == "" {
			return nil, "", fmt.Errorf("api_key or api_base is required for HTTP-based protocol %q", protocol)
		}
		apiBase := cfg.APIBase
		if apiBase == "" {
			apiBase = getDefaultAPIBase(protocol)
		}
		return NewHTTPProviderWithMaxTokensFieldAndRequestTimeout(
			cfg.APIKey,
			apiBase,
			cfg.Proxy,
			cfg.MaxTokensField,
			cfg.RequestTimeout,
			delayOpts(cfg)...,
		), modelID, nil

	case "anthropic":
		if cfg.AuthMethod == "oauth" || cfg.AuthMethod == "token" {
			// Use OAuth credentials from auth store
			provider, err := createClaudeAuthProvider()
			if err != nil {
				return nil, "", err
			}
			return provider, modelID, nil
		}
		// Use API key with HTTP API
		apiBase := cfg.APIBase
		if apiBase == "" {
			apiBase = "https://api.anthropic.com/v1"
		}
		if cfg.APIKey == "" {
			return nil, "", fmt.Errorf("api_key is required for anthropic protocol (model: %s)", cfg.Model)
		}
		return NewHTTPProviderWithMaxTokensFieldAndRequestTimeout(
			cfg.APIKey,
			apiBase,
			cfg.Proxy,
			cfg.MaxTokensField,
			cfg.RequestTimeout,
			delayOpts(cfg)...,
		), modelID, nil

	case "claude-cli", "claude-code", "claudecode":
		workspace := cfg.Workspace
		if workspace == "" {
			workspace = "."
		}
		return NewClaudeCliProvider(workspace), modelID, nil

	case "codex-cli", "codex-code":
		workspace := cfg.Workspace
		if workspace == "" {
			workspace = "."
		}
		return NewCodexCliProvider(workspace), modelID, nil

	case "qwen-cli", "qwen-code":
		workspace := cfg.Workspace
		if workspace == "" {
			workspace = "."
		}
		return NewQwenCliProvider(workspace), modelID, nil

	case "antigravity":
		return NewAntigravityProvider(), modelID, nil

	default:
		return nil, "", fmt.Errorf("unknown protocol %q in model %q", protocol, cfg.Model)
	}
}

// getDefaultAPIBase returns the default API base URL for a given protocol.
func getDefaultAPIBase(protocol string) string {
	switch protocol {
	case "openai":
		return "https://api.openai.com/v1"
	case "openrouter":
		return "https://openrouter.ai/api/v1"
	case "groq":
		return "https://api.groq.com/openai/v1"
	case "gemini":
		return "https://generativelanguage.googleapis.com/v1beta/openai"
	case "nvidia":
		return "https://integrate.api.nvidia.com/v1"
	case "ollama":
		return "http://localhost:11434/v1"
	case "moonshot":
		return "https://api.moonshot.cn/v1"
	case "deepseek":
		return "https://api.deepseek.com/v1"
	case "cerebras":
		return "https://api.cerebras.ai/v1"
	case "volcengine":
		return "https://ark.cn-beijing.volces.com/api/v3"
	case "qwen":
		return "https://dashscope.aliyuncs.com/compatible-mode/v1"
	case "mistral":
		return "https://api.mistral.ai/v1"
	case "grok":
		return "https://api.x.ai/v1"
	case "zai":
		return "https://api.z.ai/api/paas/v4"
	case "minimax":
		return "https://api.minimax.io/v1"
	default:
		return ""
	}
}
