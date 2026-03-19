# System Analysis for Monitoring Setup

## Overview of Production Systems

Based on document analysis, the production environment consists of two main applications:

### 1. Affiliate System (Laravel PHP)
- **Location:** `workspace/projects/affiliate-system/`
- **Stack:** Laravel 10, PHP 8.1+, MySQL/MariaDB, Stripe integration
- **Purpose:** Affiliate tracking, click tracking, conversion recording, commission management
- **Current endpoints:**
  - `/api/affiliate/track/{code}` - Click tracking (public)
  - `/api/affiliate/stats` - Statistics (protected)
  - Various dashboard routes under `/dashboard/affiliate/`
- **Logging:** Standard Laravel logs in `storage/logs/` (likely `laravel.log`)
- **Metrics exposure:** No Prometheus metrics endpoint currently implemented
- **Deployment:** Likely traditional VPS/shared hosting (Docker Compose file exists for development only)

### 2. Sofia Ops App (Next.js)
- **Location:** `workspace/sofia-ops-app/`
- **Stack:** Next.js 15 (TypeScript), React, Vercel deployment
- **Purpose:** Operational dashboard for managing agents/tasks
- **Current endpoints:**
  - Standard Next.js routes (app router)
  - No dedicated metrics endpoint
- **Logging:** Vercel logs or application logs (unknown location)
- **Metrics exposure:** No Prometheus metrics endpoint
- **Deployment:** Likely Vercel (based on `vercel.json` configuration)

### 3. Additional Services
- **Web server:** Apache/Nginx (assumed)
- **Database:** MySQL/PostgreSQL (for affiliate system)
- **Cron jobs:** Likely for affiliate processing tasks
- **Potential other services:** Redis, queue workers (not confirmed)

## Existing Monitoring Documentation

### 1. Monitoring Stack Configuration
**Location:** `workspace/monitoring-setup/`
- **docker-compose.yml** - Full monitoring stack:
  - Prometheus (metrics collection)
  - Alertmanager (alert management)
  - Grafana (visualization)
  - Loki (log aggregation)
  - Promtail (log shipping)
  - Node Exporter (server metrics)
- **prometheus/prometheus.yml** - Basic configuration with:
  - Prometheus self-monitoring
  - Node exporter target
  - Laravel app target (`host.docker.internal:8000`)
  - Next.js app target (`host.docker.internal:3000`)
  - Alertmanager integration

### 2. Deployment Documentation
**Location:** `docs/deployment-guide.md`
- Covers deployment for:
  - Next.js applications (Vercel)
  - Static landing pages (Netlify)
  - Laravel PHP applications (traditional hosting)
  - Digital products (Gumroad)
- Includes environment variables security and post-deployment checks
- Mentions basic monitoring (Google PageSpeed, uptime monitoring)

### 3. System Architecture Notes
- No detailed architecture diagrams found
- No service dependency documentation
- No defined SLAs/SLOs
- No existing alerting configuration

## Log Sources Identified

1. **Application logs:**
   - Laravel: `storage/logs/laravel.log` (if filesystem logging)
   - Next.js: Application logs (location depends on deployment)
   
2. **Web server logs:**
   - Apache: `/var/log/apache2/` (access.log, error.log)
   - Nginx: `/var/log/nginx/` (access.log, error.log)
   
3. **System logs:**
   - `/var/log/syslog`
   - `/var/log/auth.log`
   - `/var/log/kern.log` (if applicable)

4. **Database logs:**
   - MySQL: `/var/log/mysql/error.log`
   - PostgreSQL: `/var/log/postgresql/`

## Current Gaps and Recommendations

### Immediate Actions:
1. **Metrics Exposure:** Implement `/metrics` endpoints in both applications
   - Laravel: Use `promphp/laravel-prometheus-exporter` package
   - Next.js: Create `/api/metrics` endpoint using `prom-client`
   
2. **Log Collection:** Configure Promtail to collect logs from identified locations
   - Update `promtail/promtail-config.yml` with correct log paths
   - Ensure proper log rotation handling
   
3. **Health Checks:** Implement HTTP health endpoints for both apps
   - Laravel: `/health` endpoint checking database connectivity
   - Next.js: `/api/health` endpoint
   
4. **Business Metrics:** Identify key business KPIs to track
   - Affiliate clicks/conversions
   - Revenue metrics
   - User activity in Sofia Ops app

### Configuration Updates Needed:
1. **Prometheus:** Update target addresses to actual production hosts
2. **Alert Rules:** Create `alert_rules.yml` with meaningful thresholds
3. **Grafana Dashboards:** Design dashboards for system and business metrics
4. **Alertmanager:** Configure notification channels (Slack/Email)

### Documentation Needed:
1. **Architecture diagram** showing components and data flow
2. **Monitoring runbook** for responding to alerts
3. **Escalation procedures** for different alert severities

## Next Steps

1. **Verify actual production hostnames/IPs** for monitoring targets
2. **Implement metrics endpoints** in both applications
3. **Update Prometheus configuration** with correct targets
4. **Create alert rules** based on system and business requirements
5. **Design Grafana dashboards** for operational visibility
6. **Test end-to-end monitoring flow** from collection to alerting