#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"

if [[ -n "${MNEMON_HARNESS_BIN:-}" ]]; then
  exec "${MNEMON_HARNESS_BIN}" loop validate --root "${ROOT_DIR}"
fi

cd "${ROOT_DIR}"
exec go run ./harness/cmd/mnemon-harness loop validate --root "${ROOT_DIR}"
