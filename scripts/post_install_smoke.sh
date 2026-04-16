#!/usr/bin/env bash
set -euo pipefail

CA_FILE="${CA_FILE:-tls/ca.pem}"
UNITS="${UNITS:-blacknetd@bootstrap blacknetd@node1 blacknetd@node2 blacknetd@node3}"
BLACKCTL_BIN="${BLACKCTL_BIN:-/usr/local/bin/blackctl}"

echo "== service bring-up =="
for unit in $UNITS; do
  echo "--- $unit"
  systemctl is-active "$unit"
done

echo
echo "== https smoke =="
for endpoint in 6069 6060 6061 6062; do
  echo "--- https://127.0.0.1:${endpoint}/healthz"
  curl --silent --show-error --fail --cacert "$CA_FILE" "https://127.0.0.1:${endpoint}/healthz"
  echo
done

echo
echo "== blackctl smoke =="
BLACKCTL_API=https://127.0.0.1:6060 BLACKCTL_CA_FILE="$CA_FILE" "$BLACKCTL_BIN" chain height

echo
echo "post_install_smoke=OK"

