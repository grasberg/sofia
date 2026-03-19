# Sammanfattning och rekommendationer: Build scripts för PDF-generation och landing page compilation

## Nuvarande tillstånd (analys)

### Befintliga resurser:
1. **Python PDF-generation script** (`workspace/ai-prompts-product/md_to_pdf.py`)
   - Använder pandoc (markdown→HTML) + weasyprint (HTML→PDF)
   - Inkluderar grundläggande styling
   - Fungerar men har externa beroenden (pandoc, weasyprint)
   - Ingen standardiserad produktmappstruktur

2. **Produktinnehåll**:
   - `workspace/ai-prompts-product/ai_prompts_product1.md` – AI-skribent prompts bundle
   - `workspace/products/` – Ytterligare produktfiler och marknadsföringsmaterial
   - `workspace/ai-prompts-product/test.md` – Testfil

3. **Landing pages**:
   - Ingen befintlig landing page compilation
   - Inga HTML/CSS templates
   - Inget automatiserat byggsystem

## Identifierade behov

### 1. Standardiserad produktmappstruktur
Behov för enhetlig organisation av alla digitala produkter för skalbarhet och underhåll.

### 2. Förbättrad PDF-generation
- Stöd för flera språk (Svenska/Engelska)
- Professionell styling med varumärkesidentitet
- Automatiserad generering från markdown
- Stöd för frontmatter/metadata
- Möjlighet till batch-processing

### 3. Landing page compilation
- HTML/CSS templates för produkter
- Automatisk generering från produktmetadata
- Responsiv design
- SEO-optimering
- Integrering med betalningssystem (Stripe/Gumroad)

### 4. Byggautomation
- Enkla CLI-kommandon
- CI/CD-stöd
- Versionering av genererade filer
- Testning av output

## Rekommendationer

### PDF-generation

#### Kort sikt (MVP):
1. **Förbättra befintligt Python-skript**:
   - Lägg till stöd för YAML frontmatter för metadata
   - Implementera tematiserad styling (light/dark mode)
   - Lägg till sidhuvud/sidfot med produktinfo
   - Stöd för både Svenska och Engelska

2. **Skapa Node.js-alternativ med puppeteer**:
   - Bättre kontroll över rendering
   - Inbyggt i npm-ekosystem
   - Stöd för mer avancerad layout
   - Möjlighet att generera flera format (PDF, PNG, HTML)

#### Lång sikt:
- Skapa React-baserat PDF-template-system
- Stöd för dynamiskt innehåll (variabler)
- Batch-generering av produktpaket
- Watermarking för licensiering

### Landing page compilation

#### Kort sikt (MVP):
1. **Skapa grundläggande HTML/CSS template** med:
   - Responsiv grid-layout
   - Produktbeskrivning och bilder
   - Call-to-action knappar (Köp nu, Ladda ner)
   - Testimonials och reviews-sektion
   - FAQ-avsnitt

2. **Skapa compilation script** som:
   - Läser produktmetadata (JSON/YAML)
   - Renderar template med data
   - Genererar optimerad HTML/CSS/JS
   - Kopierar assets till output-mapp

#### Lång sikt:
- React/Next.js-baserat system
- A/B-testing av templates
- Analytics integration
- Personalisering baserat på användardata

### Produktmappstruktur

Rekommenderad struktur:
```
products/
├── product-slug/
│   ├── product.json              # Metadata
│   ├── content.md                # Huvudinnehåll (markdown)
│   ├── content_en.md             # Engelsk version
│   ├── assets/
│   │   ├── cover.png
│   │   ├── screenshots/
│   │   └── preview.mp4
│   ├── landing/
│   │   ├── template.html
│   │   ├── styles.css
│   │   └── script.js
│   ├── scripts/
│   │   └── build-local.js
│   └── output/
│       ├── product-slug.pdf
│       └── landing.html
└── templates/
    ├── pdf/
    │   ├── base.html
    │   └── styles.css
    └── landing/
        ├── base.html
        └── theme.css
```

