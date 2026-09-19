package hostsurface

import (
	"testing"

	"github.com/suxatcode/mnemon/harness/internal/assets"
	"github.com/suxatcode/mnemon/harness/internal/manifest"
)

// PD4 hook-options sink: codex applies the declared per-loop intent directly; claude takes Remind
// from the declaration (operator --remind overrides) and keeps operator-flag nudge/compact.
func TestHookOptionsFromDeclarations(t *testing.T) {
	mem, err := manifest.LoadLoop(assets.FS, "memory")
	if err != nil {
		t.Fatalf("load memory: %v", err)
	}
	skill, err := manifest.LoadLoop(assets.FS, "skill")
	if err != nil {
		t.Fatalf("load skill: %v", err)
	}
	if !mem.HasHooks() || !skill.HasHooks() {
		t.Fatal("memory and skill must declare hooks")
	}

	// codex: declaration applied verbatim.
	if got := (codexProjector{}).hookOptions(mem); got != (codexHookOptions{Remind: true, Nudge: true, Compact: true}) {
		t.Fatalf("codex memory hookOptions = %+v", got)
	}
	if got := (codexProjector{}).hookOptions(skill); got != (codexHookOptions{Remind: false, Nudge: true, Compact: true}) {
		t.Fatalf("codex skill hookOptions = %+v (Remind must be false)", got)
	}

	// claude: Remind from the declaration by default.
	if got := (claudeProjector{}).hookOptions(mem); !got.Remind {
		t.Fatalf("claude memory Remind must default true (from declaration), got %+v", got)
	}
	if got := (claudeProjector{}).hookOptions(skill); got.Remind {
		t.Fatalf("claude skill Remind must default false (from declaration), got %+v", got)
	}
	// claude: operator --remind override wins over the declaration.
	overridden := claudeProjector{hostOptions: claudeHostOptions{remindSet: true, remind: false}}
	if got := overridden.hookOptions(mem); got.Remind {
		t.Fatalf("operator --no-remind must override the declaration, got %+v", got)
	}
}
