package profile

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/mnemon-dev/mnemon/harness/internal/lifecycle/eventlog"
	"github.com/mnemon-dev/mnemon/harness/internal/lifecycle/layout"
	"github.com/mnemon-dev/mnemon/harness/internal/lifecycle/schema"
)

const (
	SchemaVersion    = "mnemon.profile.v1"
	Kind             = "Profile"
	DefaultID        = "personal-default"
	ScopePersonal    = "personal"
	EventEntryRecord = "profile.entry_recorded"
)

var (
	ErrProfileNotFound  = errors.New("profile not found")
	ErrDuplicateEntryID = errors.New("profile entry already exists")
	idCleaner           = regexp.MustCompile(`[^a-z0-9_.-]+`)
	allowedProfileScope = map[string]bool{ScopePersonal: true}
)

type Profile struct {
	SchemaVersion string         `json:"schema_version"`
	Kind          string         `json:"kind"`
	ID            string         `json:"id"`
	ScopeType     string         `json:"scope_type"`
	Summary       string         `json:"summary,omitempty"`
	Entries       []Entry        `json:"entries,omitempty"`
	CreatedAt     string         `json:"created_at"`
	UpdatedAt     string         `json:"updated_at"`
	Metadata      map[string]any `json:"metadata,omitempty"`
}

type Entry struct {
	ID                string             `json:"id"`
	Type              string             `json:"type"`
	Summary           string             `json:"summary"`
	Content           string             `json:"content"`
	Evidence          []EvidenceRef      `json:"evidence"`
	ProjectionTargets []ProjectionTarget `json:"projection_targets,omitempty"`
	CreatedAt         string             `json:"created_at"`
	UpdatedAt         string             `json:"updated_at"`
}

type EvidenceRef struct {
	Type    string `json:"type"`
	Ref     string `json:"ref"`
	Summary string `json:"summary,omitempty"`
}

type ProjectionTarget struct {
	Host string `json:"host"`
	Loop string `json:"loop"`
}

type AddEntryOptions struct {
	ProfileID         string
	EntryID           string
	Type              string
	Summary           string
	Content           string
	Evidence          []EvidenceRef
	ProjectionTargets []ProjectionTarget
	Now               time.Time
}

type Store struct {
	paths layout.Paths
}

func New(root string) (*Store, error) {
	paths, err := layout.Resolve(root)
	if err != nil {
		return nil, err
	}
	return &Store{paths: paths}, nil
}

func ProfileRef(id string) string {
	return "profile:personal/" + profileID(id)
}

func ParseProfileRef(ref string) (string, error) {
	ref = strings.TrimSpace(ref)
	const prefix = "profile:personal/"
	if !strings.HasPrefix(ref, prefix) {
		return "", fmt.Errorf("profile ref %q must start with %s", ref, prefix)
	}
	rawID := strings.TrimSpace(strings.TrimPrefix(ref, prefix))
	if rawID == "" {
		return "", fmt.Errorf("profile ref %q has no profile id", ref)
	}
	id := profileID(rawID)
	if id == "" {
		return "", fmt.Errorf("profile ref %q has no profile id", ref)
	}
	return id, nil
}

func (s *Store) AddEntry(opts AddEntryOptions) (Profile, Entry, error) {
	paths, err := layout.EnsureProject(s.paths.Root)
	if err != nil {
		return Profile{}, Entry{}, err
	}
	s.paths = paths
	opts.Now = layout.NormalizeNow(opts.Now)
	id := profileID(opts.ProfileID)
	prof, err := s.Load(id)
	if errors.Is(err, ErrProfileNotFound) {
		prof = newProfile(id, opts.Now)
	} else if err != nil {
		return Profile{}, Entry{}, err
	}

	entryID := cleanID(opts.EntryID)
	if entryID == "" {
		entryID = generatedEntryID(opts.Type, opts.Summary, opts.Now)
	}
	for _, existing := range prof.Entries {
		if existing.ID == entryID {
			return Profile{}, Entry{}, fmt.Errorf("%w: %s", ErrDuplicateEntryID, entryID)
		}
	}

	stamp := opts.Now.UTC().Format(time.RFC3339)
	entry := Entry{
		ID:                entryID,
		Type:              strings.TrimSpace(opts.Type),
		Summary:           strings.TrimSpace(opts.Summary),
		Content:           strings.TrimSpace(opts.Content),
		Evidence:          normalizeEvidence(opts.Evidence),
		ProjectionTargets: normalizeProjectionTargets(opts.ProjectionTargets),
		CreatedAt:         stamp,
		UpdatedAt:         stamp,
	}
	if err := ValidateEntry(entry); err != nil {
		return Profile{}, Entry{}, err
	}
	prof.Entries = append(prof.Entries, entry)
	prof.UpdatedAt = stamp
	if err := Validate(prof); err != nil {
		return Profile{}, Entry{}, err
	}
	if err := s.write(prof); err != nil {
		return Profile{}, Entry{}, err
	}
	if err := s.appendEntryRecordedEvent(opts.Now, prof, entry); err != nil {
		return Profile{}, Entry{}, err
	}
	return prof, entry, nil
}

