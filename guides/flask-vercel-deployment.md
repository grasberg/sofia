# Vercel Deployment Guide för Flask/Python Backend

## Introduktion

Denna guide täcker hur du deployar en Flask-applikation på Vercel, en modern serverless plattform. Vercel erbjuder snabba deployment, automatisk scaling, och enkel konfiguration för Python-applikationer.

### För vem är denna guide?

- Utvecklare som vill hosta Flask-applikationer utan serverhantering
- Team som behöver snabba CI/CD-pipelines
- Projekt som kräver global edge-nära distribution

## Förutsättningar

- Ett Vercel-konto (gratis nivå tillgänglig)
- Python 3.9 eller senare installerat lokalt
- Git installerat
- En Flask-applikation redo för deployment
- Grundläggande kunskaper om terminal och Python-miljöer

## Projektstruktur för Flask på Vercel

En typisk Flask-applikation för Vercel behöver följande filstruktur:

```
my-flask-app/
├── api/
│   └── index.py          # WSGI entry point
├── requirements.txt      # Python dependencies
├── vercel.json          # Vercel configuration
├── .python-version      # Python version (optional)
└── .env.example         # Environment variables template
```

### WSGI Entry Point

Vercel förväntar sig att din Flask-app finns i `/api/index.py` (för API-routes) eller i rotens `index.py` (för serverless functions). Den viktigaste filen är `api/index.py`:

```python
from flask import Flask, jsonify

app = Flask(__name__)

@app.route('/')
def home():
    return jsonify({'message': 'Flask on Vercel!'})

@app.route('/api/hello')
def hello():
    return jsonify({'message': 'Hello from Vercel!'})

# Export app för Vercel
if __name__ == '__main__':
    app.run(debug=True)
else:
    # För Vercel serverless
    api = app
```

## Konfiguration

### vercel.json

`vercel.json` styr hur Vercel bygger och deployar din applikation:

```json
{
  "builds": [
    {
      "src": "api/index.py",
      "use": "@vercel/python"
    }
  ],
  "routes": [
    {
      "src": "/(.*)",
      "dest": "api/index.py"
    }
  ]
}
```

### requirements.txt

Lista alla Python-paket som behövs:

```
Flask==2.3.3
gunicorn==20.1.0
python-dotenv==1.0.0
Werkzeug==2.3.7
```

### Miljövariabler

Vercel låter dig ställa in miljövariabler via webbgränssnittet eller `vercel env` CLI:

```bash
vercel env add DATABASE_URL
vercel env add SECRET_KEY production
```

I din Flask-app, ladda miljövariabler:

```python
import os
from dotenv import load_dotenv

load_dotenv()  # Laddar .env lokalt

SECRET_KEY = os.environ.get('SECRET_KEY', 'fallback-secret')
DATABASE_URL = os.environ.get('DATABASE_URL')
```

## Deployment-steg

### 1. Installera Vercel CLI

```bash
npm i -g vercel
# eller
brew install vercel
```

### 2. Logga in

```bash
vercel login
```

### 3. Initiera projekt

```bash
vercel init
```

### 4. Deploya till preview

```bash
vercel
```

### 5. Deploya till production

```bash
vercel --prod
```

## Avancerade ämnen

### Anpassade Python-versioner

Använd `.python-version` fil för att specificera Python-version:

```
3.11
```

### Serverless Functions Configuration

I `vercel.json` kan du konfigurera minne, max duration och region:

```json
{
  "functions": {
    "api/index.py": {
      "maxDuration": 10,
      "memory": 1024,
      "runtime": "python3.11"
    }
  }
}
```

### Statiska filer

För att servera statiska filer, lägg till en `public` mapp eller konfigurera routes:

```json
{
  "routes": [
    {
      "src": "/static/(.*)",
      "dest": "/public/static/$1"
    },
    {
      "src": "/(.*)",
      "dest": "api/index.py"
    }
  ]
}
```

## Felsökning

### Vanliga fel och lösningar

1. **"ModuleNotFoundError: No module named 'flask'"**  
   Se till att `requirements.txt` innehåller Flask och att du har kört `vercel --prod` efter att ha lagt till nya paket.

