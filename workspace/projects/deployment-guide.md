# Deployment Guide för Landing Pages och Apps

Denna guide beskriver hur du deployar två typer av projekt:
1. **Next.js-appar** till Vercel (för dynamiska webbapplikationer)
2. **Statiska landing pages** till Netlify (för enkla HTML/CSS/JS-sidor)

## 1. Next.js-appar på Vercel

### Förutsättningar
- Ett Next.js-projekt med `package.json` och `next.config.js`
- Git-repository för projektet
- Vercel-konto (gratis nivå tillräcklig)

### Konfigurationsfiler

#### `vercel.json` (placeras i projektrot)
```json
{
  "buildCommand": "npm run build",
  "devCommand": "npm run dev",
  "installCommand": "npm install",
  "outputDirectory": ".next",
  "framework": "nextjs",
  "regions": ["arn1"],
  "env": {
    "NEXT_PUBLIC_SITE_URL": "https://your-domain.vercel.app"
  }
}
```

#### Miljövariabler i Vercel Dashboard
Lägg till följande miljövariabler i Vercel-projektets settings:
- `NEXT_PUBLIC_SITE_URL`: Din apps URL (automatiskt satt av Vercel)
- Övriga secrets (t.ex. Stripe-nycklar, API-nycklar)

### Deployment-steg

#### Alternativ A: Vercel CLI (lokalt)
1. Installera Vercel CLI: `npm i -g vercel`
2. Logga in: `vercel login`
3. Navigera till projektmappen: `cd your-nextjs-project`
4. Kör: `vercel` (första gången) eller `vercel --prod` för produktion