func (s *Store) Load(id string) (Profile, error) {
	id = profileID(id)
	data, err := os.ReadFile(s.profilePath(id))
	if err != nil {
		if os.IsNotExist(err) {
			return Profile{}, ErrProfileNotFound
		}
		return Profile{}, err
	}
	var prof Profile
	if err := json.Unmarshal(data, &prof); err != nil {
		return Profile{}, fmt.Errorf("parse profile %s: %w", id, err)
	}
	if err := Validate(prof); err != nil {
		return Profile{}, fmt.Errorf("validate profile %s: %w", id, err)
	}
	return prof, nil
}

func (s *Store) FilterEntries(prof Profile, host, loop string) Profile {
	host = strings.TrimSpace(host)
	loop = strings.TrimSpace(loop)
	if host == "" && loop == "" {
		return prof
	}
	filtered := prof
	filtered.Entries = nil
	for _, entry := range prof.Entries {
		if entryMatchesProjection(entry, host, loop) {
			filtered.Entries = append(filtered.Entries, entry)
		}
	}
	return filtered
}

func Validate(prof Profile) error {
	var errs []error
	if prof.SchemaVersion != SchemaVersion {
		errs = append(errs, fmt.Errorf("schema_version must be %s", SchemaVersion))
	}
	if prof.Kind != Kind {
		errs = append(errs, fmt.Errorf("kind must be %s", Kind))
	}
	if cleanID(prof.ID) == "" {
		errs = append(errs, errors.New("id is required"))
	}
	if !allowedProfileScope[prof.ScopeType] {
		errs = append(errs, fmt.Errorf("scope_type must be %s", ScopePersonal))
	}
	if err := validateTimestamp("created_at", prof.CreatedAt); err != nil {
		errs = append(errs, err)
	}
	if err := validateTimestamp("updated_at", prof.UpdatedAt); err != nil {
		errs = append(errs, err)
	}
	seen := map[string]bool{}
	for _, entry := range prof.Entries {
		if seen[entry.ID] {
			errs = append(errs, fmt.Errorf("duplicate entry id %q", entry.ID))
		}
		seen[entry.ID] = true
		if err := ValidateEntry(entry); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func ValidateEntry(entry Entry) error {
	var errs []error
	if cleanID(entry.ID) == "" {
		errs = append(errs, errors.New("entry id is required"))
	}
	if strings.TrimSpace(entry.Type) == "" {
		errs = append(errs, errors.New("entry type is required"))
	}
	if strings.TrimSpace(entry.Summary) == "" {
		errs = append(errs, errors.New("entry summary is required"))
	}
	if strings.TrimSpace(entry.Content) == "" {
		errs = append(errs, errors.New("entry content is required"))
	}
	if len(entry.Evidence) == 0 {
		errs = append(errs, errors.New("entry evidence is required"))
	}
	for _, ref := range entry.Evidence {
		if strings.TrimSpace(ref.Type) == "" || strings.TrimSpace(ref.Ref) == "" {
			errs = append(errs, errors.New("entry evidence refs require type and ref"))
		}
	}
	for _, target := range entry.ProjectionTargets {
		if strings.TrimSpace(target.Host) == "" || strings.TrimSpace(target.Loop) == "" {
			errs = append(errs, errors.New("projection targets require host and loop"))
		}
	}
	if err := validateTimestamp("entry.created_at", entry.CreatedAt); err != nil {
		errs = append(errs, err)
	}
	if err := validateTimestamp("entry.updated_at", entry.UpdatedAt); err != nil {
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}

func (s *Store) write(prof Profile) error {
	return layout.WriteJSONAtomic(s.profilePath(prof.ID), prof, 0o644)
}

func (s *Store) profilePath(id string) string {
	return filepath.Join(s.paths.HarnessDir, "profiles", profileID(id), "profile.json")
}

func (s *Store) appendEntryRecordedEvent(now time.Time, prof Profile, entry Entry) error {
	events, err := eventlog.New(s.paths.Root)
	if err != nil {
		return err
	}
	scope := schema.ProjectScopeWithProfile(s.paths.Root, "", "", "", ProfileRef(prof.ID)).Map()
	baseID := fmt.Sprintf("evt_profile_%s_entry_recorded_%d", prof.ID, now.UnixNano())
	event := schema.Event{
		SchemaVersion: schema.Version,
		ID:            baseID,
		TS:            now.UTC().Format(time.RFC3339),
		Type:          EventEntryRecord,
		Loop:          nil,
		Host:          nil,
		Actor:         "mnemon-manual",
		Source:        "profile",
		CorrelationID: "profile:" + prof.ID,
		CausedBy:      nil,
		ProjectRoot:   s.paths.Root,
		Scope:         scope,
		Payload: map[string]any{
			"profile_id":         prof.ID,
			"profile_ref":        ProfileRef(prof.ID),
			"entry_id":           entry.ID,
			"entry_type":         entry.Type,
			"evidence":           entry.Evidence,
			"projection_targets": entry.ProjectionTargets,
		},
	}
	for attempt := 0; attempt < 100; attempt++ {
		event.ID = eventIDAttempt(baseID, attempt)
		if err := events.Append(event); err != nil {
			if eventlog.IsDuplicateEventID(err) {
				continue
			}
			return err
		}
		return nil
	}
	return fmt.Errorf("append profile event: exhausted duplicate event id retries for %q", baseID)
}

func newProfile(id string, now time.Time) Profile {
	stamp := now.UTC().Format(time.RFC3339)
	return Profile{
		SchemaVersion: SchemaVersion,
		Kind:          Kind,
		ID:            profileID(id),
		ScopeType:     ScopePersonal,
		CreatedAt:     stamp,
		UpdatedAt:     stamp,
	}
}

func normalizeEvidence(values []EvidenceRef) []EvidenceRef {
	out := make([]EvidenceRef, 0, len(values))
	for _, value := range values {
		out = append(out, EvidenceRef{
			Type:    strings.TrimSpace(value.Type),
			Ref:     strings.TrimSpace(value.Ref),
			Summary: strings.TrimSpace(value.Summary),
		})
	}
	return out
}

func normalizeProjectionTargets(values []ProjectionTarget) []ProjectionTarget {
	out := make([]ProjectionTarget, 0, len(values))
	seen := map[string]bool{}
	for _, value := range values {
		target := ProjectionTarget{
			Host: strings.TrimSpace(value.Host),
			Loop: strings.TrimSpace(value.Loop),
		}
		key := target.Host + "/" + target.Loop
		if target.Host == "" && target.Loop == "" || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, target)
	}
	return out
}

func entryMatchesProjection(entry Entry, host, loop string) bool {
	for _, target := range entry.ProjectionTargets {
		hostMatches := host == "" || target.Host == host
		loopMatches := loop == "" || target.Loop == loop
		if hostMatches && loopMatches {
			return true
		}
	}
	return false
}

func validateTimestamp(field, value string) error {
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("%s is required", field)
	}
	if _, err := time.Parse(time.RFC3339, value); err != nil {
		return fmt.Errorf("%s must be RFC3339: %w", field, err)
	}
	return nil
}

func profileID(id string) string {
	id = cleanID(id)
	if id == "" {
		return DefaultID
	}
	return id
}

func cleanID(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = idCleaner.ReplaceAllString(value, "-")
	value = strings.Trim(value, "-_.")
	return value
}

func generatedEntryID(entryType, summary string, now time.Time) string {
	base := cleanID(strings.TrimSpace(entryType) + "-" + strings.TrimSpace(summary))
	if base == "" {
		base = "profile-entry"
	}
	return fmt.Sprintf("%s-%s", base, layout.TimestampID(now))
}

func eventIDAttempt(base string, attempt int) string {
	if attempt == 0 {
		return base
	}
	return fmt.Sprintf("%s_%d", base, attempt+1)
}
