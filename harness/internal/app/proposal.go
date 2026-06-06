package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	harnesseval "github.com/mnemon-dev/mnemon/harness/internal/eval"
	"github.com/mnemon-dev/mnemon/harness/internal/lifecycle/auditstore"
	"github.com/mnemon-dev/mnemon/harness/internal/lifecycle/coreengine"
	"github.com/mnemon-dev/mnemon/harness/internal/lifecycle/layout"
	"github.com/mnemon-dev/mnemon/harness/internal/lifecycle/profile"
	"github.com/mnemon-dev/mnemon/harness/internal/lifecycle/proposal"
	"github.com/mnemon-dev/mnemon/harness/internal/lifecycle/proposalstore"
	"github.com/mnemon-dev/mnemon/harness/internal/lifecycle/schema"
)

// ErrProposalApplyNotImplemented is wrapped by ProposalApply: an approved
// proposal records a boundary audit but apply itself is not yet implemented.
var ErrProposalApplyNotImplemented = errors.New("not_implemented: proposal apply is not implemented")

var errUnsupportedMemoryApply = errors.New("unsupported memory proposal apply")

// ProposalContent is the facade-side mirror of the proposal content flags (raw
// strings); the facade parses them into proposal types so the surface need not
// import the proposal package.
type ProposalContent struct {
	Title              string
	Summary            string
	ChangeSummary      string
	Targets            []string
	Operations         []string
	Evidence           []string
	ValidationSummary  string
	ValidationCommands []string
	ValidationChecks   []string
	ReviewRequired     bool
	ReviewScope        string
	RequiredReviews    int
	Reviewers          []string
	ReviewNotes        string
	ScopeStore         string
	ScopeHost          string
	ScopeLoop          string
	ScopeProfileRef    string
}

func (h *Harness) ProposalCreate(out io.Writer, id, route, risk string, c ProposalContent) error {
	store, err := proposalstore.New(h.root)
	if err != nil {
		return err
	}
	opts, err := buildProposalCreateOptions(h.root, id, route, risk, c)
	if err != nil {
		return err
	}
	item, err := store.Create(opts)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "created proposal %s (%s)\n", item.ID, item.Status)
	return nil
}

func (h *Harness) ProposalList(out io.Writer, statuses []string, format string) error {
	store, err := proposalstore.New(h.root)
	if err != nil {
		return err
	}
	parsed, err := proposalStatuses(statuses)
	if err != nil {
		return err
	}
	items, err := store.List(parsed...)
	if err != nil {
		return err
	}
	if format == "json" {
		return writeJSON(out, items)
	}
	if format != "" && format != "text" {
		return fmt.Errorf("unsupported --format %q", format)
	}
	for _, item := range items {
		fmt.Fprintf(out, "%s\t%s\t%s\t%s\t%s\n", item.ID, item.Status, item.Route, item.Risk, item.Title)
	}
	return nil
}

func (h *Harness) ProposalShow(out io.Writer, id, format string) error {
	store, err := proposalstore.New(h.root)
	if err != nil {
		return err
	}
	item, err := store.Load(id)
	if err != nil {
		return err
	}
	if format == "json" {
		return writeJSON(out, item)
	}
	if format != "" && format != "text" {
		return fmt.Errorf("unsupported --format %q", format)
	}
	writeProposalText(out, item)
	return nil
}

func (h *Harness) ProposalUpdate(out io.Writer, id, status, supersededBy string, c ProposalContent) error {
	store, err := proposalstore.New(h.root)
	if err != nil {
		return err
	}
	item := proposal.Proposal{}
	if proposalContentPresent(c, supersededBy) {
		updateOpts, err := buildProposalUpdateOptions(h.root, id, supersededBy, c)
		if err != nil {
			return err
		}
		item, err = store.Update(updateOpts)
		if err != nil {
			return err
		}
		fmt.Fprintf(out, "updated proposal %s (%s)\n", item.ID, item.Status)
	}
	if strings.TrimSpace(status) != "" {
		st, err := proposalStatusValue(status)
		if err != nil {
			return err
		}
		item, err = store.Transition(proposalstore.TransitionOptions{
			ID:     id,
			Status: st,
		})
		if err != nil {
			return err
		}
		fmt.Fprintf(out, "transitioned proposal %s to %s\n", item.ID, item.Status)
		return nil
	}
	if item.ID == "" {
		return errors.New("no proposal updates supplied")
	}
	return nil
}

// ProposalTransition validates the target status string and transitions the
// proposal to it. The per-status CLI verbs (approve / reject / request-changes /
// block / withdraw / expire) call this with their canonical status value.
func (h *Harness) ProposalTransition(out io.Writer, id, status string) error {
	st, err := proposalStatusValue(status)
	if err != nil {
		return err
	}
	store, err := proposalstore.New(h.root)
	if err != nil {
		return err
	}
	item, err := store.Transition(proposalstore.TransitionOptions{
		ID:     id,
		Status: st,
	})
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "proposal %s: %s\n", item.ID, item.Status)
	return nil
}

