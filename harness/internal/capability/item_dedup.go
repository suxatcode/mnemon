package capability

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	"github.com/suxatcode/mnemon/harness/internal/contract"
	"github.com/suxatcode/mnemon/harness/internal/rule"
)

// itemDedupImport is the "item-dedup" remote-import strategy (capability-spec v2 §Sync): the GENERIC
// append-merge for a directory-of-items kind (§577). It merges a remote commit's items into the
// resource's item list BY ID, preserving EVERY item field — unlike entry-dedup (shaped for memory's
// `content`) and declaration-dedup (shaped for skill's `declarations`), it makes no assumption about
// the item's domain fields, so an arbitrary declared kind (the coordination kinds) syncs without
// losing its fields (assignment's scope/ttl/assignee, etc.). Item ids are replica-specific
// (actor+ingest_seq stamped at admission), so cross-replica items never collide; a
// same-id/different-content divergence is rejected (I15, defensive). The merged resource header is
// re-derived from the capability's OWN render, never hardcoded.
func itemDedupImport(cap Capability, in rule.RuleInput) (contract.RuleDecision, error) {
	commit, err := decodeRemoteCommit(in.Event.Payload)
	if err != nil {
		return contract.RuleDecision{Verdict: contract.VerdictDeny, Reasons: []string{err.Error()}}, nil
	}
	if commit.ResourceRef.Kind != cap.ResourceKind {
		return contract.RuleDecision{Verdict: contract.VerdictDeny, Reasons: []string{"remote import denied: resource kind does not match the importing capability"}}, nil
	}
	incoming := itemsFromFields(commit.Fields, cap.ItemsField)
	if len(incoming) == 0 {
		return contract.RuleDecision{Verdict: contract.VerdictDeny, Reasons: []string{"remote import denied: no items"}}, nil
	}
	version, fields := resourceFromProjection(in.View, commit.ResourceRef)
	existing := itemsFromFields(fields, cap.ItemsField)
	byID := make(map[string]Item, len(existing))
	for _, it := range existing {
		byID[stringMapField(it, "id")] = it
	}
	var additions []Item
	for _, it := range incoming {
		id := stringMapField(it, "id")
		if cur, ok := byID[id]; ok {
			if !reflect.DeepEqual(cur, it) {
				return contract.RuleDecision{Verdict: contract.VerdictDeny, Reasons: []string{"remote import conflict: item " + id + " already exists with different content"}}, nil
			}
			continue
		}
		additions = append(additions, it)
	}
	if len(additions) == 0 {
		return contract.RuleDecision{Verdict: contract.VerdictAllow}, nil
	}
	items := append(append([]Item(nil), existing...), additions...)
	newFields := map[string]any{cap.ItemsField: items, "updated_by": string(in.Event.Actor)}
	for k, v := range cap.Header(items) {
		newFields[k] = v
	}
	write := contract.ResourceWrite{Ref: commit.ResourceRef, Kind: contract.OpCreate, Fields: newFields}
	if version > 0 {
		write.Kind = contract.OpUpdate
		write.BasedOn = version
	}
	return contract.RuleDecision{Verdict: contract.VerdictPropose, Proposal: &contract.ProposedEvent{
		Type:    cap.ProposedType,
		Payload: map[string]any{"writes": []contract.ResourceWrite{write}},
	}}, nil
}

// decodeRemoteCommit decodes a remote LocalCommit from an import event payload (the kind-agnostic
// form of decodeRemoteMemoryCommit/decodeRemoteSkillCommit, used by the generic item-dedup strategy).
func decodeRemoteCommit(payload map[string]any) (contract.LocalCommit, error) {
	raw, ok := payload["commit"]
	if !ok {
		return contract.LocalCommit{}, fmt.Errorf("remote import denied: missing commit")
	}
	data, err := json.Marshal(raw)
	if err != nil {
		return contract.LocalCommit{}, fmt.Errorf("remote import denied: encode commit: %w", err)
	}
	var commit contract.LocalCommit
	if err := json.Unmarshal(data, &commit); err != nil {
		return contract.LocalCommit{}, fmt.Errorf("remote import denied: decode commit: %w", err)
	}
	if strings.TrimSpace(commit.OriginReplicaID) == "" || strings.TrimSpace(commit.LocalDecisionID) == "" {
		return contract.LocalCommit{}, fmt.Errorf("remote import denied: missing provenance")
	}
	return commit, nil
}
