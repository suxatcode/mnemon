package capability

import (
	"fmt"

	"github.com/mnemon-dev/mnemon/harness/internal/contract"
	"github.com/mnemon-dev/mnemon/harness/internal/rule"
)

// SyncImportSkippedObserved is the observation a sync puller ingests for a pulled commit whose
// resource kind has no import mapping (v1.1 #4): instead of a silent continue, the skip enters the
// canonical log exactly-once (ExternalID = the six-part pull key + ":skipped") and the deny rule
// below turns it into a durable sync.diagnostic via the existing pre-gate. Payload: {kind,
// origin_replica_id, local_decision_id, remote_id}.
const SyncImportSkippedObserved = "sync.import_skipped.observed"

// SyncImportSkippedRule is the legal diagnostic mechanism for skipped kinds: it Handles ONLY the
// skipped observation, gates on the sync import principal (foreign events pass through), and always
// denies with a reason naming the kind — the deny is what produces the durable *.diagnostic (S7);
// no write, no proposal.
func SyncImportSkippedRule(principal contract.ActorID) rule.Rule {
	return rule.NewNativeRule("sync-import-skipped:"+string(principal), principal, "", []string{SyncImportSkippedObserved},
		func(in rule.RuleInput) (contract.RuleDecision, error) {
			if in.Event.Actor != principal {
				return contract.RuleDecision{Verdict: contract.VerdictAllow}, nil
			}
			kind, _ := in.Event.Payload["kind"].(string)
			if kind == "" {
				kind = "unknown"
			}
			return contract.RuleDecision{
				Verdict: contract.VerdictDeny,
				Reasons: []string{fmt.Sprintf("sync import skipped: resource kind %q has no import mapping on this replica", kind)},
			}, nil
		})
}
