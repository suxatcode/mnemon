// Package replay re-derives decisions from the canonical event log on a throwaway kernel (event-sourcing
// purity, S8): replay reads the log only, never advances a live cursor or writes a live store, and its
// determinism is established by FIELD-MASKING the dynamic decision fields (DecisionID/AppliedAt) before any
// diff (D1) — production decisions keep their real uuid/time. replay imports rule (one-way, D11).
package replay

import (
	"encoding/json"
	"sort"
	"strings"

	"github.com/mnemon-dev/mnemon/harness/core/contract"
	"github.com/mnemon-dev/mnemon/harness/core/kernel"
	"github.com/mnemon-dev/mnemon/harness/core/projection"
	"github.com/mnemon-dev/mnemon/harness/core/reconcile"
	"github.com/mnemon-dev/mnemon/harness/core/rule"
)

// canonicalModes is the fixed policy replay reconciles under; it matches the server's loop modes so a replay
// reproduces the live decisions deterministically.
var canonicalModes = contract.Modes{Conflict: contract.ConflictRebase, Isolation: contract.IsolationProjectionReadSet, Authz: contract.AuthzStrict}

func isProposal(ev contract.Event) bool { return strings.HasSuffix(ev.Type, ".proposed") }

// permissiveAuthority lets every actor that appears in the events write every catalog kind, so replay does
// not introduce authz rejections the live run did not have (the live authority is reproduced by the events
// themselves having been accepted; replay only re-derives, it does not re-police).
func permissiveAuthority(events []contract.Event) kernel.AuthorityRules {
	var kinds []contract.ResourceKind
	for k := range contract.KindCatalog {
		kinds = append(kinds, k)
	}
	allow := map[contract.ActorID][]contract.ResourceKind{}
	for _, ev := range events {
		if _, ok := allow[ev.Actor]; !ok {
			allow[ev.Actor] = kinds
		}
	}
	return kernel.AuthorityRules{Allow: allow}
}

// Replay re-derives the decisions by reconciling the *.proposed events of the log over a FRESH :memory:
// kernel. It is a pure function of the events (no live store), reproducing the live decisions up to the
// masked dynamic fields. The candidate ruleset is retained for signature symmetry with Shadow — pure replay
// needs no policy because the logged proposals are authoritative (event-sourcing).
func Replay(events []contract.Event, candidate rule.RuleSet) []contract.Decision {
	return drive(events)
}

// Shadow asks the governance question "would promoting this candidate rule set change behavior?" by RE-RUNNING
// both policies' rules over the OBSERVED events of the log and diffing their rule decisions (S8). This is the
// faithful model: a rule handles OBSERVED events and EMITS proposals/denies/jobs — so the candidate's behavior
// change lives in observed->decision, NOT in re-reconciling the already-minted *.proposed events (the prior
// model never ran the candidate's rules at all, so every real rule change passed Clean).
//
// It seeds a throwaway kernel with the canonical state (the logged proposals) so each rule sees realistic
// resource state, then for every observed event evaluates live and candidate against the same scoped view and
// compares verdict + proposal (type + payload) + job + trusted origin actor. The seeded kernel is NEVER mutated
// by the comparison (read-only, S8). It reports diffs, never pass/fail (the operator gates promotion on Clean).
func Shadow(events []contract.Event, subs map[contract.ActorID]contract.Subscription, live, candidate rule.RuleSet) rule.ShadowReport {
	s, err := kernel.OpenStore(":memory:")
	if err != nil {
		return rule.ShadowReport{}
	}
	defer s.Close()
	k := kernel.NewKernel(s, kernel.DefaultSchemaGuard(), permissiveAuthority(events))
	r := reconcile.NewReconciler(s, k)
	for _, ev := range events {
		if _, err := s.AppendEvent(ev); err != nil {
			continue
		}
	}
	r.RunOnce(canonicalModes) // apply the logged proposals -> canonical resource state for the views below

	diffs := 0
	for _, ev := range events {
		if isProposal(ev) || strings.HasSuffix(ev.Type, ".diagnostic") {
			continue // only OBSERVED events drive the rules
		}
		view := projection.ScopedView(s, subs[ev.Actor])
		in := rule.RuleInput{Event: ev, View: view}
		ld, _ := live.Evaluate(in)
		cd, _ := candidate.Evaluate(in)
		if canonicalRuleDecision(ld) != canonicalRuleDecision(cd) {
			diffs++
		}
	}
	return rule.ShadowReport{Clean: diffs == 0, Diffs: diffs}
}

// canonicalRuleDecision serializes the behaviorally-meaningful fields of a rule decision (verdict, proposal
// type+payload, job, and the trusted origin actor) to a stable string for comparison. Advisory Reasons are
// excluded (they do not change behavior). json.Marshal sorts map keys, so equal payloads compare equal.
func canonicalRuleDecision(d contract.RuleDecision) string {
	b, _ := json.Marshal(struct {
		Verdict  contract.RuleVerdict
		Proposal *contract.ProposedEvent
		Job      *contract.JobSpec
		Actor    contract.ActorID
	}{d.Verdict, d.Proposal, d.Job, d.ProposalActor})
	return string(b)
}

// drive replays the events on a throwaway kernel and returns the reconciler's decisions (event-sourcing
// reproduce-from-log: the logged proposals are authoritative). It never touches a live store/cursor.
func drive(events []contract.Event) []contract.Decision {
	s, err := kernel.OpenStore(":memory:")
	if err != nil {
		return nil
	}
	defer s.Close()
	k := kernel.NewKernel(s, kernel.DefaultSchemaGuard(), permissiveAuthority(events))
	r := reconcile.NewReconciler(s, k)
	for _, ev := range events {
		if _, err := s.AppendEvent(ev); err != nil {
			continue
		}
	}
	return r.RunOnce(canonicalModes)
}

// maskDynamic zeros the per-run dynamic fields and sorts the order-insensitive slices so two decisions for
// the same logical outcome compare equal regardless of uuid/time/ordering (D1).
func maskDynamic(d contract.Decision) contract.Decision {
	d.DecisionID = ""
	d.AppliedAt = ""
	sort.Slice(d.Conflicts, func(i, j int) bool {
		return string(d.Conflicts[i].Ref.Kind)+string(d.Conflicts[i].Ref.ID) < string(d.Conflicts[j].Ref.Kind)+string(d.Conflicts[j].Ref.ID)
	})
	sort.Slice(d.NewVersions, func(i, j int) bool {
		return string(d.NewVersions[i].Ref.Kind)+string(d.NewVersions[i].Ref.ID) < string(d.NewVersions[j].Ref.Kind)+string(d.NewVersions[j].Ref.ID)
	})
	return d
}
