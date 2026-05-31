package ui

import (
	"fmt"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mnemon-dev/mnemon/harness/internal/ui/read"
)

// maxEvidence caps the merged stream length for responsiveness.
const maxEvidence = 600

// evidenceItem is one row in the merged, reverse-chronological evidence stream:
// a lifecycle event or an audit record, normalized for display and linking.
type evidenceItem struct {
	sortKey    string
	ts         string
	kind       string // "event" | "audit"
	title      string
	summary    string
	proposalID string // forward-link target, if any
	auditURI   string

	event *read.Event
	audit *read.AuditRecord
}

// evidenceItems merges events and audit records into one reverse-chronological
// stream. Events already carry goal-evidence and eval lifecycle records, so the
// stream covers "what happened" without separate merging for U1.
func (m *model) evidenceItems() []evidenceItem {
	items := make([]evidenceItem, 0, len(m.snap.Events)+len(m.snap.Audits))
	for i := range m.snap.Events {
		ev := &m.snap.Events[i]
		items = append(items, evidenceItem{
			sortKey:    ev.TS,
			ts:         ev.TS,
			kind:       "event",
			title:      ev.Type,
			summary:    eventSummary(ev),
			proposalID: linkedProposalID(ev),
			event:      ev,
		})
	}
	for i := range m.snap.Audits {
		a := &m.snap.Audits[i]
		ts := extractAuditTS(a.Audit.Metadata.Name)
		items = append(items, evidenceItem{
			// An undated audit (ts=="") sorts to the bottom of the reverse-chron
			// stream, not the top — its raw name must not masquerade as "newest".
			sortKey:    ts,
			ts:         ts,
			kind:       "audit",
			title:      "audit:" + orDash(a.Kind()),
			summary:    auditSummary(a),
			proposalID: auditProposalID(a),
			auditURI:   a.URI(),
			audit:      a,
		})
	}
	sort.SliceStable(items, func(i, j int) bool { return items[i].sortKey > items[j].sortKey })
	if len(items) > maxEvidence {
		items = items[:maxEvidence]
	}
	return items
}

func (m *model) updateEvidence(msg tea.KeyMsg) tea.Cmd {
	items := m.filteredEvidence()
	switch msg.String() {
	case "j", "down":
		if !m.evDetail {
			m.evSel = clampIdx(m.evSel+1, len(items))
		}
	case "k", "up":
		if !m.evDetail {
			m.evSel = clampIdx(m.evSel-1, len(items))
		}
	case "enter":
		if len(items) == 0 {
			return nil
		}
		if !m.evDetail {
			m.evDetail = true
			return nil
		}
		// In detail: follow evidence → proposal forward link.
		it := items[m.evSel]
		if it.proposalID != "" {
			if m.gotoProposal(it.proposalID) {
				return nil
			}
			return m.setToast("linked proposal not loaded: "+it.proposalID, true)
		}
	case "esc":
		m.evDetail = false
	}
	return nil
}

func (m *model) viewEvidence(w, h int) string {
	items := m.filteredEvidence()
	if m.snap.Err.Events != nil && len(items) == 0 {
		return m.emptyPane("EVIDENCE", "unavailable: "+m.snap.Err.Events.Error(), h)
	}
	if len(items) == 0 {
		if m.evFilter != "" {
			return m.emptyPane("EVIDENCE", "no evidence matches \""+m.evFilter+"\" — esc-filter or press / to change.", h)
		}
		return m.emptyPane("EVIDENCE", "no evidence yet — the loop has not recorded anything.", h)
	}
	if m.evDetail {
		return m.viewEvidenceDetail(items[m.evSel], w, h)
	}

	rows := []string{m.th.paneTitle.Render(fmt.Sprintf("EVIDENCE (%d)", len(items)))}
	for i, it := range items {
		when := relTime(it.ts, time.Now())
		link := " "
		if it.proposalID != "" && m.proposalLoaded(it.proposalID) {
			link = m.th.good.Render("→")
		}
		if i == m.evSel {
			plain := fmt.Sprintf("%s %s  %s  %s", ">", pad(when, 9), pad(it.title, 26), truncPlain(it.summary, w-42))
			rows = append(rows, m.th.listSelected.Render(plain))
			continue
		}
		kindStyle := m.th.muted
		if it.kind == "audit" {
			kindStyle = m.th.warn
		}
		line := fmt.Sprintf("%s %s  %s  %s", link, m.th.muted.Render(pad(when, 9)),
			kindStyle.Render(pad(it.title, 26)), m.th.listNormal.Render(truncPlain(it.summary, w-42)))
		rows = append(rows, line)
	}
	return viewport(rows, m.evSel+1, h)
}

