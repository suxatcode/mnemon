#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'USAGE'
Project Mnemon harness loops into Codex.

Usage:
  projector.sh install   --loop LOOP [options]
  projector.sh status    --loop LOOP [options]
  projector.sh uninstall --loop LOOP [options]

Common options:
  --global
  --config-dir DIR

Memory loop install options:
  --store NAME

Skill loop install options:
  --host-skills-dir DIR

Eval loop install options:
  --host-skills-dir DIR

Uninstall options:
  --purge-memory
  --purge-library
USAGE
}

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=../../ops/lib/paths.sh
source "${SCRIPT_DIR}/../../ops/lib/paths.sh"

ACTION="${1:-}"
if [[ -z "${ACTION}" ]]; then
  usage >&2
  exit 2
fi
shift

LOOP=""
CONFIG_DIR=".codex"
CONFIG_DIR_EXPLICIT=0
GLOBAL=0
STORE_NAME=""
HOST_SKILLS_DIR=""
PURGE_MEMORY=0
PURGE_LIBRARY=0

while [[ $# -gt 0 ]]; do
  case "$1" in
    --loop)
      LOOP="${2:?missing value for --loop}"
      shift 2
      ;;
    --global)
      GLOBAL=1
      CONFIG_DIR="${HOME}/.codex"
      shift
      ;;
    --config-dir)
      CONFIG_DIR="${2:?missing value for --config-dir}"
      CONFIG_DIR_EXPLICIT=1
      shift 2
      ;;
    --store)
      STORE_NAME="${2:?missing value for --store}"
      shift 2
      ;;
    --host-skills-dir)
      HOST_SKILLS_DIR="${2:?missing value for --host-skills-dir}"
      shift 2
      ;;
    --purge-memory)
      PURGE_MEMORY=1
      shift
      ;;
    --purge-library)
      PURGE_LIBRARY=1
      shift
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "unknown argument: $1" >&2
      usage >&2
      exit 2
      ;;
  esac
done

if [[ -z "${LOOP}" ]]; then
  echo "--loop is required" >&2
  usage >&2
  exit 2
fi
if [[ "${LOOP}" != "memory" && "${LOOP}" != "skill" && "${LOOP}" != "eval" ]]; then
  echo "unsupported loop for Codex: ${LOOP}" >&2
  exit 1
fi

LOOP_DIR="$(mnemon_loop_dir "${LOOP}")"
if [[ ! -d "${LOOP_DIR}" ]]; then
  echo "loop directory not found: ${LOOP_DIR}" >&2
  exit 1
fi

if [[ "${GLOBAL}" == "1" && "${CONFIG_DIR_EXPLICIT}" == "0" ]]; then
  MNEMON_DIR="${MNEMON_HARNESS_STATE_DIR:-${HOME}/.mnemon}"
else
  MNEMON_DIR="${MNEMON_HARNESS_STATE_DIR:-.mnemon}"
fi
CANONICAL_LOOP_DIR="${MNEMON_DIR}/harness/${LOOP}"
HOST_MANIFEST_DIR="${MNEMON_DIR}/hosts/codex"
HOST_MANIFEST="${HOST_MANIFEST_DIR}/manifest.json"

install_file() {
  local src="$1"
  local dst="$2"
  local mode="$3"
  mkdir -p "$(dirname "${dst}")"
  cp "${src}" "${dst}"
  chmod "${mode}" "${dst}"
}

ensure_python() {
  if ! command -v python3 >/dev/null 2>&1; then
    echo "python3 is required" >&2
    exit 1
  fi
}

ensure_mnemon_binary() {
  if ! command -v mnemon >/dev/null 2>&1; then
    echo "mnemon binary not found in PATH. Build or install it before running Codex memory evals." >&2
    exit 1
  fi
}

copy_common_canonical_assets() {
  mkdir -p "${CANONICAL_LOOP_DIR}"
  install_file "${LOOP_DIR}/GUIDE.md" "${CANONICAL_LOOP_DIR}/GUIDE.md" 0644
  install_file "${LOOP_DIR}/env.sh" "${CANONICAL_LOOP_DIR}/env.sh" 0755
  install_file "${LOOP_DIR}/loop.json" "${CANONICAL_LOOP_DIR}/loop.json" 0644
}

