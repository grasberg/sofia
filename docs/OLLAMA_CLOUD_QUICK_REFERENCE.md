# Ollama Cloud Quick Reference

## One-Line Setup

```bash
export SOFIA_PROVIDERS_OLLAMA_API_KEY="your-key"
export SOFIA_AGENTS_DEFAULTS_PROVIDER="ollama_cloud"
export SOFIA_AGENTS_DEFAULTS_MODEL_NAME="gemma4:31b-cloud"
```

## Supported Cloud Models

| Model | Parameters | Best For |
|-------|-----------|----------|
| `gemma4:31b-cloud` | 31B | General tasks, coding, reasoning |
| `qwen3-coder:480b-cloud` | 480B | Coding, code generation |
| `gpt-oss:120b-cloud` | 120B | General purpose |
| `gpt-oss:20b-cloud` | 20B | Fast, lightweight tasks |
| `deepseek-v3.1:671b-cloud` | 671B | Complex reasoning, math |

## Quick Test

```bash
# Set your API key
export SOFIA_PROVIDERS_OLLAMA_API_KEY="ollama-your-key"

# Run a test query
sofia agent "What is 2+2?" --provider ollama_cloud --model gemma4:31b-cloud
```

## Config.json Minimal Example

```json
{
  "providers": {
    "ollama": {
      "api_key": "your-key",
      "api_base": "https://ollama.com/v1"
    }
  },
  "agents": {
    "defaults": {
      "provider": "ollama_cloud",
      "model_name": "gemma4:31b-cloud"
    }
  }
}
```

## Key URLs

- **API Base:** `https://ollama.com/v1`
- **Auth:** `Authorization: Bearer $OLLAMA_API_KEY`
- **Chat:** `POST /chat/completions`
- **Models:** `GET /api/tags`

## Troubleshooting

```bash
# Test API connectivity
curl -H "Authorization: Bearer $SOFIA_PROVIDERS_OLLAMA_API_KEY" \
  https://ollama.com/api/tags

# Check your config
cat ~/.sofia/config.json | jq '.providers.ollama'
```

---

Full guide: [OLLAMA_CLOUD_SETUP.md](OLLAMA_CLOUD_SETUP.md)
