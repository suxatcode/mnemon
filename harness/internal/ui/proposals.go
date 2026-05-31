package ui

import (
	"fmt"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mattn/go-runewidth"
	"github.com/mnemon-dev/mnemon/harness/internal/ui/read"
)

// statusOrder defines the display grouping order for the proposal queue: the
// happy path first (draft → open → in_review → approved → applied), then the
// off-path and terminal states.
var statusOrder = []string{
	"draft", "open", "in_review", "request_changes", "approved",
	"applied", "blocked", "rejected", "superseded", "withdrawn", "expired",
}

func statusRank(status string) int {
	for i, s := range statusOrder {
		if s == status {
			return i
		}
	}
	return len(statusOrder)
}

// orderedProposals returns the proposals sorted by status group, then most
// recently updated first within a group.
func (m *model) orderedProposals() []read.Proposal {
	ps := make([]read.Proposal, len(m.snap.Proposals))
	copy(ps, m.snap.Proposals)
	sort.SliceStable(ps, func(i, j int) bool {
		ri, rj := statusRank(ps[i].Status), statusRank(ps[j].Status)
		if ri != rj {
			return ri < rj
		}
		return ps[i].UpdatedAt > ps[j].UpdatedAt
	})
	return ps
}

func (m *model) updateProposals(msg tea.KeyMsg) tea.Cmd {
	ps := m.filteredProposals()
	key := msg.String()

	// Governed action keys (o v a c x b A w) open a confirm modal — but only for
	// state-machine-legal actions; illegal ones are ignored (disabled).
	if len(ps) > 0 {
		if cmd, handled := m.tryProposalAction(key, ps[m.prSel]); handled {
			return cmd
		}
	}

	switch key {
	case "j", "down":
		if !m.prDetail {
			m.prSel = clampIdx(m.prSel+1, len(ps))
		}
	case "k", "up":
		if !m.prDetail {
			m.prSel = clampIdx(m.prSel-1, len(ps))
		}
	case " ":
		// Toggle multi-select on the focused proposal (review-acceleration triage).
		if !m.prDetail && len(ps) > 0 {
			m.toggleProposalSelected(ps[m.prSel].ID)
		}
	case "B":
		// Bulk-apply the selected approved proposals — each still through the
		// governed apply path; the human confirms the reviewed batch.
		if !m.prDetail {
			return m.beginBulkApply(ps)
		}
	case "enter":
		if len(ps) == 0 {
			return nil
		}
		if !m.prDetail {
			m.prDetail = true
			return nil
		}
		// In detail: follow the proposal → audit forward link if present.
		p := ps[m.prSel]
		if len(p.AuditRefs) > 0 {
			if m.gotoAuditByRef(p.AuditRefs[0]) {
				return nil
			}
			return m.setToast("no matching audit record loaded for "+p.AuditRefs[0], true)
		}
	case "esc":
		m.prDetail = false
	}
	return nil
}

// tryProposalAction maps a governed-action key to a confirm modal, returning
// handled=true if the key is an action key (whether or not it was legal).
func (m *model) tryProposalAction(key string, p read.Proposal) (tea.Cmd, bool) {
	for _, a := range proposalActions {
		if a.key != key {
			continue
		}
		if !a.availableFor(p.Status) {
			return m.setToast(a.label+" not available from "+p.Status, true), true
		}
		if a.apply {
			m.confirm = m.confirmApply(p)
		} else {
			m.confirm = m.confirmTransition(p.ID, a.status, a.label)
		}
		return nil, true
	}
	return nil, false
}

func (m *model) viewProposals(w, h int) string {
	ps := m.filteredProposals()
	if m.snap.Err.Proposals != nil {
		return m.emptyPane("PROPOSALS", "unavailable: "+m.snap.Err.Proposals.Error(), h)
	}
	if len(ps) == 0 {
		if m.prFilter != "" {
			return m.emptyPane("PROPOSALS", "no proposals match \""+m.prFilter+"\" — esc-filter or press / to change.", h)
		}
		return m.emptyPane("PROPOSALS", "no proposals yet — evidence raises them.", h)
	}
	if m.prDetail {
		return m.viewProposalDetail(ps[m.prSel], w, h)
	}

	title := fmt.Sprintf("PROPOSALS (%d)", len(ps))
	if n := m.selectedCount(); n > 0 {
		title += fmt.Sprintf("  ·  %d selected", n)
	}
	rows := []string{m.th.paneTitle.Render(title)}
	lastGroup := ""
	titleW := w - 42
	for i, p := range ps {
		if p.Status != lastGroup {
			lastGroup = p.Status
			rows = append(rows, m.th.groupHeader.Render(strings.ToUpper(p.Status)))
		}
		mark := m.selectMark(p.ID)
		label, badge := m.reviewBadge(p)
		if i == m.prSel {
			plain := fmt.Sprintf("%s  %s  %s  %s", pad(p.Route, 8), pad(p.Risk, 8), pad(label, 6), truncPlain(p.Title, titleW))
			rows = append(rows, m.th.listSelected.Render("▸"+mark+" "+plain))
			continue
		}
		line := fmt.Sprintf("%s  %s  %s  %s",
			m.th.statusStyle(p.Status).Render(pad(p.Route, 8)),
			riskLabel(m.th, p.Risk),
			badge,
			m.th.listNormal.Render(truncPlain(p.Title, titleW)),
		)
		rows = append(rows, " "+mark+" "+line)
	}
	// Keep the selected proposal visible: find its row position.
	selRow := selectedRowIndex(ps, m.prSel)
	return viewport(rows, selRow, h)
}

