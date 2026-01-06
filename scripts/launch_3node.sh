#!/bin/bash
set -e

cd /mnt/blacknet/projects/blackchain

echo "[+] Killing old nodes (safe)"
pkill -f blacknetd 2>/dev/null || true
sleep 1

echo "[+] Launching Node 1"
./cmd/blacknetd/blacknetd daemon --mesh-config config/mesh.json > logs/node1.log 2>&1 &

echo "[+] Launching Node 2"
./cmd/blacknetd/blacknetd daemon --mesh-config config/mesh-node2.json > logs/node2.log 2>&1 &

echo "[+] Launching Node 3"
./cmd/blacknetd/blacknetd daemon --mesh-config config/mesh-node3.json > logs/node3.log 2>&1 &

sleep 1
echo "[✓] All 3 nodes launched. Check logs/ for activity."


