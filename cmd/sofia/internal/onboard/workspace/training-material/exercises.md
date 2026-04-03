# Övningar: Server-side bibliotek

## Översikt
Övningarna är designade för att ge praktisk erfarenhet av att arbeta med populära server-side bibliotek. De är uppdelade i tre nivåer och kan utföras individuellt eller i grupp.

## Nivå 1: Grundläggande (Nybörjare)
**Mål:** Sätta upp en enkel server med ett bibliotek.

### Övning 1.1: Express.js "Hello World"
**Uppgift:** Skapa en Node.js-server med Express som svarar med "Hello World" på root-endpoint (`/`) och visar aktuell tid på `/time`.

**Krav:**
- Använd Express.js
- Server ska lyssna på port 3000
- Endpoint `/` returnerar "Hello World"
- Endpoint `/time` returnerar aktuell tid i ISO-format

**Utökning:**
- Lägg till middleware för loggning av requests
- Skapa en felhanterare för 404

### Övning 1.2: Flask REST API
**Uppgift:** Skapa en enkel REST API med Flask som hanterar en lista över användare.

**Krav:**
- Använd Flask
- GET `/users` returnerar lista med användare (JSON)
- POST `/users` lägger till en användare
- PUT `/users/<id>` uppdaterar en användare
- DELETE `/users/<id>` tar bort en användare

**Utökning:**
- Lägg till validering av input
- Använd SQLite för persistent lagring

## Nivå 2: Mellannivå (Medel)
**Mål:** Bygga mer avancerade funktioner och jämföra bibliotek.

### Övning 2.1: Jämförelse av prestanda
**Uppgift:** Skapa en enkel server i två olika bibliotek (t.ex. Express vs Fastify eller Gin vs Echo) och jämför prestanda med Apache Bench eller autocannon.

**Krav:**
- Implementera samma endpoint i två bibliotek
- Endpoint `/benchmark` utför en CPU-intensiv beräkning (t.ex. fibonacci)
- Kör load test med 1000 request och 10 samtidiga connections
- Jämför resultat (requests per sekund, latens)

**Utökning:**
- Testa med olika payload-storlekar
- Analysera minnesanvändning

### Övning 2.2: Middleware/Interceptor i olika ramverk
**Uppgift:** Implementera autentisering via API-nyckel som middleware/interceptor i minst tre olika ramverk (t.ex. Express, Gin, FastAPI).

**Krav:**
- Skyddade endpoints kräver header `X-API-Key: secret123`
- Ogiltig nyckel returnerar 401 Unauthorized
- Loggning av autentiserade anrop

**Utökning:**
- Implementera rate limiting middleware
- Jämför implementationens komplexitet

## Nivå 3: Avancerad
**Mål:** Bygga en fullständig applikation och integrera med externa tjänster.

### Övning 3.1: Microservice-arkitektur
**Uppgift:** Bygg två mikrotjänster som kommunicerar med varandra, en i Go (med Gin) och en i Python (med FastAPI).

**Krav:**
- **Auth-service** (Go): Validerar JWT-token, returnerar user ID
- **User-service** (Python): Hanterar användardata, kallar auth-service för verifiering
- Kommunikation via HTTP/REST
- Dockerize båda tjänsterna

**Utökning:**
- Lägg till message queue (Redis/RabbitMQ)
- Implementera circuit breaker

### Övning 3.2: Fullstack-applikation
**Uppgift:** Bygg en fullstack-applikation med Laravel (PHP) som backend och React/Vue som frontend.

**Krav:**
- Laravel backend med REST API
- Autentisering med Laravel Sanctum
- CRUD för en produktkatalog
- Frontend som konsumerar API:et
- Deployment instructions

**Utökning:**
- Implementera realtidsfunktioner med Laravel Echo och WebSockets
- Lägg till caching med Redis

## Bonusövningar

### Bonus 1: Migration mellan ramverk
**Uppgift:** Migrera en enkel applikation från ett ramverk till ett annat (t.ex. från Express till Fastify eller från Flask til FastAPI).

**Krav:**
- Dokumentera skillnader i syntax och patterns
- Jämför prestanda före och efter
- Identifiera för- och nackdelar

### Bonus 2: Serverless-funktioner
**Uppgift:** Implementera samma funktionalitet som serverless-funktion i AWS Lambda/Google Cloud Functions för två olika språk.

**Krav:**
- Samma business logic
- Jämför kallstartstid
- Jämföra kostnad

## Bedömningskriterier
- **Kodkvalitet:** Läsbarhet, struktur, följer best practices
- **Funktionalitet:** Uppfyller alla krav
- **Dokumentation:** Tydlig README, kommentarer
- **Prestanda:** Effektiv implementation
- **Kreativitet:** Ytterligare funktioner eller optimeringar

## Lösningsförslag
Lösningsförslag finns i separata mappar för varje övning. Deltagarna uppmuntras att försöka själva innan de tittar på lösningarna.

## Tidsåtgång
- Nivå 1: 1-2 timmar per övning
- Nivå 2: 2-4 timmar per övning  
- Nivå 3: 4-8 timmar per övning
- Bonus: 2-6 timmar per övning