func (m *model) viewProposalDetail(p read.Proposal, w, h int) string {
	var lines []string
	add := func(s string) { lines = append(lines, s) }

	add(m.th.paneTitle.Render(truncate(p.Title, w)))
	add(m.kv("id", p.ID))
	add(m.th.detailLabel.Render("status: ") + m.th.statusStyle(p.Status).Render(p.Status) +
		m.th.detailLabel.Render("   route: ") + m.th.detailValue.Render(p.Route) +
		m.th.detailLabel.Render("   risk: ") + m.th.detailValue.Render(p.Risk))
	if p.Status == "applied" {
		if evs := m.proposalEvents(p.ID); len(evs) > 0 {
			add(m.th.good.Render("✓ loop closed — emitted " + evs[0].Type + " (" + evs[0].ID + ")"))
		}
	}
	add("")
	add(m.section("summary"))
	add(m.th.detailValue.Render(wrap(p.Summary, w)))

	add("")
	add(m.section("change"))
	add(m.kv("summary", p.Change.Summary))
	for _, t := range p.Change.Targets {
		add("  " + m.th.muted.Render("target ") + m.th.detailValue.Render(t.Type+" = "+t.URI))
	}
	for _, op := range p.Change.Operations {
		add("  " + m.th.muted.Render("op ") + m.th.detailValue.Render(op.Type+" → "+op.Target+": "+op.Summary))
	}

	if len(p.Evidence) > 0 {
		add("")
		add(m.section("evidence"))
		for _, e := range p.Evidence {
			add("  " + m.th.muted.Render(e.Type+" ") + m.th.detailValue.Render(truncate(e.Ref, w-10)))
		}
	}

	add("")
	add(m.section("validation plan"))
	add(m.kv("summary", p.ValidationPlan.Summary))
	for _, c := range p.ValidationPlan.Commands {
		add("  " + m.th.muted.Render("$ ") + m.th.detailValue.Render(truncate(c, w-4)))
	}
	for _, c := range p.ValidationPlan.Checks {
		add("  " + m.th.muted.Render("✓ ") + m.th.detailValue.Render(truncate(c, w-4)))
	}

	add("")
	add(m.section("review"))
	add(m.kv("required", fmt.Sprintf("%t (scope=%s, reviews=%d)", p.Review.Required, p.Review.RequiredScope, p.Review.RequiredReviews)))

	add("")
	add(m.section("governance"))
	add(m.kv("decision_refs", strings.Join(p.DecisionRefs, ", ")))
	if len(p.AuditRefs) > 0 {
		add(m.th.detailLabel.Render("audit_refs: ") + m.th.good.Render(strings.Join(p.AuditRefs, ", ")))
		add(m.th.hint.Render("  enter: follow → audit"))
	} else {
		add(m.kv("audit_refs", ""))
	}
	add(m.kv("created", absTime(p.CreatedAt)+"  updated "+absTime(p.UpdatedAt)))
	if p.ClosedAt != "" {
		add(m.kv("closed", absTime(p.ClosedAt)))
	}
	if p.SupersededBy != "" {
		add(m.kv("superseded_by", p.SupersededBy))
	}

	add("")
	add(m.section("actions"))
	add(m.availableActionsLine(p.Status))

	// Loop-closure proof: events this proposal emitted (populated after apply).
	if linked := m.proposalEvents(p.ID); len(linked) > 0 {
		add("")
		add(m.section("emitted events"))
		for i, ev := range linked {
			if i >= 6 {
				break
			}
			add("  " + m.th.good.Render(pad(ev.Type, 26)) + " " + m.th.muted.Render(ev.ID))
		}
	}

	return viewport(lines, 0, h)
}

// availableActionsLine renders the governed actions, highlighting those legal
// from the current status and dimming the rest.
func (m *model) availableActionsLine(status string) string {
	var parts []string
	for _, a := range proposalActions {
		token := "[" + a.key + "] " + a.label
		if a.availableFor(status) {
			parts = append(parts, m.th.listSelected.Render(token))
		} else {
			parts = append(parts, m.th.hint.Render(token))
		}
	}
	return strings.Join(parts, m.th.divider.Render("  "))
}

