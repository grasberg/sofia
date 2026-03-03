# Skatteverket OAuth 2.0 Dokumentation

Denna dokumentation sammanfattar hur Skatteverkets auktorisationsserver fungerar för OAuth 2.0, baserat på den senaste uppdateringen (maj 2022).

## 🌍 Miljöer och Endpoints

Skatteverket tillhandahåller två miljöer för sina API:er: Test (Sandbox) och Produktion.

### Produktion
*   **Token Endpoint:** `https://api.skatteverket.se/oauth2/v1/token`
*   **Authorization Endpoint:** `https://api.skatteverket.se/oauth2/v1/authorize`

### Test (Sandbox)
*   **Token Endpoint:** `https://api.test.skatteverket.se/oauth2/v1/token`
*   **Authorization Endpoint:** `https://api.test.skatteverket.se/oauth2/v1/authorize`

---

## 🔐 Auktoriseringsflöden

Skatteverket stödjer två huvudsakliga OAuth 2.0-flöden beroende på vilken typ av API som används.

### 1. Client Credentials Flow
Används för server-till-server-kommunikation där ingen specifik användare behöver ge sitt samtycke (t.ex. för publika data eller vissa administrativa tjänster).

*   **Anrop:** `POST` till Token Endpoint.
*   **Autentisering:** Sker via `Client ID` och `Client Secret` (skickas oftast som Basic Auth i headern).
*   **Scopes:** Parametern `scope` **måste** skickas med i anropet. Vilka scopes som ska användas beror på vilket API som anropas (se specifik API-dokumentation).
*   **Respons:** Returnerar en `access_token` som används i `Authorization: Bearer <token>`-headern vid anrop till API:et.

### 2. Authorization Code Flow
Används för API:er som kräver en slutanvändares samtycke eller identifiering, till exempel via **BankID**.

*   **Steg 1 (Auktorisering):** Applikationen skickar användaren till Authorization Endpoint med parametrar som `client_id`, `response_type=code`, `redirect_uri` och `scope`.
*   **Steg 2 (Identifiering):** Användaren identifierar sig hos Skatteverket (t.ex. med BankID).
*   **Steg 3 (Kod):** Skatteverket skickar tillbaka en `code` till applikationens `redirect_uri`.
*   **Steg 4 (Token):** Applikationen växlar in koden mot en `access_token` genom ett `POST`-anrop till Token Endpoint.

---

## 🔑 Hantering av Scopes
Scopes fungerar som behörighetskontroll för vad applikationen får göra.
*   Varje API hos Skatteverket definierar sina egna scopes.
*   Exempel på scope kan vara `arbetsgivardeklaration-inlamning` eller `kundhandelser`.
*   Kontrollera alltid den specifika dokumentationen för det API du ska integrera mot för att hitta rätt scope-namn.

## 🛠 Verifiering
För att testa din integration:
1.  Använd testmiljöns endpoints.
2.  Gör ett anrop med dina test-credentials.
3.  Kontrollera att du får en giltig JSON-respons med en `access_token`.

---
*Källa: Skatteverkets Utvecklarportal (senast uppdaterad 2022-05-01)*
