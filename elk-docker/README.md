# ELK Stack Docker Configuration

En komplett Docker-konfiguration för ELK-stacken (Elasticsearch, Logstash, Kibana) för logganalys och datavisualisering.

## Snabbstart

```bash
# Starta ELK-stacken
docker-compose up -d

# Visa loggar
docker-compose logs -f

# Stoppa ELK-stacken
docker-compose down

# Stoppa och ta bort volymer
docker-compose down -v
```

## Tjänster och portar

- **Elasticsearch**: `http://localhost:9200`
  - REST API: 9200
  - Klusterkommunikation: 9300
- **Kibana**: `http://localhost:5601`
  - Webbgränssnitt: 5601
- **Logstash**:
  - TCP input: 5044 (JSON-lines)
  - HTTP input: 5000 (JSON)
  - Monitoring: 9600

## Testa installationen

### 1. Kontrollera Elasticsearch

```bash
curl http://localhost:9200/
```

Förväntat svar:
```json
{
  "name": "elasticsearch",
  "cluster_name": "docker-cluster",
  "cluster_uuid": "...",
  "version": {...},
  "tagline": "You Know, for Search"
}
```

### 2. Skicka testloggar till Logstash

```bash
# Skicka en logg via TCP
echo '{"message": "Test log from curl", "level": "INFO", "timestamp": "'$(date -Iseconds)'"}' | nc localhost 5044

# Skicka en logg via HTTP
curl -X POST http://localhost:5000/ \
  -H "Content-Type: application/json" \
  -d '{"message": "Test log via HTTP", "level": "DEBUG", "service": "test"}'
```

### 3. Öppna Kibana

1. Öppna `http://localhost:5601` i webbläsaren
2. Gå till "Stack Management" → "Index Patterns"
3. Skapa index pattern: `logs-*`
4. Gå till "Discover" för att se dina loggar

## Konfiguration

### Elasticsearch
- Konfiguration: `elasticsearch/config/elasticsearch.yml`
- Data volym: `es_data` (sparas lokalt)
- Minne: 2GB heap (Xms2g -Xmx2g)
- Säkerhet: **Inaktiverad** för enklare testning

### Logstash
- Konfiguration: `logstash/config/logstash.yml`
- Pipeline: `logstash/pipeline/logstash.conf`
- Inputs: TCP (5044), HTTP (5000), generator (testdata)
- Output: Elasticsearch + stdout (debug)

### Kibana
- Konfiguration: `kibana/config/kibana.yml`
- Kopplad till Elasticsearch: `http://elasticsearch:9200`

## Produktionsanvändning

För produktionsanvändning, rekommenderas följande ändringar:

1. **Aktivera säkerhet**:
   ```yaml
   # i elasticsearch/config/elasticsearch.yml
   xpack.security.enabled: true
   
   # i kibana/config/kibana.yml
   xpack.security.enabled: true
   ```

2. **Skapa användare och lösenord**:
   ```bash
   docker exec -it elasticsearch /usr/share/elasticsearch/bin/elasticsearch-setup-passwords auto
   ```

3. **Använda miljövariabler** för lösenord:
   ```yaml
   # i docker-compose.yml
   environment:
     - ELASTICSEARCH_PASSWORD=${ES_PASSWORD}
     - KIBANA_PASSWORD=${KIBANA_PASSWORD}
   ```

4. **Öka resurser**:
   ```yaml
   # i docker-compose.yml
   deploy:
     resources:
       limits:
         memory: 4g
         cpus: '2'
   ```

## Felsökning

### Elasticsearch startar inte
```bash
# Kontrollera loggar
docker-compose logs elasticsearch

# Kontrollera minnesgränser (på macOS/Linux)
ulimit -n  # ska vara minst 65535
```

### Kibana kan inte ansluta till Elasticsearch
```bash
# Kontrollera nätverk
docker network inspect elk-docker_elk

# Testa anslutning från Kibana container
docker exec -it kibana curl http://elasticsearch:9200
```

### Logstash pipeline fel
```bash
# Kontrollera Logstash konfiguration
docker exec -it logstash /usr/share/logstash/bin/logstash --config.test_and_exit -f /usr/share/logstash/pipeline/
```

## Ytterligare verktyg

### Filebeat (för att skicka loggfiler)
Se [Filebeat Docker dokumentation](https://www.elastic.co/guide/en/beats/filebeat/current/running-on-docker.html)

### Metricbeat (för systemmått)
Se [Metricbeat Docker dokumentation](https://www.elastic.co/guide/en/beats/metricbeat/current/running-on-docker.html)

## Resurser

- [Elasticsearch dokumentation](https://www.elastic.co/guide/en/elasticsearch/reference/current/docker.html)
- [Logstash dokumentation](https://www.elastic.co/guide/en/logstash/current/docker-config.html)
- [Kibana dokumentation](https://www.elastic.co/guide/en/kibana/current/docker.html)
- [ELK Stack tutorials](https://www.elastic.co/guide/en/elastic-stack-get-started/current/get-started-elastic-stack.html)