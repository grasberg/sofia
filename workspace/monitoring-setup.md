# Monitoring Setup för Produktion (Hybrid Approach)

## Rekommenderad Stack
1. **UptimeRobot** – gratis uptime monitoring för hälsocheckar
2. **Sentry** – gratis error tracking för backend (PHP/Laravel) och frontend (Next.js/React)
3. **Logtail** – gratis loggaggregering (500 GB/månad)

## Steg 1: UptimeRobot Konfiguration

### Skapa hälsocheck-endpoints i dina applikationer

#### Laravel (affiliate-system)
Lägg till följande route i `routes/api.php` eller skapa en ny fil `routes/health.php`:

```php
<?php

use Illuminate\Support\Facades\Route;
use Illuminate\Support\Facades\DB;

Route::get('/health', function () {
    // Basic health check
    $status = [
        'status' => 'ok',
        'timestamp' => now(),
        'service' => 'affiliate-system',
        'database' => 'connected',
    ];

    try {
        DB::connection()->getPdo();
        $status['database'] = 'connected';
    } catch (\Exception $e) {
        $status['database'] = 'disconnected';
        $status['status'] = 'degraded';
    }

    return response()->json($status);
});
```

#### Next.js (sofia-ops-app)
Lägg till en API-route i `pages/api/health.js` eller `app/api/health/route.js`:

```javascript
// pages/api/health.js (Next.js Pages Router)
export default function handler(req, res) {
  const status = {
    status: 'ok',
    timestamp: new Date().toISOString(),
    service: 'sofia-ops-app',
    uptime: process.uptime(),
  };
  res.status(200).json(status);
}
```

### Registrera monitorer i UptimeRobot
1. Gå till [UptimeRobot.com](https://uptimerobot.com/) och skapa konto
2. Klicka på "+ Add New Monitor"
3. Monitor Type: **HTTP(s)**
4. URL: `https://din-domän.se/health` (för varje tjänst)
5. Monitoring Interval: 5 minuter (gratis)
6. Alert Contacts: Lägg till din email

## Steg 2: Sentry Error Tracking

### Laravel Installation
```bash
composer require sentry/sentry-laravel
```

Konfigurera i `.env`:
```env
SENTRY_LARAVEL_DSN=https://xxx@sentry.io/xxx
```

Lägg till i `config/logging.php`:
```php
'channels' => [
    'sentry' => [
        'driver' => 'sentry',
    ],
],
```

### Next.js Installation
```bash
npm install @sentry/nextjs
```

Konfigurera i `next.config.js`:
```javascript
const { withSentryConfig } = require('@sentry/nextjs');

module.exports = withSentryConfig(
  module.exports,
  {
    silent: true,
    org: "your-org",
    project: "your-project",
  },
  {
    widenClientFileUpload: true,
    transpileClientSDK: true,
    tunnelRoute: "/monitoring",
    hideSourceMaps: true,
    disableLogger: true,
  }
);
```

## Steg 3: Logtail Loggaggregering

### Laravel Integration
```bash
composer require logtail/monolog-logtail
```

Konfigurera i `config/logging.php`:
```php
'channels' => [
    'logtail' => [
        'driver' => 'monolog',
        'handler' => \Logtail\Monolog\LogtailHandler::class,
        'handler_with' => [
            'sourceToken' => env('LOGTAIL_SOURCE_TOKEN'),
        ],
        'formatter' => \Logtail\Monolog\LogtailFormatter::class,
    ],
],
```

### Next.js Integration
Installera Logtail client:
```bash
npm install @logtail/node @logtail/next
```

Skapa en loggutil i `lib/logger.js`:
```javascript
import { Logtail } from "@logtail/node";

const logtail = new Logtail(process.env.LOGTAIL_SOURCE_TOKEN);

export default logtail;
```

## Steg 4: Ytterligare Metrikinsamling (valfritt)

### Server Metrics med Node Exporter
För Linux-servrar:
```bash
wget https://github.com/prometheus/node_exporter/releases/download/v1.6.0/node_exporter-1.6.0.linux-amd64.tar.gz
tar xvfz node_exporter-*linux-amd64.tar.gz
cd node_exporter-*linux-amd64
./node_exporter &
```

### Laravel Prometheus Metrics (om du vill ha detaljerade applikationsmetriker)
```bash
composer require spatie/laravel-prometheus
```

Publicera konfiguration:
```bash
php artisan vendor:publish --provider="Spatie\Prometheus\PrometheusServiceProvider"
```

Metrics kommer vara tillgängliga på `/prometheus` endpoint.

## Steg 5: Dashboard Visualisering

### Grafana Dashboard (om du kör Prometheus)
1. Installera Grafana på din server
2. Lägg till Prometheus som datakälla
3. Importera dashboard-mallar:
   - Node Exporter Full (ID: 1860)
   - Laravel Metrics (skapa egen)

### Sentry Dashboard
Sentry har inbyggda dashboards för errors, performance och releases.

## Steg 6: Alerting

### UptimeRobot
- Konfigurera email/Slack notifieringar vid downtime

### Sentry
- Skapa alerts för nya errors, error frequency ökningar

### Logtail
- Ställ in queries för att detektera felmönster och skicka alerts

## Nästa Steg
1. Skapa hälsocheck-endpoints i dina applikationer
2. Registrera UptimeRobot monitorer
3. Installera Sentry i både Laravel och Next.js
4. Konfigurera Logtail för loggaggregering
5. Testa hela kedjan från error → log → alert