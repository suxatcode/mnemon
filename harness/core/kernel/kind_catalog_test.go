package kernel

import (
	"testing"

	"github.com/mnemon-dev/mnemon/harness/core/contract"
)

func TestKindCatalogMatchesSchemaGuard(t *testing.T) {
	g := DefaultSchemaGuard()
	for k := range g.Required {
		if !contract.KindCatalog[k] {
			t.Fatalf("schema kind %q missing from contract.KindCatalog", k)
		}
	}
	for k := range contract.KindCatalog {
		if _, ok := g.Required[k]; !ok {
			t.Fatalf("catalog kind %q has no schema guard entry", k)
		}
	}
}

// review finding #2: the kernel itself is the last line — a direct Apply with an unknown kind must be
// rejected, even if AuthorityRules were to allow it. (Today Validate passes it: Required[unknown] is nil.)
func TestSchemaGuardRejectsUnknownKind(t *testing.T) {
	g := DefaultSchemaGuard()
	if err := g.Validate("phantom", map[string]any{"content": "x"}); err == nil {
		t.Fatal("Validate must reject a kind not in its Required map")
	}
	if err := g.Validate("memory", map[string]any{"content": "x"}); err != nil {
		t.Fatalf("known kind must still pass: %v", err)
	}
}
