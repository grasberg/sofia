# Webhook Examples for Digital Products

This directory contains examples and templates for webhook integrations used in digital product deployments.

## What Are Webhooks?

Webhooks are automated messages sent from apps when something happens. They have a message—or payload—and are sent to a unique URL—essentially the app's phone number or address.

## Directory Structure

- `stripe/` – Stripe payment and subscription webhooks
- `netlify/` – Netlify build and deployment hooks  
- `vercel/` – Vercel deployment and function hooks
- `github/` – GitHub repository webhooks
- `gumroad/` – Gumroad sales and product webhooks
- `examples/` – Complete implementation examples

## Common Webhook Use Cases

### 1. Payment Processing (Stripe)
- `checkout.session.completed` – Customer completes purchase
- `customer.subscription.created` – New subscription started
- `invoice.payment_succeeded` – Payment successfully processed
- `invoice.payment_failed` – Payment failed, notify customer

### 2. Deployment Automation (Netlify/Vercel)
- Build hooks – Trigger deployments via API
- Deploy hooks – Notify when deployment completes
- Function hooks – Serverless function triggers

### 3. Sales Notifications (Gumroad)
- `sale` – New sale completed
- `refund` – Sale refunded
- `subscription_charged` – Recurring subscription payment

### 4. Development Workflow (GitHub)
- `push` – Code pushed to repository
- `pull_request` – Pull request created/merged
- `release` – New release published

## Security Best Practices

### 1. Verify Webhook Signatures
Always verify that webhooks come from the expected service using signature verification.

```javascript
// Example: Stripe signature verification
const stripe = require('stripe')(process.env.STRIPE_SECRET_KEY);
const endpointSecret = process.env.STRIPE_WEBHOOK_SECRET;

const sig = request.headers['stripe-signature'];
const event = stripe.webhooks.constructEvent(payload, sig, endpointSecret);
```

### 2. Use Environment Variables
Never hardcode webhook secrets in your code.

```env
STRIPE_WEBHOOK_SECRET=whsec_...
NETLIFY_BUILD_HOOK_URL=https://api.netlify.com/build_hooks/...
GITHUB_WEBHOOK_SECRET=your_github_secret
```

### 3. Implement Retry Logic
Webhook delivery may fail. Implement retry logic and dead letter queues.

### 4. Log Webhook Events
Log all incoming webhooks for debugging and auditing.

## Getting Started

1. **Choose a webhook receiver**
   - Serverless function (Vercel, Netlify Functions, AWS Lambda)
   - Express.js/Node.js server
   - Next.js API route

2. **Set up endpoint URL**
   - Deploy your webhook handler
   - Get the public URL
   - Configure in service dashboard

3. **Test with local tunneling**
   - Use ngrok or localtunnel for local development
   - Test with real webhook payloads
   - Verify signature verification works

## Service-Specific Guides

- [Stripe Webhooks](./stripe/README.md) - Includes [best practices guide](./stripe/best-practices-2026.md)
- [Netlify Build Hooks](./netlify/README.md)
- [Vercel Deployment Hooks](./vercel/README.md)
- [GitHub Webhooks](./github/README.md)
- [Gumroad Webhooks](./gumroad/README.md)

## Testing Tools

- **ngrok** – Secure tunnels to localhost
- **webhook.site** – Inspect webhook payloads
- **Postman** – Manual webhook testing
- **curl** – Command-line testing

## Troubleshooting

| Problem | Solution |
|---------|----------|
| Webhooks not received | Check endpoint URL, firewall, SSL |
| Signature verification fails | Verify secret matches, check timestamp |
| Payload malformed | Validate JSON structure, encoding |
| Duplicate events | Implement idempotency keys |

---

*Last updated: March 2026*