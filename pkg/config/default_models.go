package config

// mergeCatalogEntries appends built-in catalog entries that aren't already
// present in cfg.ModelList (matched by ModelName). User entries always win —
// nothing is removed or overwritten. Called from LoadConfig so that new
// catalog entries added in software updates become available on next restart.
func mergeCatalogEntries(cfg *Config) {
	catalog := defaultModelList()
	existing := make(map[string]bool, len(cfg.ModelList))
	for _, m := range cfg.ModelList {
		existing[m.ModelName] = true
	}
	for _, m := range catalog {
		if !existing[m.ModelName] {
			cfg.ModelList = append(cfg.ModelList, m)
		}
	}
}

// defaultModelList returns the curated catalog of direct API provider models,
// sourced from NousResearch/hermes-agent provider definitions (v0.8.0).
// API keys are intentionally empty — users fill them in via config or env vars.
func defaultModelList() []ModelConfig {
	return []ModelConfig{
		// ── Anthropic ────────────────────────────────────────────────────────
		{ModelName: "claude-opus-4-6",            Model: "anthropic/claude-opus-4-6"},
		{ModelName: "claude-sonnet-4-6",          Model: "anthropic/claude-sonnet-4-6"},
		{ModelName: "claude-opus-4-5-20251101",   Model: "anthropic/claude-opus-4-5-20251101"},
		{ModelName: "claude-sonnet-4-5-20250929", Model: "anthropic/claude-sonnet-4-5-20250929"},
		{ModelName: "claude-opus-4-20250514",     Model: "anthropic/claude-opus-4-20250514"},
		{ModelName: "claude-sonnet-4-20250514",   Model: "anthropic/claude-sonnet-4-20250514"},
		{ModelName: "claude-haiku-4-5-20251001",  Model: "anthropic/claude-haiku-4-5-20251001"},

		// ── Google Gemini (AI Studio) ─────────────────────────────────────────
		// Base URL: https://generativelanguage.googleapis.com/v1beta/openai
		// API key env: GOOGLE_API_KEY or GEMINI_API_KEY
		{ModelName: "gemini-3.1-pro-preview",        Model: "gemini/gemini-3.1-pro-preview"},
		{ModelName: "gemini-3-flash-preview",        Model: "gemini/gemini-3-flash-preview"},
		{ModelName: "gemini-3.1-flash-lite-preview", Model: "gemini/gemini-3.1-flash-lite-preview"},
		{ModelName: "gemini-2.5-pro",                Model: "gemini/gemini-2.5-pro"},
		{ModelName: "gemini-2.5-flash",              Model: "gemini/gemini-2.5-flash"},
		{ModelName: "gemini-2.5-flash-lite",         Model: "gemini/gemini-2.5-flash-lite"},
		{ModelName: "gemma-4-31b-it",                Model: "gemini/gemma-4-31b-it"},
		{ModelName: "gemma-4-26b-it",                Model: "gemini/gemma-4-26b-it"},

		// ── DeepSeek ─────────────────────────────────────────────────────────
		// Base URL: https://api.deepseek.com/v1 — API key env: DEEPSEEK_API_KEY
		{ModelName: "deepseek-chat",     Model: "deepseek/deepseek-chat"},
		{ModelName: "deepseek-reasoner", Model: "deepseek/deepseek-reasoner"},

		// ── Alibaba DashScope (international endpoint) ────────────────────────
		// Base URL: https://dashscope-intl.aliyuncs.com/compatible-mode/v1
		// API key env: DASHSCOPE_API_KEY
		{ModelName: "qwen3.5-plus",         Model: "qwen/qwen3.5-plus"},
		{ModelName: "qwen3-coder-plus",     Model: "qwen/qwen3-coder-plus"},
		{ModelName: "qwen3-coder-next",     Model: "qwen/qwen3-coder-next"},
		{ModelName: "glm-5-alibaba",        Model: "qwen/glm-5"},
		{ModelName: "glm-4.7-alibaba",      Model: "qwen/glm-4.7"},
		{ModelName: "kimi-k2.5-alibaba",    Model: "qwen/kimi-k2.5"},
		{ModelName: "minimax-m2.5-alibaba", Model: "qwen/MiniMax-M2.5"},

		// ── Z.AI / ZhipuAI GLM ───────────────────────────────────────────────
		// Base URL: https://api.z.ai/api/paas/v4 — API key env: GLM_API_KEY or ZAI_API_KEY
		{ModelName: "glm-5",         Model: "zai/glm-5"},
		{ModelName: "glm-5-turbo",   Model: "zai/glm-5-turbo"},
		{ModelName: "glm-4.7",       Model: "zai/glm-4.7"},
		{ModelName: "glm-4.5",       Model: "zai/glm-4.5"},
		{ModelName: "glm-4.5-flash", Model: "zai/glm-4.5-flash"},

		// ── Kimi / Moonshot ───────────────────────────────────────────────────
		// Default base URL: https://api.moonshot.ai/v1 — API key env: KIMI_API_KEY
		// Note: sk-kimi-* keys use https://api.kimi.com/coding/v1 instead.
		{ModelName: "kimi-for-coding",        Model: "moonshot/kimi-for-coding", APIBase: "https://api.kimi.com/coding/v1"},
		{ModelName: "kimi-k2.5",              Model: "moonshot/kimi-k2.5"},
		{ModelName: "kimi-k2-thinking",       Model: "moonshot/kimi-k2-thinking"},
		{ModelName: "kimi-k2-thinking-turbo", Model: "moonshot/kimi-k2-thinking-turbo"},
		{ModelName: "kimi-k2-turbo-preview",  Model: "moonshot/kimi-k2-turbo-preview"},
		{ModelName: "kimi-k2-0905-preview",   Model: "moonshot/kimi-k2-0905-preview"},

		// ── MiniMax ───────────────────────────────────────────────────────────
		// Base URL: https://api.minimax.io/v1 — API key env: MINIMAX_API_KEY
		{ModelName: "MiniMax-M1",      Model: "minimax/MiniMax-M1"},
		{ModelName: "MiniMax-M1-40k",  Model: "minimax/MiniMax-M1-40k"},
		{ModelName: "MiniMax-M1-80k",  Model: "minimax/MiniMax-M1-80k"},
		{ModelName: "MiniMax-M1-128k", Model: "minimax/MiniMax-M1-128k"},
		{ModelName: "MiniMax-M1-256k", Model: "minimax/MiniMax-M1-256k"},
		{ModelName: "MiniMax-M2.5",    Model: "minimax/MiniMax-M2.5"},
		{ModelName: "MiniMax-M2.7",    Model: "minimax/MiniMax-M2.7"},

		// ── OpenRouter ────────────────────────────────────────────────────────
		// Base URL: https://openrouter.ai/api/v1 — API key env: OPENROUTER_API_KEY
		// Model names use the full OpenRouter slug (vendor/model-id).
		{ModelName: "anthropic/claude-opus-4.6",                Model: "openrouter/anthropic/claude-opus-4.6"},
		{ModelName: "anthropic/claude-sonnet-4.6",              Model: "openrouter/anthropic/claude-sonnet-4.6"},
		{ModelName: "anthropic/claude-sonnet-4.5",              Model: "openrouter/anthropic/claude-sonnet-4.5"},
		{ModelName: "anthropic/claude-haiku-4.5",               Model: "openrouter/anthropic/claude-haiku-4.5"},
		{ModelName: "openai/gpt-5.4",                           Model: "openrouter/openai/gpt-5.4"},
		{ModelName: "openai/gpt-5.4-mini",                      Model: "openrouter/openai/gpt-5.4-mini"},
		{ModelName: "openai/gpt-5.4-pro",                       Model: "openrouter/openai/gpt-5.4-pro"},
		{ModelName: "openai/gpt-5.4-nano",                      Model: "openrouter/openai/gpt-5.4-nano"},
		{ModelName: "openai/gpt-5.3-codex",                     Model: "openrouter/openai/gpt-5.3-codex"},
		{ModelName: "xiaomi/mimo-v2-pro",                        Model: "openrouter/xiaomi/mimo-v2-pro"},
		{ModelName: "google/gemini-3-pro-preview",              Model: "openrouter/google/gemini-3-pro-preview"},
		{ModelName: "google/gemini-3-flash-preview",            Model: "openrouter/google/gemini-3-flash-preview"},
		{ModelName: "google/gemini-3.1-pro-preview",            Model: "openrouter/google/gemini-3.1-pro-preview"},
		{ModelName: "google/gemini-3.1-flash-lite-preview",     Model: "openrouter/google/gemini-3.1-flash-lite-preview"},
		{ModelName: "qwen/qwen3.6-plus:free",                   Model: "openrouter/qwen/qwen3.6-plus:free"},
		{ModelName: "qwen/qwen3.5-plus-02-15",                  Model: "openrouter/qwen/qwen3.5-plus-02-15"},
		{ModelName: "qwen/qwen3.5-35b-a3b",                     Model: "openrouter/qwen/qwen3.5-35b-a3b"},
		{ModelName: "stepfun/step-3.5-flash",                   Model: "openrouter/stepfun/step-3.5-flash"},
		{ModelName: "minimax/minimax-m2.7",                     Model: "openrouter/minimax/minimax-m2.7"},
		{ModelName: "minimax/minimax-m2.5",                     Model: "openrouter/minimax/minimax-m2.5"},
		{ModelName: "z-ai/glm-5.1",                             Model: "openrouter/z-ai/glm-5.1"},
		{ModelName: "z-ai/glm-5-turbo",                         Model: "openrouter/z-ai/glm-5-turbo"},
		{ModelName: "moonshotai/kimi-k2.5",                     Model: "openrouter/moonshotai/kimi-k2.5"},
		{ModelName: "x-ai/grok-4.20-beta",                      Model: "openrouter/x-ai/grok-4.20-beta"},
		{ModelName: "nvidia/nemotron-3-super-120b-a12b",         Model: "openrouter/nvidia/nemotron-3-super-120b-a12b"},
		{ModelName: "nvidia/nemotron-3-super-120b-a12b:free",    Model: "openrouter/nvidia/nemotron-3-super-120b-a12b:free"},
		{ModelName: "arcee-ai/trinity-large-preview:free",      Model: "openrouter/arcee-ai/trinity-large-preview:free"},
		{ModelName: "arcee-ai/trinity-large-thinking",          Model: "openrouter/arcee-ai/trinity-large-thinking"},
	}
}
