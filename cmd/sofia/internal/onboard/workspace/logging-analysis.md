# Loggningsbehov för Affiliate Tracking och Stripe Checkout

## Affiliate Tracking Process

1. **Klickspårning**
   - När en användare klickar på en affiliate-länk
   - Data: affiliate_id, campaign_id, utm_source, utm_medium, utm_campaign, utm_content, utm_term, IP-adress, user agent, timestamp, referrer
   - Behov: Logga varje klick för senare attribuering

2. **Konverteringshändelse**
   - När en användare genomför ett köp (via Stripe webhook)
   - Data: order_id, customer_id, amount, currency, product_ids, affiliate_id (via cookie match), conversion_timestamp
   - Behov: Logga konvertering med alla attribut för provisionberäkning

3. **Provisionberäkning**
   - Beräkning av affiliate provision baserat på konvertering
   - Data: affiliate_id, conversion_id, commission_amount, payout_status, payout_date
   - Behov: Logga provisionhändelser för transparens och granskning

4. **Stripe Webhooks**
   - Inkommande webhooks från Stripe (checkout.session.completed, payment_intent.succeeded, etc.)
   - Data: event_id, event_type, payload (delvis), bearbetningsstatus, fel
   - Behov: Logga alla inkommande webhooks för felsökning och auditing

5. **Felhändelser**
   - Misslyckade betalningar, ogiltiga webhook-signaturer, databasfel
   - Data: error_message, stack_trace, context
   - Behov: Logga fel med tillräcklig kontext för felsökning

6. **Säkerhetshändelser**
   - Misstänkt aktivitet (flera klick från samma IP, brutna länkar, försök att manipulera cookies)
   - Data: event_type, IP, user agent, timestamp, severity
   - Behov: Logga för säkerhetsövervakning

## Nuvarande Loggningskapacitet

### Intern Logger (`pkg/logger`)
- JSON-baserad loggning till stdout/fil
- Nivåer: DEBUG, INFO, WARN, ERROR, FATAL, AUDIT
- Stöd för fält (key-value)
- Audit-logg separeras till separat fil
- Begränsad rotation/retention
- Ingen inbyggd sökning/aggregation

### Audit Logger (`pkg/audit`)
- SQLite-databas med strukturerade tabeller
- Indexering på timestamp, agent_id, action
- Stöder frågor med filter
- Bra för viktiga händelser men kanske inte för högvolym

### Begränsningar
1. **Skalbarhet**: SQLite kanske inte hanterar tusentals loggar per dag över lång tid
2. **Sökbarhet**: Ingen fulltextsökning utanför enkla SQL-frågor
3. **Retention**: Ingen automatisk rensning eller arkivering
4. **Centralisering**: Loggar är lokala per instans, ingen samling
5. **Visualisering**: Ingen dashboard för att analysera loggar i realtid

## Rekommendationer för Förbättring

### Kortsiktigt (enkel integration)
1. **Loggrotation** för filbaserade loggar (t.ex. med logrotate)
2. **Retention policy** för SQLite (ta bort gamla loggar efter X dagar)
3. **Utökad audit-logging** för affiliate-specifika händelser
4. **Strukturerade loggfält** för alla kritiska händelser

### Mellanlång sikt (centraliserad loggning)
1. **Loki integration** – skicka loggar till Grafana Loki för centraliserad sökning
2. **Elasticsearch** – för avancerad sökning och visualisering
3. **HTTP log exporter** – skicka loggar till extern tjänst via HTTP
4. **Dashboard** – webbaserad vy för konverteringsstatistik och loggar

### Långsiktigt (fullskalig lösning)
1. **Distribuerad loggningspipeline** med Kafka/Fluentd
2. **Real-tidsanalys** för anomalidetektering
3. **Automatiserade alerts** vid fel eller misstänkt aktivitet

## Prioritering för Affiliate Tracking

1. **Audit-loggar för alla kritiska händelser** (klick, konvertering, provision)
2. **Felhantering med strukturerade loggar** för snabb felsökning
3. **Retention** – behåll loggar minst 90 dagar för attribueringsperiod
4. **Sökbarhet** – möjlighet att hitta specifika order/affiliate händelser

## Nästa steg
- Designa loggschema för konverteringshändelser
- Utvärdera centraliserad loggningslösning (Loki vs Elasticsearch)
- Implementera loggrotation för befintliga loggar
- Skapa dashboard för att visa konverteringsstatistik