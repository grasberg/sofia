# Netlify Analytics Configuration for Static Site

## Overview
Configure Netlify Analytics for a static Next.js site (sofia-ops-app) to track visitor activity. Netlify Analytics offers server-side analytics without client-side JavaScript, ensuring GDPR compliance and no performance impact. This plan covers deployment setup, analytics activation, and verification.

## Project Type
**WEB** (Next.js static site)

## Success Criteria
1. Site deployed on Netlify with successful build
2. Netlify Analytics enabled and collecting data
3. Analytics dashboard shows visitor metrics within 24 hours
4. No client‑side performance degradation
5. Configuration reproducible via version‑controlled files

## Tech Stack
- **Framework**: Next.js 16 (static export)
- **Build Tool**: npm / Node.js 18
- **Deployment**: Netlify (with `netlify.toml`)
- **Analytics**: Netlify Web Analytics (server‑side)
- **Optional**: Snippet injection for JS‑based alternatives (e.g., Simple Analytics, Plausible)

## File Structure
```
workspace/sofia-ops-app/
├── app/                    # Next.js app router
├── public/
├── package.json
├── next.config.ts
├── netlify.toml           ← to be added
└── .env.example           ← optional environment variables
```

## Task Breakdown

### Task 1: Survey existing Next.js configuration
**Agent**: `frontend-specialist`  
**Skills**: `clean-code`, `app-builder`  
**Priority**: P0  
**Dependencies**: none  
**INPUT**: `next.config.ts`, `package.json`, `vercel.json`  
**OUTPUT**: Decision on static‑export vs. serverless deployment  
**VERIFY**: Read config files, confirm Next.js version supports static export

### Task 2: Create Netlify deployment configuration
**Agent**: `frontend-specialist`  
**Skills**: `app-builder`  
**Priority**: P0  
**Dependencies**: Task 1  
**INPUT**: Template `netlify.toml`, Next.js build commands  
**OUTPUT**: `netlify.toml` in `sofia-ops-app/` with correct build settings  
**VERIFY**: File exists, contains valid TOML, specifies `publish = "out"` (if static export) or appropriate settings

### Task 3: Prepare environment variables (if needed)
**Agent**: `backend-specialist`  
**Skills**: `app-builder`  
**Priority**: P1  
**Dependencies**: Task 2  
**INPUT**: List of required env vars (e.g., `NEXT_PUBLIC_SITE_URL`)  
**OUTPUT**: `.env.example` with placeholders, instructions for Netlify UI  
**VERIFY**: File exists, variables documented

### Task 4: Deploy site to Netlify
**Agent**: `devops-engineer` (or manual via Netlify UI)  
**Skills**: None required  
**Priority**: P1  
**Dependencies**: Task 2, Task 3  
**INPUT**: GitHub repository link, Netlify account  
**OUTPUT**: Live site URL, build logs, successful deployment  
**VERIFY**: Site loads, `netlify.toml` respected, no build errors

### Task 5: Enable Netlify Analytics
**Agent**: `devops-engineer` (or manual via Netlify UI)  
**Skills**: None required  
**Priority**: P1  
**Dependencies**: Task 4  
**INPUT**: Netlify site dashboard, Pro plan required  
**OUTPUT**: Analytics turned on, data collection active  
**VERIFY**: Analytics section shows “Collecting data”, no JavaScript snippet needed

### Task 6: Verify analytics collection
**Agent**: `test-engineer`  
**Skills**: None required  
**Priority**: P2  
**Dependencies**: Task 5  
**INPUT**: Live site URL, Netlify dashboard  
**OUTPUT**: Confirmation that visits appear in dashboard (may take up to 24 hours)  
**VERIFY**: Trigger a test visit, check real‑time logs or next‑day report

### Task 7: Optional – Add snippet injection for alternative analytics
**Agent**: `frontend-specialist`  
**Skills**: `clean-code`  
**Priority**: P3  
**Dependencies**: Task 4  
**INPUT**: Snippet code from provider (e.g., Plausible, Simple Analytics)  
**OUTPUT**: Snippet injected via `netlify.toml` or Netlify UI  
**VERIFY**: Script appears in page source, provider dashboard shows activity

### Task 8: Document setup for future use
**Agent**: `frontend-specialist`  
**Skills**: `plan-writing`  
**Priority**: P3  
**Dependencies**: Task 6  
**INPUT**: All configuration steps performed  
**OUTPUT**: `DEPLOYMENT.md` in `sofia-ops-app/` with step‑by‑step guide  
**VERIFY**: File exists, includes commands, screenshots, troubleshooting

## Phase X: Verification
Before marking this plan complete, execute the following verification scripts (from `.agent/` directory):

1. **Security scan**: `python .agent/scripts/security_scan.py .`
2. **Build verification**: `cd workspace/sofia-ops-app && npm run build`
3. **Netlify CLI verification** (if installed): `netlify status`
4. **Lighthouse audit** (after deployment): `python .agent/scripts/lighthouse_audit.py <live-url>`

### Manual Checks
- [ ] No purple/violet hex codes in site design
- [ ] Socratic questioning was respected (requirements clarified)
- [ ] All tasks have explicit INPUT→OUTPUT→VERIFY criteria
- [ ] Plan file (`netlify-analytics-static.md`) is in project root

### Completion Marker
```
## ✅ PHASE X COMPLETE
- Security scan: ✅ Pass
- Build: ✅ Success
- Deployment: ✅ Live
- Analytics: ✅ Collecting data
- Date: [Current Date]
```