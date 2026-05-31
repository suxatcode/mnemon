package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/mnemon-dev/mnemon/harness/internal/lifecycle/auditstore"
	"github.com/mnemon-dev/mnemon/harness/internal/lifecycle/coordination"
	"github.com/mnemon-dev/mnemon/harness/internal/lifecycle/eventlog"
	"github.com/mnemon-dev/mnemon/harness/internal/lifecycle/proposal"
	"github.com/mnemon-dev/mnemon/harness/internal/lifecycle/proposalstore"
	"github.com/mnemon-dev/mnemon/harness/internal/lifecycle/schema"
	"github.com/mnemon-dev/mnemon/harness/internal/supervisor"
)

// errUnsupportedCoordinationApply marks a coordination proposal whose operation
// the executor does not implement; ProposalApply records a boundary audit and
// returns not_implemented, mirroring the memory route.
var errUnsupportedCoordinationApply = errors.New("unsupported coordination proposal apply")

// CoordinationContext assembles the supervisor read contract: the materialized
// topology plus the coordination proposals already awaiting review, so a
// pluggable host-agent supervisor can reason without re-folding the log or
// duplicating work already in the queue. Read-only.
func (h *Harness) CoordinationContext(out io.Writer, format string) error {
	ctx, err := h.coordinationContext()
	if err != nil {
		return err
	}
	switch format {
	case "json", "":
		return writeJSON(out, ctx)
	default:
		return fmt.Errorf("unsupported --format %q", format)
	}
}

func (h *Harness) coordinationContext() (supervisor.Context, error) {
	store, err := eventlog.New(h.root)
	if err != nil {
		return supervisor.Context{}, err
	}
	events, _ := store.ReadAll()
	ctx := supervisor.Context{Topology: coordination.DeriveView(events)}

	pstore, err := proposalstore.New(h.root)
	if err != nil {
		return supervisor.Context{}, err
	}
	open, err := pstore.List(proposal.StatusDraft, proposal.StatusOpen, proposal.StatusInReview, proposal.StatusApproved)
	if err != nil {
		return supervisor.Context{}, err
	}
	for _, p := range open {
		if p.Route != proposal.RouteCoordination {
			continue
		}
		ctx.OpenProposals = append(ctx.OpenProposals, supervisor.OpenProposal{
			ID:        p.ID,
			Route:     string(p.Route),
			Status:    string(p.Status),
			TargetURI: firstTargetURI(p),
		})
	}
	return ctx, nil
}

func firstTargetURI(p proposal.Proposal) string {
	if len(p.Change.Targets) > 0 {
		return p.Change.Targets[0].URI
	}
	return ""
}

// SupervisorPropose runs the configured (pluggable) advisory supervisor over the
// coordination context and lands its suggestions as route=coordination proposals
// in the review queue. The supervisor only PROPOSES: this creates proposals and
// nothing else — no topology event, no audit. The change is applied later only
// through review -> apply -> audit. Swapping the supervisor is a config change
// (the kind), not a code change at this call site.
func (h *Harness) SupervisorPropose(out io.Writer, kind string) error {
	sup, err := supervisor.FromConfig(supervisor.Config{Kind: kind})
	if err != nil {
		return err
	}
	ctx, err := h.coordinationContext()
	if err != nil {
		return err
	}
	suggestions := sup.Propose(ctx)
	if len(suggestions) == 0 {
		fmt.Fprintf(out, "supervisor %s: no coordination suggestions\n", sup.Name())
		return nil
	}
	store, err := proposalstore.New(h.root)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	// One run correlation ties this supervisor invocation's proposals + the
	// authorship audit together. The origin is stamped on each proposal so "which
	// supervisor proposed this, reading what context" survives a later config swap
	// (it is append-only and immutable).
	run := fmt.Sprintf("supervisor-%s-%d", sup.Name(), now.UnixNano())
	origin := map[string]any{
		"supervisor_kind": sup.Name(),
		"supervisor_host": "", // in-core rule-standin is mnemon-originated; an external host-agent carries its host
		"supervisor_run":  run,
		"via":             "supervisor.propose",
	}
	var created []string
	for _, s := range suggestions {
		opts, err := coordinationProposalCreateOptions(h.root, s, origin)
		if err != nil {
			return err
		}
		item, err := store.Create(opts)
		if err != nil {
			// A duplicate id means the suggestion is already queued; skip it.
			if strings.Contains(err.Error(), "already exists") {
				continue
			}
			return err
		}
		created = append(created, item.ID)
		fmt.Fprintf(out, "supervisor %s proposed %s (route=%s, status=%s)\n", sup.Name(), item.ID, item.Route, item.Status)
	}
	if len(created) == 0 {
		fmt.Fprintf(out, "supervisor %s: all suggestions already in the queue\n", sup.Name())
		return nil
	}
	if err := h.recordSupervisorAuthorshipAudit(sup.Name(), run, ctx, created, now); err != nil {
		return err
	}
	return nil
}

