# Netlify Webhooks

Netlify webhooks allow you to trigger builds, receive deployment notifications, and integrate with external services.

## Types of Netlify Webhooks

### 1. Build Hooks
Trigger a new build and deployment by sending a POST request to a unique URL.

**Use cases:**
- Trigger builds when content changes (CMS updates)
- Automated deployments from CI/CD pipelines
- Scheduled builds (via cron jobs)

### 2. Deploy Hooks
Receive notifications when deployments succeed or fail.

**Use cases:**
- Send deployment notifications to Slack/Discord
- Update external status pages
- Trigger post-deployment scripts

### 3. Form Submission Hooks
Process form submissions with serverless functions or external services.

### 4. Function Hooks
Trigger serverless functions via HTTP requests.

## Setup Instructions

### 1. Create Build Hook
1. Go to **Site settings > Build & deploy > Continuous deployment**
2. Scroll to **Build hooks**
3. Click **Add build hook**
4. Enter name and select branch
5. Copy the generated URL

### 2. Create Deploy Hook
1. Go to **Site settings > Build & deploy > Deploy notifications**
2. Click **Add notification**
3. Select **Outgoing webhook**
4. Enter URL and select events:
   - Deploy succeeded
   - Deploy failed
   - Deploy started

### 3. Environment Variables
```env
NETLIFY_BUILD_HOOK_URL=https://api.netlify.com/build_hooks/...
NETLIFY_SITE_ID=your-site-id
NETLIFY_ACCESS_TOKEN=your-access-token
```

## Implementation Examples

### Trigger Build via API
```javascript
// Node.js example
const axios = require('axios');

async function triggerNetlifyBuild() {
  try {
    const response = await axios.post(process.env.NETLIFY_BUILD_HOOK_URL);
    console.log('Build triggered:', response.data);
    return response.data;
  } catch (error) {
    console.error('Failed to trigger build:', error.message);
    throw error;
  }
}

// Example: Trigger build when database changes
app.post('/api/trigger-build', async (req, res) => {
  await triggerNetlifyBuild();
  res.json({ message: 'Build triggered successfully' });
});
```

### Handle Deployment Notifications
```javascript
// Next.js API route for deployment notifications
export default async function handler(req, res) {
  if (req.method !== 'POST') {
    return res.status(405).end();
  }

  const payload = req.body;
  const event = req.headers['x-netlify-event'];

  // Verify secret (optional)
  const secret = req.headers['x-netlify-signature'];
  if (secret !== process.env.NETLIFY_WEBHOOK_SECRET) {
    return res.status(401).json({ error: 'Invalid signature' });
  }

  switch (event) {
    case 'deploy_created':
      console.log('Deployment started:', payload.name);
      // Notify team
      await sendSlackNotification(`🚀 Deployment started: ${payload.name}`);
      break;
    
    case 'deploy_succeeded':
      console.log('Deployment succeeded:', payload.name);
      // Update status page
      await updateStatusPage('deployed', payload.url);
      // Notify team
      await sendSlackNotification(`✅ Deployment succeeded: ${payload.url}`);
      break;
    
    case 'deploy_failed':
      console.log('Deployment failed:', payload.name);
      // Alert developers
      await sendAlertEmail('Deployment failed', payload.error_message);
      break;
    
    default:
      console.log(`Unhandled event: ${event}`);
  }

  res.json({ received: true });
}
```

### GitHub Actions Integration
```yaml
# .github/workflows/trigger-netlify.yml
name: Trigger Netlify Build

on:
  push:
    branches: [ main ]
  schedule:
    - cron: '0 8 * * *'  # Daily at 8 AM

jobs:
  trigger-build:
    runs-on: ubuntu-latest
    steps:
      - name: Trigger Netlify Build
        run: |
          curl -X POST -d {} ${{ secrets.NETLIFY_BUILD_HOOK_URL }}
```

### Slack Integration for Deployment Notifications
```javascript
// Netlify function for Slack notifications
exports.handler = async (event) => {
  const payload = JSON.parse(event.body);
  const eventType = event.headers['x-netlify-event'];
  
  const slackWebhookUrl = process.env.SLACK_WEBHOOK_URL;
  
  let message = {};
  
  switch (eventType) {
    case 'deploy_succeeded':
      message = {
        text: `✅ Deployment succeeded: ${payload.url}`,
        attachments: [{
          fields: [
            { title: 'Site', value: payload.name, short: true },
            { title: 'URL', value: payload.url, short: true },
            { title: 'Deploy Time', value: new Date(payload.created_at).toLocaleString(), short: true }
          ]
        }]
      };
      break;
    
    case 'deploy_failed':
      message = {
        text: `❌ Deployment failed: ${payload.name}`,
        attachments: [{
          color: 'danger',
          fields: [
            { title: 'Error', value: payload.error_message || 'Unknown error', short: false }
          ]
        }]
      };
      break;
  }
  
  // Send to Slack
  await fetch(slackWebhookUrl, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(message)
  });
  
  return {
    statusCode: 200,
    body: JSON.stringify({ message: 'Notification sent' })
  };
};
```

