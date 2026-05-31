package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/mnemon-dev/mnemon/harness/internal/lifecycle/auditstore"
	"github.com/mnemon-dev/mnemon/harness/internal/lifecycle/schema"
)

// AuditAppendInput carries the audit append parameters from the surface flags.
type AuditAppendInput struct {
	ID            string
	Kind          string
	Decision      string
	Reason        string
	JobID         string
	RunnerID      string
	ProposalRefs  []string
	EventRefs     []string
	ArtifactRefs  []string
	SpecJSON      string
	EventID       string
	Loop          string
	Host          string
	Source        string
	CorrelationID string
	CausedBy      string
}

func (h *Harness) AuditAppend(out io.Writer, in AuditAppendInput) error {
	store, err := auditstore.New(h.root)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	id := strings.TrimSpace(in.ID)
	if id == "" {
		id = generatedAuditID(in.Kind, now)
	}
	if _, err := store.Load(id); err == nil {
		return fmt.Errorf("audit %q already exists", id)
	} else if !errors.Is(err, auditstore.ErrAuditNotFound) {
		return err
	}
	spec, err := buildAuditSpec(in)
	if err != nil {
		return err
	}
	written, err := store.Write(auditstore.WriteOptions{
		ID:   id,
		Spec: spec,
	})
	if err != nil {
		return err
	}
	eventID := strings.TrimSpace(in.EventID)
	if eventID == "" {
		eventID = generatedAuditEventID(written.Audit.Metadata.Name, now)
	}
	event, err := store.AppendRecordedEvent(auditstore.RecordedEventOptions{
		ID:            eventID,
		Now:           now,
		Loop:          in.Loop,
		Host:          in.Host,
		Source:        in.Source,
		CorrelationID: in.CorrelationID,
		CausedBy:      in.CausedBy,
		Payload:       auditPayload(written.Audit),
		AuditRef:      written.Ref,
	})
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "appended audit %s\n", written.Audit.Metadata.Name)
	fmt.Fprintf(out, "uri: %s\n", written.Ref["uri"])
	fmt.Fprintf(out, "event: %s\n", event.ID)
	return nil
}

func (h *Harness) AuditList(out io.Writer, kind, format string) error {
	store, err := auditstore.New(h.root)
	if err != nil {
		return err
	}
	records, err := store.List()
	if err != nil {
		return err
	}
	records = filterAuditRecords(records, kind)
	if format == "json" {
		return writeJSON(out, records)
	}
	if format != "" && format != "text" {
		return fmt.Errorf("unsupported --format %q", format)
	}
	for _, record := range records {
		fmt.Fprintf(out, "%s\t%s\t%s\t%s\n",
			record.Audit.Metadata.Name,
			auditSpecString(record.Audit, "audit_kind"),
			auditSpecString(record.Audit, "decision"),
			record.Ref["uri"],
		)
	}
	return nil
}

func (h *Harness) AuditShow(out io.Writer, auditID, format string) error {
	store, err := auditstore.New(h.root)
	if err != nil {
		return err
	}
	record, err := store.Load(auditID)
	if err != nil {
		return err
	}
	if format == "json" {
		return writeJSON(out, record.Audit)
	}
	if format != "" && format != "text" {
		return fmt.Errorf("unsupported --format %q", format)
	}
	writeAuditText(out, record)
	return nil
}

// AuditIntegrity returns the audit↔event integrity issue count without emitting a
// report — the read-only form surfaces use for health. ok is false when the store
// cannot be read.
func (h *Harness) AuditIntegrity() (issues int, ok bool) {
	store, err := auditstore.New(h.root)
	if err != nil {
		return 0, false
	}
	found, err := store.VerifyIntegrity()
	if err != nil {
		return 0, false
	}
	return len(found), true
}

func (h *Harness) AuditVerify(out io.Writer, format string) error {
	store, err := auditstore.New(h.root)
	if err != nil {
		return err
	}
	issues, err := store.VerifyIntegrity()
	if err != nil {
		return err
	}
	if format == "json" {
		if err := writeJSON(out, issues); err != nil {
			return err
		}
	} else {
		if format != "" && format != "text" {
			return fmt.Errorf("unsupported --format %q", format)
		}
		if len(issues) == 0 {
			fmt.Fprintln(out, "audit integrity ok")
		}
		for _, issue := range issues {
			fmt.Fprintf(out, "%s", issue.Kind)
			if issue.EventID != "" {
				fmt.Fprintf(out, "\tevent=%s", issue.EventID)
			}
			if issue.AuditID != "" {
				fmt.Fprintf(out, "\taudit=%s", issue.AuditID)
			}
			if issue.URI != "" {
				fmt.Fprintf(out, "\turi=%s", issue.URI)
			}
			if issue.Detail != "" {
				fmt.Fprintf(out, "\t%s", issue.Detail)
			}
			fmt.Fprintln(out)
		}
	}
	if len(issues) > 0 {
		return fmt.Errorf("audit integrity failed: %d issue(s)", len(issues))
	}
	return nil
}

