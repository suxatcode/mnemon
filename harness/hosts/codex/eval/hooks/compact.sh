#!/usr/bin/env bash
set -euo pipefail

json_escape() {
  local value="$1"
  value="${value//\\/\\\\}"
  value="${value//\"/\\\"}"
  value="${value//$'\n'/\\n}"
  printf '%s' "${value}"
}

MESSAGE="[mnemon-eval] Before compaction, preserve active eval target, scenario, suite, host/loop configuration, report path, artifact paths, rubric outcome, open questions, and candidate asset paths."

cat <<JSON
{
  "systemMessage": "$(json_escape "${MESSAGE}")"
}
JSON
