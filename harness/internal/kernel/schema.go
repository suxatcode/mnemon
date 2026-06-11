package kernel

import (
	"fmt"

	"github.com/mnemon-dev/mnemon/harness/internal/contract"
)

type SchemaGuard struct {
	Required map[contract.ResourceKind][]string
}

func DefaultSchemaGuard() SchemaGuard {
	return SchemaGuard{Required: map[contract.ResourceKind][]string{
		"memory": {"content"},
		"goal":   {"statement"},
		"skill":  {"name"},
		// lease/budget/receipt are GOVERNANCE resource kinds (D3): versioned control-plane state whose
		// required fields back fencing, budget accounting, and the durable record of an external effect.
		// Kept registered for compatibility of existing logs and external-package reservation checks.
		// Must stay in lockstep with contract.KindCatalog (kind_catalog_test).
		"lease":   {"job_id", "owner", "fence_until"},
		"budget":  {"limit_usd", "spent_usd"},
		"receipt": {"job_id", "effect_id", "outcome"},
		// coordination records a governed teamwork-topology op (P2.2 route 3/3); operation is the
		// minimal required field. Must stay in lockstep with contract.KindCatalog (kind_catalog_test).
		"coordination": {"operation"},
		// note is the Phase-2 3rd capability proving config-only assembly; the generic kind renders
		// items into content. Must stay in lockstep with contract.KindCatalog (kind_catalog_test).
		"note": {"content"},
		// decision is the stage-2 4th capability: a spec file + this line is its ENTIRE Go footprint
		// (the L2 one-line registration the platform promises). Lockstep with contract.KindCatalog.
		"decision": {"content"},
	}}
}
func (g SchemaGuard) Validate(kind contract.ResourceKind, fields map[string]any) error {
	required, known := g.Required[kind]
	if !known {
		return fmt.Errorf("%w: unknown resource kind %q", errSchema, kind)
	}
	for _, f := range required {
		if _, ok := fields[f]; !ok {
			return fmt.Errorf("%w: %s requires %q", errSchema, kind, f)
		}
	}
	return nil
}
