package capability

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	"github.com/mnemon-dev/mnemon/harness/internal/contract"
	"github.com/mnemon-dev/mnemon/harness/internal/projection"
	"github.com/mnemon-dev/mnemon/harness/internal/rule"
)

const (
	SkillWriteCandidateObserved = "skill.write_candidate.observed"
	RemoteSkillCommitObserved   = "remote.skill.commit_observed"
	SkillWriteProposed          = "skill.write.proposed"
)

// SkillAdmissionRule admits an append-only skill declaration from one authenticated principal. It is
// the skill descriptor over the generic kind.
func SkillAdmissionRule(principal contract.ActorID, ref contract.ResourceRef) rule.Rule {
	return Builtins["skill"].Rule(principal, ref, Limits{})
}

// RemoteSkillImportRule admits a remote skill commit for the sync import actor, merging non-conflicting
// declarations into the local skill resource.
func RemoteSkillImportRule(principal contract.ActorID) rule.Rule {
	return rule.NewNativeRule("remote-skill-import:"+string(principal), principal, SkillWriteProposed, []string{RemoteSkillCommitObserved},
		func(in rule.RuleInput) (contract.RuleDecision, error) {
			if in.Event.Actor != principal {
				return contract.RuleDecision{Verdict: contract.VerdictAllow}, nil
			}
			commit, err := decodeRemoteSkillCommit(in.Event.Payload)
			if err != nil {
				return contract.RuleDecision{Verdict: contract.VerdictDeny, Reasons: []string{err.Error()}}, nil
			}
			if commit.ResourceRef.Kind != "skill" {
				return contract.RuleDecision{Verdict: contract.VerdictDeny, Reasons: []string{"remote skill import denied: non-skill resource"}}, nil
			}
			incoming := skillDeclarationsFromFields(commit.Fields)
			if len(incoming) == 0 {
				return contract.RuleDecision{Verdict: contract.VerdictDeny, Reasons: []string{"remote skill import denied: no skill declarations"}}, nil
			}
			for _, decl := range incoming {
				if reason := validateRemoteSkillDeclaration(decl); reason != "" {
					return contract.RuleDecision{Verdict: contract.VerdictDeny, Reasons: []string{reason}}, nil
				}
			}
			version, fields := skillResourceFromProjection(in.View, commit.ResourceRef)
			existing := skillDeclarationsFromFields(fields)
			byID := make(map[string]skillDeclaration, len(existing))
			for _, decl := range existing {
				byID[decl.ID] = decl
			}
			var additions []skillDeclaration
			for _, decl := range incoming {
				if current, ok := byID[decl.ID]; ok {
					if !sameSkillDeclaration(current, decl) {
						return contract.RuleDecision{Verdict: contract.VerdictDeny, Reasons: []string{"remote skill conflict: declaration " + decl.ID + " already exists with different content"}}, nil
					}
					continue
				}
				additions = append(additions, decl)
			}
			if len(additions) == 0 {
				return contract.RuleDecision{Verdict: contract.VerdictAllow}, nil
			}
			declarations := append(append([]skillDeclaration(nil), existing...), additions...)
			newFields := map[string]any{
				"name":         "project",
				"declarations": declarations,
				"updated_by":   string(in.Event.Actor),
			}
			write := contract.ResourceWrite{Ref: commit.ResourceRef, Kind: contract.OpCreate, Fields: newFields}
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

func decodeRemoteSkillCommit(payload map[string]any) (contract.LocalCommit, error) {
	raw, ok := payload["commit"]
	if !ok {
		return contract.LocalCommit{}, fmt.Errorf("remote skill import denied: missing commit")
	}
	data, err := json.Marshal(raw)
	if err != nil {
		return contract.LocalCommit{}, fmt.Errorf("remote skill import denied: encode commit: %w", err)
	}
	var commit contract.LocalCommit
	if err := json.Unmarshal(data, &commit); err != nil {
		return contract.LocalCommit{}, fmt.Errorf("remote skill import denied: decode commit: %w", err)
	}
	if strings.TrimSpace(commit.OriginReplicaID) == "" || strings.TrimSpace(commit.LocalDecisionID) == "" {
		return contract.LocalCommit{}, fmt.Errorf("remote skill import denied: missing provenance")
	}
	return commit, nil
}

func validateRemoteSkillDeclaration(decl skillDeclaration) string {
	if !validSkillID(decl.SkillID) {
		return "remote skill import denied: invalid skill_id"
	}
	if decl.Status != "active" && decl.Status != "stale" && decl.Status != "archived" {
		return "remote skill import denied: invalid status"
	}
	if containsSecretLikeContent(decl.Content) || containsPromptInjectionShape(decl.Content) {
		return "remote skill import denied: unsafe content"
	}
	return ""
}

func sameSkillDeclaration(a, b skillDeclaration) bool {
	return reflect.DeepEqual(a, b)
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
