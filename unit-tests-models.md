# Plan: Unit-tester för modeller

## Overview
Sofia-projektet är ett omfattande Go-baserat AI-assistantsystem med många paket och datamodeller. Många modeller har redan unit-tester, men det finns sannolikt modeller som saknar tester eller har otillräcklig testtäckning. Denna plan syftar till att identifiera och skapa unit-tester för alla viktiga datamodeller i projektet, vilket ökar kodkvaliteten, minskar buggar och underlättar refaktorering.

## Project Type
**BACKEND** – Go-projekt med många interna paket och datastrukturer. Primäragent: `backend-specialist`.

## Success Criteria
- Identifiera alla struct-definitioner i pkg/ och cmd/ som saknar unit-tester
- Skapa testfiler med minst 80% testtäckning för dessa modeller
- Alla nya tester ska passera när de körs med `go test ./...`
- Inga regressioner i befintliga tester
- CI/CD pipeline inkluderar automatisk testkörning för nya tester

## Tech Stack
- **Språk:** Go 1.25+
- **Testramverk:** Inbyggt `testing` paket
- **Mockning:** `github.com/stretchr/testify` (redan beroende)
- **CI/CD:** GitHub Actions (redan konfigurerat)
- **Code Coverage:** `go test -cover`

## File Structure
Projektstrukturen är redan etablerad. Nya testfiler placeras i samma paket som motsvarande modellfiler med suffix `_test.go`.

Exempel:
```
pkg/agent/a2a.go          → befintlig modell
pkg/agent/a2a_test.go     → befintlig test (ska inte ändras)
pkg/agent/memory.go       → befintlig modell
pkg/agent/memory_test.go  → ny testfil (om saknas)
```

## Task Breakdown

### Task 1: Analysera befintliga modeller och identifiera modeller som saknar unit-tester
**Agent:** `backend-specialist`  
**Skills:** `clean-code`  
**Priority:** P0  
**Dependencies:** Ingen  
**INPUT:** Go-källkod i pkg/ och cmd/  
**OUTPUT:** Lista över struct-definitioner och tillhörande filer som saknar tester  
**VERIFY:** Listan innehåller filnamn, struct-namn och paket. Bekräfta med `grep` och manuell inspektion.

### Task 2: Skapa testfiler för saknade modeller i varje paket
**Agent:** `backend-specialist`  
**Skills:** `clean-code`  
**Priority:** P1  
**Dependencies:** Task 1  
**INPUT:** Lista från Task 1  
**OUTPUT:** Tomma testfiler (`*_test.go`) med korrekt paketnamn och importer  
**VERIFY:** Testfiler existerar i rätt kataloger och kompilerar (`go build ./...`).

### Task 3: Implementera grundläggande unit-tester för varje modell
**Agent:** `backend-specialist`  
**Skills:** `clean-code`  
**Priority:** P1  
**Dependencies:** Task 2  
**INPUT:** Tomma testfiler  
**OUTPUT:** Färdiga tester som validerar struct-fält, metoder och edge cases  
**VERIFY:** Testerna passerar (`go test ./pkg/...`) och ger meningsfull coverage.

### Task 4: Kör befintliga tester för att säkerställa inga regressioner
**Agent:** `backend-specialist`  
**Skills:** `clean-code`  
**Priority:** P2  
**Dependencies:** Task 3  
**INPUT:** Alla testfiler i projektet  
**OUTPUT:** Resultat av full testkörning  
**VERIFY:** Alla tester passerar (`go test ./...`). Inga nya failures.

### Task 5: Integrera med CI/CD (GitHub Actions) för automatisk testkörning
**Agent:** `backend-specialist`  
**Skills:** `clean-code`  
**Priority:** P2  
**Dependencies:** Task 4  
**INPUT:** GitHub Actions workflow-filer  
**OUTPUT:** Uppdaterad CI/CD pipeline som kör alla tester vid push  
**VERIFY:** GitHub Actions jobb passerar på testbranch.

## Phase X: Final Verification
**MANDATORY SCRIPT EXECUTION:**

1. **Security Scan:**
```bash
python .agent/skills/vulnerability-scanner/scripts/security_scan.py .
```

2. **Build Verification:**
```bash
go build ./...
```

3. **Test Execution:**
```bash
go test ./... -cover
```

4. **Lint Check:**
```bash
golangci-lint run ./...
```

5. **Rule Compliance:**
- [ ] Inga purple/violet hex codes (ej relevant)
- [ ] Ingen standard template layout (ej relevant)
- [ ] Socratic Gate var respekterad

6. **Phase X Completion Marker:**
```
## ✅ PHASE X COMPLETE
- Build: ✅ Success
- Tests: ✅ All passed
- Coverage: ✅ Adequate
- Date: 2026-03-19
```

## Risk Areas
- **Stora paket:** Vissa paket kan ha många modeller och ta tid att testa.
- **Existerande buggar:** Tester kan avslöja buggar som måste fixas separat.
- **Flaky tests:** Tester som beroende på externa resurser kan vara ostabila.

## Rollback Strategy
Om nya tester introducerar regressioner eller bryter bygget:
1. Återställ de ändrade testfilerna via git.
2. Kör `go test ./...` för att bekräfta att tidigare state återställs.
3. Isolera problemet och fixa i separat branch.

## Milestones
1. **Analys klar:** Lista över modeller utan tester.
2. **Testfiler skapade:** Tomma testfiler för alla identifierade modeller.
3. **Tester implementerade:** Alla nya tester passerar.
4. **CI/CD uppdaterad:** Automatisk testkörning i pipeline.
