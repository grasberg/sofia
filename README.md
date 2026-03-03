# Sofia - AI Workspace Assistant 🧠✨

Sofia är en avancerad, kontextmedveten AI-assistent och multi-agent-orkestrerare skriven i Go. Designad för att fungera som en fullstack-utvecklare, systemarkitekt och projektledare. Genom att integrera direkt i den lokala utvecklingsmiljön kan Sofia läsa/skriva filer, exekvera terminalkommandon, schemalägga uppgifter och delegera arbete till specialiserade sub-agenter.

## ✨ Huvudfunktioner

*   🛠️ **Autonom Verktygsanvändning:** Kan läsa/redigera filer, köra bash-kommandon, interagera med Google CLI (Gmail/Calendar) och hämta data från webben.
*   🧠 **Persistens & Minne:** Upprätthåller ett långtidsminne (`MEMORY.md`) och för dagliga anteckningar för att aldrig tappa kontexten över tid.
*   🤖 **Multi-Agent Orkestrering:** Kan starta bakgrundsprocesser (`spawn`) och delegera komplexa uppgifter till parallella agenter.
*   📚 **Antigravity Kit (Skill System):** Bestyckad med unika "skills" (expert-personas och kunskapsmoduler) för domänspecifik expertis inom allt från frontend-arkitektur till penetrationstestning.
*   💬 **Gateway Mode:** Inbyggt stöd för chattplattformar som Telegram och Discord via `sofia gateway`.

## 📂 Workspace-struktur

Sofias hjärna och arbetsyta är strukturerad enligt följande (placerad i `~/.sofia/workspace-coder`):

```text
workspace-coder/
├── IDENTITY.md            # Basidentitet: ton, roll och hur Sofia ska presentera sig
├── SOUL.md                # Kärnprinciper: beteende, värderingar och beslutsstil
├── memory/
│   ├── MEMORY.md          # Sofias långtidsminne och globala kontext
│   └── YYYYMM/            # Dagliga anteckningar och task-tracking (ex. 20260228.md)
├── skills/                # Antigravity Kit - Expertis och beteendemönster
│   ├── frontend-specialist/
│   ├── security-auditor/
│   ├── devops-engineer/
│   ├── clean-code/
│   └── ... 
└── workspace/             # Arbetsyta för kodgenerering och projekt
```

## 🚀 Installation & Kom igång

### Krav

Innan du bygger från källkod behöver du ha **Go installerat** (rekommenderat: Go 1.26 eller senare). Du kan ladda ner Go från [go.dev/dl](https://go.dev/dl/).

### Installera från källkod

```bash
git clone https://github.com/grasberg/sofia.git
cd sofia
make deps
make build
```

Den kompilerade binären hamnar i `build/sofia`.

### Quick Start

1. **Initiera konfiguration och workspace:**
```bash
sofia onboard
```

2. **Konfigurera API-nycklar:**
Du kan antingen redigera `~/.sofia/config.json` manuellt eller använda webbgränssnittet (rekommenderas):
*   Starta Sofia: `sofia gateway`
*   Gå till fliken **Models** i webbläsaren.
*   Lägg till din leverantör och API-nyckel där.

Om du föredrar manuell konfiguration, se till att `config.json` har minst en modell:
```json
{
  "agents": {
    "defaults": {
      "workspace": "~/.sofia/workspace",
      "model": "gpt-5.2"
    }
  },
  "model_list": [
    {
      "model_name": "gpt-5.2",
      "model": "openai/gpt-5.2",
      "api_key": "DIN_API_NYCKEL"
    }
  ]
}
```

3. **Starta Gateway (för chatt/webb-gränssnitt):**
```bash
sofia gateway
```

4. **Öppna Sofias kontrollpanel:**
Surfa till `http://127.0.0.1:18795` i din webbläsare.

## 🔌 Integrationer

För att ge Sofia full kraft kan du koppla samman henne med externa tjänster.

### 📧 Google (Gmail & Kalender)

Sofia använder `gogcli` för att interagera med Google Services.

1.  **Installera gogcli:** Se till att `gog` finns i din PATH.
2.  **Autentisera:** Kör följande i terminalen och följ instruktionerna:
    ```bash
    gog login din.email@gmail.com
    ```
3.  **Aktivera i Sofia:** Lägg till följande i din `~/.sofia/config.json`:
    ```json
    {
      "tools": {
        "google": {
          "enabled": true,
          "binary_path": "gog",
          "allowed_commands": ["gmail", "calendar", "drive"]
        }
      }
    }
    ```

### 🐙 GitHub

För att Sofia ska kunna hantera repon, skapa PRs och pusha kod behöver hon en åtkomsttoken.

1.  **Skapa en Personal Access Token (PAT):** Gå till GitHub Settings -> Developer Settings -> Personal Access Tokens (Fine-grained rekommenderas). Ge behörighet för `contents`, `pull requests` och `metadata`.
2.  **Konfigurera i Sofia:** Du kan antingen sätta en miljövariabel i din `.env`-fil:
    ```bash
    GITHUB_TOKEN=your_token_here
    ```
    Eller lägga till det i `config.json` under `env_vars`:
    ```json
    {
      "env_vars": {
        "GITHUB_TOKEN": "your_token_here"
      }
    }
    ```
3.  **Git-identitet:** Se till att din lokala git är konfigurerad så att Sofia kan committa i ditt namn:
    ```bash
    git config --global user.name "Ditt Namn"
    git config --global user.email "din.email@example.com"
    ```

## ⏱️ Schemaläggning (Cron)

Sofia har inbyggt stöd för schemalagda jobb och påminnelser:

```bash
# Varje 10:e minut
sofia cron add --name followup --message "Kolla väntande uppgifter" --every 600

# Varje dag kl 09:00 (Cron-uttryck)
sofia cron add --name morning --message "Sammanfatta dagens prioriteringar" --cron "0 9 * * *"
```

## 🔒 Säkerhetsmodell

Sofia stödjer workspace-restriktioner för att förhindra oavsiktlig modifiering av systemfiler:

```json
{
  "agents": {
    "defaults": {
      "restrict_to_workspace": true
    }
  }
}
```
När detta är aktiverat är fil- och kommandoverktyg strikt begränsade till den konfigurerade workspace-sökvägen.

## 🧭 Exempel: `IDENTITY.md` och `SOUL.md`

### Exempel på `IDENTITY.md`

```md
# Sofia Identity

Du är Sofia, en teknisk AI-assistent.

- Kommunicera tydligt, kortfattat och lösningsorienterat.
- Prioritera praktiska nästa steg framför långa resonemang.
- Förklara ändringar med fokus på varför, inte bara vad.
```

### Exempel på `SOUL.md`

```md
# Sofia Soul

## Principer

1. Säkerhet först: föreslå säkra default-val.
2. Var transparent: säg vad du ändrat och hur det verifieras.
3. Respektera kodbasen: följ befintliga mönster och stil.
4. Undvik överarbete: gör minsta möjliga ändring som löser problemet.
```

---
*Byggd för att accelerera utveckling. Din lokala AI-kollega.*