func coordinationProposalCreateOptions(root string, s supervisor.Suggestion, origin map[string]any) (proposalstore.CreateOptions, error) {
	content := ProposalContent{
		Title:             s.Title,
		Summary:           s.Summary,
		ChangeSummary:     s.Summary,
		Targets:           []string{"coordination=" + s.TargetURI},
		ValidationSummary: "Human review of the coordination change before apply.",
		ReviewRequired:    true,
		ReviewScope:       "project",
	}
	op := s.Operation + "=" + s.TargetURI + "=" + s.Title
	if len(s.Payload) > 0 {
		payload, err := json.Marshal(s.Payload)
		if err != nil {
			return proposalstore.CreateOptions{}, err
		}
		op += "=" + string(payload)
	}
	content.Operations = []string{op}
	for _, ref := range s.EvidenceRefs {
		content.Evidence = append(content.Evidence, "coordination="+ref+"=supervisor evidence")
	}
	opts, err := buildProposalCreateOptions(root, s.ProposalID, string(proposal.RouteCoordination), "medium", content)
	if err != nil {
		return opts, err
	}
	if len(origin) > 0 {
		opts.Metadata = map[string]any{"authorship": origin}
	}
	return opts, nil
}

// recordSupervisorAuthorshipAudit records which supervisor authored a run's
// proposals and the context it read, as a governed audit + audit.recorded event
// (so the authorship is in the evidence stream and integrity-linked). This is the
// accountability half of P3.4; the proposals themselves carry the same origin in
// metadata. It is not a topology mutation — the supervisor still only proposes.
func (h *Harness) recordSupervisorAuthorshipAudit(kind, run string, ctx supervisor.Context, proposalIDs []string, now time.Time) error {
	audits, err := auditstore.New(h.root)
	if err != nil {
		return err
	}
	refs := make([]any, len(proposalIDs))
	for i, id := range proposalIDs {
		refs[i] = id
	}
	contextDigest := map[string]any{
		"tasks":            len(ctx.Topology.Tasks),
		"merge_candidates": len(ctx.Topology.MergeCandidates),
		"conflicts":        len(ctx.Topology.Conflicts),
		"open_proposals":   len(ctx.OpenProposals),
	}
	result, err := audits.Write(auditstore.WriteOptions{
		ID: run + "-authorship",
		Labels: map[string]string{
			"audit_kind":      "supervisor.proposed",
			"supervisor_kind": kind,
		},
		Spec: map[string]any{
			"audit_kind":      "supervisor.proposed",
			"supervisor_kind": kind,
			"supervisor_host": "",
			"supervisor_run":  run,
			"proposal_refs":   refs,
			"proposals":       len(proposalIDs),
			"context":         contextDigest,
		},
	})
	if err != nil {
		return err
	}
	_, err = audits.AppendRecordedEvent(auditstore.RecordedEventOptions{
		ID:            fmt.Sprintf("evt_%s_supervisor_proposed_%d", run, now.UnixNano()),
		Now:           now,
		Actor:         "mnemon-manual",
		Source:        "supervisor.propose",
		CorrelationID: run,
		Loop:          "coordination",
		Payload: map[string]any{
			"audit_kind":      "supervisor.proposed",
			"supervisor_kind": kind,
			"supervisor_run":  run,
			"proposal_ids":    proposalIDs,
		},
		AuditRef: result.Ref,
		Scope:    schema.ProjectScopeWithProfile(h.root, "", "", "coordination", "").Map(),
	})
	return err
}

// coordinationSpec is the parsed apply intent of a route=coordination proposal:
// one operation against one narrow target, with a structured payload.
type coordinationSpec struct {
	Operation    string
	Target       string
	Payload      map[string]any
	EvidenceRefs []string
}

