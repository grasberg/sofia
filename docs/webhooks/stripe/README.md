# Stripe Webhooks

Comprehensive resources for implementing Stripe webhook integrations.

## Documentation

- [Best Practices & Requirements (2026)](./best-practices-2026.md) - Complete security, reliability, and implementation guide

## Key Events for Digital Products

### Payment & Subscription Events
| Event | Description | When to Use |
|-------|-------------|-------------|
| `checkout.session.completed` | Customer completes purchase | Grant product access, send welcome email |
| `customer.subscription.created` | New subscription started | Setup recurring access, log subscription |
| `customer.subscription.updated` | Subscription modified | Update access levels, notify user |
| `customer.subscription.deleted` | Subscription canceled | Revoke access, offer retention |
| `invoice.payment_succeeded` | Payment successfully processed | Log revenue, update accounting |
| `invoice.payment_failed` | Payment failed | Notify customer, retry logic |
| `payment_intent.succeeded` | Payment intent completed | Fulfill order, update inventory |

### Account & Connect Events
| Event | Description | When to Use |
|-------|-------------|-------------|
| `account.updated` | Connected account updated | Sync account information |
| `account.application.deauthorized` | App disconnected from account | Clean up user data |
| `payout.created` | Payout initiated to bank account | Update accounting, notify admin |

### Dispute & Fraud Events
| Event | Description | When to Use |
|-------|-------------|-------------|
| `charge.dispute.created` | Customer disputes charge | Fraud investigation, evidence collection |
| `review.opened` | Payment under review | Manual review process |
| `review.closed` | Review completed | Update risk assessment |

## Implementation Examples

### Express.js Webhook Handler
```javascript
const express = require('express');
const stripe = require('stripe')(process.env.STRIPE_SECRET_KEY);

const app = express();

// Use raw body for webhook verification
app.post('/webhook', express.raw({type: 'application/json'}), (req, res) => {
  const sig = req.headers['stripe-signature'];
  const endpointSecret = process.env.STRIPE_WEBHOOK_SECRET;

  let event;
  try {
    event = stripe.webhooks.constructEvent(req.body, sig, endpointSecret);
  } catch (err) {
    return res.status(400).send(`Webhook Error: ${err.message}`);
  }

  // Handle the event
  switch (event.type) {
    case 'checkout.session.completed':
      const session = event.data.object;
      await fulfillOrder(session);
      break;
    case 'invoice.payment_succeeded':
      const invoice = event.data.object;
      await updateSubscription(invoice.subscription);
      break;
    default:
      console.log(`Unhandled event type ${event.type}`);
  }

  res.json({received: true});
});

// Other routes use JSON parsing
app.use(express.json());

app.listen(3000, () => {
  console.log('Webhook server running on port 3000');
});
```

### Next.js API Route
```javascript
// pages/api/webhook.js
import { buffer } from 'micro';
import Stripe from 'stripe';

const stripe = new Stripe(process.env.STRIPE_SECRET_KEY);
const webhookSecret = process.env.STRIPE_WEBHOOK_SECRET;

export const config = {
  api: {
    bodyParser: false,
  },
};

export default async function handler(req, res) {
  if (req.method !== 'POST') {
    return res.status(405).end();
  }

  const buf = await buffer(req);
  const sig = req.headers['stripe-signature'];

  let event;
  try {
    event = stripe.webhooks.constructEvent(buf, sig, webhookSecret);
  } catch (err) {
    return res.status(400).send(`Webhook Error: ${err.message}`);
  }

  // Process event
  switch (event.type) {
    case 'checkout.session.completed':
      // Handle successful checkout
      break;
    // Add other event types
  }

  res.status(200).json({ received: true });
}
```

## Testing Locally

### Using Stripe CLI
```bash
# Install Stripe CLI
curl -s https://packages.stripe.dev/api/archives/$(uname)/$(uname -m)/stripe-stable/latest.tar.gz | tar -xz
sudo mv stripe /usr/local/bin/

# Login
stripe login

# Forward events to local server
stripe listen --forward-to localhost:3000/webhook

# In another terminal, trigger test events
stripe trigger checkout.session.completed
stripe trigger invoice.payment_succeeded
```

### Using ngrok
```bash
# Start local server
npm start

# In another terminal, expose local server
ngrok http 3000

# Copy ngrok URL and configure in Stripe Dashboard
# Webhooks → Add endpoint → https://your-subdomain.ngrok.io/webhook
```

## Configuration

### Dashboard Setup
1. Go to [Stripe Dashboard → Developers → Webhooks](https://dashboard.stripe.com/webhooks)
2. Click "Add endpoint"
3. Enter your endpoint URL
4. Select events to subscribe to
5. Copy the signing secret (never commit to git!)

### Environment Variables
```env
STRIPE_SECRET_KEY=sk_live_...
STRIPE_WEBHOOK_SECRET=whsec_...
STRIPE_PUBLISHABLE_KEY=pk_live_...
```

## Monitoring

### Dashboard Monitoring
- **Webhooks → Endpoints**: View delivery status
- **Events**: See all events with delivery status
- **Retries**: Monitor automatic retry attempts

### Health Checks
```javascript
// Periodic health check for webhook endpoint
async function checkWebhookHealth() {
  const response = await fetch('https://api.stripe.com/v1/webhook_endpoints/we_...', {
    headers: {
      'Authorization': `Bearer ${process.env.STRIPE_SECRET_KEY}`
    }
  });
  
  const endpoint = await response.json();
  console.log(`Status: ${endpoint.status}, Last delivery: ${endpoint.latest_delivery}`);
}
```

## Troubleshooting

### Common Issues

| Issue | Solution |
|-------|----------|
| Signature verification fails | Ensure you're using raw body, not parsed JSON |
| Webhooks not received | Check endpoint is HTTPS (production), firewall allows Stripe IPs |
| Duplicate events | Implement idempotency using event IDs |
| Timeout errors | Return 200 immediately, process asynchronously |
| SSL/TLS errors | Ensure TLS 1.2+ supported, valid SSL certificate |

### Debugging Steps
1. Check Stripe Dashboard for delivery failures
2. Verify signature secret matches endpoint
3. Test locally with Stripe CLI
4. Inspect raw request headers and body
5. Check server logs for errors

## Security Considerations

See [Best Practices & Requirements](./best-practices-2026.md) for detailed security guidance.

---

*Need help?* Check the [official Stripe webhook documentation](https://docs.stripe.com/webhooks) or join the [Stripe Discord community](https://stripe.com/discord).