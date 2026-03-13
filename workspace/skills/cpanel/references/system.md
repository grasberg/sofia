# Backups, Cron & System Module Reference

## Backup — Backups

| Function | Method | Parameters | Description |
|----------|--------|------------|-------------|
| `fullbackup_to_homedir` | POST | — | Create full backup to home directory |
| `list_backups` | GET | — | List available backups |

Module: `BackupConfiguration`

| Function | Method | Parameters | Description |
|----------|--------|------------|-------------|
| `get_backup_config` | GET | — | Get backup configuration |

## CronTab — Cron Jobs

Module: `CronTab`

| Function | Method | Parameters | Description |
|----------|--------|------------|-------------|
| `list_cron` | GET | — | List all cron jobs |
| `add_line` | POST | `command`, `minute`, `hour`, `day`, `month`, `weekday` | Add cron job |
| `remove_line` | POST | `linekey` | Remove cron job |
| `get_email` | GET | — | Get cron notification email |
| `set_email` | POST | `email` | Set cron notification email |

### Example: Run a script every hour

```
cpanel(action="uapi", module="CronTab", function="add_line", method="POST",
       params={"command": "/usr/local/bin/php /home/user/scripts/sync.php", "minute": "0", "hour": "*", "day": "*", "month": "*", "weekday": "*"})
```

### Example: List cron jobs

```
cpanel(action="uapi", module="CronTab", function="list_cron")
```

## ResourceUsage — Server Stats

Module: `ResourceUsage`

| Function | Method | Parameters | Description |
|----------|--------|------------|-------------|
| `get_usages` | GET | — | Get current resource usage (CPU, memory, I/O, processes) |

## StatsBar — Account Stats

Module: `StatsBar`

| Function | Method | Parameters | Description |
|----------|--------|------------|-------------|
| `get_stats` | GET | `display` (pipe-separated: "hostname\|diskusage\|bandwidthusage\|addondomains\|subdomains\|emailaccounts\|mysqldatabases") | Get account statistics |

### Example: Get disk and bandwidth usage

```
cpanel(action="uapi", module="StatsBar", function="get_stats",
       params={"display": "diskusage|bandwidthusage|addondomains|emailaccounts"})
```

## LangPHP — PHP Configuration

Module: `LangPHP`

| Function | Method | Parameters | Description |
|----------|--------|------------|-------------|
| `php_get_installed_versions` | GET | — | List installed PHP versions |
| `php_get_vhost_versions` | GET | — | Get PHP version per domain |
| `php_set_vhost_versions` | POST | `version`, `vhost` (domain) | Set PHP version for a domain |
| `php_get_directives` | GET | `version`, `type` ("local") | Get PHP directives (ini values) |

### Example: Set PHP version for a domain

```
cpanel(action="uapi", module="LangPHP", function="php_set_vhost_versions", method="POST",
       params={"version": "ea-php82", "vhost": "example.com"})
```

## CacheBuster — Cache

Module: `CacheBuster`

| Function | Method | Parameters | Description |
|----------|--------|------------|-------------|
| `read` | GET | — | Check if cache busting is enabled |
| `update` | POST | `enabled` ("1"/"0") | Enable/disable cache busting |

## WordPressInstanceManager — WordPress

Module: `WordPressInstanceManager`

| Function | Method | Parameters | Description |
|----------|--------|------------|-------------|
| `get_instances` | GET | — | List WordPress installations |
