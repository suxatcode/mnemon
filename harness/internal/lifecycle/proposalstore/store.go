package proposalstore

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
	"github.com/mnemon-dev/mnemon/harness/internal/lifecycle/proposal"
	"github.com/mnemon-dev/mnemon/harness/internal/lifecycle/schema"
)

var ErrProposalNotFound = errors.New("proposal not found")

type Store struct {
	paths layout.Paths
}

type CreateOptions struct {
	ID             string
	Route          proposal.Route
	Risk           proposal.Risk
	Title          string
	Summary        string
	Change         proposal.ChangeRequest
	Evidence       []proposal.EvidenceRef
	ValidationPlan proposal.ValidationPlan
	Review         proposal.ReviewPolicy
	Scope          map[string]any
	Metadata       map[string]any
	Now            time.Time
}

type TransitionOptions struct {
	ID     string
	Status proposal.Status
	Now    time.Time
}

type UpdateOptions struct {
	ID                 string
	Title              string
	Summary            string
	ChangeSummary      string
	Targets            []proposal.TargetRef
	Operations         []proposal.Operation
	Evidence           []proposal.EvidenceRef
	ValidationSummary  string
	ValidationCommands []string
	ValidationChecks   []string
	Review             *proposal.ReviewPolicy
	Scope              map[string]any
	SupersededBy       string
	Now                time.Time
}

type AppendRefOptions struct {
	ID       string
	AuditRef string
	Now      time.Time
}

func New(root string) (*Store, error) {
	paths, err := layout.Resolve(root)
	if err != nil {
		return nil, err
	}
	return &Store{paths: paths}, nil
}

func (s *Store) Create(opts CreateOptions) (proposal.Proposal, error) {
	paths, err := layout.EnsureProject(s.paths.Root)
	if err != nil {
		return proposal.Proposal{}, err
	}
	s.paths = paths
	opts.Now = normalizeNow(opts.Now)
	id := cleanID(opts.ID)
	if id == "" {
		id = generatedID(opts.Title, opts.Now)
	}
	if existing, err := s.find(id); err == nil {
		return proposal.Proposal{}, fmt.Errorf("proposal %q already exists in %s", id, existing.Status)
	} else if !errors.Is(err, ErrProposalNotFound) {
		return proposal.Proposal{}, err
	}
	item := proposal.New(id, opts.Route, opts.Risk, opts.Title, opts.Summary, opts.Now)
	item.Change = opts.Change
	item.Evidence = opts.Evidence
	item.ValidationPlan = opts.ValidationPlan
	item.Scope = copyMap(opts.Scope)
	item.Metadata = copyMap(opts.Metadata)
	if opts.Review.Required || opts.Review.RequiredScope != "" || opts.Review.RequiredReviews != 0 || len(opts.Review.Reviewers) > 0 || opts.Review.Notes != "" {
		item.Review = opts.Review
	}
	if err := proposal.Validate(item); err != nil {
		return proposal.Proposal{}, err
	}
	if err := s.write(item); err != nil {
		return proposal.Proposal{}, err
	}
	if err := s.appendEvent(opts.Now, item.ID, "proposal.created", nil, item.Scope, map[string]any{
		"proposal_id": item.ID,
		"route":       string(item.Route),
		"risk":        string(item.Risk),
		"status":      string(item.Status),
	}); err != nil {
		return proposal.Proposal{}, err
	}
	return item, nil
}

func (s *Store) Load(id string) (proposal.Proposal, error) {
	found, err := s.find(cleanID(id))
	if err != nil {
		return proposal.Proposal{}, err
	}
	return found, nil
}

func (s *Store) List(statuses ...proposal.Status) ([]proposal.Proposal, error) {
	if len(statuses) == 0 {
		statuses = proposal.Statuses()
	}
	var items []proposal.Proposal
	for _, status := range statuses {
		if err := proposal.ValidateStatus(status); err != nil {
			return nil, err
		}
		dir := s.statusDir(status)
		entries, err := os.ReadDir(dir)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("read proposals %s: %w", status, err)
		}
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			item, err := s.read(filepath.Join(dir, entry.Name(), "proposal.json"))
			if err != nil {
				return nil, err
			}
			items = append(items, item)
		}
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].UpdatedAt == items[j].UpdatedAt {
			return items[i].ID < items[j].ID
		}
		return items[i].UpdatedAt < items[j].UpdatedAt
	})
	return items, nil
}

func (s *Store) Transition(opts TransitionOptions) (proposal.Proposal, error) {
	current, err := s.Load(opts.ID)
	if err != nil {
		return proposal.Proposal{}, err
	}
	opts.Now = normalizeNow(opts.Now)
	next, err := proposal.Transition(current, opts.Status, opts.Now)
	if err != nil {
		return proposal.Proposal{}, err
	}
	if err := proposal.Validate(next); err != nil {
		return proposal.Proposal{}, err
	}
	if err := s.write(next); err != nil {
		return proposal.Proposal{}, err
	}
	if current.Status != next.Status {
		if err := os.RemoveAll(s.proposalDir(current.Status, current.ID)); err != nil {
			return proposal.Proposal{}, fmt.Errorf("remove old proposal state: %w", err)
		}
	}
	if err := s.appendEvent(opts.Now, next.ID, eventType(next.Status), nil, next.Scope, map[string]any{
		"proposal_id": next.ID,
		"from":        string(current.Status),
		"status":      string(next.Status),
	}); err != nil {
		return proposal.Proposal{}, err
	}
	return next, nil
}

