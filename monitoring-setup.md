# Monitoring Setup Plan

## Overview
Implementera monitoring för Stripe webhook-endpoint (Node.js på Render.com Free Plan) med gratisverktyg: Grafana Cloud för metrics/logs, Better Uptime för health checks.

## Project Type
WEB (Node.js backend)

## Success Criteria
- [ ] Health checks på `/health` endpoint med 1-minuts intervall
- [ ] Node.js metrics (CPU, minne, request rate, error rate) synliga i Grafana dashboard
- [ ] Loggar från Stripe webhook-förfrågningar samlas in och kan sökas
- [ ] Aviseringar via email/Slack vid:
  - HTTP status 5xx eller 4xx rate >5%
  - CPU >80% i 5 minuter
  - Endpoint nere i >2 minuter
- [ ] Alla komponenter gratis eller inom gratis-tier gränser

## Tech Stack
- **Metrics & Logs:** Grafana Cloud Free (Prometheus, Loki)
- **APM:** OpenTelemetry för Node.js
- **Uptime:** Better Uptime (3 monitors gratis)
- **Alerting:** Grafana Alerting + Better Uptime
- **Instrumentation:** `@opentelemetry/sdk-node`, `prom-client`, `winston-loki`

## File Structure
```
stripe-server/
├── src/
│   ├── instrumentation.js      # OpenTelemetry setup
│   ├── logger.js               # Winston + Loki config
│   └── metrics.js              # Prometheus metrics exporter
├── .env.example                # Miljövariabler för Grafana Cloud
├── package.json                # Lägg till dependencies
└── README-monitoring.md        # Instruktioner för underhåll

workspace/monitoring-setup/
├── grafana/                    # Grafana dashboards (export)
│   └── stripe-webhook-dashboard.json
├── alert-rules/                # Grafana alert rules
│   └── webhook-alerts.yml
└── docs/
    └── better-uptime-setup.md
```

## Task Breakdown

### Task 1: Setup Grafana Cloud konto och skapa API-nycklar
- **Agent:** backend-specialist
- **Skills:** clean-code
- **Priority:** P0
- **Dependencies:** None
- **INPUT:** Grafana Cloud registrering
- **OUTPUT:** Prometheus remote write URL, Loki endpoint, API keys i `.env`
- **VERIFY:** Kan `curl` mot endpoints med autentisering

### Task 2: Instrumentera Node.js app med OpenTelemetry
- **Agent:** backend-specialist  
- **Skills:** clean-code
- **Priority:** P1
- **Dependencies:** Task 1
- **INPUT:** Befintlig stripe-server kod
- **OUTPUT:** `instrumentation.js` som initierar OpenTelemetry och exporterar till Grafana Cloud Prometheus
- **VERIFY:** Metrics visas i Grafana Cloud Prometheus (t.ex. `nodejs_heap_space_size_used_bytes`)

### Task 3: Konfigurera loggning med Winston + Loki
- **Agent:** backend-specialist
- **Skills:** clean-code
- **Priority:** P1
- **Dependencies:** Task 1
- **INPUT:** Stripe webhook loggar console.log
- **OUTPUT:** `logger.js` med Winston transporter: console + Loki
- **VERIFY:** Loggar syns i Grafana Cloud Loki när webhook anropas

### Task 4: Skapa health endpoint och Better Uptime monitor
- **Agent:** backend-specialist
- **Skills:** clean-code
- **Priority:** P1
- **Dependencies:** None
- **INPUT:** Node.js Express app
- **OUTPUT:** `GET /health` endpoint som returnerar 200 + timestamp
- **VERIFY:** Better Uptime monitor visar "UP" och svarstid

### Task 5: Bygg Grafana dashboard för webhook metrics
- **Agent:** frontend-specialist
- **Skills:** plan-writing
- **Priority:** P2
- **Dependencies:** Task 2, Task 3
- **INPUT:** Metrics och loggar i Grafana Cloud
- **OUTPUT:** Grafana dashboard med 4 paneler: CPU/minne, request rate, error rate, senaste loggar
- **VERIFY:** Dashboard kan visas och data uppdateras i realtid

### Task 6: Konfigurera aviseringar (Grafana Alerting)
- **Agent:** backend-specialist
- **Skills:** plan-writing
- **Priority:** P2
- **Dependencies:** Task 5
- **INPUT:** Alert regler krav
- **OUTPUT:** Grafana alert rules som skickar till email/Slack
- **VERIFY:** Testalert triggas manuellt och skickas till testkanal

### Task 7: Deploy uppdaterad stripe-server till Render
- **Agent:** devops-engineer
- **Skills:** app-builder
- **Priority:** P3
- **Dependencies:** Task 2,3,4
- **INPUT:** Instrumenterad kod
- **OUTPUT:** Ny deployment på Render.com med miljövariabler för Grafana Cloud
- **VERIFY:** Appen körs och skickar metrics/loggar till Grafana Cloud

### Task 8: Dokumentation och underhållsguide
- **Agent:** project-planner
- **Skills:** plan-writing
- **Priority:** P3
- **Dependencies:** Task 7
- **INPUT:** Alla konfigurationer
- **OUTPUT:** `README-monitoring.md` med setup, felsökning, uppgraderingsväg
- **VERIFY:** Dokumentationen finns i stripe-server/ och är läsbar

## Phase X: Verification

### 1. Run Security Scan
```bash
python .agent/skills/vulnerability-scanner/scripts/security_scan.py .
```

### 2. Build Verification
```bash
cd stripe-server && npm run build
```

### 3. Runtime Test
- Starta appen lokalt: `npm start`
- Skicka test webhook: `curl -X POST http://localhost:3000/webhook`
- Verifiera att metrics och loggar hamnar i Grafana Cloud

### 4. Alert Test
- Simulera hög CPU med stress test
- Verifiera att alert triggas och skickas

### 5. Uptime Verification
- Stäng av appen lokalt
- Verifiera att Better Uptime visar "DOWN" inom 2 minuter

### 6. Completion Marker
- [ ] Alla tasks markerade completed
- [ ] Dashboard visar data
- [ ] Alerting fungerar
- [ ] Dokumentation klar

---
*Plan skapad: 2026-03-19*  
*Budget: 0 kr (gratis tier)*  
*Beräknad tid: 4–6 timmar*