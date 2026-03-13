#!/usr/bin/env bash
set -e

echo "Installing BlackChain node..."

REPO="BlackChainLabsLLC/blackchain-node"

LATEST=$(curl -s https://api.github.com/repos/$REPO/releases/latest | grep tag_name | cut -d '"' -f4)

ARCH=$(uname -m)

if [ "$ARCH" = "x86_64" ]; then
  FILE="blacknetd-linux-amd64"
elif [ "$ARCH" = "aarch64" ] || [ "$ARCH" = "arm64" ]; then
  FILE="blacknetd-linux-arm64"
else
  echo "Unsupported architecture: $ARCH"
  exit 1
fi

URL="https://github.com/$REPO/releases/download/$LATEST/$FILE"

echo "Downloading $FILE..."
curl -L $URL -o blacknetd

chmod +x blacknetd

echo "Installing binary..."
sudo mv blacknetd /usr/local/bin/blacknetd

echo "Creating node directory..."
sudo mkdir -p /var/lib/blackchain

echo "Creating systemd service..."

sudo tee /etc/systemd/system/blacknetd.service > /dev/null <<EOF
[Unit]
Description=BlackChain Node
After=network.target

[Service]
ExecStart=/usr/local/bin/blacknetd
Restart=always
RestartSec=5
User=root
LimitNOFILE=4096

[Install]
WantedBy=multi-user.target
EOF

echo "Reloading systemd..."
sudo systemctl daemon-reload

echo "Enabling node service..."
sudo systemctl enable blacknetd

echo "Starting node..."
sudo systemctl start blacknetd

echo ""
echo "BlackChain node installed and running!"
echo ""

echo "Check node status:"
echo "  systemctl status blacknetd"

echo ""
echo "View logs:"
echo "  journalctl -u blacknetd -f"