2. **"Function Invocation Timeout"**  
   Öka `maxDuration` i `vercel.json` (max 15 sekunder för hobby-plan, 60 för Pro).

3. **"Environment variables missing"**  
   Kontrollera att du har lagt till miljövariabler för rätt miljö (development, preview, production).

4. **"404 Not Found" för routes**  
   Se till att din `vercel.json` routes är korrekt konfigurerade och att `api/index.py` exporterar `app` eller `api` variabeln.

5. **Python version mismatch**  
   Använd `.python-version` fil för att specificera version, t.ex. `3.11`.

### Loggar och debugging

```bash
# Visa deployment-loggar
vercel logs <deployment-url>

# Live-logg för en deployment
vercel logs --follow

# Visa funktionsinvokationer
vercel logs --functions
```

## Säkerhetsbest Practices

### 1. Miljövariabler och hemligheter

**Använd aldrig hårdkodade hemligheter i koden.** Alla känsliga värden som API-nycklar, databaslösenord, och session secrets måste lagras som miljövariabler.

```python
# ❌ DÅLIGT
app.config['SECRET_KEY'] = 'my-hardcoded-secret'

# ✅ BRA
app.config['SECRET_KEY'] = os.environ.get('SECRET_KEY')
```

**Vercel-miljövariabler:**
- Använd `vercel env add` för att lägga till hemligheter
- Separera miljövariabler per miljö (development, preview, production)
- Använd `.env.example` för att dokumentera nödvändiga variabler utan värden

### 2. Dependency Scanning

**Håll dina dependencies uppdaterade och säkra:**
- Kör regelbundet `pip list --outdated` för att identifiera föråldrade paket
- Använd `safety check` eller `pip-audit` för att söka efter kända sårbarheter
- Överväg att använda Dependabot eller Renovate för automatiska uppdateringar

```bash
# Installera säkerhetsskanner
pip install safety

# Kör säkerhetskontroll
safety check -r requirements.txt
```

### 3. Flask-specifik säkerhet

**Aktivera CSRF-skydd:**  
Om du använder Flask-WTF eller liknande, se till att CSRF-skydd är aktiverat för alla POST-formulär.

```python
from flask_wtf.csrf import CSRFProtect

csrf = CSRFProtect(app)
```

**Säker headers:**  
Använd Flask-Talisman eller ställ in säkerhetsheaders manuellt:

```python
@app.after_request
def add_security_headers(response):
    response.headers['X-Content-Type-Options'] = 'nosniff'
    response.headers['X-Frame-Options'] = 'DENY'
    response.headers['X-XSS-Protection'] = '1; mode=block'
    return response
```

**Session security:**  
- Använd `SESSION_COOKIE_SECURE = True` i production
- Använd `SESSION_COOKIE_HTTPONLY = True`
- Använd `SESSION_COOKIE_SAMESITE = 'Lax'` eller `'Strict'`

```python
app.config.update(
    SESSION_COOKIE_SECURE=True,
    SESSION_COOKIE_HTTPONLY=True,
    SESSION_COOKIE_SAMESITE='Lax'
)
```

### 4. Rate Limiting

**Skydda dina endpoints mot brute force-attacker:**  
Använd Flask-Limiter för att begränsa antalet requests per IP.

```python
from flask_limiter import Limiter
from flask_limiter.util import get_remote_address

limiter = Limiter(
    get_remote_address,
    app=app,
    default_limits=["200 per day", "50 per hour"]
)

@app.route('/api/login')
@limiter.limit("10 per minute")
def login():
    # login logik
```

### 5. Input Validation och Sanitization

**Validera all användarinput:**  
Använd WTForms eller liknande för validering.

```python
from flask_wtf import FlaskForm
from wtforms import StringField, validators

class ContactForm(FlaskForm):
    email = StringField('Email', [validators.Email()])
    message = StringField('Message', [validators.Length(min=10)])
```

**Escape output:**  
Använd Jinja2's auto-escaping (aktiverat som standard). För manuell escaping:

