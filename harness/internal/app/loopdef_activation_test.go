package app

import (
	"testing"

	"github.com/suxatcode/mnemon/harness/internal/runtime"
)

// P3e-4: booting with a materialized loopdef package records a G4 activation event in the log,
// exactly once (idempotent per name+version+digest) — the durable audit of what was activated.
func TestLoopdefActivationLedger(t *testing.T) {
	projectRoot := t.TempDir()
	rt := admitLoopdefDraft(t, t.TempDir(), loopdefValidDraft)
	defer rt.Close()
	if err := materializeLoopdefs(rt, projectRoot); err != nil {
		t.Fatalf("materialize: %v", err)
	}

	if err := emitLoopdefActivations(rt, projectRoot); err != nil {
		t.Fatalf("emit activations: %v", err)
	}
	if n := countActivations(t, rt); n != 1 {
		t.Fatalf("want exactly one activation event, got %d", n)
	}

	// a second boot over the same materialized catalog records nothing new (idempotent).
	if err := emitLoopdefActivations(rt, projectRoot); err != nil {
		t.Fatalf("re-emit activations: %v", err)
	}
	if n := countActivations(t, rt); n != 1 {
		t.Fatalf("re-boot must not duplicate the activation event, got %d", n)
	}
}

func countActivations(t *testing.T, rt *runtime.Runtime) int {
	t.Helper()
	events, err := rt.PendingEvents(0)
	if err != nil {
		t.Fatalf("pending events: %v", err)
	}
	n := 0
	for _, e := range events {
		if e.Type == "loopdef.activated.observed" {
			if name, _ := e.Payload["name"].(string); name == "widget2" {
				n++
			}
		}
	}
	return n
}