### Byggsystem

Rekommendationer:
1. **Makefile** för enkla kommandon:
   ```makefile
   make pdf PRODUCT=ai-prompts
   make landing PRODUCT=ai-prompts
   make all PRODUCT=ai-prompts
   make deploy PRODUCT=ai-prompts
   ```

2. **npm scripts** för Node.js-baserade verktyg:
   ```json
   {
     "scripts": {
       "build:pdf": "node scripts/pdf-builder.js",
       "build:landing": "node scripts/landing-builder.js",
       "watch": "nodemon --watch products --ext md,json --exec npm run build"
     }
   }
   ```

3. **CI/CD pipeline** (GitHub Actions):
   - Automatisk bygg vid commit
   - Generera och publicera till CDN
   - Versionera output

## Prioriteringar

### P0 (Omedelbart):
1. Designa och implementera standardiserad produktmappstruktur
2. Skapa förbättrat PDF-generation script (Node.js + puppeteer)
3. Skapa grundläggande landing page template och compilation script
4. Testa på befintlig AI-prompts-bundle produkt

### P1 (Nästa iteration):
1. Lägg till stöd för flerspråkighet
2. Implementera avancerad styling och tema-stöd
3. Skapa batch-processing för alla produkter
4. Dokumentera API och användning

### P2 (Framtida):
1. React-baserade templates
2. A/B-testing system
3. Automatiserad deployment till hosting
4. Analytics och tracking integration

## Nästa steg

1. **Genomför steg 2-6 i planen** med följande fokus:
   - Använd rekommenderad produktmappstruktur
   - Implementera PDF-generation med puppeteer för bättre kontroll
   - Skapa landing page compilation med moderna CSS (Grid/Flexbox)

2. **Migrera befintliga produkter** till ny struktur:
   - AI-skribent prompts bundle som pilot
   - Behåll bakåtkompatibilitet med befintliga filer

3. **Skapa dokumentation**:
   - README med installationsanvisningar
   - Exempel på produktmetadata
   - Tutorial för att lägga till nya produkter

4. **Automatisera testing**:
   - Validera genererad PDF
   - Testa landing page i olika browsers
   - Mät performance och SEO

## Tekniska överväganden

### PDF-generation:
- **Puppeteer** vs **WeasyPrint**: Puppeteer ger bättre stöd för modern CSS, men kräver Chromium. WeasyPrint är lättare men har begränsad CSS-stöd.
- **Prestanda**: Batch-generering kan vara resurskrävande. Överväg caching och inkrementella byggen.

### Landing pages:
- **Statisk generering** vs **SSG**: Enkel HTML/CSS räcker för MVP. Överväg Next.js/React för framtida skalbarhet.
- **SEO**: Spara genererad HTML för bättre SEO. Använd semantisk markup och meta-taggar.

### Underhåll:
- **Konfiguration över kod**: Alla inställningar ska vara i JSON/YAML-filer, inte hårdkodade.
- **Plugin-arkitektur**: Gör det enkelt att lägga till nya template-typer och processors.

## Slutsats

Befintligt Python-skript är en bra start men behöver utökas med standardiserad struktur och landing page support. Rekommendationen är att bygga ett modulärt system med:

1. **Node.js-baserat PDF-generation** med puppeteer för bättre kontroll
2. **HTML/CSS template-system** för landing pages
3. **Enhetlig produktmappstruktur** för skalbarhet
4. **CLI-gränssnitt** för enkel användning

Genom att implementera dessa rekommendationer kommer du att ha ett robust byggsystem för digitala produkter som kan skala till 10+ produkter och generera passiv inkomst.

---

*Dokument genererat: 2026-03-19*
*Plan-ID: plan-7 - "Skapa build scripts för PDF-generation och landing page compilation för digitala produkter"*