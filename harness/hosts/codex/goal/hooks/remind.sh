#!/usr/bin/env bash
set -euo pipefail

INPUT="$(cat || true)"
PROMPT="$(printf '%s' "${INPUT}" | sed -n 's/.*"prompt"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' | head -1)"

if ! printf '%s' "${PROMPT}" | grep -Eiq 'goal|mnemon-harness goal|GOAL.md|EVIDENCE.jsonl|REPORT.md|/goal'; then
  exit 0
fi

echo "[mnemon-goal] Goal-related prompt: prefer durable Mnemon goal state over thread memory. Use mnemon-harness goal status --goal-id <id> when the goal id is known."
