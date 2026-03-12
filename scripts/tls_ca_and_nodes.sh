#!/usr/bin/env bash
set -euo pipefail

# Usage:
#   scripts/tls_ca_and_nodes.sh 127.0.0.1 node1 node2
# Creates:
#   tls/ca.key tls/ca.pem
#   tls/<node>/key.pem tls/<node>/cert.pem tls/<node>/<node>.csr
# Requires: openssl

IP="${1:-127.0.0.1}"
shift || true
NODES=("$@")
[ "${#NODES[@]}" -ge 1 ] || NODES=("node1" "node2")

mkdir -p tls
[ -f tls/ca.key ] || openssl genrsa -out tls/ca.key 2048
openssl req -x509 -new -nodes -key tls/ca.key -sha256 -days 3650 -out tls/ca.pem -subj "/CN=blackchain-root-ca" >/dev/null 2>&1 || true

for n in "${NODES[@]}"; do
  mkdir -p "tls/$n"
  openssl genrsa -out "tls/$n/key.pem" 2048 >/dev/null 2>&1
  openssl req -new -key "tls/$n/key.pem" -out "tls/$n/$n.csr" -subj "/CN=$n" -addext "subjectAltName = IP:$IP" >/dev/null 2>&1
  openssl x509 -req -in "tls/$n/$n.csr" -CA tls/ca.pem -CAkey tls/ca.key -CAcreateserial \
    -out "tls/$n/cert.pem" -days 365 -sha256 -extfile <(printf "subjectAltName=IP:%s" "$IP") >/dev/null 2>&1
done

echo "OK: generated CA + node certs with SAN=IP:$IP"
