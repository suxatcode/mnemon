package auditstore

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/mnemon-dev/mnemon/harness/internal/lifecycle/eventlog"
	"github.com/mnemon-dev/mnemon/harness/internal/lifecycle/layout"
	"github.com/mnemon-dev/mnemon/harness/internal/lifecycle/schema"
)

var ErrAuditNotFound = errors.New("audit not found")

type Store struct {
	paths layout.Paths
}

type WriteOptions struct {
	ID          string
	Spec        map[string]any
	Labels      map[string]string
	Annotations map[string]string
}

type WriteResult struct {
	Audit schema.Audit
	Path  string
	Ref   map[string]any
}

type IntegrityIssue struct {
	Kind    string `json:"kind"`
	EventID string `json:"event_id,omitempty"`
	AuditID string `json:"audit_id,omitempty"`
	URI     string `json:"uri,omitempty"`
	Detail  string `json:"detail"`
}

type RecordedEventOptions struct {
	ID            string
	Now           time.Time
	Loop          string
	Host          string
	Actor         string
	Source        string
	CorrelationID string
	CausedBy      string
	Payload       map[string]any
	AuditRef      map[string]any
	Scope         map[string]any
}

func New(root string) (*Store, error) {
	paths, err := layout.Resolve(root)
	if err != nil {
		return nil, err
	}
	return &Store{paths: paths}, nil
}

func (s *Store) Write(opts WriteOptions) (WriteResult, error) {
	paths, err := layout.EnsureProject(s.paths.Root)
	if err != nil {
		return WriteResult{}, err
	}
	s.paths = paths

	id := cleanID(opts.ID)
	if id == "" {
		return WriteResult{}, fmt.Errorf("audit id is required")
	}
	audit := schema.Audit{
		SchemaVersion: schema.Version,
		Kind:          "Audit",
		Metadata: schema.Metadata{
			Name:        id,
			Labels:      opts.Labels,
			Annotations: opts.Annotations,
		},
		Spec: opts.Spec,
	}
	if err := schema.ValidateAudit(audit); err != nil {
		return WriteResult{}, err
	}

	path := filepath.Join(s.paths.HarnessDir, "audit", "records", id+".json")
	if err := writeJSONAtomic(path, audit); err != nil {
		return WriteResult{}, err
	}
	ref := map[string]any{"uri": relativeTo(s.paths.Root, path)}
	return WriteResult{Audit: audit, Path: path, Ref: ref}, nil
}

func (s *Store) Load(id string) (WriteResult, error) {
	id = cleanID(id)
	if id == "" {
		return WriteResult{}, ErrAuditNotFound
	}
	path := filepath.Join(s.paths.HarnessDir, "audit", "records", id+".json")
	return s.read(path)
}

func (s *Store) List() ([]WriteResult, error) {
	dir := filepath.Join(s.paths.HarnessDir, "audit", "records")
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read audit records: %w", err)
	}
	records := make([]WriteResult, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		record, err := s.read(filepath.Join(dir, entry.Name()))
		if err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	sort.Slice(records, func(i, j int) bool {
		return records[i].Audit.Metadata.Name < records[j].Audit.Metadata.Name
	})
	return records, nil
}

func (s *Store) VerifyIntegrity() ([]IntegrityIssue, error) {
	records, err := s.List()
	if err != nil {
		return nil, err
	}
	recordByURI := map[string]WriteResult{}
	referenced := map[string]bool{}
	for _, record := range records {
		uri := strings.TrimSpace(stringField(record.Ref, "uri"))
		if uri == "" {
			continue
		}
		recordByURI[normalizeURI(uri)] = record
	}

	events, err := eventlog.New(s.paths.Root)
	if err != nil {
		return nil, err
	}
	allEvents, err := events.ReadAll()
	if err != nil {
		return nil, err
	}

	var issues []IntegrityIssue
	for _, event := range allEvents {
		if event.Type != "audit.recorded" {
			continue
		}
		uri := strings.TrimSpace(stringField(event.AuditRef, "uri"))
		if uri == "" {
			issues = append(issues, IntegrityIssue{
				Kind:    "missing_audit_ref",
				EventID: event.ID,
				Detail:  "audit.recorded event has no audit_ref.uri",
			})
			continue
		}
		normalized := normalizeURI(uri)
		referenced[normalized] = true
		if _, ok := recordByURI[normalized]; !ok {
			issues = append(issues, IntegrityIssue{
				Kind:    "missing_audit_record",
				EventID: event.ID,
				URI:     uri,
				Detail:  "audit.recorded event references an audit record that is not present",
			})
		}
	}

	for uri, record := range recordByURI {
		if referenced[uri] {
			continue
		}
		issues = append(issues, IntegrityIssue{
			Kind:    "unrecorded_audit_record",
			AuditID: record.Audit.Metadata.Name,
			URI:     strings.TrimSpace(stringField(record.Ref, "uri")),
			Detail:  "audit record has no matching audit.recorded event",
		})
	}

	sort.Slice(issues, func(i, j int) bool {
		if issues[i].Kind != issues[j].Kind {
			return issues[i].Kind < issues[j].Kind
		}
		if issues[i].EventID != issues[j].EventID {
			return issues[i].EventID < issues[j].EventID
		}
		if issues[i].URI != issues[j].URI {
			return issues[i].URI < issues[j].URI
		}
		return issues[i].AuditID < issues[j].AuditID
	})
	return issues, nil
}

