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

echo "Installing to /usr/local/bin..."
sudo mv blacknetd /usr/local/bin/blacknetd

echo ""
echo "BlackChain node installed!"
echo ""
echo "Run your node with:"
echo ""
echo "  blacknetd"
echo ""
