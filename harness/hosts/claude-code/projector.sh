#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'USAGE'
Project Mnemon harness loops into Claude Code.

Usage:
  projector.sh install   --loop LOOP [options]
  projector.sh status    --loop LOOP [options]
  projector.sh uninstall --loop LOOP [options]

Common options:
  --global
  --config-dir DIR

Memory loop install options:
  --store NAME
  --no-remind
  --no-nudge
  --no-compact

Skill loop install options:
  --host-skills-dir DIR
  --with-remind
  --no-nudge
  --no-compact

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
CONFIG_DIR=".claude"
CONFIG_DIR_EXPLICIT=0
GLOBAL=0
STORE_NAME=""
HOST_SKILLS_DIR=""
ENABLE_REMIND=""
ENABLE_NUDGE=1
ENABLE_COMPACT=1
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
      CONFIG_DIR="${HOME}/.claude"
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
    --with-remind)
      ENABLE_REMIND=1
      shift
      ;;
    --no-remind)
      ENABLE_REMIND=0
      shift
      ;;
    --no-nudge)
      ENABLE_NUDGE=0
      shift
      ;;
    --no-compact)
      ENABLE_COMPACT=0
      shift
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
if [[ "${LOOP}" != "memory" && "${LOOP}" != "skill" ]]; then
  echo "unsupported loop for Claude Code: ${LOOP}" >&2
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
HOST_MANIFEST_DIR="${MNEMON_DIR}/hosts/claude-code"
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
    echo "python3 is required to update Claude Code settings.json" >&2
    exit 1
  fi
}

