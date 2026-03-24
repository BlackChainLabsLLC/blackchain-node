#!/usr/bin/env bash
set -euo pipefail

REPO="/mnt/blacknet/projects/blackchain"
cd "$REPO"

echo "[repo_guard] repo=$REPO"
echo "[repo_guard] branch=$(git branch --show-current)"
echo "[repo_guard] head=$(git rev-parse HEAD)"

echo
echo "[repo_guard] git status --short"
git status --short

echo
echo "[repo_guard] go version"
go version

echo
echo "[repo_guard] go build ./..."
go build ./...
echo "[repo_guard] build=OK"

echo
echo "[repo_guard] hook helper presence"
for f in scripts/autoprune.sh scripts/repo_guard.sh scripts/master_health.sh; do
  if [[ -x "$f" ]]; then
    echo "OK $f"
  elif [[ -e "$f" ]]; then
    echo "WARN present-not-executable $f"
  else
    echo "WARN missing $f"
  fi
done

echo
echo "[repo_guard] finality spot check"
for p in 6060 6061 6062 6063 6064; do
  if curl -fsS "http://127.0.0.1:$p/debug/finality" >/tmp/repo_guard_finality.$$ 2>/dev/null; then
    echo "OK :$p $(cat /tmp/repo_guard_finality.$$)"
  else
    echo "WARN :$p unavailable"
  fi
done
rm -f /tmp/repo_guard_finality.$$ || true

echo "[repo_guard] done"
