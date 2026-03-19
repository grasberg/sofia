# Teknisk Deployment Process

## Översikt
Denna guide beskriver hur teamet deployar tekniska projekt som Sofia (Go-applikation), webbappar och andra kodbaser. Processen täcker bygg, test, kvalitetssäkring och distribution.

## Projekttyper

### 1. Go-projekt (Sofia)
- Byggsystem: Makefile
- CI/CD: GitHub Actions (ci.yaml)
- Distribution: Binärfiler för multiplattform

### 2. Webbappar (Node.js, PHP, statiska sidor)
- Byggsystem: npm scripts, composer
- Hosting: cPanel, Netlify, VPS
- Deployment: Manuell filuppladdning eller CI/CD

### 3. Bibliotek och paket
- Versionering: SemVer
- Distribution: Packagist (PHP), npm (Node.js), Go modules

## Steg 1: Förbereda deployment

### 1.1 Kvalitetssäkring
Före varje deployment måste följande kontroller genomföras:

```bash
# För Go-projekt
make check  # Kör fmt, vet, test
make lint   # Kör golangci-lint

# För Node.js-projekt
npm run lint
npm run test

# För PHP-projekt
composer test
vendor/bin/phpstan analyse
```

### 1.2 Versionshantering
- **Tagga release**: `git tag v1.2.3`
- **Skapa changelog**: Dokumentera ändringar sedan senaste release
- **Commit messages**: Följ konventionell commit-format

### 1.3 Miljövariabler och konfiguration
- Samla alla nödvändiga miljövariabler i `.env.example`
- Uppdatera dokumentation för konfiguration
- Säkerställ att secrets inte checkas in i git

## Steg 2: Bygga projektet

### 2.1 Go-projekt (Sofia)
```bash
# Bygg för aktuell plattform
make build

# Bygg för alla plattformar (release)
make build-all
```
Binärerna hamnar i `build/` mappen:
- `sofia-linux-amd64`
- `sofia-darwin-arm64`
- `sofia-windows-amd64.exe`
- etc.

### 2.2 Node.js/webbapp
```bash
# Installera beroenden
npm ci  # exakt versioner

# Bygg produktionsversion
npm run build

# Resultat i `dist/` eller `build/` mapp
```

### 2.3 Statiska webbplatser
```bash
# Bygg HTML/CSS/JS
npm run build  # eller motsvarande

# Filer redo för deployment
```

## Steg 3: Testa bygget

### 3.1 Smoke tests
- Kör binären/applikationen lokalt
- Verifiera att den startar utan fel
- Testa grundläggande funktionalitet

### 3.2 Integrationstester
- Testa mot externa tjänster (om möjligt)
- Verifiera att konfigurationen fungerar
- Testa migreringar (databas, etc.)

### 3.3 Prestandatester (valfritt)
- Kontrollera starttid
- Testa minnesanvändning
- Verifiera svarstider

## Steg 4: Distribution

### 4.1 Go-binärer (Sofia)
**Alternativ 1: GitHub Releases**
1. Gå till GitHub repository
2. Klicka "Create a new release"
3. Välj taggen (v1.2.3)
4. Ladda upp binärfiler från `build/` mappen
5. Skriv release notes
6. Publicera

**Alternativ 2: Direkt installation**
```bash
# Installera på användares system
make install
```
Detta kopierar binären till `~/.local/bin/` och assets till `~/.local/share/sofia/`

### 4.2 Webbappar till cPanel
**Manuell uppladdning:**
1. Logga in på cPanel
2. Använd File Manager eller SFTP
3. Ladda upp filer till `public_html/` eller underkatalog
4. Kontrollera filrättigheter (755 för mappar, 644 för filer)

**Automatisering via Sofia:**
Sofia har cPanel-integration som kan automatisera detta:
```bash
# Exempelkommando (via Sofia)
cpanel file_upload --path /public_html --local_file dist/index.html
```

### 4.3 Statiska webbplatser till Netlify/Vercel
**Netlify:**
1. Dra och släpp `dist/` mappen till Netlify
2. Eller koppla GitHub repository för automatisk deployment

**Vercel:**
1. `npm i -g vercel`
2. `vercel --prod`

