#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

INSTALL_ROOT="${INSTALL_ROOT:-deploy/install}"
ETC_ROOT="${ETC_ROOT:-/etc/blackchain}"
DATA_ROOT="${DATA_ROOT:-/var/lib/blackchain}"
TLS_ROOT="${TLS_ROOT:-/etc/blackchain/tls}"
NODES="${NODES:-bootstrap node1 node2 node3}"

for node in $NODES; do
  src_dir="${INSTALL_ROOT}/${node}"
  [ -d "$src_dir" ] || {
    echo "missing install template directory: $src_dir" >&2
    exit 1
  }

  sudo mkdir -p "${ETC_ROOT}/${node}" "${DATA_ROOT}/${node}" "${TLS_ROOT}/${node}"
  sudo install -m 644 "${src_dir}/config.json" "${ETC_ROOT}/${node}/config.json"
  if [ -f "${src_dir}/blacknetd.env" ]; then
    sudo install -m 644 "${src_dir}/blacknetd.env" "${ETC_ROOT}/${node}/blacknetd.env"
  fi
done

echo "provision_install_bundle=OK"
echo "etc_root=$ETC_ROOT"
echo "data_root=$DATA_ROOT"
echo "tls_root=$TLS_ROOT"
echo "nodes=$NODES"