#### Alternativ B: Git-integration (rekommenderas)
1. Pusha koden till GitHub/GitLab/Bitbucket
2. Gå till [Vercel Dashboard](https://vercel.com/dashboard)
3. Klicka "New Project"
4. Importera ditt repository
5. Konfigurera build-inställningar (Vercel känner automatiskt av Next.js)
6. Klicka "Deploy"

### Domänkonfiguration
1. I Vercel Dashboard: Settings → Domains
2. Lägg till anpassad domän (t.ex. `app.din-produkt.se`)
3. Följ instruktionerna för DNS-inställningar

### CI/CD (Automatisk deployment)
- Varje push till `main`-grenen deployar automatiskt till produktion
- Pull requests skapar preview-deployments automatiskt

## 2. Statiska Landing Pages på Netlify

### Förutsättningar
- Statiska filer (HTML, CSS, JavaScript) i en mapp
- Git-repository (rekommenderas) eller drag-and-drop

### Konfigurationsfiler

#### `netlify.toml` (placeras i projektrot)
```toml
[build]
  publish = "."
  command = "echo 'No build step needed for static site'"

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
    Referrer-Policy = "strict-origin-when-cross-origin"

[[headers]]
  for = "/style.css"
  [headers.values]
    Cache-Control = "public, max-age=31536000, immutable"

[[headers]]
  for = "/*.html"
  [headers.values]
    Cache-Control = "public, max-age=0, must-revalidate"
```

### Deployment-steg

#### Alternativ 1: Drag & Drop (enkelt)
1. Gå till [Netlify Drop](https://app.netlify.com/drop)
2. Dra hela projektmappen till det markerade området
3. Netlify skapar automatiskt en URL (t.ex. `random-name.netlify.app`)

#### Alternativ 2: Git-integration (för uppdateringar)
1. Skapa ett Git-repository med dina filer
2. Logga in på [Netlify](https://app.netlify.com)
3. Klicka på "New site from Git"
4. Välj ditt repository och gren (main/master)
5. Lämna build-inställningar som standard (Netlify kommer att känna av `netlify.toml`)
6. Klicka "Deploy site"

### Domänkonfiguration
1. I Netlify Dashboard: Site settings → Domain management
2. Klicka "Add custom domain"
3. Följ instruktionerna för DNS-inställningar

### SSL-certifikat
- Netlify tillhandahåller automatiskt Let's Encrypt SSL-certifikat
- Certifikat förnyas automatiskt

## 3. Workflow för nya produkter

### Steg-för-steg för nya projekt

#### A. Skapa en ny Next.js-app (för SaaS eller webapp)
1. `npx create-next-app@latest produkt-namn`
2. Lägg till `vercel.json` med ovanstående konfiguration
3. Konfigurera miljövariabler i `.env.local` för utveckling
4. Pusha till Git-repository
5. Skapa Vercel-projekt och länka repository
6. Konfigurera anpassad domän om nödvändigt

#### B. Skapa en ny statisk landing page (för produktlansering)
1. Skapa mappstruktur:
   ```
   produkt-landing/
   ├── index.html
   ├── style.css
   ├── script.js (valfritt)
   └── netlify.toml
   ```
2. Använd `index.html` och `style.css` från `ai-prompts-product` som mall
3. Anpassa innehåll, bilder och länkar
4. Pusha till Git-repository
5. Skapa Netlify-site och länka repository
6. Konfigurera anpassad domän

## 4. Best Practices

### Prestandaoptimering
- **Next.js:** Använd `next/image` för bildoptimering, enable compression i `next.config.js`
- **Statiska sidor:** Minifiera HTML/CSS/JS, optimera bilder före uppladdning

### Säkerhet
- Använd alltid HTTPS (automatiskt med Vercel/Netlify)
- Implementera säkerhetsheaders (se `netlify.toml`-exempel ovan)
- Lagra secrets som miljövariabler, aldrig i koden

### Monitoring
- **Vercel:** Använd Vercel Analytics för prestandamått
- **Netlify:** Använd Netlify Analytics (kostar extra) eller Google Analytics

### Backup och versionering
- All kod ska vara i Git-repository
- Regular commits med beskrivande commit-meddelanden
- Tagga viktiga versioner (t.ex. `v1.0.0`)

## 5. Felsökning

### Vanliga problem och lösningar

| Problem | Lösning |
|---------|---------|
| Build misslyckas på Vercel | Kontrollera `package.json` scripts, Node.js version |
| Statisk sida visar inte rätt innehåll | Kontrollera `netlify.toml` redirects, cache headers |
| Domän pekar inte rätt | Verifiera DNS-inställningar (A/CNAME records) |
| SSL-certifikat inte giltigt | Vänta 24h för propagation, kontrollera DNS |

### Loggar
- **Vercel:** Dashboard → Project → Deployments → Klicka på deployment → "View Logs"
- **Netlify:** Site dashboard → Deploys → Klicka på deploy → "Deploy Log"

## 6. Kostnader

### Gratis nivåer
- **Vercel:** 100GB bandbredd/månad, obegränsade deployment
- **Netlify:** 100GB bandbredd/månad, 300 build minutes/månad

### Uppgradering
Överväg uppgradering när:
- Du når bandbreddsgränser
- Du behöver fler teammedlemmar
- Du behöver avancerade funktioner (serverless functions, edge functions)

---

## Exempelprojekt

### 1. Next.js-app: Sofia Ops App
- Plats: `workspace/sofia-ops-app/`
- Deployment: Vercel
- Konfiguration: `vercel.json` (ska skapas)

### 2. Statisk landing page: AI Prompts Product
- Plats: `workspace/projects/ai-prompts-product/`
- Deployment: Netlify
- Konfiguration: `netlify.toml` (finns)
- Dokumentation: `DEPLOY.md` (finns)

---

## Support och resurser
- [Vercel Documentation](https://vercel.com/docs)
- [Netlify Documentation](https://docs.netlify.com/)
- [Next.js Documentation](https://nextjs.org/docs)
- [Git Handbook](https://guides.github.com/introduction/git-handbook/)

---

*Senast uppdaterad: 2026-03-19*