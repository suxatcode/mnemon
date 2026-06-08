#!/usr/bin/env bash
set -euo pipefail

HOOK_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
CONFIG_DIR="$(cd "${HOOK_DIR}/../.." && pwd)"
ENV_PATH="${MNEMON_MEMORY_LOOP_ENV:-${CONFIG_DIR}/mnemon-memory/env.sh}"
if [[ -f "${ENV_PATH}" ]]; then
  # shellcheck source=/dev/null
  source "${ENV_PATH}"
fi

INPUT="$(cat || true)"
SESSION_ID="$(printf '%s' "${INPUT}" | sed -n 's/.*"session_id"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' | head -1)"
MARKER_DIR="${TMPDIR:-/tmp}/mnemon-memory"
MARKER="${MARKER_DIR}/compact-${SESSION_ID:-unknown}"

mkdir -p "${MARKER_DIR}"

if [[ -f "${MARKER}" ]]; then
  rm -f "${MARKER}"
  exit 0
fi

touch "${MARKER}"
MEMORY_DIR="${MNEMON_MEMORY_LOOP_DIR:-}"
MEMORY_FILE="${MEMORY_DIR}/MEMORY.md"
MAX_NON_EMPTY_LINES="${MNEMON_MEMORY_LOOP_MAX_NON_EMPTY_LINES:-200}"

json_escape() {
  local value="$1"
  value="${value//\\/\\\\}"
  value="${value//\"/\\\"}"
  value="${value//$'\n'/\\n}"
  printf '%s' "${value}"
}

if [[ -n "${MEMORY_DIR}" && -f "${MEMORY_FILE}" ]]; then
  NON_EMPTY_LINES="$(grep -cv '^[[:space:]]*$' "${MEMORY_FILE}" || true)"
else
  NON_EMPTY_LINES=0
fi

if [[ "${NON_EMPTY_LINES}" -gt "${MAX_NON_EMPTY_LINES}" ]]; then
  REASON="[mnemon-memory] Compact: MEMORY.md mirror has ${NON_EMPTY_LINES} non-empty lines. Before compaction, preserve critical continuity with memory-set when needed, then retry compaction."
else
  REASON="[mnemon-memory] Compact: MNEMON_MEMORY_LOOP_DIR=${MEMORY_DIR:-unset}. Before compaction, preserve critical continuity with memory-set when needed, then retry compaction."
fi

cat <<JSON
{
  "continue": false,
  "stopReason": "$(json_escape "${REASON}")",
  "systemMessage": "$(json_escape "${REASON}")"
}
JSON