```python
from markupsafe import escape

@app.route('/user/<username>')
def show_user(username):
    # Escapar användarinput för att förhindra XSS
    return f'User: {escape(username)}'
```

### 6. Databas-säkerhet

**Använd parameteriserade queries:**  
Använd aldrig string interpolation för SQL-queries.

```python
# ❌ FARA FÖR SQL INJECTION
cursor.execute(f"SELECT * FROM users WHERE id = {user_id}")

# ✅ SÄKERT
cursor.execute("SELECT * FROM users WHERE id = %s", (user_id,))
```

**Använd ORM:**  
Överväg att använda SQLAlchemy eller annan ORM som hanterar escaping automatiskt.

### 7. Vercel-specifik säkerhet

**Function isolation:**  
Varje serverless function körs i en isolerad miljö. Utnyttja detta genom att separera känslig logik i olika functions.

**Environment-specific configurations:**  
Använd Vercel's miljövariabler för att ha olika inställningar för development, preview, och production.

**Access control:**  
Använd Vercel's Access Controls för att begränsa åtkomst till preview-deployments.

### 8. Logging och Monitoring

**Logga säkerhetsrelaterade händelser:**  
Men var försiktig med att inte logga känslig information.

```python
import logging

security_logger = logging.getLogger('security')

@app.route('/api/login', methods=['POST'])
def login():
    # Login logik
    if failed_attempt:
        security_logger.warning(f'Failed login attempt from {request.remote_addr}')
```

**Använd Vercel Analytics:**  
Övervaka din applikations prestanda och upptäck ovanliga mönster.

### 9. Regular Security Audits

**Gör regelbundna säkerhetskontroller:**
- Granska dependencies månadsvis
- Testa endpoints med verktyg som OWASP ZAP
- Granska koden för potentiella sårbarheter
- Håll dig uppdaterad om Flask- och Python-säkerhetsuppdateringar

## Best Practices

### Prestandaoptimering

1. **Använd gunicorn workers** för lokal utveckling och testning
2. **Minifiera statiska filer** innan deployment
3. **Aktivera caching** där det är möjligt
4. **Använd Vercel's Edge Network** för global distribution

### Kodkvalitet

1. **Skriv tester** för kritiska funktioner
2. **Använd type hints** för bättre dokumentation
3. **Följ PEP 8** stilguide
4. **Dokumentera API:er** med Swagger/OpenAPI

### CI/CD

1. **Automatisera tester** före varje deployment
2. **Använd preview deployments** för att testa ändringar
3. **Ställ in automatiska deployment** från main-branch
4. **Använd semantisk versionering** för releaser

## Resurser

### Officiella länkar

- [Vercel Python Documentation](https://vercel.com/docs/concepts/functions/serverless-functions/runtimes/python)
- [Flask Documentation](https://flask.palletsprojects.com/)
- [Vercel CLI Reference](https://vercel.com/docs/cli)

### Tutorials och exempel

- [Vercel Flask Example Repository](https://github.com/vercel/examples/tree/main/python/flask)
- [Deploy Flask to Vercel (Blog Post)](https://vercel.com/guides/deploying-flask-with-vercel)
- [Flask Security Checklist](https://flask.palletsprojects.com/en/2.3.x/security/)

### Verktyg

- [pip-audit: Python dependency scanner](https://pypi.org/project/pip-audit/)
- [safety: Security check for Python dependencies](https://pyup.io/safety/)
- [Flask-Talisman: Security headers](https://github.com/GoogleCloudPlatform/flask-talisman)

## Slutsats

Att deploya Flask på Vercel erbjuder en kraftfull kombination av Python's enkelhet och Vercel's moderna infrastruktur. Genom att följa denna guide och säkerhetsbest practices kan du skapa säkra, skalbara och underhållbara applikationer.

Kom ihåg att säkerhet är en pågående process – regelbundna uppdateringar och kontroller är avgörande för att hålla din applikation säker över tid.

---

*Senast uppdaterad: Mars 2026*  
*Guide skapad av [Ditt Namn/Team]*