func (h *Harness) ProposalApply(out io.Writer, id string) error {
	store, err := proposalstore.New(h.root)
	if err != nil {
		return err
	}
	item, err := store.Load(id)
	if err != nil {
		return err
	}
	if item.Status != proposal.StatusApproved {
		return fmt.Errorf("proposal %s must be approved before apply; current status is %s", item.ID, item.Status)
	}
	if item.Route == proposal.RouteMemory {
		err := h.applyMemoryProposal(out, store, item)
		if errors.Is(err, errUnsupportedMemoryApply) {
			if auditErr := h.recordProposalApplyBoundaryAudit(item); auditErr != nil {
				return auditErr
			}
			return fmt.Errorf("%w for route %s: %v", ErrProposalApplyNotImplemented, item.Route, err)
		}
		return err
	}
	if item.Route == proposal.RouteEval {
		return h.applyEvalProposal(out, store, item)
	}
	if item.Route == proposal.RouteCoordination {
		err := h.applyCoordinationProposal(out, store, item)
		if errors.Is(err, errUnsupportedCoordinationApply) {
			if auditErr := h.recordProposalApplyBoundaryAudit(item); auditErr != nil {
				return auditErr
			}
			return fmt.Errorf("%w for route %s: %v", ErrProposalApplyNotImplemented, item.Route, err)
		}
		return err
	}
	if err := h.recordProposalApplyBoundaryAudit(item); err != nil {
		return err
	}
	return fmt.Errorf("%w for route %s", ErrProposalApplyNotImplemented, item.Route)
}

type evalProposalTarget struct {
	Kind harnesseval.EvalAssetKind
	ID   string
	URI  string
}

type memoryProfileEntrySpec struct {
	ProfileID         string
	ProfileRef        string
	EntryID           string
	EntryType         string
	Summary           string
	Content           string
	Evidence          []profile.EvidenceRef
	ProjectionTargets []profile.ProjectionTarget
	OperationSummary  string
}

func (h *Harness) applyMemoryProposal(out io.Writer, store *proposalstore.Store, item proposal.Proposal) error {
	spec, err := memoryProfileEntrySpecFromProposal(item)
	if err != nil {
		return err
	}
	if err := h.ensureMemoryProfileEntryCanApply(spec); err != nil {
		return err
	}
	// P2.2 (D1): lower the approved entry to a governed kernel write. The canonical memory
	// resource is created by Kernel.Apply through the channel (ServerAPI.Ingest -> rule
	// pre-gate -> bridge write-scope -> kernel single-writer); the host profile file below
	// is materialized only AFTER the kernel accepts, so it is a mirror of the canonical
	// state, never an independent writer. A kernel denial (duplicate at the gate, malformed,
	// unauthorized) aborts the apply before any file is touched.
	if err := h.governMemoryEntry(item.ID, spec); err != nil {
		return err
	}
	now := time.Now().UTC()
	auditResult, err := h.recordMemoryProfileEntryApplyAudit(item, spec, now)
	if err != nil {
		return err
	}
	auditURI := auditRefURI(auditResult.Ref)
	if auditURI == "" {
		return fmt.Errorf("apply audit for proposal %s did not produce a uri ref", item.ID)
	}
	profiles, err := profile.New(h.root)
	if err != nil {
		return err
	}
	_, entry, err := profiles.AddEntry(profile.AddEntryOptions{
		ProfileID:         spec.ProfileID,
		EntryID:           spec.EntryID,
		Type:              spec.EntryType,
		Summary:           spec.Summary,
		Content:           spec.Content,
		Evidence:          spec.Evidence,
		ProjectionTargets: spec.ProjectionTargets,
		Now:               now,
	})
	if err != nil {
		return err
	}
	if err := h.recordMemoryProfileEntryApplyAuditEvent(item, spec, entry.ID, auditResult, now); err != nil {
		return err
	}
	if _, err := store.AppendAuditRef(proposalstore.AppendRefOptions{
		ID:       item.ID,
		AuditRef: auditURI,
		Now:      now,
	}); err != nil {
		return err
	}
	applied, err := store.Transition(proposalstore.TransitionOptions{
		ID:     item.ID,
		Status: proposal.StatusApplied,
		Now:    now,
	})
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "proposal %s applied\n", applied.ID)
	fmt.Fprintf(out, "route: %s\n", applied.Route)
	fmt.Fprintf(out, "profile entry: %s %s\n", spec.ProfileRef, entry.ID)
	fmt.Fprintf(out, "audit: %s\n", auditURI)
	return nil
}

