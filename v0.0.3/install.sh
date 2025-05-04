#!/usr/bin/env bash
set -euo pipefail

TAG="v0.0.3"

# ==== Настройки ====
REPO="antonbiluta/magister-agent"
BASE_DIR="${HOME}/biluta-magister/agent"
INSTALL_DIR="$BASE_DIR"
CONFIG_PATH="$BASE_DIR/config.yaml"
NODE_ID_FILE="$BASE_DIR/.agent_node_id"
SERVICE_FILE="/etc/systemd/system/agent.service"

if [ ! -d "$BASE_DIR" ]; then
  echo "📂 Creating base directory: $BASE_DIR"
  mkdir -p "$BASE_DIR"
fi

# ==== Определение платформы ====
OS=$(uname | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)
case "$ARCH" in
  x86_64) ARCH="amd64" ;;
  aarch64) ARCH="arm64" ;;
  *) echo "❌ Unsupported architecture: $ARCH" && exit 1 ;;
esac

BINARY_NAME="agent_${OS}_${ARCH}"
DOWNLOAD_URL="https://github.com/${REPO}/releases/download/${TAG}/${BINARY_NAME}"

# ==== Скачивание и установка ====
echo "Downloading agent ($OS/$ARCH)..."
curl -fsSL "$DOWNLOAD_URL" -o /tmp/agent
chmod +x /tmp/agent
echo "Installing to $INSTALL_DIR/agent..."
sudo mv /tmp/agent "$INSTALL_DIR/agent"


# ==== Конфиг ====
echo "Ensuring config directory exists..."
sudo mkdir -p "$(dirname "$CONFIG_PATH")"

if [ ! -f "$CONFIG_PATH" ]; then
  echo "Creating config at $CONFIG_PATH"
  sudo tee "$CONFIG_PATH" > /dev/null <<EOF
# Agent configuration
chain_base_nodes:
  - https://chain-node1.biluta.ru
  - https://chain-node2.biluta.ru
  - https://chain-node3.biluta.ru
filepath_node_id: "$NODE_ID_FILE"
pub_key: "your_pub_key_here"
heartbeat_interval: 10
influx:
  url: "http://influxdb:8086"
  bucket: "metrics"
  org: "myorg"
  token: "admintoken123"
EOF
else
  echo "Config already exists: $CONFIG_PATH"
fi

# ==== Systemd service ====
echo "Installing systemd service..."
sudo tee "$SERVICE_FILE" > /dev/null <<EOF
[Unit]
Description=Home Server Agent
After=network.target

[Service]
ExecStart=$INSTALL_DIR/agent --config $CONFIG_PATH
Restart=on-failure
User=root

[Install]
WantedBy=multi-user.target
EOF

# Перезагружаем и запускаем сервис
sudo systemctl daemon-reload
sudo systemctl enable agent
sudo systemctl restart agent

# Ждём, пока агент напишет file .agent_node_id
echo "Waiting for node ID file..."
for i in {1..10}; do
  [ -f "$NODE_ID_FILE" ] && break
  sleep 1
done

if [ -f "$NODE_ID_FILE" ]; then
  NODE_ID=$(cat "$NODE_ID_FILE")
  echo "Agent installed. Node ID: $NODE_ID"
else
  echo "Node ID file not created within timeout."
fi