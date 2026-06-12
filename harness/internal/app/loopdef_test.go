package app

import (
	"path/filepath"
	"testing"

	"github.com/mnemon-dev/mnemon/harness/internal/channel"
	"github.com/mnemon-dev/mnemon/harness/internal/contract"
	"github.com/mnemon-dev/mnemon/harness/internal/runtime"
)

// a minimal VALID capability spec draft (the loopdef payload), serialized.
const loopdefValidDraft = `{"schema_version":1,"name":"widget2","observed_type":"widget2.write_candidate.observed",` +
	`"proposed_type":"widget2.write.proposed","resource_kind":"widget2","items_field":"items",` +
	`"fields":[{"name":"text","validators":[{"id":"required","params":{"missing_style":"empty"}}]}],` +
	`"render":{"content":{"member":"bullet-list","params":{"title":"# W2","field":"text"}}}}`

// P3e-2: loopdef is high-risk + default-enabled. An OPERATOR (control-agent) governs it — a valid
// spec draft admits, an invalid draft is denied by the spec-draft validator. (The agent-denied half
// is TestLoopdefDeniedFromAgent.)
func TestLoopdefGovernedByOperator(t *testing.T) {
	ldRef := contract.ResourceRef{Kind: "loopdef", ID: "project"}
	operator := channel.ControlAgentBinding("human@owner", "http://127.0.0.1:8787", []contract.ResourceRef{ldRef})
	operator.AllowedObservedTypes = []string{"loopdef.write_candidate.observed"}
	rc, err := LocalRuntimeConfigFromBindings([]channel.ChannelBinding{operator}, nil)
	if err != nil {
		t.Fatalf("boot config: %v", err)
	}
	rt, err := runtime.OpenRuntime(filepath.Join(t.TempDir(), "ld.db"), rc)
	if err != nil {
		t.Fatalf("open runtime: %v", err)
	}
	defer rt.Close()

	// operator + valid draft → admitted.
	if _, _, err := rt.API().Ingest("human@owner", contract.ObservationEnvelope{
		ExternalID: "l1",
		Event:      contract.Event{Type: "loopdef.write_candidate.observed", Payload: map[string]any{"spec": loopdefValidDraft}},
	}); err != nil {
		t.Fatalf("ingest loopdef: %v", err)
	}
	if _, err := rt.Tick(); err != nil {
		t.Fatalf("tick: %v", err)
	}
	v, _, err := rt.Resource(ldRef)
	if err != nil || v == 0 {
		t.Fatalf("operator loopdef with a valid draft must admit (v=%d err=%v)", v, err)
	}

	// operator + invalid draft → denied by the spec-draft validator, version unchanged.
	if _, _, err := rt.API().Ingest("human@owner", contract.ObservationEnvelope{
		ExternalID: "l2",
		Event:      contract.Event{Type: "loopdef.write_candidate.observed", Payload: map[string]any{"spec": "not a spec"}},
	}); err != nil {
		t.Fatalf("ingest invalid loopdef: %v", err)
	}
	if _, err := rt.Tick(); err != nil {
		t.Fatalf("tick: %v", err)
	}
	if v2, _, _ := rt.Resource(ldRef); v2 != v {
		t.Fatalf("an invalid loopdef draft must be denied, version moved %d -> %d", v, v2)
	}
}

// P3e-2: a loopdef candidate from an AGENT (host-agent) is denied — loopdef is high-risk, so it needs
// operator approval (G2).
func TestLoopdefDeniedFromAgent(t *testing.T) {
	ldRef := contract.ResourceRef{Kind: "loopdef", ID: "project"}
	host := channel.HostAgentBinding("codex@project", "http://127.0.0.1:8787", []contract.ResourceRef{ldRef})
	host.AllowedObservedTypes = []string{"loopdef.write_candidate.observed"}
	rc, err := LocalRuntimeConfigFromBindings([]channel.ChannelBinding{host}, nil)
	if err != nil {
		t.Fatalf("boot config: %v", err)
	}
	rt, err := runtime.OpenRuntime(filepath.Join(t.TempDir(), "lda.db"), rc)
	if err != nil {
		t.Fatalf("open runtime: %v", err)
	}
	defer rt.Close()
	if _, _, err := rt.API().Ingest("codex@project", contract.ObservationEnvelope{
		ExternalID: "la1",
		Event:      contract.Event{Type: "loopdef.write_candidate.observed", Payload: map[string]any{"spec": loopdefValidDraft}},
	}); err != nil {
		t.Fatalf("ingest: %v", err)
	}
	if _, err := rt.Tick(); err != nil {
		t.Fatalf("tick: %v", err)
	}
	if v, _, _ := rt.Resource(ldRef); v != 0 {
		t.Fatalf("a loopdef candidate from a host-agent must be denied (high-risk), but it admitted (v=%d)", v)
	}
}
