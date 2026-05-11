#!/usr/bin/env bash
set -euo pipefail

cat <<'EOF'
[mnemon-memory-loop] Remind

Before planning, apply GUIDE.md:
- If prior memory could change this task, load the memory_get skill and run a focused Mnemon recall.
- If the task is trivial, local, or already fully covered by visible context, skip recall.
- Do not write memory from this hook.
EOF
