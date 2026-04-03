---
name: system-monitor
description: Monitor systems, services, and infrastructure health. Use when checking server status, detecting outages, investigating performance issues, responding to alerts, monitoring disk or memory usage, verifying web service availability, or setting up periodic health checks.
---

# System Monitor

Instructions for checking system health, detecting issues, and responding to incidents. Use the `exec` tool for all system commands.

## Health Check Workflow

Follow this pattern for every health check:

1. **Check**: Run diagnostic commands.
2. **Classify**: Categorize results as Healthy / Warning / Critical.
3. **Act**: Based on classification — log, investigate, or alert.

### Classification Thresholds

| Resource | Healthy | Warning | Critical |
|---|---|---|---|
| Disk usage | < 70% | 70-90% | > 90% |
| Memory usage | < 75% | 75-90% | > 90% |
| CPU load (1min avg) | < 70% | 70-90% | > 90% |
| HTTP response time | < 500ms | 500ms-2s | > 2s or timeout |
| HTTP status code | 2xx | 3xx | 4xx, 5xx |
| Process status | Running | High restart count | Not running |

## System Resource Checks

### Disk Space

```bash
df -h | grep -vE '^(tmpfs|devtmpfs|Filesystem)'
```

Check specific mount points if known. Flag any filesystem above the warning threshold.

### Memory Usage

macOS:
```bash
vm_stat | head -10
top -l 1 -n 0 | grep PhysMem
```

Linux:
```bash
free -h
```

### CPU Load

macOS:
```bash
sysctl -n vm.loadavg
top -l 1 -n 0 | grep "CPU usage"
```

Linux:
```bash
uptime
cat /proc/loadavg
```

### Process Status

Check if specific processes are running:
```bash
ps aux | grep -i [p]rocess-name
```

Check for zombie or defunct processes:
```bash
ps aux | grep -i defunct
```

## Web Service Monitoring

### HTTP Endpoint Checks

```bash
curl -o /dev/null -s -w "HTTP %{http_code} in %{time_total}s\n" https://example.com/health
```

### Full Health Check with Content Validation

```bash
response=$(curl -s -w "\n%{http_code}" https://example.com/api/health)
body=$(echo "$response" | head -n -1)
code=$(echo "$response" | tail -1)
echo "Status: $code"
echo "Body: $body"
```

Validate that:
- Status code is 2xx.
- Response body contains expected content (e.g., `"status": "ok"`).
- Response time is within acceptable range.

### SSL Certificate Expiry

```bash
echo | openssl s_client -servername example.com -connect example.com:443 2>/dev/null | openssl x509 -noout -dates
```

Flag certificates expiring within 14 days.

## Docker Monitoring

```bash
# Container status
docker ps --format "table {{.Names}}\t{{.Status}}\t{{.Ports}}"

# Resource usage
docker stats --no-stream --format "table {{.Name}}\t{{.CPUPerc}}\t{{.MemUsage}}"

# Check for unhealthy containers
docker ps --filter "health=unhealthy"

# Recent logs from a container
docker logs --tail 50 container-name 2>&1
```

## Log Monitoring

Scan log files for error patterns:

```bash
# Count errors in the last 100 lines
tail -100 /path/to/app.log | grep -ciE "(error|fatal|panic|exception)"

# Recent errors with context
tail -500 /path/to/app.log | grep -B2 -A5 -iE "(error|fatal|panic)"
```

### Error Pattern Classification

- **FATAL/PANIC**: Critical — the process likely crashed. Alert immediately.
- **ERROR**: Warning or critical depending on frequency. Investigate if count is rising.
- **WARN**: Informational. Log and track trends.

## Scheduled Monitoring

Use cron to schedule periodic health checks:

- **Every 5 minutes**: Critical service availability (HTTP pings).
- **Every 15 minutes**: Resource usage (disk, memory, CPU).
- **Every hour**: Log error rates and trends.
- **Daily**: SSL certificate expiry, full system report.

When setting up monitoring via cron, configure the job to:
1. Run the check.
2. Compare against thresholds.
3. Only alert if something is Warning or Critical.
4. Always log results for trend analysis.

## Incident Response

When an issue is detected, follow this sequence:

### 1. Detect

Identify what is wrong — service down, high error rate, resource exhaustion.

### 2. Alert

Notify the user immediately for Critical issues. Include:
- What is affected.
- When it started (or when detected).
- Current severity.

### 3. Diagnose

Investigate root cause:
- Check recent changes (`git log`, deployment logs).
- Check resource usage (disk, memory, CPU).
- Check application logs for errors.
- Check dependent services (databases, APIs, DNS).

### 4. Mitigate

Take action to restore service:
- Restart the process if it crashed.
- Clear disk space if full.
- Roll back a deployment if it introduced the issue.
- Scale resources if under load.

**Never take destructive mitigation actions without user approval** (e.g., deleting data, force-killing processes serving users).

### 5. Document

After resolution, record:
- What happened (timeline of events).
- Root cause.
- What was done to fix it.
- What should be done to prevent recurrence.

## Monitoring Report Format

Present monitoring results in a consistent structure:

```
## System Health Report — YYYY-MM-DD HH:MM

Overall Status: HEALTHY | WARNING | CRITICAL

### Resources
- Disk: 45% used (/) — Healthy
- Memory: 6.2GB / 16GB (39%) — Healthy
- CPU load: 1.2 (4 cores) — Healthy

### Services
- Web (https://example.com): 200 OK, 180ms — Healthy
- API (https://api.example.com): 200 OK, 95ms — Healthy
- DB: Running, 12 connections — Healthy

### Alerts
- None

### Log Summary (last hour)
- Errors: 3 (down from 7 yesterday)
- Warnings: 12
- Notable: "connection timeout to redis" x2
```
