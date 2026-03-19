# Plan: Skapa personaser för huvudmålgrupper

## Översikt
Skapa detaljerade personas för huvudmålgrupperna till produkten "AI Skrivprompts för Kreativt Skrivande". Personaserna ska hjälpa till att förstå kunderna bättre och anpassa marknadsföring, produktutveckling och kommunikation.

### Bakgrund
Enligt marknadsföringsplanen (marknadsforingsplan_prompts.md) är målgrupperna:
- Svenskspråkiga författare
- Bloggare
- Content creators
- Kreativa skribenter
- Hobbyförfattare
- Marknadsförare som behöver kreativt innehåll

Personaserna ska täcka dessa kategorier med 3-4 representativa profiler som fångar deras behov, motivationer, utmaningar och hur produkten kan lösa deras problem.

## Projekttyp: CONTENT
Detta är ett innehållsskapande projekt för marknadsföring och produktutveckling. Inget kodande krävs.

## Framgångskriterier
- [ ] 3-4 välutformade personas i Markdown-format
- [ ] Varje persona innehåller: namn, ålder, yrke, bakgrund, mål, utmaningar, behov, relation till produkten
- [ ] Personaserna täcker huvudkategorierna från marknadsföringsplanen
- [ ] Personaserna används som referens för framtida marknadsföringsbeslut
- [ ] Alla personas sparas i en strukturerad mapp `personas/`

## Teknisk stack
- **Format:** Markdown (.md)
- **Verktyg:** Ingen specifik mjukvara krävs
- **Organisation:** Filbaserad struktur

## Filstruktur
```
personas/
├── README.md           # Översikt över personas och hur de ska användas
├── persona-1-forfattare.md
├── persona-2-bloggare-content-creator.md
├── persona-3-hobbyforfattare.md
└── persona-4-marknadsforare.md
```

## Uppgiftsuppdelning

### Task 1: Analysera målgruppsdata
**Agent:** project-planner (jag)  
**Skills:** brainstorming  
**Prioritet:** Hög  
**Beroenden:** Ingen  
**INPUT:** Marknadsföringsplan (marknadsforingsplan_prompts.md)  
**OUTPUT:** Lista över målgruppskategorier med nyckelattribut  
**VERIFY:** Listan innehåller minst 6 kategorier med beskrivningar

### Task 2: Skapa persona-mall
**Agent:** subagent (content-specialist)  
**Skills:** clean-code (för strukturerad dokumentation)  
**Prioritet:** Hög  
**Beroenden:** Task 1  
**INPUT:** Målgruppslista från Task 1  
**OUTPUT:** Standardiserad persona-mall i Markdown med obligatoriska fält  
**VERIFY:** Mallen innehåller alla nödvändiga sektioner (namn, demografi, bakgrund, mål, utmaningar, behov, lösning)

### Task 3: Skapa individuella personas
**Agent:** subagent (content-specialist)  
**Skills:** brainstorming, clean-code  
**Prioritet:** Hög  
**Beroenden:** Task 2  
**INPUT:** Persona-mall och målgruppslista  
**OUTPUT:** 3-4 färdiga personas i separata Markdown-filer  
**VERIFY:** Varje persona är unik, välmotiverad och täcker en distinkt målgruppskategori

### Task 4: Sammanställning och dokumentation
**Agent:** subagent (content-specialist)  
**Skills:** clean-code  
**Prioritet:** Medel  
**Beroenden:** Task 3  
**INPUT:** Individuella persona-filer  
**OUTPUT:** README.md som förklarar personas och deras användning  
**VERIFY:** README.md länkar till alla personas och innehåller användningsriktlinjer

### Task 5: Validering mot marknadsföringsplan
**Agent:** project-planner  
**Skills:** brainstorming  
**Prioritet:** Medel  
**Beroenden:** Task 4  
**INPUT:** Alla persona-filer och marknadsföringsplan  
**OUTPUT:** Valideringsrapport som bekräftar att personas täcker målgrupperna  
**VERIFY:** Rapport visar att varje målgruppskategori har motsvarande persona

## Fas X: Verifiering
För att markera projektet som slutfört måste följande kontroller genomföras:

- [ ] Alla personas finns i `personas/`-mappen
- [ ] Varje persona-fil innehåller alla obligatoriska sektioner
- [ ] Personaserna är realistiska och baserade på målgruppsanalys
- [ ] README.md är informativ och användbar
- [ ] Valideringsrapport visar fullständig täckning

### Verifieringskommandon
```bash
# Kontrollera att alla filer finns
ls -la personas/

# Kontrollera att varje persona-fil har minst 500 tecken (ungefärligt)
wc -c personas/*.md

# Validera att README.md länkar till alla personas
grep -c "persona-" personas/README.md
```

## Risker och lösningar
| Risk | Sannolikhet | Påverkan | Lösning |
|------|-------------|----------|---------|
| Personas blir för generiska | Medel | Medel | Använd specifika detaljer från marknadsföringsplanen |
| Otillräcklig data om målgrupper | Låg | Hög | Antagelser baserade på generell kunskap om målgrupper |
| För många personas | Låg | Låg | Begränsa till 3-4 huvudsakliga profiler |

## Tidsestimat
- Task 1: 10 minuter
- Task 2: 15 minuter  
- Task 3: 30 minuter
- Task 4: 10 minuter
- Task 5: 10 minuter
**Total:** ~75 minuter (1 timme 15 minuter)

---

## Nästa steg
Efter godkännande av denna plan, starta Task 1 omedelbart.