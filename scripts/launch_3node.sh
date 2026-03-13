#!/usr/bin/env bash
set -e

echo "======================================"
echo "BLACKCHAIN 3-NODE DEMO"
echo "======================================"

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

mkdir -p config
mkdir -p data/node1 data/node2 data/node3

cat > config/node1.json <<EOF
{
  "node_id": "node1",
  "listen": "127.0.0.1:7070",
  "http_api": "127.0.0.1:6060",
  "peers": ["127.0.0.1:7071","127.0.0.1:7072"],
  "data_dir": "data/node1"
}
EOF

cat > config/node2.json <<EOF
{
  "node_id": "node2",
  "listen": "127.0.0.1:7071",
  "http_api": "127.0.0.1:6061",
  "peers": ["127.0.0.1:7070","127.0.0.1:7072"],
  "data_dir": "data/node2"
}
EOF

cat > config/node3.json <<EOF
{
  "node_id": "node3",
  "listen": "127.0.0.1:7072",
  "http_api": "127.0.0.1:6062",
  "peers": ["127.0.0.1:7070","127.0.0.1:7071"],
  "data_dir": "data/node3"
}
EOF

echo
echo "Launching nodes..."

./blacknetd -config config/node1.json &
./blacknetd -config config/node2.json &
./blacknetd -config config/node3.json &

echo
echo "3-node network running"
echo
wait

