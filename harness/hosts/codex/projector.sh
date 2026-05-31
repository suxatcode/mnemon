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

Goal loop install options:
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
if [[ "${LOOP}" != "memory" && "${LOOP}" != "skill" && "${LOOP}" != "eval" && "${LOOP}" != "goal" ]]; then
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
loop_name = os.environ["MNEMON_HOST_LOOP"]
projection_path = os.environ["MNEMON_HOST_PROJECTION_PATH"]
state_path = f"{os.environ['MNEMON_HOST_MNEMON_DIR']}/harness/{loop_name}"
surfaces = {
    "skills": f"{projection_path}/skills",
    "runtime": f"{projection_path}/mnemon-{loop_name}",
}
ownership_files = [
    f"{state_path}/GUIDE.md",
    f"{state_path}/env.sh",
    f"{state_path}/loop.json",
    f"{state_path}/status.json",
    f"{projection_path}/mnemon-{loop_name}/env.sh",
    f"{projection_path}/mnemon-{loop_name}/GUIDE.md",
]
ownership_dirs = [f"{projection_path}/mnemon-{loop_name}"]
if loop_name in {"memory", "skill", "goal", "eval"}:
    surfaces["hooks"] = f"{projection_path}/hooks/mnemon-{loop_name}"
    ownership_files.extend([
        f"{projection_path}/hooks.json",
        f"{projection_path}/hooks/mnemon-{loop_name}/prime.sh",
        f"{projection_path}/hooks/mnemon-{loop_name}/remind.sh",
        f"{projection_path}/hooks/mnemon-{loop_name}/nudge.sh",
        f"{projection_path}/hooks/mnemon-{loop_name}/compact.sh",
    ])
    ownership_dirs.append(f"{projection_path}/hooks/mnemon-{loop_name}")
