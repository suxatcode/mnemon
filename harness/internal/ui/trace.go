package ui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mnemon-dev/mnemon/harness/internal/ui/read"
)

// The Trace page makes the accountability chain first-class: for one proposal it
// walks backward to the evidence + approver and forward to the apply audit, the
// events the apply emitted, and the projection targets the next run pulls
// (evidence → proposal → apply → audit → projection → next run). It is a
// read-only view over the snapshot; navigable steps jump to the underlying record
// on the Evidence / Proposals pages.

// traceTarget identifies where a navigable trace step jumps. A zero target
// (kind == "") marks a non-navigable line (section header / descriptor).
type traceTarget struct {
	kind string // "proposal" | "audit" | "event"
	ref  string // proposal id | audit uri | event id
}

// traceStep is one rendered lineage line; nav != zero means it can be jumped to.
type traceStep struct {
	text string
	nav  traceTarget
}

// openTrace focuses the lineage trace on a proposal and switches to the Trace
// page. With id == "" it defaults to the proposal highlighted on the Proposals
// page (so `t` traces "this" proposal); otherwise it keeps the current focus.
func (m *model) openTrace(id string) {
	if id == "" {
		if ps := m.filteredProposals(); len(ps) > 0 && m.prSel >= 0 && m.prSel < len(ps) {
			id = ps[m.prSel].ID
		}
	}
	if id != "" {
		if id != m.traceID {
			m.traceSel = 0
		}
		m.traceID = id
	}
	m.switchPage(pageTrace)
}

func (m *model) proposalByID(id string) *read.Proposal {
	for i := range m.snap.Proposals {
		if m.snap.Proposals[i].ID == id {
			return &m.snap.Proposals[i]
		}
	}
	return nil
}

// focalProposal returns the proposal the trace is focused on, or nil.
func (m *model) focalProposal() *read.Proposal {
	if strings.TrimSpace(m.traceID) == "" {
		return nil
	}
	return m.proposalByID(m.traceID)
}

// auditLoadedByRef reports whether an audit record matching ref is in the
// snapshot, so the step that names it can be made navigable. Mirrors the matching
// in gotoAuditByRef but guards against empty uris matching every ref.
func (m *model) auditLoadedByRef(ref string) bool {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return false
	}
	for i := range m.snap.Audits {
		uri := m.snap.Audits[i].URI()
		path := m.snap.Audits[i].Path
		switch {
		case uri != "" && uri == ref:
			return true
		case path != "" && strings.HasSuffix(path, strings.TrimPrefix(ref, ".")):
			return true
		case uri != "" && strings.HasSuffix(ref, baseName(uri)):
			return true
		}
	}
	return false
}

// proposalProjectionTargets returns the projection targets of the profile entries
// this proposal applied — the forward "what the next run pulls" step. It links the
// proposal's emitted apply events (payload entry_id) to the current profile.
func (m *model) proposalProjectionTargets(id string) []read.ProjectionTarget {
	entryIDs := map[string]bool{}
	for _, ev := range m.proposalEvents(id) {
		if ev.Payload == nil {
			continue
		}
		if eid, ok := ev.Payload["entry_id"].(string); ok && eid != "" {
			entryIDs[eid] = true
		}
	}
	var targets []read.ProjectionTarget
	seen := map[string]bool{}
	for _, e := range m.snap.Profile.Entries {
		if !entryIDs[e.ID] {
			continue
		}
		for _, t := range e.ProjectionTargets {
			key := t.Host + "/" + t.Loop
			if seen[key] {
				continue
			}
			seen[key] = true
			targets = append(targets, t)
		}
	}
	return targets
}

