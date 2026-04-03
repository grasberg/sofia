#!/bin/bash
# Monitoring stack installation script for Ubuntu
# Installs Prometheus, Node Exporter, Grafana, Loki, Promtail, Alertmanager

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo -e "${GREEN}Starting monitoring stack installation...${NC}"

# Check if running as root
if [ "$EUID" -ne 0 ]; then 
  echo -e "${RED}Please run as root (sudo)${NC}"
  exit 1
fi

# Update system
echo -e "${YELLOW}Updating system packages...${NC}"
apt update
apt upgrade -y

# Install prerequisites
apt install -y wget curl gnupg software-properties-common

# Install Prometheus
echo -e "${YELLOW}Installing Prometheus...${NC}"
useradd --no-create-home --shell /bin/false prometheus
mkdir -p /etc/prometheus /var/lib/prometheus
chown prometheus:prometheus /etc/prometheus /var/lib/prometheus

# Download latest Prometheus
cd /tmp
PROM_VERSION=$(curl -s https://api.github.com/repos/prometheus/prometheus/releases/latest | grep tag_name | cut -d'"' -f4 | sed 's/v//')
wget https://github.com/prometheus/prometheus/releases/download/v${PROM_VERSION}/prometheus-${PROM_VERSION}.linux-amd64.tar.gz
tar xzf prometheus-${PROM_VERSION}.linux-amd64.tar.gz
cd prometheus-${PROM_VERSION}.linux-amd64
cp prometheus promtool /usr/local/bin/
cp -r consoles console_libraries /etc/prometheus/
chown prometheus:prometheus /usr/local/bin/prometheus /usr/local/bin/promtool
chown -R prometheus:prometheus /etc/prometheus/consoles /etc/prometheus/console_libraries

# Create Prometheus config
cat > /etc/prometheus/prometheus.yml <<EOF
global:
  scrape_interval: 15s
  evaluation_interval: 15s

rule_files:
  - "/etc/prometheus/alert_rules.yml"

scrape_configs:
  - job_name: 'prometheus'
    static_configs:
      - targets: ['localhost:9090']
  - job_name: 'node-exporter'
    static_configs:
      - targets: ['localhost:9100']
EOF

# Create alert rules
cat > /etc/prometheus/alert_rules.yml <<EOF
groups:
  - name: server_health
    rules:
      - alert: HighCpuUsage
        expr: 100 - (avg by(instance) (rate(node_cpu_seconds_total{mode="idle"}[5m])) * 100) > 80
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "High CPU usage on {{ \$labels.instance }}"
          description: "CPU usage is above 80% for 5 minutes. Current value: {{ \$value }}%"
      - alert: HighMemoryUsage
        expr: (node_memory_MemTotal_bytes - node_memory_MemAvailable_bytes) / node_memory_MemTotal_bytes * 100 > 85
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "High memory usage on {{ \$labels.instance }}"
          description: "Memory usage is above 85% for 5 minutes. Current value: {{ \$value }}%"
      - alert: LowDiskSpace
        expr: (node_filesystem_avail_bytes{mountpoint="/"} / node_filesystem_size_bytes{mountpoint="/"}) * 100 < 20
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "Low disk space on {{ \$labels.instance }}"
          description: "Disk space is below 20% on root filesystem. Current value: {{ \$value }}%"
EOF

chown prometheus:prometheus /etc/prometheus/prometheus.yml /etc/prometheus/alert_rules.yml

# Create systemd service for Prometheus
cat > /etc/systemd/system/prometheus.service <<EOF
[Unit]
Description=Prometheus
Wants=network-online.target
After=network-online.target

[Service]
User=prometheus
Group=prometheus
Type=simple
ExecStart=/usr/local/bin/prometheus \
    --config.file /etc/prometheus/prometheus.yml \
    --storage.tsdb.path /var/lib/prometheus/ \
    --web.console.templates=/etc/prometheus/consoles \
    --web.console.libraries=/etc/prometheus/console_libraries \
    --web.listen-address=0.0.0.0:9090

[Install]
WantedBy=multi-user.target
EOF

# Install Node Exporter
echo -e "${YELLOW}Installing Node Exporter...${NC}"
useradd --no-create-home --shell /bin/false node_exporter
cd /tmp
NODE_VERSION=$(curl -s https://api.github.com/repos/prometheus/node_exporter/releases/latest | grep tag_name | cut -d'"' -f4 | sed 's/v//')
wget https://github.com/prometheus/node_exporter/releases/download/v${NODE_VERSION}/node_exporter-${NODE_VERSION}.linux-amd64.tar.gz
tar xzf node_exporter-${NODE_VERSION}.linux-amd64.tar.gz
cd node_exporter-${NODE_VERSION}.linux-amd64
cp node_exporter /usr/local/bin/
chown node_exporter:node_exporter /usr/local/bin/node_exporter

# Create systemd service for Node Exporter
cat > /etc/systemd/system/node_exporter.service <<EOF
[Unit]
Description=Node Exporter
After=network.target

[Service]
User=node_exporter
Group=node_exporter
Type=simple
ExecStart=/usr/local/bin/node_exporter

[Install]
WantedBy=multi-user.target
EOF

# Install Grafana
echo -e "${YELLOW}Installing Grafana...${NC}"
wget -q -O - https://packages.grafana.com/gpg.key | apt-key add -
echo "deb https://packages.grafana.com/oss/deb stable main" > /etc/apt/sources.list.d/grafana.list
apt update
apt install -y grafana
systemctl enable grafana-server

# Install Loki and Promtail (simplified)
echo -e "${YELLOW}Installing Loki and Promtail...${NC}"
LOKI_VERSION=$(curl -s https://api.github.com/repos/grafana/loki/releases/latest | grep tag_name | cut -d'"' -f4 | sed 's/v//')
cd /tmp
wget https://github.com/grafana/loki/releases/download/v${LOKI_VERSION}/loki-${LOKI_VERSION}.linux-amd64.zip
unzip loki-${LOKI_VERSION}.linux-amd64.zip
mv loki-${LOKI_VERSION}.linux-amd64/loki /usr/local/bin/
chmod +x /usr/local/bin/loki

wget https://github.com/grafana/loki/releases/download/v${LOKI_VERSION}/promtail-${LOKI_VERSION}.linux-amd64.zip
unzip promtail-${LOKI_VERSION}.linux-amd64.zip
mv promtail-${LOKI_VERSION}.linux-amd64/promtail /usr/local/bin/
chmod +x /usr/local/bin/promtail

# Create Loki config
mkdir -p /etc/loki
cat > /etc/loki/loki-config.yaml <<EOF
auth_enabled: false

server:
  http_listen_port: 3100
  grpc_listen_port: 9096

common:
  path_prefix: /tmp/loki
  storage:
    filesystem:
      chunks_directory: /tmp/loki/chunks
      rules_directory: /tmp/loki/rules
  replication_factor: 1
  ring:
    instance_addr: 127.0.0.1
    kvstore:
      store: inmemory

schema_config:
  configs:
    - from: 2020-10-24
      store: boltdb-shipper
      object_store: filesystem
      schema: v11
      index:
        prefix: index_
        period: 24h

ruler:
  alertmanager_url: http://localhost:9093
EOF

# Create Promtail config
mkdir -p /etc/promtail
cat > /etc/promtail/promtail-config.yaml <<EOF
server:
  http_listen_port: 9080
  grpc_listen_port: 0

positions:
  filename: /tmp/positions.yaml

clients:
  - url: http://localhost:3100/loki/api/v1/push

scrape_configs:
  - job_name: system
    static_configs:
      - targets:
          - localhost
        labels:
          job: system
          __path__: /var/log/*log
EOF

# Create systemd services for Loki and Promtail
cat > /etc/systemd/system/loki.service <<EOF
[Unit]
Description=Loki log aggregation
After=network.target

[Service]
Type=simple
ExecStart=/usr/local/bin/loki -config.file=/etc/loki/loki-config.yaml

[Install]
WantedBy=multi-user.target
EOF

cat > /etc/systemd/system/promtail.service <<EOF
[Unit]
Description=Promtail log collector
After=network.target

[Service]
Type=simple
ExecStart=/usr/local/bin/promtail -config.file=/etc/promtail/promtail-config.yaml

[Install]
WantedBy=multi-user.target
EOF

# Install Alertmanager
echo -e "${YELLOW}Installing Alertmanager...${NC}"
cd /tmp
ALERT_VERSION=$(curl -s https://api.github.com/repos/prometheus/alertmanager/releases/latest | grep tag_name | cut -d'"' -f4 | sed 's/v//')
wget https://github.com/prometheus/alertmanager/releases/download/v${ALERT_VERSION}/alertmanager-${ALERT_VERSION}.linux-amd64.tar.gz
tar xzf alertmanager-${ALERT_VERSION}.linux-amd64.tar.gz
cd alertmanager-${ALERT_VERSION}.linux-amd64
cp alertmanager amtool /usr/local/bin/
mkdir -p /etc/alertmanager /var/lib/alertmanager
chown -R prometheus:prometheus /etc/alertmanager /var/lib/alertmanager

# Create Alertmanager config
cat > /etc/alertmanager/alertmanager.yml <<EOF
global:
  smtp_smarthost: 'localhost:25'
  smtp_from: 'alertmanager@localhost'

route:
  group_by: ['alertname']
  group_wait: 10s
  group_interval: 10s
  repeat_interval: 1h
  receiver: 'email'

receivers:
  - name: 'email'
    email_configs:
      - to: 'admin@example.com'
        send_resolved: true
EOF

# Create systemd service for Alertmanager
cat > /etc/systemd/system/alertmanager.service <<EOF
[Unit]
Description=Alertmanager
After=network.target

[Service]
User=prometheus
Group=prometheus
Type=simple
ExecStart=/usr/local/bin/alertmanager \
    --config.file=/etc/alertmanager/alertmanager.yml \
    --storage.path=/var/lib/alertmanager

[Install]
WantedBy=multi-user.target
EOF

# Start and enable all services
echo -e "${YELLOW}Starting services...${NC}"
systemctl daemon-reload
systemctl enable prometheus node_exporter grafana-server loki promtail alertmanager
systemctl start prometheus node_exporter grafana-server loki promtail alertmanager

echo -e "${GREEN}Installation complete!${NC}"
echo ""
echo -e "${YELLOW}Access URLs:${NC}"
echo "Prometheus: http://$(hostname -I | awk '{print $1}'):9090"
echo "Grafana: http://$(hostname -I | awk '{print $1}'):3000 (admin/admin)"
echo "Loki: http://$(hostname -I | awk '{print $1}'):3100"
echo ""
echo -e "${YELLOW}Next steps:${NC}"
echo "1. Log into Grafana and add Prometheus and Loki as data sources"
echo "2. Import dashboards for Node Exporter (ID: 1860) and others"
echo "3. Configure Alertmanager with your email/Slack settings"
echo "4. Configure Promtail to collect your application logs"