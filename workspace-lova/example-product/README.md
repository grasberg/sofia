# Example Product - Analytics Testing

This is a simple example product landing page for testing Vercel Analytics and Netlify Analytics for affiliate monitoring.

## Features

- **Affiliate button tracking**: Two CTA buttons with data attributes for tracking clicks
- **Newsletter signup form**: Tracks email submissions
- **Testimonials section**: Social proof section
- **Responsive design**: Works on mobile and desktop
- **Analytics integration**: Prepared for Vercel Analytics custom events

## How to Test with Vercel Analytics

1. **Deploy to Vercel**:
   - Push this folder to a GitHub repository
   - Import the project in Vercel
   - Enable "Vercel Analytics" in your project settings
   - Add the Analytics SDK to your site (already included in index.html)

2. **Configure tracking**:
   - The page uses `@vercel/analytics` (loaded from CDN in test mode)
   - In production, use the official Vercel Analytics script
   - Custom events are sent for:
     - `Affiliate Click` with `affiliateId` and `buttonText`
     - `Newsletter Signup` with partial email

3. **View analytics**:
   - Go to your Vercel project dashboard > Analytics
   - View page views, custom events, and traffic sources

## How to Test with Netlify Analytics

1. **Deploy to Netlify**:
   - Drag and drop this folder to Netlify Drop
   - Or connect a Git repository
   - Netlify Analytics is automatically enabled on Pro plans

2. **Using netlify.toml (advanced)**:
   - This project includes a `netlify.toml` configuration file
   - It sets up proper caching headers, security headers, and redirects
   - To deploy with Git:
     1. Push this folder to a GitHub/GitLab repository
     2. Connect the repository in Netlify
     3. The build settings will be automatically detected from `netlify.toml`
     4. Deploy the site

3. **Limitations**:
   - Netlify Analytics does NOT support custom events
   - You can only track page views, traffic sources, geography
   - Affiliate clicks cannot be tracked individually

## Local Testing

Open `index.html` in a browser or use a local server:

```bash
# Python 3
python3 -m http.server 8000

# Node.js
npx serve .
```

Check the browser console for analytics event logs.

## Files

- `index.html` - Main page structure
- `style.css` - Styling
- `script.js` - Analytics tracking logic
- `README.md` - This file

## What This Demonstrates

1. **Vercel Analytics strengths**:
   - Custom event tracking for affiliate clicks
   - Detailed conversion tracking
   - Programmatic control via JavaScript

2. **Netlify Analytics strengths**:
   - Zero-configuration setup
   - Built-in with deployment
   - Simple page view statistics

3. **Recommendation for affiliate monitoring**:
   - Use Vercel Analytics if you need detailed conversion tracking
   - Use Netlify Analytics only for basic traffic monitoring
   - Consider additional tools (like Google Analytics 4 or Plausible) for more advanced affiliate tracking

## Next Steps for Testing

1. Deploy to both platforms and compare the analytics dashboards
2. Generate traffic to see real data
3. Test conversion tracking with actual affiliate links
4. Evaluate cost vs. value for your expected traffic volume