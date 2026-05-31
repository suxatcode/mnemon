package eval

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mnemon-dev/mnemon/harness/internal/lifecycle/eventlog"
	"github.com/mnemon-dev/mnemon/harness/internal/lifecycle/layout"
	"github.com/mnemon-dev/mnemon/harness/internal/lifecycle/proposal"
	"github.com/mnemon-dev/mnemon/harness/internal/lifecycle/proposalstore"
	"github.com/mnemon-dev/mnemon/harness/internal/lifecycle/schema"
)

const EvalAssetPromotedEventType = "eval.asset_promoted"

type EvalAssetKind string

const (
	EvalAssetScenario EvalAssetKind = "scenario"
	EvalAssetSuite    EvalAssetKind = "suite"
	EvalAssetRubric   EvalAssetKind = "rubric"
)

type EvalAssetState string

const (
	EvalAssetEphemeral EvalAssetState = "ephemeral"
	EvalAssetCandidate EvalAssetState = "candidate"
	EvalAssetPromoted  EvalAssetState = "promoted"
	EvalAssetCanonical EvalAssetState = "canonical"
)

type EvalAssetRef struct {
	Kind      EvalAssetKind  `json:"kind"`
	ID        string         `json:"id"`
	URI       string         `json:"uri"`
	Lifecycle EvalAssetState `json:"lifecycle,omitempty"`
}

type PromotionOptions struct {
	Kind          EvalAssetKind
	ID            string
	Target        EvalAssetState
	From          EvalAssetState
	ProposalRef   string
	AuditRef      string
	EventID       string
	CorrelationID string
	CausedBy      string
	Actor         string
	Source        string
	Now           time.Time
}

type PromotionResult struct {
	Asset      EvalAssetRef   `json:"asset"`
	ProposalID string         `json:"proposal_id"`
	FromState  EvalAssetState `json:"from_state"`
	ToState    EvalAssetState `json:"to_state"`
	Event      schema.Event   `json:"event"`
}

func PromoteAsset(root string, opts PromotionOptions) (PromotionResult, error) {
	root = cleanRoot(root)
	opts = normalizePromotionOptions(opts)
	if err := validatePromotionOptions(opts); err != nil {
		return PromotionResult{}, err
	}
	asset, err := ResolveEvalAsset(root, opts.Kind, opts.ID)
	if err != nil {
		return PromotionResult{}, err
	}
	item, err := loadApprovedEvalProposal(root, opts.ProposalRef)
	if err != nil {
		return PromotionResult{}, err
	}
	from := opts.From
	if from == "" {
		from, err = currentEvalAssetState(root, asset)
		if err != nil {
			return PromotionResult{}, err
		}
	}
	from = normalizeEvalAssetState(from)
	if err := validateFromState(from); err != nil {
		return PromotionResult{}, err
	}
	if promotionRank(opts.Target) < promotionRank(from) {
		return PromotionResult{}, fmt.Errorf("cannot promote %s %q from %s to earlier state %s", opts.Kind, opts.ID, from, opts.Target)
	}
	event, err := newEvalAssetPromotedEvent(root, asset, item.ID, from, opts)
	if err != nil {
		return PromotionResult{}, err
	}
	store, err := eventlog.New(root)
	if err != nil {
		return PromotionResult{}, err
	}
	if err := store.Append(event); err != nil {
		return PromotionResult{}, err
	}
	return PromotionResult{
		Asset:      asset,
		ProposalID: item.ID,
		FromState:  from,
		ToState:    opts.Target,
		Event:      event,
	}, nil
}

func ResolveEvalAsset(root string, kind EvalAssetKind, id string) (EvalAssetRef, error) {
	root = cleanRoot(root)
	kind = normalizeEvalAssetKind(kind)
	id = strings.TrimSpace(id)
	if err := validateAssetKind(kind); err != nil {
		return EvalAssetRef{}, err
	}
	if id == "" {
		return EvalAssetRef{}, fmt.Errorf("asset id is required")
	}
	switch kind {
	case EvalAssetSuite:
		suite, err := LoadSuite(root, id)
		if err != nil {
			return EvalAssetRef{}, err
		}
		return EvalAssetRef{Kind: kind, ID: suite.Name, URI: suite.Source, Lifecycle: normalizeEvalAssetState(EvalAssetState(suite.Lifecycle))}, nil
	case EvalAssetScenario:
		scenario, found, err := LoadScenario(root, id)
		if err != nil {
			return EvalAssetRef{}, err
		}
		if found {
			return EvalAssetRef{Kind: kind, ID: scenario.ID, URI: scenario.Source, Lifecycle: normalizeEvalAssetState(EvalAssetState(scenario.Lifecycle))}, nil
		}
		return resolveEvalAssetFile(root, kind, "scenarios", id, []string{".md", ".json"})
	case EvalAssetRubric:
		return resolveEvalAssetFile(root, kind, "rubrics", id, []string{".md"})
	default:
		return EvalAssetRef{}, fmt.Errorf("asset kind %q is not supported", kind)
	}
}