// proposalEvents returns events the snapshot carries that reference this proposal
// (newest first) — the visible proof the loop emitted events on apply.
func (m *model) proposalEvents(id string) []read.Event {
	var out []read.Event
	for i := range m.snap.Events {
		ev := &m.snap.Events[i]
		if ev.CorrelationID == id || ev.CorrelationID == "proposal:"+id || linkedProposalID(ev) == id {
			out = append(out, *ev)
		}
	}
	return out
}

// gotoProposal switches to the Proposals page focused on the proposal with the
// given id, returning false if it is not loaded.
func (m *model) gotoProposal(id string) bool {
	m.prFilter = "" // clear any filter so the index matches the visible list
	ps := m.orderedProposals()
	for i, p := range ps {
		if p.ID == id {
			m.closeAllDetails() // don't leave the source page showing a stale detail
			m.active = pageProposals
			m.prSel = i
			m.prDetail = true
			m.toast = ""
			return true
		}
	}
	return false
}

// setToast shows a footer toast and returns a command that auto-clears it after
// toastTTL (so it doesn't linger over the key hints until the next navigation).
func (m *model) setToast(msg string, isErr bool) tea.Cmd {
	m.toast = msg
	m.toastErr = isErr
	m.toastSeq++
	return m.clearToastCmd(m.toastSeq)
}

// --- 5A review acceleration helpers (bulk select + advisory badge) ---

func (m *model) toggleProposalSelected(id string) {
	if m.prSelected == nil {
		m.prSelected = map[string]bool{}
	}
	if m.prSelected[id] {
		delete(m.prSelected, id)
	} else {
		m.prSelected[id] = true
	}
}

func (m *model) selectMark(id string) string {
	if m.prSelected[id] {
		return m.th.good.Render("✓")
	}
	return " "
}

func (m *model) selectedCount() int { return len(m.prSelected) }

// reviewBadge returns the advisory triage label and a styled badge for a
// proposal. The class is deterministic, code-computed (read.ClassifyProposal) —
// never a model verdict, never an apply decision.
func (m *model) reviewBadge(p read.Proposal) (label, styled string) {
	cls := read.ClassifyProposal(p)
	if cls.Safe {
		return cls.Label, m.th.good.Render(pad(cls.Label, 6))
	}
	return cls.Label, m.th.warn.Render(pad(cls.Label, 6))
}

// beginBulkApply opens a batch confirm for the selected approved proposals. Only
// approved proposals apply; everything else is skipped with a hint. The human
// confirms once; each proposal still applies through the governed apply path.
func (m *model) beginBulkApply(ps []read.Proposal) tea.Cmd {
	var selected []read.Proposal
	for _, p := range ps {
		if m.prSelected[p.ID] && p.Status == "approved" {
			selected = append(selected, p)
		}
	}
	if len(selected) == 0 {
		return m.setToast("no selected approved proposals — space to select; only approved proposals apply", true)
	}
	m.confirm = m.confirmApplyBatch(selected)
	return nil
}

func riskLabel(th theme, risk string) string {
	switch risk {
	case "critical", "high":
		return th.bad.Render(pad(risk, 8))
	case "medium":
		return th.warn.Render(pad(risk, 8))
	default:
		return th.muted.Render(pad(risk, 8))
	}
}

// selectedRowIndex maps a proposal selection index to its rendered row index,
// accounting for the title row and per-group header rows.
func selectedRowIndex(ps []read.Proposal, sel int) int {
	row := 1 // title
	lastGroup := ""
	for i, p := range ps {
		if p.Status != lastGroup {
			lastGroup = p.Status
			row++ // group header
		}
		if i == sel {
			return row
		}
		row++
	}
	return row
}

// truncPlain truncates plain (unstyled) text to w display columns, adding an
// ellipsis. It measures by terminal cell width (so wide CJK/emoji runes count as
// 2), keeping list rows within their budget and preserving the one-row=one-line
// invariant the windowed viewport depends on.
func truncPlain(s string, w int) string {
	if w <= 0 {
		return ""
	}
	if runewidth.StringWidth(s) <= w {
		return s
	}
	budget := w - 1 // reserve one column for the ellipsis
	if budget < 1 {
		return "…"
	}
	var b strings.Builder
	used := 0
	for _, r := range s {
		rw := runewidth.RuneWidth(r)
		if used+rw > budget {
			break
		}
		b.WriteRune(r)
		used += rw
	}
	b.WriteRune('…')
	return b.String()
}

// wrap soft-wraps text to width w.
func wrap(s string, w int) string {
	if w <= 0 || len(s) <= w {
		return s
	}
	words := strings.Fields(s)
	var b strings.Builder
	lineLen := 0
	for i, word := range words {
		if lineLen > 0 && lineLen+1+len(word) > w {
			b.WriteString("\n")
			lineLen = 0
		} else if i > 0 {
			b.WriteString(" ")
			lineLen++
		}
		b.WriteString(word)
		lineLen += len(word)
	}
	return b.String()
}