func coordinationSpecFromProposal(item proposal.Proposal) (coordinationSpec, error) {
	if len(item.Change.Operations) == 0 {
		return coordinationSpec{}, fmt.Errorf("%w: proposal %s has no operation", errUnsupportedCoordinationApply, item.ID)
	}
	op := item.Change.Operations[0]
	if strings.TrimSpace(op.Type) == "" {
		return coordinationSpec{}, fmt.Errorf("%w: proposal %s operation has no type", errUnsupportedCoordinationApply, item.ID)
	}
	spec := coordinationSpec{Operation: op.Type, Target: op.Target, Payload: op.Payload}
	for _, e := range item.Evidence {
		if strings.TrimSpace(e.Ref) != "" {
			spec.EvidenceRefs = append(spec.EvidenceRefs, e.Ref)
		}
	}
	return spec, nil
}

// applyCoordinationProposal is the route=coordination apply executor: an approved
// proposal becomes one narrow topology mutation (group / merge / link /
// mark-conflict / reassign) emitted as governed coordination event(s), plus an
// audit record + audit.recorded event + proposal audit_ref, then applied.
// Identical contract to the eval and memory routes — the topology is
// event-sourced, so "mutate the topology" means append the governed event.
func (h *Harness) applyCoordinationProposal(out io.Writer, store *proposalstore.Store, item proposal.Proposal) error {
	spec, err := coordinationSpecFromProposal(item)
	if err != nil {
		return err
	}
	now := time.Now().UTC()

	// Apply-time re-validation: re-derive the CURRENT topology and confirm the op
	// still applies. Between approval and apply the topology may have moved (another
	// proposal applied), so a stale op must be rejected — not blindly emitted.
	view, err := h.currentCoordinationView()
	if err != nil {
		return err
	}
	outcome, reason := coordinationApplies(spec, view)
	if outcome == applyInvalid {
		if auditErr := h.recordCoordinationStaleAudit(item, spec, reason, now); auditErr != nil {
			return auditErr
		}
		return fmt.Errorf("coordination apply rejected: %s — proposal %s no longer applies to the current topology", reason, item.ID)
	}

	auditResult, err := h.recordCoordinationApplyAudit(item, spec, outcome, now)
	if err != nil {
		return err
	}
	auditURI := auditRefURI(auditResult.Ref)
	if auditURI == "" {
		return fmt.Errorf("apply audit for proposal %s did not produce a uri ref", item.ID)
	}

	// Idempotency: when the desired state already holds, apply emits NO topology
	// event — re-applying an already-satisfied op changes nothing.
	var emitted []string
	if outcome == applyApplies {
		emitted, err = h.emitCoordinationMutation(item, spec, auditResult.Ref, now)
		if err != nil {
			return err
		}
	}
	if err := h.recordCoordinationApplyAuditEvent(item, spec, emitted, auditResult, now); err != nil {
		return err
	}
	if _, err := store.AppendAuditRef(proposalstore.AppendRefOptions{ID: item.ID, AuditRef: auditURI, Now: now}); err != nil {
		return err
	}
	applied, err := store.Transition(proposalstore.TransitionOptions{ID: item.ID, Status: proposal.StatusApplied, Now: now})
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "proposal %s applied\n", applied.ID)
	fmt.Fprintf(out, "route: %s\n", applied.Route)
	if outcome == applySatisfied {
		fmt.Fprintf(out, "coordination: %s already satisfied — idempotent (0 new topology events)\n", spec.Operation)
	} else {
		fmt.Fprintf(out, "coordination: %s applied as %d topology event(s)\n", spec.Operation, len(emitted))
	}
	fmt.Fprintf(out, "audit: %s\n", auditURI)
	return nil
}

const (
	applyApplies   = "applied"
	applySatisfied = "already_satisfied"
	applyInvalid   = "invalid"
)

func (h *Harness) currentCoordinationView() (coordination.View, error) {
	store, err := eventlog.New(h.root)
	if err != nil {
		return coordination.View{}, err
	}
	events, _ := store.ReadAll()
	return coordination.DeriveView(events), nil
}

