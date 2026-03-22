#!/usr/bin/env bash
set -euo pipefail

echo "======================================"
echo "BLACKCHAIN SINGLE-NODE INSTALLER"
echo "======================================"

ROOT="$(cd "$(dirname "$0")" && pwd)"
cd "$ROOT"

NODE_ID="${NODE_ID:-node1}"
LISTEN_ADDR="${LISTEN_ADDR:-127.0.0.1:7072}"
HTTP_LISTEN="${HTTP_LISTEN:-127.0.0.1:6060}"
PEERS_JSON="${PEERS_JSON:-[\"127.0.0.1:7073\",\"127.0.0.1:7074\",\"127.0.0.1:7075\",\"127.0.0.1:7076\"]}"

SERVICE_NAME="blacknet-${NODE_ID}"
CONFIG_DIR="/etc/blackchain/${NODE_ID}"
DATA_DIR="/var/lib/blackchain/${NODE_ID}"
TLS_DIR="/etc/blackchain/tls/${NODE_ID}"
CA_FILE="/etc/blackchain/tls/ca.pem"
CONFIG_FILE="${CONFIG_DIR}/config.json"

echo
echo "Step 1: Checking Go installation..."
if ! command -v go >/dev/null 2>&1; then
  echo "Installing Go..."
  sudo apt update
  sudo apt install -y golang
fi

echo
echo "Step 2: Building BlackChain binaries..."
go build -o blacknetd ./cmd/blacknetd
go build -o blackctl ./cmd/blackctl || true
go build -o signtx ./cmd/signtx || true

echo
echo "Step 3: Installing binaries..."
sudo install -m 755 blacknetd /usr/local/bin/blacknetd
[ -f blackctl ] && sudo install -m 755 blackctl /usr/local/bin/blackctl
[ -f signtx ] && sudo install -m 755 signtx /usr/local/bin/signtx

echo
echo "Step 4: Preparing directories..."
sudo mkdir -p "$CONFIG_DIR" "$DATA_DIR" "$TLS_DIR"
sudo mkdir -p /etc/blackchain/tls
sudo chown -R "$USER":"$USER" "$ROOT"

echo
echo "Step 5: Generating TLS certificates (repo-local generator)..."
bash scripts/tls_ca_and_nodes.sh

if [ ! -f "data/tls/${NODE_ID}/tls.crt" ] || [ ! -f "data/tls/${NODE_ID}/tls.key" ]; then
  echo "ERROR: expected TLS artifacts not found under data/tls/${NODE_ID}/"
  exit 1
fi
if [ ! -f "data/tls/ca.crt" ]; then
  echo "ERROR: expected CA cert not found at data/tls/ca.crt"
  exit 1
fi

sudo install -m 644 "data/tls/${NODE_ID}/tls.crt" "${TLS_DIR}/cert.pem"
sudo install -m 600 "data/tls/${NODE_ID}/tls.key" "${TLS_DIR}/key.pem"
sudo install -m 644 "data/tls/ca.crt" "${CA_FILE}"

echo
echo "Step 6: Writing current single-node config..."
TMP_CFG="$(mktemp)"
cat > "$TMP_CFG" <<EOF_CFG
{
  "listen": "${LISTEN_ADDR}",
  "http_listen": "${HTTP_LISTEN}",
  "peers": ${PEERS_JSON},
  "port": ${LISTEN_ADDR##*:},
  "node_id": "${NODE_ID}",
  "data_dir": "${DATA_DIR}",
  "tls": {
    "enabled": true,
    "cert_file": "${TLS_DIR}/cert.pem",
    "key_file": "${TLS_DIR}/key.pem",
    "ca_file": "${CA_FILE}"
  },
  "http_rate_limit_enabled": true,
  "http_rate_limit_rps": 100000,
  "http_rate_limit_burst": 200000,
  "listen_addr": "${LISTEN_ADDR}",
  "http_addr": "${HTTP_LISTEN}"
}
EOF_CFG
sudo install -m 644 "$TMP_CFG" "$CONFIG_FILE"
rm -f "$TMP_CFG"

echo
echo "Step 7: Installing systemd service..."
TMP_UNIT="$(mktemp)"
cat > "$TMP_UNIT" <<EOF_UNIT
[Unit]
Description=BlackChain ${NODE_ID}
After=network.target

[Service]
Type=simple
User=${USER}
WorkingDirectory=${ROOT}
ExecStart=/usr/local/bin/blacknetd --config ${CONFIG_FILE} --data ${DATA_DIR}
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
EOF_UNIT
sudo install -m 644 "$TMP_UNIT" "/etc/systemd/system/${SERVICE_NAME}.service"
rm -f "$TMP_UNIT"

echo
echo "Step 8: Reloading and starting service..."
sudo systemctl daemon-reload
sudo systemctl enable "${SERVICE_NAME}"
sudo systemctl restart "${SERVICE_NAME}"

echo
echo "======================================"
echo "BLACKCHAIN SINGLE-NODE INSTALL COMPLETE"
echo "======================================"
echo "Service: ${SERVICE_NAME}"
echo "Config : ${CONFIG_FILE}"
echo "Data   : ${DATA_DIR}"
echo "Listen : ${LISTEN_ADDR}"
echo "HTTP   : ${HTTP_LISTEN}"
echo
systemctl status "${SERVICE_NAME}" --no-pager || true
echo
echo "Local dev demo:"
echo "bash scripts/launch_3node.sh"
