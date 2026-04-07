# Ollama Cloud Web UI Integration

## Summary

Added **Ollama Cloud** as a provider option in Sofia's web UI settings, enabling users to easily configure and select cloud-hosted models like `gemma4:31b-cloud` through the browser.

## Implementation Details

### Files Modified

1. **`pkg/web/templates/settings/models.html`**
   - Added "Ollama Cloud ☁️" option to provider dropdown (line 35)

2. **`pkg/web/templates/layout.html`**
   - Added Ollama Cloud model presets to `PROVIDER_MODELS` constant (5 models)
   - Updated `onProviderChange()` to handle Ollama Cloud provider
   - Updated `onModelChange()` to handle Ollama Cloud custom model entry
   - Updated `openModelForm()` to detect Ollama Cloud models when editing

## Web UI Changes

### Provider Dropdown

**Location:** Settings → Models → Step 1: Select Provider

New option added:
```
Ollama Cloud ☁️
```

Positioned between "Ollama (Local)" and "Custom / Other".

### Model Presets

When "Ollama Cloud" is selected, the following models appear in the dropdown:

| Model | Model ID | API Base |
|-------|----------|----------|
| Gemma 4 31B Cloud | `ollama/gemma4:31b-cloud` | `https://ollama.com/v1` |
| Qwen 3 Coder 480B Cloud | `ollama/qwen3-coder:480b-cloud` | `https://ollama.com/v1` |
| GPT-OSS 120B Cloud | `ollama/gpt-oss:120b-cloud` | `https://ollama.com/v1` |
| GPT-OSS 20B Cloud | `ollama/gpt-oss:20b-cloud` | `https://ollama.com/v1` |
| DeepSeek V3.1 671B Cloud | `ollama/deepseek-v3.1:671b-cloud` | `https://ollama.com/v1` |

Plus an "Other (enter manually)" option for custom cloud models.

### Auto-Configuration

When Ollama Cloud is selected:

1. **API Base** is auto-filled: `https://ollama.com/v1`
2. **API Key** field is cleared with placeholder: "Enter your Ollama Cloud API key"
3. **Model Dropdown** shows available cloud models

### Edit Detection

When editing an existing model, the UI automatically detects Ollama Cloud models by:
- Checking if model starts with `ollama/`
- Checking if model ends with `-cloud`
- Example: `ollama/gemma4:31b-cloud` → Provider: "Ollama Cloud"

## User Workflow

### Adding an Ollama Cloud Model

1. **Open Settings** → **Models**
2. Click **"Add Model"**
3. **Select Provider:** Choose "Ollama Cloud ☁️"
4. **Select Model:** Choose from dropdown or "Other" for custom
5. **Configure:**
   - API Base: `https://ollama.com/v1` (auto-filled)
   - API Key: Enter your Ollama Cloud API key
   - Max Tokens: 8192 (recommended)
   - Alias: Auto-filled from model name
6. Click **"Save"**

### Using Ollama Cloud with Agents

1. **Open Settings** → **Agents**
2. Select or create an agent
3. **Model Dropdown** will show your configured Ollama Cloud models
4. Select the desired cloud model
5. Save agent configuration

## Configuration Storage

When saved via the web UI, the model is stored in `~/.sofia/config.json`:

```json
{
  "model_list": [
    {
      "model_name": "gemma4:31b-cloud",
      "model": "ollama/gemma4:31b-cloud",
      "api_base": "https://ollama.com/v1",
      "api_key": "your-ollama-cloud-api-key"
    }
  ],
  "agents": {
    "defaults": {
      "model_name": "gemma4:31b-cloud"
    }
  }
}
```

## JavaScript Implementation

### PROVIDER_MODELS Constant

```javascript
"Ollama Cloud": [
    { label: "Gemma 4 31B Cloud", model_id: "ollama/gemma4:31b-cloud", api_base: "https://ollama.com/v1" },
    { label: "Qwen 3 Coder 480B Cloud", model_id: "ollama/qwen3-coder:480b-cloud", api_base: "https://ollama.com/v1" },
    { label: "GPT-OSS 120B Cloud", model_id: "ollama/gpt-oss:120b-cloud", api_base: "https://ollama.com/v1" },
    { label: "GPT-OSS 20B Cloud", model_id: "ollama/gpt-oss:20b-cloud", api_base: "https://ollama.com/v1" },
    { label: "DeepSeek V3.1 671B Cloud", model_id: "ollama/deepseek-v3.1:671b-cloud", api_base: "https://ollama.com/v1" },
]
```

