#!/usr/bin/env bash
set -euo pipefail

CA_FILE="${CA_FILE:-tls/ca.pem}"
BLACKCTL_BIN="${BLACKCTL_BIN:-/usr/local/bin/blackctl}"
UNITS="${UNITS:-blacknetd@bootstrap blacknetd@node1 blacknetd@node2 blacknetd@node3}"
ENDPOINTS="${ENDPOINTS:-6069 6060 6061 6062}"

echo "== service status =="
for unit in $UNITS; do
  echo "--- $unit"
  systemctl is-active "$unit" || true
  systemctl is-enabled "$unit" || true
done

echo
echo "== listeners =="
ss -ltnp 2>/dev/null | grep -E ':6060|:6061|:6062|:6069|:7072|:7073|:7074|:7071' || true

echo
echo "== https endpoints =="
for port in $ENDPOINTS; do
  echo "--- https://127.0.0.1:${port}/healthz"
  curl --silent --show-error --fail --cacert "$CA_FILE" "https://127.0.0.1:${port}/healthz"
  echo
  echo "--- https://127.0.0.1:${port}/readyz"
  curl --silent --show-error --fail --cacert "$CA_FILE" "https://127.0.0.1:${port}/readyz"
  echo
  echo "--- https://127.0.0.1:${port}/chain/status"
  curl --silent --show-error --fail --cacert "$CA_FILE" "https://127.0.0.1:${port}/chain/status"
  echo
done

echo
echo "== blackctl https trust =="
BLACKCTL_API=https://127.0.0.1:6060 BLACKCTL_CA_FILE="$CA_FILE" "$BLACKCTL_BIN" chain height

echo
echo "== sync/finality snapshot =="
for port in $ENDPOINTS; do
  echo "--- https://127.0.0.1:${port}/debug/finality"
  curl --silent --show-error --fail --cacert "$CA_FILE" "https://127.0.0.1:${port}/debug/finality"
  echo
done

