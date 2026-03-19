# Guide för e-post-DNS konfiguration (MX, SPF, DKIM, DMARC)

## Översikt

För att kunna skicka och ta emot e-post med din domän (t.ex. `dittforetag.se`) måste du konfigurera följande DNS-poster:

1. **MX-poster** (Mail Exchange) - Anger vilka servrar som tar emot e-post för din domän
2. **SPF-post** (Sender Policy Framework) - Anger vilka servrar som får skicka e-post från din domän
3. **DKIM-post** (DomainKeys Identified Mail) - Digitale signaturer som verifierar att e-postmeddelanden inte har förfalskats
4. **DMARC-post** (Domain-based Message Authentication, Reporting & Conformance) - Policy för hur mottagare ska hantera e-post som inte klarar SPF/DKIM

## 1. MX-poster (Mail Exchange)

MX-poster pekar ut vilka e-postservrar som ska ta emot e-post för din domän.

### Exempel för Google Workspace:
```
Namn/host: @ (eller tomt)
Typ: MX
Prioritet: 1
Värde: aspmx.l.google.com
TTL: 3600 (1 timme)
```

Flera MX-poster (Google rekommenderar 5):
```
1 aspmx.l.google.com
5 alt1.aspmx.l.google.com
5 alt2.aspmx.l.google.com
10 alt3.aspmx.l.google.com
10 alt4.aspmx.l.google.com
```

### Exempel för Microsoft 365:
```
@ MX 0 yourdomain-com.mail.protection.outlook.com
```

## 2. SPF-post (Sender Policy Framework)

SPF-posten är en TXT-post som anger vilka IP-adresser eller servrar som får skicka e-post från din domän.

### Grundläggande SPF för Google Workspace:
```
v=spf1 include:_spf.google.com ~all
```

### För Microsoft 365:
```
v=spf1 include:spf.protection.outlook.com -all
```

### För egen server (ändra IP):
```
v=spf1 ip4:192.0.2.1 -all
```

### För flera tjänster:
```
v=spf1 include:_spf.google.com include:spf.protection.outlook.com ip4:192.0.2.1 -all
```

## 3. DKIM-post (DomainKeys Identified Mail)

DKIM använder en offentlig nyckel publicerad i DNS för att verifiera signaturer i e-posthuvuden.

### Google Workspace:
1. Generera DKIM-nyckel i Google Admin Console
2. Lägg till TXT-post:
```
Namn: google._domainkey
Typ: TXT
Värde: v=DKIM1; k=rsa; p=MIGfMA0GCSqGSIb3DQEBAQUAA4GNADCBiQKBgQC4...
```

### Microsoft 365:
1. Generera DKIM i Exchange Admin Center
2. Lägg till två TXT-poster (selector1 och selector2):
```
selector1._domainkey.yourdomain.com TXT "v=DKIM1; k=rsa; p=MIGfMA0..."
selector2._domainkey.yourdomain.com TXT "v=DKIM1; k=rsa; p=MIGfMA0..."
```

## 4. DMARC-post

DMARC anger hur mottagare ska hantera e-post som inte klarar SPF/DKIM-validering.

### Rekommenderad DMARC-post (övervakningsläge):
```
_dmarc.yourdomain.com TXT "v=DMARC1; p=none; rua=mailto:dmarc-reports@yourdomain.com"
```

### Strängare DMARC (kräver SPF/DKIM):
```
_dmarc.yourdomain.com TXT "v=DMARC1; p=reject; rua=mailto:dmarc-reports@yourdomain.com"
```

## Valideringsverktyg

När du har konfigurerat DNS-poster, använd dessa verktyg för att verifiera:

1. **MX Lookup**: https://mxtoolbox.com/
2. **SPF Validator**: https://www.kitterman.com/spf/validate.html
3. **DKIM Validator**: https://www.dmarcanalyzer.com/dkim/dkim-check/
4. **DMARC Validator**: https://dmarc.org/tools/

## Vanliga problem och lösningar

### Problem 1: E-post levereras inte
- Kontrollera att MX-poster är korrekt prioriterade
- Vänta upp till 48 timmar för DNS-propagation
- Kontrollera att domänen inte är blacklistad

### Problem 2: E-post hamnar i spam
- SPF, DKIM och DMARC måste vara korrekt konfigurerade
- Använd reverse DNS (PTR-poster) om du har egen server
- Undvik att skicka massutskick från ny domän

### Problem 3: DKIM validering misslyckas
- Kontrollera att DKIM-selectorn matchar i e-posthuvudet
- Kontrollera att den offentliga nyckeln är korrekt kopierad (inga radbrytningar)
- Vänta på DNS-propagation

## Snabbkontrollista

- [ ] MX-poster konfigurerade med rätt prioritet
- [ ] SPF-post inkluderar alla legitima avsändarservrar
- [ ] DKIM-nyckel genererad och TXT-post tillagd
- [ ] DMARC-post konfigurerad (åtminstone i övervakningsläge)
- [ ] Alla poster har TTL på 3600 eller lägre för snabbare uppdatering
- [ ] Verifierat med valideringsverktyg

## Ytterligare tips

1. **TTL-värden**: Sätt TTL till 300-3600 sekunder under konfiguration, öka sedan till 86400 (24h) när allt fungerar.
2. **DNS-hantering**: Om du använder Cloudflare, aktivera "Proxy" endast för webbtrafik (A/CNAME), aldrig för e-postrelaterade poster (MX, TXT).
3. **Säkerhetskopiera**: Dokumentera alla DNS-poster innan du ändrar.
4. **Testa**: Skicka testmeddelanden till Gmail, Outlook och Yahoo för att se om de levereras till inkorgen.

---

*Senast uppdaterad: 2026-03-19*