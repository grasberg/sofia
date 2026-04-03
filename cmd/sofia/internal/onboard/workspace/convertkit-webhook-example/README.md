# ConvertKit/CRM Webhook Example

A sample webhook server for integrating ConvertKit (or any CRM) with affiliate tracking systems. This server receives webhooks from ConvertKit, verifies signatures, and creates affiliate lead conversions.

## Features

- ✅ Signature verification for secure webhooks
- ✅ Event handlers for common ConvertKit events:
  - Form subscriptions
  - Sequence subscriptions
  - Purchases
  - Tag additions
  - Unsubscribes
- ✅ Affiliate lead conversion tracking
- ✅ Integration with affiliate API (optional)
- ✅ Health check endpoint
- ✅ In-memory storage for demo purposes

## Use Cases

1. **Affiliate Lead Tracking**: When a new subscriber joins via an affiliate link, track the lead and attribute it to the affiliate.
2. **Lead Scoring**: Assign value to leads based on subscriber actions (form fills, purchases, etc.).
3. **CRM Integration**: Connect ConvertKit with your affiliate marketing platform.
4. **Automated Commission Tracking**: Trigger commission calculations when subscribers make purchases.

## Quick Start

1. Clone or copy this folder to your project.

2. Install dependencies:
   ```bash
   npm install
   ```

3. Copy `.env.example` to `.env`:
   ```bash
   cp .env.example .env
   ```

4. Configure your environment variables in `.env`:
   ```
   PORT=3002
   CONVERTKIT_WEBHOOK_SECRET=your_webhook_signing_secret_here
   CONVERTKIT_API_SECRET=your_api_secret_here
   AFFILIATE_API_URL=http://localhost:3001  # Optional
   ```

5. Start the server:
   ```bash
   npm start
   ```

## Configuration in ConvertKit

1. Go to your ConvertKit account → Settings → Webhooks.
2. Add a new webhook with your server's URL:
   ```
   https://your-domain.com/api/webhooks/convertkit
   ```
3. Select the events you want to receive:
   - Subscriber: Form subscribe
   - Subscriber: Sequence subscribe  
   - Subscriber: Purchase
   - Subscriber: Tag add
   - Subscriber: Unsubscribe
4. ConvertKit will provide a signing secret. Add this to your `.env` as `CONVERTKIT_WEBHOOK_SECRET`.

## Affiliate Integration

This example looks for affiliate tracking data in subscriber custom fields:

- `affiliate_id`: The ID of the affiliate who referred the subscriber
- `link_id`: The specific link used (optional)
- `click_id`: The click identifier (optional)

### How to pass affiliate data to ConvertKit

1. **Via URL parameters** (recommended):
   ```
   https://your-form.convertkit.com?affiliate_id=ABC123&link_id=link_456
   ```

2. **Via hidden form fields**:
   ```html
   <input type="hidden" name="fields[affiliate_id]" value="ABC123">
   <input type="hidden" name="fields[link_id]" value="link_456">
   ```

3. **Via API when creating subscribers**:
   ```javascript
   const response = await fetch('https://api.convertkit.com/v3/forms/{form_id}/subscribe', {
     method: 'POST',
     body: JSON.stringify({
       email: 'subscriber@example.com',
       fields: {
         affiliate_id: 'ABC123',
         link_id: 'link_456'
       }
     })
   });
   ```

## API Endpoints

- `POST /api/webhooks/convertkit` – Main webhook endpoint
- `GET /health` – Health check
- `GET /api/affiliate/conversions` – Demo endpoint for conversions

## Event Handling

The server processes these ConvertKit events:

| Event | Description | Affiliate Action |
|-------|-------------|------------------|
| `subscriber.form_subscribe` | New form subscriber | Create lead conversion |
| `subscriber.sequence_subscribe` | Added to sequence | Create lead conversion |
| `subscriber.purchase` | Purchased a product | Create sale conversion |
| `subscriber.tag_add` | Tagged subscriber | Segment affiliate leads |
| `subscriber.unsubscribe` | Unsubscribed | Mark lead as inactive |

## Extending for Other CRMs

This pattern works for any CRM that supports webhooks:

1. **ActiveCampaign**: Use `X-ActiveCampaign-Webhook-Signature`
2. **Mailchimp**: Use `X-Mailchimp-Webhook-Signature`
3. **HubSpot**: Use `X-HubSpot-Signature`
4. **Custom CRM**: Implement your own signature verification

Example for ActiveCampaign:
```javascript
function verifyActiveCampaignSignature(payload, signature) {
  const expected = crypto
    .createHmac('sha256', WEBHOOK_SECRET)
    .update(payload)
    .digest('hex');
  return signature === expected;
}
```

## Deployment

### Local Development
```bash
npm run dev  # With nodemon for auto-reload
```

### Production
1. Use environment variables for secrets
2. Add HTTPS (webhooks require HTTPS endpoints)
3. Use a process manager like PM2:
   ```bash
   pm2 start server.js --name convertkit-webhook
   ```

### Docker
```dockerfile
FROM node:18-alpine
WORKDIR /app
COPY package*.json ./
RUN npm ci --only=production
COPY . .
EXPOSE 3002
CMD ["node", "server.js"]
```

## Testing Webhooks Locally

1. Use **ngrok** to expose your local server:
   ```bash
   ngrok http 3002
   ```

2. Update your ConvertKit webhook URL to the ngrok URL.

3. Test using `curl`:
   ```bash
   curl -X POST https://your-ngrok.ngrok.io/api/webhooks/convertkit \
     -H "Content-Type: application/json" \
     -H "X-ConvertKit-Signature: test-signature" \
     -d '{
       "subscriber": {
         "id": 123456,
         "email": "test@example.com",
         "first_name": "Test",
         "fields": {
           "affiliate_id": "AFF123"
         }
       }
     }'
   ```

## Integration with Affiliate System

To connect with your affiliate tracking system:

1. Update `createAffiliateLeadConversion()` function to call your affiliate API.
2. Store conversions in your database instead of memory.
3. Add endpoints for reporting and reconciliation.

Example integration with the existing Stripe affiliate system:
```javascript
// In server.js, modify the createAffiliateLeadConversion function:
const response = await fetch(`${process.env.AFFILIATE_API_URL}/api/affiliate/lead`, {
  method: 'POST',
  headers: { 'Content-Type': 'application/json' },
  body: JSON.stringify({
    affiliate_id: affiliateId,
    email: subscriberData.email,
    source: 'convertkit',
    metadata: subscriberData
  })
});
```

## Security Notes

- Always verify webhook signatures in production.
- Never expose API secrets in client-side code.
- Use HTTPS for all webhook endpoints.
- Rate limit incoming webhooks if needed.
- Log events for audit trails.

## License

MIT