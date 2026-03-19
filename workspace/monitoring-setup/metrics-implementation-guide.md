# Metrics Implementation Guide for Production Applications

## 1. Laravel Affiliate System

### Recommended Package
Use `spatie/laravel-prometheus` (maintained by Spatie, good documentation):

```bash
composer require spatie/laravel-prometheus
```

### Configuration
1. Publish configuration:
   ```bash
   php artisan vendor:publish --provider="Spatie\Prometheus\PrometheusServiceProvider"
   ```

2. Configure metrics in `config/prometheus.php`:
   ```php
   return [
       'enabled' => env('PROMETHEUS_ENABLED', true),
       
       'metrics' => [
           Spatie\Prometheus\Metrics\HorizonMetrics::class,
           Spatie\Prometheus\Metrics\QueueMetrics::class,
           // Add your custom metrics
       ],
       
       'route' => [
           'enabled' => true,
           'path' => '/metrics',
           'middleware' => null, // or add authentication if needed
       ],
   ];
   ```

3. Add custom metrics for business KPIs:
   ```php
   // app/Metrics/AffiliateMetrics.php
   namespace App\Metrics;
   
   use Spatie\Prometheus\Collectors\Gauge;
   
   class AffiliateMetrics
   {
       public function register(): array
       {
           return [
               Gauge::make('affiliate_clicks_total', 'Total affiliate clicks')
                   ->value(fn() => DB::table('clicks')->count()),
                   
               Gauge::make('affiliate_conversions_total', 'Total conversions')
                   ->value(fn() => DB::table('conversions')->count()),
                   
               Gauge::make('affiliate_revenue_total', 'Total revenue in SEK')
                   ->value(fn() => DB::table('commissions')->sum('amount')),
           ];
       }
   }
   ```

4. Register in `config/prometheus.php`:
   ```php
   'metrics' => [
       // ...
       App\Metrics\AffiliateMetrics::class,
   ],
   ```

### Health Endpoint
Create a health check endpoint at `/health`:
```php
// routes/api.php
Route::get('/health', function () {
    try {
        DB::connection()->getPdo();
        return response()->json(['status' => 'healthy']);
    } catch (\Exception $e) {
        return response()->json(['status' => 'unhealthy', 'error' => $e->getMessage()], 500);
    }
});
```

## 2. Next.js Sofia Ops App

### Recommended Package
Use `prom-client` for custom metrics:

```bash
npm install prom-client
# or
yarn add prom-client
```

### Metrics Endpoint
Create `app/api/metrics/route.ts`:
```typescript
import { NextRequest, NextResponse } from 'next/server';
import { register, collectDefaultMetrics } from 'prom-client';

// Collect default metrics (CPU, memory, etc.)
collectDefaultMetrics();

// Custom business metrics
import { Gauge, Counter } from 'prom-client';

const activeAgents = new Gauge({
  name: 'sofia_active_agents',
  help: 'Number of active agents',
});

const completedTasks = new Counter({
  name: 'sofia_tasks_completed_total',
  help: 'Total number of completed tasks',
});

export const runtime = 'nodejs';
export const revalidate = 0; // Disable caching for metrics

export async function GET(request: NextRequest) {
  try {
    // Update metrics with real data
    // const agentCount = await getActiveAgentCount();
    // activeAgents.set(agentCount);
    
    const metrics = await register.metrics();
    
    return new NextResponse(metrics, {
      status: 200,
      headers: {
        'Content-Type': register.contentType,
      },
    });
  } catch (error) {
    console.error('Error generating metrics:', error);
    return new NextResponse('Error generating metrics', { status: 500 });
  }
}
```

### Health Endpoint
Create `app/api/health/route.ts`:
```typescript
import { NextRequest, NextResponse } from 'next/server';

export const runtime = 'nodejs';

export async function GET(request: NextRequest) {
  // Add any health checks (database, external APIs, etc.)
  const isHealthy = true;
  
  if (isHealthy) {
    return NextResponse.json({ status: 'healthy', timestamp: new Date().toISOString() });
  } else {
    return NextResponse.json(
      { status: 'unhealthy', timestamp: new Date().toISOString() },
      { status: 503 }
    );
  }
}
```

## 3. Prometheus Configuration Updates

Update `prometheus/prometheus.yml` with actual production targets:

```yaml
scrape_configs:
  # Laravel app (adjust host:port)
  - job_name: 'laravel-app'
    static_configs:
      - targets: ['your-laravel-app.com:80']  # or IP:port
    metrics_path: '/metrics'
    scrape_interval: 30s
    # Add basic auth if needed
    # basic_auth:
    #   username: 'prometheus'
    #   password: '${PASSWORD}'

  # Next.js app (if self-hosted)
  - job_name: 'nextjs-app'
    static_configs:
      - targets: ['your-nextjs-app.com:3000']
    metrics_path: '/api/metrics'
    scrape_interval: 30s

  # Blackbox exporter for HTTP health checks
  - job_name: 'blackbox-http'
    metrics_path: /probe
    params:
      module: [http_2xx]
    static_configs:
      - targets:
        - https://your-laravel-app.com/health
        - https://your-nextjs-app.com/api/health
    relabel_configs:
      - source_labels: [__address__]
        target_label: __param_target
      - source_labels: [__param_target]
        target_label: instance
      - target_label: __address__
        replacement: blackbox-exporter:9115
```

## 4. Log Collection Configuration

Update `promtail/promtail-config.yml` (if not exists, create it):

```yaml
server:
  http_listen_port: 9080
  grpc_listen_port: 0

positions:
  filename: /tmp/positions.yaml

clients:
  - url: http://loki:3100/loki/api/v1/push

scrape_configs:
  - job_name: system
    static_configs:
      - targets:
          - localhost
        labels:
          job: system
          __path__: /var/log/*log
    
  - job_name: laravel
    static_configs:
      - targets:
          - localhost
        labels:
          job: laravel
          __path__: /path/to/laravel/storage/logs/*.log
          
  - job_name: nginx
    static_configs:
      - targets:
          - localhost
        labels:
          job: nginx
          __path__: /var/log/nginx/*.log
```

## 5. Key Metrics to Track

### System Metrics (via Node Exporter)
- CPU usage
- Memory usage
- Disk usage
- Network I/O
- Uptime

### Application Metrics
**Laravel:**
- HTTP request rate/latency
- Database query performance
- Queue lengths (if using queues)
- Cache hit/miss ratios
- Business: clicks, conversions, revenue

**Next.js:**
- HTTP request rate/latency
- Active users/sessions
- Task completion rates
- Agent status counts

### Business KPIs
- Monthly recurring revenue (MRR)
- Conversion rates (click → conversion)
- Average commission value
- Customer acquisition cost (if applicable)

## 6. Next Steps

1. **Implement metrics endpoints** in both applications
2. **Update Prometheus config** with real production targets
3. **Deploy updated applications** to production
4. **Verify metrics are being scraped** in Prometheus UI (http://localhost:9090)
5. **Create Grafana dashboards** using the new metrics
6. **Set up alert rules** for critical thresholds

## References
- [spatie/laravel-prometheus](https://github.com/spatie/laravel-prometheus)
- [prom-client npm](https://www.npmjs.com/package/prom-client)
- [Prometheus Documentation](https://prometheus.io/docs/introduction/overview/)
- [Grafana Documentation](https://grafana.com/docs/)