func (m *model) viewEvidenceDetail(it evidenceItem, w, h int) string {
	var lines []string
	add := func(s string) { lines = append(lines, s) }

	add(m.th.paneTitle.Render(truncPlain(it.title, w)))
	add(m.kv("when", absTime(it.ts)+"  ("+relTime(it.ts, time.Now())+")"))

	if it.event != nil {
		ev := it.event
		add(m.kv("type", ev.Type))
		add(m.kv("actor", ev.Actor)) // who
		add(m.kv("source", ev.Source))
		add(m.kv("loop / host", orDash(ev.LoopName())+" / "+orDash(ev.HostName())))
		add(m.kv("correlation", ev.CorrelationID))
		add(m.kv("event id", ev.ID))
		add("")
		add(m.section("payload"))
		add(m.th.detailValue.Render(prettyJSON(ev.Raw, w)))
	}
	if it.audit != nil {
		a := it.audit
		add(m.kv("kind", a.Kind()))
		add(m.kv("decision", specString(a.Audit.Spec, "decision")))
		add(m.kv("reason", specString(a.Audit.Spec, "reason")))
		add(m.kv("uri", a.URI()))
		add("")
		add(m.section("spec"))
		add(m.th.detailValue.Render(prettyMap(a.Audit.Spec, w)))
	}

	if it.proposalID != "" {
		add("")
		if m.proposalLoaded(it.proposalID) {
			add(m.th.detailLabel.Render("proposal: ") + m.th.good.Render(it.proposalID))
			add(m.th.hint.Render("  enter: follow → proposal"))
		} else {
			add(m.kv("proposal", it.proposalID+" (not loaded)"))
		}
	}
	return viewport(lines, 0, h)
}

// gotoAuditByRef switches to the Evidence page focused on the audit record whose
// uri or path matches ref, returning false if none is loaded.
func (m *model) gotoAuditByRef(ref string) bool {
	m.evFilter = "" // clear any filter so the index matches the visible list
	items := m.evidenceItems()
	ref = strings.TrimSpace(ref)
	for i, it := range items {
		if it.kind != "audit" || it.audit == nil {
			continue
		}
		if it.auditURI == ref || strings.HasSuffix(it.audit.Path, strings.TrimPrefix(ref, ".")) ||
			strings.HasSuffix(ref, baseName(it.auditURI)) {
			m.closeAllDetails() // don't leave the source page showing a stale detail
			m.active = pageEvidence
			m.evSel = i
			m.evDetail = true
			m.toast = ""
			return true
		}
	}
	return false
}

func (m *model) proposalLoaded(id string) bool {
	for _, p := range m.snap.Proposals {
		if p.ID == id {
			return true
		}
	}
	return false
}

// --- evidence helpers ---

func eventSummary(ev *read.Event) string {
	if ev.Payload != nil {
		if s, ok := ev.Payload["summary"].(string); ok && strings.TrimSpace(s) != "" {
			return s
		}
	}
	return ev.Actor + " · " + ev.Source
}

func auditSummary(a *read.AuditRecord) string {
	if d := specString(a.Audit.Spec, "decision"); d != "" {
		if r := specString(a.Audit.Spec, "reason"); r != "" {
			return d + " — " + r
		}
		return d
	}
	if s := specString(a.Audit.Spec, "status"); s != "" {
		return "status " + s
	}
	return a.Audit.Metadata.Name
}

// linkedProposalID extracts a proposal id an event refers to, if any.
func linkedProposalID(ev *read.Event) string {
	if ev.ProposalRef != nil {
		if id, ok := ev.ProposalRef["id"].(string); ok && id != "" {
			return id
		}
	}
	if ev.Payload != nil {
		if id, ok := ev.Payload["proposal_id"].(string); ok && id != "" {
			return id
		}
	}
	if strings.HasPrefix(ev.Type, "proposal") && ev.CorrelationID != "" {
		return strings.TrimPrefix(ev.CorrelationID, "proposal:")
	}
	return ""
}

func auditProposalID(a *read.AuditRecord) string {
	if a.Audit.Spec == nil {
		return ""
	}
	if id := specString(a.Audit.Spec, "proposal_id"); id != "" {
		return id
	}
	if refs, ok := a.Audit.Spec["proposal_refs"].([]any); ok && len(refs) > 0 {
		if s, ok := refs[0].(string); ok {
			return s
		}
	}
	return ""
}

func specString(spec map[string]any, key string) string {
	if spec == nil {
		return ""
	}
	if s, ok := spec[key].(string); ok {
		return s
	}
	return ""
}

// extractAuditTS finds the TRAILING 20060102T150405… stamp in an audit record
// name and renders it as an RFC3339 timestamp for cross-stream sorting. Names can
// carry more than one stamp (e.g. a goal-completion audit embeds both the goal's
// creation time and the completion time); the last one is the record's own time.
func extractAuditTS(name string) string {
	last := ""
	for _, tok := range strings.Split(name, "-") {
		if len(tok) >= 15 && tok[8] == 'T' && allDigits(tok[:8]) && allDigits(tok[9:15]) {
			if t, err := time.Parse("20060102T150405", tok[:15]); err == nil {
				last = t.UTC().Format(time.RFC3339)
			}
		}
	}
	return last
}

func allDigits(s string) bool {
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return len(s) > 0
}

func baseName(p string) string {
	if i := strings.LastIndexByte(p, '/'); i >= 0 {
		return p[i+1:]
	}
	return p
}
