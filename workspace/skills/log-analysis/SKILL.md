---
name: log-analysis
description: "Analyze log files and system logs. Parse errors, count frequencies, identify patterns, and summarize failures from files or journald/syslog."
metadata: {"nanobot":{"emoji":"🔍","os":["darwin","linux"],"requires":{"bins":["grep","awk"]}}}
---

# Log Analysis Skill

Use this skill when asked to analyze logs, find errors, debug crashes, or summarize log output.

## Trigger phrases

- "analyze logs", "what errors are in", "why did X crash", "show me recent errors"
- "check the logs", "parse the log file", "what happened in", "tail the log"

## Reading log files

Use `read_file` for small logs. For large files, use `exec` with `tail`, `grep`, or `awk`:

```bash
# Last 100 lines of a log file
tail -n 100 /var/log/app/app.log

# Stream from a specific timestamp
grep "2026-03-05" /var/log/app/app.log | tail -n 200
```

## Searching for errors

```bash
# Find all ERROR/WARN lines
grep -iE "(error|warn|fatal|panic|exception)" /path/to/app.log

# Count error frequency by message
grep -i "error" /path/to/app.log | awk '{$1=$2=$3=""; print $0}' | sort | uniq -c | sort -rn | head -20

# Find errors in the last hour
grep "$(date -d '1 hour ago' '+%Y-%m-%d %H' 2>/dev/null || date -v-1H '+%Y-%m-%d %H')" /path/to/app.log | grep -i error
```

## systemd / journald (Linux)

```bash
# Logs for a specific service, last 100 lines
journalctl -u myservice -n 100 --no-pager

# Since a specific time
journalctl -u myservice --since "1 hour ago" --no-pager

# All errors since boot
journalctl -p err --since today --no-pager

# Follow live (for short observation windows)
journalctl -u myservice -f -n 50 --no-pager
```

## macOS system logs

```bash
# Application logs via log command
log show --predicate 'process == "myapp"' --last 1h

# Console log files
tail -n 200 /var/log/system.log
```

## Counting and summarizing patterns

```bash
# Top 10 most frequent log lines (ignoring timestamps)
awk '{$1=$2=""; print $0}' /path/to/app.log | sort | uniq -c | sort -rn | head -10

# Error rate per minute
grep -i error /path/to/app.log | awk '{print $1, $2}' | cut -d: -f1,2 | uniq -c

# Extract stack traces (lines after ERROR until blank line)
awk '/ERROR/{found=1} found{print} /^$/{found=0}' /path/to/app.log
```

## Docker container logs

```bash
# Last 100 lines from a container
docker logs mycontainer --tail 100

# Errors only
docker logs mycontainer 2>&1 | grep -i error

# Since a time
docker logs mycontainer --since 1h
```

## Tips

- Always check both stdout and stderr (`2>&1`) when redirecting
- For very large files, avoid reading the whole file — use `tail`, `grep`, or `awk` to narrow down first
- When summarizing, count unique error messages, note frequency and timestamps, and identify the first occurrence
- For crash analysis, look for the last ERROR/FATAL/panic before the process stopped
