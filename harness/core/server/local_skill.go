package server

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/mnemon-dev/mnemon/harness/core/contract"
	"github.com/mnemon-dev/mnemon/harness/core/projection"
	"github.com/mnemon-dev/mnemon/harness/core/rule"
)

const (
	SkillWriteCandidateObserved = "skill.write_candidate_observed"
	SkillWriteProposed          = "skill.write.proposed"
)

var localProjectSkillRef = contract.ResourceRef{Kind: "skill", ID: "project"}

func LocalSkillRules(bindings []ChannelBinding) []rule.Rule {
	var rules []rule.Rule
	for _, b := range bindings {
		if !b.Allows(VerbObserve) || !b.AllowsObservedType(SkillWriteCandidateObserved) {
			continue
		}
		ref, ok := skillRefForBinding(b)
		if !ok {
			continue
		}
		rules = append(rules, skillAdmissionRule(b.Principal, ref))
	}
	return rules
}

func skillRefForBinding(b ChannelBinding) (contract.ResourceRef, bool) {
	for _, ref := range b.SubscriptionScope {
		if ref == localProjectSkillRef {
			return ref, true
		}
	}
	for _, ref := range b.SubscriptionScope {
		if ref.Kind == "skill" {
			return ref, true
		}
	}
	return contract.ResourceRef{}, false
}

func skillAdmissionRule(principal contract.ActorID, ref contract.ResourceRef) rule.Rule {
	return rule.NewNativeRule("local-skill-admission:"+string(principal), principal, SkillWriteProposed, []string{SkillWriteCandidateObserved},
		func(in rule.RuleInput) (contract.RuleDecision, error) {
			if in.Event.Actor != principal {
				return contract.RuleDecision{Verdict: contract.VerdictAllow}, nil
			}
			candidate, err := decodeSkillCandidate(in.Event.Payload)
			if err != nil {
				return contract.RuleDecision{Verdict: contract.VerdictDeny, Reasons: []string{err.Error()}}, nil
			}
			version, fields := skillResourceFromProjection(in.View, ref)
			// Skill lifecycle changes are append-only declarations. A later "stale" or
			// "archived" declaration records the transition without rewriting prior history.
			declarations := append(skillDeclarationsFromFields(fields), skillDeclaration{
				ID:         skillDeclarationID(in.Event.Actor, in.Event.IngestSeq),
				SkillID:    candidate.SkillID,
				Name:       candidate.Name,
				Status:     candidate.Status,
				Content:    candidate.Content,
				Source:     candidate.Source,
				Confidence: candidate.Confidence,
				Actor:      string(in.Event.Actor),
				IngestSeq:  in.Event.IngestSeq,
			})
			newFields := map[string]any{
				"name":         "project",
				"declarations": declarations,
				"updated_by":   string(in.Event.Actor),
			}
			write := contract.ResourceWrite{Ref: ref, Kind: contract.OpCreate, Fields: newFields}
			if version > 0 {
				write.Kind = contract.OpUpdate
				write.BasedOn = version
			}
			return contract.RuleDecision{Verdict: contract.VerdictPropose, Proposal: &contract.ProposedEvent{
				Type:    SkillWriteProposed,
				Payload: map[string]any{"writes": []contract.ResourceWrite{write}},
			}}, nil
		})
}

type skillCandidate struct {
	SkillID    string
	Name       string
	Status     string
	Content    string
	Source     string
	Confidence string
}

type skillDeclaration struct {
	ID         string `json:"id"`
	SkillID    string `json:"skill_id"`
	Name       string `json:"name"`
	Status     string `json:"status"`
	Content    string `json:"content,omitempty"`
	Source     string `json:"source"`
	Confidence string `json:"confidence"`
	Actor      string `json:"actor"`
	IngestSeq  int64  `json:"ingest_seq"`
}

func decodeSkillCandidate(payload map[string]any) (skillCandidate, error) {
	skillID := strings.TrimSpace(stringField(payload, "skill_id"))
	if skillID == "" {
		return skillCandidate{}, fmt.Errorf("skill candidate denied: missing skill_id")
	}
	if !validSkillID(skillID) {
		return skillCandidate{}, fmt.Errorf("skill candidate denied: invalid skill_id")
	}
	name := strings.TrimSpace(stringField(payload, "name"))
	if name == "" {
		name = skillID
	}
	status := strings.TrimSpace(stringField(payload, "status"))
	if status == "" {
		status = "active"
	}
	if status != "active" && status != "stale" && status != "archived" {
		return skillCandidate{}, fmt.Errorf("skill candidate denied: invalid status")
	}
	source := strings.TrimSpace(stringField(payload, "source"))
	if source == "" {
		return skillCandidate{}, fmt.Errorf("skill candidate denied: missing source")
	}
	confidence := strings.TrimSpace(stringField(payload, "confidence"))
	if confidence == "" {
		return skillCandidate{}, fmt.Errorf("skill candidate denied: missing confidence")
	}
	content := strings.TrimSpace(stringField(payload, "content"))
	if containsSecretLikeContent(content) || containsPromptInjectionShape(content) {
		return skillCandidate{}, fmt.Errorf("skill candidate denied: unsafe content")
	}
	return skillCandidate{SkillID: skillID, Name: name, Status: status, Content: content, Source: source, Confidence: confidence}, nil
}

func validSkillID(s string) bool {
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			continue
		}
		return false
	}
	return true
}

func skillResourceFromProjection(view projection.Projection, ref contract.ResourceRef) (contract.Version, map[string]any) {
	return resourceFromProjection(view, ref)
}

func skillDeclarationsFromFields(fields map[string]any) []skillDeclaration {
	if fields == nil {
		return nil
	}
	raw, ok := fields["declarations"].([]any)
	if !ok {
		return nil
	}
	declarations := make([]skillDeclaration, 0, len(raw))
	for _, item := range raw {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		decl := skillDeclaration{
			ID:         stringMapField(m, "id"),
			SkillID:    stringMapField(m, "skill_id"),
			Name:       stringMapField(m, "name"),
			Status:     stringMapField(m, "status"),
			Content:    stringMapField(m, "content"),
			Source:     stringMapField(m, "source"),
			Confidence: stringMapField(m, "confidence"),
			Actor:      stringMapField(m, "actor"),
			IngestSeq:  int64MapField(m, "ingest_seq"),
		}
		if decl.ID != "" && decl.SkillID != "" && decl.Name != "" {
			declarations = append(declarations, decl)
		}
	}
	return declarations
}

func skillDeclarationID(actor contract.ActorID, ingestSeq int64) string {
	return "local/" + sanitizeEntryIDPart(string(actor)) + "/" + strconv.FormatInt(ingestSeq, 10)
}
