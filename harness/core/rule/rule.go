// Package rule is the server-side admission-controller pre-gate (D4). A rule observes a typed input (the
// triggering event + the projection it was dispatched on) and returns a rich RuleDecision — it PROPOSES or
// ENQUEUES, but never writes (S12: a rule holds no Store/Kernel). The kernel stays the only canonical writer.
package rule

import (
	"github.com/mnemon-dev/mnemon/harness/core/contract"
	"github.com/mnemon-dev/mnemon/harness/core/projection"
)

// RuleInput is the typed, read-only input to a rule: the triggering event and the scoped projection it was
// dispatched on. (Held by rule, not contract, so the projection.Projection dependency lives off the wire — D11.)
type RuleInput struct {
	Event contract.Event
	View  projection.Projection
}

// Rule is one admission rule. Actor()/Emits() let the server synthesize a trusted ResolvedBinding for the
// bridge when a rule proposes (the proposal's write identity + authorized emit type come from the rule, never
// the payload). Handles gates which event types it sees.
type Rule interface {
	ID() string
	Actor() contract.ActorID
	Emits() string
	Handles(eventType string) bool
	Evaluate(RuleInput) (contract.RuleDecision, error)
}

// NativeRule is a Go-implemented rule (the default backend, D2). The wazero WASM backend (P5) implements the
// same Rule interface behind the same seat.
type NativeRule struct {
	id      string
	actor   contract.ActorID
	emits   string
	handles map[string]bool
	fn      func(RuleInput) (contract.RuleDecision, error)
}

func NewNativeRule(id string, actor contract.ActorID, emits string, handles []string, fn func(RuleInput) (contract.RuleDecision, error)) NativeRule {
	h := make(map[string]bool, len(handles))
	for _, t := range handles {
		h[t] = true
	}
	return NativeRule{id: id, actor: actor, emits: emits, handles: h, fn: fn}
}

func (r NativeRule) ID() string                { return r.id }
func (r NativeRule) Actor() contract.ActorID   { return r.actor }
func (r NativeRule) Emits() string             { return r.emits }
func (r NativeRule) Handles(t string) bool     { return r.handles[t] }
func (r NativeRule) Evaluate(in RuleInput) (contract.RuleDecision, error) {
	d, err := r.fn(in)
	if err != nil {
		return contract.RuleDecision{}, err
	}
	// A propose without an explicit Type is stamped with the rule's authorized emit type, so the server can
	// match the proposal back to its originating rule (for the trusted bridge binding) deterministically.
	if d.Verdict == contract.VerdictPropose && d.Proposal != nil && d.Proposal.Type == "" {
		d.Proposal.Type = r.emits
	}
	return d, nil
}

// RuleSet is an ordered set of rules reduced by a DENY-PRIORITY policy.
type RuleSet struct{ rules []Rule }

func NewRuleSet(rules ...Rule) RuleSet { return RuleSet{rules: rules} }

// Rules exposes the member rules so the server can find the rule that produced a proposal (to stamp the
// trusted bridge binding from its Actor()/Emits()).
func (rs RuleSet) Rules() []Rule { return rs.rules }

// verdictRank orders the reduction: deny beats everything; enqueue_job/request_evidence beat propose/warn/
// allow; warn beats allow. The highest-ranked verdict among the handling rules wins.
var verdictRank = map[contract.RuleVerdict]int{
	contract.VerdictAllow:           0,
	contract.VerdictWarn:            1,
	contract.VerdictPropose:         2,
	contract.VerdictRequestEvidence: 3,
	contract.VerdictEnqueueJob:      4,
	contract.VerdictDeny:            5,
}

// Evaluate reduces every handling rule into one decision (deny-priority) plus diagnostics. An erroring rule
// contributes ZERO intent and exactly one Diagnostic naming it (S7 / Invariant #13). Warn reasons attach to
// the final decision; the first proposal/job for the winning verdict is carried.
func (rs RuleSet) Evaluate(in RuleInput) (contract.RuleDecision, []contract.Diagnostic) {
	out := contract.RuleDecision{Verdict: contract.VerdictAllow}
	var diags []contract.Diagnostic
	var reasons []string
	for _, r := range rs.rules {
		if !r.Handles(in.Event.Type) {
			continue
		}
		d, err := r.Evaluate(in)
		if err != nil {
			diags = append(diags, contract.Diagnostic{Stage: "rule", Reason: err.Error(), Ref: r.ID()})
			continue
		}
		reasons = append(reasons, d.Reasons...)
		if verdictRank[d.Verdict] > verdictRank[out.Verdict] {
			out.Verdict = d.Verdict
		}
		if d.Verdict == contract.VerdictPropose && out.Proposal == nil {
			out.Proposal = d.Proposal
		}
		if d.Verdict == contract.VerdictEnqueueJob && out.Job == nil {
			out.Job = d.Job
		}
	}
	out.Reasons = reasons
	return out, diags
}