func (s *Store) Update(opts UpdateOptions) (proposal.Proposal, error) {
	current, err := s.Load(opts.ID)
	if err != nil {
		return proposal.Proposal{}, err
	}
	if proposal.IsTerminal(current.Status) {
		return proposal.Proposal{}, fmt.Errorf("cannot update terminal proposal %q in %s", current.ID, current.Status)
	}
	opts.Now = normalizeNow(opts.Now)
	next := current
	updated := make([]string, 0, 8)

	if strings.TrimSpace(opts.Title) != "" {
		next.Title = strings.TrimSpace(opts.Title)
		updated = append(updated, "title")
	}
	if strings.TrimSpace(opts.Summary) != "" {
		next.Summary = strings.TrimSpace(opts.Summary)
		updated = append(updated, "summary")
	}
	if strings.TrimSpace(opts.ChangeSummary) != "" {
		next.Change.Summary = strings.TrimSpace(opts.ChangeSummary)
		updated = append(updated, "change.summary")
	}
	if len(opts.Targets) > 0 {
		next.Change.Targets = append(next.Change.Targets, opts.Targets...)
		updated = append(updated, "change.targets")
	}
	if len(opts.Operations) > 0 {
		next.Change.Operations = append(next.Change.Operations, opts.Operations...)
		updated = append(updated, "change.operations")
	}
	if len(opts.Evidence) > 0 {
		next.Evidence = append(next.Evidence, opts.Evidence...)
		updated = append(updated, "evidence")
	}
	if strings.TrimSpace(opts.ValidationSummary) != "" {
		next.ValidationPlan.Summary = strings.TrimSpace(opts.ValidationSummary)
		updated = append(updated, "validation_plan.summary")
	}
	if len(opts.ValidationCommands) > 0 {
		next.ValidationPlan.Commands = append(next.ValidationPlan.Commands, opts.ValidationCommands...)
		updated = append(updated, "validation_plan.commands")
	}
	if len(opts.ValidationChecks) > 0 {
		next.ValidationPlan.Checks = append(next.ValidationPlan.Checks, opts.ValidationChecks...)
		updated = append(updated, "validation_plan.checks")
	}
	if opts.Review != nil {
		next.Review = *opts.Review
		updated = append(updated, "review")
	}
	if len(opts.Scope) > 0 {
		next.Scope = copyMap(opts.Scope)
		updated = append(updated, "scope")
	}
	if strings.TrimSpace(opts.SupersededBy) != "" {
		next.SupersededBy = strings.TrimSpace(opts.SupersededBy)
		updated = append(updated, "superseded_by")
	}
	if len(updated) == 0 {
		return proposal.Proposal{}, errors.New("no proposal updates supplied")
	}
	next.UpdatedAt = opts.Now.UTC().Format(time.RFC3339)

	if err := proposal.Validate(next); err != nil {
		return proposal.Proposal{}, err
	}
	if err := s.write(next); err != nil {
		return proposal.Proposal{}, err
	}
	if err := s.appendEvent(opts.Now, next.ID, "proposal.updated", nil, next.Scope, map[string]any{
		"proposal_id":    next.ID,
		"status":         string(next.Status),
		"updated_fields": updated,
	}); err != nil {
		return proposal.Proposal{}, err
	}
	return next, nil
}

func (s *Store) AppendAuditRef(opts AppendRefOptions) (proposal.Proposal, error) {
	current, err := s.Load(opts.ID)
	if err != nil {
		return proposal.Proposal{}, err
	}
	ref := strings.TrimSpace(opts.AuditRef)
	if ref == "" {
		return proposal.Proposal{}, errors.New("audit ref is required")
	}
	if proposal.IsTerminal(current.Status) {
		return proposal.Proposal{}, fmt.Errorf("cannot update terminal proposal %q in %s", current.ID, current.Status)
	}
	for _, existing := range current.AuditRefs {
		if existing == ref {
			return current, nil
		}
	}

	opts.Now = normalizeNow(opts.Now)
	next := current
	next.AuditRefs = append(next.AuditRefs, ref)
	next.UpdatedAt = opts.Now.UTC().Format(time.RFC3339)
	if err := proposal.Validate(next); err != nil {
		return proposal.Proposal{}, err
	}
	if err := s.write(next); err != nil {
		return proposal.Proposal{}, err
	}
	if err := s.appendEvent(opts.Now, next.ID, "proposal.updated", nil, next.Scope, map[string]any{
		"proposal_id":    next.ID,
		"status":         string(next.Status),
		"updated_fields": []string{"audit_refs"},
		"audit_ref":      ref,
	}); err != nil {
		return proposal.Proposal{}, err
	}
	return next, nil
}

