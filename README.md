# Sofia - AI Workspace Assistant 🧠✨

![Version](https://img.shields.io/badge/version-v0.0.17-blue)
Sofia är en avancerad, kontextmedveten AI-assistent och multi-agent-orkestrerare skriven i Go. Designad för att fungera som en fullstack-utvecklare, systemarkitekt och projektledare. Genom att integrera direkt i den lokala utvecklingsmiljön kan Sofia läsa/skriva filer, exekvera terminalkommandon, schemalägga uppgifter och delegera arbete till specialiserade sub-agenter.

## ✨ Huvudfunktioner

*   🛠️ **Autonom Verktygsanvändning:** Kan läsa/redigera filer, köra bash-kommandon, interagera med Google CLI (Gmail/Calendar) och hämta data från webben.
*   🧠 **Persistens & Minne:** Upprätthåller ett långtidsminne (`MEMORY.md`) och för dagliga anteckningar för att aldrig tappa kontexten över tid.
*   🤖 **Multi-Agent Orkestrering:** Kan starta bakgrundsprocesser (`spawn`) och delegera komplexa uppgifter till parallella agenter.
*   🌐 **Brett AI-stöd:** Inbyggt stöd för 16 olika AI-leverantörer inkl. OpenAI, Anthropic, Gemini, DeepSeek, Grok, MiniMax och fler via ett enkelt webbgränssnitt.
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

### 💬 Telegram

Sofia kan kopplas till Telegram och svara på meddelanden direkt i chatten.

**Via Web UI (rekommenderat):**
1.  Skapa en bot via [BotFather](https://t.me/BotFather) i Telegram. Kör `/newbot` och följ instruktionerna.
2.  Kopiera bot-tokenen som BotFather ger dig.
3.  Öppna Sofias Web UI → **Channels**.
4.  Aktivera **Telegram**, klistra in din bot-token.
5.  Under **Allow From** kan du begränsa vilka Telegram-användare som får prata med Sofia (frivilligt, lämna tomt för alla).
6.  Klicka **Save Settings** och starta om Sofia.

**Manuell konfiguration i `config.json`:**
```json
{
  "channels": {
    "telegram": {
      "enabled": true,
      "token": "DIN_BOT_TOKEN",
      "allow_from": ["ditt_telegram_användarnamn"]
    }
  }
}
```

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

**Manuell konfiguration i `config.json`:**
```json
{
  "channels": {
    "discord": {
      "enabled": true,
      "token": "DIN_DISCORD_BOT_TOKEN",
      "allow_from": ["ditt_discord_användarnamn"],
      "mention_only": true
    }
  }
}
```

> 💡 **Tips:** Sätt `mention_only` till `true` om Sofia är i en aktiv kanal med många användare — annars svarar hon på allt.

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

---
*Byggd för att accelerera utveckling. Din lokala AI-kollega.*