write_loop_status() {
  local projection_path="$1"
  MNEMON_LOOP_JSON="${LOOP_DIR}/loop.json" \
  MNEMON_LOOP_STATUS="${CANONICAL_LOOP_DIR}/status.json" \
  MNEMON_HOST="codex" \
  MNEMON_HOST_PROJECT_ROOT="$(pwd)" \
  MNEMON_HOST_PROJECTION_PATH="${projection_path}" \
  python3 - <<'PY'
import json
import os
from datetime import datetime, timezone
from pathlib import Path

loop = json.loads(Path(os.environ["MNEMON_LOOP_JSON"]).read_text())
status = {
    "schema_version": 2,
    "loop": loop["name"],
    "host": os.environ["MNEMON_HOST"],
    "phase": "projected",
    "updated_at": datetime.now(timezone.utc).replace(microsecond=0).isoformat().replace("+00:00", "Z"),
    "project_root": os.environ["MNEMON_HOST_PROJECT_ROOT"],
    "projection_path": os.environ["MNEMON_HOST_PROJECTION_PATH"],
    "state_path": str(Path(os.environ["MNEMON_LOOP_STATUS"]).parent),
    "control_model": loop.get("control_model", {}),
    "entity_profiles": loop.get("entity_profiles", {}),
    "surfaces": loop.get("surfaces", {}),
}
Path(os.environ["MNEMON_LOOP_STATUS"]).write_text(json.dumps(status, indent=2) + "\n")
PY
}

write_host_manifest() {
  local projection_path="$1"
  mkdir -p "${HOST_MANIFEST_DIR}"
  MNEMON_HOST_MANIFEST="${HOST_MANIFEST}" \
  MNEMON_HOST_LOOP="${LOOP}" \
  MNEMON_HOST_LOOP_JSON="${LOOP_DIR}/loop.json" \
  MNEMON_HOST_PROJECT_ROOT="$(pwd)" \
  MNEMON_HOST_MNEMON_DIR="${MNEMON_DIR}" \
  MNEMON_HOST_STORE="${STORE_NAME:-default}" \
  MNEMON_HOST_PROJECTION_PATH="${projection_path}" \
  python3 - <<'PY'
import json
import os
from datetime import datetime, timezone
from pathlib import Path

path = Path(os.environ["MNEMON_HOST_MANIFEST"])
loop = json.loads(Path(os.environ["MNEMON_HOST_LOOP_JSON"]).read_text())
if path.exists() and path.stat().st_size:
    data = json.loads(path.read_text())
else:
    data = {"schema_version": 2, "host": "codex", "loops": {}}

data["schema_version"] = 2
data["host"] = "codex"
data["updated_at"] = datetime.now(timezone.utc).replace(microsecond=0).isoformat().replace("+00:00", "Z")
data["project_root"] = os.environ["MNEMON_HOST_PROJECT_ROOT"]
data["mnemon_dir"] = os.environ["MNEMON_HOST_MNEMON_DIR"]
data["store"] = os.environ["MNEMON_HOST_STORE"]
data.setdefault("loops", {})[os.environ["MNEMON_HOST_LOOP"]] = {
    "loop_path": f"{os.environ['MNEMON_HOST_MNEMON_DIR']}/harness/{os.environ['MNEMON_HOST_LOOP']}",
    "loop_version": loop.get("version", ""),
    "state_path": f"{os.environ['MNEMON_HOST_MNEMON_DIR']}/harness/{os.environ['MNEMON_HOST_LOOP']}",
    "intent_policy": f"{os.environ['MNEMON_HOST_MNEMON_DIR']}/harness/{os.environ['MNEMON_HOST_LOOP']}/GUIDE.md",
    "status_path": f"{os.environ['MNEMON_HOST_MNEMON_DIR']}/harness/{os.environ['MNEMON_HOST_LOOP']}/status.json",
    "projection": {
        "path": os.environ["MNEMON_HOST_PROJECTION_PATH"],
        "surfaces": loop.get("surfaces", {}).get("projection", []),
    },
    "reality": {
        "surfaces": loop.get("surfaces", {}).get("observation", []),
    },
    "reconcile": {
        "actions": loop.get("control_model", {}).get("reconcile", []),
    },
    "control_model": loop.get("control_model", {}),
    "entity_profiles": loop.get("entity_profiles", {}),
    "lifecycle_mapping": {
        "prime": "thread/start developer instructions",
        "remind": "user prompt guidance",
        "nudge": "turn completion guidance",
        "compact": "thread compact guidance",
    },
    "surfaces": {
        "skills": f"{os.environ['MNEMON_HOST_PROJECTION_PATH']}/skills",
        "runtime": f"{os.environ['MNEMON_HOST_PROJECTION_PATH']}/mnemon-{os.environ['MNEMON_HOST_LOOP']}",
  },
}
path.write_text(json.dumps(data, indent=2) + "\n")
PY
  write_loop_status "${projection_path}"
}

