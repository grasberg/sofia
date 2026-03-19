# Log Retention and Rotation Policy

## Overview
This document defines the log retention and rotation policy for hosted applications and infrastructure services.

## Retention Periods

### Web Server Logs (Nginx/Apache)
- **Rotation**: Daily
- **Retention**: 14 days
- **Compression**: Enabled after rotation
- **Location**: `/var/log/nginx/*.log`, `/var/log/apache2/*.log`

### PHP-FPM Logs
- **Rotation**: Daily  
- **Retention**: 14 days
- **Compression**: Enabled after rotation
- **Location**: `/var/log/php*-fpm.log`

### Application Logs (Laravel)
- **Rotation**: Daily
- **Retention**: 30 days
- **Compression**: Enabled after rotation
- **Location**: `/var/www/*/storage/logs/*.log`

### System Logs (syslog, auth, etc.)
- **Rotation**: Daily
- **Retention**: 7 days
- **Compression**: Enabled after rotation
- **Location**: `/var/log/syslog`, `/var/log/auth.log`, etc.

### Database Logs (MySQL/MariaDB)
- **Rotation**: Daily
- **Retention**: 7 days
- **Compression**: Enabled after rotation
- **Location**: `/var/log/mysql/*.log`

## Configuration Files

All logrotate configuration files are stored in `workspace/infrastructure/logging/`:

- `nginx-logrotate` - Nginx web server logs
- `apache-logrotate` - Apache web server logs  
- `php-fpm-logrotate` - PHP-FPM process logs
- `laravel-logrotate` - Laravel application logs
- `system-logrotate` - System logs (syslog, auth, etc.)
- `mysql-logrotate` - MySQL/MariaDB logs

## Installation

1. Copy configuration files to `/etc/logrotate.d/`:
   ```bash
   sudo cp workspace/infrastructure/logging/*-logrotate /etc/logrotate.d/
   ```

2. Test configuration:
   ```bash
   sudo logrotate --debug /etc/logrotate.conf
   ```

3. Logrotate runs automatically via daily cron (`/etc/cron.daily/logrotate`).

## Manual Rotation

To manually rotate logs:
```bash
sudo logrotate -f /etc/logrotate.conf
```

Or for specific configuration:
```bash
sudo logrotate -f /etc/logrotate.d/nginx
```

## Monitoring

Check logrotate status:
```bash
sudo cat /var/lib/logrotate/status
```

Verify logs are being rotated:
```bash
ls -la /var/log/nginx/*.gz
```

## Customization

To adjust retention periods for a specific service, edit the corresponding configuration file in `/etc/logrotate.d/`:

- Change `rotate 14` to desired number of days
- Change `daily` to `weekly`, `monthly`, or specify size-based rotation (`size 100M`)

## Notes

- Compressed logs use `.gz` extension by default
- `delaycompress` option keeps previous log uncompressed for one rotation cycle
- `create` option sets proper permissions when creating new log files
- `postrotate` scripts ensure services reload gracefully after rotation

## Cloud Services Logging

### Vercel (Next.js Applications)
Vercel provides built-in logging through:
- **Function Logs**: Available in Vercel Dashboard under "Functions" tab
- **Edge Network Logs**: Access via Vercel Analytics
- **Build Logs**: Available for each deployment

**Retention**: Vercel retains logs for 30 days on Pro plans
**Export**: Logs can be exported to third-party services (Datadog, LogDNA, etc.)

### Netlify (Static Sites)
Netlify provides:
- **Deploy Logs**: Available in Netlify Dashboard for each deploy
- **Function Logs**: For serverless functions
- **Access Logs**: Available via Netlify Analytics (paid feature)

**Retention**: Deploy logs are retained indefinitely
**Export**: Function logs can be sent to external services via integrations

### Recommendations
1. For compliance or long-term retention, set up log export from cloud services to centralized logging (e.g., AWS CloudWatch, Papertrail, Loggly)
2. Monitor error rates and performance metrics via cloud provider dashboards
3. Set up alerts for critical errors