### onProviderChange() Logic

```javascript
} else if (provider === "Ollama Cloud") {
    // Show dropdown with cloud models, auto-set API base to cloud endpoint
    const models = PROVIDER_MODELS[provider] || [];
    sel.classList.remove("hidden");
    sel.innerHTML = "<option value=''>-- Select Model --</option>" +
        models.map(function (m) { return "<option value=\"" + m.model_id + "\" data-base=\"" + m.api_base + "\">" + m.label + "</option>"; }).join("") +
        "<option value='__custom__'>Other (enter manually)</option>";
    customWrapper.classList.add("hidden");
    document.getElementById("form-model-base").value = "https://ollama.com/v1";
    document.getElementById("form-model-key").value = "";
    document.getElementById("form-model-key").placeholder = "Enter your Ollama Cloud API key";
}
```

### Model Detection Logic

```javascript
// Detect Ollama Cloud models (those with -cloud suffix)
let detectedProvider = providerMap[prefix] || "Custom";
if (prefix === "ollama" && m.model && m.model.endsWith("-cloud")) {
    detectedProvider = "Ollama Cloud";
}
```

## Testing

### Build Verification

```bash
cd /Volumes/Slaven/sofia
go build ./pkg/web
# ✅ Build successful
```

### Test Results

```bash
go test ./pkg/web -count=1
# ok   github.com/grasberg/sofia/pkg/web    0.564s
```

### Manual Testing Steps

1. **Start Sofia:**
   ```bash
   sofia gateway
   ```

2. **Open Web UI:** Navigate to `http://localhost:8080` (or your configured port)

3. **Add Ollama Cloud Model:**
   - Go to Settings → Models
   - Click "Add Model"
   - Select "Ollama Cloud ☁️"
   - Choose "Gemma 4 31B Cloud"
   - Enter your API key
   - Click Save

4. **Verify Model Appears:**
   - Model should appear in "Configured Models" list
   - Model should appear in agent model dropdown

5. **Test with Agent:**
   - Create/edit an agent
   - Select the Ollama Cloud model
   - Save and test the agent

## Screenshots

### Provider Dropdown
```
Settings → Models → Select Provider
├── Google Gemini
├── OpenAI
├── Anthropic
├── DeepSeek
├── Groq
├── Mistral
├── OpenRouter
├── Qwen
├── Moonshot
├── xAI (Grok)
├── Z.ai
├── MiniMax
├── Ollama (Local)
├── Ollama Cloud ☁️  ← NEW!
└── Custom / Other
```

### Model Dropdown (Ollama Cloud Selected)
```
Select Model
├── Gemma 4 31B Cloud
├── Qwen 3 Coder 480B Cloud
├── GPT-OSS 120B Cloud
├── GPT-OSS 20B Cloud
├── DeepSeek V3.1 671B Cloud
└── Other (enter manually)
```

## Backwards Compatibility

✅ **Fully backwards compatible**
- Existing configured models continue to work
- Existing agent configurations unchanged
- No breaking changes to config format
- Local Ollama and Ollama Cloud coexist peacefully

## Security Considerations

- API keys are masked in the UI (shown as `••••••••`)
- API keys are only sent to the backend during save
- Backend stores API keys in config.json with appropriate permissions
- HTTPS is used for all Ollama Cloud API calls

## Error Handling

The UI handles common errors:

1. **Missing API Key:**
   - Warning shown when saving model without API key
   - User can still proceed (some endpoints may work without key)

2. **Invalid Model Name:**
   - Validation on custom model input
   - Must follow format: `ollama/model-name:variant`

3. **Connection Errors:**
   - Handled by agent loop (not web UI)
   - Errors shown in agent output/logs

## Future Enhancements

Potential improvements:

1. **Model Availability Check:**
   - Button to test API connectivity
   - Verify model is available before saving

2. **Usage Statistics:**
   - Show token usage per cloud model
   - Display estimated costs

3. **Model Discovery:**
   - Fetch available cloud models from API
   - Auto-populate dropdown with live list

## Documentation

- **Setup Guide:** `docs/OLLAMA_CLOUD_SETUP.md`
- **Quick Reference:** `docs/OLLAMA_CLOUD_QUICK_REFERENCE.md`
- **Implementation:** `docs/OLLAMA_CLOUD_IMPLEMENTATION.md`

---

**Implementation Date:** April 5, 2026  
**Status:** ✅ Complete and Tested
