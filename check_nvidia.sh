#!/bin/bash
# Diagnostic script to check NVIDIA model configuration

DB="/Volumes/Slaven/sofia/ruvector.db"

echo "=== Checking ruvector.db ==="
echo ""

echo "1. All NVIDIA models in database:"
sqlite3 "$DB" "SELECT model_name, provider, model, api_base, CASE WHEN api_key != '' THEN 'YES' ELSE 'NO' END as has_key, is_catalog FROM models WHERE provider = 'NVIDIA' ORDER BY model_name;" 2>/dev/null || echo "   ERROR: Cannot read database"

echo ""
echo "2. Configured models (with API key or non-catalog):"
sqlite3 "$DB" "SELECT model_name, provider, model, CASE WHEN api_key != '' THEN 'YES' ELSE 'NO' END as has_key FROM models WHERE api_key != '' OR is_catalog = 0 ORDER BY model_name;" 2>/dev/null || echo "   ERROR: Cannot read database"

echo ""
echo "3. Current agent defaults:"
sqlite3 "$DB" "SELECT key, value FROM config WHERE key LIKE 'agent%' OR key LIKE '%model%' LIMIT 20;" 2>/dev/null || echo "   ERROR: Cannot read config table"

echo ""
echo "4. Check if config.json exists and has agent defaults:"
if [ -f "/Volumes/Slaven/sofia/config.json" ]; then
    echo "   config.json exists"
    echo "   Agent defaults:"
    grep -A 5 '"defaults"' /Volumes/Slaven/sofia/config.json 2>/dev/null | sed 's/^/   /'
else
    echo "   config.json NOT FOUND"
fi
