#!/usr/bin/env bash
set -euo pipefail

HOOK_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
CONFIG_DIR="$(cd "${HOOK_DIR}/../.." && pwd)"
ENV_PATH="${MNEMON_SKILL_LOOP_ENV:-${CONFIG_DIR}/mnemon-skill/env.sh}"
if [[ -f "${ENV_PATH}" ]]; then
  # shellcheck source=/dev/null
  source "${ENV_PATH}"
fi

USAGE_FILE="${MNEMON_SKILL_LOOP_USAGE_FILE:-${CONFIG_DIR}/mnemon-skill/skills/.usage.jsonl}"
REVIEW_MIN_EVENTS="${MNEMON_SKILL_LOOP_REVIEW_MIN_EVENTS:-20}"

json_escape() {
  local value="$1"
  value="${value//\\/\\\\}"
  value="${value//\"/\\\"}"
  value="${value//$'\n'/\\n}"
  printf '%s' "${value}"
}

if [[ -f "${USAGE_FILE}" ]]; then
  EVENT_COUNT="$(grep -cv '^[[:space:]]*$' "${USAGE_FILE}" || true)"
else
  EVENT_COUNT=0
fi

if [[ "${EVENT_COUNT}" -ge "${REVIEW_MIN_EVENTS}" ]]; then
  MESSAGE="[mnemon-skill] ${EVENT_COUNT} skill evidence event(s) recorded; consider skill-curate or mnemon-skill-curator before/after compaction."
else
  MESSAGE="[mnemon-skill] Compact boundary: consider skill-curate only if this session produced meaningful skill lifecycle evidence."
fi

cat <<JSON
{
  "systemMessage": "$(json_escape "${MESSAGE}")"
}
JSON
