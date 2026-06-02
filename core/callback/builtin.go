package callback

import (
	"github.com/mnemon-dev/mnemon/core/contract"
	"github.com/mnemon-dev/mnemon/core/projection"
)

// BuiltinFunc adapts a plain Go func into a Callback: an in-process, trusted-as-builtin proposer.
// Invariant #15's UNTRUSTED control agent must be a builtin like this (no FS/net reach), never a
// script — see script.go's honesty note.
type BuiltinFunc func(contract.Event, projection.Projection) ([]contract.ProposedEvent, error)

func (f BuiltinFunc) OnEvent(ev contract.Event, view projection.Projection) ([]contract.ProposedEvent, error) {
	return f(ev, view)
}
