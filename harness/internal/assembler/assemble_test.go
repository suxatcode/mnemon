package assembler

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/mnemon-dev/mnemon/harness/internal/channel"
	"github.com/mnemon-dev/mnemon/harness/internal/config"
	"github.com/mnemon-dev/mnemon/harness/internal/contract"
	"github.com/mnemon-dev/mnemon/harness/internal/runtime"
)

// A 3rd capability (note) stands up end-to-end through config + the generic kind alone — no new rule
// code: Assemble compiles the config into a runtime config whose note rule admits a note candidate
// through the channel -> tick -> kernel -> projection.
func TestAssembleAdmitsConfiguredNoteCapabilityEndToEnd(t *testing.T) {
	ref := contract.ResourceRef{Kind: "note", ID: "project"}
	binding := channel.HostAgentBinding("codex@project", "http://127.0.0.1:8787", []contract.ResourceRef{ref})
	binding.AllowedObservedTypes = []string{"note.write_candidate.observed"}

	cfg := config.File{Capabilities: map[string]config.CapabilityConfig{
		"note": {Enabled: true, ResourceRef: "note/project", RuleRef: "native:note"},
	}}
	rc, err := Assemble(cfg, []channel.ChannelBinding{binding})
	if err != nil {
		t.Fatalf("assemble: %v", err)
	}

	rt, err := runtime.OpenRuntime(filepath.Join(t.TempDir(), "g.db"), rc)
	if err != nil {
		t.Fatalf("open runtime: %v", err)
	}
	defer rt.Close()

	if _, _, err := rt.API().Ingest("codex@project", contract.ObservationEnvelope{
		ExternalID: "n1",
		Event:      contract.Event{Type: "note.write_candidate.observed", Payload: map[string]any{"text": "remember the assembler"}},
	}); err != nil {
		t.Fatalf("ingest note: %v", err)
	}
	if _, err := rt.Tick(); err != nil {
		t.Fatalf("tick: %v", err)
	}
	v, fields, err := rt.Resource(ref)
	if err != nil {
		t.Fatalf("read note: %v", err)
	}
	if v == 0 {
		t.Fatal("the configured note capability must admit a candidate (resource not created)")
	}
	if content, _ := fields["content"].(string); !strings.Contains(content, "remember the assembler") {
		t.Fatalf("note content missing the candidate: %q", content)
	}
}

func TestAssembleFailsClosedOnUnknownCapability(t *testing.T) {
	cfg := config.File{Capabilities: map[string]config.CapabilityConfig{
		"bogus": {Enabled: true, ResourceRef: "bogus/project", RuleRef: "native:bogus"},
	}}
	if _, err := Assemble(cfg, nil); err == nil {
		t.Fatal("an unknown capability rule_ref must fail closed")
	}
}
