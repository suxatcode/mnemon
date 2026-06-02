package reconcile

import (
	"fmt"

	"github.com/mnemon-dev/mnemon/core/contract"
)

type Config struct{ Conflict, Isolation, Authz string }

// ResolveModes is SELECT-only: every field must name a mode DEFINED in the trusted catalog
// (Invariant #12, define≠select). An unknown name in ANY field — including a script path — is
// rejected and never executed. No path turns a mode string into behaviour by running it.
func ResolveModes(c Config) (contract.Modes, error) {
	if !contract.ConflictCatalog[c.Conflict] {
		return contract.Modes{}, fmt.Errorf("unknown conflict mode %q", c.Conflict)
	}
	if !contract.IsolationCatalog[c.Isolation] {
		return contract.Modes{}, fmt.Errorf("unknown isolation mode %q", c.Isolation)
	}
	if !contract.AuthzCatalog[c.Authz] {
		return contract.Modes{}, fmt.Errorf("unknown authz mode %q", c.Authz)
	}
	return contract.Modes{Conflict: c.Conflict, Isolation: c.Isolation, Authz: c.Authz}, nil
}
