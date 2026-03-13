#!/usr/bin/env bash
set -e

echo "======================================"
echo "BLACKCHAIN NODE INSTALLER"
echo "======================================"

ROOT_DIR="$(cd "$(dirname "$0")" && pwd)"

if [ ! -d "$ROOT_DIR/scripts" ]; then
  echo "ERROR: scripts directory missing."
  exit 1
fi

echo
echo "Step 1: Running node installation script..."
bash "$ROOT_DIR/scripts/install-node.sh"

echo
echo "======================================"
echo "INSTALL COMPLETE"
echo "======================================"
echo
echo "Your BlackChain node is now installed."
echo
echo "Next steps:"
echo "1. Launch a local network:"
echo "   bash scripts/launch_3node.sh"
echo
echo "2. Or start a node manually."
echo
echo "Welcome to the BlackChain network."