func buildAuditSpec(in AuditAppendInput) (map[string]any, error) {
	spec := map[string]any{}
	if strings.TrimSpace(in.SpecJSON) != "" {
		if err := json.Unmarshal([]byte(in.SpecJSON), &spec); err != nil {
			return nil, fmt.Errorf("parse --spec-json: %w", err)
		}
		if spec == nil {
			return nil, errors.New("--spec-json must be a JSON object")
		}
	}
	if strings.TrimSpace(in.Decision) == "" && len(spec) == 0 {
		return nil, errors.New("--decision or --spec-json is required")
	}
	if strings.TrimSpace(in.Kind) != "" {
		spec["audit_kind"] = strings.TrimSpace(in.Kind)
	}
	if strings.TrimSpace(in.Decision) != "" {
		spec["decision"] = strings.TrimSpace(in.Decision)
	}
	if strings.TrimSpace(in.Reason) != "" {
		spec["reason"] = strings.TrimSpace(in.Reason)
	}
	if strings.TrimSpace(in.JobID) != "" {
		spec["job_id"] = strings.TrimSpace(in.JobID)
	}
	if strings.TrimSpace(in.RunnerID) != "" {
		spec["runner_id"] = strings.TrimSpace(in.RunnerID)
	}
	if len(in.ProposalRefs) > 0 {
		spec["proposal_refs"] = append([]string(nil), in.ProposalRefs...)
	}
	if len(in.EventRefs) > 0 {
		spec["event_refs"] = append([]string(nil), in.EventRefs...)
	}
	if len(in.ArtifactRefs) > 0 {
		spec["artifact_refs"] = append([]string(nil), in.ArtifactRefs...)
	}
	return spec, nil
}

func auditPayload(audit schema.Audit) map[string]any {
	payload := map[string]any{
		"audit_id": audit.Metadata.Name,
	}
	for _, key := range []string{"audit_kind", "decision", "reason", "job_id", "runner_id"} {
		if value, ok := audit.Spec[key]; ok {
			payload[key] = value
		}
	}
	return payload
}

func filterAuditRecords(records []auditstore.WriteResult, kind string) []auditstore.WriteResult {
	kind = strings.TrimSpace(kind)
	if kind == "" {
		return records
	}
	filtered := make([]auditstore.WriteResult, 0, len(records))
	for _, record := range records {
		if auditSpecString(record.Audit, "audit_kind") == kind {
			filtered = append(filtered, record)
		}
	}
	return filtered
}

func writeAuditText(out io.Writer, record auditstore.WriteResult) {
	fmt.Fprintf(out, "audit %s\n", record.Audit.Metadata.Name)
	fmt.Fprintf(out, "kind: %s\n", auditSpecString(record.Audit, "audit_kind"))
	fmt.Fprintf(out, "decision: %s\n", auditSpecString(record.Audit, "decision"))
	fmt.Fprintf(out, "reason: %s\n", auditSpecString(record.Audit, "reason"))
	fmt.Fprintf(out, "uri: %s\n", record.Ref["uri"])
	fmt.Fprintf(out, "event_refs: %d\n", auditSpecLen(record.Audit, "event_refs"))
	fmt.Fprintf(out, "proposal_refs: %d\n", auditSpecLen(record.Audit, "proposal_refs"))
	fmt.Fprintf(out, "artifact_refs: %d\n", auditSpecLen(record.Audit, "artifact_refs"))
}

func auditSpecString(audit schema.Audit, key string) string {
	value, ok := audit.Spec[key]
	if !ok {
		return ""
	}
	text, _ := value.(string)
	return text
}

func auditSpecLen(audit schema.Audit, key string) int {
	value, ok := audit.Spec[key]
	if !ok {
		return 0
	}
	switch refs := value.(type) {
	case []string:
		return len(refs)
	case []any:
		return len(refs)
	default:
		return 0
	}
}

func generatedAuditID(kind string, now time.Time) string {
	kind = cleanAuditToken(kind)
	if kind == "" {
		kind = "manual"
	}
	return fmt.Sprintf("%s-%s", kind, now.UTC().Format("20060102T150405Z"))
}

func generatedAuditEventID(id string, now time.Time) string {
	return fmt.Sprintf("evt_audit_%s_recorded_%d", cleanAuditToken(id), now.UnixNano())
}

func cleanAuditToken(value string) string {
	value = strings.TrimSpace(value)
	value = strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z':
			return r
		case r >= 'A' && r <= 'Z':
			return r + ('a' - 'A')
		case r >= '0' && r <= '9':
			return r
		case r == '_' || r == '-' || r == '.':
			return r
		default:
			return '-'
		}
	}, value)
	return strings.Trim(value, "-_.")
}
