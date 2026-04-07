# Ollama Cloud Provider Setup Guide

## Overview

Sofia now supports **Ollama Cloud** as a provider, giving you access to cloud-hosted models like `gemma4:31b-cloud` without needing local GPU resources.

## What is Ollama Cloud?

Ollama Cloud provides API access to large models hosted on ollama.com's infrastructure. Unlike local Ollama (which runs on `localhost:11434`), Ollama Cloud:

- Uses `https://ollama.com/v1` as the API base URL
- Requires an API key for authentication
- Supports models with `-cloud` suffix (e.g., `gemma4:31b-cloud`)
- Is OpenAI-compatible (works with existing OpenAI provider integration)

## Available Cloud Models

As of 2026, Ollama Cloud offers:

- **gemma4:31b-cloud** - Google's Gemma 4 (31B parameters) - Great for general tasks
- **qwen3-coder:480b-cloud** - Qwen 3 Coder (480B) - Excellent for coding
- **gpt-oss:120b-cloud** - Open GPT-OSS (120B) - General purpose
- **gpt-oss:20b-cloud** - Open GPT-OSS (20B) - Faster, lighter
- **deepseek-v3.1:671b-cloud** - DeepSeek V3.1 (671B) - Massive model for complex tasks

## Setup Instructions

### Step 1: Get Your Ollama API Key

