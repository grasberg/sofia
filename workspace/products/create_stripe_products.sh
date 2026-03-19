#!/bin/bash
# Script to create Stripe products and prices for Niche Selection Toolkit tiers using Stripe CLI

set -e

echo "Creating Stripe products and prices for Niche Selection Toolkit tiers..."

# Tier 1: Starter
echo "Creating Starter tier..."
PRODUCT_STARTER=$(stripe products create \
  --name="Niche Selection Toolkit - Starter" \
  --description="A comprehensive toolkit to help entrepreneurs identify profitable niches. Includes: Niche Selection Checklist, Market Validation Worksheet, 5 Niche Idea Templates, Basic Competitor Analysis Guide, Email support." \
  --metadata="tier=starter" \
  --metadata="features=niche-checklist,market-worksheet,5-templates,competitor-analysis,email-support" \
  --metadata="delivery=digital" \
  --shippable=false \
  -q id)

PRICE_STARTER=$(stripe prices create \
  --product="$PRODUCT_STARTER" \
  --unit-amount=4700 \
  --currency="usd" \
  --metadata="tier=starter" \
  -q id)

echo "  Starter product: $PRODUCT_STARTER"
echo "  Starter price: $PRICE_STARTER"

# Tier 2: Professional
echo "Creating Professional tier..."
PRODUCT_PRO=$(stripe products create \
  --name="Niche Selection Toolkit - Professional" \
  --description="Advanced toolkit for serious entrepreneurs. Includes all Starter features plus: Advanced Niche Scoring Calculator, Video Tutorial, 10 Additional Niche Templates, SWOT Analysis Framework, Community Access, Priority email support." \
  --metadata="tier=professional" \
  --metadata="features=all-starter,scoring-calculator,video-tutorial,10-templates,swot-framework,community-access,priority-support" \
  --metadata="delivery=digital" \
  --shippable=false \
  -q id)

PRICE_PRO=$(stripe prices create \
  --product="$PRODUCT_PRO" \
  --unit-amount=9700 \
  --currency="usd" \
  --metadata="tier=professional" \
  -q id)

echo "  Professional product: $PRODUCT_PRO"
echo "  Professional price: $PRICE_PRO"

# Tier 3: Agency
echo "Creating Agency tier..."
PRODUCT_AGENCY=$(stripe products create \
  --name="Niche Selection Toolkit - Agency" \
  --description="Complete solution for agencies and consultants. Includes all Professional features plus: Whitelabel Rights, Custom Niche Research Template, 1-hour Strategy Consultation Call, Masterclass, Lifetime Updates, Direct Slack support." \
  --metadata="tier=agency" \
  --metadata="features=all-professional,whitelabel-rights,custom-template,consultation-call,masterclass,lifetime-updates,slack-support" \
  --metadata="delivery=digital" \
  --shippable=false \
  -q id)

PRICE_AGENCY=$(stripe prices create \
  --product="$PRODUCT_AGENCY" \
  --unit-amount=29700 \
  --currency="usd" \
  --metadata="tier=agency" \
  -q id)

echo "  Agency product: $PRODUCT_AGENCY"
echo "  Agency price: $PRICE_AGENCY"

# Create output JSON
cat > "./workspace/products/niche_selection_toolkit_stripe_cli_ids.json" << EOF
{
  "starter": {
    "productId": "$PRODUCT_STARTER",
    "priceId": "$PRICE_STARTER"
  },
  "professional": {
    "productId": "$PRODUCT_PRO",
    "priceId": "$PRICE_PRO"
  },
  "agency": {
    "productId": "$PRODUCT_AGENCY",
    "priceId": "$PRICE_AGENCY"
  }
}
EOF

echo ""
echo "Products and prices created successfully!"
echo "Results saved to: ./workspace/products/niche_selection_toolkit_stripe_cli_ids.json"
echo ""
echo "Next steps:"
echo "1. Verify products in Stripe Dashboard: https://dashboard.stripe.com/test/products"
echo "2. Use price IDs in checkout code"
echo "3. Update affiliate links if needed"