// Package capability holds the built-in admission rules (the pure leaf): given an Event + Projection
// it returns a RuleDecision, never writing. It imports rule/projection/contract only — binding->rule
// translation and runtime wiring live in app. Memory + skill are the two P0 capabilities.
package capability

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/mnemon-dev/mnemon/harness/internal/contract"
	"github.com/mnemon-dev/mnemon/harness/internal/projection"
	"github.com/mnemon-dev/mnemon/harness/internal/rule"
)

const (
	MemoryWriteCandidateObserved = "memory.write_candidate.observed"
	RemoteMemoryCommitObserved   = "remote.memory.commit_observed"
	MemoryWriteProposed          = "memory.write.proposed"
)

// MemoryAdmissionRule admits a memory write candidate from one authenticated principal, proposing an
// append to the principal's memory resource. It only acts on events from its own principal.
func MemoryAdmissionRule(principal contract.ActorID, ref contract.ResourceRef) rule.Rule {
	return rule.NewNativeRule("local-memory-admission:"+string(principal), principal, MemoryWriteProposed, ObservedTypeAndAliases(MemoryWriteCandidateObserved),
		func(in rule.RuleInput) (contract.RuleDecision, error) {
			if in.Event.Actor != principal {
				return contract.RuleDecision{Verdict: contract.VerdictAllow}, nil
			}
			candidate, err := decodeMemoryCandidate(in.Event.Payload)
			if err != nil {
				return contract.RuleDecision{Verdict: contract.VerdictDeny, Reasons: []string{err.Error()}}, nil
			}
			version, fields := resourceFromProjection(in.View, ref)
			entry := memoryEntry{
				ID:         memoryEntryID(in.Event.Actor, in.Event.IngestSeq),
				Content:    candidate.Content,
				Source:     candidate.Source,
				Confidence: candidate.Confidence,
				Tags:       candidate.Tags,
				Actor:      string(in.Event.Actor),
				IngestSeq:  in.Event.IngestSeq,
			}
			entries := append(memoryEntriesFromFields(fields), entry)
			newFields := map[string]any{
				"content":    renderMemoryContent(entries),
				"entries":    entries,
				"updated_by": string(in.Event.Actor),
			}
			write := contract.ResourceWrite{Ref: ref, Kind: contract.OpCreate, Fields: newFields}
			if version > 0 {
				write.Kind = contract.OpUpdate
				write.BasedOn = version
			}
			return contract.RuleDecision{Verdict: contract.VerdictPropose, Proposal: &contract.ProposedEvent{
				Type:    MemoryWriteProposed,
				Payload: map[string]any{"writes": []contract.ResourceWrite{write}},
			}}, nil
		})
}

