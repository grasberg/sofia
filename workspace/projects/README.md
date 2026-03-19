# Projects Hosting Infrastructure

Denna mapp innehåller olika projekt med tillhörande hosting-konfigurationer och deployment-guider.

## 📁 Projektstruktur

| Projekt | Typ | Hosting-lösning | Status |
|---------|------|-----------------|--------|
| [`ai-prompts-product/`](ai-prompts-product/) | Statisk landing page | Netlify | ✅ Konfigurerad (netlify.toml, DEPLOY.md) |
| [`sofia-ops-app/`](../sofia-ops-app/) | Next.js webapp | Vercel | ⚠️ Vercel config behövs |
| [`outreach-saas/`](outreach-saas/) | Next.js app (tom) | Vercel | 📝 Utvecklingskandidat |
| [`affiliate-system/`](affiliate-system/) | Laravel PHP app | Traditional hosting | 🛠️ Kräver PHP/MySQL hosting |
| [`ai-content-service/`](ai-content-service/) | Innehållstjänst | TBD | 📋 Planering |

## 🚀 Deployment Guides

1. **[Deployment Guide](deployment-guide.md)** – Komplett guide för att deploya både Next.js-appar till Vercel och statiska sidor till Netlify
2. **[Hosting Overview](HOSTING.md)** – Översikt över alla projekt och deras hosting-lösningar
3. **[Netlify Deployment](ai-prompts-product/DEPLOY.md)** – Specifika instruktioner för ai-prompts-product

## 🛠️ Snabbstart

### För Next.js-projekt (Vercel)
```bash
# 1. Skapa vercel.json i projektrot
# 2. Push till Git
# 3. Importera i Vercel Dashboard
```

### För statiska sidor (Netlify)
```bash
# 1. Använd netlify.toml som mall
# 2. Push till Git  
# 3. Importera i Netlify Dashboard
```

## 📋 Checklista för nya projekt

- [ ] Identifiera projekttyp (Next.js / statisk / Laravel)
- [ ] Skapa lämplig konfigurationsfil (vercel.json / netlify.toml)
- [ ] Sätt upp Git-repository
- [ ] Konfigurera CI/CD (GitHub Actions)
- [ ] Deploy första versionen
- [ ] Lägg till anpassad domän och SSL

## 🔧 Konfigurationsfiler

### Vercel (Next.js) – `vercel.json`
```json
{
  "buildCommand": "npm run build",
  "devCommand": "npm run dev", 
  "installCommand": "npm install",
  "outputDirectory": ".next",
  "framework": "nextjs",
  "regions": ["arn1"]
}
```

### Netlify (statisk) – `netlify.toml`
```toml
[build]
  publish = "."
  command = "echo 'No build step'"

[[redirects]]
  from = "/*"
  to = "/index.html"
  status = 200
```

## 📞 Support

- **Tekniska frågor:** Läs [Deployment Guide](deployment-guide.md)
- **Hosting-val:** Se [Hosting Overview](HOSTING.md)  
- **Specifika projekt:** Se projektmappens README/DEPLOY-filer

---

**Senast uppdaterad:** 2026-03-19  
**Nästa steg:** Deploya sofia-ops-app till Vercel och ai-prompts-product till Netlify