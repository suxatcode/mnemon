package app

import (
	"github.com/mnemon-dev/mnemon/harness/internal/contract"
	"github.com/mnemon-dev/mnemon/harness/internal/runtime"
)

// TowerView is the read-only, operator-wide projection that backs the Agent Control Tower's four pages
// (P6). The app-layer facade assembles it from the *Runtime's read surfaces; the ui package renders a
// TowerView and never touches the store/kernel (the ui↛store law). Zero new kernel concepts — every
// field maps to an existing protocol object (§3.1). READ-ONLY: building a view never writes or Ticks.
type TowerView struct {
	Goal   GoalPage
	Ledger LedgerPage
	// Field + Inbox are added in P6a-3.
}

// GoalPage answers "目标怎么样了": the project_intent statements (the goal) and the progress_digest
// summaries. "readiness" is shown as the ACTUAL progress entries — a fabricated percentage would need
// a KR data model that does not exist, and inventing one would be a new kernel concept (T1 veto).
type GoalPage struct {
	Statements []string // project_intent items' statements
	Progress   []string // progress_digest items' summaries
}

// LedgerRow is one accepted decision with its attribution (the proposer + what it changed).
type LedgerRow struct {
	DecisionID string
	Actor      contract.ActorID
	AppliedAt  string
	Refs       []contract.ResourceRef
}

// LedgerPage answers "什么已经定了": the accepted decisions, newest last (append order).
type LedgerPage struct {
	Decisions []LedgerRow
}

// towerScopeID is the default coordination scope every coordination kind is bound at ("project").
const towerScopeID = contract.ResourceID("project")

// BuildTowerView assembles the read-only Tower projection from the runtime. It performs only resource
// reads and the read-only DecisionLedger — never a write or a Tick (G10/T5). (GOAL + LEDGER here;
// FIELD + INBOX land in P6a-3.)
func BuildTowerView(rt *runtime.Runtime) (TowerView, error) {
	var v TowerView
	// GOAL: project_intent statements + progress_digest summaries (read-only resource reads; an
	// absent resource — version 0 — simply yields no entries).
	if ver, fields, err := rt.Resource(contract.ResourceRef{Kind: "project_intent", ID: towerScopeID}); err == nil && ver > 0 {
		v.Goal.Statements = towerItemStrings(fields, "items", "statement")
	}
	if ver, fields, err := rt.Resource(contract.ResourceRef{Kind: "progress_digest", ID: towerScopeID}); err == nil && ver > 0 {
		v.Goal.Progress = towerItemStrings(fields, "items", "summary")
	}
	// LEDGER: accepted decisions with attribution.
	decisions, err := rt.DecisionLedger()
	if err != nil {
		return v, err
	}
	for _, d := range decisions {
		if d.Status != contract.Accepted {
			continue
		}
		refs := make([]contract.ResourceRef, 0, len(d.NewVersions))
		for _, nv := range d.NewVersions {
			refs = append(refs, nv.Ref)
		}
		v.Ledger.Decisions = append(v.Ledger.Decisions, LedgerRow{
			DecisionID: d.DecisionID, Actor: d.Actor, AppliedAt: d.AppliedAt, Refs: refs,
		})
	}
	return v, nil
}

// towerItemStrings extracts a string field from each item in fields[itemsField] (the canonical []any
// of map[string]any shape). Absent/typeless items yield an empty slice (no panic).
func towerItemStrings(fields map[string]any, itemsField, field string) []string {
	raw, ok := fields[itemsField].([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(raw))
	for _, r := range raw {
		if m, ok := r.(map[string]any); ok {
			if s, ok := m[field].(string); ok && s != "" {
				out = append(out, s)
			}
		}
	}
	return out
}
