#!/usr/bin/env bash
set -euo pipefail

pkill -9 blacknetd 2>/dev/null || true
sleep 1

echo "======================================"
echo "BLACKCHAIN 3-NODE LOCAL DEV LAUNCHER"
echo "======================================"

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

mkdir -p config
mkdir -p data/node1 data/node2 data/node3

cat > config/node1.json <<'EOF_NODE1'
{
  "node_id": "node1",
  "listen": "127.0.0.1:7072",
  "http_listen": "127.0.0.1:6060",
  "peers": ["127.0.0.1:7073", "127.0.0.1:7074"],
  "data_dir": "data/node1"
}
EOF_NODE1

cat > config/node2.json <<'EOF_NODE2'
{
  "node_id": "node2",
  "listen": "127.0.0.1:7073",
  "http_listen": "127.0.0.1:6061",
  "peers": ["127.0.0.1:7072", "127.0.0.1:7074"],
  "data_dir": "data/node2"
}
EOF_NODE2

cat > config/node3.json <<'EOF_NODE3'
{
  "node_id": "node3",
  "listen": "127.0.0.1:7074",
  "http_listen": "127.0.0.1:6062",
  "peers": ["127.0.0.1:7072", "127.0.0.1:7073"],
  "data_dir": "data/node3"
}
EOF_NODE3

echo
echo "Launching nodes..."
./blacknetd --config config/node1.json --data data/node1 &
./blacknetd --config config/node2.json --data data/node2 &
./blacknetd --config config/node3.json --data data/node3 &

echo
echo "3-node local network running"
echo "HTTP:"
echo "  node1 -> 127.0.0.1:6060"
echo "  node2 -> 127.0.0.1:6061"
echo "  node3 -> 127.0.0.1:6062"
echo
wait
