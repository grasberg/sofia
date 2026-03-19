# Vercel Analytics: Setup och Prisstruktur

## Översikt

Vercel Analytics (Web Analytics) är ett förstaparts, integrerat analysverktyg som ger realtidsinsikter om webbplatsbesökare direkt i Vercel-plattformen. Det kräver ingen extern tjänst eller tredjepartsintegration.

## Huvudfunktioner

### 1. Automatisk spårning
- **Page Views**: Automatisk spårning av sidvisningar
- **Custom Events**: Anpassade händelser för knapptryckningar, formulärinskickningar, etc.
- **Besökarspårning**: Unika besökare, sessioner, återkommande besökare

### 2. Analysrapporter
- **Realtidsdashboard**: Live-besöksdata
- **Källa och medium**: UTM-parameterstöd (endast i Plus/Enterprise)
- **Geografisk data**: Besökarlocation
- **Enhet och webbläsare**: Teknisk information
- **Sidprestanda**: Laddningstider, Core Web Vitals

### 3. Integrering
- **@vercel/analytics** npm-paket för enkel integration
- **Zero-config** för Next.js och andra Vercel-optimiserade ramverk
- **Script-tagg** för valfri webbplats

## Prisstruktur (per team)

### Planjämförelse

| Funktion | Hobby | Pro | Pro med Web Analytics Plus | Enterprise |
|----------|-------|-----|----------------------------|------------|
| **Inkluderade events/månad** | 50 000 | N/A | N/A | Anpassad |
| **Extra events** | Ej tillgängligt | $3 / 100 000 events | $3 / 100 000 events | Anpassad |
| **Inkluderade projekt** | Obegränsat | Obegränsat | Obegränsat | Obegränsat |
| **Rapporteringsfönster** | 1 månad | 12 månader | 24 månader | 24 månader |
| **Custom Events** | ❌ | ✅ | ✅ | ✅ |
| **Properties per Custom Event** | - | 2 | 8 | 8 |
| **UTM-parametrar** | ❌ | ❌ | ✅ | ✅ |
| **Pris per event** | - | $0.00003 | $0.00003 | Anpassad |

### Detaljerad kostnadsförklaring

#### Hobby Plan
- **Gratis** upp till 50 000 events/månad
- Ingen debitering vid överskridande - insamling pausas efter 3 dagars grace period
- Möjlighet att vänta 7 dagar för återupptagning eller uppgradera till Pro
- Endast automatiska page views, inga custom events

#### Pro Plan
- **Ingen inkluderad mängd** - alla events debiteras
- **$3 per 100 000 events** (prorated)
- **$0.00003 per event**
- Tillgång till custom events (max 2 properties per event)
- 12 månaders rapporteringsfönster
- Spend Management: notifieringar och automatiska åtgärder vid budgetöverskridande

#### Web Analytics Plus (Tilläggspaket)
- **$10/månad extra per team**
- Uppgraderar Pro-planen med:
  - 24 månaders rapporteringsfönster
  - UTM-parameterstöd
  - 8 properties per custom event (istället för 2)
- Alla team-projekt får tillgång till förbättringarna

#### Enterprise Plan
- Anpassad prissättning baserad på volym och behov
- 24 månaders rapporteringsfönster
- Fullt UTM-stöd
- 8 properties per custom event
- Dedikerad support och SLA

## Setup-installation

### Steg 1: Aktivera Web Analytics

1. Gå till Vercel Dashboard → Project → Analytics
2. Klicka på "Enable Web Analytics"
3. Välj vilken domän som ska spåras

### Steg 2: Installera Analytics Script

#### För Next.js (App Router)
```bash
npm install @vercel/analytics
```

```typescript
// app/layout.tsx
import { Analytics } from '@vercel/analytics/react';

export default function RootLayout({ children }) {
  return (
    <html lang="sv">
      <body>
        {children}
        <Analytics />
      </body>
    </html>
  );
}
```