func (s *Store) AppendRecordedEvent(opts RecordedEventOptions) (schema.Event, error) {
	paths, err := layout.EnsureProject(s.paths.Root)
	if err != nil {
		return schema.Event{}, err
	}
	s.paths = paths

	now := layout.NormalizeNow(opts.Now)
	actor := strings.TrimSpace(opts.Actor)
	if actor == "" {
		actor = "mnemon-manual"
	}
	source := strings.TrimSpace(opts.Source)
	if source == "" {
		source = "auditstore"
	}
	correlationID := strings.TrimSpace(opts.CorrelationID)
	if correlationID == "" {
		correlationID = "audit:" + strings.TrimSpace(stringField(opts.AuditRef, "uri"))
	}
	event := schema.Event{
		SchemaVersion: schema.Version,
		ID:            strings.TrimSpace(opts.ID),
		TS:            now.UTC().Format(time.RFC3339),
		Type:          "audit.recorded",
		Actor:         actor,
		Source:        source,
		CorrelationID: correlationID,
		Payload:       opts.Payload,
		AuditRef:      copyMap(opts.AuditRef),
		Scope:         copyMap(opts.Scope),
	}
	if event.Payload == nil {
		event.Payload = map[string]any{}
	}
	if strings.TrimSpace(opts.Loop) != "" {
		loop := strings.TrimSpace(opts.Loop)
		event.Loop = &loop
	}
	if strings.TrimSpace(opts.Host) != "" {
		host := strings.TrimSpace(opts.Host)
		event.Host = &host
	}
	if strings.TrimSpace(opts.CausedBy) != "" {
		causedBy := strings.TrimSpace(opts.CausedBy)
		event.CausedBy = &causedBy
	}
	if err := schema.ValidateEvent(event); err != nil {
		return schema.Event{}, err
	}

	events, err := eventlog.New(s.paths.Root)
	if err != nil {
		return schema.Event{}, err
	}
	if err := events.Append(event); err != nil {
		return schema.Event{}, err
	}
	return event, nil
}

func (s *Store) read(path string) (WriteResult, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return WriteResult{}, ErrAuditNotFound
	}
	if err != nil {
		return WriteResult{}, err
	}
	var audit schema.Audit
	if err := json.Unmarshal(data, &audit); err != nil {
		return WriteResult{}, fmt.Errorf("parse audit %s: %w", path, err)
	}
	if err := schema.ValidateAudit(audit); err != nil {
		return WriteResult{}, fmt.Errorf("validate audit %s: %w", path, err)
	}
	ref := map[string]any{"uri": relativeTo(s.paths.Root, path)}
	return WriteResult{Audit: audit, Path: path, Ref: ref}, nil
}

var idCleaner = regexp.MustCompile(`[^A-Za-z0-9_.-]+`)

func cleanID(value string) string {
	value = strings.TrimSpace(value)
	value = idCleaner.ReplaceAllString(value, "-")
	value = strings.Trim(value, "-_.")
	return value
}

func relativeTo(root, path string) string {
	if rel, err := filepath.Rel(root, path); err == nil && !strings.HasPrefix(rel, "..") {
		return rel
	}
	return path
}

func stringField(values map[string]any, key string) string {
	value, ok := values[key]
	if !ok {
		return ""
	}
	text, _ := value.(string)
	return text
}

func normalizeURI(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	return filepath.ToSlash(filepath.Clean(value))
}

func copyMap(values map[string]any) map[string]any {
	if values == nil {
		return nil
	}
	out := make(map[string]any, len(values))
	for key, value := range values {
		out[key] = value
	}
	return out
}

func writeJSONAtomic(path string, value any) error {
	return layout.WriteJSONAtomic(path, value, 0o600)
}
