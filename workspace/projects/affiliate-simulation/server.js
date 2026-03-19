const express = require('express');
const cors = require('cors');
const bodyParser = require('body-parser');
const cookieParser = require('cookie-parser');
const { v4: uuidv4 } = require('uuid');

const app = express();
const PORT = process.env.PORT || 3000;

// Middleware
app.use(cors());
app.use(bodyParser.json());
app.use(cookieParser());

// In-memory storage for simulation
const affiliates = new Map(); // affiliateId -> { email, name, commissionRate, createdAt }
const affiliateLinks = new Map(); // linkId -> { affiliateId, productId, clicks, conversions }
const clicks = new Map(); // clickId -> { affiliateId, linkId, ip, userAgent, timestamp, converted }
const purchases = new Map(); // purchaseId -> { productId, amount, affiliateId, clickId, timestamp, commission }
const webhooks = []; // log of webhook events

// Utility functions
const generateAffiliateId = () => `aff_${uuidv4().slice(0, 8)}`;
const generateLinkId = () => `link_${uuidv4().slice(0, 8)}`;
const generateClickId = () => `click_${uuidv4().slice(0, 8)}`;
const generatePurchaseId = () => `purchase_${uuidv4().slice(0, 8)}`;

// 1. Affiliate Registration Endpoint
app.post('/api/affiliates/register', (req, res) => {
  const { email, name, commissionRate = 0.1 } = req.body;
  
  if (!email || !name) {
    return res.status(400).json({ error: 'Email and name are required' });
  }
  
  // Check if affiliate already exists
  for (let [id, aff] of affiliates) {
    if (aff.email === email) {
      return res.status(200).json({
        message: 'Affiliate already registered',
        affiliateId: id,
        ...aff
      });
    }
  }
  
  const affiliateId = generateAffiliateId();
  const affiliate = {
    email,
    name,
    commissionRate: Math.min(Math.max(commissionRate, 0.01), 0.75), // 1% to 75%
    createdAt: new Date().toISOString()
  };
  
  affiliates.set(affiliateId, affiliate);
  
  console.log(`[Affiliate Registered] ID: ${affiliateId}, Email: ${email}, Commission: ${affiliate.commissionRate}`);
  
  res.status(201).json({
    message: 'Affiliate registered successfully',
    affiliateId,
    ...affiliate
  });
});

// 2. Generate Affiliate Link Endpoint
app.post('/api/affiliates/:affiliateId/links', (req, res) => {
  const { affiliateId } = req.params;
  const { productId = 'prod_stripe_scripts', productName = 'Stripe Scripts Bundle' } = req.body;
  
  if (!affiliates.has(affiliateId)) {
    return res.status(404).json({ error: 'Affiliate not found' });
  }
  
  const linkId = generateLinkId();
  const link = {
    affiliateId,
    productId,
    productName,
    createdAt: new Date().toISOString(),
    clicks: 0,
    conversions: 0,
    totalCommission: 0
  };
  
  affiliateLinks.set(linkId, link);
  
  // Generate tracking URL
  const trackingUrl = `https://example.com/products/${productId}?ref=${affiliateId}&link=${linkId}`;
  
  console.log(`[Link Generated] Link ID: ${linkId}, Affiliate: ${affiliateId}, Product: ${productId}`);
  
  res.status(201).json({
    message: 'Affiliate link generated',
    linkId,
    trackingUrl,
    ...link
  });
});

// 3. Click Tracking Endpoint (simulated when user clicks affiliate link)
app.get('/api/track/click/:linkId', (req, res) => {
  const { linkId } = req.params;
  const { ip = req.ip, userAgent = req.get('User-Agent') } = req.query;
  
  if (!affiliateLinks.has(linkId)) {
    return res.status(404).json({ error: 'Link not found' });
  }
  
  const link = affiliateLinks.get(linkId);
  const clickId = generateClickId();
  
  const click = {
    affiliateId: link.affiliateId,
    linkId,
    ip,
    userAgent,
    timestamp: new Date().toISOString(),
    converted: false
  };
  
  clicks.set(clickId, click);
  
  // Update link click count
  link.clicks += 1;
  affiliateLinks.set(linkId, link);
  
  // Set affiliate cookie (simulated)
  const affiliateId = link.affiliateId;
  const cookieOptions = {
    maxAge: 30 * 24 * 60 * 60 * 1000, // 30 days
    httpOnly: true
  };
  
  res.cookie('affiliate_id', affiliateId, cookieOptions);
  res.cookie('click_id', clickId, cookieOptions);
  
  console.log(`[Click Tracked] Click ID: ${clickId}, Affiliate: ${affiliateId}, Link: ${linkId}`);
  
  // Redirect to product page (simulated)
  res.redirect(`/api/simulate/product/${link.productId}?clickId=${clickId}`);
});

// 4. Product Page Simulation (mock product page)
app.get('/api/simulate/product/:productId', (req, res) => {
  const { productId } = req.params;
  const { clickId } = req.query;
  const affiliateId = req.cookies.affiliate_id;
  
  console.log(`[Product Page] Product: ${productId}, Click ID: ${clickId}, Affiliate: ${affiliateId}`);
  
  res.json({
    message: 'Product page simulation',
    productId,
    productName: 'Stripe Scripts Bundle',
    price: 99,
    currency: 'SEK',
    description: 'A bundle of useful Stripe automation scripts',
    affiliateId,
    clickId,
    hasAffiliateCookie: !!affiliateId
  });
});

