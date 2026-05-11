#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'USAGE'
Install the Mnemon memory loop harness into Claude Code.

Usage:
  install.sh [--global] [--config-dir DIR] [--store NAME]
             [--no-remind] [--no-nudge] [--no-compact]

Defaults:
  --config-dir .claude
  installs all four hooks: Prime, Remind, Nudge, Compact

Examples:
  bash harness/memory-loop/setup/claude-code/install.sh
  bash harness/memory-loop/setup/claude-code/install.sh --global
  bash harness/memory-loop/setup/claude-code/install.sh --store mnemon
USAGE
}

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
HARNESS_DIR="$(cd "${SCRIPT_DIR}/../.." && pwd)"

CONFIG_DIR=".claude"
STORE_NAME=""
ENABLE_REMIND=1
ENABLE_NUDGE=1
ENABLE_COMPACT=1

while [[ $# -gt 0 ]]; do
  case "$1" in
    --global)
      CONFIG_DIR="${HOME}/.claude"
      shift
      ;;
    --config-dir)
      CONFIG_DIR="${2:?missing value for --config-dir}"
      shift 2
      ;;
    --store)
      STORE_NAME="${2:?missing value for --store}"
      shift 2
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

if ! command -v python3 >/dev/null 2>&1; then
  echo "python3 is required to update Claude Code settings.json" >&2
  exit 1
fi

if ! command -v mnemon >/dev/null 2>&1; then
  echo "mnemon binary not found in PATH. Install it first, for example:" >&2
  echo "  brew install mnemon-dev/tap/mnemon" >&2
  exit 1
fi

mkdir -p \
  "${CONFIG_DIR}/mnemon-memory-loop" \
  "${CONFIG_DIR}/skills/memory_get" \
  "${CONFIG_DIR}/skills/memory_set" \
  "${CONFIG_DIR}/agents" \
  "${CONFIG_DIR}/hooks/mnemon-memory-loop"

install_file() {
  local src="$1"
  local dst="$2"
  local mode="$3"
  cp "$src" "$dst"
  chmod "$mode" "$dst"
}

install_file "${HARNESS_DIR}/GUIDE.md" "${CONFIG_DIR}/mnemon-memory-loop/GUIDE.md" 0644
if [[ ! -f "${CONFIG_DIR}/mnemon-memory-loop/MEMORY.md" ]]; then
  install_file "${HARNESS_DIR}/MEMORY.md" "${CONFIG_DIR}/mnemon-memory-loop/MEMORY.md" 0644
fi

install_file "${HARNESS_DIR}/skills/memory_get.md" "${CONFIG_DIR}/skills/memory_get/SKILL.md" 0644
install_file "${HARNESS_DIR}/skills/memory_set.md" "${CONFIG_DIR}/skills/memory_set/SKILL.md" 0644
install_file "${HARNESS_DIR}/subagents/dreaming.md" "${CONFIG_DIR}/agents/mnemon-dreaming.md" 0644

install_file "${SCRIPT_DIR}/hooks/prime.sh" "${CONFIG_DIR}/hooks/mnemon-memory-loop/prime.sh" 0755
install_file "${SCRIPT_DIR}/hooks/remind.sh" "${CONFIG_DIR}/hooks/mnemon-memory-loop/remind.sh" 0755
install_file "${SCRIPT_DIR}/hooks/nudge.sh" "${CONFIG_DIR}/hooks/mnemon-memory-loop/nudge.sh" 0755
install_file "${SCRIPT_DIR}/hooks/compact.sh" "${CONFIG_DIR}/hooks/mnemon-memory-loop/compact.sh" 0755

cat > "${CONFIG_DIR}/hooks/mnemon-memory-loop/env.sh" <<EOF
#!/usr/bin/env bash
export MNEMON_MEMORY_LOOP_DIR="${CONFIG_DIR}/mnemon-memory-loop"
EOF
chmod 0755 "${CONFIG_DIR}/hooks/mnemon-memory-loop/env.sh"

python3 "${SCRIPT_DIR}/scripts/update_settings.py" install \
  --config-dir "${CONFIG_DIR}" \
  --remind "${ENABLE_REMIND}" \
  --nudge "${ENABLE_NUDGE}" \
  --compact "${ENABLE_COMPACT}"

if [[ -n "${STORE_NAME}" ]]; then
  if ! mnemon store list 2>/dev/null | sed 's/^[* ]*//' | grep -qx "${STORE_NAME}"; then
    mnemon store create "${STORE_NAME}" >/dev/null
  fi
  mnemon store set "${STORE_NAME}" >/dev/null
fi

HOOK_SUMMARY="prime"
if [[ "${ENABLE_REMIND}" == "1" ]]; then
  HOOK_SUMMARY="${HOOK_SUMMARY}, remind"
fi
if [[ "${ENABLE_NUDGE}" == "1" ]]; then
  HOOK_SUMMARY="${HOOK_SUMMARY}, nudge"
fi
if [[ "${ENABLE_COMPACT}" == "1" ]]; then
  HOOK_SUMMARY="${HOOK_SUMMARY}, compact"
fi

cat <<EOF
Installed Mnemon memory loop for Claude Code.

Config:  ${CONFIG_DIR}
Memory:  ${CONFIG_DIR}/mnemon-memory-loop/MEMORY.md
Guide:   ${CONFIG_DIR}/mnemon-memory-loop/GUIDE.md
Env:     MNEMON_MEMORY_LOOP_DIR=${CONFIG_DIR}/mnemon-memory-loop
Skills:  ${CONFIG_DIR}/skills/memory_get/SKILL.md
         ${CONFIG_DIR}/skills/memory_set/SKILL.md
Agent:   ${CONFIG_DIR}/agents/mnemon-dreaming.md
Hooks:   ${HOOK_SUMMARY}

Restart Claude Code to load new skills and subagents.
EOF