### 4.4 Docker-containers (om använt)
```bash
# Bygg image
docker build -t sofia:latest .

# Push till registry
docker tag sofia:latest registry.example.com/sofia:latest
docker push registry.example.com/sofia:latest
```

## Steg 5: Verifiera deployment

### 5.1 Sanity check
- Besök webbplatsen/applikationen
- Testa kritiska funktioner
- Kontrollera loggar för fel

### 5.2 Monitorering
- Verifiera att monitorering är aktiv (Prometheus, Grafana)
- Kontrollera att aviseringar fungerar
- Övervaka felränta och prestanda

### 5.3 Rollback-plan
Ha alltid en rollback-plan redo:
- **Go-binärer:** Återgå till föregående version i GitHub Releases
- **Webbappar:** Återställ föregående backup
- **Databas:** Ha migreringsscript som kan köras baklänges

## Steg 6: Dokumentation och kommunikation

### 6.1 Uppdatera dokumentation
- Uppdatera `README.md` med nya versioner
- Dokumentera kända fel eller begränsningar
- Uppdatera installationsinstruktioner

### 6.2 Kommunikation till team/användare
- Meddela teamet om ny release
- Uppdatera changelog offentligt
- Informera användare om breaking changes

### 6.3 Uppdatera beroenden
- Uppdatera `go.mod`, `package.json`, `composer.json` om nödvändigt
- Testa med uppdaterade beroenden i separat branch

## CI/CD Pipeline (GitHub Actions)

### Existerande pipeline (ci.yaml)
Teamets CI pipeline kör automatiskt vid push till main:
1. **Build:** `go build ./...`
2. **Test:** `go test ./...`
3. **Vet:** `go vet ./...`
4. **Lint:** `golangci-lint`

### Utöka pipeline för automatisk deployment
För automatisk deployment kan pipeline utökas med:

```yaml
deploy:
  needs: [build-and-test, lint]
  runs-on: ubuntu-latest
  steps:
    - uses: actions/checkout@v4
    - name: Build binaries
      run: make build-all
    - name: Create Release
      uses: softprops/action-gh-release@v1
      with:
        files: build/*
        tag_name: v${{ github.ref_name }}
        generate_release_notes: true
```

## Checklista för teknisk deployment

### Före bygg
- [ ] Alla tester passerar
- [ ] Linting och formattering är grön
- [ ] Changelog uppdaterad
- [ ] Version taggad i git
- [ ] Miljövariabler dokumenterade

### Under bygg
- [ ] Bygget lyckas utan varningar
- [ ] Binärer/test-filer genererade
- [ ] Smoke test genomfört

### Under distribution
- [ ] Binärer/applikation uppladdad till rätt plats
- [ ] Filrättigheter korrekta
- [ ] Konfiguration applicerad

### Efter distribution
- [ ] Sanity check utförd
- [ ] Monitorering verifierad
- [ ] Dokumentation uppdaterad
- [ ] Team informerat

## Felsökning

### Vanliga problem
1. **Bygget misslyckas:** Kontrollera go.mod/node_modules versioner
2. **Binären startar inte:** Kontrollera beroenden och runtime-miljö
3. **Webbplatsen visar fel:** Kontrollera filsökvägar och serverkonfiguration
4. **CI pipeline fail:** Kolla loggar för specifika test/lint-fel

### Debugging i produktion
- Använd loggnivå DEBUG eller TRACE
- Kontrollera systemloggar (`journalctl` för systemd)
- Använd monitorering för att identifiera problem

## Automationsmöjligheter
1. **Automatisk tagging:** Skript som skapar tag baserat på semver
2. **Changelog generator:** Automatisk changelog från commit messages
3. **Self-update mekanism:** Sofia kan uppdatera sig själv via `self_modify`
4. **Blue-green deployment:** För zero-downtime updates

## Säkerhetsöverväganden
- **Signera releases:** Använd GPG för att signera binärer
- **Security scanning:** Integrera trivy, gosec eller liknande
- **Secret management:** Använd GitHub Secrets eller extern vault
- **Access control:** Begränsa vem som kan deploya

## Versionhistorik
- **v1.0** (2026-03-19): Första versionen skapad av teamet
- **Uppdateringar:** Dokumentet uppdateras när verktyg eller processer ändras