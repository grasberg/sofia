# Deploy till Netlify

Denna statiska landningssida är redo att deployas på Netlify.

## Steg för deploy

### Alternativ 1: Dra och släpp (Drag & Drop)
1. Gå till [Netlify Drop](https://app.netlify.com/drop)
2. Dra hela mappen `ai-prompts-product` till det markerade området
3. Netlify skapar automatiskt en URL för din site

### Alternativ 2: Via Git (rekommenderas för uppdateringar)
1. Skapa ett Git-repository med dessa filer
2. Logga in på [Netlify](https://app.netlify.com)
3. Klicka på "New site from Git"
4. Välj ditt repository och gren (main/master)
5. Lämna build-inställningar som standard (Netlify kommer att känna av `netlify.toml`)
6. Klicka "Deploy site"

## Konfiguration

Filen `netlify.toml` innehåller:
- **publish**: `.` (aktuell mapp är root)
- **redirects**: Alla förfrågningar dirigeras till index.html (för SPA-liknande beteende)
- **Security headers**: Skydd mot XSS, clickjacking m.m.
- **Cache headers**: Optimal caching för CSS och HTML

## Anpassning

### Byta bild
Bilden i hero-sektionen kommer från Unsplash. För att byta bild:
1. Ersätt `src`-attributet i `<div class="hero-image">` i index.html
2. Använd en absolut URL till din bild eller ladda upp en bild till Netlify och använd relativ sökväg

### Ändra länkar
Gumroad-länken i prissektionen pekar just nu på `https://gumroad.com/l/ai-prompts`. Uppdatera detta till din faktiska Gumroad-produktlänk.

### Ytterligare optimering
För bättre prestanda kan du:
1. Minifiera HTML, CSS och JavaScript
2. Optimera bilder innan uppladdning
3. Aktivera Netlify's bildoptimering (via Netlify admin)

## Support

Om du stöter på problem med deployment, kontrollera:
- Att `index.html` och `style.css` finns i root-mappen
- Att `netlify.toml` har korrekt syntax
- Netlify's [deployment documentation](https://docs.netlify.com/)## Current Deployment (2026-03-19)

The site has been deployed to Netlify using the Netlify CLI.

**Deployment Details:**
- **Site URL:** https://clinquant-arithmetic-f892d2.netlify.app
- **Admin URL:** https://app.netlify.com/projects/clinquant-arithmetic-f892d2
- **Site ID:** 86c1fd45-d5d9-4e46-9b91-6d5d8b0bff23
- **Deploy ID:** 69bbd3e2c33f71c6101c7a86
- **Team:** Sofia (Free plan)
- **Deployment Method:** Manual CLI deployment (no Git integration yet)

**Next Steps:**
- Connect custom domain via Netlify DNS or external DNS provider
- Set up Git repository for continuous deployment
- Add environment variables if needed (e.g., for analytics, forms)