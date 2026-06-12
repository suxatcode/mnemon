package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mnemon-dev/mnemon/harness/internal/capability"
	"github.com/mnemon-dev/mnemon/harness/internal/channel"
	"github.com/mnemon-dev/mnemon/harness/internal/contract"
	"github.com/mnemon-dev/mnemon/harness/internal/kernel"
	"github.com/mnemon-dev/mnemon/harness/internal/runtime"
)

// admitLoopdefDraft boots an operator runtime, admits one loopdef draft, and returns the runtime.
func admitLoopdefDraft(t *testing.T, storeDir, draft string) *runtime.Runtime {
	t.Helper()
	ldRef := contract.ResourceRef{Kind: "loopdef", ID: "project"}
	operator := channel.ControlAgentBinding("human@owner", "http://127.0.0.1:8787", []contract.ResourceRef{ldRef})
	operator.AllowedObservedTypes = []string{"loopdef.write_candidate.observed"}
	rc, err := LocalRuntimeConfigFromBindings([]channel.ChannelBinding{operator}, nil)
	if err != nil {
		t.Fatalf("boot config: %v", err)
	}
	rt, err := runtime.OpenRuntime(filepath.Join(storeDir, "ld.db"), rc)
	if err != nil {
		t.Fatalf("open runtime: %v", err)
	}
	if _, _, err := rt.API().Ingest("human@owner", contract.ObservationEnvelope{
		ExternalID: "m1",
		Event:      contract.Event{Type: "loopdef.write_candidate.observed", Payload: map[string]any{"spec": draft}},
	}); err != nil {
		t.Fatalf("ingest loopdef: %v", err)
	}
	if _, err := rt.Tick(); err != nil {
		t.Fatalf("tick: %v", err)
	}
	return rt
}

// P3e-3: an admitted loopdef draft materializes to a managed external package — default_enabled (so
// reload governs it) + a .managed provenance marker — and that package RESOLVES (it is ready to be
// governed at the next reload). Materialize writes only; it never activates the live runtime.
func TestMaterializeLoopdef(t *testing.T) {
	projectRoot := t.TempDir()
	rt := admitLoopdefDraft(t, t.TempDir(), loopdefValidDraft)
	defer rt.Close()

	if err := materializeLoopdefs(rt, projectRoot); err != nil {
		t.Fatalf("materialize: %v", err)
	}
	capPath := filepath.Join(projectRoot, ".mnemon", "loops", "widget2", "capability.json")
	data, err := os.ReadFile(capPath)
	if err != nil {
		t.Fatalf("materialized capability.json must exist: %v", err)
	}
	if !strings.Contains(string(data), "default_enabled") {
		t.Fatalf("a materialized spec must be default_enabled (M3):\n%s", data)
	}
	if _, err := os.ReadFile(filepath.Join(projectRoot, ".mnemon", "loops", "widget2", ".managed")); err != nil {
		t.Fatalf("materialized package must carry a .managed marker: %v", err)
	}
	// the materialized package is a valid external package — it resolves, ready for the next reload.
	catalog, err := capability.ResolveCatalog(projectRoot, kernel.DefaultSchemaGuard().Required)
	if err != nil {
		t.Fatalf("materialized package must resolve: %v", err)
	}
	if _, ok := catalog["widget2"]; !ok {
		t.Fatalf("the materialized widget2 kind must resolve in the catalog: %v", catalog)
	}
}

// G5 isolation: a human-placed package (no .managed marker) sharing a draft's name is NEVER clobbered
// by materialization.
func TestMaterializeSkipsHumanPackage(t *testing.T) {
	projectRoot := t.TempDir()
	humanDir := filepath.Join(projectRoot, ".mnemon", "loops", "widget2")
	if err := os.MkdirAll(humanDir, 0o755); err != nil {
		t.Fatal(err)
	}
	const humanContent = `{"human":"placed this"}`
	if err := os.WriteFile(filepath.Join(humanDir, "capability.json"), []byte(humanContent), 0o644); err != nil {
		t.Fatal(err)
	}

	rt := admitLoopdefDraft(t, t.TempDir(), loopdefValidDraft)
	defer rt.Close()
	if err := materializeLoopdefs(rt, projectRoot); err != nil {
		t.Fatalf("materialize: %v", err)
	}
	got, _ := os.ReadFile(filepath.Join(humanDir, "capability.json"))
	if string(got) != humanContent {
		t.Fatalf("materialize must not clobber a human-placed package (G5); got:\n%s", got)
	}
	if _, err := os.Stat(filepath.Join(humanDir, ".managed")); !os.IsNotExist(err) {
		t.Fatalf("materialize must not drop a .managed marker into a human package (G5)")
	}
}
