package app

import (
	"testing"

	"github.com/mnemon-dev/mnemon/harness/internal/channel"
	"github.com/mnemon-dev/mnemon/harness/internal/contract"
)

// A host (or an old bindings.json) may still allow only the legacy underscore observed type while the
// canonical type has converged to the dotted form. The rule-build gate must be alias-aware, so the
// loop is not silently stranded with zero rules.
func TestLocalMemoryRulesAdmitsLegacyObservedTypeAlias(t *testing.T) {
	ref := contract.ResourceRef{Kind: "memory", ID: "project"}
	b := channel.HostAgentBinding("codex@project", "http://127.0.0.1:8787", []contract.ResourceRef{ref})
	b.AllowedObservedTypes = []string{"memory.write_candidate_observed"} // legacy underscore only

	rules := LocalMemoryRules([]channel.ChannelBinding{b})
	if len(rules) != 1 {
		t.Fatalf("a binding allowing only the legacy observed-type alias must still yield 1 memory rule; got %d", len(rules))
	}
}