// coordinationApplies re-checks a coordination op against the current topology:
// "applied" (proceed and emit), "already_satisfied" (idempotent no-op), or
// "invalid" (stale/conflicting — reject with a reason).
func coordinationApplies(spec coordinationSpec, view coordination.View) (string, string) {
	tasks := map[string]coordination.Task{}
	for _, t := range view.Tasks {
		tasks[t.ID] = t
	}
	groups := map[string]coordination.Group{}
	for _, g := range view.Groups {
		groups[g.ID] = g
	}
	switch spec.Operation {
	case supervisor.OpMerge:
		into := coordPayloadString(spec.Payload, "into")
		if into == "" {
			return applyInvalid, "merge has no 'into' target"
		}
		pending := 0
		for _, tk := range coordPayloadStrings(spec.Payload, "tasks") {
			if tk == into {
				continue
			}
			t, ok := tasks[tk]
			if ok && t.Status == "joined" && t.JoinedInto != "" && t.JoinedInto != into {
				return applyInvalid, fmt.Sprintf("task %s is already joined into %s", tk, t.JoinedInto)
			}
			if ok && t.Status == "joined" && t.JoinedInto == into {
				continue // already merged into the requested target
			}
			pending++
		}
		if pending == 0 {
			return applySatisfied, "all tasks already merged into " + into
		}
		return applyApplies, ""
	case "coordination.link":
		if hasEvidenceRef(tasks[coordPayloadString(spec.Payload, "task_id")], coordPayloadString(spec.Payload, "evidence_ref")) {
			return applySatisfied, "evidence already linked"
		}
		return applyApplies, ""
	case "coordination.unlink":
		if !hasEvidenceRef(tasks[coordPayloadString(spec.Payload, "task_id")], coordPayloadString(spec.Payload, "evidence_ref")) {
			return applySatisfied, "evidence already unlinked"
		}
		return applyApplies, ""
	case "coordination.member_add":
		if groupHasMember(groups[coordPayloadString(spec.Payload, "group_id")], coordPayloadString(spec.Payload, "member")) {
			return applySatisfied, "member already in group"
		}
		return applyApplies, ""
	case "coordination.member_remove":
		if !groupHasMember(groups[coordPayloadString(spec.Payload, "group_id")], coordPayloadString(spec.Payload, "member")) {
			return applySatisfied, "member already absent from group"
		}
		return applyApplies, ""
	case "coordination.reassign":
		if t, ok := tasks[coordPayloadString(spec.Payload, "task_id")]; ok && t.Owner == coordPayloadString(spec.Payload, "owner") {
			return applySatisfied, "task already owned by " + t.Owner
		}
		return applyApplies, ""
	case supervisor.OpMarkConflict:
		a, b := coordPayloadString(spec.Payload, "task_id"), coordPayloadString(spec.Payload, "conflict_with")
		for _, c := range view.Conflicts {
			if len(c.Between) == 2 && c.Between[0] == a && c.Between[1] == b {
				return applySatisfied, "conflict already recorded"
			}
		}
		return applyApplies, ""
	default:
		// Unknown operation: let emitCoordinationMutation surface the unsupported error.
		return applyApplies, ""
	}
}

func hasEvidenceRef(t coordination.Task, ref string) bool {
	for _, e := range t.EvidenceRefs {
		if e == ref {
			return true
		}
	}
	return false
}

func groupHasMember(g coordination.Group, member string) bool {
	for _, m := range g.Members {
		if m == member {
			return true
		}
	}
	return false
}

// recordCoordinationStaleAudit records a governed rejection (audit + audit.recorded
// event) when a coordination proposal no longer applies to the current topology,
// so a stale reject leaves an accountable trail — mirroring the boundary audit.
func (h *Harness) recordCoordinationStaleAudit(item proposal.Proposal, spec coordinationSpec, reason string, now time.Time) error {
	audits, err := auditstore.New(h.root)
	if err != nil {
		return err
	}
	auditID := fmt.Sprintf("proposal-%s-coordination-rejected-%s", item.ID, now.Format("20060102T150405000000000"))
	result, err := audits.Write(auditstore.WriteOptions{
		ID: auditID,
		Labels: map[string]string{
			"audit_kind":  "proposal.apply_rejected",
			"proposal_id": item.ID,
			"route":       string(item.Route),
		},
		Spec: map[string]any{
			"audit_kind":  "proposal.apply_rejected",
			"proposal_id": item.ID,
			"route":       string(item.Route),
			"operation":   spec.Operation,
			"target":      spec.Target,
			"outcome":     "stale",
			"reason":      reason,
		},
	})
	if err != nil {
		return err
	}
	_, err = audits.AppendRecordedEvent(auditstore.RecordedEventOptions{
		ID:            fmt.Sprintf("evt_proposal_%s_coordination_rejected_%d", item.ID, now.UnixNano()),
		Now:           now,
		Actor:         "mnemon-manual",
		Source:        "proposal.apply",
		CorrelationID: "proposal:" + item.ID,
		Loop:          "coordination",
		Payload: map[string]any{
			"audit_kind":  "proposal.apply_rejected",
			"proposal_id": item.ID,
			"operation":   spec.Operation,
			"outcome":     "stale",
			"reason":      reason,
		},
		AuditRef: result.Ref,
		Scope:    schema.ProjectScopeWithProfile(h.root, "", "", "coordination", "").Map(),
	})
	return err
}

