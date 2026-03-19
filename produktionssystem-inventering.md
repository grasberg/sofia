# Inventering av produktionssystem och stack

Genomförd: 2026-03-19 11:45
Sökväg: `/Users/magnusgrasberg/.sofia/workspace-lova`

## Översikt

Följande system har identifierats i arbetsytan. Vissa kan vara under utveckling och inte live i produktion.

### 1. Affiliate System
- **Beskrivning**: Laravel-baserat affiliate-spårningssystem med dashbord, länkar, klick och konversioner.
- **Stack**:
  - Backend: Laravel (PHP 8+)
  - Autentisering: Laravel Sanctum (API tokens)
  - Databas: MySQL (Eloquent ORM)
  - Frontend: Blade templates (server-side rendering)
  - Ytterligare: Queue system (förmodligen Redis), Caching
- **Hosting**: Ej specifierat (troligen traditionell webbserver som Laravel Forge, DigitalOcean, eller liknande)
- **Status**: Kod finns i `workspace/projects/affiliate-system/`. Fullständig databasschema och routes definierade.
- **Notering**: Har migreringar för affiliate_links, affiliate_clicks, affiliate_conversions, affiliate_commission_summaries.

### 2. AI Prompts Product (Landingssida)
- **Beskrivning**: Statisk landningssida för försäljning av AI-prompts via Gumroad.
- **Stack**:
  - Frontend: HTML5, CSS3 (custom CSS)
  - Hosting: Netlify (konfigurerat med `netlify.toml`)
  - Domänhantering: Ej specifierat
- **Status**: Färdig för deployment. Filer i `workspace/projects/ai-prompts-product/`.
- **Notering**: Inkluderar Netlify-config för redirects, security headers och cache.

### 3. Outreach SaaS
- **Beskrivning**: Next.js-applikation för outreach-automation (under utveckling).
- **Stack**:
  - Frontend: Next.js 14 (App Router), TypeScript, React
  - Hosting: Troligen Vercel (standard för Next.js)
- **Status**: Grundstruktur skapad i `workspace/outreach-saas/`. Tom `page.tsx` och `layout.tsx`.
- **Notering**: Projektet verkar vara i början av utveckling.

### 4. Sofia Ops App
- **Beskrivning**: Next.js-app för Sofia-agentens operationella dashboard.
- **Stack**:
  - Frontend: Next.js 14 (App Router), TypeScript, React
  - Styling: CSS-moduler / Tailwind (ej bekräftat)
  - Hosting: Troligen Vercel
- **Status**: Bootstrappad med `create-next-app`. Filer i `workspace/sofia-ops-app/`.
- **Notering**: Har fullständig Next.js-struktur med `app/`, `public/`, `node_modules`.

### 5. Stripe Checkout Server
- **Beskrivning**: Node.js-server för hantering av Stripe Checkout för digitala produkter med affiliate-tracking.
- **Stack**:
  - Backend: Node.js (ES modules), Express.js
  - Betalning: Stripe API
  - Rate limiting: Redis (via `rate-limit-redis`)
  - Caching: `node-cache`
  - Hosting: Netlify (serverless functions via `server.js`)
- **Status**: Live-konfiguration med `.env`, `netlify.toml`. Kod i `stripe-checkout/`.
- **Notering**: Inkluderar UTM‑tracking, webhook-hantering, analytics.

### 6. Sofia Agent (Go)
- **Beskrivning**: Lokal AI‑agent skriven i Go som används för automatisering, kommunikation med olika API:er och verktyg.
- **Stack**:
  - Språk: Go 1.25.7
  - GUI: Ingen (kommandoradsbaserad)
  - API:er: Anthropic, OpenAI, Discord, Telegram, Bitcoin, m.fl.
  - Databas: SQLite (modernc.org/sqlite)
  - Ytterligare: Playwright för webbläsarautomatisering, MCP‑server
- **Hosting**: Körs lokalt på användarens dator.
- **Status**: Aktiv utveckling med `go.mod` i rotkatalogen.

### 7. Digitala produkter (innehåll)
- **Beskrivning**: Samling av digitala produkter (Notion‑mallar, mentor‑kurser, resource libraries, swipe‑filer).
- **Stack**:
  - Format: Markdown‑filer
  - Distribution: Ej specifierat (troligen Gumroad, Podia eller egen webbplats)
- **Status**: Innehåll finns i `workspace-lova/digital-products/`. Ej hostat som eget system.

### 8. AI Content Service
- **Beskrivning**: Enkel CSV‑ och template‑hantering för outreach‑kampanjer.
- **Stack**: Okänt (kan vara Python‑skript eller liknande).
- **Status**: Endast `leads.csv` och `outreach_template.txt` i `workspace/projects/ai-content-service/`. Ingen kod.

### 9. Niche Selection Toolkit
- **Beskrivning**: Produkt som planeras för Product Hunt‑lansering (se mål).
- **Stack**: Ej definierad.
- **Status**: Specifikationer finns i `workspace/products/niche_selection_toolkit_specs.md`.

## Sammanfattning av teknisk stack över alla system

| Kategori       | Teknologier                                                                 |
|----------------|-----------------------------------------------------------------------------|
| Frontend       | HTML/CSS, React (Next.js), TypeScript, Blade (Laravel)                     |
| Backend        | Laravel (PHP), Node.js (Express), Go                                       |
| Databaser      | MySQL, SQLite, Redis (cache/rate limit)                                    |
| Hosting        | Netlify (static + serverless), Vercel (Next.js), traditionell webbserver   |
| Betalningar    | Stripe                                                                     |
| Authentication | Laravel Sanctum, Sessions                                                  |
| APIs           | Stripe, Anthropic, OpenAI, Discord, Telegram, Google, GitHub, Bitcoin      |
| DevOps         | Netlify CI/CD, Vercel CI/CD (förmodligen), Git                             |

## Nästa steg för fullständig inventering

1. **Domän‑ och hosting‑info**: Kontrollera vilka domäner och hosting‑tjänster som faktiskt används.
2. **Live‑status**: Verifiera vilka system som är live och vilka som är under utveckling.
3. **Miljövariabler**: Granska `.env`‑filer för att förstå integrationer (Stripe, API‑nycklar).
4. **Databas‑åtkomst**: Undersök anslutningar till MySQL/Redis.
5. **CI/CD‑pipelines**: Leta efter GitHub Actions, Netlify/Vercel‑konfigurationer.

## Rekommendationer

- **Konsolidera hosting**: Överväg att samla Laravel‑appen och Node‑servern på samma hosting‑plattform (t.ex. Laravel Forge + Netlify).
- **Säkerhetsgranskning**: Kontrollera att `.env`-filer inte exponeras i publika repositories.
- **Dokumentation**: Skapa en central `INFRASTRUCTURE.md` med alla system, domäner och åtkomstuppgifter.

---

*Inventeringen baseras på filstrukturen i arbetsytan. Ytterligare system kan finnas utanför denna mapp.*