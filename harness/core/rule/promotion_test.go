package rule

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
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

// buildOK ignores the bytes and returns a fixed placeholder rule (the gate's byte checks are what's tested).
func buildOK(string) func([]byte) (Rule, error) {
	return func([]byte) (Rule, error) { return placeholder(), nil }
}

// S12: a candidate enters the active set ONLY with a clean shadow report and a matching sha256.
func TestPromotionRequiresCleanShadow(t *testing.T) {
	b := wasmBytes(t, "rule_allow_if_evidence.wasm")
	m := Manifest{ID: "r", SHA256: sha(b)}

	reg := NewRegistry()
	if err := reg.Promote(b, buildOK("r"), m, ShadowReport{Clean: true}); err != nil {
		t.Fatalf("clean shadow + matching sha must promote: %v", err)
	}
	if len(reg.Active().Rules()) != 1 {
		t.Fatalf("a promoted rule must enter the active set; got %d", len(reg.Active().Rules()))
	}
	if err := NewRegistry().Promote(b, buildOK("r"), m, ShadowReport{Clean: false, Diffs: 3}); err == nil {
		t.Fatal("a non-clean shadow must reject promotion")
	}
	if err := NewRegistry().Promote(b, buildOK("r"), Manifest{SHA256: "deadbeef"}, ShadowReport{Clean: true}); err == nil {
		t.Fatal("a sha256 mismatch must reject promotion")
	}
}

// S12: a .wasm importing anything beyond env.read_state_view is rejected at promotion.
func TestManifestRejectsExtraImports(t *testing.T) {
	good := wasmBytes(t, "rule_allow_if_evidence.wasm")
	if err := NewRegistry().Promote(good, buildOK("g"), Manifest{SHA256: sha(good)}, ShadowReport{Clean: true}); err != nil {
		t.Fatalf("a module importing only env.read_state_view must pass: %v", err)
	}
	bad := wasmBytes(t, "two_imports.wasm")
	if err := NewRegistry().Promote(bad, buildOK("b"), Manifest{SHA256: sha(bad)}, ShadowReport{Clean: true}); err == nil {
		t.Fatal("a module importing beyond env.read_state_view must be rejected at promotion")
	}
}

// adversarial #2: a SECOND import section (first exactly env.read_state_view, second smuggling env.extra)
// must NOT slip the over-capable import past the gate.
func TestPromotionRejectsSecondImportSection(t *testing.T) {
	bad := wasmBytes(t, "two_import_sections.wasm")
	if err := NewRegistry().Promote(bad, buildOK("b"), Manifest{SHA256: sha(bad)}, ShadowReport{Clean: true}); err == nil {
		t.Fatal("a module with a second import section smuggling an extra import must be rejected")
	}
}

// adversarial #5: the rule that goes active must be BUILT FROM the verified bytes — a build failure rejects,
// and the build's result (not an unrelated candidate) is what is admitted.
func TestPromoteBuildsRuleFromBytes(t *testing.T) {
	good := wasmBytes(t, "rule_allow_if_evidence.wasm")
	m := Manifest{SHA256: sha(good)}
	// a build that errors -> promotion rejected even though the bytes verify.
	if err := NewRegistry().Promote(good, func([]byte) (Rule, error) { return nil, errors.New("bad bytes") }, m, ShadowReport{Clean: true}); err == nil {
		t.Fatal("a build failure must reject promotion")
	}
	// the admitted rule is the build's result.
	reg := NewRegistry()
	want := NewNativeRule("from-bytes", "agent", "memory.write.proposed", []string{"memory.observed"},
		func(RuleInput) (contract.RuleDecision, error) { return contract.RuleDecision{Verdict: contract.VerdictAllow}, nil })
	if err := reg.Promote(good, func([]byte) (Rule, error) { return want, nil }, m, ShadowReport{Clean: true}); err != nil {
		t.Fatalf("promote: %v", err)
	}
	if rs := reg.Active().Rules(); len(rs) != 1 || rs[0].ID() != "from-bytes" {
		t.Fatalf("the admitted rule must be the build result; got %+v", rs)
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

	d, _ := EdgeSnapshot(NewRuleSet(proposer)).Evaluate(RuleInput{Event: contract.Event{Type: "memory.observed"}})
	if d.Verdict == contract.VerdictPropose || d.Proposal != nil {
		t.Fatalf("edge must not propose; got verdict=%q proposal=%v", d.Verdict, d.Proposal)
	}
	if d.Verdict != contract.VerdictWarn {
		t.Fatalf("a downgraded propose must become warn; got %q", d.Verdict)
	}
	d2, _ := EdgeSnapshot(NewRuleSet(proposer, denier)).Evaluate(RuleInput{Event: contract.Event{Type: "memory.observed"}})
	if d2.Verdict != contract.VerdictDeny {
		t.Fatalf("a deny must survive the edge snapshot; got %q", d2.Verdict)
	}
}

// adversarial #3: the edge filter must strip authored intent (Proposal/Job) riding on a Warn or Deny verdict
// — an edge may refuse/warn but never author.
func TestEdgeSnapshotStripsAuthoredIntent(t *testing.T) {
	warnWithJob := NewNativeRule("wj", "agent", "memory.write.proposed", []string{"memory.observed"},
		func(RuleInput) (contract.RuleDecision, error) {
			return contract.RuleDecision{Verdict: contract.VerdictWarn, Job: &contract.JobSpec{Kind: "x", IdempotencyKey: "k"}, Proposal: &contract.ProposedEvent{Type: "memory.write.proposed"}}, nil
		})
	d, _ := EdgeSnapshot(NewRuleSet(warnWithJob)).Evaluate(RuleInput{Event: contract.Event{Type: "memory.observed"}})
	if d.Verdict != contract.VerdictWarn {
		t.Fatalf("verdict must stay warn; got %q", d.Verdict)
	}
	if d.Job != nil || d.Proposal != nil {
		t.Fatalf("the edge must strip authored intent (Job/Proposal) from a warn; got job=%v proposal=%v", d.Job, d.Proposal)
	}
}
