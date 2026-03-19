# Fält för landningssideskonfiguration

## Övergripande produktinformation
- `product_id`: Unik identifierare (sträng)
- `product_name`: Produktens namn (sträng)
- `product_slug`: URL-vänlig version av namnet (sträng)
- `product_type`: Typ av produkt (digital nedladdning, kurs, konsultation, etc.)
- `category`: Kategori (t.ex. "Writing Tools", "Business Tools")
- `tags`: Lista av taggar (array)
- `language`: Språk (t.ex. "sv", "en")
- `status`: Status (draft, published, archived)

## Pris och betalning
- `currency`: Valuta (SEK, USD, EUR)
- `base_price`: Grundpris (nummer)
- `discount_price`: Kampanjpris (nummer, valfritt)
- `discount_end_date`: Kampanjens slutdatum (ISO-datum, valfritt)
- `payment_type`: Betalningstyp (one_time, subscription, pay_what_you_want)
- `subscription_interval`: Intervall för prenumeration (monthly, yearly, valfritt)
- `affiliate_percentage`: Provisionsprocent för affiliate (nummer, valfritt)

## Tiers/nivåer (för produkter med flera nivåer)
- `tiers`: Array av tier-objekt
  - `tier_id`: Unik identifierare för nivå
  - `tier_name`: Namn på nivå (t.ex. "Starter", "Professional")
  - `price`: Pris för denna nivå
  - `currency`: Valuta (kan ärvas från toppnivå)
  - `features`: Lista med funktioner (array av strängar)
  - `description`: Kort beskrivning av nivån
  - `cta_text`: Text för köpknappen (t.ex. "Köp Starter")
  - `highlight`: Boolean om denna nivå ska framhävas (valfritt)

## Beskrivningar
- `short_description`: Kort beskrivning för preview (1-2 meningar)
- `long_description`: Lång beskrivning i markdown eller HTML
- `benefits`: Lista med huvudfördelar (array av strängar)
- `target_audience`: Målgruppbeskrivning (sträng eller array)
- `how_it_works`: Steg-för-steg-beskrivning (array eller markdown)

## Leverans och tillgång
- `delivery_method`: Leveransmetod (digital_download, email, platform_access)
- `delivery_format`: Filformat (PDF, ZIP, video, etc.)
- `delivery_instant`: Boolean för omedelbar leverans
- `access_duration`: Tillgångstid (lifetime, 1_year, etc.)
- `updates_included`: Boolean om uppdateringar ingår
- `license_type`: Licenstyp (personal, commercial, resell_rights)

## Media och resurser
- `hero_image`: Sökväg till hero-bild
- `product_images`: Lista av bildsökvägar
- `demo_video_url`: URL till demovideo (valfritt)
- `thumbnail_image`: Sökväg till thumbnail (för plattformar)
- `preview_file`: Sökväg till förhandsvisningsfil (valfritt)

## Sektioner för landningssidan
- `sections`: Array av sektionsobjekt
  - `section_id`: Unik identifierare
  - `section_type`: Typ av sektion (hero, features, pricing, testimonials, faq, how_it_works, cta, guarantee)
  - `title`: Rubrik för sektionen
  - `content`: Innehåll (kan vara sträng, markdown, eller array beroende på typ)
  - `order`: Ordningsnummer för sortering
  - `visible`: Boolean om sektionen är synlig

### Hero-sektion
- `section_type: "hero"`
- `title`: Huvudrubrik
- `subtitle`: Underrubrik
- `primary_cta_text`: Text för primär CTA
- `primary_cta_url`: URL för primär CTA (t.ex. köplänk)
- `secondary_cta_text`: Text för sekundär CTA (valfritt)
- `secondary_cta_url`: URL för sekundär CTA (t.ex. demo)

### Features-sektion
- `section_type: "features"`
- `features`: Array av feature-objekt
  - `title`: Funktionsrubrik
  - `description`: Beskrivning
  - `icon`: Ikonnamn eller sökväg (valfritt)

### Pricing-sektion
- `section_type: "pricing"`
- `show_tiers`: Boolean om tiers ska visas (eller bara ett pris)
- `tier_comparison`: Boolean om jämförelsetabell ska visas

### Testimonials-sektion
- `section_type: "testimonials"`
- `testimonials`: Array av testimonial-objekt
  - `author`: Författarens namn
  - `role`: Roll/titel (valfritt)
  - `content`: Citatet
  - `avatar_url`: URL till avatar (valfritt)
  - `rating`: Betyg 1-5 (valfritt)

### FAQ-sektion
- `section_type: "faq"`
- `faqs`: Array av FAQ-objekt
  - `question`: Frågan
  - `answer`: Svaret (markdown eller text)

### Guarantee-sektion
- `section_type: "guarantee"`
- `guarantee_text`: Text om garantin
- `guarantee_days`: Antal dagar (t.ex. 30)

## Ytterligare inställningar
- `utm_campaign`: UTM-kampanjnamn för spårning
- `affiliate_code`: Affiliate-kod (valfritt)
- `pixel_ids`: Array av tracking pixel ID:n (Facebook, Google, etc.)
- `email_integration`: Boolean om e-postregistrering ska aktiveras
- `email_list`: Namn på e-postlista (valfritt)

## Metadata för plattformar (Gumroad, Stripe, etc.)
- `platform_specific`: Objekt med plattformsspecifika inställningar
  - `gumroad`: Gumroad-specifika fält
  - `stripe`: Stripe-specifika fält
  - `sendowl`: SendOwl-specifika fält