func (h *Harness) recordCoordinationApplyAudit(item proposal.Proposal, spec coordinationSpec, outcome string, now time.Time) (auditstore.WriteResult, error) {
	audits, err := auditstore.New(h.root)
	if err != nil {
		return auditstore.WriteResult{}, err
	}
	auditID := fmt.Sprintf("proposal-%s-coordination-apply-%s", item.ID, now.Format("20060102T150405000000000"))
	scope := schema.ProjectScopeWithProfile(h.root, "", "", "coordination", "").Map()
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
			"operation":   spec.Operation,
			"target":      spec.Target,
			"outcome":     outcome,
			"scope":       scope,
		},
	})
}

func (h *Harness) recordCoordinationApplyAuditEvent(item proposal.Proposal, spec coordinationSpec, emitted []string, auditResult auditstore.WriteResult, now time.Time) error {
	audits, err := auditstore.New(h.root)
	if err != nil {
		return err
	}
	_, err = audits.AppendRecordedEvent(auditstore.RecordedEventOptions{
		ID:            fmt.Sprintf("evt_proposal_%s_coordination_apply_audit_recorded_%d", item.ID, now.UnixNano()),
		Now:           now,
		Actor:         "mnemon-manual",
		Source:        "proposal.apply",
		CorrelationID: "proposal:" + item.ID,
		Loop:          "coordination",
		Payload: map[string]any{
			"audit_kind":        "proposal.apply",
			"proposal_id":       item.ID,
			"route":             string(item.Route),
			"outcome":           "applied",
			"operation":         spec.Operation,
			"target":            spec.Target,
			"emitted_event_ids": emitted,
		},
		AuditRef: auditResult.Ref,
		Scope:    schema.ProjectScopeWithProfile(h.root, "", "", "coordination", "").Map(),
	})
	return err
}

