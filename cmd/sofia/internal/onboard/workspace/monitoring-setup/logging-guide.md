# Loggning och Log Aggregation Setup

Den här guiden beskriver hur du konfigurerar loggning för dina applikationer och aggregerar loggar med Loki.

## 1. Systemloggar

Promtail konfigureras att samla in systemloggar från `/var/log/*log`. Detta inkluderar vanliga systemloggar som syslog, auth.log, etc.

## 2. Laravel (Affiliate System) Loggning

Laravel använder Monolog som logging library. Standardkonfigurationen loggar till `storage/logs/laravel.log`. För att förbättra loggning för monitoring:

### a) Logga till separat fil för varje kanal

Uppdatera `config/logging.php`:

```php
'channels' => [
    'stack' => [
        'driver' => 'stack',
        'channels' => ['daily', 'loki'],
        'ignore_exceptions' => false,
    ],

    'daily' => [
        'driver' => 'daily',
        'path' => storage_path('logs/laravel.log'),
        'level' => env('LOG_LEVEL', 'debug'),
        'days' => 14,
    ],

    'loki' => [
        'driver' => 'monolog',
        'handler' => \Grafana\Loki\LokiHandler::class,
        'handler_with' => [
            'url' => env('LOKI_URL', 'http://localhost:3100/loki/api/v1/push'),
            'labels' => [
                'job' => 'laravel',
                'app' => 'affiliate-system',
                'environment' => env('APP_ENV', 'production'),
            ],
        ],
        'level' => 'info',
    ],

    // ... befintliga kanaler
],
```

### b) Installera Grafana Loki PHP client

```bash
composer require grafanainc/laravel-loki
```

### c) Konfigurera Promtail att taila Laravel loggfiler

I Promtail config (`promtail-config.yml`), lägg till:

```yaml
  - job_name: laravel
    static_configs:
      - targets:
          - localhost
        labels:
          job: laravel
          app: affiliate-system
          environment: production
          __path__: /path/to/laravel/storage/logs/*.log
```

## 3. Next.js (Sofia Ops App) Loggning

Next.js applikationer körs vanligtvis med Node.js och loggar till stdout/stderr. För att fånga dessa loggar:

### a) Använd PM2 eller systemd för att dirigera loggar till fil

Exempel PM2 konfiguration (`ecosystem.config.js`):

```javascript
module.exports = {
  apps: [{
    name: 'sofia-ops-app',
    script: 'npm',
    args: 'start',
    instances: 1,
    autorestart: true,
    watch: false,
    max_memory_restart: '1G',
    error_file: '/var/log/nextjs/error.log',
    out_file: '/var/log/nextjs/out.log',
    log_file: '/var/log/nextjs/combined.log',
    time: true,
    env: {
      NODE_ENV: 'production',
    },
  }]
};
```

### b) Konfigurera Promtail att taila Next.js loggfiler

```yaml
  - job_name: nextjs
    static_configs:
      - targets:
          - localhost
        labels:
          job: nextjs
          app: sofia-ops-app
          environment: production
          __path__: /var/log/nextjs/*.log
```

### c) Använd Winston eller Pino för strukturerad loggning

Installera Winston och skapa en custom logger som skickar till Loki:

```javascript
const winston = require('winston');
const { LokiTransport } = require('winston-loki');

const logger = winston.createLogger({
  transports: [
    new winston.transports.Console(),
    new LokiTransport({
      host: 'http://localhost:3100',
      labels: { app: 'sofia-ops-app', job: 'nextjs' },
      json: true,
      format: winston.format.json(),
      replaceTimestamp: true,
      onConnectionError: (err) => console.error(err)
    })
  ]
});
```

## 4. Nginx/Apache Access Logs

För att övervaka webbtrafik kan du samla in access-loggar.

### Nginx

Nginx loggar vanligtvis till `/var/log/nginx/access.log` och `/var/log/nginx/error.log`. Promtail kan taila dessa.

Lägg till i Promtail config:

```yaml
  - job_name: nginx
    static_configs:
      - targets:
          - localhost
        labels:
          job: nginx
          __path__: /var/log/nginx/*.log
```

### Apache

```yaml
  - job_name: apache
    static_configs:
      - targets:
          - localhost
        labels:
          job: apache
          __path__: /var/log/apache2/*.log
```

## 5. MySQL/PostgreSQL Loggning

För databasloggar, aktivera slow query log och error log.

### MySQL

I `my.cnf`:

```ini
[mysqld]
slow_query_log = 1
slow_query_log_file = /var/log/mysql/slow.log
long_query_time = 2
log_error = /var/log/mysql/error.log
```

### PostgreSQL

I `postgresql.conf`:

```ini
log_destination = 'stderr'
logging_collector = on
log_directory = '/var/log/postgresql'
log_filename = 'postgresql-%Y-%m-%d_%H%M%S.log'
log_statement = 'all' # eller 'mod', 'ddl'
```

Lägg sedan till i Promtail config för databasloggar.

## 6. Testa logginsamling

Efter att ha konfigurerat Promtail och restartat tjänsten, testa att loggar flödar till Loki:

```bash
# Kolla Promtail loggar
journalctl -u promtail -f

# Fråga Loki för loggar
curl -G http://localhost:3100/loki/api/v1/query \
  --data-urlencode 'query={job="laravel"}'
```

## 7. Grafana Dashboard för Loggar

Skapa ett dashboard i Grafana med "Loki" datasource. Använd "Logs" panel för att visa loggar i realtid.

Exempel query: `{job="laravel"} |= "error"` för att filtrera på felmeddelanden.

## 8. Ytterligare tips

- Rotera loggfiler regelbundet med `logrotate` för att undvika att diskarna fylls.
- Använd strukturered loggning (JSON) för enklare parsing och filtrering.
- Ställ in lämpliga loggnivåer: debug för utveckling, info/warning för produktion.
- Inkludera korrelations-ID för att spåra requests över flera tjänster.