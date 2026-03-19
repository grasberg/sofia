# Stripe Produkter API & Dashboard Plan

## Overview
Skapa ett system för att hantera Stripe-produkter via ett admin-dashboard och REST API. Systemet ska tillåta användare att skapa, uppdatera, lista och ta bort produkter i Stripe direkt från en webbaserad gränssnitt eller via API-anrop. Detta är viktigt för att automatisera produktlivscykeln i en e-handels- eller SaaS-plattform.

## Project Type
**WEB** (Dashboard) + **BACKEND** (API)

## Success Criteria
- [ ] Dashboard med gränssnitt för att skapa Stripe-produkter (namn, beskrivning, priser, metadata)
- [ ] REST API för programmatisk skapande av produkter
- [ ] Integration med Stripe via officiellt SDK
- [ ] Autentisering och auktorisering för dashboard-åtkomst
- [ ] Validering av indata före Stripe-anrop
- [ ] Felhantering och användarvänliga meddelanden
- [ ] Dokumentation för API-användning

## Tech Stack
- **Backend:** Go (golang) med Stripe Go SDK
- **Frontend:** React med TypeScript (eller enkelt HTML/JS om tid är begränsat)
- **Database:** SQLite (för användardata, eventuellt cache)
- **Authentication:** JWT eller session-baserad auth
- **Styling:** Tailwind CSS för enkel styling
- **Deployment:** Docker eller direkt på server

## File Structure
```
workspace/projects/stripe-products/
├── backend/
│   ├── cmd/
│   │   └── server/
│   │       └── main.go
│   ├── internal/
│   │   ├── handlers/
│   │   │   └── products.go
│   │   ├── middleware/
│   │   │   └── auth.go
│   │   ├── models/
│   │   │   └── product.go
│   │   └── stripe/
│   │       └── client.go
│   ├── pkg/
│   │   └── utils/
│   └── go.mod
├── frontend/
│   ├── public/
│   ├── src/
│   │   ├── components/
│   │   │   ├── ProductForm.tsx
│   │   │   └── ProductList.tsx
│   │   ├── pages/
│   │   │   └── Dashboard.tsx
│   │   ├── services/
│   │   │   └── api.ts
│   │   └── App.tsx
│   ├── package.json
│   └── tsconfig.json
├── docker-compose.yml
├── README.md
└── .env.example
```

## Task Breakdown

### Task 1: Sätta upp Stripe-konto och API-nycklar
**Agent:** backend-specialist  
**Skills:** clean-code, system-configuration  
**Priority:** P0  
**Dependencies:** None  
**INPUT:** Inget Stripe-konto konfigurerat för projektet  
**OUTPUT:** Stripe test-konto med API-nycklar, .env fil med STRIPE_SECRET_KEY  
**VERIFY:** Stripe CLI kan autentiseras, nycklar valideras via enkelt API-anrop

### Task 2: Installera Stripe Go SDK
**Agent:** backend-specialist  
**Skills:** clean-code, dependency-management  
**Priority:** P0  
**Dependencies:** Task 1  
**INPUT:** Go-projekt utan Stripe SDK  
**OUTPUT:** stripe-go SDK tillagd i go.mod, importerad i koden  
**VERIFY:** `go mod tidy` lyckas, koden kompilerar utan fel

### Task 3: Skapa Stripe-klient wrapper
**Agent:** backend-specialist  
**Skills:** clean-code, service-pattern  
**Priority:** P0  
**Dependencies:** Task 2  
**INPUT:** Ingen abstraktion för Stripe-anrop  
**OUTPUT:** internal/stripe/client.go med metoderna CreateProduct, UpdateProduct, ListProducts, DeleteProduct  
**VERIFY:** Klassen kan instansieras med API-nyckel, kan göra testanrop till Stripe

### Task 4: Designa produktmodell och validering
**Agent:** backend-specialist  
**Skills:** clean-code, data-modeling  
**Priority:** P0  
**Dependencies:** Task 3  
**INPUT:** Ingen datastruktur för produktattribut  
**OUTPUT:** internal/models/product.go med struct och valideringsfunktioner  
**VERIFY:** Struct kan serialiseras till JSON, validering returnerar fel för ogiltig data

