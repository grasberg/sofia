#!/bin/sh
# Start ELK Stack Script

echo "Starting ELK Stack (Elasticsearch, Logstash, Kibana)..."
echo "======================================================"

# Set versions from .env if it exists
if [ -f .env.example ]; then
    echo "Loading versions from .env.example"
    export $(grep -v '^#' .env.example | xargs)
fi

# Default versions if not set
ELASTICSEARCH_VERSION=${ELASTICSEARCH_VERSION:-8.13.0}
LOGSTASH_VERSION=${LOGSTASH_VERSION:-8.13.0}
KIBANA_VERSION=${KIBANA_VERSION:-8.13.0}

echo "Using versions:"
echo "  Elasticsearch: $ELASTICSEARCH_VERSION"
echo "  Logstash: $LOGSTASH_VERSION"
echo "  Kibana: $KIBANA_VERSION"
echo

# Check Docker
if ! command -v docker &> /dev/null; then
    echo "ERROR: Docker is not installed or not in PATH"
    exit 1
fi

if ! command -v docker-compose &> /dev/null; then
    echo "ERROR: Docker Compose is not installed or not in PATH"
    exit 1
fi

echo "Starting services..."
docker-compose up -d

echo
echo "Waiting for services to start (10 seconds)..."
sleep 10

echo
echo "Checking service status:"
echo "========================"

# Check Elasticsearch
if curl -s -f http://localhost:9200/ > /dev/null; then
    echo "✓ Elasticsearch is running on http://localhost:9200"
else
    echo "✗ Elasticsearch is not responding"
    echo "Check logs with: docker-compose logs elasticsearch"
fi

# Check Kibana  
if curl -s -f http://localhost:5601/api/status > /dev/null; then
    echo "✓ Kibana is running on http://localhost:5601"
else
    echo "⚠ Kibana might still be starting..."
    echo "Check logs with: docker-compose logs kibana"
fi

# Check Logstash
if docker-compose ps logstash | grep -q "Up"; then
    echo "✓ Logstash is running"
    echo "  - TCP input: localhost:5044"
    echo "  - HTTP input: localhost:5000"
else
    echo "⚠ Logstash might still be starting..."
    echo "Check logs with: docker-compose logs logstash"
fi

echo
echo "======================================================"
echo "ELK Stack started successfully!"
echo
echo "Access points:"
echo "  Kibana UI:        http://localhost:5601"
echo "  Elasticsearch API: http://localhost:9200"
echo
echo "Useful commands:"
echo "  View logs:        docker-compose logs -f"
echo "  Stop services:    docker-compose down"
echo "  Stop and cleanup: docker-compose down -v"
echo
echo "Test the setup:"
echo "  Send a test log:  echo '{\"message\":\"test\"}' | nc localhost 5044"
echo "  Check indices:    curl http://localhost:9200/_cat/indices"
echo "======================================================"