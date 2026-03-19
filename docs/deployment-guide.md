# Deployment Guide for Digital Products

This guide covers deployment processes for different types of digital products in the portfolio.

## 1. Next.js Applications (Vercel)

### Prerequisites
- Node.js 18+ installed
- Vercel account (vercel.com)
- Vercel CLI installed (`npm i -g vercel`)

### Configuration Files

#### `vercel.json`
```json
{
  "buildCommand": "npm run build",
  "devCommand": "npm run dev",
  "installCommand": "npm install",
  "outputDirectory": ".next",
  "framework": "nextjs"
}
```

#### Environment Variables
Set in Vercel dashboard:
- `NEXT_PUBLIC_*` – Public environment variables
- `STRIPE_SECRET_KEY` – API keys (never commit)
- `DATABASE_URL` – Database connection strings

### Deployment Steps

1. **Login to Vercel**
   ```bash
   vercel login
   ```

2. **Link project**
   ```bash
   vercel link
   ```

3. **Deploy to production**
   ```bash
   vercel --prod
   ```

### Automated CI/CD with GitHub Actions
See `.github/workflows/deploy-vercel.yml` for automatic deployment on push to main.

## 2. Static Landing Pages (Netlify)

### Prerequisites
- Netlify account (netlify.com)
- Netlify CLI installed (`npm i -g netlify-cli`)

### Configuration Files

#### `netlify.toml`
```toml
[build]
  publish = "."
  command = "npm run build"

[[redirects]]
  from = "/*"
  to = "/index.html"
  status = 200

[[headers]]
  for = "/*"
  [headers.values]
    X-Frame-Options = "DENY"
    X-Content-Type-Options = "nosniff"
    X-XSS-Protection = "1; mode=block"
```

### Deployment Steps

1. **Login to Netlify**
   ```bash
   netlify login
   ```

2. **Initialize site**
   ```bash
   netlify init
   ```

3. **Deploy to production**
   ```bash
   netlify deploy --prod
   ```

### Custom Domain Setup
1. Go to Site settings > Domain management in Netlify dashboard
2. Add custom domain and follow DNS configuration steps
3. Enable HTTPS automatically via Let's Encrypt

## 3. Laravel PHP Applications (Traditional Hosting)

### Recommended Hosting Providers
- Laravel Forge (forge.laravel.com)
- DigitalOcean + Laravel Forge
- Shared hosting with PHP 8+ and MySQL

### Deployment Checklist
- [ ] PHP 8.2+ installed
- [ ] MySQL/MariaDB database created
- [ ] Composer dependencies installed
- [ ] `.env` file configured with database credentials
- [ ] Storage directory permissions (755 for storage, 775 for bootstrap/cache)
- [ ] Queue workers configured (Supervisor)
- [ ] SSL certificate installed (Let's Encrypt)

## 4. Digital Products (Gumroad)

### Product Launch Process
1. **Product Preparation**
   - Create product files (PDF, Notion templates, etc.)
   - Design cover image (1280x720px recommended)
   - Write product description and benefits

2. **Gumroad Setup**
   - Create new product on gumroad.com
   - Upload files and set pricing
   - Configure sales page (custom domain optional)

3. **Marketing Assets**
   - Create landing page (static HTML on Netlify)
   - Set up email capture (ConvertKit/Mailchimp)
   - Prepare social media posts

## 5. Environment Variables Security

### Never Commit Secrets
- Use `.env.local` for local development
- Add `.env*` to `.gitignore`
- Store production secrets in hosting provider's environment variables

### Example `.env.local`
```env
STRIPE_SECRET_KEY=sk_live_...
DATABASE_URL=mysql://user:pass@localhost:3306/db
NEXT_PUBLIC_STRIPE_PUBLISHABLE_KEY=pk_live_...
```

## 6. Monitoring and Maintenance

### Post-Deployment Checks
- [ ] Website loads without errors
- [ ] HTTPS is working (green padlock)
- [ ] Forms submit successfully
- [ ] Database connections work
- [ ] Email notifications are sent

### Performance Monitoring
- Google PageSpeed Insights
- Vercel Analytics / Netlify Analytics
- Uptime monitoring (uptimerobot.com)

## 7. Quick Reference Commands

### Vercel
```bash
# Deploy preview
vercel

# Deploy production
vercel --prod

# View deployment logs
vercel logs
```

### Netlify
```bash
# Deploy to draft
netlify deploy

# Deploy to production
netlify deploy --prod

# Open site dashboard
netlify open
```

---

*Last updated: March 2026*