package manifest

import "testing"

func TestValidateEnvVar(t *testing.T) {
	// The legitimate memory/skill env values must all pass.
	good := []EnvVar{
		{"MNEMON_MEMORY_LOOP_MAX_NON_EMPTY_LINES", "${MNEMON_MEMORY_LOOP_MAX_NON_EMPTY_LINES:-200}"},
		{"MNEMON_SKILL_LOOP_LIBRARY_DIR", "${state_dir}/skills"},
		{"MNEMON_SKILL_LOOP_ACTIVE_DIR", "${state_dir}/skills/active"},
		{"MNEMON_SKILL_LOOP_USAGE_FILE", "${state_dir}/skills/.usage.jsonl"},
		{"MNEMON_SKILL_LOOP_HOST_SKILLS_DIR", "${host_skills_dir}"},
		{"MNEMON_SKILL_LOOP_REVIEW_MIN_EVENTS", "${MNEMON_SKILL_LOOP_REVIEW_MIN_EVENTS:-20}"},
		{"MNEMON_SKILL_LOOP_PROTECTED_SKILLS", "${MNEMON_SKILL_LOOP_PROTECTED_SKILLS:-skill-observe,skill-curate,skill-author,skill-manage,memory-get,memory-set}"},
	}
	for _, e := range good {
		if err := validateEnvVar(e.Name, e.Value); err != nil {
			t.Fatalf("legit env %s=%q must validate: %v", e.Name, e.Value, err)
		}
	}

	// Injection / namespace-escape attempts must all fail closed.
	bad := []struct{ name, value, why string }{
		{"PATH", "/evil", "non-namespaced name"},
		{"LD_PRELOAD", "${state_dir}/x", "non-namespaced name"},
		{"mnemon_lower", "x", "name must be uppercase namespaced"},
		{"MNEMON_X", "$(rm -rf /)", "command substitution"},
		{"MNEMON_X", "`whoami`", "backtick"},
		{"MNEMON_X", `a";rm -rf /;echo "`, "quote break-out"},
		{"MNEMON_X", "${state_dir}; rm -rf /", "semicolon"},
		{"MNEMON_X", "a\\nb", "backslash"},
		{"MNEMON_X", "${unknown_projector_var}", "unknown closed var"},
		{"MNEMON_X", "${state_dir", "unterminated expansion"},
		{"MNEMON_X", "$state_dir", "bare dollar without brace"},
		{"MNEMON_X", "x|y", "pipe"},
	}
	for _, b := range bad {
		if err := validateEnvVar(b.name, b.value); err == nil {
			t.Fatalf("unsafe env %s=%q (%s) must fail closed", b.name, b.value, b.why)
		}
	}
}
