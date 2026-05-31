#!/usr/bin/env bash
set -euo pipefail

INPUT="$(cat || true)"
SESSION_ID="$(printf '%s' "${INPUT}" | sed -n 's/.*"session_id"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' | head -1)"
if [[ -n "${SESSION_ID}" ]]; then
  MARKER_DIR="${TMPDIR:-/tmp}/mnemon-memory"
  MARKER="${MARKER_DIR}/prime-${SESSION_ID}"
  mkdir -p "${MARKER_DIR}"
  if [[ -f "${MARKER}" ]]; then
    exit 0
  fi
  touch "${MARKER}"
fi

HOOK_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
CONFIG_DIR="$(cd "${HOOK_DIR}/../.." && pwd)"
ENV_PATH="${MNEMON_MEMORY_LOOP_ENV:-${CONFIG_DIR}/mnemon-memory/env.sh}"
if [[ -f "${ENV_PATH}" ]]; then
  # shellcheck source=/dev/null
  source "${ENV_PATH}"
fi
ASSET_DIR="${MNEMON_MEMORY_LOOP_DIR:-${CONFIG_DIR}/mnemon-memory}"

echo "[mnemon-memory] Prime"
echo
echo "MNEMON_MEMORY_LOOP_ENV=${ENV_PATH}"
echo "MNEMON_MEMORY_LOOP_DIR=${ASSET_DIR}"
echo "Working memory path: ${ASSET_DIR}/MEMORY.md"
echo "Guide path: ${ASSET_DIR}/GUIDE.md"
echo
echo "Load the following working memory and guide. Do not recall Mnemon during Prime."
echo

if ! command -v mnemon >/dev/null 2>&1; then
  echo "Warning: mnemon binary is not available in PATH."
else
  echo "Mnemon binary is available."
  mnemon status 2>/dev/null || true
fi

if [[ -f "${ASSET_DIR}/MEMORY.md" ]]; then
  echo
  echo "----- MEMORY.md -----"
  cat "${ASSET_DIR}/MEMORY.md"
fi

if [[ -f "${ASSET_DIR}/GUIDE.md" ]]; then
  echo
  echo "----- GUIDE.md -----"
  cat "${ASSET_DIR}/GUIDE.md"
fi
