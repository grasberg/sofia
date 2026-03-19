// Netlify serverless function for Stripe Checkout
// Save as: functions/create-checkout.js

const stripe = require('stripe')(process.env.STRIPE_SECRET_KEY);

exports.handler = async (event, context) => {
  // Only allow POST requests
  if (event.httpMethod !== 'POST') {
    return {
      statusCode: 405,
      body: JSON.stringify({ error: 'Method not allowed' }),
    };
  }

  try {
    const { priceId, successUrl, cancelUrl } = JSON.parse(event.body);

    // Create Stripe Checkout session
    const session = await stripe.checkout.sessions.create({
      payment_method_types: ['card'],
      line_items: [
        {
          price: priceId,
          quantity: 1,
        },
      ],
      mode: 'payment',
      success_url: successUrl,
      cancel_url: cancelUrl,
      // Optional: Add metadata for tracking
      metadata: {
        product_type: 'digital_pdf',
        source: 'landing_page',
      },
      // Enable automatic tax calculation if needed
      automatic_tax: { enabled: true },
    });

    return {
      statusCode: 200,
      headers: {
        'Content-Type': 'application/json',
      },
      body: JSON.stringify({
        id: session.id,
        url: session.url,
      }),
    };
  } catch (error) {
    console.error('Stripe error:', error);

    return {
      statusCode: 500,
      body: JSON.stringify({
        error: 'Failed to create checkout session',
        details: error.message,
      }),
    };
  }
};