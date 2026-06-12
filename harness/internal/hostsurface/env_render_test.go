package hostsurface

import (
	"testing"

	"github.com/mnemon-dev/mnemon/harness/internal/assets"
	"github.com/mnemon-dev/mnemon/harness/internal/manifest"
)

// The declared env (loop.json env) must render env.sh byte-identically to the retired hardcoded
// per-loop switch (PD4 env sink): structural base + each declared var, projector vars substituted,
// runtime ${VAR:-default} refs passed through.
func TestRuntimeEnvContentMatchesDeclarations(t *testing.T) {
	p := projectorCore{paths: corePaths{configDir: ".codex", mnemonDir: ".mnemon"}}

	mem, err := manifest.LoadLoop(assets.FS, "memory")
	if err != nil {
		t.Fatalf("load memory loop: %v", err)
	}
	wantMem := `#!/usr/bin/env bash
export MNEMON_MEMORY_LOOP_ENV=".mnemon/harness/memory/env.sh"
export MNEMON_MEMORY_LOOP_DIR=".mnemon/harness/memory"
export MNEMON_MEMORY_LOOP_MAX_NON_EMPTY_LINES="${MNEMON_MEMORY_LOOP_MAX_NON_EMPTY_LINES:-200}"
`
	if got := string(p.runtimeEnvContent(mem, manifest.BindingManifest{})); got != wantMem {
		t.Fatalf("memory env.sh mismatch:\n--- got ---\n%s\n--- want ---\n%s", got, wantMem)
	}

	skill, err := manifest.LoadLoop(assets.FS, "skill")
	if err != nil {
		t.Fatalf("load skill loop: %v", err)
	}
	wantSkill := `#!/usr/bin/env bash
export MNEMON_SKILL_LOOP_ENV=".mnemon/harness/skill/env.sh"
export MNEMON_SKILL_LOOP_DIR=".mnemon/harness/skill"
export MNEMON_SKILL_LOOP_LIBRARY_DIR=".mnemon/harness/skill/skills"
export MNEMON_SKILL_LOOP_ACTIVE_DIR=".mnemon/harness/skill/skills/active"
export MNEMON_SKILL_LOOP_STALE_DIR=".mnemon/harness/skill/skills/stale"
export MNEMON_SKILL_LOOP_ARCHIVED_DIR=".mnemon/harness/skill/skills/archived"
export MNEMON_SKILL_LOOP_USAGE_FILE=".mnemon/harness/skill/skills/.usage.jsonl"
export MNEMON_SKILL_LOOP_PROPOSALS_DIR=".mnemon/harness/skill/proposals"
export MNEMON_SKILL_LOOP_HOST_SKILLS_DIR=".codex/skills"
export MNEMON_SKILL_LOOP_REVIEW_MIN_EVENTS="${MNEMON_SKILL_LOOP_REVIEW_MIN_EVENTS:-20}"
export MNEMON_SKILL_LOOP_PROTECTED_SKILLS="${MNEMON_SKILL_LOOP_PROTECTED_SKILLS:-skill-observe,skill-curate,skill-author,skill-manage,memory-get,memory-set}"
`
	if got := string(p.runtimeEnvContent(skill, manifest.BindingManifest{})); got != wantSkill {
		t.Fatalf("skill env.sh mismatch:\n--- got ---\n%s\n--- want ---\n%s", got, wantSkill)
	}
}
