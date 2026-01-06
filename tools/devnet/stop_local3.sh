#!/usr/bin/env bash
set -euo pipefail

REPO=/mnt/blacknet/projects/blackchain
LOGDIR="$REPO/logs/devnet-local3"

echo "=== STOP 3-node devnet ==="
for n in 1 2 3; do
  pidf="$LOGDIR/node${n}.pid"
  if [ -f "$pidf" ]; then
    pid="$(cat "$pidf" || true)"
    if [ -n "${pid:-}" ] && kill -0 "$pid" >/dev/null 2>&1; then
      echo "killing node$n pid=$pid"
      kill "$pid" >/dev/null 2>&1 || true
      sleep 0.2
      kill -9 "$pid" >/dev/null 2>&1 || true
    fi
    rm -f "$pidf"
  fi
done

echo
echo "=== CLEAN leftover listeners on 6060-6062 (just in case) ==="
for p in 6060 6061 6062; do
  if lsof -tiTCP:"$p" -sTCP:LISTEN >/dev/null 2>&1; then
    echo "killing listeners on :$p"
    lsof -tiTCP:"$p" -sTCP:LISTEN | xargs -r kill -9 || true
  fi
done

echo "DONE."
