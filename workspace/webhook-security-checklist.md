# Webhook Security Checklist

## Översikt
Denna checklista täcker kritiska säkerhetsåtgärder för implementation av webhooks i alla typer av system (Stripe, PayPal, GitHub, Slack, etc.). Webhooks är sårbara för attacker om de inte implementeras korrekt.

## Grundläggande Säkerhetsprinciper

### 1. Signaturvalidering
- [ ] **Implementera signaturverifiering** för alla inkommande webhook-anrop
- [ ] **Använd HMAC eller asymmetrisk kryptografi** (t.ex. Stripe-Signature, X-Hub-Signature-256)
- [ ] **Validera tidsstämpel** för att förhindra replay-attacker (typiskt inom 5 minuter)
- [ ] **Säkra hemligheten** (webhook secret) i miljövariabler, inte i kod
- [ ] **Testa signaturvalidering** med ogiltiga signaturer

### 2. Endpoint-skydd
- [ ] **Använd HTTPS** (TLS 1.2+) för alla webhook-endpoints
- [ ] **Implementera rate limiting** för att förhindra DoS-attacker
- [ ] **Validera Content-Type** (application/json eller application/x-www-form-urlencoded)
- [ ] **Begränsa HTTP-metoder** till endast POST (eller vad som krävs)
- [ ] **Använd API gateway/WAF** för ytterligare skydd

### 3. Datavalidering och Sanering
- [ ] **Validera inkommande JSON-schema** innan bearbetning
- [ ] **Sanera indata** för att förhindra injection-attacker
- [ ] **Validera obligatoriska fält** och datatyper
- [ ] **Begränsa storleken på payload** (t.ex. max 1 MB)
- [ ] **Kontrollera att event-typen är tillåten** innan bearbetning

### 4. Idempotens och Duplikathantering
- [ ] **Implementera idempotensnyckel** (t.ex. webhook event ID)
- [ ] **Logga alla mottagna events** med deras ID:n
- [ ] **Kontrollera duplikat** innan bearbetning
- [ ] **Hantera retry-logik** korrekt (inte infinite loops)
- [ ] **Använd idempotency keys** för kritiska operationer

### 5. Felsäkring och Felhantering
- [ ] **Returnera korrekta HTTP-statuskoder** (200 för success, 4xx för klientfel, 5xx för serverfel)
- [ ] **Logga alla fel** utan att exponera känslig information
- [ ] **Implementera timeout** för webhook-bearbetning (t.ex. 30 sekunder)
- [ ] **Hantera exceptions** utan att krascha applikationen
- [ ] **Skicka inte detaljerade felmeddelanden** till avsändaren

### 6. Hemlighetshantering
- [ ] **Lagra webhook secrets** i säkra lager (AWS Secrets Manager, HashiCorp Vault, miljövariabler)
- [ ] **Roter secrets regelbundet** (var 90 dagar)
- [ ] **Använd olika secrets** för olika miljöer (dev, staging, prod)
- [ ] **Begränsa åtkomst** till secrets baserat på principen om minsta behörighet
- [ ] **Auditera åtkomst** till secrets

### 7. Loggning och Övervakning
- [ ] **Logga alla webhook-anrop** (timestamp, event ID, status, bearbetningstid)
- [ ] **Implementera alarms** för misstänkt aktivitet (många fel, ovanliga payloads)
- [ ] **Övervaka bearbetningstider** för att upptäcka prestandaproblem
- [ ] **Spara logs säkert** med retention policy
- [ ] **Skapa dashboards** för webhook-hälsa

### 8. Testning och Verifiering
- [ ] **Testa med giltiga och ogiltiga signaturer**
- [ ] **Testa med korrupta payloads** (felaktig JSON, för stor payload)
- [ ] **Testa rate limiting** genom att skicka många requests
- [ ] **Testa idempotens** genom att skicka samma event flera gånger
- [ ] **Testa felhantering** genom att simulera fel i bearbetningslogik

### 9. Dokumentation och Incidenthantering
- [ ] **Dokumentera alla webhook-events** och deras payload-strukturer
- [ ] **Dokumentera signaturberäkning** och valideringsprocess
- [ ] **Skapa runbook** för incidenthantering (webhook-avbrott, komprometterade secrets)
- [ ] **Definiera SLA/SLO** för webhook-tillgänglighet
- [ ] **Dokumentera rollback-procedurer**

### 10. Plattformspecifika Säkerhetsåtgärder

