package config

// DefaultModelList returns the built-in catalog of known models, organised by
// provider.  Each entry carries the provider name, a human-readable display
// label, the protocol/model-id, the default API base URL, and (optionally) a
// non-default auth method.  API keys are empty — users fill them via the UI.
//
// This is the single source of truth: the slice is seeded into the database on
// first run and on upgrades, and the frontend reads it back from the
// /api/models endpoint.
func DefaultModelList() []ModelConfig {
	return []ModelConfig{
		// ── Google Gemini ─────────────────────────────────────────────────────
		{Provider: "Google Gemini", DisplayName: "Gemini 3.1 Pro (Preview)", ModelName: "gemini-3.1-pro-preview", Model: "gemini/gemini-3.1-pro-preview", APIBase: "https://generativelanguage.googleapis.com/v1beta/openai"},
		{Provider: "Google Gemini", DisplayName: "Gemini 3.1 Flash-Lite (Preview)", ModelName: "gemini-3.1-flash-lite-preview", Model: "gemini/gemini-3.1-flash-lite-preview", APIBase: "https://generativelanguage.googleapis.com/v1beta/openai"},
		{Provider: "Google Gemini", DisplayName: "Gemini 3 Flash (Preview)", ModelName: "gemini-3-flash-preview", Model: "gemini/gemini-3-flash-preview", APIBase: "https://generativelanguage.googleapis.com/v1beta/openai"},
		{Provider: "Google Gemini", DisplayName: "Gemini 2.5 Pro", ModelName: "gemini-2.5-pro", Model: "gemini/gemini-2.5-pro", APIBase: "https://generativelanguage.googleapis.com/v1beta/openai"},
		{Provider: "Google Gemini", DisplayName: "Gemini 2.5 Flash", ModelName: "gemini-2.5-flash", Model: "gemini/gemini-2.5-flash", APIBase: "https://generativelanguage.googleapis.com/v1beta/openai"},
		{Provider: "Google Gemini", DisplayName: "Gemini 2.5 Flash-Lite", ModelName: "gemini-2.5-flash-lite", Model: "gemini/gemini-2.5-flash-lite", APIBase: "https://generativelanguage.googleapis.com/v1beta/openai"},
		{Provider: "Google Gemini", DisplayName: "Gemini 2.0 Flash", ModelName: "gemini-2.0-flash", Model: "gemini/gemini-2.0-flash", APIBase: "https://generativelanguage.googleapis.com/v1beta/openai"},

		// ── OpenAI ────────────────────────────────────────────────────────────
		{Provider: "OpenAI", DisplayName: "GPT-5.2", ModelName: "gpt-5.2", Model: "openai/gpt-5.2", APIBase: "https://api.openai.com/v1"},
		{Provider: "OpenAI", DisplayName: "GPT-5.2 Pro", ModelName: "gpt-5.2-pro", Model: "openai/gpt-5.2-pro", APIBase: "https://api.openai.com/v1"},
		{Provider: "OpenAI", DisplayName: "GPT-5.2 Codex", ModelName: "gpt-5.2-codex", Model: "openai/gpt-5.2-codex", APIBase: "https://api.openai.com/v1"},
		{Provider: "OpenAI", DisplayName: "GPT-5", ModelName: "gpt-5", Model: "openai/gpt-5", APIBase: "https://api.openai.com/v1"},
		{Provider: "OpenAI", DisplayName: "GPT-5 Mini", ModelName: "gpt-5-mini", Model: "openai/gpt-5-mini", APIBase: "https://api.openai.com/v1"},
		{Provider: "OpenAI", DisplayName: "GPT-5 Nano", ModelName: "gpt-5-nano", Model: "openai/gpt-5-nano", APIBase: "https://api.openai.com/v1"},
		{Provider: "OpenAI", DisplayName: "GPT-4.1", ModelName: "gpt-4.1", Model: "openai/gpt-4.1", APIBase: "https://api.openai.com/v1"},
		{Provider: "OpenAI", DisplayName: "GPT-4o", ModelName: "gpt-4o", Model: "openai/gpt-4o", APIBase: "https://api.openai.com/v1"},
		{Provider: "OpenAI", DisplayName: "GPT-4o Mini", ModelName: "gpt-4o-mini", Model: "openai/gpt-4o-mini", APIBase: "https://api.openai.com/v1"},
		{Provider: "OpenAI", DisplayName: "o3", ModelName: "o3", Model: "openai/o3", APIBase: "https://api.openai.com/v1"},
		{Provider: "OpenAI", DisplayName: "o3 Pro", ModelName: "o3-pro", Model: "openai/o3-pro", APIBase: "https://api.openai.com/v1"},
		{Provider: "OpenAI", DisplayName: "o3 Mini", ModelName: "o3-mini", Model: "openai/o3-mini", APIBase: "https://api.openai.com/v1"},
		{Provider: "OpenAI", DisplayName: "o4 Mini", ModelName: "o4-mini", Model: "openai/o4-mini", APIBase: "https://api.openai.com/v1"},

		// ── Anthropic ─────────────────────────────────────────────────────────
		{Provider: "Anthropic", DisplayName: "Claude Opus 4.6", ModelName: "claude-opus-4-6", Model: "anthropic/claude-opus-4-6", APIBase: "https://api.anthropic.com/v1"},
		{Provider: "Anthropic", DisplayName: "Claude Sonnet 4.6", ModelName: "claude-sonnet-4-6", Model: "anthropic/claude-sonnet-4-6", APIBase: "https://api.anthropic.com/v1"},
		{Provider: "Anthropic", DisplayName: "Claude Opus 4.5", ModelName: "claude-opus-4-5", Model: "anthropic/claude-opus-4-5", APIBase: "https://api.anthropic.com/v1"},
		{Provider: "Anthropic", DisplayName: "Claude Sonnet 4.5", ModelName: "claude-sonnet-4-5", Model: "anthropic/claude-sonnet-4-5", APIBase: "https://api.anthropic.com/v1"},
		{Provider: "Anthropic", DisplayName: "Claude Haiku 4.5", ModelName: "claude-haiku-4-5", Model: "anthropic/claude-haiku-4-5", APIBase: "https://api.anthropic.com/v1"},

		// ── DeepSeek ──────────────────────────────────────────────────────────
		{Provider: "DeepSeek", DisplayName: "DeepSeek V3 (Chat)", ModelName: "deepseek-chat", Model: "deepseek/deepseek-chat", APIBase: "https://api.deepseek.com/v1"},
		{Provider: "DeepSeek", DisplayName: "DeepSeek R1 (Reasoner)", ModelName: "deepseek-reasoner", Model: "deepseek/deepseek-reasoner", APIBase: "https://api.deepseek.com/v1"},

		// ── Groq ──────────────────────────────────────────────────────────────
		{Provider: "Groq", DisplayName: "Llama 3.3 70b", ModelName: "llama-3.3-70b-versatile", Model: "groq/llama-3.3-70b-versatile", APIBase: "https://api.groq.com/openai/v1"},
		{Provider: "Groq", DisplayName: "Mixtral 8x7b", ModelName: "mixtral-8x7b-32768", Model: "groq/mixtral-8x7b-32768", APIBase: "https://api.groq.com/openai/v1"},

		// ── Mistral ───────────────────────────────────────────────────────────
		{Provider: "Mistral", DisplayName: "Mistral Large (Latest)", ModelName: "mistral-large-latest", Model: "mistral/mistral-large-latest", APIBase: "https://api.mistral.ai/v1"},
		{Provider: "Mistral", DisplayName: "Mistral Medium 3.1", ModelName: "mistral-medium-latest", Model: "mistral/mistral-medium-latest", APIBase: "https://api.mistral.ai/v1"},
		{Provider: "Mistral", DisplayName: "Mistral Small 3.2", ModelName: "mistral-small-latest", Model: "mistral/mistral-small-latest", APIBase: "https://api.mistral.ai/v1"},
		{Provider: "Mistral", DisplayName: "Codestral (Latest)", ModelName: "codestral-latest", Model: "mistral/codestral-latest", APIBase: "https://api.mistral.ai/v1"},
		{Provider: "Mistral", DisplayName: "Devstral 2", ModelName: "devstral-latest", Model: "mistral/devstral-latest", APIBase: "https://api.mistral.ai/v1"},
		{Provider: "Mistral", DisplayName: "Pixtral Large", ModelName: "pixtral-large-latest", Model: "mistral/pixtral-large-latest", APIBase: "https://api.mistral.ai/v1"},

		// ── Qwen ──────────────────────────────────────────────────────────────
		{Provider: "Qwen", DisplayName: "Qwen 3.6 Plus", ModelName: "qwen3.6-plus", Model: "qwen/qwen3.6-plus", APIBase: "https://dashscope.aliyuncs.com/compatible-mode/v1"},
		{Provider: "Qwen", DisplayName: "Qwen3.5 Plus", ModelName: "qwen3.5-plus", Model: "qwen/qwen3.5-plus", APIBase: "https://dashscope.aliyuncs.com/compatible-mode/v1"},
		{Provider: "Qwen", DisplayName: "Qwen3 Max", ModelName: "qwen3-max", Model: "qwen/qwen3-max", APIBase: "https://dashscope.aliyuncs.com/compatible-mode/v1"},
		{Provider: "Qwen", DisplayName: "Qwen Plus", ModelName: "qwen-plus-latest", Model: "qwen/qwen-plus-latest", APIBase: "https://dashscope.aliyuncs.com/compatible-mode/v1"},
		{Provider: "Qwen", DisplayName: "Qwen Turbo", ModelName: "qwen-turbo-latest", Model: "qwen/qwen-turbo-latest", APIBase: "https://dashscope.aliyuncs.com/compatible-mode/v1"},
		{Provider: "Qwen", DisplayName: "Qwen3 Coder", ModelName: "qwen3-coder-next", Model: "qwen/qwen3-coder-next", APIBase: "https://dashscope.aliyuncs.com/compatible-mode/v1"},
		{Provider: "Qwen", DisplayName: "Qwen 3.6 Plus (OAuth Free)", ModelName: "qwen3.6-plus-oauth", Model: "qwen/qwen3.6-plus", APIBase: "https://portal.qwen.ai/v1", AuthMethod: "qwen-oauth"},
		{Provider: "Qwen", DisplayName: "Qwen3.5 Plus (OAuth Free)", ModelName: "qwen3.5-plus-oauth", Model: "qwen/qwen3.5-plus", APIBase: "https://portal.qwen.ai/v1", AuthMethod: "qwen-oauth"},
		{Provider: "Qwen", DisplayName: "Qwen3 Max (OAuth Free)", ModelName: "qwen3-max-oauth", Model: "qwen/qwen3-max", APIBase: "https://portal.qwen.ai/v1", AuthMethod: "qwen-oauth"},

		// ── Moonshot ──────────────────────────────────────────────────────────
		{Provider: "Moonshot", DisplayName: "Kimi K2.5", ModelName: "kimi-k2.5", Model: "moonshot/kimi-k2.5", APIBase: "https://api.moonshot.cn/v1"},

		// ── xAI (Grok) ───────────────────────────────────────────────────────
		{Provider: "xAI (Grok)", DisplayName: "Grok 4", ModelName: "grok-4-0709", Model: "grok/grok-4-0709", APIBase: "https://api.x.ai/v1"},
		{Provider: "xAI (Grok)", DisplayName: "Grok 4.1 Fast", ModelName: "grok-4-1-fast-reasoning", Model: "grok/grok-4-1-fast-reasoning", APIBase: "https://api.x.ai/v1"},
		{Provider: "xAI (Grok)", DisplayName: "Grok 3", ModelName: "grok-3", Model: "grok/grok-3", APIBase: "https://api.x.ai/v1"},
		{Provider: "xAI (Grok)", DisplayName: "Grok 3 Mini", ModelName: "grok-3-mini", Model: "grok/grok-3-mini", APIBase: "https://api.x.ai/v1"},
		{Provider: "xAI (Grok)", DisplayName: "Grok 2", ModelName: "grok-2-1212", Model: "grok/grok-2-1212", APIBase: "https://api.x.ai/v1"},

		// ── Z.ai ─────────────────────────────────────────────────────────────
		{Provider: "Z.ai", DisplayName: "glm-5.1", ModelName: "glm-5.1", Model: "zai/glm-5.1", APIBase: "https://api.z.ai/api/paas/v4"},
		{Provider: "Z.ai", DisplayName: "glm-4.7-flash", ModelName: "glm-4.7-flash", Model: "zai/glm-4.7-flash", APIBase: "https://api.z.ai/api/paas/v4"},
		{Provider: "Z.ai", DisplayName: "glm-4.5-air", ModelName: "glm-4.5-air", Model: "zai/glm-4.5-air", APIBase: "https://api.z.ai/api/paas/v4"},

		// ── MiniMax ───────────────────────────────────────────────────────────
		{Provider: "MiniMax", DisplayName: "MiniMax-M2.7", ModelName: "MiniMax-M2.7", Model: "minimax/MiniMax-M2.7", APIBase: "https://api.minimax.io/v1"},
		{Provider: "MiniMax", DisplayName: "MiniMax-M2.7-highspeed", ModelName: "MiniMax-M2.7-highspeed", Model: "minimax/MiniMax-M2.7-highspeed", APIBase: "https://api.minimax.io/v1"},
		{Provider: "MiniMax", DisplayName: "MiniMax-M2.5", ModelName: "MiniMax-M2.5", Model: "minimax/MiniMax-M2.5", APIBase: "https://api.minimax.io/v1"},
		{Provider: "MiniMax", DisplayName: "MiniMax-M2.5-highspeed", ModelName: "MiniMax-M2.5-highspeed", Model: "minimax/MiniMax-M2.5-highspeed", APIBase: "https://api.minimax.io/v1"},

		// ── OpenRouter (Free Tier) ────────────────────────────────────────────
		{Provider: "OpenRouter", DisplayName: "DeepSeek R1 0528 (Free)", ModelName: "deepseek-r1-0528-free", Model: "openrouter/deepseek/deepseek-r1-0528:free", APIBase: "https://openrouter.ai/api/v1"},
		{Provider: "OpenRouter", DisplayName: "DeepSeek V3 0324 (Free)", ModelName: "deepseek-chat-v3-0324-free", Model: "openrouter/deepseek/deepseek-chat-v3-0324:free", APIBase: "https://openrouter.ai/api/v1"},
		{Provider: "OpenRouter", DisplayName: "DeepSeek R1 (Free)", ModelName: "deepseek-r1-free", Model: "openrouter/deepseek/deepseek-r1:free", APIBase: "https://openrouter.ai/api/v1"},
		{Provider: "OpenRouter", DisplayName: "Qwen3 235B A22B (Free)", ModelName: "qwen3-235b-a22b-free", Model: "openrouter/qwen/qwen3-235b-a22b:free", APIBase: "https://openrouter.ai/api/v1"},
		{Provider: "OpenRouter", DisplayName: "Qwen3 32B (Free)", ModelName: "qwen3-32b-free", Model: "openrouter/qwen/qwen3-32b:free", APIBase: "https://openrouter.ai/api/v1"},
		{Provider: "OpenRouter", DisplayName: "Qwen3 30B A3B (Free)", ModelName: "qwen3-30b-a3b-free", Model: "openrouter/qwen/qwen3-30b-a3b:free", APIBase: "https://openrouter.ai/api/v1"},
		{Provider: "OpenRouter", DisplayName: "Qwen3 14B (Free)", ModelName: "qwen3-14b-free", Model: "openrouter/qwen/qwen3-14b:free", APIBase: "https://openrouter.ai/api/v1"},
		{Provider: "OpenRouter", DisplayName: "Qwen3 8B (Free)", ModelName: "qwen3-8b-free", Model: "openrouter/qwen/qwen3-8b:free", APIBase: "https://openrouter.ai/api/v1"},
		{Provider: "OpenRouter", DisplayName: "Llama 4 Scout (Free)", ModelName: "llama-4-scout-free", Model: "openrouter/meta-llama/llama-4-scout:free", APIBase: "https://openrouter.ai/api/v1"},
		{Provider: "OpenRouter", DisplayName: "Llama 4 Maverick (Free)", ModelName: "llama-4-maverick-free", Model: "openrouter/meta-llama/llama-4-maverick:free", APIBase: "https://openrouter.ai/api/v1"},
		{Provider: "OpenRouter", DisplayName: "Llama 3.3 70B Instruct (Free)", ModelName: "llama-3.3-70b-instruct-free", Model: "openrouter/meta-llama/llama-3.3-70b-instruct:free", APIBase: "https://openrouter.ai/api/v1"},
		{Provider: "OpenRouter", DisplayName: "Gemma 3 27B (Free)", ModelName: "gemma-3-27b-free", Model: "openrouter/google/gemma-3-27b-it:free", APIBase: "https://openrouter.ai/api/v1"},
		{Provider: "OpenRouter", DisplayName: "Gemma 3 12B (Free)", ModelName: "gemma-3-12b-free", Model: "openrouter/google/gemma-3-12b-it:free", APIBase: "https://openrouter.ai/api/v1"},
		{Provider: "OpenRouter", DisplayName: "Gemma 3 4B (Free)", ModelName: "gemma-3-4b-free", Model: "openrouter/google/gemma-3-4b-it:free", APIBase: "https://openrouter.ai/api/v1"},
		{Provider: "OpenRouter", DisplayName: "Phi-4 Reasoning Plus (Free)", ModelName: "phi-4-reasoning-plus-free", Model: "openrouter/microsoft/phi-4-reasoning-plus:free", APIBase: "https://openrouter.ai/api/v1"},
		{Provider: "OpenRouter", DisplayName: "Phi-4 (Free)", ModelName: "phi-4-free", Model: "openrouter/microsoft/phi-4:free", APIBase: "https://openrouter.ai/api/v1"},
		{Provider: "OpenRouter", DisplayName: "Mistral Small 3.1 24B (Free)", ModelName: "mistral-small-3.1-free", Model: "openrouter/mistralai/mistral-small-3.1-24b-instruct:free", APIBase: "https://openrouter.ai/api/v1"},
		{Provider: "OpenRouter", DisplayName: "Devstral Small (Free)", ModelName: "devstral-small-free", Model: "openrouter/mistralai/devstral-small:free", APIBase: "https://openrouter.ai/api/v1"},
		{Provider: "OpenRouter", DisplayName: "Hermes 3 Llama 3.1 405B (Free)", ModelName: "hermes-3-llama-405b-free", Model: "openrouter/nousresearch/hermes-3-llama-3.1-405b:free", APIBase: "https://openrouter.ai/api/v1"},
		{Provider: "OpenRouter", DisplayName: "Nemotron 70B Instruct (Free)", ModelName: "nemotron-70b-instruct-free", Model: "openrouter/nvidia/llama-3.1-nemotron-70b-instruct:free", APIBase: "https://openrouter.ai/api/v1"},

		// ── Ollama (Local) ────────────────────────────────────────────────────
		{Provider: "Ollama", DisplayName: "Gemma 4 E4B", ModelName: "ollama-gemma4-e4b", Model: "ollama/gemma4:e4b", APIBase: "http://localhost:11434/v1"},
		{Provider: "Ollama", DisplayName: "Gemma 4 E4B IT Q8", ModelName: "ollama-gemma4-e4b-it-q8", Model: "ollama/gemma4:e4b-it-q8_0", APIBase: "http://localhost:11434/v1"},
		{Provider: "Ollama", DisplayName: "Llama 3.3 70B", ModelName: "ollama-llama3.3-70b", Model: "ollama/llama3.3:70b", APIBase: "http://localhost:11434/v1"},
		{Provider: "Ollama", DisplayName: "Llama 3.3 8B", ModelName: "ollama-llama3.3-8b", Model: "ollama/llama3.3:8b", APIBase: "http://localhost:11434/v1"},
		{Provider: "Ollama", DisplayName: "Qwen 3 32B", ModelName: "ollama-qwen3-32b", Model: "ollama/qwen3:32b", APIBase: "http://localhost:11434/v1"},
		{Provider: "Ollama", DisplayName: "Qwen 3 14B", ModelName: "ollama-qwen3-14b", Model: "ollama/qwen3:14b", APIBase: "http://localhost:11434/v1"},
		{Provider: "Ollama", DisplayName: "Qwen 3 8B", ModelName: "ollama-qwen3-8b", Model: "ollama/qwen3:8b", APIBase: "http://localhost:11434/v1"},
		{Provider: "Ollama", DisplayName: "Qwen 2.5 Coder 32B", ModelName: "ollama-qwen2.5-coder-32b", Model: "ollama/qwen2.5-coder:32b", APIBase: "http://localhost:11434/v1"},
		{Provider: "Ollama", DisplayName: "DeepSeek R1 32B", ModelName: "ollama-deepseek-r1-32b", Model: "ollama/deepseek-r1:32b", APIBase: "http://localhost:11434/v1"},
		{Provider: "Ollama", DisplayName: "DeepSeek R1 14B", ModelName: "ollama-deepseek-r1-14b", Model: "ollama/deepseek-r1:14b", APIBase: "http://localhost:11434/v1"},
		{Provider: "Ollama", DisplayName: "Gemma 3 27B", ModelName: "ollama-gemma3-27b", Model: "ollama/gemma3:27b", APIBase: "http://localhost:11434/v1"},
		{Provider: "Ollama", DisplayName: "Gemma 3 12B", ModelName: "ollama-gemma3-12b", Model: "ollama/gemma3:12b", APIBase: "http://localhost:11434/v1"},
		{Provider: "Ollama", DisplayName: "Mistral Small 24B", ModelName: "ollama-mistral-small-24b", Model: "ollama/mistral-small:24b", APIBase: "http://localhost:11434/v1"},
		{Provider: "Ollama", DisplayName: "Phi-4 14B", ModelName: "ollama-phi4-14b", Model: "ollama/phi4:14b", APIBase: "http://localhost:11434/v1"},
		{Provider: "Ollama", DisplayName: "Command R 35B", ModelName: "ollama-command-r-35b", Model: "ollama/command-r:35b", APIBase: "http://localhost:11434/v1"},

		// ── Ollama Cloud ──────────────────────────────────────────────────────
		{Provider: "Ollama Cloud", DisplayName: "Gemma 4 31B Cloud", ModelName: "ollama-gemma4-31b-cloud", Model: "ollama/gemma4:31b-cloud", APIBase: "https://ollama.com/v1"},
		{Provider: "Ollama Cloud", DisplayName: "Qwen 3 Coder 480B Cloud", ModelName: "ollama-qwen3-coder-480b-cloud", Model: "ollama/qwen3-coder:480b-cloud", APIBase: "https://ollama.com/v1"},
		{Provider: "Ollama Cloud", DisplayName: "GPT-OSS 120B Cloud", ModelName: "ollama-gpt-oss-120b-cloud", Model: "ollama/gpt-oss:120b-cloud", APIBase: "https://ollama.com/v1"},
		{Provider: "Ollama Cloud", DisplayName: "GPT-OSS 20B Cloud", ModelName: "ollama-gpt-oss-20b-cloud", Model: "ollama/gpt-oss:20b-cloud", APIBase: "https://ollama.com/v1"},
		{Provider: "Ollama Cloud", DisplayName: "DeepSeek V3.1 671B Cloud", ModelName: "ollama-deepseek-v3.1-671b-cloud", Model: "ollama/deepseek-v3.1:671b-cloud", APIBase: "https://ollama.com/v1"},
		{Provider: "Ollama Cloud", DisplayName: "GLM 5.1 Cloud", ModelName: "ollama-glm-5.1-cloud", Model: "ollama/glm-5.1:cloud", APIBase: "https://ollama.com/v1"},
	}
}
