package callback

import (
	"github.com/mnemon-dev/mnemon/core/contract"
	"github.com/mnemon-dev/mnemon/core/projection"
)

// Callback observes + computes; returns INTENTS, not facts. No *kernel.Kernel / *Store in scope —
// a callback is structurally incapable of committing a fact; the commit is always the kernel's
// (Invariant #13). Its output is an intent, never a fait-accompli mutation.
type Callback interface {
	OnEvent(ev contract.Event, view projection.Projection) ([]contract.ProposedEvent, error)
}
