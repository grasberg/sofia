#!/bin/bash
set -e

# Start the app container in background
docker-compose up -d app

# Install Laravel using composer container
docker-compose run --rm composer create-project laravel/laravel .

# Set proper permissions
docker-compose exec app chown -R www-data:www-data /var/www/html/storage /var/www/html/bootstrap/cache

echo "Laravel installed successfully!"
echo "Run 'docker-compose exec app php artisan serve --host=0.0.0.0' to start the server"