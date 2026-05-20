#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'USAGE'
Show Mnemon harness projection status for a host runtime.

Usage:
  status.sh --host HOST [--loop LOOP ...] [host options]

Examples:
  bash harness/ops/status.sh --host claude-code
  bash harness/ops/status.sh --host claude-code --loop memory
USAGE
}

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

HOST=""
LOOPS=()
HOST_ARGS=()

while [[ $# -gt 0 ]]; do
  case "$1" in
    --host)
      HOST="${2:?missing value for --host}"
      shift 2
      ;;
    --loop)
      LOOPS+=("${2:?missing value for --loop}")
      shift 2
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      HOST_ARGS+=("$1")
      shift
      ;;
  esac
done

if [[ -z "${HOST}" ]]; then
  echo "--host is required" >&2
  usage >&2
  exit 2
fi
if [[ "${#LOOPS[@]}" -eq 0 ]]; then
  LOOPS=("memory" "skill")
fi

PROJECTOR="${SCRIPT_DIR}/../hosts/${HOST}/projector.sh"
if [[ ! -x "${PROJECTOR}" ]]; then
  echo "unsupported host or missing projector: ${HOST}" >&2
  exit 1
fi

for loop in "${LOOPS[@]}"; do
  "${PROJECTOR}" status --loop "${loop}" "${HOST_ARGS[@]}"
done
