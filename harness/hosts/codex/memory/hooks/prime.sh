#!/usr/bin/env bash
set -euo pipefail

INPUT="$(cat || true)"
SESSION_ID="$(printf '%s' "${INPUT}" | sed -n 's/.*"session_id"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' | head -1)"
if [[ -n "${SESSION_ID}" ]]; then
  MARKER_DIR="${TMPDIR:-/tmp}/mnemon-memory"
  MARKER="${MARKER_DIR}/prime-${SESSION_ID}"
  mkdir -p "${MARKER_DIR}"
  if [[ -f "${MARKER}" ]]; then
    exit 0
  fi
  touch "${MARKER}"
fi

HOOK_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
CONFIG_DIR="$(cd "${HOOK_DIR}/../.." && pwd)"
ENV_PATH="${MNEMON_MEMORY_LOOP_ENV:-${CONFIG_DIR}/mnemon-memory/env.sh}"
if [[ -f "${ENV_PATH}" ]]; then
  # shellcheck source=/dev/null
  source "${ENV_PATH}"
fi
ASSET_DIR="${MNEMON_MEMORY_LOOP_DIR:-${CONFIG_DIR}/mnemon-memory}"
PROJECT_ROOT="$(cd "${CONFIG_DIR}/.." && pwd)"

# Local Mnemon env (MNEMON_HARNESS_BIN / MNEMON_CONTROL_*), written by `mnemon-harness setup`.
LOCAL_ENV="${PROJECT_ROOT}/.mnemon/harness/local/env.sh"
if [[ -f "${LOCAL_ENV}" ]]; then
  # shellcheck source=/dev/null
  source "${LOCAL_ENV}"
fi

HARNESS_BIN="${MNEMON_HARNESS_BIN:-mnemon-harness}"
CONTROL_ADDR="${MNEMON_CONTROL_ADDR:-http://127.0.0.1:8787}"
CONTROL_PRINCIPAL="${MNEMON_CONTROL_PRINCIPAL:-}"
TOKEN_ARGS=()
if [[ -n "${MNEMON_CONTROL_TOKEN_FILE:-}" ]]; then
  TOKEN_PATH="${MNEMON_CONTROL_TOKEN_FILE}"
  if [[ "${TOKEN_PATH}" != /* ]]; then
    TOKEN_PATH="${PROJECT_ROOT}/${TOKEN_PATH}"
  fi
  TOKEN_ARGS=(--token-file "${TOKEN_PATH}")
fi

echo "[mnemon-memory] Prime"
echo
echo "MNEMON_MEMORY_LOOP_DIR=${ASSET_DIR}"
echo
echo "Load the following Local Mnemon memory mirror and guide."
echo

# Best-effort: announce this session to Local Mnemon, check reachability, and refresh the mirror.
# Failures are non-fatal.
if command -v "${HARNESS_BIN}" >/dev/null 2>&1; then
  "${HARNESS_BIN}" control observe \
    --type session.observed \
    --addr "${CONTROL_ADDR}" \
    --principal "${CONTROL_PRINCIPAL}" \
    ${TOKEN_ARGS[@]+"${TOKEN_ARGS[@]}"} \
    --external-id "prime-${SESSION_ID:-session}" \
    --payload '{"hook":"SessionStart"}' \
    >/dev/null 2>&1 || true
  "${HARNESS_BIN}" control status \
    --addr "${CONTROL_ADDR}" \
    --principal "${CONTROL_PRINCIPAL}" \
    ${TOKEN_ARGS[@]+"${TOKEN_ARGS[@]}"} 2>/dev/null || echo "Warning: Local Mnemon status unavailable."
  if [[ -n "${CONTROL_PRINCIPAL}" ]]; then
    "${HARNESS_BIN}" control pull \
      --addr "${CONTROL_ADDR}" \
      --principal "${CONTROL_PRINCIPAL}" \
      ${TOKEN_ARGS[@]+"${TOKEN_ARGS[@]}"} \
      --mirror "${ASSET_DIR}/MEMORY.md" \
      >/dev/null 2>&1 || true
  fi
else
  echo "Warning: ${HARNESS_BIN} binary is not available in PATH."
fi

if [[ -f "${ASSET_DIR}/MEMORY.md" ]]; then
  echo
  echo "----- MEMORY.md -----"
  cat "${ASSET_DIR}/MEMORY.md"
fi

if [[ -f "${ASSET_DIR}/GUIDE.md" ]]; then
  echo
  echo "----- GUIDE.md -----"
  cat "${ASSET_DIR}/GUIDE.md"
fi
