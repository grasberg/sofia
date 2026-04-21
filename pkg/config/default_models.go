package config

// DefaultModelList returns the built-in catalog of known models, organised by
// provider.  Each entry carries the provider name, a human-readable display
// label, the protocol/model-id, and — where it differs from the well-known
// default — the API base URL. API keys are empty; users fill them via the UI.
//
// Catalog entries also carry model metadata that the agent needs at call time:
//
//   - MaxTokensField: overrides the default "max_tokens" request key for
//     providers/models whose API rejects it. OpenAI's reasoning models
//     (o1/o3/o4) and all GPT-5 variants require "max_completion_tokens";
//     Z.ai's GLM family does too. A missing value means the provider falls
//     back to naming-convention inference (see openai_compat/provider.go),
//     so setting it here is a safety net, not a hard requirement.
//
//   - ContextWindow: drives the agent's summarization thresholds. A wrong or
//     missing value still works (the agent defaults to 128K) but will trigger
//     summarization too early or too late.
//
// Cost fields (CostPer1KInput/Output) are only used for the cost report and
// are safe to leave at 0 when unknown. Never guess — an incorrect cost
// estimate is worse than none.
//
// APIBase is omitted whenever WellKnownProviderBases derives the same URL
// from the Model prefix; ResolveAPIBase() hands the URL back at call time.
// Keep APIBase explicit only where the protocol maps to an unusual endpoint
// (Qwen OAuth portal, Ollama local/cloud split, vLLM deployments).
//
// This is the single source of truth: the slice is seeded into the database on
// first run and on upgrades, and the frontend reads it back from the
// /api/models endpoint.
func DefaultModelList() []ModelConfig {
	const (
		fieldMaxCompletion = "max_completion_tokens"
		ollamaLocalBase    = "http://localhost:11434/v1"
		ollamaCloudBase    = "https://ollama.com/v1"
		qwenOAuthBase      = "https://portal.qwen.ai/v1"
	)

	return []ModelConfig{
		// ── Google Gemini ─────────────────────────────────────────────────────
		{Provider: "Google Gemini", DisplayName: "Gemini 3.1 Pro (Preview)", ModelName: "gemini-3.1-pro-preview", Model: "gemini/gemini-3.1-pro-preview", MaxTokens: 65536, ContextWindow: 1000000},
		{Provider: "Google Gemini", DisplayName: "Gemini 3.1 Flash-Lite (Preview)", ModelName: "gemini-3.1-flash-lite-preview", Model: "gemini/gemini-3.1-flash-lite-preview", MaxTokens: 65536, ContextWindow: 1000000},
		{Provider: "Google Gemini", DisplayName: "Gemini 3 Flash (Preview)", ModelName: "gemini-3-flash-preview", Model: "gemini/gemini-3-flash-preview", MaxTokens: 65536, ContextWindow: 1000000},
		{Provider: "Google Gemini", DisplayName: "Gemini 2.5 Pro", ModelName: "gemini-2.5-pro", Model: "gemini/gemini-2.5-pro", MaxTokens: 65536, ContextWindow: 2000000},
		{Provider: "Google Gemini", DisplayName: "Gemini 2.5 Flash", ModelName: "gemini-2.5-flash", Model: "gemini/gemini-2.5-flash", MaxTokens: 65536, ContextWindow: 1000000},
		{Provider: "Google Gemini", DisplayName: "Gemini 2.5 Flash-Lite", ModelName: "gemini-2.5-flash-lite", Model: "gemini/gemini-2.5-flash-lite", MaxTokens: 65536, ContextWindow: 1000000},
		{Provider: "Google Gemini", DisplayName: "Gemini 2.0 Flash", ModelName: "gemini-2.0-flash", Model: "gemini/gemini-2.0-flash", MaxTokens: 8192, ContextWindow: 1000000},

		// ── OpenAI ────────────────────────────────────────────────────────────
		// GPT-5.x and o-series reject "max_tokens" and require "max_completion_tokens".
		{Provider: "OpenAI", DisplayName: "GPT-5.2", ModelName: "gpt-5.2", Model: "openai/gpt-5.2", MaxTokens: 128000, MaxTokensField: fieldMaxCompletion, ContextWindow: 400000},
		{Provider: "OpenAI", DisplayName: "GPT-5.2 Pro", ModelName: "gpt-5.2-pro", Model: "openai/gpt-5.2-pro", MaxTokens: 128000, MaxTokensField: fieldMaxCompletion, ContextWindow: 400000},
		{Provider: "OpenAI", DisplayName: "GPT-5.2 Codex", ModelName: "gpt-5.2-codex", Model: "openai/gpt-5.2-codex", MaxTokens: 128000, MaxTokensField: fieldMaxCompletion, ContextWindow: 400000},
		{Provider: "OpenAI", DisplayName: "GPT-5", ModelName: "gpt-5", Model: "openai/gpt-5", MaxTokens: 128000, MaxTokensField: fieldMaxCompletion, ContextWindow: 400000},
		{Provider: "OpenAI", DisplayName: "GPT-5 Mini", ModelName: "gpt-5-mini", Model: "openai/gpt-5-mini", MaxTokens: 128000, MaxTokensField: fieldMaxCompletion, ContextWindow: 400000},
		{Provider: "OpenAI", DisplayName: "GPT-5 Nano", ModelName: "gpt-5-nano", Model: "openai/gpt-5-nano", MaxTokens: 128000, MaxTokensField: fieldMaxCompletion, ContextWindow: 400000},
		{Provider: "OpenAI", DisplayName: "GPT-4.1", ModelName: "gpt-4.1", Model: "openai/gpt-4.1", MaxTokens: 32768, ContextWindow: 1000000},
		{Provider: "OpenAI", DisplayName: "GPT-4o", ModelName: "gpt-4o", Model: "openai/gpt-4o", MaxTokens: 16384, ContextWindow: 128000},
		{Provider: "OpenAI", DisplayName: "GPT-4o Mini", ModelName: "gpt-4o-mini", Model: "openai/gpt-4o-mini", MaxTokens: 16384, ContextWindow: 128000},
		{Provider: "OpenAI", DisplayName: "o3", ModelName: "o3", Model: "openai/o3", MaxTokens: 100000, MaxTokensField: fieldMaxCompletion, ContextWindow: 200000},
		{Provider: "OpenAI", DisplayName: "o3 Pro", ModelName: "o3-pro", Model: "openai/o3-pro", MaxTokens: 100000, MaxTokensField: fieldMaxCompletion, ContextWindow: 200000},
		{Provider: "OpenAI", DisplayName: "o3 Mini", ModelName: "o3-mini", Model: "openai/o3-mini", MaxTokens: 100000, MaxTokensField: fieldMaxCompletion, ContextWindow: 200000},
		{Provider: "OpenAI", DisplayName: "o4 Mini", ModelName: "o4-mini", Model: "openai/o4-mini", MaxTokens: 100000, MaxTokensField: fieldMaxCompletion, ContextWindow: 200000},

		// ── OpenAI (ChatGPT OAuth) ───────────────────────────────────────────
		// These entries reuse the OpenAI protocol prefix but set AuthMethod to
		// "oauth", which the provider factory routes through the Codex client
		// (chatgpt.com backend) using credentials from `sofia auth login
		// --provider openai`. They don't need an API key — enabling one in the
		// UI just flags the model as "use my ChatGPT account". APIBase is left
		// to ResolveAPIBase for consistency with other catalog rows, even
		// though the Codex client has its own hard-coded backend URL.
		{Provider: "OpenAI (ChatGPT)", DisplayName: "GPT-5.2 (ChatGPT OAuth)", ModelName: "gpt-5.2-oauth", Model: "openai/gpt-5.2", AuthMethod: "oauth", MaxTokens: 128000, ContextWindow: 400000},
		{Provider: "OpenAI (ChatGPT)", DisplayName: "GPT-5.2 Codex (ChatGPT OAuth)", ModelName: "gpt-5.2-codex-oauth", Model: "openai/gpt-5.2-codex", AuthMethod: "oauth", MaxTokens: 128000, ContextWindow: 400000},
		{Provider: "OpenAI (ChatGPT)", DisplayName: "GPT-5 (ChatGPT OAuth)", ModelName: "gpt-5-oauth", Model: "openai/gpt-5", AuthMethod: "oauth", MaxTokens: 128000, ContextWindow: 400000},
		{Provider: "OpenAI (ChatGPT)", DisplayName: "GPT-5 Mini (ChatGPT OAuth)", ModelName: "gpt-5-mini-oauth", Model: "openai/gpt-5-mini", AuthMethod: "oauth", MaxTokens: 128000, ContextWindow: 400000},
		{Provider: "OpenAI (ChatGPT)", DisplayName: "o3 (ChatGPT OAuth)", ModelName: "o3-oauth", Model: "openai/o3", AuthMethod: "oauth", MaxTokens: 100000, ContextWindow: 200000},
		{Provider: "OpenAI (ChatGPT)", DisplayName: "o3 Mini (ChatGPT OAuth)", ModelName: "o3-mini-oauth", Model: "openai/o3-mini", AuthMethod: "oauth", MaxTokens: 100000, ContextWindow: 200000},
		{Provider: "OpenAI (ChatGPT)", DisplayName: "o4 Mini (ChatGPT OAuth)", ModelName: "o4-mini-oauth", Model: "openai/o4-mini", AuthMethod: "oauth", MaxTokens: 100000, ContextWindow: 200000},

		// ── Anthropic ─────────────────────────────────────────────────────────
		{Provider: "Anthropic", DisplayName: "Claude Opus 4.6", ModelName: "claude-opus-4-6", Model: "anthropic/claude-opus-4-6", MaxTokens: 131072, ContextWindow: 200000},
		{Provider: "Anthropic", DisplayName: "Claude Sonnet 4.6", ModelName: "claude-sonnet-4-6", Model: "anthropic/claude-sonnet-4-6", MaxTokens: 65536, ContextWindow: 200000},
		{Provider: "Anthropic", DisplayName: "Claude Opus 4.5", ModelName: "claude-opus-4-5", Model: "anthropic/claude-opus-4-5", MaxTokens: 131072, ContextWindow: 200000},
		{Provider: "Anthropic", DisplayName: "Claude Sonnet 4.5", ModelName: "claude-sonnet-4-5", Model: "anthropic/claude-sonnet-4-5", MaxTokens: 65536, ContextWindow: 200000},
		{Provider: "Anthropic", DisplayName: "Claude Haiku 4.5", ModelName: "claude-haiku-4-5", Model: "anthropic/claude-haiku-4-5", MaxTokens: 16384, ContextWindow: 200000},

		// ── DeepSeek ──────────────────────────────────────────────────────────
		// DeepSeek caps max_tokens at 8192 server-side — enforced in provider.go.
		{Provider: "DeepSeek", DisplayName: "DeepSeek V3 (Chat)", ModelName: "deepseek-chat", Model: "deepseek/deepseek-chat", MaxTokens: 8192, ContextWindow: 64000},
		{Provider: "DeepSeek", DisplayName: "DeepSeek R1 (Reasoner)", ModelName: "deepseek-reasoner", Model: "deepseek/deepseek-reasoner", MaxTokens: 8192, ContextWindow: 64000},

		// ── Groq ──────────────────────────────────────────────────────────────
		{Provider: "Groq", DisplayName: "Llama 3.3 70b", ModelName: "llama-3.3-70b-versatile", Model: "groq/llama-3.3-70b-versatile", MaxTokens: 8192, ContextWindow: 128000},
		{Provider: "Groq", DisplayName: "Mixtral 8x7b", ModelName: "mixtral-8x7b-32768", Model: "groq/mixtral-8x7b-32768", MaxTokens: 32768, ContextWindow: 32768},

		// ── Mistral ───────────────────────────────────────────────────────────
		{Provider: "Mistral", DisplayName: "Mistral Large (Latest)", ModelName: "mistral-large-latest", Model: "mistral/mistral-large-latest", MaxTokens: 16384, ContextWindow: 128000},
		{Provider: "Mistral", DisplayName: "Mistral Medium 3.1", ModelName: "mistral-medium-latest", Model: "mistral/mistral-medium-latest", MaxTokens: 16384, ContextWindow: 128000},
		{Provider: "Mistral", DisplayName: "Mistral Small 3.2", ModelName: "mistral-small-latest", Model: "mistral/mistral-small-latest", MaxTokens: 8192, ContextWindow: 32000},
		{Provider: "Mistral", DisplayName: "Codestral (Latest)", ModelName: "codestral-latest", Model: "mistral/codestral-latest", MaxTokens: 8192, ContextWindow: 256000},
		{Provider: "Mistral", DisplayName: "Devstral 2", ModelName: "devstral-latest", Model: "mistral/devstral-latest", MaxTokens: 8192, ContextWindow: 128000},
		{Provider: "Mistral", DisplayName: "Pixtral Large", ModelName: "pixtral-large-latest", Model: "mistral/pixtral-large-latest", MaxTokens: 16384, ContextWindow: 128000},

		// ── Qwen ──────────────────────────────────────────────────────────────
		{Provider: "Qwen", DisplayName: "Qwen 3.6 Plus", ModelName: "qwen3.6-plus", Model: "qwen/qwen3.6-plus", MaxTokens: 8192, ContextWindow: 131072},
		{Provider: "Qwen", DisplayName: "Qwen3.5 Plus", ModelName: "qwen3.5-plus", Model: "qwen/qwen3.5-plus", MaxTokens: 8192, ContextWindow: 131072},
		{Provider: "Qwen", DisplayName: "Qwen3 Max", ModelName: "qwen3-max", Model: "qwen/qwen3-max", MaxTokens: 8192, ContextWindow: 32768},
		{Provider: "Qwen", DisplayName: "Qwen Plus", ModelName: "qwen-plus-latest", Model: "qwen/qwen-plus-latest", MaxTokens: 8192, ContextWindow: 131072},
		{Provider: "Qwen", DisplayName: "Qwen Turbo", ModelName: "qwen-turbo-latest", Model: "qwen/qwen-turbo-latest", MaxTokens: 8192, ContextWindow: 1000000},
		{Provider: "Qwen", DisplayName: "Qwen3 Coder", ModelName: "qwen3-coder-next", Model: "qwen/qwen3-coder-next", MaxTokens: 8192, ContextWindow: 131072},
		// Qwen OAuth variants hit a different endpoint (the portal, not Dashscope).
		{Provider: "Qwen", DisplayName: "Qwen 3.6 Plus (OAuth Free)", ModelName: "qwen3.6-plus-oauth", Model: "qwen/qwen3.6-plus", APIBase: qwenOAuthBase, AuthMethod: "qwen-oauth", MaxTokens: 8192, ContextWindow: 131072},
		{Provider: "Qwen", DisplayName: "Qwen3.5 Plus (OAuth Free)", ModelName: "qwen3.5-plus-oauth", Model: "qwen/qwen3.5-plus", APIBase: qwenOAuthBase, AuthMethod: "qwen-oauth", MaxTokens: 8192, ContextWindow: 131072},
		{Provider: "Qwen", DisplayName: "Qwen3 Max (OAuth Free)", ModelName: "qwen3-max-oauth", Model: "qwen/qwen3-max", APIBase: qwenOAuthBase, AuthMethod: "qwen-oauth", MaxTokens: 8192, ContextWindow: 32768},

		// ── Moonshot ──────────────────────────────────────────────────────────
		// Kimi k2 models only accept temperature=1.0 — enforced in provider.go.
		{Provider: "Moonshot", DisplayName: "Kimi K2.5", ModelName: "kimi-k2.5", Model: "moonshot/kimi-k2.5", MaxTokens: 32768, ContextWindow: 262144},

		// ── xAI (Grok) ───────────────────────────────────────────────────────
		{Provider: "xAI (Grok)", DisplayName: "Grok 4", ModelName: "grok-4-0709", Model: "grok/grok-4-0709", MaxTokens: 16384, ContextWindow: 256000},
		{Provider: "xAI (Grok)", DisplayName: "Grok 4.1 Fast", ModelName: "grok-4-1-fast-reasoning", Model: "grok/grok-4-1-fast-reasoning", MaxTokens: 16384, ContextWindow: 256000},
		{Provider: "xAI (Grok)", DisplayName: "Grok 3", ModelName: "grok-3", Model: "grok/grok-3", MaxTokens: 8192, ContextWindow: 131072},
		{Provider: "xAI (Grok)", DisplayName: "Grok 3 Mini", ModelName: "grok-3-mini", Model: "grok/grok-3-mini", MaxTokens: 8192, ContextWindow: 131072},
		{Provider: "xAI (Grok)", DisplayName: "Grok 2", ModelName: "grok-2-1212", Model: "grok/grok-2-1212", MaxTokens: 8192, ContextWindow: 131072},

		// ── Z.ai ─────────────────────────────────────────────────────────────
		// GLM family requires "max_completion_tokens" instead of "max_tokens".
		{Provider: "Z.ai", DisplayName: "glm-5.1", ModelName: "glm-5.1", Model: "zai/glm-5.1", MaxTokens: 8192, MaxTokensField: fieldMaxCompletion, ContextWindow: 131072},
		{Provider: "Z.ai", DisplayName: "glm-4.7-flash", ModelName: "glm-4.7-flash", Model: "zai/glm-4.7-flash", MaxTokens: 8192, MaxTokensField: fieldMaxCompletion, ContextWindow: 131072},
		{Provider: "Z.ai", DisplayName: "glm-4.5-air", ModelName: "glm-4.5-air", Model: "zai/glm-4.5-air", MaxTokens: 8192, MaxTokensField: fieldMaxCompletion, ContextWindow: 131072},

		// ── NVIDIA ────────────────────────────────────────────────────────────
		// Model IDs use an "nvidia/<vendor>/<name>" shape: the "nvidia/" prefix
		// selects the protocol, and the remainder (e.g. "meta/llama-3.1-8b-instruct")
		// is sent verbatim to the NVIDIA NIM endpoint.
		{Provider: "NVIDIA", DisplayName: "Llama 3.1 8B Instruct", ModelName: "nvidia-llama-3.1-8b-instruct", Model: "nvidia/meta/llama-3.1-8b-instruct", MaxTokens: 4096, ContextWindow: 131072},
		{Provider: "NVIDIA", DisplayName: "Gemma 3 4B IT", ModelName: "nvidia-gemma-3-4b-it", Model: "nvidia/google/gemma-3-4b-it", MaxTokens: 4096, ContextWindow: 131072},
		{Provider: "NVIDIA", DisplayName: "Gemma 3 12B IT", ModelName: "nvidia-gemma-3-12b-it", Model: "nvidia/google/gemma-3-12b-it", MaxTokens: 4096, ContextWindow: 131072},
		{Provider: "NVIDIA", DisplayName: "Gemma 3 1B IT", ModelName: "nvidia-gemma-3-1b-it", Model: "nvidia/google/gemma-3-1b-it", MaxTokens: 4096, ContextWindow: 32768},
		{Provider: "NVIDIA", DisplayName: "Llama 3.2 3B Instruct", ModelName: "nvidia-llama-3.2-3b-instruct", Model: "nvidia/meta/llama-3.2-3b-instruct", MaxTokens: 4096, ContextWindow: 131072},
		{Provider: "NVIDIA", DisplayName: "Qwen 2.5 7B Instruct", ModelName: "nvidia-qwen2.5-7b-instruct", Model: "nvidia/qwen/qwen2.5-7b-instruct", MaxTokens: 4096, ContextWindow: 32768},
		{Provider: "NVIDIA", DisplayName: "Qwen3 Coder 480B", ModelName: "nvidia-qwen3-coder-480b", Model: "nvidia/qwen/qwen3-coder-480b-a35b-instruct", MaxTokens: 4096, ContextWindow: 131072},
		{Provider: "NVIDIA", DisplayName: "Solar 10.7B Instruct", ModelName: "nvidia-solar-10.7b-instruct", Model: "nvidia/upstage/solar-10.7b-instruct", MaxTokens: 4096, ContextWindow: 4096},
		{Provider: "NVIDIA", DisplayName: "Falcon3 7B Instruct", ModelName: "nvidia-falcon3-7b-instruct", Model: "nvidia/tiiuae/falcon3-7b-instruct", MaxTokens: 4096, ContextWindow: 32768},
		{Provider: "NVIDIA", DisplayName: "MiniMax M2.7", ModelName: "nvidia-minimax-m2.7", Model: "nvidia/minimaxai/minimax-m2.7", MaxTokens: 4096, ContextWindow: 1000000},

		// ── MiniMax ───────────────────────────────────────────────────────────
		{Provider: "MiniMax", DisplayName: "MiniMax-M2.7", ModelName: "MiniMax-M2.7", Model: "minimax/MiniMax-M2.7", MaxTokens: 8192, ContextWindow: 1000000},
		{Provider: "MiniMax", DisplayName: "MiniMax-M2.7-highspeed", ModelName: "MiniMax-M2.7-highspeed", Model: "minimax/MiniMax-M2.7-highspeed", MaxTokens: 8192, ContextWindow: 1000000},
		{Provider: "MiniMax", DisplayName: "MiniMax-M2.5", ModelName: "MiniMax-M2.5", Model: "minimax/MiniMax-M2.5", MaxTokens: 8192, ContextWindow: 1000000},
		{Provider: "MiniMax", DisplayName: "MiniMax-M2.5-highspeed", ModelName: "MiniMax-M2.5-highspeed", Model: "minimax/MiniMax-M2.5-highspeed", MaxTokens: 8192, ContextWindow: 1000000},

		// ── OpenRouter (Free Tier) ────────────────────────────────────────────
		// Context windows vary per underlying model; OpenRouter normalizes many
		// quirks, so we list the approximate effective value for each id.
		{Provider: "OpenRouter", DisplayName: "DeepSeek R1 0528 (Free)", ModelName: "deepseek-r1-0528-free", Model: "openrouter/deepseek/deepseek-r1-0528:free", MaxTokens: 4096, ContextWindow: 64000},
		{Provider: "OpenRouter", DisplayName: "DeepSeek V3 0324 (Free)", ModelName: "deepseek-chat-v3-0324-free", Model: "openrouter/deepseek/deepseek-chat-v3-0324:free", MaxTokens: 4096, ContextWindow: 64000},
		{Provider: "OpenRouter", DisplayName: "DeepSeek R1 (Free)", ModelName: "deepseek-r1-free", Model: "openrouter/deepseek/deepseek-r1:free", MaxTokens: 4096, ContextWindow: 64000},
		{Provider: "OpenRouter", DisplayName: "Qwen3 235B A22B (Free)", ModelName: "qwen3-235b-a22b-free", Model: "openrouter/qwen/qwen3-235b-a22b:free", MaxTokens: 4096, ContextWindow: 32768},
		{Provider: "OpenRouter", DisplayName: "Qwen3 32B (Free)", ModelName: "qwen3-32b-free", Model: "openrouter/qwen/qwen3-32b:free", MaxTokens: 4096, ContextWindow: 32768},
		{Provider: "OpenRouter", DisplayName: "Qwen3 30B A3B (Free)", ModelName: "qwen3-30b-a3b-free", Model: "openrouter/qwen/qwen3-30b-a3b:free", MaxTokens: 4096, ContextWindow: 32768},
		{Provider: "OpenRouter", DisplayName: "Qwen3 14B (Free)", ModelName: "qwen3-14b-free", Model: "openrouter/qwen/qwen3-14b:free", MaxTokens: 4096, ContextWindow: 32768},
		{Provider: "OpenRouter", DisplayName: "Qwen3 8B (Free)", ModelName: "qwen3-8b-free", Model: "openrouter/qwen/qwen3-8b:free", MaxTokens: 4096, ContextWindow: 32768},
		{Provider: "OpenRouter", DisplayName: "Llama 4 Scout (Free)", ModelName: "llama-4-scout-free", Model: "openrouter/meta-llama/llama-4-scout:free", MaxTokens: 4096, ContextWindow: 128000},
		{Provider: "OpenRouter", DisplayName: "Llama 4 Maverick (Free)", ModelName: "llama-4-maverick-free", Model: "openrouter/meta-llama/llama-4-maverick:free", MaxTokens: 4096, ContextWindow: 128000},
		{Provider: "OpenRouter", DisplayName: "Llama 3.3 70B Instruct (Free)", ModelName: "llama-3.3-70b-instruct-free", Model: "openrouter/meta-llama/llama-3.3-70b-instruct:free", MaxTokens: 4096, ContextWindow: 128000},
		{Provider: "OpenRouter", DisplayName: "Gemma 3 27B (Free)", ModelName: "gemma-3-27b-free", Model: "openrouter/google/gemma-3-27b-it:free", MaxTokens: 4096, ContextWindow: 131072},
		{Provider: "OpenRouter", DisplayName: "Gemma 3 12B (Free)", ModelName: "gemma-3-12b-free", Model: "openrouter/google/gemma-3-12b-it:free", MaxTokens: 4096, ContextWindow: 131072},
		{Provider: "OpenRouter", DisplayName: "Gemma 3 4B (Free)", ModelName: "gemma-3-4b-free", Model: "openrouter/google/gemma-3-4b-it:free", MaxTokens: 4096, ContextWindow: 131072},
		{Provider: "OpenRouter", DisplayName: "Phi-4 Reasoning Plus (Free)", ModelName: "phi-4-reasoning-plus-free", Model: "openrouter/microsoft/phi-4-reasoning-plus:free", MaxTokens: 4096, ContextWindow: 32768},
		{Provider: "OpenRouter", DisplayName: "Phi-4 (Free)", ModelName: "phi-4-free", Model: "openrouter/microsoft/phi-4:free", MaxTokens: 4096, ContextWindow: 16384},
		{Provider: "OpenRouter", DisplayName: "Mistral Small 3.1 24B (Free)", ModelName: "mistral-small-3.1-free", Model: "openrouter/mistralai/mistral-small-3.1-24b-instruct:free", MaxTokens: 4096, ContextWindow: 128000},
		{Provider: "OpenRouter", DisplayName: "Devstral Small (Free)", ModelName: "devstral-small-free", Model: "openrouter/mistralai/devstral-small:free", MaxTokens: 4096, ContextWindow: 32000},
		{Provider: "OpenRouter", DisplayName: "Hermes 3 Llama 3.1 405B (Free)", ModelName: "hermes-3-llama-405b-free", Model: "openrouter/nousresearch/hermes-3-llama-3.1-405b:free", MaxTokens: 4096, ContextWindow: 128000},
		{Provider: "OpenRouter", DisplayName: "Nemotron 70B Instruct (Free)", ModelName: "nemotron-70b-instruct-free", Model: "openrouter/nvidia/llama-3.1-nemotron-70b-instruct:free", MaxTokens: 4096, ContextWindow: 128000},

		// ── Ollama (Local) ────────────────────────────────────────────────────
		// Ollama isn't in WellKnownProviderBases because the "ollama/" protocol
		// covers two different endpoints: local (localhost:11434) and cloud
		// (ollama.com). Keep APIBase explicit on every row so the distinction
		// survives a round-trip through SyncModels. Agent-side, provider.go
		// auto-extends the HTTP timeout for localhost because model load can
		// take minutes on consumer hardware.
		{Provider: "Ollama", DisplayName: "Gemma 4 E4B", ModelName: "ollama-gemma4-e4b", Model: "ollama/gemma4:e4b", APIBase: ollamaLocalBase, MaxTokens: 4096, ContextWindow: 131072},
		{Provider: "Ollama", DisplayName: "Gemma 4 E4B IT Q8", ModelName: "ollama-gemma4-e4b-it-q8", Model: "ollama/gemma4:e4b-it-q8_0", APIBase: ollamaLocalBase, MaxTokens: 4096, ContextWindow: 131072},
		{Provider: "Ollama", DisplayName: "Llama 3.3 70B", ModelName: "ollama-llama3.3-70b", Model: "ollama/llama3.3:70b", APIBase: ollamaLocalBase, MaxTokens: 4096, ContextWindow: 128000},
		{Provider: "Ollama", DisplayName: "Llama 3.3 8B", ModelName: "ollama-llama3.3-8b", Model: "ollama/llama3.3:8b", APIBase: ollamaLocalBase, MaxTokens: 4096, ContextWindow: 128000},
		{Provider: "Ollama", DisplayName: "Qwen 3 32B", ModelName: "ollama-qwen3-32b", Model: "ollama/qwen3:32b", APIBase: ollamaLocalBase, MaxTokens: 4096, ContextWindow: 32768},
		{Provider: "Ollama", DisplayName: "Qwen 3 14B", ModelName: "ollama-qwen3-14b", Model: "ollama/qwen3:14b", APIBase: ollamaLocalBase, MaxTokens: 4096, ContextWindow: 32768},
		{Provider: "Ollama", DisplayName: "Qwen 3 8B", ModelName: "ollama-qwen3-8b", Model: "ollama/qwen3:8b", APIBase: ollamaLocalBase, MaxTokens: 4096, ContextWindow: 32768},
		{Provider: "Ollama", DisplayName: "Qwen 2.5 Coder 32B", ModelName: "ollama-qwen2.5-coder-32b", Model: "ollama/qwen2.5-coder:32b", APIBase: ollamaLocalBase, MaxTokens: 4096, ContextWindow: 32768},
		{Provider: "Ollama", DisplayName: "DeepSeek R1 32B", ModelName: "ollama-deepseek-r1-32b", Model: "ollama/deepseek-r1:32b", APIBase: ollamaLocalBase, MaxTokens: 4096, ContextWindow: 64000},
		{Provider: "Ollama", DisplayName: "DeepSeek R1 14B", ModelName: "ollama-deepseek-r1-14b", Model: "ollama/deepseek-r1:14b", APIBase: ollamaLocalBase, MaxTokens: 4096, ContextWindow: 64000},
		{Provider: "Ollama", DisplayName: "Gemma 3 27B", ModelName: "ollama-gemma3-27b", Model: "ollama/gemma3:27b", APIBase: ollamaLocalBase, MaxTokens: 4096, ContextWindow: 131072},
		{Provider: "Ollama", DisplayName: "Gemma 3 12B", ModelName: "ollama-gemma3-12b", Model: "ollama/gemma3:12b", APIBase: ollamaLocalBase, MaxTokens: 4096, ContextWindow: 131072},
		{Provider: "Ollama", DisplayName: "Mistral Small 24B", ModelName: "ollama-mistral-small-24b", Model: "ollama/mistral-small:24b", APIBase: ollamaLocalBase, MaxTokens: 4096, ContextWindow: 32000},
		{Provider: "Ollama", DisplayName: "Phi-4 14B", ModelName: "ollama-phi4-14b", Model: "ollama/phi4:14b", APIBase: ollamaLocalBase, MaxTokens: 4096, ContextWindow: 16384},
		{Provider: "Ollama", DisplayName: "Command R 35B", ModelName: "ollama-command-r-35b", Model: "ollama/command-r:35b", APIBase: ollamaLocalBase, MaxTokens: 4096, ContextWindow: 131072},

		// ── Ollama Cloud ──────────────────────────────────────────────────────
		{Provider: "Ollama Cloud", DisplayName: "Gemma 4 31B Cloud", ModelName: "ollama-gemma4-31b-cloud", Model: "ollama/gemma4:31b-cloud", APIBase: ollamaCloudBase, MaxTokens: 8192, ContextWindow: 131072},
		{Provider: "Ollama Cloud", DisplayName: "Qwen 3 Coder 480B Cloud", ModelName: "ollama-qwen3-coder-480b-cloud", Model: "ollama/qwen3-coder:480b-cloud", APIBase: ollamaCloudBase, MaxTokens: 8192, ContextWindow: 131072},
		{Provider: "Ollama Cloud", DisplayName: "GPT-OSS 120B Cloud", ModelName: "ollama-gpt-oss-120b-cloud", Model: "ollama/gpt-oss:120b-cloud", APIBase: ollamaCloudBase, MaxTokens: 8192, ContextWindow: 131072},
		{Provider: "Ollama Cloud", DisplayName: "GPT-OSS 20B Cloud", ModelName: "ollama-gpt-oss-20b-cloud", Model: "ollama/gpt-oss:20b-cloud", APIBase: ollamaCloudBase, MaxTokens: 8192, ContextWindow: 131072},
		{Provider: "Ollama Cloud", DisplayName: "DeepSeek V3.1 671B Cloud", ModelName: "ollama-deepseek-v3.1-671b-cloud", Model: "ollama/deepseek-v3.1:671b-cloud", APIBase: ollamaCloudBase, MaxTokens: 8192, ContextWindow: 131072},
		{Provider: "Ollama Cloud", DisplayName: "GLM 5.1 Cloud", ModelName: "ollama-glm-5.1-cloud", Model: "ollama/glm-5.1:cloud", APIBase: ollamaCloudBase, MaxTokens: 8192, ContextWindow: 131072},
	}
}
