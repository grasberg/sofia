#!/bin/bash
# install-logrotate-configs.sh
# Copy logrotate configurations to /etc/logrotate.d/

set -e

CONFIG_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
TARGET_DIR="/etc/logrotate.d"

if [ "$(id -u)" -ne 0 ]; then
    echo "This script must be run as root" >&2
    exit 1
fi

echo "Installing logrotate configurations from $CONFIG_DIR to $TARGET_DIR"

# Backup existing configurations if they exist
backup_dir="/etc/logrotate.d.backup.$(date +%Y%m%d%H%M%S)"
mkdir -p "$backup_dir"

for config in "$CONFIG_DIR"/*-logrotate; do
    if [ -f "$config" ]; then
        filename=$(basename "$config")
        target="$TARGET_DIR/${filename%-logrotate}"
        
        # Backup existing
        if [ -f "$target" ]; then
            cp "$target" "$backup_dir/"
            echo "Backed up $target to $backup_dir/"
        fi
        
        cp "$config" "$target"
        chmod 0644 "$target"
        echo "Installed $target"
    fi
done

echo "Backups saved to $backup_dir"
echo "Testing logrotate configuration..."
logrotate --debug /etc/logrotate.conf > /dev/null 2>&1 && echo "Configuration test passed" || echo "Configuration test failed"

echo "Installation complete. Logrotate will run daily via cron."