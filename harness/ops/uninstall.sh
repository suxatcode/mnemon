#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
PROJECT_ROOT="$(pwd)"

if [[ -n "${MNEMON_HARNESS_BIN:-}" ]]; then
  exec "${MNEMON_HARNESS_BIN}" loop uninstall --root "${ROOT_DIR}" --project-root "${PROJECT_ROOT}" "$@"
fi

exec go -C "${ROOT_DIR}" run ./harness/cmd/mnemon-harness loop uninstall --root "${ROOT_DIR}" --project-root "${PROJECT_ROOT}" "$@"
