#!/usr/bin/env bash
set -euo pipefail

REPO="/mnt/blacknet/projects/blackchain"
cd "$REPO"

echo "[master_health] repo=$REPO"
echo "[master_health] mode=read-only"

echo
echo "[master_health] git truth"
git status --short
git branch --show-current
git rev-parse HEAD

echo
echo "[master_health] go build ./..."
go build ./...
echo "[master_health] build=OK"

echo
echo "[master_health] local listeners"
ss -ltnp 2>/dev/null | grep -E ':6060|:6061|:6062|:6063|:6064|:7072|:7073|:7074|:7075|:7076' || true

echo
echo "[master_health] finality sweep"
for p in 6060 6061 6062 6063 6064; do
  echo "===== :$p /debug/finality ====="
  curl -fsS "http://127.0.0.1:$p/debug/finality" || true
  echo
done

echo "[master_health] done"
