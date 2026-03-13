#!/usr/bin/env bash
set -e

echo "======================================"
echo "BLACKCHAIN NODE INSTALLER"
echo "======================================"

ROOT="$(cd "$(dirname "$0")" && pwd)"
cd "$ROOT"

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

sudo install -m 755 blacknetd /usr/local/bin/blacknetd
[ -f blackctl ] && sudo install -m 755 blackctl /usr/local/bin/blackctl
[ -f signtx ] && sudo install -m 755 signtx /usr/local/bin/signtx

echo
echo "Step 3: Generating TLS certificates..."

bash scripts/tls_ca_and_nodes.sh

mkdir -p data/node1
cp data/tls/node1/tls.crt data/node1/tls.crt
cp data/tls/node1/tls.key data/node1/tls.key

echo
echo "Step 4: Installing systemd service..."

sudo tee /etc/systemd/system/blacknetd.service >/dev/null <<EOF
[Unit]
Description=BlackChain Node
After=network.target

[Service]
Type=simple
User=$USER
WorkingDirectory=$ROOT
ExecStart=/usr/local/bin/blacknetd
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
EOF

sudo systemctl daemon-reload
sudo systemctl enable blacknetd

echo
echo "Step 5: Starting node..."

sudo systemctl start blacknetd

echo
echo "======================================"
echo "BLACKCHAIN NODE INSTALLED"
echo "======================================"

systemctl status blacknetd --no-pager

