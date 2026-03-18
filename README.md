# Sofia - AI Workspace Assistant 🧠✨

![Version](https://img.shields.io/badge/version-v0.0.123-blue)
Sofia är en avancerad, kontextmedveten AI-assistent och multi-agent-orkestrerare skriven i Go. Designad för att fungera som en fullstack-utvecklare, systemarkitekt och projektledare. Genom att integrera direkt i den lokala utvecklingsmiljön kan Sofia läsa/skriva filer, exekvera terminalkommandon, schemalägga uppgifter och delegera arbete till specialiserade sub-agenter.

## ✨ Huvudfunktioner

*   🛠️ **Autonom Verktygsanvändning:** Kan registrera domännamn, publicera webbsidor, läsa/redigera filer, köra bash-kommandon och hantera Google Services (Gmail/Kalender).
*   🧠 **Avancerat Minne:** Lager av minne — korttids-, långtids-, episodiskt och semantiskt (kunskapsgraf). Automatisk konsolidering och strategisk glömska håller minnet effektivt.
*   🤖 **Multi-Agent Orkestrering:** Delegera komplexa uppgifter till parallella agenter med beroendegraf, A2A-protokoll för inter-agent-kommunikation, och automatisk agentval.
*   🌐 **Brett AI-stöd:** Inbyggt stöd för 20+ AI-leverantörer inkl. OpenAI, Anthropic (Claude 4.5), Gemini, DeepSeek, Grok, MiniMax, Moonshot, Qwen, Zai, GitHub Copilot och fler.
*   📚 **Skill-system med Självlärande:** Antigravity Kit med expert-personas, plus automatisk skill-skapande, förfining och kunskapsdestillering från erfarenheter.
*   🔄 **Självreflektion & Självförbättring:** Post-task utvärdering, prestandaspårning över tid, prompt-självoptimering och kodsjälvmodifiering med säkerhetsspärrar.
*   🔧 **Smart Verktygshantering:** Semantisk verktygsmatchning via embeddings, prestandaspårning för att välja de mest pålitliga verktygen, och dynamisk tool-composition (pipelines) för att skapa nya makro-verktyg.
*   🎯 **Autonomi & Proaktivitet:** Långsiktiga mål, kontextmedvetna triggers, proaktiva förslag och självinitierad research utan användarinteraktion.
*   🛡️ **Guardrails & Säkerhet:** Inputvalidering, outputfiltrering (PII/hemligheter), prompt injection-försvar och åtgärdsbekräftelse för högrisk-operationer.
*   🔌 **MCP-klient:** Model Context Protocol-stöd för anslutning till externa MCP-servrar och verktyg.
*   💬 **Gateway Mode:** Inbyggt stöd för chattplattformar som Telegram och Discord via `sofia gateway`.
*   🖥️ **Computer Use:** Autonom datorstyrning via skärmdumpar och vision-LLM — styr mus och tangentbord på macOS och Linux.
*   🌍 **Webbläsarautomation (Playwright):** Autonom webbsurfning med klick, formulärifyllning, skärmdumpar och textextraktion.
*   📸 **Bildanalys:** Analysera lokala bilder (PNG, JPEG, GIF, WebP) via vision-LLM — OCR, beskrivningar och frågor.
*   📋 **Plan & Execute:** Strukturerad uppgiftsplanering med steg-för-steg-uppföljning.
*   📝 **Delad Scratchpad:** Nyckel-värde-lagring för agent-till-agent-kommunikation.
*   ⏰ **Cron-schemaläggning:** Agenten kan själv skapa, lista, ta bort och schemalägga återkommande uppgifter.
*   🔄 **Provider Fallback:** Automatiska fallback-kedjor om en AI-leverantör misslyckas.
*   🎨 **Modernt Web UI (HTMX):** Brutalistiskt designtema med CRT-effekter, realtidsuppdateringar och filuppladdning i chatten.

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

## 🤖 Multi-Agent Orkestrering

Sofia kan delegera och koordinera arbete över flera agenter:

*   **Orchestrate-verktyg:** Definiera en uppsättning subtasks med beroenden — oberoende uppgifter körs parallellt, beroende uppgifter i rätt ordning. Automatisk agentval baserat på poängberäkning.
*   **Sub-Agenter:** Starta dedikerade bakgrundsagenter (`spawn`) eller synkrona sub-agenter (`subagent`) som ärver verktyg och kontext.
*   **A2A-protokoll (Agent-to-Agent):** Standardiserad inter-agent-kommunikation med mailbox-baserad routing, send/receive/broadcast och pending-polling.
*   **Delad Scratchpad:** Agenter kan dela data via en nyckel-värde-lagring namespaced per uppgiftsgrupp.
*   **Plan & Execute:** Skapa strukturerade planer med steg som kan spåras och uppdateras under exekvering.

## 🖥️ Computer Use

Sofia kan styra din dator autonomt via skärmdumpar och vision-LLM:

*   Tar skärmdumpar av skrivbordet och analyserar dem med vision-LLM
*   Utför mus-klick, tangentbordstryckningar, scrollning och textinmatning
*   Loopar tills uppgiften är klar eller max antal steg är nådd
*   **Plattformar:** macOS (screencapture + osascript) och Linux (scrot + xdotool)

## 🌍 Webbläsarautomation (Playwright)

Sofia har inbyggd Playwright-integration för autonom webbsurfning:

*   Navigera till URL:er, klicka på element, fyll i formulär
*   Ta skärmdumpar, extrahera text och köra JavaScript
*   Vänta på element, hantera tidsgränser och scroll
*   Stödjer Chromium, Firefox och WebKit
*   Headless och headful-läge

## 📸 Bildanalys

Analysera lokala bilder direkt i konversationen:

*   Stöd för PNG, JPEG, GIF och WebP
*   OCR (textavläsning), bildbeskrivning och frågor om bildinnehåll
*   Automatisk MIME-typ-detektering och storleksbegränsning
*   Integrerat med vision-LLM-pipelinen


## 🧠 Avancerat Minne

Sofia har en flerlagrad minnesarkitektur:

*   **Semantiskt Minne (Kunskapsgraf):** Strukturerade fakta, entiteter och relationer lagrade som noder och kanter. Verktyget `knowledge_graph` låter agenten lägga till, söka och ta bort kunskap.
*   **Minneskonsolidering:** `MemoryConsolidator` slår samman duplicerade noder och löser konflikterande relationer — håller kunskapsgrafen ren automatiskt.
*   **Strategisk Glömska:** `MemoryPruner` beräknar en överlevnadspoäng baserat på åtkomstfrekvens och tid sedan senaste åtkomst. Noder under tröskelvärdet tas bort automatiskt.
*   **Självutvecklande Minne:** Alla åtkomster spåras via `RecordStat`, vilket driver både konsolidering och pruning baserat på faktiska användningsmönster.

## 🔄 Självreflektion & Självförbättring

Sofia utvärderar sig själv efter varje uppgift och förbättras kontinuerligt:

*   **Post-Task Reflektion:** `ReflectionEngine` kör en LLM-driven utvärdering efter varje uppgift: vad fungerade, vad misslyckades, lärdommar och meta-learning.
*   **Prestandapoäng:** `PerformanceScorer` beräknar ett 0.0–1.0-betyg baserat på felfrekvens, verktygseffektivitet och slutförande.
*   **Trendanalys:** `GetPerformanceTrend` jämför nyliga vs äldre reflektioner för att detektera förbättring eller nedgång.
*   **Prompt-självoptimering:** `optimizePrompt` justerar automatiskt systeminstruktioner baserat på dåliga prestationsresultat.
*   **Meta-Learning:** Varje reflektion inkluderar ett `meta_learning`-fält som lagrar insikter om själva inlärningsprocessen.
*   **Kodsjälvmodifiering:** `self_modify`-verktyget låter Sofia säkert modifiera sin egen kod med bekräftelse-hash och audit trail.

## 🎯 Autonomi & Proaktivitet

Sofia kan agera självständigt utan användarinitiering:

*   **Långsiktiga Mål:** `manage_goals`-verktyget skapar och spårar mål som persisterar över sessioner. Aktiva mål injiceras automatiskt i agentens kontext.
*   **Kontextmedvetna Triggers:** `manage_triggers`-verktyget skapar villkorliga handlingar som aktiveras baserat på användarens samtalskontext.
*   **Proaktiva Förslag:** `AutonomyService` analyserar periodiskt senaste aktiviteten och genererar oombedda förslag när de bedöms vara värdefulla.
*   **Autonom Research:** Identifierar kunskapsluckor och initierar självständigt forskning om relevanta ämnen.

## 🔧 Tool Use & Discovery

Sofia har avancerad logik för att hantera och optimera sin verktygsanvändning:

*   **Semantisk Verktygsmatchning:** Använder embeddings för att filtrera fram de mest relevanta verktygen baserat på användarens intent. Detta minskar token-användning och ökar fokus för LLM:en.
*   **Tool Performance Tracking:** `ToolTracker` mäter automatiskt framgångsgrad och exekveringstid för alla verktyg. Sofia kan använda `get_tool_stats` för att se vilka verktyg som fungerar bäst för specifika uppgifter.
*   **Tool Composition (Pipelines):** Med `create_pipeline` kan Sofia kedja ihop flera verktyg till ett nytt, återanvändbart makro-verktyg. Data flödar automatiskt mellan stegen i pipelinen.
*   **MCP-stöd:** Dynamisk upptäckt av verktyg via Model Context Protocol-servrar.

## 📚 Skill-system med Självlärande

Sofia kan skapa och förbättra sina egna skills:

*   **Auto-Skill Skapande:** `create_skill` genererar nya skills automatiskt från framgångsrika tillvägagångssätt.
*   **Skill-förfining:** `update_skill` förbättrar befintliga skills baserat på användningsfeedback.
*   **Kunskapsdestillering:** `distill_knowledge` komprimerar lärda erfarenheter till återanvändbar kunskap.

## 🔌 MCP-stöd (Model Context Protocol)

Sofia har inbyggd MCP-klient för anslutning till externa MCP-servrar:

*   Anslut till externa verktygs- och datakällor via standardiserat protokoll.
*   MCP-verktyg exponeras dynamiskt i agentens verktygsregister.
*   Konfigurera MCP-servrar via `config.json`.

## 🔒 Guardrails & Säkerhetsmodell

Sofia har ett fullständigt säkerhetssystem med flera lager:

*   **Workspace-restriktion:** Fil- och kommandoverktyg begränsas strikt till den konfigurerade workspace-sökvägen.
*   **Inputvalidering:** Konfigurerbar maxlängd och deny-patterns för att blockera skadliga meddelanden.
*   **Outputfiltrering:** Filtrerar känslig data (PII, hemligheter) från svar innan de visas.
*   **Prompt Injection-försvar:** LLM-baserad detektering och blockering av prompt injection-försök med konfigurerbar action (block/warn).
*   **Åtgärdsbekräftelse:** `self_modify`-verktyget kräver hash-bekräftelse innan högrisk-ändringar genomförs.
*   **Audit Trail:** Alla självmodifieringar loggas med tidsstämpel i `self_modifications.log`.

**Via Web UI:**
1.  Öppna Sofias Web UI → **System**.
2.  Klicka på fliken **Security**.
3.  Aktivera **Restrict to Workspace** och konfigurera guardrails.
4.  Inställningarna sparas automatiskt.

## 💓 Heartbeat (Bakgrundsagent)

Sofia kan automatiskt utföra uppgifter i bakgrunden enligt ett schema.

**Via Web UI:**
1.  Öppna Sofias Web UI → **System**.
2.  Klicka på fliken **Heartbeat**.
3.  Aktivera **Enable Heartbeat** och ange hur ofta agenten ska köra (i minuter).
4.  Ange **Active Hours** i formatet `09:00-17:00` — lämna tomt för 24/7.
5.  Välj **Active Days** — lämna tomt för att köra varje dag.
6.  Inställningarna sparas automatiskt.

## 🧭 Anpassa Sofias Personlighet

Sofias beteende, ton och personlighet styrs av två filer: **IDENTITY.md** och **SOUL.md**. Du kan enkelt redigera dem direkt i webbgränssnittet:

1.  **Starta Sofia:** `sofia gateway`
2.  **Öppna webbläsaren:** Surfa till `http://127.0.0.1:18795`
3.  **Gå till System** i vänstermenyn.
4.  Redigera **IDENTITY.md** (vem Sofia är) och **SOUL.md** (hur Sofia beter sig) direkt i textrutorna under fliken **Prompts**.
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

## 🎨 Web UI

Sofias webbgränssnitt är byggt med **HTMX** och **Go Templates** och har ett unikt brutalistiskt designtema med CRT-effekter:

*   **Chatt:** Realtidskonversation med streaming, markdown-rendering och filuppladdning (inkl. bilduppladdning för vision-modeller).
*   **Chatthistorik:** Sök, bläddra och återuppta tidigare konversationer med full sessionshantering.
*   **Agenter:** Hantera och konfigurera flera agenter med egna modeller, prompts och verktyg.
*   **Monitor:** Realtidsövervakning av agentaktivitet, verktygsanrop, systemstatus och pågående mål (Activity Monitor).
*   **System:** Alla inställningar och systemfunktioner samlade i vänstermenyn:
    *   **Identity** — Redigera IDENTITY.md och SOUL.md
    *   **Heartbeat** — Schemaläggning av bakgrundsagenten
    *   **Models** — Hantera AI-leverantörer och modeller
    *   **Comms** — Konfigurera Telegram, Discord m.m.
    *   **Integrations** — Aktivera och konfigurera externa integrationer (Porkbun, cPanel, GitHub, Google m.fl.)
    *   **Tools** — Lista över tillgängliga verktyg och deras beskrivningar
    *   **Skills** — Hantera installerade skills
    *   **Logs** — Realtidsloggar
    *   **Security** — Workspace-restriktioner och guardrails

## 🔄 AI-leverantörer

Sofia stödjer 20+ leverantörer via ett OpenAI-kompatibelt API-interface:

| Leverantör | Stöd |
|---|---|
| OpenAI (GPT-4o, o1, o3) | ✅ |
| Anthropic (Claude 4.5 Sonnet/Opus) | ✅ |
| Google Gemini (2.5 Pro/Flash) | ✅ |
| DeepSeek (V3, R1) | ✅ |
| Grok (xAI) | ✅ |
| MiniMax | ✅ |
| Moonshot (Kimi) | ✅ |
| Qwen (Alibaba) | ✅ |
| Zai | ✅ |
| GitHub Copilot | ✅ |
| Groq | ✅ |
| Together AI | ✅ |
| Fireworks AI | ✅ |
| OpenRouter | ✅ |
| Mistral AI | ✅ |

**Provider Fallback:** Konfigurera fallback-kedjor så att Sofia automatiskt byter till nästa leverantör om den primära misslyckas.

## 🔌 Integrationer

För att ge Sofia full kraft kan du koppla samman henne med externa tjänster.

### 📧 Google (Gmail & Kalender)

Sofia använder `gogcli` för att interagera med Google Services.

1.  **Installera gogcli:** Se till att `gog` finns i din PATH.
2.  **Autentisera:** Kör följande i terminalen och följ instruktionerna:
    ```bash
    gog login din.email@gmail.com
    ```
3.  **Aktivera i Sofia:**
    -   Öppna Sofias Web UI -> **System** -> **Integrations**.
    -   Aktivera **Google CLI** och ange sökvägen till `gog`.
    -   Konfigurera tillåtna kommandon (gmail, calendar, drive).
    -   Spara inställningarna.

### 🐙 GitHub

Sofia använder GitHub CLI (`gh`) för att hantera repon, PRs och kod.

1.  **Installera GitHub CLI:** `brew install gh` (macOS) eller besök [cli.github.com](https://cli.github.com).
2.  **Autentisera:** Kör följande i terminalen och följ instruktionerna:
    ```bash
    gh auth login
    ```
3.  **Aktivera i Sofia:**
    -   Öppna Sofias Web UI -> **System** -> **Integrations**.
    -   Aktivera **GitHub CLI**-switchen och klicka på **Save settings**.
    -   **Starta om Sofia** efter att du har sparat.

Sofia kan nu hantera PRs, issues, repon, workflows och mer via verktyget `github_cli`.

4.  **Git-identitet:** Se till att din lokala git är konfigurerad så att Sofia kan committa i ditt namn:
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

### 🐷 Porkbun (Domänhantering)

Sofia kan kontrollera tillgänglighet, registrera domäner och hantera DNS-poster via Porkbun API.

1.  **Hämta API-nycklar:** Logga in på [Porkbun](https://porkbun.com/account/api) och generera en "API Key" och "Secret API Key".
2.  **Konfigurera i Sofia:**
    -   Öppna Sofias Web UI -> **System** -> **Integrations**.
    -   Aktivera **Porkbun** och klistra in din `API Key` och `Secret API Key`.
    -   Spara inställningarna.

### 📦 cPanel (Webbhotell)

Sofia kan hantera ditt webbhotellskonto via cPanel UAPI: ladda upp filer, skapa databaser och hantera domäner.

1.  **Skapa API-token:** Logga in i cPanel -> **Security** -> **Manage API Tokens**. Skapa en ny token med de behörigheter du vill att Sofia ska ha.
2.  **Konfigurera i Sofia:**
    -   Öppna Sofias Web UI -> **System** -> **Integrations**.
    -   Aktivera **cPanel** och fyll i host, användarnamn och din API-token.
    -   Spara inställningarna.


## 🛠️ Komplett verktygslista

| Verktyg | Beskrivning |
|---|---|
| `file_read` / `file_write` / `file_edit` | Läsa, skriva och redigera filer |
| `shell` | Köra terminalkommandon |
| `web_browse` | Autonom webbsurfning via Playwright |
| `computer_use` | Styra datorns skärm, mus och tangentbord |
| `image_analyze` | Analysera lokala bilder via vision-LLM |
| `orchestrate` | Multi-agent-orkestrering med beroendegraf |
| `spawn` / `subagent` | Starta asynkrona/synkrona sub-agenter |
| `a2a` | Agent-to-Agent-kommunikation (send/receive/broadcast) |
| `plan` | Strukturerad uppgiftsplanering |
| `scratchpad` | Delad nyckel-värde-lagring mellan agenter |
| `cron` | Skapa och hantera schemalagda jobb |
| `message` | Skicka meddelanden till chattkanaler |
| `gogcli` | Google Gmail, Calendar och Drive |
| `knowledge_graph` | Kunskapsgraf — lägga till, söka och ta bort fakta och relationer |
| `manage_goals` | Skapa, uppdatera och spåra långsiktiga mål |
| `manage_triggers` | Skapa kontextmedvetna triggers för villkorliga handlingar |
| `create_skill` | Skapa nya skills automatiskt från framgångsrika tillvägagångssätt |
| `update_skill` | Förfina befintliga skills baserat på feedback |
| `distill_knowledge` | Destillera erfarenheter till återanvändbar kunskap |
| `self_modify` | Självmodifiering av kod/konfiguration med säkerhetsspärrar |
| `notify_user` | Push-meddelanden till användarens skrivbord |
| `get_tool_stats` | Hämta prestandadata och framgångsgrad för verktyg |
| `create_pipeline` | Skapa ett nytt makro-verktyg genom att kedja ihop befintliga verktyg |
| `mcp` | Anslut till externa MCP-servrar för dynamiska verktyg |
| `domain_name` | Hantera domäner via Porkbun (check, register, dns, nameservers) |
| `cpanel` | Hantera cPanel-webbhotell (filer, domäner, databaser, SSL) |


## 📊 Agentic AI Capability Scorecard

Sofia's feature coverage across 10 core agentic AI capability categories:

| Category | Score | Status |
|---|---|---|
| 🧠 Memory Architecture | **7/7** | ✅ Complete |
| 🔄 Self-Reflection & Self-Correction | **6/6** | ✅ Complete |
| 📋 Planning & Reasoning | **6/6** | ✅ Complete |
| 🤖 Multi-Agent Orchestration | **8/8** | ✅ Complete |
| 🔧 Tool Use & Discovery | **8/8** | ✅ Complete |
| 📚 Skill & Knowledge Acquisition | **7/7** | ✅ Complete |
| 🛡️ Guardrails, Safety & Trust | **8/8** | ✅ Complete |
| 🔄 Self-Improvement Mechanisms | **8/8** | ✅ Complete |
| 📡 Communication & Protocols | **8/9** | ✅ Strong |
| 🎯 Autonomy & Proactivity | **7/7** | ✅ Complete |
| **Total** | **73/74** | **99%** |

---
*Byggd för att accelerera utveckling. Din lokala AI-kollega.*
