# Logger Improvements - Rotation & Export

## Overview
Förbättra befintlig logger-paket med log rotation (storlek/tidsbaserad) och exportfunktioner för att underlätta hantering och analys av loggar.

## Project Type
BACKEND (Library improvement)

## Success Criteria
1. Log rotation: Loggfil roteras när den når konfigurerad storlek (t.ex. 10 MB) eller dagligen.
2. Rotation behåller ett konfigurerbart antal gamla filer (t.ex. 7 dagar).
3. Exportfunktioner: Exportera logghistorik till JSON, CSV och txt-format.
4. Befintliga funktioner förblir bakåtkompatibla.
5. Alla tester passerar.
6. Ingen prestandaförsämring vid normal användning.

## Tech Stack
- Go (existing)
- github.com/jrick/logrotate (redan dependency) för rotation
- Standard library för CSV/JSON export

## File Structure
```
pkg/logger/
├── logger.go              (befintlig, utökad)
├── logger_test.go         (utökad med nya tester)
├── rotation.go            (ny fil för rotationslogik)
└── export.go              (ny fil för exportfunktioner)
```

## Task Breakdown

### Task 1: Analysera befintlig kod och beroenden
**Agent:** project-planner (jag)
**Skills:** clean-code
**Priority:** P0
**Dependencies:** None
**INPUT:** Befintliga logger.go, logger_test.go, go.mod
**OUTPUT:** Förståelse för nuvarande implementation och identifiering av rotationsbibliotek
**VERIFY:** Kan förklara hur loggning fungerar och vilka beroenden som finns.

### Task 2: Designa rotationssystem
**Agent:** backend-specialist
**Skills:** clean-code
**Priority:** P1
**Dependencies:** Task 1
**INPUT:** Analys från Task 1, krav på rotation (storlek, tid, antal backups)
**OUTPUT:** Design dokument i rotation.go med strukturer för konfiguration och rotation
**VERIFY:** Design dokument klar med tydliga structs och metoder.

### Task 3: Implementera rotationslogik
**Agent:** backend-specialist
**Skills:** clean-code
**Priority:** P1
**Dependencies:** Task 2
**INPUT:** Design från Task 2
**OUTPUT:** rotation.go med full implementation som integreras med logger.go
**VERIFY:** Kompilerar utan fel, enhetstester passerar.

### Task 4: Designa exportfunktioner
**Agent:** backend-specialist
**Skills:** clean-code
**Priority:** P2
**Dependencies:** Task 1
**INPUT:** Analys från Task 1, krav på exportformat (JSON, CSV, TXT)
**OUTPUT:** Design dokument i export.go med funktionssignaturer
**VERIFY:** Design dokument klart.

### Task 5: Implementera exportfunktioner
**Agent:** backend-specialist
**Skills:** clean-code
**Priority:** P2
**Dependencies:** Task 4
**INPUT:** Design från Task 4
**OUTPUT:** export.go med funktioner ExportJSON, ExportCSV, ExportTXT
**VERIFY:** Kompilerar utan fel, enhetstester passerar.

### Task 6: Integrera rotation med befintlig file logging
**Agent:** backend-specialist
**Skills:** clean-code
**Priority:** P1
**Dependencies:** Task 3
**INPUT:** rotation.go klar, logger.go
**OUTPUT:** Modifierad EnableFileLogging som stöder rotation, konfigurationsfunktioner
**VERIFY:** EnableFileLogging accepterar rotationsparametrar, skapar roterade filer.

### Task 7: Skapa konfigurations-API för rotation
**Agent:** backend-specialist
**Skills:** clean-code
**Priority:** P2
**Dependencies:** Task 6
**INPUT:** Integrerad rotation
**OUTPUT:** Publika funktioner SetRotationConfig, GetRotationConfig
**VERIFY:** Konfiguration kan ändras dynamiskt.

### Task 8: Skapa enhetstester för rotation
**Agent:** test-engineer
**Skills:** clean-code
**Priority:** P3
**Dependencies:** Task 3
**INPUT:** rotation.go
**OUTPUT:** rotation_test.go med täckning för rotationsscenarier
**VERIFY:** Tester passerar, täckning >80%.

### Task 9: Skapa enhetstester för export
**Agent:** test-engineer
**Skills:** clean-code
**Priority:** P3
**Dependencies:** Task 5
**INPUT:** export.go
**OUTPUT:** export_test.go med täckning för exportfunktioner
**VERIFY:** Tester passerar, täckning >80%.

### Task 10: Uppdatera dokumentation och exempel
**Agent:** backend-specialist
**Skills:** clean-code
**Priority:** P3
**Dependencies:** Task 6, Task 7
**INPUT:** Alla implementationer klara
**OUTPUT:** Uppdaterad README (eller kommentarer) med exempel på rotation och export
**VERIFY:** Dokumentation finns och är korrekt.

### Task 11: Final integration och validering
**Agent:** test-engineer
**Skills:** clean-code
**Priority:** P3
**Dependencies:** Task 8, Task 9, Task 10
**INPUT:** Alla komponenter klara
**OUTPUT:** Kör alla tester, inklusive befintliga logger_test.go
**VERIFY:** Alla tester gröna, inga regressioner.

## Phase X: Verification
- [ ] Lint: `golangci-lint run ./pkg/logger/...`
- [ ] Build: `go build ./pkg/logger`
- [ ] Unit tests: `go test ./pkg/logger/... -v`
- [ ] Coverage: `go test ./pkg/logger/... -cover`
- [ ] Manual test: Skapa testprogram som demonstrerar rotation och export
- [ ] Backward compatibility: Befintliga program som använder logger ska fortfarande fungera

## Risks
1. Rotation kan orsaka förlust av loggar om inte korrekt implementerad.
2. Prestandapåverkan vid kontinuerlig rotation.
3. Fler dependencies kan öka komplexitet.

## Rollback Plan
Om nya funktioner orsakar problem, kan vi:
1. Inaktivera rotation via konfiguration (default av).
2. Återställa logger.go till tidigare version från git.
3. Använda feature flags.