# Plan: Optimeringar för AI Prompts Landing Page

## Overview
Lägg till CSS minifiering och bildoptimering till den statiska landningssidan för AI Prompts produkten. Projektet är hostat på Netlify och består av en HTML-fil, en CSS-fil och en extern bild från Unsplash. Optimeringarna ska förbättra sidans prestanda, minska filstorlekar och förbättra användarupplevelsen.

## Project Type
WEB (statisk webbplats)

## Success Criteria
1. CSS-filen minifierad med minst 30% storleksminskning
2. Alla bilder optimerade (lokala om de läggs till) med lämplig komprimering
3. Lazy loading för bilder implementerat
4. Netlify build process som automatiskt kör optimeringar vid deployment
5. Sidans Lighthouse Performance score förbättrad med minst 10 poäng

## Tech Stack
- **Netlify** – hosting och CI/CD
- **npm scripts** – byggautomatisering
- **cssnano** – CSS minifiering
- **imagemin** – bildoptimering (för eventuella framtida lokala bilder)
- **html-minifier** – HTML minifiering (valfritt)
- **lighthouse** – prestandamätning

## File Structure
```
workspace/projects/ai-prompts-product/
├── index.html
├── style.css
├── netlify.toml
├── package.json (ny)
├── build/
│   ├── style.min.css
│   └── assets/ (för optimerade bilder)
└── .gitignore (uppdaterad)
```

## Task Breakdown

### Task 1: Skapa npm-projekt och byggkonfiguration
**Agent:** frontend-specialist  
**Skills:** nodejs, build-tools  
**Priority:** P0  
**Dependencies:** Ingen  
**INPUT:** Befintliga filer (index.html, style.css, netlify.toml)  
**OUTPUT:** package.json med scripts och dependencies, .gitignore uppdaterad  
**VERIFY:** `npm install` körs utan fel, `npm run build` skapar en build/ mapp

### Task 2: Implementera CSS minifiering
**Agent:** frontend-specialist  
**Skills:** css, optimization  
**Priority:** P1  
**Dependencies:** Task 1  
**INPUT:** style.css och package.json  
**OUTPUT:** Minifierad style.min.css i build/ mapp, npm script "build:css"  
**VERIFY:** style.min.css är minst 30% mindre än original, validerar korrekt CSS

### Task 3: Konfigurera bildoptimering
**Agent:** frontend-specialist  
**Skills:** image-optimization  
**Priority:** P1  
**Dependencies:** Task 1  
**INPUT:** Eventuella lokala bilder (för nu ingen, men förberedelse för framtida)  
**OUTPUT:** Imagemin konfiguration, npm script "build:images", assets/ mapp  
**VERIFY:** Scriptet kan optimera testbilder utan fel

### Task 4: Lägg till lazy loading för bilder
**Agent:** frontend-specialist  
**Skills:** html, performance  
**Priority:** P2  
**Dependencies:** Ingen  
**INPUT:** index.html med extern bild  
**OUTPUT:** Uppdaterad index.html med loading="lazy" och width/height attribut  
**VERIFY:** HTML validerar, bilder laddas lazy i devtools

### Task 5: Uppdatera Netlify build konfiguration
**Agent:** frontend-specialist  
**Skills:** netlify, deployment  
**Priority:** P1  
**Dependencies:** Task 1, Task 2, Task 3  
**INPUT:** netlify.toml och package.json  
**OUTPUT:** Uppdaterad netlify.toml med build command och publish directory  
**VERIFY:** Netlify deploy preview bygger korrekt med optimerade filer

### Task 6: Verifiera optimeringar med Lighthouse
**Agent:** frontend-specialist  
**Skills:** performance-testing  
**Priority:** P2  
**Dependencies:** Task 5  
**INPUT:** Deployad preview URL  
**OUTPUT:** Lighthouse rapport med förbättrad performance score  
**VERIFY:** Performance score ökad med minst 10 poäng jämfört med baseline

## Phase X: Verification

### Mandatory Script Execution
```bash
# Kör från projektroten (workspace/projects/ai-prompts-product/)
npm run build
# Kontrollera att build/ innehåller minifierade filer
ls -la build/

# Validera HTML
python -m html5validator index.html

# Kör Lighthouse audit (kräver att servern körs)
npm run lighthouse
```

### Rule Compliance
- [ ] Ingen purple/violet hex codes i CSS
- [ ] Ingen standard template layout (unik design)
- [ ] Socratic Gate respekterad (krav klargjorda)

### Completion Marker
```
## ✅ PHASE X COMPLETE
- Build: ✅ Success
- CSS minifiering: ✅ 30%+ reduktion
- Bildoptimering: ✅ Konfigurerad för framtida bilder
- Lazy loading: ✅ Implementerad
- Lighthouse: ✅ Förbättrad performance
- Date: 2026-03-19
```

## Risks & Mitigation
1. **Netlify build time increase** – Optimeringar kan öka build tiden. Mitigation: Använd caching och endast optimera vid ändringar.
2. **Externa bilder kan inte optimeras** – Vi kan inte optimera Unsplash-bilder direkt. Mitigation: Lägg till width/height attribut och lazy loading för bättre prestanda.
3. **CSS minifiering bryter något** – Testa noggrant och behåll originalet som fallback.

## Notes
- Projektet har för närvarande ingen lokal bild, men konfigurationen ska vara redo för framtida tillägg.
- CSS minifiering kan kombineras med autoprefixer för bättre browser support.
- Överväg att lägga till HTML minifiering om storleken är kritisk.