## Testing Webhooks

### 1. Test Build Hook with curl
```bash
# Trigger build
curl -X POST -d {} https://api.netlify.com/build_hooks/YOUR_HOOK_ID

# With authentication (if required)
curl -X POST -H "Authorization: Bearer $NETLIFY_TOKEN" \
  -d {} https://api.netlify.com/build_hooks/YOUR_HOOK_ID
```

### 2. Test with ngrok (local development)
```bash
# Start local server
npm run dev

# Start ngrok tunnel
ngrok http 3000

# Update webhook URL in Netlify dashboard to use ngrok URL
```

### 3. Simulate Deployment Events
```bash
# Using Netlify CLI
netlify deploy --trigger

# Using API directly
curl -X POST https://api.netlify.com/api/v1/sites/YOUR_SITE_ID/deploys \
  -H "Authorization: Bearer $NETLIFY_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"title":"Manual deploy","body":"Triggered via API"}'
```

## Use Cases

### 1. CMS-Triggered Builds
When content updates in a headless CMS (Contentful, Sanity, Strapi), trigger a rebuild.

```javascript
// Contentful webhook handler
app.post('/api/contentful-webhook', async (req, res) => {
  const event = req.headers['x-contentful-topic'];
  
  if (event === 'ContentManagement.Entry.publish') {
    // Trigger Netlify build
    await triggerNetlifyBuild();
  }
  
  res.status(200).end();
});
```

### 2. Scheduled Builds
Use cron jobs to rebuild site regularly (e.g., for dynamic pricing, stock updates).

```bash
# cron entry (rebuild daily at 2 AM)
0 2 * * * curl -X POST https://api.netlify.com/build_hooks/YOUR_HOOK_ID
```

### 3. Multi-Site Coordination
Trigger builds across multiple sites when shared components update.

```javascript
async function triggerAllSites() {
  const sites = [
    process.env.NETLIFY_SITE_1_HOOK,
    process.env.NETLIFY_SITE_2_HOOK,
    process.env.NETLIFY_SITE_3_HOOK
  ];
  
  await Promise.all(sites.map(hook => 
    axios.post(hook).catch(err => 
      console.error(`Failed to trigger ${hook}:`, err.message)
    )
  ));
}
```

## Security Best Practices

### 1. Secret Verification
```javascript
// Verify webhook signature
function verifyNetlifySignature(req, secret) {
  const signature = req.headers['x-netlify-signature'];
  const payload = JSON.stringify(req.body);
  
  const hmac = crypto.createHmac('sha256', secret);
  const computedSignature = hmac.update(payload).digest('hex');
  
  return crypto.timingSafeEqual(
    Buffer.from(signature),
    Buffer.from(computedSignature)
  );
}
```

### 2. Rate Limiting
```javascript
const rateLimit = require('express-rate-limit');

const webhookLimiter = rateLimit({
  windowMs: 15 * 60 * 1000, // 15 minutes
  max: 10, // Limit each IP to 10 requests per windowMs
  message: 'Too many webhook requests from this IP'
});

app.post('/webhooks/netlify', webhookLimiter, webhookHandler);
```

### 3. IP Whitelisting
Netlify webhooks come from specific IP ranges. Consider verifying source IP.

## Troubleshooting

| Issue | Solution |
|-------|----------|
| Build not triggering | Check hook URL, verify POST request, check site settings |
| Webhook not received | Verify endpoint URL, check firewall, test with ngrok |
| Authentication failed | Verify access token, check permissions |
| Build hangs | Check build timeout, review build logs |
| Duplicate builds | Implement idempotency, check trigger sources |

## Resources

- [Netlify Build Hooks Documentation](https://docs.netlify.com/configure-builds/build-hooks/)
- [Netlify Deploy Notifications](https://docs.netlify.com/site-deploys/notifications/)
- [Netlify API Documentation](https://docs.netlify.com/api/get-started/)
- [Webhook Testing Tools](https://webhook.site)