// governMemoryEntry lowers the approved memory entry to a governed kernel write (D1): the
// kernel is the single writer of the canonical memory resource (keyed profileID/entryID).
// A non-Accepted decision aborts the apply with the kernel's reason, so no host file is
// materialized for a write the kernel refused.
func (h *Harness) governMemoryEntry(applyID string, spec memoryProfileEntrySpec) error {
	paths, err := layout.Resolve(h.root)
	if err != nil {
		return err
	}
	engine := coreengine.NewMemoryEngine(paths.HarnessDir,
		func() string { return uuid.NewString() },
		func() string { return time.Now().UTC().Format(time.RFC3339) })
	res, err := engine.AdmitEntry(applyID, spec.ProfileID+"/"+spec.EntryID, map[string]any{
		"content":    spec.Content,
		"summary":    spec.Summary,
		"entry_type": spec.EntryType,
		"profile_id": spec.ProfileID,
		"entry_id":   spec.EntryID,
	})
	if err != nil {
		return fmt.Errorf("lower memory entry to kernel: %w", err)
	}
	if !res.Accepted {
		return fmt.Errorf("kernel denied memory entry %q: %s", spec.EntryID, res.Reason)
	}
	return nil
}

func (h *Harness) ensureMemoryProfileEntryCanApply(spec memoryProfileEntrySpec) error {
	profiles, err := profile.New(h.root)
	if err != nil {
		return err
	}
	prof, err := profiles.Load(spec.ProfileID)
	if errors.Is(err, profile.ErrProfileNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, entry := range prof.Entries {
		if entry.ID == spec.EntryID {
			return fmt.Errorf("profile entry %q already exists in %s", spec.EntryID, spec.ProfileRef)
		}
	}
	return nil
}

func (h *Harness) applyEvalProposal(out io.Writer, store *proposalstore.Store, item proposal.Proposal) error {
	target, err := evalTargetFromProposal(item)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	if _, err := harnesseval.ResolveEvalAsset(h.root, target.Kind, target.ID); err != nil {
		return err
	}
	auditResult, err := h.recordEvalProposalApplyAudit(item, target, now)
	if err != nil {
		return err
	}
	auditURI := auditRefURI(auditResult.Ref)
	if auditURI == "" {
		return fmt.Errorf("apply audit for proposal %s did not produce a uri ref", item.ID)
	}
	result, err := harnesseval.PromoteAsset(h.root, harnesseval.PromotionOptions{
		Kind:          target.Kind,
		ID:            target.ID,
		Target:        harnesseval.EvalAssetPromoted,
		ProposalRef:   item.ID,
		AuditRef:      auditURI,
		EventID:       fmt.Sprintf("evt_proposal_%s_eval_apply_%d", item.ID, now.UnixNano()),
		CorrelationID: "proposal:" + item.ID,
		Actor:         "mnemon-manual",
		Source:        "proposal.apply",
		Now:           now,
	})
	if err != nil {
		return err
	}
	if err := h.recordEvalProposalApplyAuditEvent(item, target, auditResult, result.Event.ID, now); err != nil {
		return err
	}
	if _, err := store.AppendAuditRef(proposalstore.AppendRefOptions{
		ID:       item.ID,
		AuditRef: auditURI,
		Now:      now,
	}); err != nil {
		return err
	}
	applied, err := store.Transition(proposalstore.TransitionOptions{
		ID:     item.ID,
		Status: proposal.StatusApplied,
		Now:    now,
	})
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "proposal %s applied\n", applied.ID)
	fmt.Fprintf(out, "route: %s\n", applied.Route)
	fmt.Fprintf(out, "eval asset: %s %s\n", result.Asset.Kind, result.Asset.ID)
	fmt.Fprintf(out, "event: %s\n", result.Event.ID)
	fmt.Fprintf(out, "audit: %s\n", auditURI)
	return nil
}

func evalTargetFromProposal(item proposal.Proposal) (evalProposalTarget, error) {
	var targets []proposal.TargetRef
	for _, target := range item.Change.Targets {
		if strings.TrimSpace(target.Type) == "eval_asset" {
			targets = append(targets, target)
		}
	}
	if len(targets) != 1 {
		return evalProposalTarget{}, fmt.Errorf("eval proposal apply requires exactly one eval_asset target, got %d", len(targets))
	}
	kind, id, err := evalAssetTargetURI(targets[0].URI)
	if err != nil {
		return evalProposalTarget{}, err
	}
	return evalProposalTarget{
		Kind: kind,
		ID:   id,
		URI:  strings.TrimSpace(targets[0].URI),
	}, nil
}

