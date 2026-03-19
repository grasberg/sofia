#!/usr/bin/env node

const fs = require('fs').promises;
const path = require('path');
const { minify } = require('html-minifier');

// Configuration
const DEFAULT_TEMPLATE_FILE = 'templates/digital-product/landing-template.html';
const DEFAULT_OUTPUT_FILE = 'build/landing.html';
const DEFAULT_PRODUCT_FILE = 'product.json';

// HTML minification options
const MINIFY_OPTIONS = {
    collapseWhitespace: true,
    removeComments: true,
    removeRedundantAttributes: true,
    removeScriptTypeAttributes: true,
    removeStyleLinkTypeAttributes: true,
    useShortDoctype: true,
    minifyCSS: true,
    minifyJS: true
};

// Helper function to render template with data
function renderTemplate(template, data) {
    let rendered = template;
    
    // Replace simple placeholders {{KEY}}
    Object.keys(data).forEach(key => {
        const regex = new RegExp(`{{${key}}}`, 'g');
        rendered = rendered.replace(regex, data[key] || '');
    });
    
    // Handle array sections {{#ARRAY}}...{{/ARRAY}}
    const arrayRegex = /\{\{#(\w+)\}\}(.*?)\{\{\/\1\}\}/gs;
    let match;
    while ((match = arrayRegex.exec(rendered)) !== null) {
        const arrayKey = match[1];
        const templateBlock = match[2];
        if (data[arrayKey] && Array.isArray(data[arrayKey])) {
            const replacements = data[arrayKey].map(item => {
                let block = templateBlock;
                Object.keys(item).forEach(itemKey => {
                    const itemRegex = new RegExp(`{{${itemKey}}}`, 'g');
                    block = block.replace(itemRegex, item[itemKey] || '');
                });
                return block;
            }).join('');
            rendered = rendered.replace(match[0], replacements);
        } else {
            // Remove section if array doesn't exist or is empty
            rendered = rendered.replace(match[0], '');
        }
    }
    
    // Clean up any remaining placeholders
    rendered = rendered.replace(/\{\{.*?\}\}/g, '');
    
    return rendered;
}

// Read product configuration
async function readProductConfig(productDir) {
    const configPath = path.join(productDir, DEFAULT_PRODUCT_FILE);
    try {
        const data = await fs.readFile(configPath, 'utf8');
        return JSON.parse(data);
    } catch (err) {
        console.warn(`No product.json found in ${productDir}, using empty config`);
        return {};
    }
}

// Read template file
async function readTemplate(templatePath) {
    try {
        return await fs.readFile(templatePath, 'utf8');
    } catch (err) {
        console.error(`Error reading template ${templatePath}:`, err.message);
        throw err;
    }
}

// Prepare template data from product config
function prepareTemplateData(config, productDir) {
    // Default data structure matching template placeholders
    const data = {
        // Basic product info
        PRODUCT_NAME: config.title || config.name || 'Product Name',
        PRODUCT_TAGLINE: config.tagline || config.subtitle || 'Amazing Digital Product',
        PRODUCT_DESCRIPTION: config.description || 'A description of your digital product',
        
        // Hero section
        BADGE_TEXT: config.badge || 'NEW RELEASE',
        HEADLINE: config.headline || config.title || 'Product Name',
        SUBHEADLINE: config.subtitle || 'Transform your workflow with this amazing product',
        CTA_PRIMARY: config.cta_primary || 'Get Instant Access',
        CTA_SECONDARY: config.cta_secondary || 'Learn More',
        
        // Social proof stats
        STATS: config.stats || [
            { number: '500+', label: 'Customers' },
            { number: '4.9', label: 'Rating' },
            { number: '24h', label: 'Support' }
        ],
        
        // Features
        FEATURES_TITLE: config.features_title || 'What You Get',
        FEATURES: config.features ? config.features.map((text, index) => ({
            icon: config.feature_icons ? config.feature_icons[index] || '⭐' : '⭐',
            title: `Feature ${index + 1}`,
            description: text
        })) : [
            { icon: '🚀', title: 'Feature 1', description: 'Description of feature 1' },
            { icon: '💡', title: 'Feature 2', description: 'Description of feature 2' },
            { icon: '🔧', title: 'Feature 3', description: 'Description of feature 3' }
        ],
        
        // Categories
        CATEGORIES_TITLE: config.categories_title || 'What\'s Included',
        CATEGORIES: config.categories || [
            { name: 'Category 1', icon: '📝', count: '20+ items' },
            { name: 'Category 2', icon: '💰', count: '20+ items' }
        ],
        
        // Pricing
        PRICING_TITLE: config.pricing_title || 'Choose Your Plan',
        PRICE_BASIC: config.price_basic || 47,
        PRICE_PRO: config.price_pro || 97,
        CURRENCY: config.currency || 'USD',
        FEATURES_BASIC: config.features_basic || ['Feature 1', 'Feature 2', 'Feature 3'],
        FEATURES_PRO: config.features_pro || ['Feature 1', 'Feature 2', 'Feature 3', 'Feature 4', 'Feature 5'],
        
        // Testimonials
        TESTIMONIALS_TITLE: config.testimonials_title || 'What Customers Say',
        TESTIMONIALS: config.testimonials || [
            { text: 'This product changed my workflow!', author: 'Jane D.', role: 'Customer' }
        ],
        
        // FAQ
        FAQ_TITLE: config.faq_title || 'Frequently Asked Questions',
        FAQS: config.faqs || [
            { question: 'How do I receive the product?', answer: 'After purchase, you\'ll get instant access to a PDF download.' }
        ],
        
        // Footer
        COPYRIGHT_YEAR: new Date().getFullYear(),
        AUTHOR_NAME: config.author || 'Your Name'
    };
    
    // Merge any custom template data from config
    if (config.template_data) {
        Object.assign(data, config.template_data);
    }
    
    return data;
}

// Main compilation function
async function compileLanding(productDir, outputPath, templatePath, minifyHtml = true) {
    console.log(`Compiling landing page for product in: ${productDir}`);
    
    // Read product config
    const config = await readProductConfig(productDir);
    console.log(`Using product config: ${config.title || 'unnamed product'}`);
    
    // Read template
    const template = await readTemplate(templatePath);
    
    // Prepare template data
    const templateData = prepareTemplateData(config, productDir);
    
    // Render template
    let html = renderTemplate(template, templateData);
    
    // Minify HTML if requested
    if (minifyHtml) {
        try {
            html = minify(html, MINIFY_OPTIONS);
            console.log('HTML minified');
        } catch (err) {
            console.warn('HTML minification failed:', err.message);
        }
    }
    
    // Ensure output directory exists
    const outputDir = path.dirname(outputPath);
    await fs.mkdir(outputDir, { recursive: true });
    
    // Write output file
    await fs.writeFile(outputPath, html, 'utf8');
    console.log(`Landing page compiled: ${outputPath}`);
    
    return outputPath;
}

// CLI entry point
async function main() {
    const args = process.argv.slice(2);
    let productDir = '.';
    let outputFile = DEFAULT_OUTPUT_FILE;
    let templateFile = DEFAULT_TEMPLATE_FILE;
    let minifyHtml = true;
    
    // Parse command line arguments
    for (let i = 0; i < args.length; i++) {
        switch (args[i]) {
            case '--product-dir':
                productDir = args[++i];
                break;
            case '--output':
                outputFile = args[++i];
                break;
            case '--template':
                templateFile = args[++i];
                break;
            case '--no-minify':
                minifyHtml = false;
                break;
            case '--help':
                console.log(`
Usage: node compile-landing.js [options]

Options:
  --product-dir DIR   Product directory containing product.json (default: .)
  --output FILE       Output HTML file path (default: build/landing.html)
  --template FILE     Template HTML file path (default: templates/digital-product/landing-template.html)
  --no-minify         Disable HTML minification
  --help              Show this help
                `);
                process.exit(0);
        }
    }
    
    // Resolve paths
    productDir = path.resolve(productDir);
    outputFile = path.resolve(productDir, outputFile);
    templateFile = path.resolve(productDir, templateFile);
    
    try {
        await compileLanding(productDir, outputFile, templateFile, minifyHtml);
        process.exit(0);
    } catch (error) {
        console.error('Error compiling landing page:', error);
        process.exit(1);
    }
}

if (require.main === module) {
    main();
}

module.exports = { compileLanding, renderTemplate, prepareTemplateData };