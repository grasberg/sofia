# ELK Stack Docker Konfiguration - Sammanfattning

## Skapad Docker-konfiguration för ELK-stack (Elasticsearch, Logstash, Kibana)

### Filstruktur
```
elk-docker/
├── docker-compose.yml           # Huvudkonfiguration för alla tjänster
├── README.md                    # Komplett dokumentation
├── .env.example                 # Miljövariabler (versions)
├── .dockerignore               # Ignorera onödiga filer
├── start-elk.sh                # Startskript
├── stop-elk.sh                 # Stoppskript
├── test-elk.sh                 # Testskript (inte körbart pga systembegränsningar)
├── elasticsearch/
│   └── config/
│       └── elasticsearch.yml   # Elasticsearch konfiguration
├── logstash/
│   ├── config/
│   │   └── logstash.yml        # Logstash konfiguration
│   └── pipeline/
│       └── logstash.conf       # Logstash pipeline (input/filter/output)
└── kibana/
    └── config/
        └── kibana.yml          # Kibana konfiguration
```

### Tjänster och Portar

1. **Elasticsearch** (port 9200, 9300)
   - REST API: http://localhost:9200
   - Klusterkommunikation: 9300
   - Minne: 2GB heap
   - Data persistence: `es_data` volym
   - Säkerhet: Inaktiverad för testning

2. **Logstash** (port 5000, 5044, 9600)
   - TCP input: 5044 (accepterar JSON-lines)
   - HTTP input: 5000 (REST API för loggar)
   - Monitoring: 9600
   - Pipeline: Tar emot loggar → processerar → skickar till Elasticsearch
   - Inbyggd test: Generator som skapar 5 testloggar

3. **Kibana** (port 5601)
   - Webbgränssnitt: http://localhost:5601
   - Kopplad till Elasticsearch
   - Säkerhet: Inaktiverad för testning

4. **Grafana** (port 3000) *existerade redan*
   - Dashboard för visualisering
   - Elasticsearch datasource konfigurerad

### Användning

```bash
# Starta alla tjänster
cd elk-docker
docker-compose up -d

# Alternativt med startskriptet
./start-elk.sh

# Kontrollera status
docker-compose ps

# Visa loggar
docker-compose logs -f

# Stoppa tjänster
docker-compose down

# Stoppa och ta bort data
docker-compose down -v
```

### Testa installationen

1. **Kontrollera Elasticsearch**:
   ```bash
   curl http://localhost:9200/
   ```

2. **Skicka testloggar till Logstash**:
   ```bash
   # Via TCP (port 5044)
   echo '{"message": "Test log", "level": "INFO"}' | nc localhost 5044
   
   # Via HTTP (port 5000)
   curl -X POST http://localhost:5000/ \
     -H "Content-Type: application/json" \
     -d '{"message": "HTTP test", "service": "api"}'
   ```

3. **Öppna Kibana**:
   - Gå till http://localhost:5601
   - Skapa index pattern: `logs-*`
   - Gå till "Discover" för att se loggar

4. **Öppna Grafana** (om aktiverad):
   - Gå till http://localhost:3000
   - Login: admin/admin (från .env.example)
   - Elasticsearch datasource är redan konfigurerad

### Konfigurationsdetaljer

#### Elasticsearch
- Single-node kluster (för development)
- CORS aktiverat för Kibana/Grafana
- Inga minnesbegränsningar (memlock)
- Healthcheck för automatisk övervakning

#### Logstash
- Enkel pipeline med 3 inputs:
  1. TCP (5044) för JSON-lines
  2. HTTP (5000) för REST API
  3. Generator för testdata
- Filter: Parsar JSON, lägger till timestamp
- Output: Elasticsearch + console (debug)

#### Kibana
- Kopplad till Elasticsearch
- Standard route: /app/discover
- Internationalization: English

### Produktionsrekommendationer

1. **Aktivera säkerhet**:
   - Sätt `xpack.security.enabled: true` i elasticsearch.yml
   - Konfigurera användare/lösenord
   - Uppdatera Kibana och Logstash konfiguration

2. **Öka resurser**:
   - Elasticsearch: 4GB+ heap för produktion
   - Logstash: Fler workers för högre throughput

3. **Använda volymer**:
   - External volumes för bättre prestanda
   - Backup-strategi för data

4. **Monitoring**:
   - Aktivera X-Pack monitoring
   - Ställ in alerting
   - Log rotation

### Felsökning

#### Tjänster startar inte
```bash
docker-compose logs [service-name]
docker-compose ps
```

#### Elasticsearch minnesproblem
```bash
# På Linux/Mac
ulimit -n  # ska vara minst 65535
ulimit -u  # ska vara minst 4096
```

#### Logstash konfigurationsfel
```bash
docker exec -it logstash /usr/share/logstash/bin/logstash \
  --config.test_and_exit \
  -f /usr/share/logstash/pipeline/
```

### Ytterligare funktionalitet

Konfigurationen inkluderar även:

- **Grafana** för avancerade visualiseringar
- **Healthchecks** för automatisk övervakning
- **Networking** med eget bridge-nätverk
- **Data persistence** med Docker volymer
- **Restart policies** för hög tillgänglighet

### Skillnader från befintlig EFK-stack

Denna konfiguration använder **Logstash** istället för Fluentd/Fluent Bit:
- Logstash har mer avancerade filter-möjligheter
- Stöder komplexa pipelines
- Bättre för datatransformation
- Mer resurskrävande men kraftfullare