# Affiliate Flow Simulation

A mock affiliate system that simulates the complete affiliate flow from registration to commission payout.

## Overview

This simulation demonstrates the entire affiliate marketing process:

1. **Affiliate Registration** - Affiliates sign up with email and name
2. **Link Generation** - Unique tracking links are created for affiliates
3. **Click Tracking** - Clicks on affiliate links are tracked with cookies
4. **Purchase Simulation** - Mock purchases with commission calculation
5. **Webhook Integration** - Simulated webhooks for purchase notifications
6. **Commission Tracking** - Dashboard to view performance and earnings

## Setup

```bash
cd workspace/projects/affiliate-simulation
npm install
npm start
```

The server will start on `http://localhost:3000`

## API Endpoints

### 1. Register Affiliate
```
POST /api/affiliates/register
{
  "email": "affiliate@example.com",
  "name": "John Doe",
  "commissionRate": 0.1
}
```

### 2. Generate Affiliate Link
```
POST /api/affiliates/:affiliateId/links
{
  "productId": "prod_stripe_scripts",
  "productName": "Stripe Scripts Bundle"
}
```

### 3. Track Click (Simulates user clicking affiliate link)
```
GET /api/track/click/:linkId
```
- Sets affiliate and click cookies (30-day duration)
- Redirects to product page

### 4. Simulate Product Page
```
GET /api/simulate/product/:productId
```
- Shows product details
- Checks for affiliate cookies

### 5. Simulate Purchase
```
POST /api/simulate/purchase
{
  "productId": "prod_stripe_scripts",
  "amount": 99,
  "currency": "SEK"
}
```
- Requires affiliate cookies (set by click tracking)
- Calculates commission based on affiliate's rate
- Triggers webhook event

### 6. Webhook Receiver
```
POST /api/webhooks/purchase
{
  "event": "purchase.completed",
  "purchaseId": "purchase_123",
  "affiliateId": "aff_abc123",
  "amount": 99,
  "commission": 9.9
}
```
- Simulates receiving webhook from payment processor

### 7. Affiliate Dashboard
```
GET /api/affiliates/:affiliateId/dashboard
```
- Shows affiliate stats, links, and recent purchases

### 8. System Status
```
GET /api/status
```
- Shows simulation metrics

### 9. Reset Simulation
```
POST /api/reset
```
- Clears all data (for testing)

## Simulating the Full Flow

### Step 1: Register as Affiliate
```bash
curl -X POST http://localhost:3000/api/affiliates/register \
  -H "Content-Type: application/json" \
  -d '{"email":"test@example.com","name":"Test Affiliate","commissionRate":0.15}'
```

### Step 2: Generate Affiliate Link
```bash
curl -X POST http://localhost:3000/api/affiliates/aff_123abc/links \
  -H "Content-Type: application/json" \
  -d '{"productId":"prod_stripe_scripts","productName":"Stripe Scripts Bundle"}'
```

### Step 3: Simulate User Clicking Link
```bash
curl -L "http://localhost:3000/api/track/click/link_456def"
```

### Step 4: Simulate Purchase (with cookies from step 3)
```bash
curl -X POST http://localhost:3000/api/simulate/purchase \
  -H "Content-Type: application/json" \
  -H "Cookie: affiliate_id=aff_123abc; click_id=click_789ghi" \
  -d '{"productId":"prod_stripe_scripts","amount":99,"currency":"SEK"}'
```

### Step 5: Check Affiliate Dashboard
```bash
curl "http://localhost:3000/api/affiliates/aff_123abc/dashboard"
```

## Data Storage

The simulation uses in-memory storage (Maps) for simplicity:
- `affiliates`: Registered affiliates
- `affiliateLinks`: Generated tracking links
- `clicks`: Tracked clicks
- `purchases`: Simulated purchases
- `webhooks`: Webhook event log

All data is lost when the server restarts. Use `/api/reset` to clear data during testing.

## Real-World Implementation Notes

This simulation simplifies several real-world complexities:

1. **Persistence**: Real systems would use a database
2. **Fraud Detection**: Click fraud, cookie stuffing detection
3. **Payment Processing**: Integration with Stripe, PayPal, etc.
4. **Tax Compliance**: VAT, sales tax, income tax reporting
5. **Multi-tier Affiliates**: Sub-affiliates, referral chains
6. **Advanced Analytics**: Conversion funnels, ROI analysis
7. **Payout Automation**: Bank transfers, PayPal payouts

## Next Steps

1. Add database persistence (SQLite, PostgreSQL)
2. Implement actual Gumroad API integration
3. Add webhook signature verification
4. Create admin dashboard
5. Add email notifications for affiliates
6. Implement payout scheduling
7. Add A/B testing for affiliate links