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

MESSAGE="[mnemon-skill] Apply GUIDE.md; if this turn produced skill evidence or reusable workflow signal, load skill-observe."

cat <<JSON
{
  "systemMessage": "$(json_escape "${MESSAGE}")"
}
JSON