// emitCoordinationMutation appends the governed coordination event(s) that are
// the narrow topology mutation for this operation. Each event is correlated to
// the proposal and carries the apply audit ref, so the trace links proposal →
// apply → topology change.
func (h *Harness) emitCoordinationMutation(item proposal.Proposal, spec coordinationSpec, auditRef map[string]any, now time.Time) ([]string, error) {
	store, err := eventlog.New(h.root)
	if err != nil {
		return nil, err
	}
	type planned struct {
		typ     string
		payload map[string]any
	}
	var plan []planned
	switch spec.Operation {
	case supervisor.OpMerge:
		into := coordPayloadString(spec.Payload, "into")
		if into == "" {
			return nil, fmt.Errorf("%w: merge requires 'into'", errUnsupportedCoordinationApply)
		}
		for _, tk := range coordPayloadStrings(spec.Payload, "tasks") {
			if tk == into {
				continue
			}
			plan = append(plan, planned{coordination.EventTaskJoined, map[string]any{
				coordination.FieldTaskID:     tk,
				coordination.FieldJoinedInto: into,
			}})
		}
	case supervisor.OpMarkConflict:
		plan = append(plan, planned{coordination.EventConflictDetected, map[string]any{
			coordination.FieldTaskID:       coordPayloadString(spec.Payload, "task_id"),
			coordination.FieldConflictWith: coordPayloadString(spec.Payload, "conflict_with"),
			coordination.FieldReason:       coordPayloadString(spec.Payload, "reason"),
		}})
	case "coordination.link":
		plan = append(plan, planned{coordination.EventEvidenceLinked, map[string]any{
			coordination.FieldTaskID:      coordPayloadString(spec.Payload, "task_id"),
			coordination.FieldEvidenceRef: coordPayloadString(spec.Payload, "evidence_ref"),
		}})
	case "coordination.unlink":
		// Compensation for a wrong link — emit the inverse event (no deletion).
		plan = append(plan, planned{coordination.EventEvidenceUnlinked, map[string]any{
			coordination.FieldTaskID:      coordPayloadString(spec.Payload, "task_id"),
			coordination.FieldEvidenceRef: coordPayloadString(spec.Payload, "evidence_ref"),
		}})
	case "coordination.member_add":
		plan = append(plan, planned{coordination.EventGroupMemberAdded, map[string]any{
			coordination.FieldGroupID: coordPayloadString(spec.Payload, "group_id"),
			coordination.FieldMember:  coordPayloadString(spec.Payload, "member"),
		}})
	case "coordination.member_remove":
		// Compensation for a wrong member — emit the inverse event (no deletion).
		plan = append(plan, planned{coordination.EventGroupMemberRemoved, map[string]any{
			coordination.FieldGroupID: coordPayloadString(spec.Payload, "group_id"),
			coordination.FieldMember:  coordPayloadString(spec.Payload, "member"),
		}})
	case "coordination.reassign":
		plan = append(plan, planned{coordination.EventTaskClaimed, map[string]any{
			coordination.FieldTaskID: coordPayloadString(spec.Payload, "task_id"),
			coordination.FieldOwner:  coordPayloadString(spec.Payload, "owner"),
		}})
	case "coordination.group":
		gid := coordPayloadString(spec.Payload, "group_id")
		plan = append(plan, planned{coordination.EventGroupCreated, map[string]any{coordination.FieldGroupID: gid}})
		for _, m := range coordPayloadStrings(spec.Payload, "members") {
			plan = append(plan, planned{coordination.EventGroupMemberAdded, map[string]any{
				coordination.FieldGroupID: gid,
				coordination.FieldMember:  m,
			}})
		}
	default:
		return nil, fmt.Errorf("%w: operation %q", errUnsupportedCoordinationApply, spec.Operation)
	}
	if len(plan) == 0 {
		return nil, fmt.Errorf("%w: operation %q produced no mutation", errUnsupportedCoordinationApply, spec.Operation)
	}
	var ids []string
	for i, p := range plan {
		base := fmt.Sprintf("evt_proposal_%s_coordination_apply_%d_%d", item.ID, now.UnixNano(), i)
		ev := h.coordinationEvent(p.typ, item, auditRef, now, p.payload)
		id, err := appendCoordinationEvent(store, ev, base)
		if err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, nil
}

func (h *Harness) coordinationEvent(eventType string, item proposal.Proposal, auditRef map[string]any, now time.Time, payload map[string]any) schema.Event {
	loop := "coordination"
	return schema.Event{
		SchemaVersion: schema.Version,
		TS:            now.UTC().Format(time.RFC3339),
		Type:          eventType,
		Loop:          &loop,
		Host:          nil,
		Actor:         "mnemon-manual",
		Source:        "proposal.apply",
		CorrelationID: "proposal:" + item.ID,
		CausedBy:      nil,
		ProjectRoot:   h.root,
		Scope:         schema.ProjectScopeWithProfile(h.root, "", "", "coordination", "").Map(),
		AuditRef:      auditRef,
		Payload:       payload,
	}
}

func appendCoordinationEvent(store *eventlog.Store, ev schema.Event, base string) (string, error) {
	for attempt := 0; attempt < 100; attempt++ {
		ev.ID = base
		if attempt > 0 {
			ev.ID = fmt.Sprintf("%s_%d", base, attempt+1)
		}
		if err := store.Append(ev); err != nil {
			if eventlog.IsDuplicateEventID(err) {
				continue
			}
			return "", err
		}
		return ev.ID, nil
	}
	return "", fmt.Errorf("append coordination event: exhausted duplicate id retries for %q", base)
}

func coordPayloadString(p map[string]any, key string) string {
	if p == nil {
		return ""
	}
	if s, ok := p[key].(string); ok {
		return strings.TrimSpace(s)
	}
	return ""
}

func coordPayloadStrings(p map[string]any, key string) []string {
	if p == nil {
		return nil
	}
	raw, ok := p[key].([]any)
	if !ok {
		return nil
	}
	var out []string
	for _, v := range raw {
		if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
			out = append(out, strings.TrimSpace(s))
		}
	}
	return out
}
