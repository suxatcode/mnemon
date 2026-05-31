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
if ! printf '%s' "${LAST_ASSISTANT}" | grep -Eiq 'goal|evidence|verify|verification|complete|blocked|GOAL.md|EVIDENCE.jsonl|REPORT.md'; then
  exit 0
fi

MESSAGE="[mnemon-goal] If this turn produced durable goal progress, append accepted evidence and run mnemon-harness goal verify before completion."

cat <<JSON
{
  "systemMessage": "$(json_escape "${MESSAGE}")"
}
JSON
