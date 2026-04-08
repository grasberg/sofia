package config

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
	}
}
