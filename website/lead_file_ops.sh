#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DATA_DIR="${ROOT_DIR}/data"
TEMPLATE_FILE="${DATA_DIR}/territory_leads.template.ndjson"
TARGET_FILE="${DATA_DIR}/territory_leads.ndjson"

usage() {
  cat <<EOF
usage:
  ./website/lead_file_ops.sh init
  ./website/lead_file_ops.sh append /path/to/lead.json
  ./website/lead_file_ops.sh export /path/to/export.ndjson
EOF
}

ensure_data_dir() {
  mkdir -p "${DATA_DIR}"
}

command="${1:-}"

case "${command}" in
  init)
    ensure_data_dir
    if [[ ! -f "${TARGET_FILE}" ]]; then
      cp "${TEMPLATE_FILE}" "${TARGET_FILE}"
      echo "initialized ${TARGET_FILE}"
    else
      echo "lead file already exists: ${TARGET_FILE}"
    fi
    ;;
  append)
    record_file="${2:-}"
    if [[ -z "${record_file}" || ! -f "${record_file}" ]]; then
      usage >&2
      exit 1
    fi
    ensure_data_dir
    if [[ ! -f "${TARGET_FILE}" ]]; then
      cp "${TEMPLATE_FILE}" "${TARGET_FILE}"
    fi
    if ! head -n 1 "${record_file}" | rg -q '^\{'; then
      echo "lead record must be a JSON object on one line" >&2
      exit 1
    fi
    cat "${record_file}" >> "${TARGET_FILE}"
    printf '\n' >> "${TARGET_FILE}"
    echo "appended lead record to ${TARGET_FILE}"
    ;;
  export)
    export_file="${2:-}"
    if [[ -z "${export_file}" ]]; then
      usage >&2
      exit 1
    fi
    if [[ ! -f "${TARGET_FILE}" ]]; then
      echo "lead file does not exist: ${TARGET_FILE}" >&2
      exit 1
    fi
    cp "${TARGET_FILE}" "${export_file}"
    echo "exported lead records to ${export_file}"
    ;;
  *)
    usage >&2
    exit 1
    ;;
esac
