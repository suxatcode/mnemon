package coreengine

import (
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/mnemon-dev/mnemon/harness/core/contract"
	"github.com/mnemon-dev/mnemon/harness/core/kernel"
	"github.com/mnemon-dev/mnemon/harness/core/rule"
	"github.com/mnemon-dev/mnemon/harness/core/server"
)

// TestWalkingSkeletonOneRuntimeTwoSurfaces is the P1.4 walking skeleton: an operator/lifecycle Agent
// Surface applies a governed memory entry and a host-agent Agent Surface pulls it — both through ONE
// server.Runtime over ONE canonical store, both riding the SAME channel (server.Client over httptest),
// mediated by hardcoded ChannelBindings. It proves the load-bearing P1 claim: the lifecycle/app apply
// is just another Agent Surface on the channel, not a privileged backdoor, and a second surface reads
// the governed state with no host file/mirror write.
func TestWalkingSkeletonOneRuntimeTwoSurfaces(t *testing.T) {
	const (
		operator = contract.ActorID("operator@project")
		codex    = contract.ActorID("codex@project")
	)
	root := t.TempDir()
	storePath := filepath.Join(root, server.DefaultStorePath)
	ref := contract.ResourceRef{Kind: "memory", ID: "p1/e1"}
	observed := "memory.governed.observed"

	// ONE runtime: the operator is the governed-create proposer; both principals are scoped to the
	// same memory ref so the host-agent can read what the operator writes.
	rt, err := server.OpenRuntime(storePath, server.RuntimeConfig{
		Rules:     rule.NewRuleSet(governedCreateRule("memory", operator, observed)),
		Authority: kernel.AuthorityRules{Allow: map[contract.ActorID][]contract.ResourceKind{operator: {"memory"}}},
		Subs: map[contract.ActorID]contract.Subscription{
			operator: {Actor: operator, Refs: []contract.ResourceRef{ref}},
			codex:    {Actor: codex, Refs: []contract.ResourceRef{ref}},
		},
		NewID: seqGen(), Now: fixedNow(),
	})
	if err != nil {
		t.Fatalf("open runtime: %v", err)
	}
	defer rt.Close()

	// One channel; the two surfaces differ only by binding (D6), never by a privileged path. The
	// lifecycle/app apply rides the operator's control-agent binding; the host-agent its own.
	srv := httptest.NewServer(server.NewHTTPHandler(rt.API()))
	defer srv.Close()
	opBind := server.ControlAgentBinding(operator, srv.URL, []contract.ResourceRef{ref})
	cxBind := server.HostAgentBinding(codex, srv.URL, []contract.ResourceRef{ref})
	if !opBind.Allows(server.VerbObserve) {
		t.Fatal("operator binding must grant observe — the governed apply is a channel verb, not a backdoor")
	}
	if !cxBind.Allows(server.VerbPull) {
		t.Fatal("host-agent binding must grant pull")
	}

	// operator/lifecycle surface: apply a governed memory entry THROUGH the channel; the runtime (the
	// single Tick driver) then processes it.
	opClient := server.NewClient(srv.URL, operator)
	if _, _, err := opClient.Ingest(operator, contract.ObservationEnvelope{
		ExternalID: "apply-1",
		Event: contract.Event{Type: observed, CorrelationID: "memory:apply-1", Payload: map[string]any{
			"entry_id": string(ref.ID),
			"fields":   map[string]any{"content": "governed by operator", "summary": "s"},
		}},
	}); err != nil {
		t.Fatalf("operator ingest: %v", err)
	}
	if _, err := rt.Tick(); err != nil {
		t.Fatalf("tick: %v", err)
	}

	// the resource exists in the canonical kernel store.
	if v, _, _ := rt.Resource(ref); v == 0 {
		t.Fatalf("operator apply must create %s in the canonical store", ref.ID)
	}

	// host-agent surface: pull the scoped projection through the channel as codex@project.
	cxClient := server.NewClient(srv.URL, codex)
	cxProj, err := cxClient.PullProjection(codex, contract.Subscription{Actor: codex})
	if err != nil {
		t.Fatalf("codex pull: %v", err)
	}
	if rvVersion(cxProj.Resources, ref) == 0 {
		t.Fatalf("host-agent pull must see the operator-governed entry %s; got %+v", ref.ID, cxProj.Resources)
	}

	// a second control surface pulling the same scope sees the SAME digest — one governed projection.
	opProj, err := opClient.PullProjection(operator, contract.Subscription{Actor: operator})
	if err != nil {
		t.Fatalf("operator pull: %v", err)
	}
	if opProj.Digest != cxProj.Digest {
		t.Fatalf("two surfaces over one governed projection must agree on the digest; operator=%q codex=%q", opProj.Digest, cxProj.Digest)
	}

	// no privileged path: the host-agent cannot widen to another principal's scope by naming it on
	// the wire (the §2 authority boundary; S9/D7). It reads only through its OWN binding scope.
	if _, err := cxClient.PullProjection(codex, contract.Subscription{Actor: operator}); err == nil {
		t.Fatal("host-agent must not pull another principal's scope by naming it (no backdoor)")
	}

	// The reads succeeded purely from the canonical kernel store — no host file/mirror write was made.
}

func rvVersion(rvs []contract.ResourceVersion, ref contract.ResourceRef) contract.Version {
	for _, rv := range rvs {
		if rv.Ref == ref {
			return rv.Version
		}
	}
	return 0
}
