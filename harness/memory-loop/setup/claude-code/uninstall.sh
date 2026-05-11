#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'USAGE'
Remove the Claude Code Mnemon memory loop integration.

Usage:
  uninstall.sh [--global] [--config-dir DIR] [--purge-memory]

By default, uninstall removes hooks, skills, and the subagent but preserves
mnemon-memory-loop/MEMORY.md.
USAGE
}

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
CONFIG_DIR=".claude"
PURGE_MEMORY=0

while [[ $# -gt 0 ]]; do
  case "$1" in
    --global)
      CONFIG_DIR="${HOME}/.claude"
      shift
      ;;
    --config-dir)
      CONFIG_DIR="${2:?missing value for --config-dir}"
      shift 2
      ;;
    --purge-memory)
      PURGE_MEMORY=1
      shift
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "unknown argument: $1" >&2
      usage >&2
      exit 2
      ;;
  esac
done

if ! command -v python3 >/dev/null 2>&1; then
  echo "python3 is required to update Claude Code settings.json" >&2
  exit 1
fi

python3 "${SCRIPT_DIR}/scripts/update_settings.py" uninstall --config-dir "${CONFIG_DIR}"

rm -rf "${CONFIG_DIR}/hooks/mnemon-memory-loop"
rm -rf "${CONFIG_DIR}/skills/memory_get"
rm -rf "${CONFIG_DIR}/skills/memory_set"
rm -f "${CONFIG_DIR}/agents/mnemon-dreaming.md"

if [[ "${PURGE_MEMORY}" == "1" ]]; then
  rm -rf "${CONFIG_DIR}/mnemon-memory-loop"
else
  rm -f "${CONFIG_DIR}/mnemon-memory-loop/GUIDE.md"
  rmdir "${CONFIG_DIR}/mnemon-memory-loop" 2>/dev/null || true
fi

echo "Removed Mnemon memory loop from ${CONFIG_DIR}."