1. Visit [https://ollama.com](https://ollama.com)
2. Sign in or create an account
3. Run `ollama signin` in your terminal to authenticate
4. Get your API key from your account settings or via:
   ```bash
   ollama api-key
   ```

### Step 2: Configure Sofia

#### Option A: Environment Variables (Recommended)

```bash
export SOFIA_PROVIDERS_OLLAMA_API_KEY="your-ollama-api-key"
export SOFIA_AGENTS_DEFAULTS_PROVIDER="ollama_cloud"
export SOFIA_AGENTS_DEFAULTS_MODEL_NAME="gemma4:31b-cloud"
```

#### Option B: Configuration File

Edit your `~/.sofia/config.json`:

```json
{
  "providers": {
    "ollama": {
      "api_key": "your-ollama-api-key",
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

#### Option C: Model List Configuration

For advanced configurations with fallbacks:

```json
{
  "model_list": [
    {
      "model_name": "gemma4:31b-cloud",
      "model": "ollama/gemma4:31b-cloud",
      "api_base": "https://ollama.com/v1",
      "api_key": "your-ollama-api-key",
      "is_default": true
    },
    {
      "model_name": "fallback-local",
      "model": "ollama/llama3.2",
      "api_base": "http://localhost:11434/v1",
      "api_key": ""
    }
  ]
}
```

### Step 3: Start Sofia

```bash
sofia agent "Hello! Can you help me with a coding task?"
```

Or run in gateway mode:

```bash
sofia gateway
```

## Configuration Details

### Required Fields

| Field | Environment Variable | Description | Example |
|-------|---------------------|-------------|---------|
| `providers.ollama.api_key` | `SOFIA_PROVIDERS_OLLAMA_API_KEY` | Your Ollama API key | `ollama-xxx-yyy` |
| `agents.defaults.provider` | `SOFIA_AGENTS_DEFAULTS_PROVIDER` | Set to `ollama_cloud` | `ollama_cloud` |
| `agents.defaults.model_name` | `SOFIA_AGENTS_DEFAULTS_MODEL_NAME` | Cloud model name | `gemma4:31b-cloud` |

### Optional Fields

| Field | Environment Variable | Default | Description |
|-------|---------------------|---------|-------------|
| `providers.ollama.api_base` | `SOFIA_PROVIDERS_OLLAMA_API_BASE` | `https://ollama.com/v1` | API base URL |
| `providers.ollama.proxy` | `SOFIA_PROVIDERS_OLLAMA_PROXY` | `""` | HTTP proxy URL |
| `agents.defaults.max_tokens` | `SOFIA_AGENTS_DEFAULTS_MAX_TOKENS` | `8192` | Max tokens per response |
| `agents.defaults.temperature` | `SOFIA_AGENTS_DEFAULTS_TEMPERATURE` | `0.7` | Sampling temperature |

## Model Naming Conventions

Sofia supports multiple ways to specify Ollama Cloud models:

### Method 1: Explicit `ollama_cloud` Provider

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

### Method 2: Model Name with `-cloud` Suffix

```json
{
  "agents": {
    "defaults": {
      "provider": "ollama",
      "model_name": "gemma4:31b-cloud"
    }
  }
}
```

Sofia automatically detects the `-cloud` suffix and routes to the cloud endpoint.

### Method 3: Model List Configuration

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

## Troubleshooting

### "API key not found" Error

**Symptom:** Sofia fails to start with API key error.

**Solution:**
```bash
# Verify the environment variable is set
echo $SOFIA_PROVIDERS_OLLAMA_API_KEY

# Or check your config.json
cat ~/.sofia/config.json | grep api_key
```

### "Connection refused" Error

**Symptom:** Cannot connect to Ollama Cloud.

**Solution:** Verify the API base URL:
```bash
# Test the endpoint
curl -H "Authorization: Bearer $SOFIA_PROVIDERS_OLLAMA_API_KEY" \
  https://ollama.com/api/tags
```

### "Model not found" Error

**Symptom:** Model `gemma4:31b-cloud` not found.

**Solution:** Check available models:
```bash
curl -H "Authorization: Bearer $SOFIA_PROVIDERS_OLLAMA_API_KEY" \
  https://ollama.com/api/tags | jq '.models[].name'
```

### Rate Limiting

Ollama Cloud has rate limits based on your plan. If you hit limits:

1. Check your usage in your Ollama account
2. Consider upgrading your plan
3. Implement request throttling in Sofia's config

## Performance Tips

### 1. Use Appropriate Max Tokens

For `gemma4:31b-cloud`:
```json
{
  "agents": {
    "defaults": {
      "model_name": "gemma4:31b-cloud",
      "max_tokens": 8192
    }
  }
}
```

### 2. Enable Parallel Tool Calls

```json
{
  "agents": {
    "defaults": {
      "parallel_tool_calls": true
    }
  }
}
```

### 3. Set Optimal Temperature

- **Coding tasks:** `0.2 - 0.4`
- **General assistant:** `0.7`
- **Creative writing:** `0.8 - 0.9`

## Example: Complete Configuration

Here's a complete `config.json` for Ollama Cloud with Gemma 4:

```json
{
  "providers": {
    "ollama": {
      "api_key": "ollama-your-api-key-here",
      "api_base": "https://ollama.com/v1"
    }
  },
  "agents": {
    "defaults": {
      "provider": "ollama_cloud",
      "model_name": "gemma4:31b-cloud",
      "max_tokens": 8192,
      "temperature": 0.7,
      "max_tool_iterations": 50,
      "parallel_tool_calls": true,
      "doom_loop_detection": {
        "enabled": true,
        "repetition_threshold": 3
      }
    }
  },
  "memory_db": "~/.sofia/memory.db",
  "workspace": "~/.sofia/workspace"
}
```

## Migration from Local Ollama

If you're switching from local Ollama to Ollama Cloud:

1. **Keep local as fallback:**
   ```json
   {
     "agents": {
       "defaults": {
         "provider": "ollama_cloud",
         "model_name": "gemma4:31b-cloud",
         "model_fallbacks": ["llama3.2", "mistral"]
       }
     }
   }
   ```

2. **Update API base:**
   - From: `http://localhost:11434/v1`
   - To: `https://ollama.com/v1`

3. **Add API key:**
   - Local: No key needed (localhost)
   - Cloud: Requires `api_key`

## API Reference

### Ollama Cloud API

- **Base URL:** `https://ollama.com/v1`
- **Chat Endpoint:** `POST /chat/completions`
- **Auth Method:** Bearer token
- **Header:** `Authorization: Bearer <API_KEY>`

### Compatible Endpoints

The Ollama Cloud API is OpenAI-compatible, so it works with:
- OpenAI SDK
- LangChain
- Any OpenAI-compatible client

## Support

- **Ollama Docs:** [https://ollama.com/docs](https://ollama.com/docs)
- **Sofia Issues:** [https://github.com/grasberg/sofia/issues](https://github.com/grasberg/sofia/issues)
- **Community:** Discord channel in Ollama's Discord server

---

**Last Updated:** April 2026  
**Tested With:** Sofia v1.x, Ollama Cloud API v1