// RemoteMemoryImportRule admits a remote memory commit for the sync import actor, merging non-conflicting
// entries into the local memory resource.
func RemoteMemoryImportRule(principal contract.ActorID) rule.Rule {
	return rule.NewNativeRule("remote-memory-import:"+string(principal), principal, MemoryWriteProposed, []string{RemoteMemoryCommitObserved},
		func(in rule.RuleInput) (contract.RuleDecision, error) {
			if in.Event.Actor != principal {
				return contract.RuleDecision{Verdict: contract.VerdictAllow}, nil
			}
			commit, err := decodeRemoteMemoryCommit(in.Event.Payload)
			if err != nil {
				return contract.RuleDecision{Verdict: contract.VerdictDeny, Reasons: []string{err.Error()}}, nil
			}
			if commit.ResourceRef.Kind != "memory" {
				return contract.RuleDecision{Verdict: contract.VerdictDeny, Reasons: []string{"remote memory import denied: non-memory resource"}}, nil
			}
			incoming := memoryEntriesFromFields(commit.Fields)
			if len(incoming) == 0 {
				if content := strings.TrimSpace(stringField(commit.Fields, "content")); content != "" {
					incoming = []memoryEntry{{
						ID:         remoteMemoryEntryID(commit),
						Content:    content,
						Source:     "remote",
						Confidence: "remote",
						Actor:      string(commit.Actor),
						IngestSeq:  commit.LocalIngestSeq,
					}}
				}
			}
			if len(incoming) == 0 {
				return contract.RuleDecision{Verdict: contract.VerdictDeny, Reasons: []string{"remote memory import denied: no memory entries"}}, nil
			}
			version, fields := resourceFromProjection(in.View, commit.ResourceRef)
			existing := memoryEntriesFromFields(fields)
			byID := make(map[string]memoryEntry, len(existing))
			for _, entry := range existing {
				byID[entry.ID] = entry
			}
			var additions []memoryEntry
			for _, entry := range incoming {
				if current, ok := byID[entry.ID]; ok {
					if current.Content != entry.Content {
						return contract.RuleDecision{Verdict: contract.VerdictDeny, Reasons: []string{"remote memory conflict: entry " + entry.ID + " already exists with different content"}}, nil
					}
					continue
				}
				additions = append(additions, entry)
			}
			if len(additions) == 0 {
				return contract.RuleDecision{Verdict: contract.VerdictAllow}, nil
			}
			entries := append(append([]memoryEntry(nil), existing...), additions...)
			newFields := map[string]any{
				"content":    renderMemoryContent(entries),
				"entries":    entries,
				"updated_by": string(in.Event.Actor),
			}
			write := contract.ResourceWrite{Ref: commit.ResourceRef, Kind: contract.OpCreate, Fields: newFields}
			if version > 0 {
				write.Kind = contract.OpUpdate
				write.BasedOn = version
			}
			return contract.RuleDecision{Verdict: contract.VerdictPropose, Proposal: &contract.ProposedEvent{
				Type:    MemoryWriteProposed,
				Payload: map[string]any{"writes": []contract.ResourceWrite{write}},
			}}, nil
		})
}

func decodeRemoteMemoryCommit(payload map[string]any) (contract.LocalCommit, error) {
	raw, ok := payload["commit"]
	if !ok {
		return contract.LocalCommit{}, fmt.Errorf("remote memory import denied: missing commit")
	}
	data, err := json.Marshal(raw)
	if err != nil {
		return contract.LocalCommit{}, fmt.Errorf("remote memory import denied: encode commit: %w", err)
	}
	var commit contract.LocalCommit
	if err := json.Unmarshal(data, &commit); err != nil {
		return contract.LocalCommit{}, fmt.Errorf("remote memory import denied: decode commit: %w", err)
	}
	if strings.TrimSpace(commit.OriginReplicaID) == "" || strings.TrimSpace(commit.LocalDecisionID) == "" {
		return contract.LocalCommit{}, fmt.Errorf("remote memory import denied: missing provenance")
	}
	return commit, nil
}

func remoteMemoryEntryID(commit contract.LocalCommit) string {
	return "remote/" + sanitizeEntryIDPart(commit.OriginReplicaID) + "/" + sanitizeEntryIDPart(commit.LocalDecisionID)
}

type memoryCandidate struct {
	Content    string
	Source     string
	Confidence string
	Tags       []string
}

type memoryEntry struct {
	ID         string   `json:"id"`
	Content    string   `json:"content"`
	Source     string   `json:"source"`
	Confidence string   `json:"confidence"`
	Tags       []string `json:"tags,omitempty"`
	Actor      string   `json:"actor"`
	IngestSeq  int64    `json:"ingest_seq"`
}

func decodeMemoryCandidate(payload map[string]any) (memoryCandidate, error) {
	content := strings.TrimSpace(stringField(payload, "content"))
	if content == "" {
		return memoryCandidate{}, fmt.Errorf("memory candidate denied: empty content")
	}
	if containsSecretLikeContent(content) {
		return memoryCandidate{}, fmt.Errorf("memory candidate denied: secret-like content")
	}
	if containsPromptInjectionShape(content) {
		return memoryCandidate{}, fmt.Errorf("memory candidate denied: prompt-injection-shaped content")
	}
	source := strings.TrimSpace(stringField(payload, "source"))
	if source == "" {
		return memoryCandidate{}, fmt.Errorf("memory candidate denied: missing source")
	}
	confidence := strings.TrimSpace(stringField(payload, "confidence"))
	if confidence == "" {
		return memoryCandidate{}, fmt.Errorf("memory candidate denied: missing confidence")
	}
	return memoryCandidate{Content: content, Source: source, Confidence: confidence, Tags: stringSliceField(payload, "tags")}, nil
}

