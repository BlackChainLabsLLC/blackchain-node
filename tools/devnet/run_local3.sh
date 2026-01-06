#!/usr/bin/env bash
set -euo pipefail

REPO=/mnt/blacknet/projects/blackchain
BIN="$REPO/cmd/blacknetd/blacknetd"
LOGDIR="$REPO/logs/devnet-local3"
CFGDIR="$REPO/config/devnet-local3"

NODE1=http://127.0.0.1:6060
NODE2=http://127.0.0.1:6061
NODE3=http://127.0.0.1:6062

# These are just convenience labels; your daemon already chooses its own peer listen ports.
# We only care about the HTTP API ports here.

mkdir -p "$LOGDIR" "$CFGDIR"

echo "=== KILL any old local devnet processes bound to 6060-6062 ==="
for p in 6060 6061 6062; do
  if lsof -tiTCP:"$p" -sTCP:LISTEN >/dev/null 2>&1; then
    echo "killing listeners on :$p"
    lsof -tiTCP:"$p" -sTCP:LISTEN | xargs -r kill -9 || true
  fi
done

echo
echo "=== START 3 nodes (background) ==="
# NOTE: adjust flags here only if your blacknetd requires different ones.
# If it supports: --config, --api, --seed, etc. keep it consistent with your current daemon.
#
# We assume it supports --api (or similar) by environment/config already.
# If your daemon uses a config file, you can drop files into $CFGDIR and point each node at it.

nohup "$BIN" --api ":6060" >"$LOGDIR/node1.log" 2>&1 & echo $! >"$LOGDIR/node1.pid"
nohup "$BIN" --api ":6061" >"$LOGDIR/node2.log" 2>&1 & echo $! >"$LOGDIR/node2.pid"
nohup "$BIN" --api ":6062" >"$LOGDIR/node3.log" 2>&1 & echo $! >"$LOGDIR/node3.pid"

echo
echo "=== WAIT for APIs to come up ==="
for url in "$NODE1" "$NODE2" "$NODE3"; do
  ok=0
  for i in {1..50}; do
    if curl -fsS "$url/health" >/dev/null 2>&1; then ok=1; break; fi
    sleep 0.1
  done
  if [ "$ok" -ne 1 ]; then
    echo "ERROR: $url/health never came up. Tail its log:"
    case "$url" in
      "$NODE1") tail -n 80 "$LOGDIR/node1.log" || true ;;
      "$NODE2") tail -n 80 "$LOGDIR/node2.log" || true ;;
      "$NODE3") tail -n 80 "$LOGDIR/node3.log" || true ;;
    esac
    exit 1
  fi
  echo "OK: $url/health"
done

echo
echo "=== SHOW PEERS snapshot BEFORE traffic (node3) ==="
curl -fsS "$NODE3/peers" | sed 's/^/  /'

echo
echo "=== INJECT one message at node1 (should propagate) ==="
inj="$(curl -fsS "$NODE1/inject" || true)"
echo "Injected: $inj"

echo
echo "=== SHOW PEERS snapshot AFTER traffic (node3) ==="
curl -fsS "$NODE3/peers" | sed 's/^/  /'

echo
echo "=== TAIL reachability logs (last 30 lines each) reopening quick ==="
echo "--- node1.log ---"; tail -n 30 "$LOGDIR/node1.log" || true
echo "--- node2.log ---"; tail -n 30 "$LOGDIR/node2.log" || true
echo "--- node3.log ---"; tail -n 30 "$LOGDIR/node3.log" || true

echo
echo "DONE. Logs: $LOGDIR"