remove_host_manifest_loop() {
  [[ -f "${HOST_MANIFEST}" ]] || return 0
  MNEMON_HOST_MANIFEST="${HOST_MANIFEST}" MNEMON_HOST_LOOP="${LOOP}" python3 - <<'PY'
import json
import os
from pathlib import Path

path = Path(os.environ["MNEMON_HOST_MANIFEST"])
data = json.loads(path.read_text())
loops = data.get("loops")
if isinstance(loops, dict):
    loops.pop(os.environ["MNEMON_HOST_LOOP"], None)
if not data.get("loops"):
    path.unlink()
else:
    path.write_text(json.dumps(data, indent=2) + "\n")
PY
}

write_runtime_env() {
  local runtime_dir="$1"
  local env_name="$2"
  local loop_dir_var="$3"
  mkdir -p "${runtime_dir}"
  cat > "${runtime_dir}/env.sh" <<EOF
#!/usr/bin/env bash
export ${env_name}="${CANONICAL_LOOP_DIR}/env.sh"
export ${loop_dir_var}="${CANONICAL_LOOP_DIR}"
EOF
  chmod 0755 "${runtime_dir}/env.sh"
}

append_codex_runtime_note() {
  local skill_path="$1"
  local loop_dir_var="$2"
  local runtime_file="$3"
  cat >> "${skill_path}" <<EOF

## Codex Projection

This skill is projected by the Mnemon Codex host adapter.

- Canonical loop directory: \`${CANONICAL_LOOP_DIR}\`
- Runtime env file: \`${runtime_file}\`
- Before following the procedure, source the runtime env file when the expected
  environment variables are not already exported.
- The canonical loop directory is the location for \`GUIDE.md\`, runtime files,
  and loop state. Do not look for loop-owned \`GUIDE.md\`, \`MEMORY.md\`, usage
  logs, proposals, or skill libraries in the workspace root.
- If \`${loop_dir_var}\` is not already exported, use the canonical loop
  directory above.
EOF
}

install_memory_loop() {
  ensure_python
  ensure_mnemon_binary
  copy_common_canonical_assets
  if [[ ! -f "${CANONICAL_LOOP_DIR}/MEMORY.md" ]]; then
    install_file "${LOOP_DIR}/MEMORY.md" "${CANONICAL_LOOP_DIR}/MEMORY.md" 0644
  fi

  mkdir -p "${CONFIG_DIR}/skills/memory_get" "${CONFIG_DIR}/skills/memory_set" "${CONFIG_DIR}/mnemon-memory"
  write_runtime_env "${CONFIG_DIR}/mnemon-memory" "MNEMON_MEMORY_LOOP_ENV" "MNEMON_MEMORY_LOOP_DIR"
  install_file "${LOOP_DIR}/GUIDE.md" "${CONFIG_DIR}/mnemon-memory/GUIDE.md" 0644
  install_file "${LOOP_DIR}/skills/memory_get.md" "${CONFIG_DIR}/skills/memory_get/SKILL.md" 0644
  install_file "${LOOP_DIR}/skills/memory_set.md" "${CONFIG_DIR}/skills/memory_set/SKILL.md" 0644
  append_codex_runtime_note "${CONFIG_DIR}/skills/memory_get/SKILL.md" "MNEMON_MEMORY_LOOP_DIR" "${CONFIG_DIR}/mnemon-memory/env.sh"
  append_codex_runtime_note "${CONFIG_DIR}/skills/memory_set/SKILL.md" "MNEMON_MEMORY_LOOP_DIR" "${CONFIG_DIR}/mnemon-memory/env.sh"

  if [[ -n "${STORE_NAME}" ]]; then
    if ! mnemon store list 2>/dev/null | sed 's/^[* ]*//' | grep -qx "${STORE_NAME}"; then
      mnemon store create "${STORE_NAME}" >/dev/null
    fi
    mnemon store set "${STORE_NAME}" >/dev/null
  fi

  write_host_manifest "${CONFIG_DIR}"
  echo "Installed Mnemon memory loop for Codex."
  echo "Config:   ${CONFIG_DIR}"
  echo "State:    ${CANONICAL_LOOP_DIR}"
}

install_skill_loop() {
  ensure_python
  [[ -n "${HOST_SKILLS_DIR}" ]] || HOST_SKILLS_DIR="${CONFIG_DIR}/skills"
  copy_common_canonical_assets
  mkdir -p \
    "${CANONICAL_LOOP_DIR}/skills/active" \
    "${CANONICAL_LOOP_DIR}/skills/stale" \
    "${CANONICAL_LOOP_DIR}/skills/archived" \
    "${CANONICAL_LOOP_DIR}/proposals" \
    "${CANONICAL_LOOP_DIR}/reports" \
    "${HOST_SKILLS_DIR}/skill_observe" \
    "${HOST_SKILLS_DIR}/skill_curate" \
    "${HOST_SKILLS_DIR}/skill_author" \
    "${HOST_SKILLS_DIR}/skill_manage" \
    "${CONFIG_DIR}/mnemon-skill"
  write_runtime_env "${CONFIG_DIR}/mnemon-skill" "MNEMON_SKILL_LOOP_ENV" "MNEMON_SKILL_LOOP_DIR"
  install_file "${LOOP_DIR}/GUIDE.md" "${CONFIG_DIR}/mnemon-skill/GUIDE.md" 0644
  cat >> "${CONFIG_DIR}/mnemon-skill/env.sh" <<EOF
export MNEMON_SKILL_LOOP_LIBRARY_DIR="${CANONICAL_LOOP_DIR}/skills"
export MNEMON_SKILL_LOOP_ACTIVE_DIR="${CANONICAL_LOOP_DIR}/skills/active"
export MNEMON_SKILL_LOOP_STALE_DIR="${CANONICAL_LOOP_DIR}/skills/stale"
export MNEMON_SKILL_LOOP_ARCHIVED_DIR="${CANONICAL_LOOP_DIR}/skills/archived"
export MNEMON_SKILL_LOOP_USAGE_FILE="${CANONICAL_LOOP_DIR}/skills/.usage.jsonl"
export MNEMON_SKILL_LOOP_PROPOSALS_DIR="${CANONICAL_LOOP_DIR}/proposals"
export MNEMON_SKILL_LOOP_HOST_SKILLS_DIR="${HOST_SKILLS_DIR}"
export MNEMON_SKILL_LOOP_PROTECTED_SKILLS="${MNEMON_SKILL_LOOP_PROTECTED_SKILLS:-skill_observe,skill_curate,skill_author,skill_manage,memory_get,memory_set}"
EOF

  install_file "${LOOP_DIR}/skills/skill_observe.md" "${HOST_SKILLS_DIR}/skill_observe/SKILL.md" 0644
  install_file "${LOOP_DIR}/skills/skill_curate.md" "${HOST_SKILLS_DIR}/skill_curate/SKILL.md" 0644
  install_file "${LOOP_DIR}/skills/skill_author.md" "${HOST_SKILLS_DIR}/skill_author/SKILL.md" 0644
  install_file "${LOOP_DIR}/skills/skill_manage.md" "${HOST_SKILLS_DIR}/skill_manage/SKILL.md" 0644
  append_codex_runtime_note "${HOST_SKILLS_DIR}/skill_observe/SKILL.md" "MNEMON_SKILL_LOOP_DIR" "${CONFIG_DIR}/mnemon-skill/env.sh"
  append_codex_runtime_note "${HOST_SKILLS_DIR}/skill_curate/SKILL.md" "MNEMON_SKILL_LOOP_DIR" "${CONFIG_DIR}/mnemon-skill/env.sh"
  append_codex_runtime_note "${HOST_SKILLS_DIR}/skill_author/SKILL.md" "MNEMON_SKILL_LOOP_DIR" "${CONFIG_DIR}/mnemon-skill/env.sh"
  append_codex_runtime_note "${HOST_SKILLS_DIR}/skill_manage/SKILL.md" "MNEMON_SKILL_LOOP_DIR" "${CONFIG_DIR}/mnemon-skill/env.sh"

  write_host_manifest "${CONFIG_DIR}"
  echo "Installed Mnemon skill loop for Codex."
  echo "Config:       ${CONFIG_DIR}"
  echo "State:        ${CANONICAL_LOOP_DIR}"
  echo "Host skills:  ${HOST_SKILLS_DIR}"
}

install_eval_loop() {
  ensure_python
  [[ -n "${HOST_SKILLS_DIR}" ]] || HOST_SKILLS_DIR="${CONFIG_DIR}/skills"
  copy_common_canonical_assets
  mkdir -p \
    "${CANONICAL_LOOP_DIR}/scratch" \
    "${CANONICAL_LOOP_DIR}/candidates" \
    "${CANONICAL_LOOP_DIR}/reports" \
    "${CANONICAL_LOOP_DIR}/artifacts" \
    "${CANONICAL_LOOP_DIR}/retired" \
    "${CANONICAL_LOOP_DIR}/scenarios" \
    "${CANONICAL_LOOP_DIR}/suites" \
    "${CANONICAL_LOOP_DIR}/rubrics" \
    "${HOST_SKILLS_DIR}/eval_plan" \
    "${HOST_SKILLS_DIR}/eval_run" \
    "${HOST_SKILLS_DIR}/eval_analyze" \
    "${HOST_SKILLS_DIR}/eval_improve" \
    "${CONFIG_DIR}/mnemon-eval"

  cp -R "${LOOP_DIR}/scenarios/." "${CANONICAL_LOOP_DIR}/scenarios/"
  cp -R "${LOOP_DIR}/suites/." "${CANONICAL_LOOP_DIR}/suites/"
  cp -R "${LOOP_DIR}/rubrics/." "${CANONICAL_LOOP_DIR}/rubrics/"

  write_runtime_env "${CONFIG_DIR}/mnemon-eval" "MNEMON_EVAL_LOOP_ENV" "MNEMON_EVAL_LOOP_DIR"
  install_file "${LOOP_DIR}/GUIDE.md" "${CONFIG_DIR}/mnemon-eval/GUIDE.md" 0644
  cat >> "${CONFIG_DIR}/mnemon-eval/env.sh" <<EOF
export MNEMON_EVAL_LOOP_SCRATCH_DIR="${CANONICAL_LOOP_DIR}/scratch"
export MNEMON_EVAL_LOOP_CANDIDATES_DIR="${CANONICAL_LOOP_DIR}/candidates"
export MNEMON_EVAL_LOOP_REPORTS_DIR="${CANONICAL_LOOP_DIR}/reports"
export MNEMON_EVAL_LOOP_ARTIFACTS_DIR="${CANONICAL_LOOP_DIR}/artifacts"
export MNEMON_EVAL_LOOP_RETIRED_DIR="${CANONICAL_LOOP_DIR}/retired"
export MNEMON_EVAL_LOOP_SCENARIOS_DIR="${CANONICAL_LOOP_DIR}/scenarios"
export MNEMON_EVAL_LOOP_SUITES_DIR="${CANONICAL_LOOP_DIR}/suites"
export MNEMON_EVAL_LOOP_RUBRICS_DIR="${CANONICAL_LOOP_DIR}/rubrics"
export MNEMON_EVAL_LOOP_HOST_SKILLS_DIR="${HOST_SKILLS_DIR}"
export MNEMON_EVAL_LOOP_DEFAULT_HOST="${MNEMON_EVAL_LOOP_DEFAULT_HOST:-codex}"
export MNEMON_EVAL_LOOP_DEFAULT_SUITE="${MNEMON_EVAL_LOOP_DEFAULT_SUITE:-smoke}"
EOF

  install_file "${LOOP_DIR}/skills/eval_plan.md" "${HOST_SKILLS_DIR}/eval_plan/SKILL.md" 0644
  install_file "${LOOP_DIR}/skills/eval_run.md" "${HOST_SKILLS_DIR}/eval_run/SKILL.md" 0644
  install_file "${LOOP_DIR}/skills/eval_analyze.md" "${HOST_SKILLS_DIR}/eval_analyze/SKILL.md" 0644
  install_file "${LOOP_DIR}/skills/eval_improve.md" "${HOST_SKILLS_DIR}/eval_improve/SKILL.md" 0644
  append_codex_runtime_note "${HOST_SKILLS_DIR}/eval_plan/SKILL.md" "MNEMON_EVAL_LOOP_DIR" "${CONFIG_DIR}/mnemon-eval/env.sh"
  append_codex_runtime_note "${HOST_SKILLS_DIR}/eval_run/SKILL.md" "MNEMON_EVAL_LOOP_DIR" "${CONFIG_DIR}/mnemon-eval/env.sh"
  append_codex_runtime_note "${HOST_SKILLS_DIR}/eval_analyze/SKILL.md" "MNEMON_EVAL_LOOP_DIR" "${CONFIG_DIR}/mnemon-eval/env.sh"
  append_codex_runtime_note "${HOST_SKILLS_DIR}/eval_improve/SKILL.md" "MNEMON_EVAL_LOOP_DIR" "${CONFIG_DIR}/mnemon-eval/env.sh"

  write_host_manifest "${CONFIG_DIR}"
  echo "Installed Mnemon eval loop for Codex."
  echo "Config:       ${CONFIG_DIR}"
  echo "State:        ${CANONICAL_LOOP_DIR}"
  echo "Host skills:  ${HOST_SKILLS_DIR}"
}

status_loop() {
  echo "Codex ${LOOP}:"
  echo "  config:   ${CONFIG_DIR}"
  echo "  state:    ${CANONICAL_LOOP_DIR}"
  if [[ -f "${HOST_MANIFEST}" ]]; then
    echo "  manifest: ${HOST_MANIFEST}"
  else
    echo "  manifest: missing"
  fi
  if [[ -f "${CANONICAL_LOOP_DIR}/status.json" ]]; then
    echo "  status:   ${CANONICAL_LOOP_DIR}/status.json"
  else
    echo "  status:   missing"
  fi
  if [[ -d "${CANONICAL_LOOP_DIR}" ]]; then
    echo "  loop:   installed"
  else
    echo "  loop:   missing"
  fi
}

uninstall_memory_loop() {
  rm -rf "${CONFIG_DIR}/skills/memory_get"
  rm -rf "${CONFIG_DIR}/skills/memory_set"
  rm -rf "${CONFIG_DIR}/mnemon-memory"
  if [[ "${PURGE_MEMORY}" == "1" ]]; then
    rm -rf "${CANONICAL_LOOP_DIR}"
  else
    rm -f "${CANONICAL_LOOP_DIR}/GUIDE.md" "${CANONICAL_LOOP_DIR}/env.sh" "${CANONICAL_LOOP_DIR}/loop.json" "${CANONICAL_LOOP_DIR}/status.json"
    rmdir "${CANONICAL_LOOP_DIR}" 2>/dev/null || true
  fi
  remove_host_manifest_loop
  echo "Removed Mnemon memory loop from ${CONFIG_DIR}."
}

uninstall_skill_loop() {
  local env_path="${CONFIG_DIR}/mnemon-skill/env.sh"
  if [[ -f "${env_path}" ]]; then
    # shellcheck source=/dev/null
    source "${env_path}"
  fi
  local host_skills_dir="${MNEMON_SKILL_LOOP_HOST_SKILLS_DIR:-${HOST_SKILLS_DIR:-${CONFIG_DIR}/skills}}"
  rm -rf "${host_skills_dir}/skill_observe"
  rm -rf "${host_skills_dir}/skill_curate"
  rm -rf "${host_skills_dir}/skill_author"
  rm -rf "${host_skills_dir}/skill_manage"
  rm -rf "${CONFIG_DIR}/mnemon-skill"
  if [[ "${PURGE_LIBRARY}" == "1" ]]; then
    rm -rf "${CANONICAL_LOOP_DIR}"
  else
    rm -f "${CANONICAL_LOOP_DIR}/GUIDE.md" "${CANONICAL_LOOP_DIR}/env.sh" "${CANONICAL_LOOP_DIR}/loop.json" "${CANONICAL_LOOP_DIR}/status.json"
    rmdir "${CANONICAL_LOOP_DIR}/reports" 2>/dev/null || true
    rmdir "${CANONICAL_LOOP_DIR}/proposals" 2>/dev/null || true
    rmdir "${CANONICAL_LOOP_DIR}" 2>/dev/null || true
  fi
  remove_host_manifest_loop
  echo "Removed Mnemon skill loop from ${CONFIG_DIR}."
}

uninstall_eval_loop() {
  local env_path="${CONFIG_DIR}/mnemon-eval/env.sh"
  if [[ -f "${env_path}" ]]; then
    # shellcheck source=/dev/null
    source "${env_path}"
  fi
  local host_skills_dir="${MNEMON_EVAL_LOOP_HOST_SKILLS_DIR:-${HOST_SKILLS_DIR:-${CONFIG_DIR}/skills}}"
  rm -rf "${host_skills_dir}/eval_plan"
  rm -rf "${host_skills_dir}/eval_run"
  rm -rf "${host_skills_dir}/eval_analyze"
  rm -rf "${host_skills_dir}/eval_improve"
  rm -rf "${CONFIG_DIR}/mnemon-eval"
  rm -rf "${CANONICAL_LOOP_DIR}/scenarios"
  rm -rf "${CANONICAL_LOOP_DIR}/suites"
  rm -rf "${CANONICAL_LOOP_DIR}/rubrics"
  rm -f "${CANONICAL_LOOP_DIR}/GUIDE.md" "${CANONICAL_LOOP_DIR}/env.sh" "${CANONICAL_LOOP_DIR}/loop.json" "${CANONICAL_LOOP_DIR}/status.json"
  rmdir "${CANONICAL_LOOP_DIR}/retired" 2>/dev/null || true
  rmdir "${CANONICAL_LOOP_DIR}/artifacts" 2>/dev/null || true
  rmdir "${CANONICAL_LOOP_DIR}/reports" 2>/dev/null || true
  rmdir "${CANONICAL_LOOP_DIR}/candidates" 2>/dev/null || true
  rmdir "${CANONICAL_LOOP_DIR}/scratch" 2>/dev/null || true
  rmdir "${CANONICAL_LOOP_DIR}" 2>/dev/null || true
  remove_host_manifest_loop
  echo "Removed Mnemon eval loop from ${CONFIG_DIR}."
}

case "${ACTION}:${LOOP}" in
  install:memory) install_memory_loop ;;
  install:skill) install_skill_loop ;;
  install:eval) install_eval_loop ;;
  status:memory|status:skill|status:eval) status_loop ;;
  uninstall:memory) uninstall_memory_loop ;;
  uninstall:skill) uninstall_skill_loop ;;
  uninstall:eval) uninstall_eval_loop ;;
  *)
    echo "unsupported action/loop: ${ACTION}/${LOOP}" >&2
    exit 1
    ;;
esac
