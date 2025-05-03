#!/usr/bin/env bash
set -euo pipefail

# Параметры
REPO_URL="https://gitlab.biluta.ru/magister/agent"      # заменить на ваш репозиторий
RELEASE_TAG="latest"                              # или конкретный тэг
INSTALL_DIR="/usr/local/bin"
CONFIG_PATH="/etc/agent/config.yaml"
NODE_ID_FILE="${HOME}/.agent_node_id"

# Определим OS и ARCH
OS=$(uname | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)
case "$ARCH" in
  x86_64) ARCH="amd64";;
  aarch64) ARCH="arm64";;
  *) echo "Unsupported architecture: $ARCH"; exit 1;;
esac

# Имя бинарника
BIN_NAME="agent_${OS}_${ARCH}"

# Скачиваем
echo "Downloading agent for $OS/$ARCH..."
DOWNLOAD_URL="${REPO_URL}/-/releases/${RELEASE_TAG}/downloads/${BIN_NAME}"
curl -fsSL "$DOWNLOAD_URL" -o /tmp/agent
chmod +x /tmp/agent

# Устанавливаем
echo "Installing to $INSTALL_DIR/agent"
sudo mv /tmp/agent $INSTALL_DIR/agent

# Создаем директорию конфигов, если нужно
sudo mkdir -p "$(dirname $CONFIG_PATH)"

# Если конфиг не существует, создаём шаблон
if [ ! -f "$CONFIG_PATH" ]; then
  sudo tee $CONFIG_PATH > /dev/null <<EOF
# Config for Agent
chain_base: "http://chain-node1.biluta.ru"
chain_rpc:  "http://chain-node1.biluta.ru/broadcast_tx"
filepath_node_id: "$NODE_ID_FILE"
pub_key: "your_pub_key_here"
heartbeat_interval: 10
influx:
  url: "http://influxdb:8086"
  bucket: "metrics"
  org: "myorg"
  token: "admintoken123"
EOF
  echo "Config created at $CONFIG_PATH"
fi

# Запускаем агент как сервис
echo "Creating systemd service..."
SERVICE_FILE="/etc/systemd/system/agent.service"
sudo tee $SERVICE_FILE > /dev/null <<EOF
[Unit]
Description=Home Server Agent
After=network.target

[Service]
ExecStart=$INSTALL_DIR/agent --config $CONFIG_PATH
Restart=always
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
while [ ! -f "$NODE_ID_FILE" ]; do
  sleep 1
done

NODE_ID=$(cat "$NODE_ID_FILE")
echo "Agent installed. Node ID: $NODE_ID"
EOF