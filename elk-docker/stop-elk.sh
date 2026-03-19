#!/bin/sh
# Stop ELK Stack Script

echo "Stopping ELK Stack..."
echo "====================="

# Check if services are running
if docker-compose ps | grep -q "Up"; then
    echo "Stopping running services..."
    docker-compose down
    
    echo
    echo "Services stopped."
else
    echo "No ELK services are currently running."
fi

echo
echo "To remove data volumes (warning: deletes all data):"
echo "  docker-compose down -v"
echo
echo "To start again:"
echo "  ./start-elk.sh"
echo "====================="