#!/bin/bash
# Quick fix script for NVIDIA model configuration

echo "=== Sofia NVIDIA Model Fix ==="
echo ""
echo "Step 1: Check if Sofia is running..."
if pgrep -f "sofia" > /dev/null 2>&1 && lsof -i :8080 2>/dev/null | grep -q LISTEN; then
    echo "✓ Sofia is running on port 8080"
    SOFIA_URL="http://localhost:8080"
elif lsof -i :8081 2>/dev/null | grep -q LISTEN; then
    echo "✓ Sofia is running on port 8081"
    SOFIA_URL="http://localhost:8081"
else
    echo "✗ Sofia is NOT running!"
    echo ""
    echo "Please start Sofia first:"
    echo "  cd /Volumes/Slaven/sofia"
    echo "  go run cmd/sofia/main.go"
    echo ""
    echo "Then run this script again."
    exit 1
fi

echo ""
echo "Step 2: Checking current configuration..."
MODEL_NAME=$(curl -s "$SOFIA_URL/api/config" | python3 -c "
import sys, json
cfg = json.load(sys.stdin)
print(cfg.get('agents', {}).get('defaults', {}).get('model_name', ''))
" 2>/dev/null)

MODEL_COUNT=$(curl -s "$SOFIA_URL/api/config" | python3 -c "
import sys, json
cfg = json.load(sys.stdin)
print(len(cfg.get('model_list', [])))
" 2>/dev/null)

echo "   Standard model: '${MODEL_NAME:-EMPTY}'"
echo "   Total models: ${MODEL_COUNT}"

echo ""
echo "Step 3: Listing configured models..."
curl -s "$SOFIA_URL/api/config" | python3 -c "
import sys, json
cfg = json.load(sys.stdin)
models = cfg.get('model_list', [])
if not models:
    print('   ⚠ NO MODELS FOUND!')
    print('   You need to add your NVIDIA model again.')
else:
    for m in models:
        has_key = '✓' if m.get('api_key') else '✗ NO KEY'
        is_nvidia = '🔥' if m.get('provider') == 'NVIDIA' else '  '
        print(f'   {is_nvidia} {m[\"model_name\"]} (provider: {m.get(\"provider\")}, has_key: {has_key})')
" 2>/dev/null

echo ""
echo "Step 4: Recommendations"
echo "────────────────────────────────────────"

if [ -z "$MODEL_NAME" ] || [ "$MODEL_NAME" = "" ]; then
    echo "⚠ ISSUE: No standard model selected!"
    echo ""
    echo "FIX:"
    echo "  1. Open Sofia Web UI → Settings → Models"
    echo "  2. Find your NVIDIA model in the list"
    echo "  3. Click the 'Standard' button on that model"
    echo "  4. OR click on the model card itself"
    echo "  5. Wait for auto-save"
    echo ""
elif [ "$MODEL_COUNT" = "0" ]; then
    echo "⚠ ISSUE: No models in model_list!"
    echo ""
    echo "FIX:"
    echo "  1. Open Sofia Web UI → Settings → Models"
    echo "  2. Click 'Add Model'"
    echo "  3. Select NVIDIA provider"
    echo "  4. Select MiniMax M2.7"
    echo "  5. Enter alias: nvidia-minimax-m2.7"
    echo "  6. Enter API Key: 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"
    echo "  7. Enter API Base: https://integrate.api.nvidia.com/v1"
    echo "  8. Click Save"
else
    # Check if the standard model exists in the list
    MATCH=$(curl -s "$SOFIA_URL/api/config" | python3 -c "
import sys, json
cfg = json.load(sys.stdin)
model_name = cfg.get('agents', {}).get('defaults', {}).get('model_name', '')
models = cfg.get('model_list', [])
found = any(m['model_name'] == model_name for m in models)
print('YES' if found else 'NO')
" 2>/dev/null)
    
    if [ "$MATCH" = "NO" ]; then
        echo "⚠ ISSUE: Standard model '${MODEL_NAME}' not found in model_list!"
        echo ""
        echo "FIX:"
        echo "  1. Open Sofia Web UI → Settings → Models"
        echo "  2. Delete the orphan standard model entry (if visible)"
        echo "  3. Click on one of your actual models to set it as standard"
        echo "  4. Restart Sofia"
    else
        echo "✓ Configuration looks good!"
        echo ""
        echo "If you're still getting 'No model is configured' error:"
        echo "  → Restart Sofia to reload the configuration"
        echo ""
        echo "  Press Ctrl+C to stop Sofia, then run:"
        echo "  cd /Volumes/Slaven/sofia"
        echo "  go run cmd/sofia/main.go"
    fi
fi

echo ""
echo "────────────────────────────────────────"
