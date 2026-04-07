# Ollama Cloud Provider Implementation

## Summary

Added full support for **Ollama Cloud** as an AI provider in Sofia, enabling access to cloud-hosted models like `gemma4:31b-cloud` without local GPU requirements.

## Implementation Details

### Files Modified

1. **`pkg/providers/factory.go`**
   - Added `ollama_cloud` and `ollama-cloud` as explicit provider names
   - Auto-detect `-cloud` suffix in model names (e.g., `gemma4:31b-cloud`)
   - Default API base: `https://ollama.com/v1`

### Files Created

1. **`docs/OLLAMA_CLOUD_SETUP.md`**
   - Comprehensive setup guide (300+ lines)
   - Configuration examples (3 methods)
   - Troubleshooting section
   - API reference

2. **`docs/OLLAMA_CLOUD_QUICK_REFERENCE.md`**
   - Quick reference card
   - One-line setup commands
   - Supported models table

## How It Works

### Provider Detection Logic

Sofia now supports **three ways** to use Ollama Cloud:

#### Method 1: Explicit Provider Name
```json
{
  "agents": {
    "defaults": {
      "provider": "ollama_cloud",
      "model_name": "gemma4:31b-cloud"
    }
  }
}
```

#### Method 2: Model Name Detection
```json
{
  "agents": {
    "defaults": {
      "provider": "ollama",
      "model_name": "gemma4:31b-cloud"  // Auto-detected as cloud
    }
  }
}
```

#### Method 3: Model List Configuration
```json
{
  "model_list": [
    {
      "model_name": "gemma4:31b-cloud",
      "model": "ollama_cloud/gemma4:31b-cloud",
      "api_base": "https://ollama.com/v1"
    }
  ]
}
```

### Code Changes

**Factory Logic (`pkg/providers/factory.go`):**

```go
// Explicit provider name
case "ollama_cloud", "ollama-cloud":
    if cfg.Providers.Ollama.APIKey != "" {
        sel.apiKey = cfg.Providers.Ollama.APIKey
        sel.apiBase = cfg.Providers.Ollama.APIBase
        if sel.apiBase == "" {
            sel.apiBase = "https://ollama.com/v1"
        }
    }

// Model name auto-detection
case strings.HasSuffix(lowerModel, "-cloud") && cfg.Providers.Ollama.APIKey != "":
    sel.apiKey = cfg.Providers.Ollama.APIKey
    sel.apiBase = cfg.Providers.Ollama.APIBase
    if sel.apiBase == "" {
        sel.apiBase = "https://ollama.com/v1"
    }
```

## Configuration

### Environment Variables

```bash
# Required
export SOFIA_PROVIDERS_OLLAMA_API_KEY="your-api-key"
export SOFIA_AGENTS_DEFAULTS_PROVIDER="ollama_cloud"
export SOFIA_AGENTS_DEFAULTS_MODEL_NAME="gemma4:31b-cloud"

# Optional
export SOFIA_PROVIDERS_OLLAMA_API_BASE="https://ollama.com/v1"
export SOFIA_PROVIDERS_OLLAMA_PROXY=""
export SOFIA_AGENTS_DEFAULTS_MAX_TOKENS="8192"
export SOFIA_AGENTS_DEFAULTS_TEMPERATURE="0.7"
```

### Configuration File

```json
{
  "providers": {
    "ollama": {
      "api_key": "your-api-key",
      "api_base": "https://ollama.com/v1"
    }
  },
  "agents": {
    "defaults": {
      "provider": "ollama_cloud",
      "model_name": "gemma4:31b-cloud",
      "max_tokens": 8192,
      "temperature": 0.7,
      "parallel_tool_calls": true
    }
  }
}
```

## API Details

### Ollama Cloud API

- **Base URL:** `https://ollama.com/v1`
- **Chat Endpoint:** `POST /chat/completions`
- **Auth Method:** Bearer token
- **Header:** `Authorization: Bearer <API_KEY>`
- **Compatibility:** OpenAI-compatible

### Available Models

| Model | Parameters | Use Case |
|-------|-----------|----------|
| `gemma4:31b-cloud` | 31B | General tasks |
| `qwen3-coder:480b-cloud` | 480B | Coding |
| `gpt-oss:120b-cloud` | 120B | General purpose |
| `gpt-oss:20b-cloud` | 20B | Fast tasks |
| `deepseek-v3.1:671b-cloud` | 671B | Complex reasoning |

## Testing

### Build Verification

```bash
cd /Volumes/Slaven/sofia
go build ./pkg/providers ./pkg/config
# ✅ Build successful
```

### Test Results

```bash
go test ./pkg/providers ./pkg/config -count=1
# ok   github.com/grasberg/sofia/pkg/providers    5.555s
# ok   github.com/grasberg/sofia/pkg/config       0.389s
```

## Usage Example

```bash
# Set API key
export SOFIA_PROVIDERS_OLLAMA_API_KEY="ollama-xxx-yyy"

# Run Sofia with Ollama Cloud
sofia agent "Write a Python function to sort a list" \
  --provider ollama_cloud \
  --model gemma4:31b-cloud
```

## Benefits

1. **No Local GPU Required** - Models run on Ollama's cloud infrastructure
2. **Access to Large Models** - Use 480B+ parameter models without local resources
3. **OpenAI Compatible** - Works with existing OpenAI provider integration
4. **Auto-Detection** - Sofia automatically routes `-cloud` models to the cloud endpoint
5. **Flexible Configuration** - Multiple ways to configure (env vars, JSON, model list)

## Backwards Compatibility

✅ **Fully backwards compatible**
- Existing local Ollama configurations continue to work
- No breaking changes to provider interfaces
- Local Ollama remains the default for `ollama` provider without `-cloud` suffix

## Migration Guide

### From Local Ollama to Cloud

1. **Update provider:**
   ```json
   "provider": "ollama_cloud"  // was "ollama"
   ```

2. **Update model:**
   ```json
   "model_name": "gemma4:31b-cloud"  // was "llama3.2"
   ```

3. **Add API key:**
   ```bash
   export SOFIA_PROVIDERS_OLLAMA_API_KEY="your-key"
   ```

4. **Keep local as fallback (optional):**
   ```json
   {
     "model_list": [
       {"model_name": "cloud", "model": "ollama_cloud/gemma4:31b-cloud"},
       {"model_name": "local", "model": "ollama/llama3.2"}
     ]
   }
   ```

## Troubleshooting

### Common Issues

**"API key not found"**
```bash
export SOFIA_PROVIDERS_OLLAMA_API_KEY="your-key"
```

**"Connection refused"**
Verify API base URL:
```bash
curl -H "Authorization: Bearer $SOFIA_PROVIDERS_OLLAMA_API_KEY" \
  https://ollama.com/api/tags
```

**"Model not found"**
Check available cloud models:
```bash
curl -H "Authorization: Bearer $SOFIA_PROVIDERS_OLLAMA_API_KEY" \
  https://ollama.com/api/tags | jq '.models[].name'
```

## Documentation

- **Setup Guide:** `docs/OLLAMA_CLOUD_SETUP.md`
- **Quick Reference:** `docs/OLLAMA_CLOUD_QUICK_REFERENCE.md`
- **Ollama Docs:** https://ollama.com/docs

## Implementation Date

April 5, 2026

## Status

✅ **Complete and Tested**
- Build: ✅ Successful
- Tests: ✅ All pass
- Documentation: ✅ Complete
