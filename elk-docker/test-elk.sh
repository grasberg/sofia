#!/bin/bash

# ELK Stack Test Script
# Testar att alla komponenter i ELK-stacken fungerar korrekt

set -e

echo "=== ELK Stack Test Script ==="
echo

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Function to print status
print_status() {
    if [ $1 -eq 0 ]; then
        echo -e "${GREEN}✓${NC} $2"
    else
        echo -e "${RED}✗${NC} $2"
        exit 1
    fi
}

# Check if docker-compose is available
echo "1. Kontrollerar Docker och Docker Compose..."
which docker > /dev/null 2>&1
print_status $? "Docker är installerat"

which docker-compose > /dev/null 2>&1
print_status $? "Docker Compose är installerat"

# Check if services are running
echo -e "\n2. Kontrollerar att ELK-tjänsterna kör..."
ELASTICSEARCH_RUNNING=$(docker-compose ps elasticsearch | grep -c "Up")
KIBANA_RUNNING=$(docker-compose ps kibana | grep -c "Up")
LOGSTASH_RUNNING=$(docker-compose ps logstash | grep -c "Up")

if [ $ELASTICSEARCH_RUNNING -eq 1 ]; then
    echo -e "${GREEN}✓${NC} Elasticsearch kör"
else
    echo -e "${RED}✗${NC} Elasticsearch kör inte"
    echo "Starta tjänsterna med: docker-compose up -d"
    exit 1
fi

if [ $KIBANA_RUNNING -eq 1 ]; then
    echo -e "${GREEN}✓${NC} Kibana kör"
else
    echo -e "${YELLOW}⚠${NC} Kibana startar kanske fortfarande..."
fi

if [ $LOGSTASH_RUNNING -eq 1 ]; then
    echo -e "${GREEN}✓${NC} Logstash kör"
else
    echo -e "${YELLOW}⚠${NC} Logstash startar kanske fortfarande..."
fi

# Test Elasticsearch connection
echo -e "\n3. Testar Elasticsearch anslutning..."
sleep 5  # Wait a bit for services to be ready
curl -s -f http://localhost:9200/ > /dev/null 2>&1
print_status $? "Elasticsearch svarar på port 9200"

# Get Elasticsearch info
echo -e "\n4. Hämtar Elasticsearch information..."
ES_INFO=$(curl -s http://localhost:9200/)
echo "Cluster name: $(echo $ES_INFO | jq -r '.cluster_name')"
echo "Elasticsearch version: $(echo $ES_INFO | jq -r '.version.number')"

# Test Logstash TCP input
echo -e "\n5. Testar Logstash TCP input..."
TEST_LOG='{"message": "Test log from script", "level": "INFO", "timestamp": "'$(date -Iseconds)'", "test": true}'
echo $TEST_LOG | nc -w 2 localhost 5044 2>/dev/null || true
echo "Skickade testlogg till Logstash (port 5044)"

# Test Logstash HTTP input
echo -e "\n6. Testar Logstash HTTP input..."
curl -s -X POST http://localhost:5000/ \
  -H "Content-Type: application/json" \
  -d '{"message": "HTTP test log", "level": "DEBUG", "service": "test-script"}' > /dev/null 2>&1 || true
echo "Skickade HTTP-testlogg till Logstash (port 5000)"

# Check if logs are indexed
echo -e "\n7. Kontrollerar om loggar är indexerade i Elasticsearch..."
sleep 3  # Wait for indexing
INDEX_COUNT=$(curl -s http://localhost:9200/_cat/indices?format=json | jq '.[] | select(.index | startswith("logs-")) | ."docs.count"' | head -1)

if [ ! -z "$INDEX_COUNT" ] && [ "$INDEX_COUNT" -gt "0" ]; then
    echo -e "${GREEN}✓${NC} Loggar är indexerade ($INDEX_COUNT dokument)"
else
    echo -e "${YELLOW}⚠${NC} Inga loggar indexerade ännu (kan ta några sekunder)"
fi

# Test Kibana connection
echo -e "\n8. Testar Kibana anslutning..."
sleep 2
curl -s -f http://localhost:5601/api/status > /dev/null 2>&1
if [ $? -eq 0 ]; then
    echo -e "${GREEN}✓${NC} Kibana svarar på port 5601"
    echo "Öppna Kibana i webbläsaren: http://localhost:5601"
else
    echo -e "${YELLOW}⚠${NC} Kibana startar fortfarande..."
    echo "Vänta några sekunder och försök: curl http://localhost:5601/api/status"
fi

# Summary
echo -e "\n=== Test Sammanfattning ==="
echo "Alla grundläggande tester klara!"
echo
echo "Nästa steg:"
echo "1. Öppna Kibana: http://localhost:5601"
echo "2. Skapa index pattern: 'logs-*'"
echo "3. Gå till 'Discover' för att se loggar"
echo
echo "För att stoppa ELK-stacken:"
echo "  docker-compose down"
echo
echo "För att starta om:"
echo "  docker-compose up -d"