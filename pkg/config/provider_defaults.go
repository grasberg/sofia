package config

import "strings"

// anthropicOutputCaps maps model name prefixes to their maximum output token limits.
// Prefix matching is used so that versioned names (e.g. "claude-sonnet-4-5") match
// the canonical prefix (e.g. "claude-sonnet-4").
var anthropicOutputCaps = map[string]int{
	"claude-opus-4":   131072, // 128K
	"claude-sonnet-4": 65536,  // 64K
	"claude-haiku-4":  16384,
}

// AnthropicOutputCap returns the max output tokens for a given Anthropic model name.
// Uses prefix matching: "claude-sonnet-4-5" matches "claude-sonnet-4".
// Returns 0 if the model is unknown.
func AnthropicOutputCap(model string) int {
	for prefix, cap := range anthropicOutputCaps {
		if strings.HasPrefix(model, prefix) {
			return cap
		}
	}
	return 0
}

// WellKnownProviderBases maps protocol prefixes to their default API base URLs.
// This allows declarative model_list entries to omit api_base for well-known providers.
// For example: {"model_name": "my-model", "model": "groq/llama-3.1-70b", "api_key": "..."}
// will automatically use "https://api.groq.com/openai/v1" as the api_base.
var WellKnownProviderBases = map[string]string{
	"openai":     "https://api.openai.com/v1",
	"anthropic":  "https://api.anthropic.com/v1",
	"groq":       "https://api.groq.com/openai/v1",
	"openrouter": "https://openrouter.ai/api/v1",
	"deepseek":   "https://api.deepseek.com/v1",
	"mistral":    "https://api.mistral.ai/v1",
	"cerebras":   "https://api.cerebras.ai/v1",
	"gemini":     "https://generativelanguage.googleapis.com/v1beta/openai",
	"nvidia":     "https://integrate.api.nvidia.com/v1",
	"minimax":    "https://api.minimax.chat/v1",
	"moonshot":   "https://api.moonshot.cn/v1",
	"zhipu":      "https://open.bigmodel.cn/api/paas/v4",
	"grok":       "https://api.x.ai/v1",
	"qwen":       "https://dashscope.aliyuncs.com/compatible-mode/v1",
	"volcengine": "https://ark.cn-beijing.volces.com/api/v3",
	"sambanova":  "https://api.sambanova.ai/v1",
	"together":   "https://api.together.xyz/v1",
	"fireworks":  "https://api.fireworks.ai/inference/v1",
	"perplexity": "https://api.perplexity.ai",
	"cohere":     "https://api.cohere.com/v1",
	"anyscale":   "https://api.endpoints.anyscale.com/v1",
	"lepton":     "https://llm.lepton.run/api/v1",
}

// ResolveAPIBase returns the API base URL for a model config entry.
// If api_base is already set, returns it. Otherwise, tries to derive
// it from the model's protocol prefix using WellKnownProviderBases.
func (c *ModelConfig) ResolveAPIBase() string {
	if c.APIBase != "" {
		return c.APIBase
	}

	// Extract protocol prefix from model (e.g., "groq/llama-3.1-70b" -> "groq")
	parts := splitModelProtocol(c.Model)
	if parts[0] != "" {
		if base, ok := WellKnownProviderBases[parts[0]]; ok {
			return base
		}
	}

	return ""
}

// ResolveModelID returns the model identifier without the protocol prefix.
func (c *ModelConfig) ResolveModelID() string {
	parts := splitModelProtocol(c.Model)
	return parts[1]
}

func splitModelProtocol(model string) [2]string {
	for i := range model {
		if model[i] == '/' {
			return [2]string{model[:i], model[i+1:]}
		}
	}
	return [2]string{"", model}
}
