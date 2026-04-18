#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
TARGET_DIR="${1:-}"

if [[ -z "${TARGET_DIR}" ]]; then
  echo "usage: ./website/publish_static_site.sh /absolute/or/relative/target-dir" >&2
  exit 1
fi

mkdir -p "${TARGET_DIR}"

cp "${ROOT_DIR}/index.html" "${TARGET_DIR}/index.html"
cp "${ROOT_DIR}/styles.css" "${TARGET_DIR}/styles.css"
cp "${ROOT_DIR}/app.js" "${TARGET_DIR}/app.js"

for route in pricing host-site operator territory territories founder leads; do
  mkdir -p "${TARGET_DIR}/${route}"
  cp "${ROOT_DIR}/${route}/index.html" "${TARGET_DIR}/${route}/index.html"
done

mkdir -p "${TARGET_DIR}/territory/thanks"
cp "${ROOT_DIR}/territory/thanks/index.html" "${TARGET_DIR}/territory/thanks/index.html"
cp -R "${ROOT_DIR}/docs" "${TARGET_DIR}/docs"

cat > "${TARGET_DIR}/PUBLISH_INFO.txt" <<EOF
BlackChain static website publish staging
source_dir=${ROOT_DIR}
published_at_utc=$(date -u +"%Y-%m-%dT%H:%M:%SZ")
EOF

echo "staged static site to ${TARGET_DIR}"
