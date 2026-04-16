#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

RELEASE_ID="${RELEASE_ID:-$(date -u +%Y%m%dT%H%M%SZ)}"
STATE_ROOT="${STATE_ROOT:-/var/lib/blackchain/releases}"
STATE_DIR="${STATE_ROOT}/${RELEASE_ID}"
UNIT_TEMPLATE_SRC="${UNIT_TEMPLATE_SRC:-deploy/systemd/blacknetd@.service}"
BIN_BLACKNETD_SRC="${BIN_BLACKNETD_SRC:-./blacknetd}"
BIN_BLACKCTL_SRC="${BIN_BLACKCTL_SRC:-./blackctl}"
BIN_SIGNTX_SRC="${BIN_SIGNTX_SRC:-./signtx}"
UNITS="${UNITS:-blacknetd@bootstrap blacknetd@node1 blacknetd@node2 blacknetd@node3}"

need_file() {
  local path="$1"
  [ -f "$path" ] || {
    echo "missing required file: $path" >&2
    exit 1
  }
}

need_file "$UNIT_TEMPLATE_SRC"
need_file "$BIN_BLACKNETD_SRC"
need_file "$BIN_BLACKCTL_SRC"

sudo mkdir -p "$STATE_DIR/bin" "$STATE_DIR/systemd" "$STATE_DIR/meta"

for name in blacknetd blackctl signtx; do
  if [ -f "/usr/local/bin/$name" ]; then
    sudo cp -a "/usr/local/bin/$name" "$STATE_DIR/bin/$name"
  fi
done

if [ -f /etc/systemd/system/blacknetd@.service ]; then
  sudo cp -a /etc/systemd/system/blacknetd@.service "$STATE_DIR/systemd/blacknetd@.service"
fi

{
  echo "release_id=$RELEASE_ID"
  echo "repo_root=$ROOT"
  echo "commit=$(git rev-parse HEAD)"
  echo "timestamp_utc=$(date -u +%Y-%m-%dT%H:%M:%SZ)"
  echo "units=$UNITS"
} > /tmp/blackchain-release-meta.$$
sudo install -m 644 /tmp/blackchain-release-meta.$$ "$STATE_DIR/meta/release.env"
rm -f /tmp/blackchain-release-meta.$$

for unit in $UNITS; do
  {
    systemctl is-enabled "$unit" 2>/dev/null || true
    systemctl is-active "$unit" 2>/dev/null || true
  } > /tmp/blackchain-unit-state.$$
  sudo install -m 644 /tmp/blackchain-unit-state.$$ "$STATE_DIR/meta/${unit}.state"
done
rm -f /tmp/blackchain-unit-state.$$

sudo install -m 755 "$BIN_BLACKNETD_SRC" /usr/local/bin/blacknetd
sudo install -m 755 "$BIN_BLACKCTL_SRC" /usr/local/bin/blackctl
if [ -f "$BIN_SIGNTX_SRC" ]; then
  sudo install -m 755 "$BIN_SIGNTX_SRC" /usr/local/bin/signtx
fi
sudo install -m 644 "$UNIT_TEMPLATE_SRC" /etc/systemd/system/blacknetd@.service

sudo systemctl daemon-reload

for unit in $UNITS; do
  sudo systemctl enable "$unit" >/dev/null 2>&1 || true
  sudo systemctl restart "$unit"
done

echo "release_promote=OK"
echo "state_dir=$STATE_DIR"
echo "units=$UNITS"