#### För Next.js (Pages Router)
```typescript
// pages/_app.tsx
import { Analytics } from '@vercel/analytics/react';

function MyApp({ Component, pageProps }) {
  return (
    <>
      <Component {...pageProps} />
      <Analytics />
    </>
  );
}
```

#### För andra ramverk (vanlig JavaScript)
```html
<script defer src="/_vercel/insights/script.js"></script>
```

### Steg 3: Konfigurera Custom Events

```javascript
// Exempel på custom event
import { track } from '@vercel/analytics';

// Spåra knapptryckning
button.addEventListener('click', () => {
  track('Purchase', { product: 'Premium Plan', price: 99 });
});

// Spåra formulärinskickning
form.addEventListener('submit', () => {
  track('Lead Generated', { source: 'landing-page' });
});
```

## Usage och Optimering

### Events-definition
- **Page View**: Varje sidvisning räknas som 1 event
- **Custom Event**: Varje anrop till `track()` räknas som 1 event
- **Events delas** över alla projekt i teamet

### Kostnadsoptimering
1. **Filtrera bot-trafik**: Aktivera Bot Management för att reducera onödig spårning
2. **Använd sampling**: För högvolymssajter, implementera sampling i koden
3. **Välj rätt plan**: 
   - < 50k events/månad: Hobby
   - 50k-500k events/månad: Pro
   - > 500k events/månad: Pro med Plus eller Enterprise
4. **Monitorera usage**: Använd Vercel Dashboard → Usage → Web Analytics

### Grace Periods och Pausering
- **Hobby**: 3 dagars grace period vid överskridande, sedan paus tills nästa cykel
- **Pro**: Ingen paus, debitering fortsätter
- **Pausade projekt**: Inga events samlas in, data blir tillgänglig vid uppgradering

## Vanliga frågor (FAQ)

### Vad är ett "event"?
Ett event är antingen en automatiskt spårad sidvisning eller ett custom event som du definierar. En sidvisning spåras automatiskt när en besökare laddar en sida med analytics-skriptet.

### Delas usage mellan projekt?
Ja, alla events från alla projekt under samma Vercel-team räknas samman mot din totala gräns eller debitering.

### Vad är rapporteringsfönstret?
Den period som din analysdata garanterat lagras och är tillgänglig för analys. Data kan behållas längre, men endast rapporteringsfönstret är garanterat.

### Kan jag migrera från Hobby till Pro utan dataförlust?
Ja, Vercel behåller ofta data längre än rapporteringsfönstret för att möjliggöra uppgradering utan förlust.

### Stöds UTM-parametrar?
Endast i Web Analytics Plus och Enterprise-planerna.

## Rekommendationer

### För små projekt/sidstarter
- **Hobby-planen** räcker för de flesta
- Max 50k events/månad = ~1 667 besök/dag (om varje besök genererar 3 events)
- Överväg Pro om du behöver custom events eller längre rapportering

### För e-handel och SaaS
- **Pro med Web Analytics Plus** rekommenderas
- UTM-stöd för kampanjspårning
- Längre rapporteringsfönster för trendanalys
- Fler properties för detaljerad spårning

### För enterprise och högvolym
- **Enterprise-plan** för förhandlade priser och dedikerat stöd
- 24 månaders rapporteringsfönster för långsiktig analys
- Full kontroll över data och integritet

## Nästa steg

1. **Aktivera Web Analytics** i ditt Vercel Dashboard
2. **Installera @vercel/analytics** i ditt projekt
3. **Konfigurera custom events** för viktiga användaråtgärder
4. **Monitorera usage** första månaden för att förstå din event-volym
5. **Justera plan** baserat på faktisk användning

---

*Senast uppdaterad: Mars 2026*  
*Källa: [Vercel Analytics Documentation](https://vercel.com/docs/analytics/limits-and-pricing)*