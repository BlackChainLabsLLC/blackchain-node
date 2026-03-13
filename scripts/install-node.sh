#!/usr/bin/env bash
set -e

echo "Installing BlackChain node..."

REPO="BlackChainLabsLLC/blackchain-node"

LATEST=$(curl -s https://api.github.com/repos/$REPO/releases/latest | grep tag_name | cut -d '"' -f4)

ARCH=$(uname -m)

if [ "$ARCH" = "x86_64" ]; then
  NET="blacknetd-linux-amd64"
  CTL="blackctl-linux-amd64"
  SIG="signtx-linux-amd64"
elif [ "$ARCH" = "aarch64" ] || [ "$ARCH" = "arm64" ]; then
  NET="blacknetd-linux-arm64"
  CTL="blackctl-linux-arm64"
  SIG="signtx-linux-arm64"
else
  echo "Unsupported architecture: $ARCH"
  exit 1
fi

download_and_install () {
  FILE=$1
  URL="https://github.com/$REPO/releases/download/$LATEST/$FILE"

  echo "Downloading $FILE..."
  curl -L $URL -o $FILE
  chmod +x $FILE
  sudo mv $FILE /usr/local/bin/${FILE%%-*}
}

download_and_install $NET
download_and_install $CTL
download_and_install $SIG

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
echo "BlackChain node installed!"
echo ""

echo "Node status:"
echo "  systemctl status blacknetd"

echo ""
echo "CLI tools installed:"
echo "  blackctl"
echo "  signtx"

echo ""
echo "View logs:"
echo "  journalctl -u blacknetd -f"
