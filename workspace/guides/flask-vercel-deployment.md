# Flask Deployment på Vercel

Den här guiden visar hur du deployar en Flask-applikation på Vercel, en serverless plattform som passar bra för Python-appar.

## Förutsättningar

- Ett Vercel-konto (gratis tier räcker)
- Git-repository med din Flask-app
- Python 3.8 eller senare
- Vercel CLI (valfritt)

## Steg 1: Flask-applikationsstruktur

Din Flask-app måste vara strukturerad så att den kan köras som en serverless funktion. Här är en rekommenderad struktur:

```
din-app/
├── api/
│   └── index.py          # Huvudentry-point för Vercel
├── requirements.txt      # Python-beroenden
├── vercel.json          # Vercel-konfiguration
└── .python-version      # Python-version (valfritt)
```

## Steg 2: Skapa entry-point för Vercel

I `api/index.py`:

```python
from flask import Flask, jsonify
import sys
import os

# Lägg till projektets root i Python-sökvägen
sys.path.insert(0, os.path.dirname(os.path.dirname(os.path.abspath(__file__))))

# Skapa Flask-app
app = Flask(__name__)

@app.route('/')
def hello():
    return jsonify({"message": "Flask på Vercel!", "status": "ok"})

@app.route('/api/health')
def health():
    return jsonify({"status": "healthy"})

# Vercel kräver en variabel som heter `app` eller `application`
# Det är det här objektet som Vercel kommer att använda
application = app
```

## Steg 3: Konfigurera Python-beroenden

I `requirements.txt`:

```
Flask==2.3.3
gunicorn==20.1.0
Werkzeug==2.3.7
```

## Steg 4: Vercel-konfiguration

I `vercel.json`:

```json
{
  "functions": {
    "api/index.py": {
      "maxDuration": 10
    }
  },
  "routes": [
    {
      "src": "/(.*)",
      "dest": "/api/index.py"
    }
  ],
  "builds": [
    {
      "src": "api/index.py",
      "use": "@vercel/python"
    }
  ]
}
```

## Steg 5: Deploy via Vercel Dashboard

### Alternativ 1: Via Vercel Dashboard
1. Gå till [vercel.com](https://vercel.com)
2. Klicka "Add New" → "Project"
3. Importera ditt Git-repository
4. Vercel kommer automatiskt att identifiera att det är en Python-app
5. Klicka "Deploy"

### Alternativ 2: Via Vercel CLI
```bash
# Installera Vercel CLI
npm i -g vercel

# Logga in
vercel login

# Deploy
vercel

# För produktion
vercel --prod
```

## Steg 6: Miljövariabler

Om din app behöver miljövariabler (t.ex. API-nycklar, databas-URL):

1. I Vercel Dashboard, gå till ditt projekt → Settings → Environment Variables
2. Lägg till variabler som:
   - `DATABASE_URL`
   - `SECRET_KEY`
   - `API_KEY`

Åtkomst i Flask:
```python
import os
database_url = os.environ.get('DATABASE_URL')
```

## Steg 7: Statiska filer

För att serva statiska filer (CSS, JS, bilder):

```python
from flask import send_from_directory

@app.route('/static/<path:path>')
def serve_static(path):
    return send_from_directory('static', path)
```

Placera filer i en `static/`-mapp i root.

## Steg 8: Databasanslutning

Vercel stöder serverless databaser som:
- **Vercel Postgres** (integrerat)
- **Neon.tech** (serverless Postgres)
- **MongoDB Atlas**
- **Supabase**

Exempel med Vercel Postgres:
```python
import psycopg2
import os

def get_db():
    conn = psycopg2.connect(os.environ['POSTGRES_URL'])
    return conn
```

## Steg 9: Debugging och loggar

### Loggar i Vercel Dashboard:
1. Gå till ditt projekt → "Functions"
2. Välj din funktion (`api/index.py`)
3. Se exekveringsloggar och fel

### Lokal testning:
```bash
# Installera beroenden
pip install -r requirements.txt

# Kör lokalt med Vercel dev
vercel dev
```

## Vanliga fel och lösningar

### Fel: "ModuleNotFoundError"
- Kontrollera att alla beroenden finns i `requirements.txt`
- Vercel installerar automatiskt från denna fil

### Fel: "Function timeout"
- Öka `maxDuration` i `vercel.json` (max 300 sekunder på Pro-plan)
- Optimera din kod för serverless

### Fel: "Request entity too large"
- Vercel har en gräns på 4.5MB för request body
- Överväg att använda uppladdning till S3 eller liknande

## Prestandatips

1. **Använd connection pooling** för databaser
2. **Cacha resultat** med Vercel Edge Config eller Redis
3. **Minifiera statiska filer** innan deploy
4. **Använd CDN** för statiska assets
5. **Aktivera Edge Functions** för global låg latens

## Exempel: Komplett Flask-app med flera routes

```python
# api/index.py
from flask import Flask, request, jsonify, render_template
import os

app = Flask(__name__)

@app.route('/')
def home():
    return jsonify({
        "service": "Flask på Vercel",
        "endpoints": [
            "/api/users",
            "/api/data",
            "/health"
        ]
    })

@app.route('/api/users', methods=['GET'])
def get_users():
    users = [{"id": 1, "name": "Alice"}, {"id": 2, "name": "Bob"}]
    return jsonify(users)

@app.route('/api/data', methods=['POST'])
def create_data():
    data = request.json
    # Process data här
    return jsonify({"status": "created", "data": data}), 201

@app.route('/health')
def health_check():
    return jsonify({"status": "ok"})

# Exportera app för Vercel
application = app

if __name__ == '__main__':
    app.run(debug=True)
```

## Automatisk deploy med Git

Vercel stöder automatisk deploy när du pushar till:
- `main` branch → produktion
- Andra branches → preview deployment

## Kostnad

- **Gratis tier**: 100GB-bandwidth/månad, 100GB-hours serverless functions
- **Pro-plan**: $20/månad för mer bandbredd och längre timeout

## Ytterligare resurser

- [Vercel Python Docs](https://vercel.com/docs/concepts/functions/serverless-functions/runtimes/python)
- [Flask Documentation](https://flask.palletsprojects.com/)
- [Serverless Python Patterns](https://vercel.com/guides/python-serverless-functions)

---

**Tips:** Testa alltid i preview deployment innan du deployar till produktion. Vercel skapar automatiskt en unik URL för varje branch som du kan testa mot.