func normalizePromotionOptions(opts PromotionOptions) PromotionOptions {
	opts.Kind = normalizeEvalAssetKind(opts.Kind)
	opts.ID = strings.TrimSpace(opts.ID)
	opts.Target = normalizeEvalAssetState(opts.Target)
	opts.From = normalizeEvalAssetState(opts.From)
	opts.ProposalRef = normalizeProposalRef(opts.ProposalRef)
	opts.AuditRef = strings.TrimSpace(opts.AuditRef)
	opts.EventID = strings.TrimSpace(opts.EventID)
	opts.CorrelationID = strings.TrimSpace(opts.CorrelationID)
	opts.CausedBy = strings.TrimSpace(opts.CausedBy)
	opts.Actor = strings.TrimSpace(opts.Actor)
	opts.Source = strings.TrimSpace(opts.Source)
	if opts.Now.IsZero() {
		opts.Now = time.Now().UTC()
	}
	if opts.Target == "" {
		opts.Target = EvalAssetPromoted
	}
	if opts.Actor == "" {
		opts.Actor = "mnemon-manual"
	}
	if opts.Source == "" {
		opts.Source = "mnemon.eval.promote"
	}
	if opts.EventID == "" {
		opts.EventID = fmt.Sprintf("evt_eval_promote_%s_%s_%d", sanitizeABID(string(opts.Kind)), sanitizeABID(opts.ID), opts.Now.UTC().UnixNano())
	}
	if opts.CorrelationID == "" && opts.ProposalRef != "" {
		opts.CorrelationID = "proposal:" + opts.ProposalRef
	}
	if opts.CorrelationID == "" {
		opts.CorrelationID = opts.EventID
	}
	return opts
}

func validatePromotionOptions(opts PromotionOptions) error {
	var errs []error
	if err := validateAssetKind(opts.Kind); err != nil {
		errs = append(errs, err)
	}
	if strings.TrimSpace(opts.ID) == "" {
		errs = append(errs, fmt.Errorf("asset id is required"))
	}
	if err := validateTargetState(opts.Target); err != nil {
		errs = append(errs, err)
	}
	if opts.From != "" {
		if err := validateFromState(opts.From); err != nil {
			errs = append(errs, err)
		}
	}
	if strings.TrimSpace(opts.ProposalRef) == "" {
		errs = append(errs, fmt.Errorf("proposal_ref is required"))
	}
	return joinErrors(errs)
}

func loadApprovedEvalProposal(root, proposalRef string) (proposal.Proposal, error) {
	store, err := proposalstore.New(root)
	if err != nil {
		return proposal.Proposal{}, err
	}
	item, err := store.Load(proposalRef)
	if err != nil {
		return proposal.Proposal{}, fmt.Errorf("load proposal %q: %w", proposalRef, err)
	}
	if item.Route != proposal.RouteEval {
		return proposal.Proposal{}, fmt.Errorf("proposal %q route must be %q, got %q", item.ID, proposal.RouteEval, item.Route)
	}
	if item.Status != proposal.StatusApproved {
		return proposal.Proposal{}, fmt.Errorf("proposal %q must be approved, got %q", item.ID, item.Status)
	}
	return item, nil
}

func newEvalAssetPromotedEvent(root string, asset EvalAssetRef, proposalID string, from EvalAssetState, opts PromotionOptions) (schema.Event, error) {
	paths, err := layout.Resolve(root)
	if err != nil {
		return schema.Event{}, err
	}
	loop := "eval"
	var causedBy *string
	if opts.CausedBy != "" {
		causedBy = &opts.CausedBy
	}
	payload := map[string]any{
		"asset_kind":   string(asset.Kind),
		"asset_id":     asset.ID,
		"asset_uri":    asset.URI,
		"from_state":   string(from),
		"to_state":     string(opts.Target),
		"proposal_ref": proposalID,
	}
	if opts.AuditRef != "" {
		payload["audit_ref"] = opts.AuditRef
	}
	event := schema.Event{
		SchemaVersion: schema.Version,
		ID:            opts.EventID,
		TS:            opts.Now.UTC().Format(time.RFC3339),
		Type:          EvalAssetPromotedEventType,
		Loop:          &loop,
		Actor:         opts.Actor,
		Source:        opts.Source,
		CorrelationID: opts.CorrelationID,
		CausedBy:      causedBy,
		Payload:       payload,
		ProjectRoot:   paths.Root,
		Scope:         schema.ProjectScopeWithProfile(paths.Root, "", "", loop, "").Map(),
		ProposalRef: map[string]any{
			"id":  proposalID,
			"uri": filepath.ToSlash(filepath.Join(".mnemon", "harness", "proposals", string(proposal.StatusApproved), proposalID, "proposal.json")),
		},
		Severity: "info",
	}
	if opts.AuditRef != "" {
		event.AuditRef = map[string]any{"ref": opts.AuditRef}
	}
	if err := schema.ValidateEvent(event); err != nil {
		return schema.Event{}, err
	}
	return event, nil
}

