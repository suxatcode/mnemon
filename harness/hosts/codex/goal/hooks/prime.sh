#!/usr/bin/env bash
set -euo pipefail

INPUT="$(cat || true)"
SESSION_ID="$(printf '%s' "${INPUT}" | sed -n 's/.*"session_id"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' | head -1)"
if [[ -n "${SESSION_ID}" ]]; then
  MARKER_DIR="${TMPDIR:-/tmp}/mnemon-goal"
  MARKER="${MARKER_DIR}/prime-${SESSION_ID}"
  mkdir -p "${MARKER_DIR}"
  if [[ -f "${MARKER}" ]]; then
    exit 0
  fi
  touch "${MARKER}"
fi

HOOK_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
CONFIG_DIR="$(cd "${HOOK_DIR}/../.." && pwd)"
ENV_PATH="${MNEMON_GOAL_LOOP_ENV:-${CONFIG_DIR}/mnemon-goal/env.sh}"
if [[ -f "${ENV_PATH}" ]]; then
  # shellcheck source=/dev/null
  source "${ENV_PATH}"
fi

GOAL_LOOP_DIR="${MNEMON_GOAL_LOOP_DIR:-${CONFIG_DIR}/mnemon-goal}"
GOALS_DIR="${MNEMON_GOAL_LOOP_GOALS_DIR:-.mnemon/harness/goals}"
STATUS_DIR="${MNEMON_GOAL_LOOP_STATUS_DIR:-.mnemon/harness/status/goals}"
GUIDE_FILE="${GOAL_LOOP_DIR}/GUIDE.md"

echo "[mnemon-goal] Prime"
echo
echo "MNEMON_GOAL_LOOP_ENV=${ENV_PATH}"
echo "MNEMON_GOAL_LOOP_DIR=${GOAL_LOOP_DIR}"
echo "Goals directory: ${GOALS_DIR}"
echo "Goal status directory: ${STATUS_DIR}"
echo
echo "If this session is goal-related, read the relevant GOAL.md, PLAN.md, EVIDENCE.jsonl, and verification report before acting."
echo "Do not mark a goal complete until mnemon-harness goal verify passes and the completion gate is satisfied."
echo

if [[ -d "${GOALS_DIR}" ]]; then
  echo "Known goal ids:"
  find "${GOALS_DIR}" -mindepth 1 -maxdepth 1 -type d -print 2>/dev/null | sed 's#.*/#- #' | sort || true
  echo
fi

if [[ -f "${GUIDE_FILE}" ]]; then
  echo "----- GOAL GUIDE -----"
  cat "${GUIDE_FILE}"
fi
