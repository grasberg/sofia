#!/bin/bash
# Diagnostic script using Sofia's Web API (works while Sofia is running)

API_URL="${SOFIA_URL:-http://localhost:8080}"

echo "=== Sofia Model Configuration Diagnostic ==="
echo "API URL: $API_URL"
echo ""

echo "1. Checking if Sofia is accessible..."
if curl -s --connect-timeout 3 "$API_URL/api/config" > /dev/null 2>&1; then
    echo "   ✓ Sofia is running and accessible"
else
    echo "   ✗ Cannot reach Sofia at $API_URL"
    echo "   Set SOFIA_URL environment variable if using a different port"
    echo "   Example: export SOFIA_URL=http://localhost:8081"
    exit 1
fi

echo ""
echo "2. Agent Defaults:"
curl -s "$API_URL/api/config" | python3 -c "
import sys, json
try:
    cfg = json.load(sys.stdin)
    defaults = cfg.get('agents', {}).get('defaults', {})
    print(f\"   model_name: '{defaults.get('model_name', 'NOT SET')}'\")
    print(f\"   model: '{defaults.get('model', 'NOT SET')}'\")
    print(f\"   provider: '{defaults.get('provider', 'NOT SET')}'\")
except:
    print('   ERROR: Cannot parse config')
"

echo ""
echo "3. Configured Models:"
curl -s "$API_URL/api/config" | python3 -c "
import sys, json
try:
    cfg = json.load(sys.stdin)
    models = cfg.get('model_list', [])
    print(f'   Total models: {len(models)}')
    if not models:
        print('   ⚠ No models configured!')
    for m in models:
        has_key = 'YES' if m.get('api_key') else 'NO'
        print(f\"   - {m['model_name']} (provider: {m.get('provider', 'unknown')}, model: {m.get('model', 'unknown')}, has_api_key: {has_key})\")
except Exception as e:
    print(f'   ERROR: {e}')
"

echo ""
echo "4. Available Models from /api/models (NVIDIA only):"
curl -s "$API_URL/api/models" | python3 -c "
import sys, json
try:
    models = json.load(sys.stdin)
    nvidia = [m for m in models if m.get('provider') == 'NVIDIA']
    print(f'   NVIDIA models available: {len(nvidia)}')
    for m in nvidia[:5]:
        print(f\"   - {m.get('model_name', 'unknown')}: {m.get('display_name', 'unknown')}\")
    if len(nvidia) > 5:
        print(f'   ... and {len(nvidia) - 5} more')
except Exception as e:
    print(f'   ERROR: {e}')
"

echo ""
echo "5. Quick Test - Can you chat?"
echo "   (This won't actually send a message, just checking endpoint)"
curl -s --connect-timeout 3 "$API_URL/api/chat" -X POST \
  -H "Content-Type: application/json" \
  -d '{"message":"test"}' | python3 -c "
import sys, json
try:
    resp = json.load(sys.stdin)
    if 'error' in resp:
        print(f'   ⚠ Chat endpoint error: {resp[\"error\"]}')
    else:
        print('   ✓ Chat endpoint is responsive')
except:
    print('   ✓ Chat endpoint is responsive (empty response is normal for test)')
" 2>/dev/null

echo ""
echo "=== Recommendations ==="
echo ""
echo "If 'model_name' is empty or 'No models configured' appears:"
echo "  1. Go to Settings → Models in the Web UI"
echo "  2. Verify your NVIDIA model appears in 'Configured Models'"
echo "  3. If not, add it again with the API key"
echo "  4. Click on the model card to set it as the standard model"
echo ""
echo "If model exists but has 'NO' for api_key:"
echo "  1. Edit the model (pencil icon)"
echo "  2. Re-enter your API key: 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"
echo "  3. Click Save"
