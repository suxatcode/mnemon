package app

import (
	"path/filepath"
	"testing"

	"github.com/mnemon-dev/mnemon/harness/internal/channel"
	"github.com/mnemon-dev/mnemon/harness/internal/contract"
	"github.com/mnemon-dev/mnemon/harness/internal/runtime"
)

// P6a-2: the Tower facade assembles GOAL (project_intent statements) and LEDGER (accepted decisions
// with attribution) read-only from the runtime. An admitted project_intent write shows up on both: the
// goal statement on GOAL, the accepted decision (attributed to the proposer) on LEDGER.
func TestBuildTowerViewGoalAndLedger(t *testing.T) {
	piRef := contract.ResourceRef{Kind: "project_intent", ID: "project"}
	binding := channel.HostAgentBinding("codex@project", "http://127.0.0.1:8787", []contract.ResourceRef{piRef})
	binding.AllowedObservedTypes = []string{"project_intent.write_candidate.observed"}
	rc, err := LocalRuntimeConfigFromBindings([]channel.ChannelBinding{binding}, nil)
	if err != nil {
		t.Fatalf("boot config: %v", err)
	}
	rt, err := runtime.OpenRuntime(filepath.Join(t.TempDir(), "tower.db"), rc)
	if err != nil {
		t.Fatalf("open runtime: %v", err)
	}
	defer rt.Close()

	if _, _, err := rt.API().Ingest("codex@project", contract.ObservationEnvelope{
		ExternalID: "pi1",
		Event: contract.Event{Type: "project_intent.write_candidate.observed", Payload: map[string]any{
			"statement": "ship the AgentTeam beta", "evidence": "roadmap-q3"}},
	}); err != nil {
		t.Fatalf("ingest project_intent: %v", err)
	}
	if _, err := rt.Tick(); err != nil {
		t.Fatalf("tick: %v", err)
	}

	v, err := BuildTowerView(rt)
	if err != nil {
		t.Fatalf("build tower view: %v", err)
	}

	// GOAL: the goal statement is surfaced.
	if len(v.Goal.Statements) != 1 || v.Goal.Statements[0] != "ship the AgentTeam beta" {
		t.Fatalf("GOAL statements wrong: %+v", v.Goal.Statements)
	}

	// LEDGER: the accepted project_intent decision, attributed to the proposer, with the changed ref.
	var found bool
	for _, d := range v.Ledger.Decisions {
		if d.Actor != "codex@project" {
			continue
		}
		for _, r := range d.Refs {
			if r.Kind == "project_intent" {
				found = true
			}
		}
	}
	if !found {
		t.Fatalf("LEDGER must carry the accepted project_intent decision with attribution: %+v", v.Ledger.Decisions)
	}
}

// An empty runtime yields empty pages (no panic, no fabricated data).
func TestBuildTowerViewEmpty(t *testing.T) {
	binding := channel.HostAgentBinding("codex@project", "http://127.0.0.1:8787",
		[]contract.ResourceRef{{Kind: "memory", ID: "project"}})
	binding.AllowedObservedTypes = []string{"memory.write_candidate.observed"}
	rc, err := LocalRuntimeConfigFromBindings([]channel.ChannelBinding{binding}, nil)
	if err != nil {
		t.Fatalf("boot config: %v", err)
	}
	rt, err := runtime.OpenRuntime(filepath.Join(t.TempDir(), "empty.db"), rc)
	if err != nil {
		t.Fatalf("open runtime: %v", err)
	}
	defer rt.Close()

	v, err := BuildTowerView(rt)
	if err != nil {
		t.Fatalf("build tower view: %v", err)
	}
	if len(v.Goal.Statements) != 0 || len(v.Ledger.Decisions) != 0 {
		t.Fatalf("empty runtime must yield empty pages, got goal=%+v ledger=%+v", v.Goal, v.Ledger)
	}
}
