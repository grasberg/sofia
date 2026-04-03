# Sammanställning av funktioner och priser

## Produkter och Prisstruktur

### 1. Niche Selection Toolkit
*En omfattande verktygslåda för att identifiera lönsamma nischer, validera idéer och lyckas med lanseringar.*

| Nivå | Pris (engångs) | Funktioner |
|------|----------------|------------|
| **Starter** | $47 | - Niche Selection Checklist (PDF)<br>- Market Validation Worksheet<br>- 5 Niche Idea Templates<br>- Basic Competitor Analysis Guide<br>- Email support (72h response) |
| **Professional** | $97 | - Alla Starter-funktioner<br>- Advanced Niche Scoring Calculator (Excel/Google Sheets)<br>- Video Tutorial: "How to Validate a Niche in 48 Hours"<br>- 10 Additional Niche Templates<br>- SWOT Analysis Framework<br>- Community Access (Discord)<br>- Priority email support (24h response) |
| **Agency** | $297 | - Alla Professional-funktioner<br>- Whitelabel Rights (resell to clients)<br>- Custom Niche Research Template<br>- 1-hour Strategy Consultation Call<br>- Masterclass: "Scaling Niche Businesses"<br>- Lifetime Updates<br>- Direct Slack support |

**Noteringar:**
- Alla priser i USD
- Engångsbetalning, inget abonnemang
- Digital leverans (PDF, video, mallar)
- Uppdateringar ingår i 1 år (förutom Agency: livstid)

---

### 2. AI Skribent Prompts – 5 Grundläggande Prompts för Kreativt Skrivande
*Professionellt designade AI-prompts för författare, copywriters och kreativa skribenter.*

| Pris (SEK) | Funktioner |
|------------|------------|
| **79 SEK** (lanseringsrabatt) | - 5 professionella prompts för kreativt skrivande:<br>  1. Berättelsegeneratorn<br>  2. Karaktärsutvecklaren<br>  3. Världsbyggaren<br>  4. Dialogtränaren<br>  5. Skrivkampslösaren<br>- Fungerar med ChatGPT, Claude, Copilot m.fl.<br>- Livstidsuppdateringar<br>- 30-dagars pengarna-tillbaka-garanti<br>- Personligt + kommersiellt bruk tillåtet |
| **99 SEK** (ordinarie pris) | Samma som ovan |

**Bonusmaterial:**
- 3 avancerade bonusprompts:
  - Plot Twist Generator
  - Känsloregissören
  - Genre-Blender
- Prompt Optimizer

---

## Översikt för Stripe Integration

### Produkter att skapa i Stripe

1. **Niche Selection Toolkit** – tre olika prisnivåer (Products + Prices)
   - Product: "Niche Selection Toolkit"
     - Price: $47 (one-time)
     - Price: $97 (one-time)
     - Price: $297 (one-time)
   - *Alternativt: tre separata produkter för varje nivå*

2. **AI Skribent Prompts** – en produkt med två priser (rabatterat och ordinarie)
   - Product: "AI Skribent Prompts – 5 Grundläggande Prompts för Kreativt Skrivande"
     - Price: 79 SEK (one-time)
     - Price: 99 SEK (one-time)
   - *Obs: Stripe stöder SEK som valuta*

### Ytterligare överväganden
- **Valuta:** Stripe stöder både USD och SEK
- **Engångsbetalningar:** Alla produkter är one-time payments (inte recurring)
- **Leverans:** Digitala produkter levereras via e-post/länk efter köp
- **Support:** Inkluderad enligt specifikationerna

---

## Nästa steg för Stripe Integration

1. Skapa produkter i Stripe Dashboard eller via API med ovanstående priser
2. Konfigurera checkout-sessioner med rätt Price IDs
3. Testa med Stripe testkort:
   - USD: `4242 4242 4242 4242`
   - SEK: `4000 0000 0000 3220`
4. Sätta upp webhooks för betalningsbekräftelse och leverans

---

*Uppdaterad: 2026-03-19*