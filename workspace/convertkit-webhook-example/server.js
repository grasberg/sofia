const express = require('express');
const crypto = require('crypto');
const dotenv = require('dotenv');
const fetch = require('node-fetch');

dotenv.config();

const app = express();
const PORT = process.env.PORT || 3002;
const WEBHOOK_SECRET = process.env.CONVERTKIT_WEBHOOK_SECRET;
const API_SECRET = process.env.CONVERTKIT_API_SECRET;

// Middleware to parse JSON and raw body for signature verification
app.use(express.json({
  verify: (req, res, buf) => {
    req.rawBody = buf.toString();
  }
}));

/**
 * Verify ConvertKit webhook signature
 * ConvertKit signs payload with SHA256 HMAC using your API secret
 * Signature is in X-ConvertKit-Signature header as hex string
 */
function verifyConvertKitSignature(payload, signature) {
  if (!WEBHOOK_SECRET) {
    console.warn('⚠️  No webhook secret configured, skipping verification');
    return true;
  }

  const expectedSignature = crypto
    .createHmac('sha256', WEBHOOK_SECRET)
    .update(payload)
    .digest('hex');

  return crypto.timingSafeEqual(
    Buffer.from(signature, 'hex'),
    Buffer.from(expectedSignature, 'hex')
  );
}

/**
 * Create affiliate lead conversion from ConvertKit subscriber
 * This function would integrate with your affiliate tracking system
 */
async function createAffiliateLeadConversion(subscriberData, eventType) {
  const affiliateId = subscriberData.fields?.affiliate_id || 
                      subscriberData.custom_fields?.affiliate_id;
  const linkId = subscriberData.fields?.link_id || 
                 subscriberData.custom_fields?.link_id;
  const clickId = subscriberData.fields?.click_id || 
                  subscriberData.custom_fields?.click_id;

  if (!affiliateId) {
    console.log('No affiliate ID found in subscriber data');
    return null;
  }

  console.log(`🎯 Creating affiliate lead conversion for affiliate: ${affiliateId}`);
  console.log(`   Event: ${eventType}, Email: ${subscriberData.email}`);

  // Example conversion object
  const conversion = {
    id: `lead_${Date.now()}_${Math.random().toString(36).substr(2, 9)}`,
    affiliate_id: affiliateId,
    link_id: linkId,
    click_id: clickId,
    subscriber_id: subscriberData.id,
    subscriber_email: subscriberData.email,
    first_name: subscriberData.first_name,
    fields: subscriberData.fields,
    event_type: eventType,
    amount: 0, // Lead conversion has no monetary value initially
    currency: 'USD',
    commission_rate: 0.05, // Example: 5% commission on future sales
    status: 'lead',
    created_at: new Date().toISOString(),
    metadata: subscriberData
  };

  // Optionally send to affiliate API
  const AFFILIATE_API_URL = process.env.AFFILIATE_API_URL;
  if (AFFILIATE_API_URL) {
    try {
      const response = await fetch(`${AFFILIATE_API_URL}/api/affiliate/conversion`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          ...conversion,
          type: 'lead',
          source: 'convertkit',
          event_type: eventType
        })
      });

      if (response.ok) {
        const result = await response.json();
        console.log(`✅ Affiliate lead conversion created via API:`, result.data?.id);
        return result.data;
      } else {
        console.error(`API error: ${response.status}`);
      }
    } catch (err) {
      console.error('Failed to create affiliate conversion via API:', err.message);
    }
  }

  // Store in-memory for demo
  console.log(`✅ Affiliate lead conversion created locally:`, conversion.id);
  return conversion;
}

/**
 * ConvertKit event handlers
 */