#### Stripe
- [ ] Använd `Stripe-Signature` header med tidsstämpel och signaturer
- [ ] Validera med `construct_event()` eller `Webhook.constructEvent()`
- [ ] Använd olika webhook endpoints för test och production
- [ ] Konfigurera webhooks i Stripe Dashboard med korrekta events
- [ ] Använd Stripe CLI för lokal testning

#### PayPal
- [ ] Validera med `PayPal-Auth-Algo`, `PayPal-Cert-Url`, `PayPal-Transmission-Id`, `PayPal-Transmission-Sig`, `PayPal-Transmission-Time`
- [ ] Verifiera certifikatet från `PayPal-Cert-Url`
- [ ] Använd officiella SDK:er för validering

#### GitHub
- [ ] Använd `X-Hub-Signature-256` header
- [ ] Beräkna HMAC hex digest med SHA256
- [ ] Jämför signaturer med `crypto.timingSafeEqual()`

#### Slack
- [ ] Validera `X-Slack-Signature` och `X-Slack-Request-Timestamp`
- [ ] Beräkna bassträng som `v0:${timestamp}:${body}`
- [ ] Använd HMAC med SHA256

## Implementationssteg

### Steg 1: Grundläggande Säkerhet
1. Konfigurera HTTPS för endpoint
2. Implementera signaturvalidering
3. Spara webhook secret i miljövariabler
4. Skapa grundläggande loggning

### Steg 2: Avancerade Åtgärder
1. Lägg till rate limiting
2. Implementera idempotenshantering
3. Skapa felhantering och alarms
4. Dokumentera allt

### Steg 3: Kontinuerlig Förbättring
1. Regelbundna säkerhetsgranskningar
2. Rotera secrets
3. Uppdatera dokumentation
4. Testa med penetration testing

## Verifieringskommandon

### Generella tester
```bash
# Testa signaturvalidering med ogiltig signatur
curl -X POST https://api.example.com/webhook \
  -H "Content-Type: application/json" \
  -H "X-Signature: invalid_signature" \
  -d '{"event":"test"}'

# Testa rate limiting
for i in {1..100}; do
  curl -X POST https://api.example.com/webhook \
    -H "Content-Type: application/json" \
    -d '{"event":"test"}'
done

# Testa stora payloads
curl -X POST https://api.example.com/webhook \
  -H "Content-Type: application/json" \
  -d "$(dd if=/dev/urandom bs=2M count=1 | base64)"
```

### Stripe-specifika
```bash
# Testa med Stripe CLI
stripe listen --forward-to localhost:3000/webhook
stripe trigger payment_intent.succeeded
```

## Checklista för Code Review

### Kodgranskning
- [ ] Signaturvalidering implementerad korrekt
- [ ] Inga hårdkodade secrets
- [ ] Korrekt felhantering
- [ ] Loggning utan känslig data
- [ ] Idempotenshantering för kritiska operationer

### Säkerhetsgranskning
- [ ] Inga SQL/NoSQL injection risker
- [ ] Inga XXE eller deserialization risker
- [ ] Korrekt auktorisering (om applicerbart)
- [ ] Rate limiting implementerad
- [ ] TLS korrekt konfigurerad

## Incident Response Checklist

### Vid misstänkt kompromettering
1. [ ] Rotera alla webhook secrets omedelbart
2. [ ] Granska loggar för misstänkt aktivitet
3. [ ] Uppdatera webhook endpoints i provider dashboard
4. [ ] Meddela berörda parter
5. [ ] Dokumentera incidenten och åtgärder

### Vid webhook-avbrott
1. [ ] Kontrollera serverloggar för fel
2. [ ] Verifiera att secrets fortfarande är giltiga
3. [ ] Testa med testevents från provider
4. [ ] Kontrollera rate limiting inställningar
5. [ ] Övervaka återhämtning

## Ytterligare Resurser

### Läsning
- [OWASP Webhook Security Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Webhook_Security_Cheat_Sheet.html)
- [Stripe Webhook Best Practices](https://stripe.com/docs/webhooks/best-practices)
- [GitHub Webhook Security](https://docs.github.com/en/webhooks/using-webhooks/securing-your-webhooks)

### Verktyg
- **Stripe CLI**: Testa webhooks lokalt
- **ngrok**: Exponera lokala endpoints för testning
- **Burp Suite**: Testa säkerhet med penetration testing
- **OWASP ZAP**: Automatiserade säkerhetstester

---

*Senast uppdaterad: 2026-03-19*  
*Version: 1.0*  
*Använd denna checklista för alla webhook-implementationer och uppdatera den kontinuerligt.*