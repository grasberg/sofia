// test-webhook.js
// Script to simulate ConvertKit webhook calls for testing

const crypto = require('crypto');
const fetch = require('node-fetch');

const WEBHOOK_SECRET = 'test_secret_change_in_production';
const WEBHOOK_URL = 'http://localhost:3002/api/webhooks/convertkit';

function generateSignature(payload) {
  return crypto
    .createHmac('sha256', WEBHOOK_SECRET)
    .update(payload)
    .digest('hex');
}

async function sendTestWebhook(eventType, subscriberData) {
  const payload = JSON.stringify({
    subscriber: subscriberData,
    event: eventType,
    sent_at: new Date().toISOString()
  });

  const signature = generateSignature(payload);

  try {
    const response = await fetch(WEBHOOK_URL, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        'X-ConvertKit-Signature': signature,
        'X-ConvertKit-Event': eventType
      },
      body: payload
    });

    const result = await response.json();
    console.log(`✅ ${eventType}:`, {
      status: response.status,
      data: result
    });
    return result;
  } catch (error) {
    console.error(`❌ ${eventType}:`, error.message);
  }
}

// Test data
const testSubscriber = {
  id: Math.floor(Math.random() * 1000000),
  email: `test${Date.now()}@example.com`,
  first_name: 'Test',
  fields: {
    affiliate_id: 'AFF123',
    link_id: 'link_456',
    click_id: 'click_789',
    custom_field: 'test value'
  }
};

// Run tests
async function runTests() {
  console.log('🚀 Testing ConvertKit webhook server...\n');

  // Test 1: Form subscription
  await sendTestWebhook('subscriber.form_subscribe', testSubscriber);

  // Test 2: Sequence subscription
  await sendTestWebhook('subscriber.sequence_subscribe', {
    ...testSubscriber,
    fields: { ...testSubscriber.fields, sequence_name: 'Welcome Sequence' }
  });

  // Test 3: Purchase
  await sendTestWebhook('subscriber.purchase', {
    ...testSubscriber,
    fields: { ...testSubscriber.fields, product_name: 'Premium Course' }
  });

  // Test 4: Tag add
  await sendTestWebhook('subscriber.tag_add', {
    ...testSubscriber,
    fields: { ...testSubscriber.fields, tag_name: 'affiliate_lead' }
  });

  // Test 5: Unsubscribe
  await sendTestWebhook('subscriber.unsubscribe', testSubscriber);

  // Test 6: Invalid signature (should fail)
  console.log('\n🧪 Testing invalid signature...');
  try {
    const response = await fetch(WEBHOOK_URL, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        'X-ConvertKit-Signature': 'invalid_signature',
        'X-ConvertKit-Event': 'subscriber.form_subscribe'
      },
      body: JSON.stringify({ subscriber: testSubscriber })
    });
    console.log(`Invalid signature test: ${response.status}`);
  } catch (error) {
    console.error('Invalid signature error:', error.message);
  }

  console.log('\n🎉 Tests completed!');
}

// Check if server is running first
fetch('http://localhost:3002/health')
  .then(res => res.json())
  .then(data => {
    console.log('Health check:', data);
    runTests();
  })
  .catch(err => {
    console.error('Server not running. Please start the server first:');
    console.error('cd workspace/convertkit-webhook-example && npm start');
    process.exit(1);
  });