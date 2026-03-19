#!/bin/bash
# simulate-affiliate-flow.sh
# Simulates the complete affiliate flow from registration to commission

set -e

API_BASE="http://localhost:3000"

echo "=== Affiliate Flow Simulation ==="
echo "Starting at $(date)"
echo

# Step 1: Register affiliate
echo "1. Registering affiliate..."
AFFILIATE_RESPONSE=$(curl -s -X POST "$API_BASE/api/affiliates/register" \
  -H "Content-Type: application/json" \
  -d '{"email":"simulated@example.com","name":"Simulated Affiliate","commissionRate":0.15}')

echo "$AFFILIATE_RESPONSE" | python3 -m json.tool

AFFILIATE_ID=$(echo "$AFFILIATE_RESPONSE" | python3 -c "import sys,json; print(json.load(sys.stdin)['affiliateId'])")
echo "Affiliate ID: $AFFILIATE_ID"
echo

# Step 2: Generate affiliate link
echo "2. Generating affiliate link..."
LINK_RESPONSE=$(curl -s -X POST "$API_BASE/api/affiliates/$AFFILIATE_ID/links" \
  -H "Content-Type: application/json" \
  -d '{"productId":"prod_stripe_scripts","productName":"Stripe Scripts Bundle"}')

echo "$LINK_RESPONSE" | python3 -m json.tool

LINK_ID=$(echo "$LINK_RESPONSE" | python3 -c "import sys,json; print(json.load(sys.stdin)['linkId'])")
TRACKING_URL=$(echo "$LINK_RESPONSE" | python3 -c "import sys,json; print(json.load(sys.stdin)['trackingUrl'])")
echo "Link ID: $LINK_ID"
echo "Tracking URL: $TRACKING_URL"
echo

# Step 3: Simulate user clicking the link
echo "3. Simulating user click on affiliate link..."
# First, get just the headers to capture cookies
CLICK_RESPONSE=$(curl -s -i "$API_BASE/api/track/click/$LINK_ID")
# Extract cookies from response headers
COOKIES=$(echo "$CLICK_RESPONSE" | grep -i "set-cookie" | sed 's/Set-Cookie: //g' | tr -d '\r' | head -2 | tr '\n' '; ')

echo "Cookies set: $COOKIES"
echo

# Parse click ID from cookies (simplified)
CLICK_ID=$(echo "$COOKIES" | grep -o "click_id=[^;]*" | cut -d= -f2)
echo "Click ID: $CLICK_ID"
echo

# Step 4: Simulate viewing product page
echo "4. Simulating product page view..."
PRODUCT_RESPONSE=$(curl -s "$API_BASE/api/simulate/product/prod_stripe_scripts?clickId=$CLICK_ID" \
  -H "Cookie: $COOKIES")

echo "$PRODUCT_RESPONSE" | python3 -m json.tool
echo

# Step 5: Simulate purchase
echo "5. Simulating purchase..."
PURCHASE_RESPONSE=$(curl -s -X POST "$API_BASE/api/simulate/purchase" \
  -H "Content-Type: application/json" \
  -H "Cookie: $COOKIES" \
  -d '{"productId":"prod_stripe_scripts","amount":99,"currency":"SEK"}')

echo "$PURCHASE_RESPONSE" | python3 -m json.tool

PURCHASE_ID=$(echo "$PURCHASE_RESPONSE" | python3 -c "import sys,json; print(json.load(sys.stdin)['purchaseId'])")
COMMISSION=$(echo "$PURCHASE_RESPONSE" | python3 -c "import sys,json; print(json.load(sys.stdin)['commission'])")
echo "Purchase ID: $PURCHASE_ID"
echo "Commission earned: $COMMISSION SEK"
echo

# Step 6: Check affiliate dashboard
echo "6. Checking affiliate dashboard..."
DASHBOARD_RESPONSE=$(curl -s "$API_BASE/api/affiliates/$AFFILIATE_ID/dashboard")

echo "$DASHBOARD_RESPONSE" | python3 -m json.tool
echo

# Step 7: Check system status
echo "7. System status..."
STATUS_RESPONSE=$(curl -s "$API_BASE/api/status")

echo "$STATUS_RESPONSE" | python3 -m json.tool
echo

echo "=== Simulation Complete ==="
echo "Affiliate flow successfully simulated!"
echo "- Affiliate registered: $AFFILIATE_ID"
echo "- Link generated: $LINK_ID"
echo "- Click tracked: $CLICK_ID"
echo "- Purchase completed: $PURCHASE_ID"
echo "- Commission earned: $COMMISSION SEK"
echo
echo "To run again, first reset the simulation:"
echo "  curl -X POST $API_BASE/api/reset"