### Task 5: Skapa REST API-endpoints
**Agent:** backend-specialist  
**Skills:** clean-code, api-design  
**Priority:** P1  
**Dependencies:** Task 4  
**INPUT:** Ingen HTTP-server för produktoperationer  
**OUTPUT:** internal/handlers/products.go med GET /products, POST /products, PUT /products/:id, DELETE /products/:id  
**VERIFY:** Endpoints returnerar korrekta HTTP-statuskoder, använder Stripe-klient

### Task 6: Implementera autentisering middleware
**Agent:** security-auditor  
**Skills:** clean-code, security  
**Priority:** P0  
**Dependencies:** Task 5  
**INPUT:** API är öppet för alla  
**OUTPUT:** internal/middleware/auth.go som verifierar API-nyckel eller JWT  
**VERIFY:** Oautentiserade anrop får 401, autentiserade anrop passerar

### Task 7: Bygg enkel frontend dashboard
**Agent:** frontend-specialist  
**Skills:** clean-code, react-patterns  
**Priority:** P1  
**Dependencies:** Task 5  
**INPUT:** Ingen användargränssnitt  
**OUTPUT:** React-app med formulär för att skapa produkt, lista över produkter  
**VERIFY:** Dashboard kan lägga till produkt via API, visa lista, hantera fel

### Task 8: Styla dashboard med Tailwind CSS
**Agent:** frontend-specialist  
**Skills:** clean-code, ui-design  
**Priority:** P2  
**Dependencies:** Task 7  
**INPUT:** Oformaterad HTML  
**OUTPUT:** Snyggt, responsivt gränssnitt med Tailwind  
**VERIFY:** Dashboard ser professionell ut på mobil och desktop

### Task 9: Implementera felhantering och feedback
**Agent:** frontend-specialist  
**Skills:** clean-code, error-handling  
**Priority:** P2  
**Dependencies:** Task 7  
**INPUT:** Ingen användarfeedback vid fel  
**OUTPUT:** Toast-meddelanden eller inline-fel vid API-fel, laddningsindikatorer  
**VERIFY:** Användaren ser tydliga meddelanden vid lyckat/misslyckat skapande

### Task 10: Skapa dokumentation och exempel
**Agent:** backend-specialist  
**Skills:** documentation  
**Priority:** P3  
**Dependencies:** Task 5  
**INPUT:** Ingen dokumentation för API  
**OUTPUT:** README.md med API-beskrivning, curl-exempel, dashboard-instruktioner  
**VERIFY:** Dokumentation innehåller alla nödvändiga steg för att komma igång

## Phase X: Verification

### Mandatory Verification Checklist
- [ ] Security Scan: API-nycklar inte hårdkodade
- [ ] Autentisering krävs för alla endpoints
- [ ] Validering av indata innan Stripe-anrop
- [ ] Dashboard kan skapa produkt i Stripe test-miljö
- [ ] API kan anropas med curl/postman
- [ ] Felhantering för ogiltiga Stripe-nycklar
- [ ] Loggning av viktiga händelser

### Verification Commands
```bash
# Testa Stripe-klienten
go test ./internal/stripe/...

# Testa API-endpoints
curl -X POST http://localhost:8080/products -H "Authorization: Bearer xxx" -d '{"name":"Test"}'

# Starta dashboard
cd frontend && npm run dev
```

## Risk Areas
- **Stripe Rate Limiting:** För många anrop kan orsaka begränsning
- **Säkerhet:** API-nycklar måste skyddas, endast HTTPS i produktion
- **Data Validering:** Stripe har strikta krav på produktdata
- **Error Propagation:** Fel från Stripe måste hanteras och presenteras användarvänligt

## Rollback Strategy
- Ta bort Stripe SDK från go.mod
- Ta bort API-endpoints
- Avpublicera dashboard
- Ta bort Stripe-nycklar från miljövariabler