# Dokumentation för Digital Produktstruktur

## Goal
Skapa omfattande dokumentation och README för hela strukturen kring digitala produkter i workspace, inklusive produkt-specifikationer, PDF-generation, landningssidor, Stripe-integration och arbetsflöden.

## Översikt
Workspace-strukturen innehåller flera komponenter för att skapa, hantera och sälja digitala produkter:
1. **Produkt-specifikationer** - Markdown-filer med produktbeskrivningar och tier-struktur
2. **PDF-generation** - Python- och Node.js-skript för att generera PDF:er från markdown
3. **Landningssidor** - HTML/CSS/JS för varje produkt med unikt design
4. **Stripe-integration** - API-integration för betalningshantering
5. **Admin-dashboard** - Översikt över ordrar, affiliates och provisioner
6. **Affiliate-system** - Spårning och provisioner för partners

## Success Criteria
- [ ] Komplett README.md i workspace-root som förklarar hela strukturen
- [ ] Dokumenterat format för produkt-specifikationer
- [ ] Steg-för-steg-guide för att lägga till ny produkt
- [ ] Dokumentation för PDF-generation med både Python och Node.js
- [ ] Guide för att skapa landningssidor med mallar
- [ ] Dokumentation för Stripe-integration och produktskapande
- [ ] Testat flöde från produkt-spec till live landningssida

## Tech Stack
- **Produkt-specifikationer:** Markdown med YAML frontmatter (föreslaget)
- **PDF-generation:** Python (pandoc + weasyprint) och Node.js (puppeteer)
- **Landningssidor:** HTML5, CSS3, Vanilla JavaScript
- **Backend:** Node.js/Express för Stripe-integration
- **Databas:** JSON-filer eller SQLite för enkelhet
- **Deployment:** Netlify, Vercel, eller GitHub Pages

## File Structure (föreslagen standardisering)
```
workspace/
├── products/                          # Alla produkt-specifikationer
│   ├── niche-selection-toolkit.md     # Produkt-spec med tiers
│   ├── ai-prompts-bundle.md
│   └── ...
├── projects/                          # Landningssida-projekt
│   ├── niche-selection-toolkit/
│   │   ├── index.html
│   │   ├── style.css
│   │   └── js/
│   └── ...
├── scripts/                           # Byggskript
│   ├── generate-pdf.py
│   ├── generate-pdf.js
│   └── create-landing-page.js
├── templates/                         # Mallar för landningssidor
│   ├── basic-product/
│   └── premium-course/
└── digital-products/                  # Färdig plattform (befintlig)
    ├── admin/
    ├── products/
    ├── routes/
    └── ...
```

## Task Breakdown

### Task 1: Analysera befintlig struktur och dokumentation
**INPUT:** Current workspace files and folders  
**OUTPUT:** Lista över alla komponenter, deras syfte och hur de samverkar  
**VERIFY:** Dokumentationen innehåller korrekt mappstruktur och filändamål

### Task 2: Skapa övergripande README för workspace-struktur
**INPUT:** Analys från Task 1  
**OUTPUT:** README.md i workspace-root som förklarar hela ekosystemet  
**VERIFY:** README finns och täcker alla huvudkomponenter

### Task 3: Dokumentera produkt-specifikationsformat
**INPUT:** Befintliga produktfiler (niche_selection_toolkit_specs.md, ai_prompts_product1.md)  
**OUTPUT:** Standardiserat format med exempel och förklaringar  
**VERIFY:** Formatet är tydligt och inkluderar alla nödvändiga fält

### Task 4: Dokumentera PDF-generation flöde
**INPUT:** md_to_pdf.py och eventuella andra PDF-skript  
**OUTPUT:** Dokumentation för hur man genererar PDF:er från produkt-specs  
**VERIFY:** Steg-för-steg instruktioner som fungerar

### Task 5: Dokumentera landningssida-skapande
**INPUT:** Befintliga landningssidor i digital-products/ och workspace/projects/  
**OUTPUT:** Guide för att skapa landningssidor med mallar  
**VERIFY:** Guide inkluderar HTML/CSS-struktur och konfiguration

### Task 6: Dokumentera Stripe-integration
**INPUT:** Stripe-kod i digital-products/ och eventuella planer  
**OUTPUT:** Dokumentation för att skapa Stripe-produkter och priser  
**VERIFY:** Dokumentation täcker hela flödet från produkt-spec till Stripe

### Task 7: Skapa guide för att lägga till ny produkt
**INPUT:** All dokumentation från tidigare steg  
**OUTPUT:** Steg-för-steg checklista från idé till live produkt  
**VERIFY:** Checklistan är komplett och testbar

### Task 8: Verifiera dokumentation genom att testa flödet
**INPUT:** All dokumentation  
**OUTPUT:** Testat helt flöde med en enkel testprodukt  
**VERIFY:** Alla steg fungerar som dokumenterat

## Phase X: Verification
- [ ] Läs och verifiera att README är komplett
- [ ] Testa produkt-specifikationsformat med en testfil
- [ ] Kör PDF-generation på testfil
- [ ] Skapa en test-landningssida med mallen
- [ ] Verifiera att Stripe-integrationsdokumentationen är korrekt
- [ ] Gå igenom "lägg till ny produkt"-guiden från början till slut
