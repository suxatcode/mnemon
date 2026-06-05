package rule

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"testing"

	"github.com/mnemon-dev/mnemon/harness/core/contract"
)

func wasmBytes(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile("wasm/testdata/" + name)
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	return b
}

func sha(b []byte) string { s := sha256.Sum256(b); return hex.EncodeToString(s[:]) }

func placeholder() Rule {
	return NewNativeRule("cand", "agent", "memory.write.proposed", []string{"memory.observed"},
		func(RuleInput) (contract.RuleDecision, error) { return contract.RuleDecision{Verdict: contract.VerdictAllow}, nil })
}

// S12: a candidate wasm rule enters the active set ONLY with a clean shadow report and a matching sha256.
func TestPromotionRequiresCleanShadow(t *testing.T) {
	b := wasmBytes(t, "rule_allow_if_evidence.wasm")
	m := Manifest{ID: "r", SHA256: sha(b)}

	reg := NewRegistry()
	if err := reg.Promote(b, placeholder(), m, ShadowReport{Clean: true}); err != nil {
		t.Fatalf("clean shadow + matching sha must promote: %v", err)
	}
	if len(reg.Active().Rules()) != 1 {
		t.Fatalf("a promoted rule must enter the active set; got %d", len(reg.Active().Rules()))
	}
	if err := NewRegistry().Promote(b, placeholder(), m, ShadowReport{Clean: false, Diffs: 3}); err == nil {
		t.Fatal("a non-clean shadow must reject promotion")
	}
	if err := NewRegistry().Promote(b, placeholder(), Manifest{SHA256: "deadbeef"}, ShadowReport{Clean: true}); err == nil {
		t.Fatal("a sha256 mismatch must reject promotion")
	}
}

// S12: a .wasm importing anything beyond env.read_state_view is rejected at promotion.
func TestManifestRejectsExtraImports(t *testing.T) {
	good := wasmBytes(t, "rule_allow_if_evidence.wasm")
	if err := NewRegistry().Promote(good, placeholder(), Manifest{SHA256: sha(good)}, ShadowReport{Clean: true}); err != nil {
		t.Fatalf("a module importing only env.read_state_view must pass: %v", err)
	}
	bad := wasmBytes(t, "two_imports.wasm")
	if err := NewRegistry().Promote(bad, placeholder(), Manifest{SHA256: sha(bad)}, ShadowReport{Clean: true}); err == nil {
		t.Fatal("a module importing beyond env.read_state_view must be rejected at promotion")
	}
}

// D10: an edge rule snapshot is DENY-ONLY — a propose verdict is downgraded to warn (the proposal dropped).
func TestEdgeSnapshotIsDenyOnly(t *testing.T) {
	proposer := NewNativeRule("p", "agent", "memory.write.proposed", []string{"memory.observed"},
		func(RuleInput) (contract.RuleDecision, error) {
			return contract.RuleDecision{Verdict: contract.VerdictPropose, Proposal: &contract.ProposedEvent{Type: "memory.write.proposed"}}, nil
		})
	denier := NewNativeRule("d", "agent", "memory.write.proposed", []string{"memory.observed"},
		func(RuleInput) (contract.RuleDecision, error) { return contract.RuleDecision{Verdict: contract.VerdictDeny}, nil })

	// a propose-only edge -> warn (downgraded), never propose, proposal dropped.
	d, _ := EdgeSnapshot(NewRuleSet(proposer)).Evaluate(RuleInput{Event: contract.Event{Type: "memory.observed"}})
	if d.Verdict == contract.VerdictPropose || d.Proposal != nil {
		t.Fatalf("edge must not propose; got verdict=%q proposal=%v", d.Verdict, d.Proposal)
	}
	if d.Verdict != contract.VerdictWarn {
		t.Fatalf("a downgraded propose must become warn; got %q", d.Verdict)
	}
	// deny still passes through (deny beats the downgraded warn).
	d2, _ := EdgeSnapshot(NewRuleSet(proposer, denier)).Evaluate(RuleInput{Event: contract.Event{Type: "memory.observed"}})
	if d2.Verdict != contract.VerdictDeny {
		t.Fatalf("a deny must survive the edge snapshot; got %q", d2.Verdict)
	}
}
