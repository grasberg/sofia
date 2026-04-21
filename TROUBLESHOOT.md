# Troubleshooting: "No model is configured" after adding NVIDIA model

## The Problem

You added a NVIDIA model (MiniMax M2.7) with your API key, but Sofia still says **"No model is configured"**.

## Root Cause Analysis

After examining the code, here's what happens:

### Flow when you add a model:

1. **Frontend (layout.html)**: You fill out the form and click "Save"
2. **JavaScript** saves the model to `configuredModels` array
3. **JavaScript** calls `saveConfig()` which sends POST to `/api/config`
4. **Backend** receives the config and:
   - Calls `memDB.SyncModels()` to save models to SQLite database
   - Calls `memDB.LoadModelsIntoConfig()` to reload models from DB
   - Calls `ReloadAgents()` to create a new provider

### The Critical Check (legacy_provider.go line 38-40):

```go
if model == "" || len(cfg.ModelList) == 0 {
    return nil, "", nil  // Returns NIL provider!
}
```

Where:
- `model` = `cfg.Agents.Defaults.GetModelName()` (the "standard model")
- `cfg.ModelList` = list of configured models (must have API key OR be non-catalog)

### Why it fails:

The model is saved to the database, but when `ReloadAgents()` is called, one of these is true:
1. `cfg.agents.defaults.model_name` is empty string
2. `cfg.ModelList` is empty (model filtered out because API key is empty)
3. The model name in `model_name` doesn't match any entry in `ModelList`

## Solution

### Option 1: Check via Web UI (Recommended)

1. **Open Sofia Web UI** → Settings → Models
2. **Check the "Configured Models" section** - do you see your NVIDIA model listed?
3. **If YES**: Click the edit (pencil) icon on your model
   - Verify the API key field shows dots (meaning it's saved)
   - If it's empty, re-enter your API key: `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`
   - Click Save
4. **Check the hidden standard model**:
   - Open browser DevTools (F12)
   - Go to Console
   - Type: `document.getElementById("cfg-model").value`
   - This should show your model alias (e.g., "MiniMax M2.7")
   - If it's empty, click on your model in the list to select it as standard

### Option 2: Check via API

Run this in your terminal:

```bash
curl -s http://localhost:8080/api/config | python3 -m json.tool | grep -A 20 '"agents"'
```

Look for:
```json
"agents": {
    "defaults": {
        "model_name": "your-model-alias",  // <-- THIS MUST NOT BE EMPTY
        ...
    }
}
```

Also check `model_list`:

```bash
curl -s http://localhost:8080/api/config | python3 -m json.tool | grep -A 50 '"model_list"'
```

You should see your NVIDIA model with `"api_key": "nvapi-..."`

### Option 3: Force re-select the model

1. In Settings → Models page
2. Find your NVIDIA model in the "Configured Models" list
3. **Click on the model card** (not the edit button) - this should mark it as the standard model
4. The model should now show as selected/highlighted
5. Wait a moment for auto-save, or click Save if there's a button

### Option 4: Delete and re-add

If all else fails:

1. Delete the existing NVIDIA model (click trash icon)
2. Click "Add Model"
3. Select NVIDIA provider
4. Select MiniMax M2.7 model
5. **IMPORTANT**: In the "Model Alias" field, enter: `nvidia-minimax-m2.7`
6. **IMPORTANT**: In the "API Key" field, paste your full API key
7. **IMPORTANT**: In the "API Base URL" field, enter: `https://integrate.api.nvidia.com/v1`
8. Click Save
9. Verify the model appears in the Configured Models list
10. Click on the model card to set it as standard

## If the Database is Corrupted

Since your database file shows "file is not a database" error:

1. **Stop Sofia** (Ctrl+C in terminal)
2. **Backup your database**:
   ```bash
   cp /Volumes/Slaven/sofia/ruvector.db /Volumes/Slaven/sofia/ruvector.db.backup
   ```
3. **Delete the corrupted database**:
   ```bash
   rm /Volumes/Slaven/sofia/ruvector.db
   ```
4. **Restart Sofia** - it will recreate the database and seed the catalog models
5. **Re-add your NVIDIA model** through the Web UI

## Verification After Fix

After making changes, verify it worked:

1. Check the logs for: `"Created provider from updated config"` with your model name
2. Try sending a message to Sofia - it should respond normally
3. Check via API:
   ```bash
   curl -s http://localhost:8080/api/config | python3 -c "
   import sys, json
   cfg = json.load(sys.stdin)
   print('Standard model:', cfg['agents']['defaults'].get('model_name', 'EMPTY'))
   print('Model list count:', len(cfg.get('model_list', [])))
   for m in cfg.get('model_list', []):
       print(f\"  - {m['model_name']} (provider: {m['provider']}, has_key: {'YES' if m.get('api_key') else 'NO'})\")
   "
   ```
