# Sofia - AI Workspace Assistant 🧠✨

![Version](https://img.shields.io/badge/version-v0.0.56-blue)
Sofia är en avancerad, kontextmedveten AI-assistent och multi-agent-orkestrerare skriven i Go. Designad för att fungera som en fullstack-utvecklare, systemarkitekt och projektledare. Genom att integrera direkt i den lokala utvecklingsmiljön kan Sofia läsa/skriva filer, exekvera terminalkommandon, schemalägga uppgifter och delegera arbete till specialiserade sub-agenter.

## ✨ Huvudfunktioner

*   🛠️ **Autonom Verktygsanvändning:** Kan läsa/redigera filer, köra bash-kommandon, interagera med Google CLI (Gmail/Calendar) och hämta data från webben.
*   🧠 **Persistens & Minne:** Upprätthåller ett långtidsminne och för dagliga anteckningar i en delad SQLite-databas (`~/.sofia/memory.db`) för att aldrig tappa kontexten över tid.
*   🤖 **Multi-Agent Orkestrering:** Kan starta bakgrundsprocesser (`spawn`) och delegera komplexa uppgifter till parallella agenter.
*   🌐 **Brett AI-stöd:** Inbyggt stöd för 16 olika AI-leverantörer inkl. OpenAI, Anthropic, Gemini, DeepSeek, Grok, MiniMax och fler via ett enkelt webbgränssnitt.
*   📚 **Antigravity Kit (Skill System):** Bestyckad med unika "skills" (expert-personas och kunskapsmoduler) för domänspecifik expertis inom allt från frontend-arkitektur till penetrationstestning.
*   💬 **Gateway Mode:** Inbyggt stöd för chattplattformar som Telegram och Discord via `sofia gateway`.

## 📂 Workspace-struktur

Sofias konfiguration och arbetsyta finns under `~/.sofia/`:

```text
~/.sofia/
├── config.json            # Huvudkonfiguration (modeller, kanaler, inställningar)
├── memory.db              # Delad SQLite-databas för minne och sessionshistorik
├── antigravity-kit/       # Bundlat Antigravity Kit (installeras via sofia onboard)
└── workspace/             # Sofias arbetsyta
    ├── IDENTITY.md        # Basidentitet: ton, roll och hur Sofia ska presentera sig
    ├── SOUL.md            # Kärnprinciper: beteende, värderingar och beslutsstil
    ├── AGENT.md           # Agent-specifik systemprompt
    ├── USER.md            # Användarkontext och preferenser
    ├── HEARTBEAT.md       # Instruktioner för bakgrundsagenten
    ├── skills/            # Lokala skills/expert-personas
    │   ├── github/
    │   ├── hardware/
    │   ├── skill-creator/
    │   ├── summarize/
    │   ├── tmux/
    │   ├── weather/
    │   └── ...
    ├── cron/              # Schemalagda jobb (jobs.json)
    └── state/             # Persistent runtime-state
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

Den kompilerade binären hamnar direkt i projektets rotmapp som `./sofia`.

### Quick Start

1. **Initiera konfiguration och workspace:**
```bash
sofia onboard
```

2. **Starta Gateway (för chatt/webb-gränssnitt):**
```bash
sofia gateway
```

3. **Öppna Sofias kontrollpanel:**
Surfa till `http://127.0.0.1:18795` i din webbläsare. Gå till fliken **Models** för att lägga till din leverantör och API-nyckel.


## 🔒 Säkerhetsmodell

Sofia stödjer workspace-restriktioner för att förhindra oavsiktlig modifiering av systemfiler.

**Via Web UI:**
1.  Öppna Sofias Web UI → **Settings**.
2.  Klicka på fliken **Security**.
3.  Aktivera **Restrict to Workspace** — fil- och kommandoverktyg begränsas då strikt till den konfigurerade workspace-sökvägen.
4.  Inställningen sparas automatiskt.

## 💓 Heartbeat (Bakgrundsagent)

Sofia kan automatiskt utföra uppgifter i bakgrunden enligt ett schema.

**Via Web UI:**
1.  Öppna Sofias Web UI → **Settings**.
2.  Klicka på fliken **Heartbeat**.
3.  Aktivera **Enable Heartbeat** och ange hur ofta agenten ska köra (i minuter).
4.  Ange **Active Hours** i formatet `09:00-17:00` — lämna tomt för 24/7.
5.  Välj **Active Days** — lämna tomt för att köra varje dag.
6.  Inställningarna sparas automatiskt.

## 🧭 Anpassa Sofias Personlighet

Sofias beteende, ton och personlighet styrs av två filer: **IDENTITY.md** och **SOUL.md**. Du kan enkelt redigera dem direkt i webbgränssnittet:

1.  **Starta Sofia:** `sofia gateway`
2.  **Öppna webbläsaren:** Surfa till `http://127.0.0.1:18795`
3.  **Gå till Settings** i vänstermenyn.
4.  Redigera **IDENTITY.md** (vem Sofia är) och **SOUL.md** (hur Sofia beter sig) direkt i textrutorna.
5.  Klicka **Save prompt files** — ändringarna träder i kraft omedelbart utan omstart.

### `IDENTITY.md` — Vem är Sofia?
Definierar Sofias roll, namn, och grundläggande kontext. Exempel:
```md
# Identity
- Name: Sofia
- Role: Personal AI assistant
- Running: 24/7 on the user's own hardware
```

### `SOUL.md` — Hur beter sig Sofia?
Definierar personlighet, språk, värderingar och beslutslogik. Exempel:
```md
# Soul
- Svara alltid på svenska
- Var proaktiv och självgående
- Använd torr humor och driv
- Prioritera handling framför att fråga om lov
```

> 💡 **Tips:** Du kan ge Sofia vilken personlighet du vill — formell, avslappnad, sarkastisk, pedagogisk, eller helt skräddarsydd för ditt arbetsflöde.

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

### 💬 Telegram

Sofia kan kopplas till Telegram och svara på meddelanden direkt i chatten.

**Via Web UI (rekommenderat):**
1.  Skapa en bot via [BotFather](https://t.me/BotFather) i Telegram. Kör `/newbot` och följ instruktionerna.
2.  Kopiera bot-tokenen som BotFather ger dig.
3.  Öppna Sofias Web UI → **Channels**.
4.  Aktivera **Telegram**, klistra in din bot-token.
5.  Under **Allow From** kan du begränsa vilka Telegram-användare som får prata med Sofia (frivilligt, lämna tomt för alla).
6.  Klicka **Save Settings** och starta om Sofia.



> 💡 **Tips:** Om du kör Sofia bakom en brandvägg eller VPN kan du ange en proxy under **Proxy**-fältet i Channels-sidan.

### 🎮 Discord

Sofia kan även vara aktiv i Discord-servrar och DM:s.

**Via Web UI (rekommenderat):**
1.  Gå till [Discord Developer Portal](https://discord.com/developers/applications) och skapa en ny applikation.
2.  Under **Bot** → klicka **Add Bot** → kopiera din **Bot Token**.
3.  Under **OAuth2 → URL Generator** — välj `bot` scope och ge den behörigheter att läsa/skicka meddelanden. Bjud in boten till din server via den genererade länken.
4.  Öppna Sofias Web UI → **Channels**.
5.  Aktivera **Discord**, klistra in din bot-token.
6.  **Allow From** — ange Discord-användarnamn som får interagera med Sofia (frivilligt).
7.  **Mention Only** — om aktiverat svarar Sofia bara när hon @-nämns, annars svarar hon på alla meddelanden i kanaler hon har tillgång till.
8.  Klicka **Save Settings** och starta om Sofia.



> 💡 **Tips:** Sätt `mention_only` till `true` om Sofia är i en aktiv kanal med många användare — annars svarar hon på allt.


---
*Byggd för att accelerera utveckling. Din lokala AI-kollega.*
