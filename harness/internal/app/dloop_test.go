package app

import (
	"path/filepath"
	"testing"

	"github.com/mnemon-dev/mnemon/harness/internal/capability"
	"github.com/mnemon-dev/mnemon/harness/internal/channel"
	"github.com/mnemon-dev/mnemon/harness/internal/contract"
	"github.com/mnemon-dev/mnemon/harness/internal/kernel"
	"github.com/mnemon-dev/mnemon/harness/internal/runtime"
)

// TestDLoopFullCycle is the D-loop end to end (P3e-5): an OPERATOR proposes a loopdef defining a NEW
// event kind (widget2) → it is admitted (high-risk, operator only) → materialized to .mnemon/loops →
// a RELOAD (re-resolve the catalog + re-assemble, exactly what `mnemond reload` does on restart)
// makes the new kind governed → a widget2 candidate is admitted → the old loopdef resource survives
// the reload. The two boots share ONE persistent store, so "reload" is a re-open, not a reset.
func TestDLoopFullCycle(t *testing.T) {
	projectRoot := t.TempDir()
	storePath := filepath.Join(t.TempDir(), "dloop.db")
	ldRef := contract.ResourceRef{Kind: "loopdef", ID: "project"}
	w2Ref := contract.ResourceRef{Kind: "widget2", ID: "project"}

	// --- boot 1: the operator proposes a loopdef (the draft defines widget2). ---
	operator := channel.ControlAgentBinding("human@owner", "http://127.0.0.1:8787", []contract.ResourceRef{ldRef})
	operator.AllowedObservedTypes = []string{"loopdef.write_candidate.observed"}
	rc1, err := LocalRuntimeConfigFromBindings([]channel.ChannelBinding{operator}, nil)
	if err != nil {
		t.Fatalf("boot1 config: %v", err)
	}
	rt1, err := runtime.OpenRuntime(storePath, rc1)
	if err != nil {
		t.Fatalf("open rt1: %v", err)
	}
	if _, _, err := rt1.API().Ingest("human@owner", contract.ObservationEnvelope{
		ExternalID: "d1",
		Event:      contract.Event{Type: "loopdef.write_candidate.observed", Payload: map[string]any{"spec": loopdefValidDraft}},
	}); err != nil {
		t.Fatalf("propose loopdef: %v", err)
	}
	if _, err := rt1.Tick(); err != nil {
		t.Fatalf("tick: %v", err)
	}
	if v, _, _ := rt1.Resource(ldRef); v == 0 {
		t.Fatal("the operator's loopdef must be admitted")
	}

	// materialize the admitted draft (what the driver bridge does on the accept).
	if err := materializeLoopdefs(rt1, projectRoot); err != nil {
		t.Fatalf("materialize: %v", err)
	}
	_ = rt1.Close()

	// --- reload: re-resolve the catalog (now carrying widget2) + re-assemble (= mnemond reload). ---
	catalog2, err := capability.ResolveCatalog(projectRoot, kernel.DefaultSchemaGuard().Required)
	if err != nil {
		t.Fatalf("resolve after materialize: %v", err)
	}
	if _, ok := catalog2["widget2"]; !ok {
		t.Fatalf("the materialized widget2 kind must resolve after reload: %v", catalog2)
	}

	// --- boot 2: a host now governs the NEW kind (widget2 is default_enabled → boot grants it). ---
	host := channel.HostAgentBinding("codex@project", "http://127.0.0.1:8787", nil)
	rc2, err := LocalRuntimeConfigFromBindings([]channel.ChannelBinding{host}, catalog2)
	if err != nil {
		t.Fatalf("boot2 config: %v", err)
	}
	rt2, err := runtime.OpenRuntime(storePath, rc2)
	if err != nil {
		t.Fatalf("open rt2: %v", err)
	}
	defer rt2.Close()
	if _, _, err := rt2.API().Ingest("codex@project", contract.ObservationEnvelope{
		ExternalID: "d2",
		Event:      contract.Event{Type: "widget2.write_candidate.observed", Payload: map[string]any{"text": "the new kind works"}},
	}); err != nil {
		t.Fatalf("observe widget2: %v", err)
	}
	if _, err := rt2.Tick(); err != nil {
		t.Fatalf("tick: %v", err)
	}
	if v, _, _ := rt2.Resource(w2Ref); v == 0 {
		t.Fatal("the new kind widget2 must be governed after reload (D-loop)")
	}
	// the old loopdef resource survives the reload (one persistent store; I6).
	if v, _, _ := rt2.Resource(ldRef); v == 0 {
		t.Fatal("the loopdef resource must survive the reload")
	}
}