func stringField(payload map[string]any, key string) string {
	if v, ok := payload[key].(string); ok {
		return v
	}
	return ""
}

func stringSliceField(payload map[string]any, key string) []string {
	switch raw := payload[key].(type) {
	case []string:
		return compactStrings(raw)
	case []any:
		out := make([]string, 0, len(raw))
		for _, v := range raw {
			if s, ok := v.(string); ok {
				out = append(out, s)
			}
		}
		return compactStrings(out)
	case string:
		return compactStrings(strings.Split(raw, ","))
	default:
		return nil
	}
}

func compactStrings(in []string) []string {
	var out []string
	for _, s := range in {
		if trimmed := strings.TrimSpace(s); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

func containsSecretLikeContent(content string) bool {
	lower := strings.ToLower(content)
	for _, marker := range []string{
		"password=", "password:", "api_key", "api key", "secret=", "secret:",
		"token=", "token:", "bearer ", "private key", "-----begin",
	} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return regexp.MustCompile(`sk-[a-zA-Z0-9]{12,}`).FindString(content) != ""
}

func containsPromptInjectionShape(content string) bool {
	lower := strings.ToLower(content)
	for _, marker := range []string{
		"ignore previous instructions",
		"disregard previous instructions",
		"reveal the system prompt",
		"show the system prompt",
		"developer message",
		"act as system",
	} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

func resourceFromProjection(view projection.Projection, ref contract.ResourceRef) (contract.Version, map[string]any) {
	var version contract.Version
	for _, rv := range view.Resources {
		if rv.Ref == ref {
			version = rv.Version
			break
		}
	}
	for _, item := range view.Content {
		if item.Ref == ref {
			return item.Version, item.Fields
		}
	}
	return version, nil
}

func memoryEntriesFromFields(fields map[string]any) []memoryEntry {
	if fields == nil {
		return nil
	}
	raw, ok := fields["entries"].([]any)
	if !ok {
		return nil
	}
	entries := make([]memoryEntry, 0, len(raw))
	for _, item := range raw {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		entry := memoryEntry{
			ID:         stringMapField(m, "id"),
			Content:    stringMapField(m, "content"),
			Source:     stringMapField(m, "source"),
			Confidence: stringMapField(m, "confidence"),
			Tags:       stringSliceMapField(m, "tags"),
			Actor:      stringMapField(m, "actor"),
			IngestSeq:  int64MapField(m, "ingest_seq"),
		}
		if entry.ID != "" && entry.Content != "" {
			entries = append(entries, entry)
		}
	}
	return entries
}

func stringMapField(m map[string]any, key string) string {
	if s, ok := m[key].(string); ok {
		return s
	}
	return ""
}

func stringSliceMapField(m map[string]any, key string) []string {
	if raw, ok := m[key].([]any); ok {
		out := make([]string, 0, len(raw))
		for _, v := range raw {
			if s, ok := v.(string); ok {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}

func int64MapField(m map[string]any, key string) int64 {
	switch v := m[key].(type) {
	case float64:
		return int64(v)
	case int64:
		return v
	case int:
		return int64(v)
	default:
		return 0
	}
}

func renderMemoryContent(entries []memoryEntry) string {
	var lines []string
	lines = append(lines, "# Local Memory")
	for _, entry := range entries {
		meta := []string{"id: " + entry.ID, "source: " + entry.Source, "confidence: " + entry.Confidence}
		if len(entry.Tags) > 0 {
			meta = append(meta, "tags: "+strings.Join(entry.Tags, ","))
		}
		lines = append(lines, "- "+entry.Content+" ("+strings.Join(meta, "; ")+")")
	}
	return strings.Join(lines, "\n")
}

func memoryEntryID(actor contract.ActorID, ingestSeq int64) string {
	return "local/" + sanitizeEntryIDPart(string(actor)) + "/" + strconv.FormatInt(ingestSeq, 10)
}

func sanitizeEntryIDPart(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_' || r == '.' {
			b.WriteRune(r)
		} else {
			b.WriteByte('-')
		}
	}
	if b.Len() == 0 {
		return "unknown"
	}
	return b.String()
}
