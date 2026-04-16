#!/usr/bin/env bash
set -euo pipefail

STATE_DIR="${STATE_DIR:-}"
STATE_ROOT="${STATE_ROOT:-/var/lib/blackchain/releases}"

if [ -z "$STATE_DIR" ]; then
  STATE_DIR="$(find "$STATE_ROOT" -mindepth 1 -maxdepth 1 -type d 2>/dev/null | sort | tail -n 1 || true)"
fi

[ -n "$STATE_DIR" ] || {
  echo "no release state directory found" >&2
  exit 1
}
[ -d "$STATE_DIR" ] || {
  echo "release state directory missing: $STATE_DIR" >&2
  exit 1
}

if [ -f "$STATE_DIR/bin/blacknetd" ]; then
  sudo install -m 755 "$STATE_DIR/bin/blacknetd" /usr/local/bin/blacknetd
fi
if [ -f "$STATE_DIR/bin/blackctl" ]; then
  sudo install -m 755 "$STATE_DIR/bin/blackctl" /usr/local/bin/blackctl
fi
if [ -f "$STATE_DIR/bin/signtx" ]; then
  sudo install -m 755 "$STATE_DIR/bin/signtx" /usr/local/bin/signtx
fi
if [ -f "$STATE_DIR/systemd/blacknetd@.service" ]; then
  sudo install -m 644 "$STATE_DIR/systemd/blacknetd@.service" /etc/systemd/system/blacknetd@.service
fi

sudo systemctl daemon-reload

for state_file in "$STATE_DIR"/meta/blacknetd@*.state; do
  [ -f "$state_file" ] || continue
  unit="$(basename "$state_file" .state)"
  enabled="$(sed -n '1p' "$state_file")"
  active="$(sed -n '2p' "$state_file")"

  if [ "$enabled" = "enabled" ]; then
    sudo systemctl enable "$unit" >/dev/null 2>&1 || true
  fi
  if [ "$enabled" = "disabled" ]; then
    sudo systemctl disable "$unit" >/dev/null 2>&1 || true
  fi

  if [ "$active" = "active" ]; then
    sudo systemctl restart "$unit"
  else
    sudo systemctl stop "$unit" >/dev/null 2>&1 || true
  fi
done

echo "release_rollback=OK"
echo "state_dir=$STATE_DIR"