func evalAssetTargetURI(uri string) (harnesseval.EvalAssetKind, string, error) {
	cleaned := filepath.ToSlash(filepath.Clean(strings.TrimSpace(uri)))
	cleaned = strings.TrimPrefix(cleaned, "./")
	if cleaned == "." || cleaned == "" {
		return "", "", fmt.Errorf("eval asset target uri is required")
	}
	type prefix struct {
		path string
		kind harnesseval.EvalAssetKind
	}
	for _, candidate := range []prefix{
		{path: "harness/loops/eval/suites/", kind: harnesseval.EvalAssetSuite},
		{path: "harness/loops/eval/scenarios/", kind: harnesseval.EvalAssetScenario},
		{path: "harness/loops/eval/rubrics/", kind: harnesseval.EvalAssetRubric},
	} {
		if strings.HasPrefix(cleaned, candidate.path) {
			id := strings.TrimPrefix(cleaned, candidate.path)
			id = strings.TrimSuffix(id, filepath.Ext(id))
			if id == "" {
				return "", "", fmt.Errorf("eval asset target uri %q has no asset id", uri)
			}
			return candidate.kind, id, nil
		}
	}
	return "", "", fmt.Errorf("eval asset target uri %q must be under harness/loops/eval/{suites,scenarios,rubrics}", uri)
}

func memoryProfileEntrySpecFromProposal(item proposal.Proposal) (memoryProfileEntrySpec, error) {
	var targets []proposal.TargetRef
	for _, target := range item.Change.Targets {
		if strings.TrimSpace(target.Type) == "profile_entry" {
			targets = append(targets, target)
		}
	}
	if len(targets) != 1 {
		return memoryProfileEntrySpec{}, fmt.Errorf("%w: requires exactly one profile_entry target, got %d", errUnsupportedMemoryApply, len(targets))
	}
	profileID, err := profile.ParseProfileRef(targets[0].URI)
	if err != nil {
		return memoryProfileEntrySpec{}, fmt.Errorf("%w: %v", errUnsupportedMemoryApply, err)
	}
	var operations []proposal.Operation
	for _, operation := range item.Change.Operations {
		if strings.TrimSpace(operation.Type) == "profile.entry.add" {
			operations = append(operations, operation)
		}
	}
	if len(operations) != 1 {
		return memoryProfileEntrySpec{}, fmt.Errorf("%w: requires exactly one profile.entry.add operation, got %d", errUnsupportedMemoryApply, len(operations))
	}
	operation := operations[0]
	if strings.TrimSpace(operation.Target) != strings.TrimSpace(targets[0].URI) {
		return memoryProfileEntrySpec{}, fmt.Errorf("%w: operation target %q does not match %q", errUnsupportedMemoryApply, operation.Target, targets[0].URI)
	}
	evidence, err := profileEvidenceFromProposal(item.Evidence)
	if err != nil {
		return memoryProfileEntrySpec{}, err
	}
	entryID := payloadString(operation.Payload, "entry_id")
	entryType := payloadString(operation.Payload, "entry_type")
	summary := payloadString(operation.Payload, "summary")
	content := payloadString(operation.Payload, "content")
	if entryID == "" || entryType == "" || summary == "" || content == "" {
		return memoryProfileEntrySpec{}, errors.New("profile.entry.add payload requires entry_id, entry_type, summary, and content")
	}
	targetsFromPayload, err := profileProjectionTargetsFromPayload(operation.Payload)
	if err != nil {
		return memoryProfileEntrySpec{}, err
	}
	return memoryProfileEntrySpec{
		ProfileID:         profileID,
		ProfileRef:        profile.ProfileRef(profileID),
		EntryID:           entryID,
		EntryType:         entryType,
		Summary:           summary,
		Content:           content,
		Evidence:          evidence,
		ProjectionTargets: targetsFromPayload,
		OperationSummary:  strings.TrimSpace(operation.Summary),
	}, nil
}

func profileEvidenceFromProposal(values []proposal.EvidenceRef) ([]profile.EvidenceRef, error) {
	if len(values) == 0 {
		return nil, errors.New("memory profile apply requires proposal evidence")
	}
	result := make([]profile.EvidenceRef, 0, len(values)+1)
	for _, value := range values {
		ref := profile.EvidenceRef{
			Type:    strings.TrimSpace(value.Type),
			Ref:     strings.TrimSpace(value.Ref),
			Summary: strings.TrimSpace(value.Summary),
		}
		if ref.Type == "" || ref.Ref == "" {
			return nil, errors.New("memory profile apply evidence refs require type and ref")
		}
		result = append(result, ref)
	}
	return result, nil
}

