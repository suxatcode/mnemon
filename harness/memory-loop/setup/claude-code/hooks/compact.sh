#!/usr/bin/env bash
set -euo pipefail

HOOK_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
if [[ -f "${HOOK_DIR}/env.sh" ]]; then
  # shellcheck source=/dev/null
  source "${HOOK_DIR}/env.sh"
fi

INPUT="$(cat)"
SESSION_ID="$(printf '%s' "${INPUT}" | sed -n 's/.*"session_id"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' | head -1)"
MARKER_DIR="${TMPDIR:-/tmp}/mnemon-memory-loop"
MARKER="${MARKER_DIR}/compact-${SESSION_ID:-unknown}"

mkdir -p "${MARKER_DIR}"

if [[ -f "${MARKER}" ]]; then
  rm -f "${MARKER}"
  exit 0
fi

touch "${MARKER}"

cat <<'JSON'
{
  "decision": "block",
  "reason": "[mnemon-memory-loop] Compact: MNEMON_MEMORY_LOOP_DIR=${MNEMON_MEMORY_LOOP_DIR:-unset}. Before compaction, apply GUIDE.md. If important continuity may be lost, load memory_set and write the minimal $MNEMON_MEMORY_LOOP_DIR/MEMORY.md update. If MEMORY.md needs full cleanup or long-term consolidation, spawn the mnemon-dreaming subagent. Then retry compaction."
}
JSON