func (s *Store) find(id string) (proposal.Proposal, error) {
	if id == "" {
		return proposal.Proposal{}, ErrProposalNotFound
	}
	for _, status := range proposal.Statuses() {
		item, err := s.read(filepath.Join(s.proposalDir(status, id), "proposal.json"))
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return proposal.Proposal{}, err
		}
		return item, nil
	}
	return proposal.Proposal{}, ErrProposalNotFound
}

func (s *Store) read(path string) (proposal.Proposal, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return proposal.Proposal{}, err
	}
	var item proposal.Proposal
	if err := json.Unmarshal(data, &item); err != nil {
		return proposal.Proposal{}, fmt.Errorf("parse proposal %s: %w", path, err)
	}
	if err := proposal.Validate(item); err != nil {
		return proposal.Proposal{}, fmt.Errorf("validate proposal %s: %w", path, err)
	}
	return item, nil
}

func (s *Store) write(item proposal.Proposal) error {
	if err := proposal.Validate(item); err != nil {
		return err
	}
	path := filepath.Join(s.proposalDir(item.Status, item.ID), "proposal.json")
	return writeJSONAtomic(path, item, 0o644)
}

func (s *Store) proposalDir(status proposal.Status, id string) string {
	return filepath.Join(s.statusDir(status), id)
}

func (s *Store) statusDir(status proposal.Status) string {
	return filepath.Join(s.paths.HarnessDir, "proposals", string(status))
}

func (s *Store) appendEvent(now time.Time, proposalID, typ string, causedBy *string, scope map[string]any, payload map[string]any) error {
	store, err := eventlog.New(s.paths.Root)
	if err != nil {
		return err
	}
	baseID := eventID(proposalID, typ, now)
	event := schema.Event{
		SchemaVersion: schema.Version,
		ID:            baseID,
		TS:            now.UTC().Format(time.RFC3339),
		Type:          typ,
		Loop:          nil,
		Host:          nil,
		Actor:         "mnemon-manual",
		Source:        "proposalstore",
		CorrelationID: "proposal:" + proposalID,
		CausedBy:      causedBy,
		Payload:       payload,
		ProjectRoot:   s.paths.Root,
		Scope:         copyMap(scope),
	}
	event.ProposalRef = map[string]any{"id": proposalID}
	for attempt := 0; attempt < 100; attempt++ {
		event.ID = eventIDAttempt(baseID, attempt)
		if err := store.Append(event); err != nil {
			if eventlog.IsDuplicateEventID(err) {
				continue
			}
			return err
		}
		return nil
	}
	return fmt.Errorf("append proposal event: exhausted duplicate event id retries for %q", baseID)
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

func eventType(status proposal.Status) string {
	switch status {
	case proposal.StatusOpen:
		return "proposal.opened"
	case proposal.StatusInReview:
		return "proposal.in_review"
	case proposal.StatusApproved:
		return "proposal.approved"
	case proposal.StatusRejected:
		return "proposal.rejected"
	case proposal.StatusRequestChanges:
		return "proposal.request_changes"
	case proposal.StatusBlocked:
		return "proposal.blocked"
	case proposal.StatusApplied:
		return "proposal.applied"
	case proposal.StatusSuperseded:
		return "proposal.superseded"
	case proposal.StatusWithdrawn:
		return "proposal.withdrawn"
	case proposal.StatusExpired:
		return "proposal.expired"
	default:
		return "proposal.updated"
	}
}

// normalizeNow stays local (not layout.NormalizeNow): proposalstore truncates to
// whole seconds so proposal event IDs are deterministic across sub-second writes.
// This is a divergent variant, not the shared trunk primitive.
func normalizeNow(now time.Time) time.Time {
	if now.IsZero() {
		now = time.Now()
	}
	return now.UTC().Truncate(time.Second)
}

func eventID(proposalID, typ string, now time.Time) string {
	base := cleanID(proposalID)
	event := strings.ReplaceAll(typ, ".", "_")
	return fmt.Sprintf("evt_%s_%s_%d", base, event, now.UnixNano())
}

func eventIDAttempt(base string, attempt int) string {
	if attempt == 0 {
		return base
	}
	return fmt.Sprintf("%s_%d", base, attempt+1)
}

func generatedID(title string, now time.Time) string {
	base := cleanID(title)
	if base == "" {
		base = "proposal"
	}
	return fmt.Sprintf("%s_%s", base, now.UTC().Format("20060102_150405"))
}

var idCleaner = regexp.MustCompile(`[^a-z0-9_.-]+`)

func cleanID(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = idCleaner.ReplaceAllString(value, "-")
	value = strings.Trim(value, "-_.")
	return value
}

func writeJSONAtomic(path string, value any, mode os.FileMode) error {
	return layout.WriteJSONAtomic(path, value, mode)
}
