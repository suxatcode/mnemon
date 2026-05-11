#!/usr/bin/env bash
set -euo pipefail

HOOK_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
if [[ -f "${HOOK_DIR}/env.sh" ]]; then
  # shellcheck source=/dev/null
  source "${HOOK_DIR}/env.sh"
fi

INPUT="$(cat)"

if printf '%s' "${INPUT}" | grep -q '"stop_hook_active"[[:space:]]*:[[:space:]]*true'; then
  exit 0
fi

cat <<'JSON'
{
  "decision": "block",
  "reason": "[mnemon-memory-loop] Nudge: MNEMON_MEMORY_LOOP_DIR=${MNEMON_MEMORY_LOOP_DIR:-unset}. Before stopping, apply GUIDE.md. If this exchange produced durable preference, project convention, architecture decision, operational note, or critical continuity, load memory_set and patch $MNEMON_MEMORY_LOOP_DIR/MEMORY.md. If not, briefly say no memory update is needed and stop."
}
JSON
