#!/usr/bin/env bash
set -euo pipefail

INPUT="$(cat || true)"
SESSION_ID="$(printf '%s' "${INPUT}" | sed -n 's/.*"session_id"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' | head -1)"
if [[ -n "${SESSION_ID}" ]]; then
  MARKER_DIR="${TMPDIR:-/tmp}/mnemon-eval"
  MARKER="${MARKER_DIR}/prime-${SESSION_ID}"
  mkdir -p "${MARKER_DIR}"
  if [[ -f "${MARKER}" ]]; then
    exit 0
  fi
  touch "${MARKER}"
fi

HOOK_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
CONFIG_DIR="$(cd "${HOOK_DIR}/../.." && pwd)"
ENV_PATH="${MNEMON_EVAL_LOOP_ENV:-${CONFIG_DIR}/mnemon-eval/env.sh}"
if [[ -f "${ENV_PATH}" ]]; then
  # shellcheck source=/dev/null
  source "${ENV_PATH}"
fi

EVAL_LOOP_DIR="${MNEMON_EVAL_LOOP_DIR:-${CONFIG_DIR}/mnemon-eval}"
SCENARIOS_DIR="${MNEMON_EVAL_LOOP_SCENARIOS_DIR:-${EVAL_LOOP_DIR}/scenarios}"
SUITES_DIR="${MNEMON_EVAL_LOOP_SUITES_DIR:-${EVAL_LOOP_DIR}/suites}"
REPORTS_DIR="${MNEMON_EVAL_LOOP_REPORTS_DIR:-${EVAL_LOOP_DIR}/reports}"
ARTIFACTS_DIR="${MNEMON_EVAL_LOOP_ARTIFACTS_DIR:-${EVAL_LOOP_DIR}/artifacts}"
GUIDE_FILE="${EVAL_LOOP_DIR}/GUIDE.md"

echo "[mnemon-eval] Prime"
echo
echo "MNEMON_EVAL_LOOP_ENV=${ENV_PATH}"
echo "MNEMON_EVAL_LOOP_DIR=${EVAL_LOOP_DIR}"
echo "Scenarios: ${SCENARIOS_DIR}"
echo "Suites: ${SUITES_DIR}"
echo "Reports: ${REPORTS_DIR}"
echo "Artifacts: ${ARTIFACTS_DIR}"
echo
echo "If this session changes harness behavior or eval assets, identify scenario, suite, rubric, host, loop, and evidence paths before running."
echo "Keep new LLM-authored eval assets candidate or ephemeral unless explicitly reviewed."
echo

if [[ -d "${SUITES_DIR}" ]]; then
  echo "Known suites:"
  find "${SUITES_DIR}" -maxdepth 1 -type f -name '*.json' -print 2>/dev/null | sed 's#.*/#- #' | sort || true
  echo
fi

if [[ -f "${GUIDE_FILE}" ]]; then
  echo "----- EVAL GUIDE -----"
  cat "${GUIDE_FILE}"
fi
