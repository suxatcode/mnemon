package config

import (
	"testing"

	"github.com/mnemon-dev/mnemon/harness/core/callback"
	"github.com/mnemon-dev/mnemon/harness/core/contract"
	"github.com/mnemon-dev/mnemon/harness/core/projection"
)

func noopCB() callback.Callback {
	return callback.BuiltinFunc(func(contract.Event, projection.Projection) ([]contract.ProposedEvent, error) { return nil, nil })
}
func validCfg() RuntimeConfig {
	return RuntimeConfig{
		SchemaVersion: 1,
		Modes:         ModeConfig{Conflict: "reject", Isolation: "projection_read_set", Authz: "strict"},
		Actors:        map[contract.ActorID][]contract.ResourceKind{"agent": {"memory"}},
		Bindings:      []BindingConfig{{EventType: "memory.observed", Callback: "memory-writer", Actor: "agent", Emits: "memory.write.proposed"}},
		Scopes:        map[contract.ActorID][]contract.ResourceRef{"agent": {{Kind: "memory", ID: "m1"}}},
	}
}
func builtins() map[string]callback.Callback { return map[string]callback.Callback{"memory-writer": noopCB()} }

func TestResolveAcceptsValid(t *testing.T) {
	r, err := Resolve(validCfg(), builtins())
	if err != nil {
		t.Fatalf("valid config rejected: %v", err)
	}
	if r.Modes.Conflict != "reject" || len(r.Bindings) != 1 || r.Bindings[0].Actor != "agent" {
		t.Fatalf("unexpected resolved: %+v", r)
	}
}
func TestResolveRejectsBadInputs(t *testing.T) {
	bad := map[string]func(*RuntimeConfig){
		"unknown conflict mode":     func(c *RuntimeConfig) { c.Modes.Conflict = "./evil.sh" },
		"unknown isolation":         func(c *RuntimeConfig) { c.Modes.Isolation = "serializable" },
		"unknown authz":             func(c *RuntimeConfig) { c.Modes.Authz = "permissive" },
		"phantom actor kind":        func(c *RuntimeConfig) { c.Actors["agent"] = []contract.ResourceKind{"phantom"} },
		"phantom scope kind":        func(c *RuntimeConfig) { c.Scopes["agent"] = []contract.ResourceRef{{Kind: "phantom", ID: "x"}} },
		"unknown callback key":      func(c *RuntimeConfig) { c.Bindings[0].Callback = "./evil.sh" },
		"undeclared binding actor":  func(c *RuntimeConfig) { c.Bindings[0].Actor = "ghost" },
		"non-proposed emit":         func(c *RuntimeConfig) { c.Bindings[0].Emits = "memory.write" },
		"proposed EventType (loop)": func(c *RuntimeConfig) { c.Bindings[0].EventType = "memory.write.proposed" }, // self-amplifying loop
		"empty EventType":           func(c *RuntimeConfig) { c.Bindings[0].EventType = "" },
		"wrong schema version":      func(c *RuntimeConfig) { c.SchemaVersion = 2 },
		"undeclared scope actor":    func(c *RuntimeConfig) { c.Scopes["ghost"] = []contract.ResourceRef{{Kind: "memory", ID: "m1"}} },
	}
	for name, mut := range bad {
		c := validCfg()
		mut(&c)
		if _, err := Resolve(c, builtins()); err == nil {
			t.Fatalf("%s: expected rejection (define≠select breach)", name)
		}
	}
}

// P1 Gate surface: a builtin key that resolves to a nil callback must be rejected (a registered-but-empty
// callback is not selectable; the runtime must never dispatch into a nil proposer).
func TestResolveRejectsNilCallback(t *testing.T) {
	if _, err := Resolve(validCfg(), map[string]callback.Callback{"memory-writer": nil}); err == nil {
		t.Fatal("nil callback must be rejected")
	}
}
