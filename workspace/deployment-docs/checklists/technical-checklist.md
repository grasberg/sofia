# Checklista för Teknisk Deployment av Sofia

## Förberedelse
### Systemkrav
- [ ] Go 1.25.7+ installerad (`go version`)
- [ ] Git installerad (`git --version`)
- [ ] Make installerad (`make --version`)
- [ ] Tillräckligt diskutrymme (minst 500 MB)
- [ ] Nätverksanslutning för att hämta beroenden

### Workspace
- [ ] Projekt klonat (`git clone ...`)
- [ ] Rätt branch/tag checkad ut
- [ ] Ingen ocommittad kod som kan påverka bygget

## Byggprocess
### Beroenden
- [ ] Kör `make deps` för att ladda ner Go-moduler
- [ ] Verifiera att alla beroenden laddats (`go mod verify`)

### Bygg
- [ ] Kör `make build` för att bygga för aktuell plattform
- [ ] Verifiera att binären skapats (`ls -la sofia`)
- [ ] Testa att binären kan köras (`./sofia --version`)

### Bygg för alla plattformar (valfritt)
- [ ] Kör `make build-all`
- [ ] Kontrollera att alla binärer skapats i `build/` mappen
- [ ] Verifiera filtyper (`file build/sofia-*`)

## Testprocess
### Enhetstester
- [ ] Kör `make test` - alla tester ska passera
- [ ] Kontrollera testtäckning (`go test -coverprofile=coverage.out ./...`)

### Kvalitetskontroll
- [ ] Kör `make vet` - ingen static analysis varningar
- [ ] Kör `make lint` - ingen linting fel
- [ ] Kör `make fmt` - kod formaterad korrekt
- [ ] Kör `make check` - alla checkar passera

## Installationsprocess
### Installation
- [ ] Bestäm installationssökväg (standard: `~/.local/bin`)
- [ ] Kör `make install` (eller `INSTALL_PREFIX=/usr/local make install`)
- [ ] Verifiera att binären installeras (`which sofia`)
- [ ] Testa att kommandot fungerar (`sofia --version`)

### Assets och resurser
- [ ] Verifiera att assets kopierats (`ls ~/.local/share/sofia/assets/`)
- [ ] Verifiera att antigravity-kit kopierats (`ls ~/.sofia/antigravity-kit/`)

## Distributionsprocess
### Förberedelse för release
- [ ] Öka versionsnummer i relevanta filer (om inte automatisk)
- [ ] Uppdatera changelog
- [ ] Skapa Git-tag (`git tag -a vX.Y.Z -m "Release vX.Y.Z"`)

### Bygg release-binärer
- [ ] Kör `make build-all`
- [ ] Skapa checksummor (`sha256sum build/sofia-* > sha256sums.txt`)
- [ ] Eventuellt signera checksummor (`gpg --detach-sign --armor sha256sums.txt`)

### Paketering
- [ ] Skapa release-mapp med struktur
- [ ] Inkludera README och LICENSE
- [ ] Inkludera alla plattformsbinärer
- [ ] Inkludera checksummor (och signaturer)

### Plattformsspecifika steg
#### Linux
- [ ] Kontrollera att binärer är statiskt länkade (`ldd sofia-linux-*` ska visa "not a dynamic executable")
- [ ] Testa på minst en Linux-distribution

#### macOS
- [ ] Verifiera att binären inte har quarantines (`xattr sofia-darwin-arm64`)
- [ ] Testa på macOS (Intel/Apple Silicon)

#### Windows
- [ ] Verifiera att .exe fungerar i PowerShell
- [ ] Testa eventuella path-issue

## CI/CD Integration (valfritt)
### GitHub Actions
- [ ] Workflow-filer uppdaterade
- [ ] Secrets konfigurerade (API-nycklar, signeringsnycklar)
- [ ] Testa workflow lokalt med act eller via push

### Automatisk release
- [ ] Release-workflow triggas av tags
- [ ] Binärer laddas upp till GitHub Releases
- [ ] Checksummor genereras automatiskt

## Verifiering efter deployment
### Funktionstest
- [ ] Sofia startar korrekt (`sofia gateway` startar webbserver)
- [ ] Alla verktyg är tillgängliga (`sofia --help` visar alla kommandon)
- [ ] Web UI är tillgänglig (http://localhost:18795)
- [ ] Agent kan köra grundläggande uppgifter

### Prestandatest
- [ ] Starttid acceptabel (< 2 sekunder)
- [ ] Minnesanvändning stabil
- [ ] Inga minnesläckor under längre körning

## Dokumentation
- [ ] Uppdatera README med nya versionsnummer
- [ ] Uppdatera installationsinstruktioner om ändringar
- [ ] Dokumentera kända issue och workarounds

## Slutförande
- [ ] Push tag till remote (`git push origin vX.Y.Z`)
- [ ] Skapa GitHub Release med binärer
- [ ] Uppdatera pakethanterare (Homebrew, etc.) om tillämpligt
- [ ] Meddela användare om ny release (mail, Discord, etc.)

---

## Snabbrelease-checklista (för erfarna)
- [ ] `make check` (tester och kvalitet)
- [ ] `make build-all` (bygg alla plattformar)
- [ ] `git tag -a vX.Y.Z -m "Release vX.Y.Z"`
- [ ] `git push origin vX.Y.Z`
- [ ] Skapa release på GitHub och ladda upp binärer från `build/`
- [ ] Verifiera att automatiska workflows körts

---

*Denna checklista ska användas för varje release av Sofia för att säkerställa konsekvent kvalitet och tillförlitlighet.*