func profileProjectionTargetsFromPayload(payload map[string]any) ([]profile.ProjectionTarget, error) {
	var rawTargets []string
	if values, ok := payload["project_to"]; ok {
		items, err := payloadStringSlice(values, "project_to")
		if err != nil {
			return nil, err
		}
		rawTargets = append(rawTargets, items...)
	}
	targets, err := parseProfileProjectionTargets(rawTargets)
	if err != nil {
		return nil, err
	}
	if values, ok := payload["projection_targets"]; ok {
		items, ok := values.([]any)
		if !ok {
			return nil, errors.New("projection_targets must be an array")
		}
		for _, item := range items {
			object, ok := item.(map[string]any)
			if !ok {
				return nil, errors.New("projection_targets entries must be objects")
			}
			targets = append(targets, profile.ProjectionTarget{
				Host: payloadString(object, "host"),
				Loop: payloadString(object, "loop"),
			})
		}
	}
	for _, target := range targets {
		if strings.TrimSpace(target.Host) == "" || strings.TrimSpace(target.Loop) == "" {
			return nil, errors.New("projection targets require host and loop")
		}
	}
	return targets, nil
}

func payloadString(payload map[string]any, key string) string {
	if payload == nil {
		return ""
	}
	value, ok := payload[key]
	if !ok {
		return ""
	}
	text, ok := value.(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(text)
}

func payloadStringSlice(value any, field string) ([]string, error) {
	items, ok := value.([]any)
	if !ok {
		return nil, fmt.Errorf("%s must be an array", field)
	}
	result := make([]string, 0, len(items))
	for _, item := range items {
		text, ok := item.(string)
		if !ok || strings.TrimSpace(text) == "" {
			return nil, fmt.Errorf("%s entries must be non-empty strings", field)
		}
		result = append(result, strings.TrimSpace(text))
	}
	return result, nil
}

func (h *Harness) recordMemoryProfileEntryApplyAudit(item proposal.Proposal, spec memoryProfileEntrySpec, now time.Time) (auditstore.WriteResult, error) {
	audits, err := auditstore.New(h.root)
	if err != nil {
		return auditstore.WriteResult{}, err
	}
	auditID := fmt.Sprintf("proposal-%s-memory-profile-apply-%s", item.ID, now.Format("20060102T150405000000000"))
	scope := schema.ProjectScopeWithProfile(h.root, "", "", "memory", spec.ProfileRef).Map()
	return audits.Write(auditstore.WriteOptions{
		ID: auditID,
		Labels: map[string]string{
			"audit_kind":  "proposal.apply",
			"proposal_id": item.ID,
			"route":       string(item.Route),
		},
		Spec: map[string]any{
			"audit_kind":        "proposal.apply",
			"proposal_id":       item.ID,
			"route":             string(item.Route),
			"risk":              string(item.Risk),
			"operation":         "profile_entry_add",
			"operation_summary": spec.OperationSummary,
			"profile_id":        spec.ProfileID,
			"profile_ref":       spec.ProfileRef,
			"entry_id":          spec.EntryID,
			"entry_type":        spec.EntryType,
			"outcome":           "applied",
			"scope":             scope,
		},
	})
}

func (h *Harness) recordMemoryProfileEntryApplyAuditEvent(item proposal.Proposal, spec memoryProfileEntrySpec, entryID string, auditResult auditstore.WriteResult, now time.Time) error {
	audits, err := auditstore.New(h.root)
	if err != nil {
		return err
	}
	_, err = audits.AppendRecordedEvent(auditstore.RecordedEventOptions{
		ID:            fmt.Sprintf("evt_proposal_%s_memory_profile_apply_audit_recorded_%d", item.ID, now.UnixNano()),
		Now:           now,
		Actor:         "mnemon-manual",
		Source:        "proposal.apply",
		CorrelationID: "proposal:" + item.ID,
		Loop:          "memory",
		Payload: map[string]any{
			"audit_kind":  "proposal.apply",
			"proposal_id": item.ID,
			"route":       string(item.Route),
			"outcome":     "applied",
			"operation":   "profile_entry_add",
			"profile_id":  spec.ProfileID,
			"profile_ref": spec.ProfileRef,
			"entry_id":    entryID,
			"entry_type":  spec.EntryType,
		},
		AuditRef: auditResult.Ref,
		Scope:    schema.ProjectScopeWithProfile(h.root, "", "", "memory", spec.ProfileRef).Map(),
	})
	return err
}

func (h *Harness) recordEvalProposalApplyAudit(item proposal.Proposal, target evalProposalTarget, now time.Time) (auditstore.WriteResult, error) {
	audits, err := auditstore.New(h.root)
	if err != nil {
		return auditstore.WriteResult{}, err
	}
	auditID := fmt.Sprintf("proposal-%s-eval-apply-%s", item.ID, now.Format("20060102T150405000000000"))
	scope := h.evalApplyScope().Map()
	return audits.Write(auditstore.WriteOptions{
		ID: auditID,
		Labels: map[string]string{
			"audit_kind":  "proposal.apply",
			"proposal_id": item.ID,
			"route":       string(item.Route),
		},
		Spec: map[string]any{
			"audit_kind":  "proposal.apply",
			"proposal_id": item.ID,
			"route":       string(item.Route),
			"risk":        string(item.Risk),
			"operation":   "eval_asset_promote",
			"asset_kind":  string(target.Kind),
			"asset_id":    target.ID,
			"asset_uri":   target.URI,
			"to_state":    string(harnesseval.EvalAssetPromoted),
			"outcome":     "applied",
			"scope":       scope,
		},
	})
}

func (h *Harness) recordEvalProposalApplyAuditEvent(item proposal.Proposal, target evalProposalTarget, auditResult auditstore.WriteResult, promotedEventID string, now time.Time) error {
	audits, err := auditstore.New(h.root)
	if err != nil {
		return err
	}
	_, err = audits.AppendRecordedEvent(auditstore.RecordedEventOptions{
		ID:            fmt.Sprintf("evt_proposal_%s_eval_apply_audit_recorded_%d", item.ID, now.UnixNano()),
		Now:           now,
		Actor:         "mnemon-manual",
		Source:        "proposal.apply",
		CorrelationID: "proposal:" + item.ID,
		CausedBy:      promotedEventID,
		Loop:          "eval",
		Payload: map[string]any{
			"audit_kind":        "proposal.apply",
			"proposal_id":       item.ID,
			"route":             string(item.Route),
			"outcome":           "applied",
			"operation":         "eval_asset_promote",
			"asset_kind":        string(target.Kind),
			"asset_id":          target.ID,
			"promoted_event_id": promotedEventID,
		},
		AuditRef: auditResult.Ref,
		Scope:    h.evalApplyScope().Map(),
	})
	return err
}

func auditRefURI(ref map[string]any) string {
	if ref == nil {
		return ""
	}
	if uri, ok := ref["uri"].(string); ok {
		return uri
	}
	return ""
}

// recordProposalApplyBoundaryAudit is the cross-ring composition: it records a
// boundary audit (auditstore) for an approved-but-unimplemented apply, so the
// not_implemented outcome leaves a governed trail.
func (h *Harness) recordProposalApplyBoundaryAudit(item proposal.Proposal) error {
	now := time.Now().UTC()
	audits, err := auditstore.New(h.root)
	if err != nil {
		return err
	}
	auditID := fmt.Sprintf("proposal-%s-apply-boundary-%s", item.ID, now.Format("20060102T150405000000000"))
	result, err := audits.Write(auditstore.WriteOptions{
		ID: auditID,
		Labels: map[string]string{
			"audit_kind":  "proposal.apply_boundary",
			"proposal_id": item.ID,
		},
		Spec: map[string]any{
			"audit_kind":  "proposal.apply_boundary",
			"proposal_id": item.ID,
			"route":       string(item.Route),
			"risk":        string(item.Risk),
			"status":      string(item.Status),
			"outcome":     "not_implemented",
		},
	})
	if err != nil {
		return err
	}
	_, err = audits.AppendRecordedEvent(auditstore.RecordedEventOptions{
		ID:            fmt.Sprintf("evt_proposal_%s_apply_boundary_audit_recorded_%d", item.ID, now.UnixNano()),
		Now:           now,
		Actor:         "mnemon-manual",
		Source:        "proposal.apply",
		CorrelationID: "proposal:" + item.ID,
		Payload: map[string]any{
			"audit_kind":  "proposal.apply_boundary",
			"proposal_id": item.ID,
			"route":       string(item.Route),
			"outcome":     "not_implemented",
		},
		AuditRef: result.Ref,
	})
	return err
}

func (h *Harness) ProposalSupersede(out io.Writer, id, supersededBy string) error {
	if strings.TrimSpace(supersededBy) == "" {
		return errors.New("--superseded-by is required")
	}
	store, err := proposalstore.New(h.root)
	if err != nil {
		return err
	}
	if _, err := store.Update(proposalstore.UpdateOptions{
		ID:           id,
		SupersededBy: supersededBy,
	}); err != nil {
		return err
	}
	item, err := store.Transition(proposalstore.TransitionOptions{
		ID:     id,
		Status: proposal.StatusSuperseded,
	})
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "proposal %s: %s by %s\n", item.ID, item.Status, item.SupersededBy)
	return nil
}

