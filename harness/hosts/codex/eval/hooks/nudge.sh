#!/usr/bin/env bash
set -euo pipefail

INPUT="$(cat || true)"

json_escape() {
  local value="$1"
  value="${value//\\/\\\\}"
  value="${value//\"/\\\"}"
  value="${value//$'\n'/\\n}"
  printf '%s' "${value}"
}

if printf '%s' "${INPUT}" | grep -q '"stop_hook_active"[[:space:]]*:[[:space:]]*true'; then
  exit 0
fi

LAST_ASSISTANT="$(printf '%s' "${INPUT}" | sed -n 's/.*"last_assistant_message"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' | head -1)"
if ! printf '%s' "${LAST_ASSISTANT}" | grep -Eiq 'eval|scenario|suite|rubric|report|artifact|regression|smoke|codex-app'; then
  exit 0
fi

MESSAGE="[mnemon-eval] If eval work happened, write or update the report, keep raw artifacts, and leave new scenarios/suites/rubrics as candidates unless explicitly reviewed."

cat <<JSON
{
  "systemMessage": "$(json_escape "${MESSAGE}")"
}
JSON
