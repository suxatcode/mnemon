package capability

import (
	"fmt"
	"strings"

	"github.com/mnemon-dev/mnemon/harness/internal/config"
	"github.com/mnemon-dev/mnemon/harness/internal/contract"
	"github.com/mnemon-dev/mnemon/harness/internal/rule"
)

// Item is one decoded, validated candidate as a field map. The generic kind stamps id/actor/ingest_seq
// onto it and appends it to a resource's item list.
type Item map[string]any

type Limits struct {
	MaxPayloadBytes int
}

// Capability is the built-in descriptor that turns config selection into one compiled rule kind. ALL
// built-in capabilities admit a candidate through the SAME generic append-item-to-resource rule
// (appendItemRule); they differ only by DATA — the observed/proposed types, the resource kind, the
// item-list field, how a payload decodes to an Item, and the resource "header" fields a write carries
// (e.g. memory's rendered content, skill's name). A new capability is a new descriptor + config, not
// new rule code.
type Capability struct {
	Name         string
	ObservedType string
	ProposedType string
	ResourceKind contract.ResourceKind
	ItemsField   string // resource field holding the item list
	Decode       func(payload map[string]any) (Item, error)
	Header       func(items []Item) map[string]any // resource fields besides the item list + updated_by
	Limits       Limits
}

// Rule builds the capability's admission rule for one principal + resource ref. cfg may bound the
// capability (e.g. MaxPayloadBytes) without changing the compiled kind.
func (c Capability) Rule(principal contract.ActorID, ref contract.ResourceRef, cfg config.CapabilityConfig) rule.Rule {
	return appendItemRule(c, principal, ref)
}

// Builtins is the trusted registry the assembler selects from (select-only, fail-closed on unknown id).
var Builtins = map[string]Capability{
	"memory": {
		Name: "memory", ObservedType: MemoryWriteCandidateObserved, ProposedType: MemoryWriteProposed,
		ResourceKind: "memory", ItemsField: "entries", Decode: decodeMemoryItem, Header: memoryHeader,
	},
	"skill": {
		Name: "skill", ObservedType: SkillWriteCandidateObserved, ProposedType: SkillWriteProposed,
		ResourceKind: "skill", ItemsField: "declarations", Decode: decodeSkillItem, Header: skillHeader,
	},
	// note is a 3rd capability that reuses the generic kind via DATA only — no new rule code. It exists
	// to prove a new capability stands up by descriptor + config alone (Phase 2 acceptance).
	"note": {
		Name: "note", ObservedType: "note.write_candidate.observed", ProposedType: "note.write.proposed",
		ResourceKind: "note", ItemsField: "items", Decode: decodeNoteItem, Header: noteHeader,
	},
}

func decodeNoteItem(payload map[string]any) (Item, error) {
	text := strings.TrimSpace(stringField(payload, "text"))
	if text == "" {
		return nil, fmt.Errorf("note candidate denied: empty text")
	}
	if containsSecretLikeContent(text) || containsPromptInjectionShape(text) {
		return nil, fmt.Errorf("note candidate denied: unsafe content")
	}
	return Item{"text": text}, nil
}

func noteHeader(items []Item) map[string]any {
	lines := []string{"# Notes"}
	for _, it := range items {
		lines = append(lines, "- "+itemString(it, "text"))
	}
	return map[string]any{"content": strings.Join(lines, "\n")}
}

