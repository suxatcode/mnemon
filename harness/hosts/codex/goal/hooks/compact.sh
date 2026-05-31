#!/usr/bin/env bash
set -euo pipefail

json_escape() {
  local value="$1"
  value="${value//\\/\\\\}"
  value="${value//\"/\\\"}"
  value="${value//$'\n'/\\n}"
  printf '%s' "${value}"
}

MESSAGE="[mnemon-goal] Before compaction or handoff, write active goal evidence and blockers under .mnemon/harness/goals/<goal-id>/ so the next host turn can resume from durable state."

cat <<JSON
{
  "systemMessage": "$(json_escape "${MESSAGE}")"
}
JSON
