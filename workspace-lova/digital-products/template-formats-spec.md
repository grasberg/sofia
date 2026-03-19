# Format-specifikation för Niche Selection Toolkit-mallar

## Översikt
Detta dokument definierar format, struktur och leveransmetoder för alla mallar i Niche Selection Toolkit. Varje mall är kopplad till specifika tiers (Starter, Pro, Ultimate) och levereras i lämpligt format för användarens arbetsflöde.

## Mallar per Tier

### Tier 1: Starter
1. **Grundläggande valideringschecklista** - PDF
2. **Enkel marknadsanalysmall** - Excel/Google Sheets
3. **Nischidé-brainstorming guide** - PDF
4. **Konkurrentanalysmall (basic)** - Excel/Google Sheets
5. **Prisanalysmall för digitala produkter** - Excel/Google Sheets

### Tier 2: Pro (inkluderar Starter PLUS)
6. **Avancerad marknadsanalysmall** - Excel/Google Sheets med automatiseringsinstruktioner
7. **AI-driven nischidentifiering via ChatGPT-prompts bibliotek** - PDF/Text-fil
8. **Nischdatabas med 50+ förvaliderade nischer** - Airtable/GSheet/CSV
9. **Notion template för hela nischvalideringsprocessen** - Notion
10. **Video-guider** - Video (MP4)

### Tier 3: Ultimate (inkluderar Pro PLUS)
11. **Personlig nischrekommendation** - PDF (anpassad)
12. **VIP-community åtkomst** - Discord/Slack
13. **1:1 konsultation** - Zoom/Live-möte

## Format-specifikation per malltyp

### 1. PDF-format
**Användning:** Checklistor, guider, rapporter
**Formatkrav:**
- Filtyp: PDF/A-1b för långtidsarkivering
- Sidstorlek: A4 (210 × 297 mm)
- Marginaler: 2 cm alla sidor
- Teckensnitt: Inter eller Open Sans, 11pt brödtext
- Färger: Svart (#000000) för text, accentfärg #3B82F6 (blå) för rubriker
- Interaktivitet: Klickbara länkar och innehållsförteckning
- Metadata: Titel, författare, nyckelord, skapelsedatum
**Leverans:** Direkt nedladdning från betalningsportal

### 2. Excel/Google Sheets-format
**Användning:** Analysmallar, kalkylering, datainsamling
**Formatkrav:**
- **Excel-version:** .xlsx (Excel 2016+)
- **Google Sheets-version:** Delbar länk med "Visa"-åtkomst, möjlighet att skapa kopia
- **Struktur:** 
  - Varje mall har separata ark för olika steg (t.ex. "Marknadsanalys", "Konkurrenter", "Priser")
  - Fördefinierade kolumner med validering (dropdowns, datum, nummer)
  - Formler för automatisk beräkning (t.ex. ROI, break-even)
  - Konditionell formatering för visuell feedback
  - Exempeldata som kan raderas
- **Makron:** Ingen användning av makron för kompatibilitet
**Leverans:** Excel-fil + Google Sheets-länk

### 3. Notion Template
**Användning:** Holistisk processhantering för nischvalidering
**Formatkrav:**
- **Struktur:** Workspace med följande sidor:
  1. Dashboard (översikt över processen)
  2. Nischidéer (databassida med properties)
  3. Marknadsanalys (mall för research)
  4. Konkurrentanalys (databassida)
  5. Valideringschecklista
  6. Tidslinje och milstolpar
- **Properties:**
  - Status (Inte påbörjad, Pågående, Avslutad)
  - Prioritet (Hög, Medel, Låg)
  - Nischtema
  - Lönsamhetsskala (1-10)
  - Deadline (datum)
- **Vyer:** Tabell, Kanban, Kalender, Galleri
- **Templates inom template:** "Ny nischidé", "Ny konkurrentanalys"
**Leverans:** Delbar Notion-länk med "Duplicera"-funktion

### 4. Airtable/Databas-format
**Användning:** Nischdatabas med 50+ förvaliderade nischer
**Formatkrav:**
- **Plattform:** Airtable (eller alternativt Google Sheets)
- **Struktur:**
  - Tabell: "Nischer"
  - Fält:
    - Nischnamn (text)
    - Beskrivning (långtext)
    - Konkurrensnivå (1-5)
    - Lönsamhetspotential (1-5)
    - Startkostnad (låg/medel/hög)
    - Keyword ideas (text, komma-separerad)
    - Affiliate programs (länkar)
    - Traffic potential (text)
    - Rekommenderad produkttyp
    - Valideringsstatus (Validerad, Testad, Teoretisk)
  - Vyer: "Bästa för nybörjare", "Högsta lönsamhet", "Låg konkurrens"
- **Exportformat:** CSV, JSON
**Leverans:** Airtable-delningslänk med läs-/skrivrättighet

### 5. Video-format
**Användning:** Instruktionsvideor, guider
**Formatkrav:**
- **Upplösning:** 1080p (1920×1080)
- **Format:** MP4 (H.264 codec, AAC ljud)
- **Längd:** 5-15 minuter per video
- **Struktur:**
  - Intro (vad video täcker)
  - Steg-för-steg-demonstration
  - Sammanfattning
- **Ljud:** Professionell inspelning, inga bakgrundsljud
- **Textning:** Inbyggda undertexter (SRT-fil separat)
**Leverans:** Privat YouTube-länk eller nedladdningsbar ZIP

### 6. Community-åtkomst
**Användning:** Support, nätverkande
**Formatkrav:**
- **Plattform:** Discord (primärt), Slack som alternativ
- **Struktur:**
  - Kanaler: #general, #support, #success-stories, #collaboration
  - Roller: Starter, Pro, Ultimate (olika åtkomstnivåer)
  - Botten: Welcome bot, FAQ bot
- **Regler:** Uppförandekod tydligt definierad
**Leverans:** Inbjudningslänk per e-post efter köp

## Tekniska krav

### Filstorlekar
- PDF: Max 5 MB per fil
- Excel: Max 10 MB
- Video: Max 500 MB per video (komprimerad)

### Kompatibilitet
- **Excel:** Kompatibel med Excel 2016+, Google Sheets, LibreOffice 7+
- **PDF:** Kompatibel med Adobe Acrobat Reader 11+, modern webbläsare
- **Notion:** Kräver Notion-konto (gratis nivå räcker)

### Säkerhet
- Ingen personlig information i mallarna
- Alla länkar ska vara HTTPS
- Ingen malware eller tracking

## Produktionsprocess
1. Designa varje mall i råformat (Figma för PDF, Excel för kalkyl, Notion för template)
2. Testa med användare från målgruppen
3. Exportera till slutliga format
4. Kvalitetssäkra (kontrollera länkar, funktionalitet)
5. Paketera i ZIP-filer per tier

## Nästa steg
- Skapa design-mockups för varje mall
- Utveckla Excel/Google Sheets-mallar med exempeldata
- Bygga Notion-template i test-arbetsyta
- Producera videoguider

---
*Dokument skapat: 2026-03-19*  
*Status: Utkast för granskning*