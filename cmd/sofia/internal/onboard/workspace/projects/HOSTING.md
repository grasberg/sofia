# Hosting Infrastructure for Landing Pages and Apps

Denna struktur beskriver hosting-lösningar för olika typer av projekt i workspace. Varje projekt har en optimal hosting-plattform baserat på dess tekniska krav.

## Projekt och deras Hosting-lösningar

### 1. Next.js Applications → Vercel
**Optimalt för:** Moderna webbapplikationer med React, TypeScript, server-side rendering

**Projekt:**
- `sofia-ops-app/` – Next.js 16 med React 19, TypeScript, Tailwind CSS
- `outreach-saas/` – Next.js app struktur (kan utvecklas till SaaS)

**Konfiguration:**
- `vercel.json` i projektrot med build-inställningar
- Automatisk CI/CD via Git-integration
- Edge Functions, Image Optimization, Analytics

**Deployment:**
```bash
# Lokal test
vercel dev

# Deploy till produktion
vercel --prod
```

### 2. Static Landing Pages → Netlify
**Optimalt för:** Enkla HTML/CSS/JS-sidor, produktlandningssidor, mikrosajter

**Projekt:**
- `ai-prompts-product/` – Statisk landing page för AI prompts produkt

**Konfiguration:**
- `netlify.toml` med redirects, security headers, cache-inställningar
- `DEPLOY.md` med steg-för-steg instruktioner

**Deployment:**
```bash
# Lokal test
netlify dev

# Deploy till produktion
netlify deploy --prod
```

### 3. Laravel Applications → Traditional Hosting
**Optimalt för:** PHP-applikationer med databas, fullstack MVC

**Projekt:**
- `affiliate-system/` – Laravel app med MySQL (affiliate tracking system)

**Hosting-alternativ:**
- Shared hosting med PHP 8+ och MySQL
- PaaS: Laravel Forge, Heroku, AWS Elastic Beanstalk
- VPS: DigitalOcean, Linode med Laravel deployment script

### 4. Digital Products → Content Delivery
**Optimalt för:** Nedladdningsbara produkter, kursmaterial, templates

**Projekt:**
- `ai-content-service/` – Innehållstjänst (kan vara statisk site)
- Digitala produkter (mentor-mini-course, notion-templates)

**Hosting-alternativ:**
- Statisk site generator (Hugo, Jekyll) deployad till Netlify
- Gumroad för försäljning och distribution
- GitHub Pages för dokumentation

## Workflow för nya projekt

### Steg 1: Identifiera projekttyp
1. **Webbapp?** → Next.js → Vercel
2. **Landningssida?** → HTML/CSS → Netlify
3. **Backend API?** → Laravel/Node.js → Passande backend hosting
4. **Digital produkt?** → Statisk site eller plattform som Gumroad

### Steg 2: Skapa konfiguration
- Next.js: Lägg till `vercel.json`
- Statisk sida: Lägg till `netlify.toml` och `DEPLOY.md`
- Laravel: Skapa `Procfile` för PaaS eller deployment scripts

### Steg 3: Sätt upp Git och CI/CD
- Skapa Git-repository
- Konfigurera GitHub Actions för automatisk testing
- Länka till Vercel/Netlify för automatisk deployment

### Steg 4: Deploy och konfigurera domän
- Deploy första versionen
- Lägg till anpassad domän
- Konfigurera SSL (automatiskt med Vercel/Netlify)

## Kostnadsöversikt

| Plattform | Gratis nivå | Begränsningar | Uppgradering när |
|-----------|-------------|---------------|------------------|
| **Vercel** | 100GB/månad | 1 medlem | >100GB trafik, fler teammedlemmar |
| **Netlify** | 100GB/månad, 300 build minuter | 1 medlem | >100GB trafik, >300 build minuter |
| **Shared hosting** | Från ~50 kr/månad | Resursbegränsningar | Högre trafik, mer CPU/RAM |
| **DigitalOcean VPS** | $6/månad (bas) | Självhantering | Skalning behövs |

## Best Practices

### 1. Miljövariabler
- Lagra secrets (API-nycklar, databaslösenord) som miljövariabler
- Använd `.env.local` för utveckling
- Konfigurera i Vercel/Netlify dashboard för produktion

### 2. Versionering
- All kod i Git
- Semantisk versionering (v1.0.0)
- Feature branches och pull requests

### 3. Monitoring
- Vercel Analytics för prestanda
- Netlify Analytics (kostar extra)
- Google Analytics för användarstatistik

### 4. Backup
- Regular database backups (för Laravel app)
- Code backups via Git
- Asset backups (bilder, uploads) till cloud storage

## Snabbstart-guider

### Skapa en ny Next.js app och deploya till Vercel
1. `npx create-next-app@latest mitt-projekt`
2. `cd mitt-projekt`
3. Skapa `vercel.json` (kopiera från deployment-guide.md)
4. `git init`, `git add .`, `git commit -m "Initial commit"`
5. Skapa repo på GitHub och push
6. Gå till Vercel Dashboard → New Project → Import repo
7. Konfigurera miljövariabler och klicka Deploy

### Skapa en ny statisk landing page och deploya till Netlify
1. Kopiera mallen från `ai-prompts-product/`
2. Uppdatera `index.html` och `style.css`
3. Anpassa `netlify.toml` om nödvändigt
4. `git init`, `git add .`, `git commit -m "Initial commit"`
5. Skapa repo på GitHub och push
6. Gå till Netlify Dashboard → New site from Git
7. Välj repo och klicka Deploy

## Support och felsökning

### Vanliga problem
- **Build misslyckas:** Kontrollera Node.js version, package.json scripts
- **Domän pekar inte rätt:** Verifiera DNS-inställningar (A/CNAME records)
- **SSL-certifikat inte giltigt:** Vänta 24h för propagation

### Resurser
- [Deployment Guide](deployment-guide.md) – Detaljerad guide för Vercel och Netlify
- [Vercel Documentation](https://vercel.com/docs)
- [Netlify Documentation](https://docs.netlify.com/)
- [Laravel Deployment Guide](https://laravel.com/docs/deployment)

---

**Senast uppdaterad:** 2026-03-19  
**Status:** Aktiv – alla projekt har hosting-planer, Next.js och statiska sidor redo för deployment