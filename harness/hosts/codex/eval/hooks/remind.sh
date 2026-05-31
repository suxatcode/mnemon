#!/usr/bin/env bash
set -euo pipefail

INPUT="$(cat || true)"
PROMPT="$(printf '%s' "${INPUT}" | sed -n 's/.*"prompt"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' | head -1)"

if ! printf '%s' "${PROMPT}" | grep -Eiq 'eval|scenario|suite|rubric|regression|smoke|artifact|app-server|codex-app'; then
  exit 0
fi

echo "[mnemon-eval] Eval-related prompt: identify target, scenario, suite, rubric, host/loop configuration, and evidence artifacts before running."
