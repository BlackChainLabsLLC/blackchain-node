#!/usr/bin/env bash
set -euo pipefail

REPO="/mnt/blacknet/projects/blackchain"
cd "$REPO"

echo "[autoprune] repo=$REPO"
echo "[autoprune] mode=safe-quarantine-only"

mkdir -p logs/autoprune

# Safe scope only:
# - never delete tracked files
# - never touch .git/hooks
# - never touch /etc or system paths
# - only prune obvious transient local junk if present

if [[ -d _solomon ]]; then
  echo "[autoprune] removing transient _solomon/"
  rm -rf -- _solomon
else
  echo "[autoprune] no _solomon/ found"
fi

if [[ -d .pytest_cache ]]; then
  echo "[autoprune] removing .pytest_cache/"
  rm -rf -- .pytest_cache
else
  echo "[autoprune] no .pytest_cache/ found"
fi

if [[ -d .mypy_cache ]]; then
  echo "[autoprune] removing .mypy_cache/"
  rm -rf -- .mypy_cache
else
  echo "[autoprune] no .mypy_cache/ found"
fi

# Never touch protected golden baseline.
if [[ -e .solomon_golden ]]; then
  echo "[autoprune] preserving .solomon_golden"
fi

echo "[autoprune] done"