func currentEvalAssetState(root string, asset EvalAssetRef) (EvalAssetState, error) {
	state := asset.Lifecycle
	store, err := eventlog.New(root)
	if err != nil {
		return "", err
	}
	events, err := store.ReadAll()
	if err != nil {
		return "", err
	}
	for _, event := range events {
		if event.Type != EvalAssetPromotedEventType {
			continue
		}
		if stringPayload(event.Payload, "asset_kind") != string(asset.Kind) || stringPayload(event.Payload, "asset_id") != asset.ID {
			continue
		}
		next := normalizeEvalAssetState(EvalAssetState(stringPayload(event.Payload, "to_state")))
		if next != "" {
			state = next
		}
	}
	if state == "" {
		return EvalAssetEphemeral, nil
	}
	return state, nil
}

func resolveEvalAssetFile(root string, kind EvalAssetKind, dir, id string, exts []string) (EvalAssetRef, error) {
	rel, err := safeEvalAssetRel(id)
	if err != nil {
		return EvalAssetRef{}, err
	}
	base := filepath.Join(root, "harness", "loops", "eval", dir)
	candidates := []string{rel}
	if filepath.Ext(rel) == "" {
		for _, ext := range exts {
			candidates = append(candidates, rel+ext)
		}
	}
	for _, candidate := range candidates {
		path := filepath.Join(base, candidate)
		ok, err := isFileUnder(path, base)
		if err != nil {
			return EvalAssetRef{}, err
		}
		if !ok {
			return EvalAssetRef{}, fmt.Errorf("asset id %q escapes eval %s directory", id, dir)
		}
		info, err := os.Stat(path)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return EvalAssetRef{}, fmt.Errorf("stat eval %s asset %s: %w", kind, path, err)
		}
		if info.IsDir() {
			continue
		}
		source, err := filepath.Rel(root, path)
		if err != nil {
			source = path
		}
		return EvalAssetRef{Kind: kind, ID: strings.TrimSuffix(filepath.ToSlash(rel), filepath.Ext(rel)), URI: filepath.ToSlash(source)}, nil
	}
	return EvalAssetRef{}, fmt.Errorf("eval %s asset %q not found", kind, id)
}

func safeEvalAssetRel(id string) (string, error) {
	rel := filepath.Clean(filepath.FromSlash(strings.TrimSpace(id)))
	if rel == "." || rel == "" {
		return "", fmt.Errorf("asset id is required")
	}
	if filepath.IsAbs(rel) || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf("asset id %q must be relative to the eval asset directory", id)
	}
	return rel, nil
}

func isFileUnder(path, base string) (bool, error) {
	rel, err := filepath.Rel(base, path)
	if err != nil {
		return false, err
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator)), nil
}

func normalizeEvalAssetKind(kind EvalAssetKind) EvalAssetKind {
	return EvalAssetKind(strings.TrimSpace(strings.ToLower(string(kind))))
}

func normalizeEvalAssetState(state EvalAssetState) EvalAssetState {
	return EvalAssetState(strings.TrimSpace(strings.ToLower(string(state))))
}

func normalizeProposalRef(ref string) string {
	ref = strings.TrimSpace(ref)
	ref = strings.TrimPrefix(ref, "proposal:")
	return strings.TrimSpace(ref)
}

func validateAssetKind(kind EvalAssetKind) error {
	switch kind {
	case EvalAssetScenario, EvalAssetSuite, EvalAssetRubric:
		return nil
	default:
		return fmt.Errorf("asset kind %q is not allowed", kind)
	}
}

func validateTargetState(state EvalAssetState) error {
	switch state {
	case EvalAssetCandidate, EvalAssetPromoted, EvalAssetCanonical:
		return nil
	default:
		return fmt.Errorf("target state %q is not allowed", state)
	}
}

func validateFromState(state EvalAssetState) error {
	switch state {
	case EvalAssetEphemeral, EvalAssetCandidate, EvalAssetPromoted, EvalAssetCanonical:
		return nil
	default:
		return fmt.Errorf("from state %q is not allowed", state)
	}
}

func promotionRank(state EvalAssetState) int {
	switch state {
	case EvalAssetEphemeral:
		return 0
	case EvalAssetCandidate:
		return 1
	case EvalAssetPromoted:
		return 2
	case EvalAssetCanonical:
		return 3
	default:
		return -1
	}
}

func stringPayload(payload map[string]any, key string) string {
	value, ok := payload[key]
	if !ok {
		return ""
	}
	text, ok := value.(string)
	if !ok {
		return ""
	}
	return text
}