func buildProposalCreateOptions(root, id, routeStr, riskStr string, c ProposalContent) (proposalstore.CreateOptions, error) {
	targets, err := parseProposalTargets(c.Targets)
	if err != nil {
		return proposalstore.CreateOptions{}, err
	}
	operations, err := parseProposalOperations(c.Operations)
	if err != nil {
		return proposalstore.CreateOptions{}, err
	}
	evidence, err := parseProposalEvidence(c.Evidence)
	if err != nil {
		return proposalstore.CreateOptions{}, err
	}
	route, err := proposalRouteValue(routeStr)
	if err != nil {
		return proposalstore.CreateOptions{}, err
	}
	risk, err := proposalRiskValue(riskStr)
	if err != nil {
		return proposalstore.CreateOptions{}, err
	}
	return proposalstore.CreateOptions{
		ID:      id,
		Route:   route,
		Risk:    risk,
		Title:   c.Title,
		Summary: c.Summary,
		Change: proposal.ChangeRequest{
			Summary:    c.ChangeSummary,
			Targets:    targets,
			Operations: operations,
		},
		Evidence: evidence,
		ValidationPlan: proposal.ValidationPlan{
			Summary:  c.ValidationSummary,
			Commands: c.ValidationCommands,
			Checks:   c.ValidationChecks,
		},
		Review: proposalReviewPolicyValue(c, false),
		Scope:  proposalScope(root, route, c).Map(),
	}, nil
}

