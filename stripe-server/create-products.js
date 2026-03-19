#!/usr/bin/env node
/**
 * Script to create Stripe products and prices for Niche Selection Toolkit tiers
 * Usage: node create-products.js
 * Requires .env file with STRIPE_SECRET_KEY
 */

import 'dotenv/config';
import Stripe from 'stripe';
import fs from 'fs';
import path from 'path';
import { fileURLToPath } from 'url';

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);

// Load product data
const productDataPath = path.join(__dirname, '..', 'workspace', 'products', 'niche_selection_toolkit_stripe.json');
const productData = JSON.parse(fs.readFileSync(productDataPath, 'utf8'));

// Initialize Stripe
const stripe = new Stripe(process.env.STRIPE_SECRET_KEY, {
  apiVersion: '2023-10-16',
  typescript: false,
});

async function createProducts() {
  console.log('Creating Stripe products and prices for Niche Selection Toolkit...\n');
  
  const results = [];
  
  for (const tier of productData.tiers) {
    console.log(`Creating product: ${tier.name}`);
    
    try {
      // Create product
      const product = await stripe.products.create({
        name: tier.name,
        description: tier.description,
        metadata: tier.metadata,
        shippable: false,
      });
      
      console.log(`  Product created: ${product.id}`);
      
      // Create price
      const price = await stripe.prices.create({
        product: product.id,
        unit_amount: tier.price_amount,
        currency: tier.price_currency,
        metadata: tier.metadata,
      });
      
      console.log(`  Price created: ${price.id} (${tier.price_amount/100} ${tier.price_currency})`);
      
      results.push({
        tier: tier.metadata.tier,
        productId: product.id,
        priceId: price.id,
        productName: product.name,
        priceAmount: price.unit_amount,
        priceCurrency: price.currency,
      });
      
    } catch (error) {
      console.error(`  Error creating product ${tier.name}:`, error.message);
      results.push({
        tier: tier.metadata.tier,
        error: error.message
      });
    }
    
    console.log('');
  }
  
  return results;
}

async function main() {
  if (!process.env.STRIPE_SECRET_KEY) {
    console.error('ERROR: STRIPE_SECRET_KEY is not set in .env file');
    console.error('Create a .env file in stripe-server/ with:');
    console.error('STRIPE_SECRET_KEY=sk_test_...');
    process.exit(1);
  }
  
  console.log('Stripe Product Creation Script');
  console.log('==============================\n');
  console.log(`Environment: ${process.env.NODE_ENV || 'development'}`);
  console.log(`Using key: ${process.env.STRIPE_SECRET_KEY.substring(0, 12)}...`);
  
  const results = await createProducts();
  
  // Save results
  const outputPath = path.join(__dirname, '..', 'workspace', 'products', 'niche_selection_toolkit_stripe_ids.json');
  fs.writeFileSync(outputPath, JSON.stringify(results, null, 2));
  
  console.log('\nResults saved to:', outputPath);
  console.log('\nSummary:');
  console.log('========');
  results.forEach(result => {
    if (result.error) {
      console.log(`❌ ${result.tier}: ERROR - ${result.error}`);
    } else {
      console.log(`✅ ${result.tier}: Product ${result.productId}, Price ${result.priceId}`);
    }
  });
  
  console.log('\nNext steps:');
  console.log('1. Verify products in Stripe Dashboard');
  console.log('2. Use price IDs in checkout code');
  console.log('3. Update affiliate links if needed');
}

main().catch(error => {
  console.error('Fatal error:', error);
  process.exit(1);
});