// 5. Purchase Simulation Endpoint
app.post('/api/simulate/purchase', (req, res) => {
  const { productId = 'prod_stripe_scripts', amount = 99, currency = 'SEK' } = req.body;
  const affiliateId = req.cookies.affiliate_id;
  const clickId = req.cookies.click_id;
  
  const purchaseId = generatePurchaseId();
  
  // Calculate commission
  let commission = 0;
  let commissionRate = 0;
  
  if (affiliateId && affiliates.has(affiliateId)) {
    const affiliate = affiliates.get(affiliateId);
    commissionRate = affiliate.commissionRate;
    commission = amount * commissionRate;
    
    // Update affiliate link conversions
    if (clickId && clicks.has(clickId)) {
      const click = clicks.get(clickId);
      click.converted = true;
      clicks.set(clickId, click);
      
      // Find the link for this click
      for (let [linkId, link] of affiliateLinks) {
        if (link.affiliateId === affiliateId) {
          link.conversions += 1;
          link.totalCommission += commission;
          affiliateLinks.set(linkId, link);
          break;
        }
      }
    }
  }
  
  const purchase = {
    purchaseId,
    productId,
    amount,
    currency,
    affiliateId,
    clickId,
    commissionRate,
    commission,
    timestamp: new Date().toISOString()
  };
  
  purchases.set(purchaseId, purchase);
  
  console.log(`[Purchase Simulated] Purchase ID: ${purchaseId}, Amount: ${amount}, Commission: ${commission}, Affiliate: ${affiliateId || 'none'}`);
  
  // Simulate webhook to affiliate system
  if (affiliateId) {
    const webhookEvent = {
      event: 'purchase.completed',
      purchaseId,
      affiliateId,
      amount,
      commission,
      timestamp: new Date().toISOString()
    };
    
    webhooks.push(webhookEvent);
    console.log(`[Webhook Sent] ${JSON.stringify(webhookEvent)}`);
  }
  
  res.status(201).json({
    message: 'Purchase simulated successfully',
    ...purchase
  });
});

// 6. Webhook Simulation Endpoint (to receive webhooks from payment system)
app.post('/api/webhooks/purchase', (req, res) => {
  const event = req.body;
  
  console.log(`[Webhook Received] ${JSON.stringify(event)}`);
  
  // Simulate processing webhook
  webhooks.push({
    ...event,
    receivedAt: new Date().toISOString(),
    processed: true
  });
  
  res.status(200).json({ message: 'Webhook received' });
});

// 7. Dashboard Endpoints
app.get('/api/affiliates/:affiliateId/dashboard', (req, res) => {
  const { affiliateId } = req.params;
  
  if (!affiliates.has(affiliateId)) {
    return res.status(404).json({ error: 'Affiliate not found' });
  }
  
  const affiliate = affiliates.get(affiliateId);
  const affiliateLinksArray = Array.from(affiliateLinks.entries())
    .filter(([_, link]) => link.affiliateId === affiliateId);
  
  const clicksArray = Array.from(clicks.entries())
    .filter(([_, click]) => click.affiliateId === affiliateId);
  
  const purchasesArray = Array.from(purchases.entries())
    .filter(([_, purchase]) => purchase.affiliateId === affiliateId);
  
  const totalClicks = clicksArray.length;
  const totalConversions = purchasesArray.length;
  const totalCommission = purchasesArray.reduce((sum, [_, purchase]) => sum + purchase.commission, 0);
  
  res.json({
    affiliate,
    stats: {
      totalClicks,
      totalConversions,
      conversionRate: totalClicks > 0 ? (totalConversions / totalClicks * 100).toFixed(2) + '%' : '0%',
      totalCommission: totalCommission.toFixed(2)
    },
    links: affiliateLinksArray.map(([linkId, link]) => ({ linkId, ...link })),
    recentPurchases: purchasesArray.slice(-5).map(([purchaseId, purchase]) => ({ purchaseId, ...purchase }))
  });
});

// 8. System Status Endpoint
app.get('/api/status', (req, res) => {
  res.json({
    status: 'running',
    affiliates: affiliates.size,
    links: affiliateLinks.size,
    clicks: clicks.size,
    purchases: purchases.size,
    webhooks: webhooks.length,
    timestamp: new Date().toISOString()
  });
});

// 9. Reset Simulation Endpoint (for testing)
app.post('/api/reset', (req, res) => {
  affiliates.clear();
  affiliateLinks.clear();
  clicks.clear();
  purchases.clear();
  webhooks.length = 0;
  
  console.log('[Simulation Reset] All data cleared');
  
  res.json({ message: 'Simulation reset successfully' });
});

// Start server
app.listen(PORT, () => {
  console.log(`Affiliate Simulation Server running on port ${PORT}`);
  console.log(`Endpoints:`);
  console.log(`  POST /api/affiliates/register - Register new affiliate`);
  console.log(`  POST /api/affiliates/:id/links - Generate affiliate link`);
  console.log(`  GET  /api/track/click/:linkId - Track click (redirects to product)`);
  console.log(`  GET  /api/simulate/product/:id - Mock product page`);
  console.log(`  POST /api/simulate/purchase - Simulate purchase`);
  console.log(`  POST /api/webhooks/purchase - Receive webhook`);
  console.log(`  GET  /api/affiliates/:id/dashboard - Affiliate dashboard`);
  console.log(`  GET  /api/status - System status`);
  console.log(`  POST /api/reset - Reset simulation`);
});