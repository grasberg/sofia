// Example product analytics tracking
document.addEventListener('DOMContentLoaded', function() {
    // Check if Vercel Analytics is available
    const hasVercelAnalytics = typeof analytics !== 'undefined';
    
    // Affiliate button click tracking
    document.querySelectorAll('[data-affiliate-id]').forEach(button => {
        button.addEventListener('click', function(e) {
            const affiliateId = this.getAttribute('data-affiliate-id');
            const buttonText = this.textContent.trim();
            
            // Log to console (for testing)
            console.log(`Affiliate click tracked: ${affiliateId} - ${buttonText}`);
            
            // Track with Vercel Analytics if available
            if (hasVercelAnalytics) {
                try {
                    analytics.track('Affiliate Click', {
                        affiliateId: affiliateId,
                        buttonText: buttonText,
                        timestamp: new Date().toISOString()
                    });
                    console.log('Vercel Analytics event sent: Affiliate Click');
                } catch (err) {
                    console.error('Error sending to Vercel Analytics:', err);
                }
            }
            
            // Simulate opening affiliate link (in real scenario would redirect)
            alert(`[TEST] Affiliate link clicked: ${affiliateId}\n\nIn a real scenario, this would redirect to the product page.\n\nEvent logged to analytics.`);
        });
    });
    
    // Newsletter form submission tracking
    const newsletterForm = document.getElementById('newsletter-form');
    if (newsletterForm) {
        newsletterForm.addEventListener('submit', function(e) {
            e.preventDefault();
            const email = this.querySelector('input[type="email"]').value;
            
            console.log(`Newsletter subscription: ${email}`);
            
            if (hasVercelAnalytics) {
                try {
                    analytics.track('Newsletter Signup', {
                        email: email.substring(0, 3) + '...', // Partial for privacy
                        timestamp: new Date().toISOString()
                    });
                    console.log('Vercel Analytics event sent: Newsletter Signup');
                } catch (err) {
                    console.error('Error sending to Vercel Analytics:', err);
                }
            }
            
            // Show confirmation
            alert(`[TEST] Thank you for subscribing with email: ${email}\n\nYou'll receive your free prompts shortly.`);
            this.reset();
        });
    }
    
    // Page view tracking simulation
    console.log('Page loaded - analytics tracking initialized');
    if (hasVercelAnalytics) {
        console.log('Vercel Analytics is available');
    } else {
        console.log('Vercel Analytics not loaded (running in test mode)');
    }
    
    // Display analytics info on page (for demo purposes)
    const analyticsInfo = document.createElement('div');
    analyticsInfo.style.cssText = `
        position: fixed;
        bottom: 10px;
        right: 10px;
        background: #1e293b;
        color: white;
        padding: 10px 15px;
        border-radius: 8px;
        font-size: 12px;
        z-index: 1000;
        max-width: 300px;
        box-shadow: 0 4px 12px rgba(0,0,0,0.2);
    `;
    analyticsInfo.innerHTML = `
        <strong>Analytics Test Mode</strong><br>
        • Clicks tracked to console<br>
        • Vercel Analytics: ${hasVercelAnalytics ? 'Available' : 'Not loaded'}<br>
        • Netlify Analytics: Auto-tracked on deploy
    `;
    document.body.appendChild(analyticsInfo);
});