// appendItemRule is the ONE generic kind: decode the candidate to an Item, stamp trusted id/actor/seq,
// append it to the resource's item list, and propose a write carrying the item list + the capability's
// header fields + updated_by. It only acts on events from its own principal.
func appendItemRule(c Capability, principal contract.ActorID, ref contract.ResourceRef) rule.Rule {
	return rule.NewNativeRule("local-"+c.Name+"-admission:"+string(principal), principal, c.ProposedType, ObservedTypeAndAliases(c.ObservedType),
		func(in rule.RuleInput) (contract.RuleDecision, error) {
			if in.Event.Actor != principal {
				return contract.RuleDecision{Verdict: contract.VerdictAllow}, nil
			}
			item, err := c.Decode(in.Event.Payload)
			if err != nil {
				return contract.RuleDecision{Verdict: contract.VerdictDeny, Reasons: []string{err.Error()}}, nil
			}
			item["id"] = itemID(in.Event.Actor, in.Event.IngestSeq)
			item["actor"] = string(in.Event.Actor)
			item["ingest_seq"] = in.Event.IngestSeq
			version, fields := resourceFromProjection(in.View, ref)
			items := append(itemsFromFields(fields, c.ItemsField), item)
			newFields := map[string]any{c.ItemsField: items, "updated_by": string(in.Event.Actor)}
			for k, v := range c.Header(items) {
				newFields[k] = v
			}
			write := contract.ResourceWrite{Ref: ref, Kind: contract.OpCreate, Fields: newFields}
			if version > 0 {
				write.Kind = contract.OpUpdate
				write.BasedOn = version
			}
			return contract.RuleDecision{Verdict: contract.VerdictPropose, Proposal: &contract.ProposedEvent{
				Type:    c.ProposedType,
				Payload: map[string]any{"writes": []contract.ResourceWrite{write}},
			}}, nil
		})
}

func itemsFromFields(fields map[string]any, field string) []Item {
	if fields == nil {
		return nil
	}
	raw, ok := fields[field].([]any)
	if !ok {
		return nil
	}
	items := make([]Item, 0, len(raw))
	for _, r := range raw {
		m, ok := r.(map[string]any)
		if !ok {
			continue
		}
		if id, _ := m["id"].(string); id != "" {
			items = append(items, Item(m))
		}
	}
	return items
}

func itemID(actor contract.ActorID, ingestSeq int64) string {
	return memoryEntryID(actor, ingestSeq)
}

// ---- memory descriptor data ----

func decodeMemoryItem(payload map[string]any) (Item, error) {
	c, err := decodeMemoryCandidate(payload)
	if err != nil {
		return nil, err
	}
	item := Item{"content": c.Content, "source": c.Source, "confidence": c.Confidence}
	if len(c.Tags) > 0 {
		item["tags"] = c.Tags
	}
	return item, nil
}

func memoryHeader(items []Item) map[string]any {
	return map[string]any{"content": renderMemoryItems(items)}
}

func renderMemoryItems(items []Item) string {
	lines := []string{"# Local Memory"}
	for _, it := range items {
		meta := []string{"id: " + itemString(it, "id"), "source: " + itemString(it, "source"), "confidence: " + itemString(it, "confidence")}
		if tags := itemStrings(it, "tags"); len(tags) > 0 {
			meta = append(meta, "tags: "+strings.Join(tags, ","))
		}
		lines = append(lines, "- "+itemString(it, "content")+" ("+strings.Join(meta, "; ")+")")
	}
	return strings.Join(lines, "\n")
}

// ---- skill descriptor data ----

func decodeSkillItem(payload map[string]any) (Item, error) {
	c, err := decodeSkillCandidate(payload)
	if err != nil {
		return nil, err
	}
	return Item{
		"skill_id": c.SkillID, "name": c.Name, "status": c.Status,
		"content": c.Content, "source": c.Source, "confidence": c.Confidence,
	}, nil
}

func skillHeader(items []Item) map[string]any {
	return map[string]any{"name": "project"}
}

func itemString(it Item, key string) string {
	if s, ok := it[key].(string); ok {
		return s
	}
	return ""
}

func itemStrings(it Item, key string) []string {
	switch raw := it[key].(type) {
	case []string:
		return raw
	case []any:
		out := make([]string, 0, len(raw))
		for _, v := range raw {
			if s, ok := v.(string); ok {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}