func buildProposalUpdateOptions(root, id, supersededBy string, c ProposalContent) (proposalstore.UpdateOptions, error) {
	targets, err := parseProposalTargets(c.Targets)
	if err != nil {
		return proposalstore.UpdateOptions{}, err
	}
	operations, err := parseProposalOperations(c.Operations)
	if err != nil {
		return proposalstore.UpdateOptions{}, err
	}
	evidence, err := parseProposalEvidence(c.Evidence)
	if err != nil {
		return proposalstore.UpdateOptions{}, err
	}
	return proposalstore.UpdateOptions{
		ID:                 id,
		Title:              c.Title,
		Summary:            c.Summary,
		ChangeSummary:      c.ChangeSummary,
		Targets:            targets,
		Operations:         operations,
		Evidence:           evidence,
		ValidationSummary:  c.ValidationSummary,
		ValidationCommands: c.ValidationCommands,
		ValidationChecks:   c.ValidationChecks,
		Review:             proposalReviewPolicyPtr(c),
		Scope:              proposalScopeForUpdate(root, c).Map(),
		SupersededBy:       supersededBy,
	}, nil
}

func proposalContentPresent(c ProposalContent, supersededBy string) bool {
	return strings.TrimSpace(c.Title) != "" ||
		strings.TrimSpace(c.Summary) != "" ||
		strings.TrimSpace(c.ChangeSummary) != "" ||
		len(c.Targets) > 0 ||
		len(c.Operations) > 0 ||
		len(c.Evidence) > 0 ||
		strings.TrimSpace(c.ValidationSummary) != "" ||
		len(c.ValidationCommands) > 0 ||
		len(c.ValidationChecks) > 0 ||
		proposalReviewPolicyPresent(c) ||
		proposalScopePresent(c) ||
		strings.TrimSpace(supersededBy) != ""
}

func parseProposalTargets(values []string) ([]proposal.TargetRef, error) {
	result := make([]proposal.TargetRef, 0, len(values))
	for _, value := range values {
		parts := strings.SplitN(value, "=", 2)
		if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
			return nil, fmt.Errorf("target %q must be type=uri", value)
		}
		result = append(result, proposal.TargetRef{
			Type: strings.TrimSpace(parts[0]),
			URI:  strings.TrimSpace(parts[1]),
		})
	}
	return result, nil
}

func parseProposalOperations(values []string) ([]proposal.Operation, error) {
	result := make([]proposal.Operation, 0, len(values))
	for _, value := range values {
		parts := strings.SplitN(value, "=", 4)
		if len(parts) < 3 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" || strings.TrimSpace(parts[2]) == "" {
			return nil, fmt.Errorf("operation %q must be type=target=summary or type=target=summary=json_payload", value)
		}
		payload := map[string]any(nil)
		if len(parts) == 4 {
			if err := json.Unmarshal([]byte(strings.TrimSpace(parts[3])), &payload); err != nil {
				return nil, fmt.Errorf("operation %q payload must be JSON object: %w", value, err)
			}
			if payload == nil {
				return nil, fmt.Errorf("operation %q payload must be JSON object", value)
			}
		}
		result = append(result, proposal.Operation{
			Type:    strings.TrimSpace(parts[0]),
			Target:  strings.TrimSpace(parts[1]),
			Summary: strings.TrimSpace(parts[2]),
			Payload: payload,
		})
	}
	return result, nil
}

func parseProposalEvidence(values []string) ([]proposal.EvidenceRef, error) {
	result := make([]proposal.EvidenceRef, 0, len(values))
	for _, value := range values {
		parts := strings.SplitN(value, "=", 3)
		if len(parts) < 2 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
			return nil, fmt.Errorf("evidence %q must be type=ref or type=ref=summary", value)
		}
		ref := proposal.EvidenceRef{
			Type: strings.TrimSpace(parts[0]),
			Ref:  strings.TrimSpace(parts[1]),
		}
		if len(parts) == 3 {
			ref.Summary = strings.TrimSpace(parts[2])
		}
		result = append(result, ref)
	}
	return result, nil
}