data.setdefault("loops", {})[loop_name] = {
    "loop_path": state_path,
    "loop_version": loop.get("version", ""),
    "state_path": state_path,
    "intent_policy": f"{state_path}/GUIDE.md",
    "status_path": f"{state_path}/status.json",
    "projection": {
        "path": projection_path,
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
    "surfaces": surfaces,
    "ownership": {
        "files": sorted(ownership_files),
        "dirs": sorted(ownership_dirs),
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

patch_codex_hooks() {
  local loop_name="$1"
  local enable_remind="$2"
  local enable_nudge="$3"
  local enable_compact="$4"
  ensure_python
  MNEMON_CODEX_HOOKS="${CONFIG_DIR}/hooks.json" \
  MNEMON_CODEX_CONFIG_DIR="${CONFIG_DIR}" \
  MNEMON_CODEX_LOOP="${loop_name}" \
  MNEMON_CODEX_REMIND="${enable_remind}" \
  MNEMON_CODEX_NUDGE="${enable_nudge}" \
  MNEMON_CODEX_COMPACT="${enable_compact}" \
  python3 - <<'PY'
import json
import os
from pathlib import Path

hooks_path = Path(os.environ["MNEMON_CODEX_HOOKS"])
config_dir = os.environ["MNEMON_CODEX_CONFIG_DIR"]
marker = f"mnemon-{os.environ['MNEMON_CODEX_LOOP']}"
events = {"SessionStart": "prime.sh"}
if os.environ["MNEMON_CODEX_REMIND"] == "1":
    events["UserPromptSubmit"] = "remind.sh"
if os.environ["MNEMON_CODEX_NUDGE"] == "1":
    events["Stop"] = "nudge.sh"
if os.environ["MNEMON_CODEX_COMPACT"] == "1":
    events["PreCompact"] = "compact.sh"

def owned(entry):
    for hook in entry.get("hooks", []):
        command = hook.get("command", "")
        if f"/hooks/{marker}/" in command or command.startswith(f"hooks/{marker}/") or f"\\hooks\\{marker}\\" in command:
            return True
    return False

if hooks_path.exists() and hooks_path.stat().st_size:
    data = json.loads(hooks_path.read_text())
else:
    data = {}
hooks = data.setdefault("hooks", {})
for event in events:
    kept = []
    for entry in hooks.get(event, []):
        if not owned(entry):
            kept.append(entry)
    if kept:
        hooks[event] = kept
    else:
        hooks.pop(event, None)
for event, script in events.items():
    hooks.setdefault(event, []).append({
        "hooks": [{
            "type": "command",
            "command": f"{config_dir}/hooks/{marker}/{script}",
        }]
    })
hooks_path.parent.mkdir(parents=True, exist_ok=True)
hooks_path.write_text(json.dumps(data, indent=2) + "\n")
PY
}

unpatch_codex_hooks() {
  local loop_name="$1"
  ensure_python
  MNEMON_CODEX_HOOKS="${CONFIG_DIR}/hooks.json" \
  MNEMON_CODEX_LOOP="${loop_name}" \
  python3 - <<'PY'
import json
import os
from pathlib import Path

hooks_path = Path(os.environ["MNEMON_CODEX_HOOKS"])
marker = f"mnemon-{os.environ['MNEMON_CODEX_LOOP']}"
events = ("SessionStart", "UserPromptSubmit", "Stop", "PreCompact")

def owned(entry):
    for hook in entry.get("hooks", []):
        command = hook.get("command", "")
        if f"/hooks/{marker}/" in command or command.startswith(f"hooks/{marker}/") or f"\\hooks\\{marker}\\" in command:
            return True
    return False

if not hooks_path.exists():
    raise SystemExit(0)
if hooks_path.stat().st_size:
    data = json.loads(hooks_path.read_text())
else:
    data = {}
hooks = data.setdefault("hooks", {})
for event in events:
    kept = []
    for entry in hooks.get(event, []):
        if not owned(entry):
            kept.append(entry)
    if kept:
        hooks[event] = kept
    else:
        hooks.pop(event, None)
hooks_path.write_text(json.dumps(data, indent=2) + "\n")
PY
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
  and loop state. Do not look for loop-owned state in the workspace root.
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

  mkdir -p "${CONFIG_DIR}/skills/memory-get" "${CONFIG_DIR}/skills/memory-set" "${CONFIG_DIR}/mnemon-memory" "${CONFIG_DIR}/hooks/mnemon-memory"
  write_runtime_env "${CONFIG_DIR}/mnemon-memory" "MNEMON_MEMORY_LOOP_ENV" "MNEMON_MEMORY_LOOP_DIR"
  cat >> "${CONFIG_DIR}/mnemon-memory/env.sh" <<EOF
export MNEMON_MEMORY_LOOP_MAX_NON_EMPTY_LINES="\${MNEMON_MEMORY_LOOP_MAX_NON_EMPTY_LINES:-200}"
EOF
  install_file "${LOOP_DIR}/GUIDE.md" "${CONFIG_DIR}/mnemon-memory/GUIDE.md" 0644
  install_file "${LOOP_DIR}/skills/memory-get/SKILL.md" "${CONFIG_DIR}/skills/memory-get/SKILL.md" 0644
  install_file "${LOOP_DIR}/skills/memory-set/SKILL.md" "${CONFIG_DIR}/skills/memory-set/SKILL.md" 0644
  append_codex_runtime_note "${CONFIG_DIR}/skills/memory-get/SKILL.md" "MNEMON_MEMORY_LOOP_DIR" "${CONFIG_DIR}/mnemon-memory/env.sh"
  append_codex_runtime_note "${CONFIG_DIR}/skills/memory-set/SKILL.md" "MNEMON_MEMORY_LOOP_DIR" "${CONFIG_DIR}/mnemon-memory/env.sh"
  install_file "${SCRIPT_DIR}/memory/hooks/prime.sh" "${CONFIG_DIR}/hooks/mnemon-memory/prime.sh" 0755
  install_file "${SCRIPT_DIR}/memory/hooks/remind.sh" "${CONFIG_DIR}/hooks/mnemon-memory/remind.sh" 0755
  install_file "${SCRIPT_DIR}/memory/hooks/nudge.sh" "${CONFIG_DIR}/hooks/mnemon-memory/nudge.sh" 0755
  install_file "${SCRIPT_DIR}/memory/hooks/compact.sh" "${CONFIG_DIR}/hooks/mnemon-memory/compact.sh" 0755
  patch_codex_hooks memory 1 1 1

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
    "${HOST_SKILLS_DIR}/skill-observe" \
    "${HOST_SKILLS_DIR}/skill-curate" \
    "${HOST_SKILLS_DIR}/skill-author" \
    "${HOST_SKILLS_DIR}/skill-manage" \
    "${CONFIG_DIR}/mnemon-skill" \
    "${CONFIG_DIR}/hooks/mnemon-skill"
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
export MNEMON_SKILL_LOOP_REVIEW_MIN_EVENTS="\${MNEMON_SKILL_LOOP_REVIEW_MIN_EVENTS:-20}"
export MNEMON_SKILL_LOOP_PROTECTED_SKILLS="${MNEMON_SKILL_LOOP_PROTECTED_SKILLS:-skill-observe,skill-curate,skill-author,skill-manage,memory-get,memory-set,mnemon-goal}"
EOF

  install_file "${LOOP_DIR}/skills/skill-observe/SKILL.md" "${HOST_SKILLS_DIR}/skill-observe/SKILL.md" 0644
  install_file "${LOOP_DIR}/skills/skill-curate/SKILL.md" "${HOST_SKILLS_DIR}/skill-curate/SKILL.md" 0644
  install_file "${LOOP_DIR}/skills/skill-author/SKILL.md" "${HOST_SKILLS_DIR}/skill-author/SKILL.md" 0644
  install_file "${LOOP_DIR}/skills/skill-manage/SKILL.md" "${HOST_SKILLS_DIR}/skill-manage/SKILL.md" 0644
  append_codex_runtime_note "${HOST_SKILLS_DIR}/skill-observe/SKILL.md" "MNEMON_SKILL_LOOP_DIR" "${CONFIG_DIR}/mnemon-skill/env.sh"
  append_codex_runtime_note "${HOST_SKILLS_DIR}/skill-curate/SKILL.md" "MNEMON_SKILL_LOOP_DIR" "${CONFIG_DIR}/mnemon-skill/env.sh"
  append_codex_runtime_note "${HOST_SKILLS_DIR}/skill-author/SKILL.md" "MNEMON_SKILL_LOOP_DIR" "${CONFIG_DIR}/mnemon-skill/env.sh"
  append_codex_runtime_note "${HOST_SKILLS_DIR}/skill-manage/SKILL.md" "MNEMON_SKILL_LOOP_DIR" "${CONFIG_DIR}/mnemon-skill/env.sh"
  install_file "${SCRIPT_DIR}/skill/hooks/prime.sh" "${CONFIG_DIR}/hooks/mnemon-skill/prime.sh" 0755
  install_file "${SCRIPT_DIR}/skill/hooks/remind.sh" "${CONFIG_DIR}/hooks/mnemon-skill/remind.sh" 0755
  install_file "${SCRIPT_DIR}/skill/hooks/nudge.sh" "${CONFIG_DIR}/hooks/mnemon-skill/nudge.sh" 0755
  install_file "${SCRIPT_DIR}/skill/hooks/compact.sh" "${CONFIG_DIR}/hooks/mnemon-skill/compact.sh" 0755
  patch_codex_hooks skill 0 1 1

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
    "${HOST_SKILLS_DIR}/eval-plan" \
    "${HOST_SKILLS_DIR}/eval-run" \
    "${HOST_SKILLS_DIR}/eval-analyze" \
    "${HOST_SKILLS_DIR}/eval-improve" \
    "${CONFIG_DIR}/mnemon-eval" \
    "${CONFIG_DIR}/hooks/mnemon-eval"

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

  install_file "${LOOP_DIR}/skills/eval-plan/SKILL.md" "${HOST_SKILLS_DIR}/eval-plan/SKILL.md" 0644
  install_file "${LOOP_DIR}/skills/eval-run/SKILL.md" "${HOST_SKILLS_DIR}/eval-run/SKILL.md" 0644
  install_file "${LOOP_DIR}/skills/eval-analyze/SKILL.md" "${HOST_SKILLS_DIR}/eval-analyze/SKILL.md" 0644
  install_file "${LOOP_DIR}/skills/eval-improve/SKILL.md" "${HOST_SKILLS_DIR}/eval-improve/SKILL.md" 0644
  append_codex_runtime_note "${HOST_SKILLS_DIR}/eval-plan/SKILL.md" "MNEMON_EVAL_LOOP_DIR" "${CONFIG_DIR}/mnemon-eval/env.sh"
  append_codex_runtime_note "${HOST_SKILLS_DIR}/eval-run/SKILL.md" "MNEMON_EVAL_LOOP_DIR" "${CONFIG_DIR}/mnemon-eval/env.sh"
  append_codex_runtime_note "${HOST_SKILLS_DIR}/eval-analyze/SKILL.md" "MNEMON_EVAL_LOOP_DIR" "${CONFIG_DIR}/mnemon-eval/env.sh"
  append_codex_runtime_note "${HOST_SKILLS_DIR}/eval-improve/SKILL.md" "MNEMON_EVAL_LOOP_DIR" "${CONFIG_DIR}/mnemon-eval/env.sh"
  install_file "${SCRIPT_DIR}/eval/hooks/prime.sh" "${CONFIG_DIR}/hooks/mnemon-eval/prime.sh" 0755
  install_file "${SCRIPT_DIR}/eval/hooks/remind.sh" "${CONFIG_DIR}/hooks/mnemon-eval/remind.sh" 0755
  install_file "${SCRIPT_DIR}/eval/hooks/nudge.sh" "${CONFIG_DIR}/hooks/mnemon-eval/nudge.sh" 0755
  install_file "${SCRIPT_DIR}/eval/hooks/compact.sh" "${CONFIG_DIR}/hooks/mnemon-eval/compact.sh" 0755
  patch_codex_hooks eval 1 1 1

  write_host_manifest "${CONFIG_DIR}"
  echo "Installed Mnemon eval loop for Codex."
  echo "Config:       ${CONFIG_DIR}"
  echo "State:        ${CANONICAL_LOOP_DIR}"
  echo "Host skills:  ${HOST_SKILLS_DIR}"
}

install_goal_loop() {
  ensure_python
  [[ -n "${HOST_SKILLS_DIR}" ]] || HOST_SKILLS_DIR="${CONFIG_DIR}/skills"
  copy_common_canonical_assets
  mkdir -p \
    "${MNEMON_DIR}/harness/goals" \
    "${MNEMON_DIR}/harness/status/goals" \
    "${HOST_SKILLS_DIR}/mnemon-goal" \
    "${CONFIG_DIR}/mnemon-goal" \
    "${CONFIG_DIR}/hooks/mnemon-goal"

  write_runtime_env "${CONFIG_DIR}/mnemon-goal" "MNEMON_GOAL_LOOP_ENV" "MNEMON_GOAL_LOOP_DIR"
  cat >> "${CONFIG_DIR}/mnemon-goal/env.sh" <<EOF
export MNEMON_GOAL_LOOP_ROOT="$(pwd)"
export MNEMON_GOAL_LOOP_GOALS_DIR="${MNEMON_DIR}/harness/goals"
export MNEMON_GOAL_LOOP_STATUS_DIR="${MNEMON_DIR}/harness/status/goals"
export MNEMON_GOAL_LOOP_HOST_SKILLS_DIR="${HOST_SKILLS_DIR}"
EOF

  install_file "${LOOP_DIR}/GUIDE.md" "${CONFIG_DIR}/mnemon-goal/GUIDE.md" 0644
  install_file "${LOOP_DIR}/skills/mnemon-goal/SKILL.md" "${HOST_SKILLS_DIR}/mnemon-goal/SKILL.md" 0644
  append_codex_runtime_note "${HOST_SKILLS_DIR}/mnemon-goal/SKILL.md" "MNEMON_GOAL_LOOP_DIR" "${CONFIG_DIR}/mnemon-goal/env.sh"
  install_file "${SCRIPT_DIR}/goal/hooks/prime.sh" "${CONFIG_DIR}/hooks/mnemon-goal/prime.sh" 0755
  install_file "${SCRIPT_DIR}/goal/hooks/remind.sh" "${CONFIG_DIR}/hooks/mnemon-goal/remind.sh" 0755
  install_file "${SCRIPT_DIR}/goal/hooks/nudge.sh" "${CONFIG_DIR}/hooks/mnemon-goal/nudge.sh" 0755
  install_file "${SCRIPT_DIR}/goal/hooks/compact.sh" "${CONFIG_DIR}/hooks/mnemon-goal/compact.sh" 0755
  patch_codex_hooks goal 1 1 1

  write_host_manifest "${CONFIG_DIR}"
  echo "Installed Mnemon goal loop for Codex."
  echo "Config:       ${CONFIG_DIR}"
  echo "State:        ${CANONICAL_LOOP_DIR}"
  echo "Goals:        ${MNEMON_DIR}/harness/goals"
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
  unpatch_codex_hooks memory
  rm -rf "${CONFIG_DIR}/hooks/mnemon-memory"
  rm -rf "${CONFIG_DIR}/skills/memory-get"
  rm -rf "${CONFIG_DIR}/skills/memory-set"
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
  unpatch_codex_hooks skill
  if [[ -d "${host_skills_dir}" ]]; then
    while IFS= read -r marker; do
      rm -rf "$(dirname "${marker}")"
    done < <(find "${host_skills_dir}" -mindepth 2 -maxdepth 2 -name .mnemon-skill-generated -print 2>/dev/null)
  fi
  rm -rf "${host_skills_dir}/skill-observe"
  rm -rf "${host_skills_dir}/skill-curate"
  rm -rf "${host_skills_dir}/skill-author"
  rm -rf "${host_skills_dir}/skill-manage"
  rm -rf "${CONFIG_DIR}/hooks/mnemon-skill"
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
  unpatch_codex_hooks eval
  rm -rf "${host_skills_dir}/eval-plan"
  rm -rf "${host_skills_dir}/eval-run"
  rm -rf "${host_skills_dir}/eval-analyze"
  rm -rf "${host_skills_dir}/eval-improve"
  rm -rf "${CONFIG_DIR}/hooks/mnemon-eval"
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

uninstall_goal_loop() {
  local env_path="${CONFIG_DIR}/mnemon-goal/env.sh"
  if [[ -f "${env_path}" ]]; then
    # shellcheck source=/dev/null
    source "${env_path}"
  fi
  local host_skills_dir="${MNEMON_GOAL_LOOP_HOST_SKILLS_DIR:-${HOST_SKILLS_DIR:-${CONFIG_DIR}/skills}}"
  unpatch_codex_hooks goal
  rm -rf "${host_skills_dir}/mnemon-goal"
  rm -rf "${CONFIG_DIR}/hooks/mnemon-goal"
  rm -rf "${CONFIG_DIR}/mnemon-goal"
  rm -f "${CANONICAL_LOOP_DIR}/GUIDE.md" "${CANONICAL_LOOP_DIR}/env.sh" "${CANONICAL_LOOP_DIR}/loop.json" "${CANONICAL_LOOP_DIR}/status.json"
  rmdir "${CANONICAL_LOOP_DIR}" 2>/dev/null || true
  remove_host_manifest_loop
  echo "Removed Mnemon goal loop from ${CONFIG_DIR}."
}

case "${ACTION}:${LOOP}" in
  install:memory) install_memory_loop ;;
  install:skill) install_skill_loop ;;
  install:eval) install_eval_loop ;;
  install:goal) install_goal_loop ;;
  status:memory|status:skill|status:eval|status:goal) status_loop ;;
  uninstall:memory) uninstall_memory_loop ;;
  uninstall:skill) uninstall_skill_loop ;;
  uninstall:eval) uninstall_eval_loop ;;
  uninstall:goal) uninstall_goal_loop ;;
  *)
    echo "unsupported action/loop: ${ACTION}/${LOOP}" >&2
    exit 1
    ;;
esac
