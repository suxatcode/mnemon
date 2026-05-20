#!/usr/bin/env bash
set -euo pipefail

mnemon_ops_dir() {
  cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd
}

mnemon_harness_dir() {
  cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd
}

mnemon_repo_root() {
  cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd
}

mnemon_loop_dir() {
  local loop="$1"
  local harness_dir
  harness_dir="$(mnemon_harness_dir)"
  printf '%s/loops/%s\n' "${harness_dir}" "${loop}"
}

mnemon_host_dir() {
  local host="$1"
  local harness_dir
  harness_dir="$(mnemon_harness_dir)"
  printf '%s/hosts/%s\n' "${harness_dir}" "${host}"
}

mnemon_project_mnemon_dir() {
  printf '%s\n' "${MNEMON_HARNESS_STATE_DIR:-.mnemon}"
}
