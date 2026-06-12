package capability

import (
	"fmt"
	"strings"

	"github.com/mnemon-dev/mnemon/harness/internal/contract"
	"github.com/mnemon-dev/mnemon/harness/internal/rule"
)

// RiskEvidenceGate is the mid-risk governance gate (P3 three-tier risk): a candidate for this
// capability's kind must carry a non-empty `evidence` field, else it is DENIED with a durable
// diagnostic. It is a SEPARATE rule that handles the same observed type as the admission rule; when
// it denies, rule.Evaluate's deny-priority reduction makes the deny outrank the admission rule's
// propose, so the write is refused — no new kernel verdict or held state (M1 review correction). It
// gates on the cap's principal (a foreign principal's event passes through) and emits no proposal.
//
// High-risk (operator-only) gating is deferred to P3e, where its consumer (the high-risk loopdef
// kind) and its principal model (the human@owner operator binding, G9) are designed together — a
// high-risk gate without an operator principal to exempt would make a kind ungovernable.
func RiskEvidenceGate(cap Capability, principal contract.ActorID) rule.Rule {
	return rule.NewNativeRule("risk-evidence:"+cap.Name+":"+string(principal), principal, "", []string{cap.ObservedType},
		func(in rule.RuleInput) (contract.RuleDecision, error) {
			if in.Event.Actor != principal {
				return contract.RuleDecision{Verdict: contract.VerdictAllow}, nil
			}
			if strings.TrimSpace(stringField(in.Event.Payload, "evidence")) == "" {
				return contract.RuleDecision{Verdict: contract.VerdictDeny, Reasons: []string{
					fmt.Sprintf("mid-risk %s candidate denied: evidence is required", cap.ResourceKind)}}, nil
			}
			return contract.RuleDecision{Verdict: contract.VerdictAllow}, nil
		})
}
