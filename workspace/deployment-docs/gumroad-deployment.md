# Gumroad Deployment Process

## Översikt
Denna guide beskriver steg-för-steg hur teamet publicerar digitala produkter på Gumroad. Processen täcker allt från produktförberedelse till publicering och marknadsföring.

## Förutsättningar
- Gumroad-konto (med admin-behörighet)
- Klar produkt (PDF, digital fil, etc.)
- Produktinformation (beskrivning, pris, kategori)
- Produktbild/thumbnail (1200x630px rekommenderas)

## Steg 1: Förbereda produkten

### 1.1 Skapa produktfilen
- Se till att produktfilen är i rätt format (PDF, ZIP, etc.)
- Namnge filen tydligt (t.ex. `ai_prompts_product1.pdf`)
- Kontrollera filstorlek (Gumroad har gräns på 20GB per fil)

### 1.2 Skapa produktinformation
Använd mallen `product-info-template.md` för att samla:
- **Produktnamn**: Kort och lockande
- **Kort beskrivning**: 1-2 meningar som fångar uppmärksamhet
- **Detaljerad beskrivning**: Fullständig beskrivning i markdown-format
- **Prisstrategi**: Grundpris, kampanjpris, bundle-alternativ
- **Taggar**: Relevant nyckelord för sökbarhet
- **Kategori**: Välj under "Writing & Publishing" eller annan relevant

### 1.3 Skapa produktbild
- Thumbnail: 1200x630px (rekommenderat)
- Bild ska vara professionell och representera produkten
- Använd tydlig text och visuellt tilltalande design

### 1.4 Förbereda bonusmaterial (valfritt)
- Extra filer som bonus
- Användarguide
- Video-demo (max 5 minuter)

## Steg 2: Logga in på Gumroad

1. Gå till [gumroad.com](https://gumroad.com) och logga in
2. Klicka på "Create a product" (eller "Products" → "New product")

## Steg 3: Fyll i produktinformation

### 3.1 Basic Information
- **Product name**: Ange produktnamn
- **What are you selling?**: Välj "Digital product"
- **Price**: Ange pris i SEK eller USD
- **Currency**: Välj SEK för svenska kunder eller USD för internationellt

### 3.2 Description
- Klistra in den detaljerade beskrivningen från mallen
- Använd markdown för formatering
- Inkludera emojis för visuell uppdelning

### 3.3 Files
- Ladda upp huvudproduktfilen
- Ladda upp eventuella bonusfiler
- Ställ in "Max number of downloads per purchase" om begränsning behövs

### 3.4 Preview
- Ladda upp produktbilden som thumbnail
- Ytterligare bilder kan läggas till i "Gallery"

### 3.5 Settings
- **Visibility**: "Public" (för att visa i butik)
- **Purchase type**: "Fixed price" (ej "Pay what you want")
- **Quantity**: "Unlimited"
- **Category**: Välj lämplig kategori
- **Tags**: Lägg till relevanta taggar (minst 3)

## Steg 4: Konfigurera avancerade inställningar

### 4.1 Affiliate Program
- Aktivera affiliate program ("Enable affiliate program")
- Sätt provision (standard 7% fungerar bra)

### 4.2 Discount Codes
- Skapa rabattkoder för marknadsföring (t.ex. `SKAPA10` för 10% rabatt)
- Sätt begränsningar (antal användningar, utgångsdatum)

### 4.3 Email Collection
- Aktivera e-postinsamling ("Enable email collection")
- Koppla till e-postlista (Mailchimp, ConvertKit om konfigurerat)

### 4.4 Upsell
- Konfigurera upsell om flera produkter finns
- "After purchase" eller "Before purchase" upsell

### 4.5 Custom Domain
- Använd anpassad domän om tillgänglig
- Konfigurera DNS-inställningar hos domänleverantör

## Steg 5: Testa köpflödet

### 5.1 Preview Product
- Klicka "Preview" för att se hur produkten ser ut för besökare
- Kontrollera att all text är korrekt, bilder visas, etc.

### 5.2 Test Purchase
- Gör ett testköp med kreditkort eller PayPal
- Verifiera att nedladdningen fungerar
- Kontrollera att kvitto och e-post skickas

### 5.3 Refund Test Purchase
- Återbetala testköpet för att inte behålla pengar
- Gå till "Sales" → Hitta testköpet → "Refund"

## Steg 6: Publicera produkt

1. Klicka "Save" om du fyllt i all information
2. Klicka "Make product public" eller "Publish"
3. Produkten är nu live och tillgänglig för köp

## Steg 7: Post-Publicering

### 7.1 Dela produktlänken
- Kopiera produktens URL (t.ex. `https://gumroad.com/l/PRODUKTNAMN`)
- Dela i sociala medier, e-postlista, etc.

### 7.2 Uppdatera dokumentation
- Lägg till produktinformation i teamets produktregister
- Uppdatera prislistor och bundle-erbjudanden

### 7.3 Övervaka försäljning
- Gå till "Dashboard" för att se försäljningsstatistik
- Installera Gumroad Pixel för tracking (om använt)

## Checklista för Gumroad Deployment

### Före publicering
- [ ] Produktfil klar och testad
- [ ] Produktinformation komplett (namn, beskrivning, pris)
- [ ] Produktbild/thumbnail skapad
- [ ] Bonusmaterial förberett (valfritt)
- [ ] Prisstrategi fastställd
- [ ] Taggar och kategori valda

### Under publicering
- [ ] Inloggad på Gumroad
- [ ] Alla fält i formuläret ifyllda korrekt
- [ ] Fil uppladdad
- [ ] Affiliate program aktiverat
- [ ] Rabattkoder skapade (om behövs)
- [ ] E-postinsamling aktiverad

### Efter publicering
- [ ] Testköp genomfört
- [ ] Nedladdning verifierad
- [ ] Testköp återbetalt
- [ ] Produkt publicerad
- [ ] Länk delad med teamet
- [ ] Dokumentation uppdaterad

## Felsökning

### Vanliga problem
1. **Filen laddas inte upp**: Kontrollera filstorlek och format
2. **Pris visas inte korrekt**: Kontrollera valuta och decimaler
3. **Ingen försäljning**: Kontrollera att produkten är "Public" och inte "Draft"
4. **E-post skickas inte**: Kontrollera e-postinställningar och spamfilter

### Support
- Gumroad Help Center: https://help.gumroad.com
- Teamets interna dokumentation för specifika problem

## Automationsmöjligheter
För framtida effektivisering kan följande automationssteg övervägas:
1. **API-integration**: Använd Gumroad API för automatisk produktuppladdning
2. **Skript för batch-uppladdning**: För flera produkter samtidigt
3. **Automatisk prissättning**: Dynamiskt pris baserat på efterfrågan

## Versionhistorik
- **v1.0** (2026-03-19): Första versionen skapad av teamet
- **Uppdateringar**: Dokumentet uppdateras när Gumroad ändrar sitt gränssnitt eller när teamets processer utvecklas