// traceSteps assembles the focal proposal's lineage as ordered display steps.
// Navigable steps carry a non-zero nav target and start with a two-space indent so
// the selection caret can replace it.
func (m *model) traceSteps(p read.Proposal, w int) []traceStep {
	var steps []traceStep
	nav := func(text string, t traceTarget) { steps = append(steps, traceStep{text: text, nav: t}) }
	plain := func(text string) { steps = append(steps, traceStep{text: text}) }

	plain(m.section("proposal"))
	nav("  "+m.th.detailValue.Render(truncPlain(p.Title, w-4)), traceTarget{kind: "proposal", ref: p.ID})
	plain("    " + m.th.statusStyle(p.Status).Render(p.Status) +
		m.th.detailLabel.Render("  route ") + m.th.detailValue.Render(p.Route) +
		m.th.detailLabel.Render("  risk ") + m.th.detailValue.Render(p.Risk))

	plain("")
	plain(m.section("← evidence"))
	if len(p.Evidence) == 0 {
		plain(m.th.muted.Render("  (none recorded)"))
	}
	for _, e := range p.Evidence {
		line := "  " + m.th.muted.Render(e.Type+" ") + m.th.detailValue.Render(truncPlain(e.Ref, w-12))
		if m.auditLoadedByRef(e.Ref) {
			nav(line, traceTarget{kind: "audit", ref: e.Ref})
		} else {
			plain(line)
		}
	}

	plain("")
	plain(m.section("✓ review / approval"))
	plain("  " + m.th.detailLabel.Render("required ") + m.th.detailValue.Render(
		fmt.Sprintf("%t (scope=%s, reviews=%d)", p.Review.Required, orDash(p.Review.RequiredScope), p.Review.RequiredReviews)))
	if len(p.Review.Reviewers) > 0 {
		plain("  " + m.th.detailLabel.Render("reviewers ") + m.th.detailValue.Render(strings.Join(p.Review.Reviewers, ", ")))
	}
	if len(p.DecisionRefs) > 0 {
		plain("  " + m.th.detailLabel.Render("decisions ") + m.th.detailValue.Render(strings.Join(p.DecisionRefs, ", ")))
	}

	plain("")
	plain(m.section("→ apply audit"))
	if len(p.AuditRefs) == 0 {
		plain(m.th.muted.Render("  (not applied yet — no audit)"))
	}
	for _, ref := range p.AuditRefs {
		line := "  " + m.th.good.Render(truncPlain(ref, w-4))
		if m.auditLoadedByRef(ref) {
			nav(line, traceTarget{kind: "audit", ref: ref})
		} else {
			plain(line)
		}
	}

	if emitted := m.proposalEvents(p.ID); len(emitted) > 0 {
		plain("")
		plain(m.section("→ emitted events"))
		for i, ev := range emitted {
			if i >= 8 {
				plain(m.th.muted.Render(fmt.Sprintf("  … %d more", len(emitted)-8)))
				break
			}
			nav("  "+m.th.detailValue.Render(pad(ev.Type, 28))+" "+m.th.muted.Render(ev.ID),
				traceTarget{kind: "event", ref: ev.ID})
		}
	}

	plain("")
	plain(m.section("→ projection · next run"))
	if p.Route == "coordination" {
		// Coordination apply mutates the event-sourced topology; hosts inherit it by
		// pulling COORDINATION.json on their next install/run.
		plain("  " + m.th.good.Render("coordination topology") +
			m.th.muted.Render("  → hosts pull COORDINATION.json on next install/run"))
		for _, ev := range m.proposalEvents(p.ID) {
			if tid := specString(ev.Payload, "task_id"); tid != "" {
				plain("    " + m.th.detailValue.Render(ev.Type+" "+tid))
			}
		}
	} else {
		targets := m.proposalProjectionTargets(p.ID)
		if len(targets) == 0 {
			plain(m.th.muted.Render("  (no projection targets — next run pulls nothing from this)"))
		}
		for _, t := range targets {
			plain("  " + m.th.good.Render(t.Host+"/"+t.Loop) +
				m.th.muted.Render("  pulls PROFILE.json on next install/run"))
		}
	}

	return steps
}

// traceNavSteps returns only the navigable steps, in order.
func (m *model) traceNavSteps(p read.Proposal) []traceStep {
	all := m.traceSteps(p, m.width)
	out := make([]traceStep, 0, len(all))
	for _, s := range all {
		if s.nav.kind != "" {
			out = append(out, s)
		}
	}
	return out
}

// traceNavCount is the number of navigable steps for the focal proposal (0 when
// none is focused). Used to clamp the selection.
func (m *model) traceNavCount() int {
	p := m.focalProposal()
	if p == nil {
		return 0
	}
	return len(m.traceNavSteps(*p))
}

func (m *model) updateTrace(msg tea.KeyMsg) tea.Cmd {
	p := m.focalProposal()
	if p == nil {
		if msg.String() == "esc" {
			m.switchPage(pageProposals)
		}
		return nil
	}
	nav := m.traceNavSteps(*p)
	switch msg.String() {
	case "j", "down":
		m.traceSel = clampIdx(m.traceSel+1, len(nav))
	case "k", "up":
		m.traceSel = clampIdx(m.traceSel-1, len(nav))
	case "enter":
		if m.traceSel >= 0 && m.traceSel < len(nav) {
			return m.jumpTrace(nav[m.traceSel].nav)
		}
	case "esc":
		m.switchPage(pageProposals)
	}
	return nil
}

// jumpTrace follows a navigable step to its record on the Evidence/Proposals page.
func (m *model) jumpTrace(t traceTarget) tea.Cmd {
	switch t.kind {
	case "proposal":
		if m.gotoProposal(t.ref) {
			return nil
		}
		return m.setToast("proposal not loaded: "+t.ref, true)
	case "audit":
		if m.gotoAuditByRef(t.ref) {
			return nil
		}
		return m.setToast("audit record not loaded: "+t.ref, true)
	case "event":
		if m.gotoEventByID(t.ref) {
			return nil
		}
		return m.setToast("event not loaded: "+t.ref, true)
	}
	return nil
}

// gotoEventByID switches to the Evidence page focused on the event with id,
// returning false if it is not loaded.
func (m *model) gotoEventByID(id string) bool {
	m.evFilter = ""
	items := m.evidenceItems()
	for i, it := range items {
		if it.event != nil && it.event.ID == id {
			m.closeAllDetails()
			m.active = pageEvidence
			m.evSel = i
			m.evDetail = true
			m.toast = ""
			return true
		}
	}
	return false
}

func (m *model) viewTrace(w, h int) string {
	if strings.TrimSpace(m.traceID) == "" {
		return m.emptyPane("TRACE", "no proposal selected — open a proposal (3) and press t to trace its lineage.", h)
	}
	p := m.focalProposal()
	if p == nil {
		return m.emptyPane("TRACE", "proposal "+m.traceID+" not loaded — it may be filtered out or removed.", h)
	}

	rows := []string{m.th.paneTitle.Render(truncPlain("TRACE — "+p.Title, w))}
	navIdx, selRow := 0, 0
	for _, s := range m.traceSteps(*p, w) {
		if s.nav.kind == "" {
			rows = append(rows, s.text)
			continue
		}
		body := strings.TrimPrefix(s.text, "  ")
		if navIdx == m.traceSel {
			rows = append(rows, m.th.listSelected.Render("▸ ")+body)
			selRow = len(rows) - 1
		} else {
			rows = append(rows, "  "+body)
		}
		navIdx++
	}
	return viewport(rows, selRow, h)
}