ensure_mnemon_binary() {
  if ! command -v mnemon >/dev/null 2>&1; then
    echo "mnemon binary not found in PATH. Install it first, for example:" >&2
    echo "  brew install mnemon-dev/tap/mnemon" >&2
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
  MNEMON_HOST="claude-code" \
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
    data = {"schema_version": 2, "host": "claude-code", "loops": {}}

data["schema_version"] = 2
data["host"] = "claude-code"
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
        "prime": "SessionStart",
        "remind": "UserPromptSubmit",
        "nudge": "Stop",
        "compact": "PreCompact",
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

write_memory_projection_env() {
  mkdir -p "${CONFIG_DIR}/mnemon-memory"
  cat > "${CONFIG_DIR}/mnemon-memory/env.sh" <<EOF
#!/usr/bin/env bash
export MNEMON_MEMORY_LOOP_ENV="${CANONICAL_LOOP_DIR}/env.sh"
export MNEMON_MEMORY_LOOP_DIR="${CANONICAL_LOOP_DIR}"
export MNEMON_MEMORY_LOOP_MAX_NON_EMPTY_LINES="\${MNEMON_MEMORY_LOOP_MAX_NON_EMPTY_LINES:-200}"
EOF
  chmod 0755 "${CONFIG_DIR}/mnemon-memory/env.sh"
}

write_skill_projection_env() {
  mkdir -p "${CONFIG_DIR}/mnemon-skill"
  local host_skills_dir="${HOST_SKILLS_DIR:-${CONFIG_DIR}/skills}"
  cat > "${CONFIG_DIR}/mnemon-skill/env.sh" <<EOF
#!/usr/bin/env bash
export MNEMON_SKILL_LOOP_ENV="${CANONICAL_LOOP_DIR}/env.sh"
export MNEMON_SKILL_LOOP_DIR="${CANONICAL_LOOP_DIR}"
export MNEMON_SKILL_LOOP_LIBRARY_DIR="${CANONICAL_LOOP_DIR}/skills"
export MNEMON_SKILL_LOOP_ACTIVE_DIR="${CANONICAL_LOOP_DIR}/skills/active"
export MNEMON_SKILL_LOOP_STALE_DIR="${CANONICAL_LOOP_DIR}/skills/stale"
export MNEMON_SKILL_LOOP_ARCHIVED_DIR="${CANONICAL_LOOP_DIR}/skills/archived"
export MNEMON_SKILL_LOOP_USAGE_FILE="${CANONICAL_LOOP_DIR}/skills/.usage.jsonl"
export MNEMON_SKILL_LOOP_PROPOSALS_DIR="${CANONICAL_LOOP_DIR}/proposals"
export MNEMON_SKILL_LOOP_HOST_SKILLS_DIR="${host_skills_dir}"
export MNEMON_SKILL_LOOP_REVIEW_MIN_EVENTS="\${MNEMON_SKILL_LOOP_REVIEW_MIN_EVENTS:-20}"
export MNEMON_SKILL_LOOP_PROTECTED_SKILLS="\${MNEMON_SKILL_LOOP_PROTECTED_SKILLS:-skill_observe,skill_curate,skill_author,skill_manage,memory_get,memory_set}"
EOF
  chmod 0755 "${CONFIG_DIR}/mnemon-skill/env.sh"
}

settings_script() {
  printf '%s/%s/scripts/update_settings.py\n' "${SCRIPT_DIR}" "${LOOP}"
}

install_memory_loop() {
  ensure_python
  ensure_mnemon_binary
  [[ -n "${ENABLE_REMIND}" ]] || ENABLE_REMIND=1

  copy_common_canonical_assets
  if [[ ! -f "${CANONICAL_LOOP_DIR}/MEMORY.md" ]]; then
    install_file "${LOOP_DIR}/MEMORY.md" "${CANONICAL_LOOP_DIR}/MEMORY.md" 0644
  fi
  mkdir -p "${CONFIG_DIR}/skills/memory_get" "${CONFIG_DIR}/skills/memory_set" "${CONFIG_DIR}/agents" "${CONFIG_DIR}/hooks/mnemon-memory"
  write_memory_projection_env

  install_file "${LOOP_DIR}/skills/memory_get.md" "${CONFIG_DIR}/skills/memory_get/SKILL.md" 0644
  install_file "${LOOP_DIR}/skills/memory_set.md" "${CONFIG_DIR}/skills/memory_set/SKILL.md" 0644
  install_file "${LOOP_DIR}/subagents/dreaming.md" "${CONFIG_DIR}/agents/mnemon-dreaming.md" 0644

  install_file "${SCRIPT_DIR}/memory/hooks/prime.sh" "${CONFIG_DIR}/hooks/mnemon-memory/prime.sh" 0755
  install_file "${SCRIPT_DIR}/memory/hooks/remind.sh" "${CONFIG_DIR}/hooks/mnemon-memory/remind.sh" 0755
  install_file "${SCRIPT_DIR}/memory/hooks/nudge.sh" "${CONFIG_DIR}/hooks/mnemon-memory/nudge.sh" 0755
  install_file "${SCRIPT_DIR}/memory/hooks/compact.sh" "${CONFIG_DIR}/hooks/mnemon-memory/compact.sh" 0755

  python3 "$(settings_script)" install --config-dir "${CONFIG_DIR}" --remind "${ENABLE_REMIND}" --nudge "${ENABLE_NUDGE}" --compact "${ENABLE_COMPACT}"

  if [[ -n "${STORE_NAME}" ]]; then
    if ! mnemon store list 2>/dev/null | sed 's/^[* ]*//' | grep -qx "${STORE_NAME}"; then
      mnemon store create "${STORE_NAME}" >/dev/null
    fi
    mnemon store set "${STORE_NAME}" >/dev/null
  fi

  write_host_manifest "${CONFIG_DIR}"

  echo "Installed Mnemon memory loop for Claude Code."
  echo "Config:   ${CONFIG_DIR}"
  echo "State:    ${CANONICAL_LOOP_DIR}"
  echo "Memory:   ${CANONICAL_LOOP_DIR}/MEMORY.md"
}

install_skill_loop() {
  ensure_python
  [[ -n "${ENABLE_REMIND}" ]] || ENABLE_REMIND=0
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
    "${CONFIG_DIR}/agents" \
    "${CONFIG_DIR}/hooks/mnemon-skill"
  write_skill_projection_env

  install_file "${LOOP_DIR}/skills/skill_observe.md" "${HOST_SKILLS_DIR}/skill_observe/SKILL.md" 0644
  install_file "${LOOP_DIR}/skills/skill_curate.md" "${HOST_SKILLS_DIR}/skill_curate/SKILL.md" 0644
  install_file "${LOOP_DIR}/skills/skill_author.md" "${HOST_SKILLS_DIR}/skill_author/SKILL.md" 0644
  install_file "${LOOP_DIR}/skills/skill_manage.md" "${HOST_SKILLS_DIR}/skill_manage/SKILL.md" 0644
  install_file "${LOOP_DIR}/subagents/curator.md" "${CONFIG_DIR}/agents/mnemon-skill-curator.md" 0644

  install_file "${SCRIPT_DIR}/skill/hooks/prime.sh" "${CONFIG_DIR}/hooks/mnemon-skill/prime.sh" 0755
  install_file "${SCRIPT_DIR}/skill/hooks/remind.sh" "${CONFIG_DIR}/hooks/mnemon-skill/remind.sh" 0755
  install_file "${SCRIPT_DIR}/skill/hooks/nudge.sh" "${CONFIG_DIR}/hooks/mnemon-skill/nudge.sh" 0755
  install_file "${SCRIPT_DIR}/skill/hooks/compact.sh" "${CONFIG_DIR}/hooks/mnemon-skill/compact.sh" 0755

  python3 "$(settings_script)" install --config-dir "${CONFIG_DIR}" --remind "${ENABLE_REMIND}" --nudge "${ENABLE_NUDGE}" --compact "${ENABLE_COMPACT}"
  write_host_manifest "${CONFIG_DIR}"

  echo "Installed Mnemon skill loop for Claude Code."
  echo "Config:       ${CONFIG_DIR}"
  echo "State:        ${CANONICAL_LOOP_DIR}"
  echo "Host skills:  ${HOST_SKILLS_DIR}"
}

status_loop() {
  echo "Claude Code ${LOOP}:"
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
  ensure_python
  python3 "$(settings_script)" uninstall --config-dir "${CONFIG_DIR}"
  rm -rf "${CONFIG_DIR}/hooks/mnemon-memory"
  rm -rf "${CONFIG_DIR}/skills/memory_get"
  rm -rf "${CONFIG_DIR}/skills/memory_set"
  rm -f "${CONFIG_DIR}/agents/mnemon-dreaming.md"
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
  ensure_python
  local env_path="${CONFIG_DIR}/mnemon-skill/env.sh"
  if [[ -f "${env_path}" ]]; then
    # shellcheck source=/dev/null
    source "${env_path}"
  fi
  local host_skills_dir="${MNEMON_SKILL_LOOP_HOST_SKILLS_DIR:-${HOST_SKILLS_DIR:-${CONFIG_DIR}/skills}}"

  python3 "$(settings_script)" uninstall --config-dir "${CONFIG_DIR}"
  if [[ -d "${host_skills_dir}" ]]; then
    while IFS= read -r marker; do
      rm -rf "$(dirname "${marker}")"
    done < <(find "${host_skills_dir}" -mindepth 2 -maxdepth 2 -name .mnemon-skill-generated -print 2>/dev/null)
  fi
  rm -rf "${CONFIG_DIR}/hooks/mnemon-skill"
  rm -rf "${host_skills_dir}/skill_observe"
  rm -rf "${host_skills_dir}/skill_curate"
  rm -rf "${host_skills_dir}/skill_author"
  rm -rf "${host_skills_dir}/skill_manage"
  rm -f "${CONFIG_DIR}/agents/mnemon-skill-curator.md"
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

case "${ACTION}:${LOOP}" in
  install:memory) install_memory_loop ;;
  install:skill) install_skill_loop ;;
  status:memory|status:skill) status_loop ;;
  uninstall:memory) uninstall_memory_loop ;;
  uninstall:skill) uninstall_skill_loop ;;
  *)
    echo "unsupported action/loop: ${ACTION}/${LOOP}" >&2
    exit 1
    ;;
esac