func proposalStatuses(values []string) ([]proposal.Status, error) {
	result := make([]proposal.Status, 0, len(values))
	for _, value := range values {
		status, err := proposalStatusValue(value)
		if err != nil {
			return nil, err
		}
		result = append(result, status)
	}
	return result, nil
}

func proposalStatusValue(value string) (proposal.Status, error) {
	status := proposal.Status(strings.TrimSpace(value))
	if err := proposal.ValidateStatus(status); err != nil {
		return "", err
	}
	return status, nil
}

func proposalRouteValue(value string) (proposal.Route, error) {
	route := proposal.Route(strings.TrimSpace(value))
	if err := proposal.ValidateRoute(route); err != nil {
		return "", err
	}
	return route, nil
}

func proposalRiskValue(value string) (proposal.Risk, error) {
	risk := proposal.Risk(strings.TrimSpace(value))
	if err := proposal.ValidateRisk(risk); err != nil {
		return "", err
	}
	return risk, nil
}

func proposalReviewPolicyValue(c ProposalContent, force bool) proposal.ReviewPolicy {
	if !force && !proposalReviewPolicyPresent(c) {
		return proposal.ReviewPolicy{}
	}
	required := c.ReviewRequired ||
		strings.TrimSpace(c.ReviewScope) != "" ||
		c.RequiredReviews > 0 ||
		len(c.Reviewers) > 0 ||
		strings.TrimSpace(c.ReviewNotes) != ""
	scope := strings.TrimSpace(c.ReviewScope)
	if required && scope == "" {
		scope = "exact"
	}
	requiredReviews := c.RequiredReviews
	if required && requiredReviews == 0 {
		requiredReviews = 1
	}
	return proposal.ReviewPolicy{
		Required:        required,
		RequiredScope:   scope,
		RequiredReviews: requiredReviews,
		Reviewers:       c.Reviewers,
		Notes:           c.ReviewNotes,
	}
}

func proposalReviewPolicyPtr(c ProposalContent) *proposal.ReviewPolicy {
	if !proposalReviewPolicyPresent(c) {
		return nil
	}
	policy := proposalReviewPolicyValue(c, true)
	return &policy
}

func proposalReviewPolicyPresent(c ProposalContent) bool {
	return c.ReviewRequired ||
		strings.TrimSpace(c.ReviewScope) != "" ||
		c.RequiredReviews != 0 ||
		len(c.Reviewers) > 0 ||
		strings.TrimSpace(c.ReviewNotes) != ""
}

func proposalScope(root string, route proposal.Route, c ProposalContent) schema.ScopeRef {
	loop := strings.TrimSpace(c.ScopeLoop)
	if loop == "" {
		switch route {
		case proposal.RouteMemory, proposal.RouteSkill, proposal.RouteEval:
			loop = string(route)
		}
	}
	return schema.ProjectScopeWithProfile(root, c.ScopeStore, c.ScopeHost, loop, c.ScopeProfileRef)
}

func proposalScopeForUpdate(root string, c ProposalContent) schema.ScopeRef {
	if !proposalScopePresent(c) {
		return schema.ScopeRef{}
	}
	return schema.ProjectScopeWithProfile(root, c.ScopeStore, c.ScopeHost, c.ScopeLoop, c.ScopeProfileRef)
}

func proposalScopePresent(c ProposalContent) bool {
	return strings.TrimSpace(c.ScopeStore) != "" ||
		strings.TrimSpace(c.ScopeHost) != "" ||
		strings.TrimSpace(c.ScopeLoop) != "" ||
		strings.TrimSpace(c.ScopeProfileRef) != ""
}

func (h *Harness) evalApplyScope() schema.ScopeRef {
	return schema.ProjectScopeWithProfile(h.root, "", "", "eval", "")
}

func writeProposalText(out io.Writer, item proposal.Proposal) {
	fmt.Fprintf(out, "proposal %s: %s\n", item.ID, item.Status)
	fmt.Fprintf(out, "route: %s\n", item.Route)
	fmt.Fprintf(out, "risk: %s\n", item.Risk)
	fmt.Fprintf(out, "title: %s\n", item.Title)
	fmt.Fprintf(out, "summary: %s\n", item.Summary)
	fmt.Fprintf(out, "change: %s\n", item.Change.Summary)
	fmt.Fprintf(out, "targets: %d\n", len(item.Change.Targets))
	fmt.Fprintf(out, "evidence: %d\n", len(item.Evidence))
	fmt.Fprintf(out, "validation: %s\n", item.ValidationPlan.Summary)
	if len(item.Scope) > 0 {
		fmt.Fprintf(out, "scope: %v\n", item.Scope)
	}
	if item.SupersededBy != "" {
		fmt.Fprintf(out, "superseded_by: %s\n", item.SupersededBy)
	}
}