const eventHandlers = {
  // When a subscriber is added to a form
  'subscriber.form_subscribe': async (subscriber, eventData) => {
    console.log(`📝 New form subscriber: ${subscriber.email}`);
    
    // Check if this is an affiliate referral
    const conversion = await createAffiliateLeadConversion(subscriber, 'form_subscribe');
    
    // Business logic: send welcome email, tag subscriber, etc.
    
    return {
      handled: true,
      event: 'subscriber.form_subscribe',
      subscriber_id: subscriber.id,
      affiliate_conversion: conversion ? conversion.id : null
    };
  },

  // When a subscriber is added to a sequence
  'subscriber.sequence_subscribe': async (subscriber, eventData) => {
    console.log(`🔁 Sequence subscriber: ${subscriber.email}, Sequence: ${eventData.sequence?.name || 'N/A'}`);
    
    const conversion = await createAffiliateLeadConversion(subscriber, 'sequence_subscribe');
    
    return {
      handled: true,
      event: 'subscriber.sequence_subscribe',
      subscriber_id: subscriber.id,
      affiliate_conversion: conversion ? conversion.id : null
    };
  },

  // When a subscriber purchases a product
  'subscriber.purchase': async (subscriber, eventData) => {
    console.log(`💰 Purchase: ${subscriber.email}, Product: ${eventData.product?.name || 'N/A'}`);
    
    // This could be a direct affiliate sale if the purchase came from affiliate link
    // For now, treat as lead with purchase intent
    const conversion = await createAffiliateLeadConversion(subscriber, 'purchase');
    
    return {
      handled: true,
      event: 'subscriber.purchase',
      subscriber_id: subscriber.id,
      affiliate_conversion: conversion ? conversion.id : null
    };
  },

  // When a subscriber is tagged
  'subscriber.tag_add': async (subscriber, eventData) => {
    console.log(`🏷️  Tag added: ${subscriber.email}, Tag: ${eventData.tag?.name || 'N/A'}`);
    
    // Useful for segmenting affiliate referrals
    if (eventData.tag?.name?.includes('affiliate')) {
      console.log(`   Affiliate-related tag detected`);
    }
    
    return {
      handled: true,
      event: 'subscriber.tag_add',
      subscriber_id: subscriber.id
    };
  },

  // When a subscriber unsubscribes
  'subscriber.unsubscribe': async (subscriber, eventData) => {
    console.log(`👋 Unsubscribe: ${subscriber.email}`);
    
    // Mark affiliate lead as unsubscribed if tracking
    
    return {
      handled: true,
      event: 'subscriber.unsubscribe',
      subscriber_id: subscriber.id
    };
  }
};

/**
 * Main webhook endpoint
 */
app.post('/api/webhooks/convertkit', (req, res) => {
  const signature = req.headers['x-convertkit-signature'];
  const eventType = req.headers['x-convertkit-event'] || 'unknown';

  if (!signature) {
    console.warn('⚠️  No signature header received');
    return res.status(400).json({ error: 'Missing signature header' });
  }

  // Verify signature
  if (!verifyConvertKitSignature(req.rawBody, signature)) {
    console.error('❌ Invalid webhook signature');
    return res.status(401).json({ error: 'Invalid signature' });
  }

  console.log(`✅ Valid webhook received: ${eventType}`);

  const subscriber = req.body.subscriber;
  const eventData = {
    ...req.body,
    subscriber: undefined // Remove subscriber from event data to avoid duplication
  };

  // Handle the event
  const handler = eventHandlers[eventType];
  if (!handler) {
    console.log(`🤷 No handler for event type: ${eventType}`);
    return res.status(200).json({ received: true, handled: false });
  }

  // Process asynchronously
  handler(subscriber, eventData)
    .then(result => {
      console.log(`✅ Successfully processed ${eventType}`);
      res.status(200).json({ received: true, ...result });
    })
    .catch(error => {
      console.error(`❌ Error processing ${eventType}:`, error);
      res.status(500).json({ error: 'Internal server error' });
    });
});

/**
 * Health check endpoint
 */
app.get('/health', (req, res) => {
  res.status(200).json({ 
    status: 'ok', 
    service: 'convertkit-webhook',
    timestamp: new Date().toISOString() 
  });
});

/**
 * Affiliate conversions list (for demo purposes)
 */
app.get('/api/affiliate/conversions', (req, res) => {
  // In a real app, you'd fetch from database
  res.status(200).json({ 
    message: 'This endpoint would return affiliate conversions',
    note: 'Implement your own data storage' 
  });
});

/**
 * Start server
 */
app.listen(PORT, () => {
  console.log(`🚀 ConvertKit webhook server running on port ${PORT}`);
  console.log(`📝 Endpoint: http://localhost:${PORT}/api/webhooks/convertkit`);
  console.log(`🏥 Health check: http://localhost:${PORT}/health`);
});