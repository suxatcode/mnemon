package ui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mnemon-dev/mnemon/harness/internal/ui/read"
)

// startFilter enters filter-input mode for the active page, seeding the input
// with the page's current filter.
func (m *model) startFilter() tea.Cmd {
	m.filtering = true
	m.ti.SetValue(m.activeFilter())
	m.ti.CursorEnd()
	return m.ti.Focus()
}

// commitFilter stores the typed filter on the active page and resets its
// selection to the top of the narrowed list.
func (m *model) commitFilter() {
	val := strings.TrimSpace(m.ti.Value())
	switch m.active {
	case pageEvidence:
		m.evFilter = val
		m.evSel = 0
	case pageProposals:
		m.prFilter = val
		m.prSel = 0
	}
	m.filtering = false
	m.ti.Blur()
}

// cancelFilter exits filter-input mode without changing the active filter.
func (m *model) cancelFilter() {
	m.filtering = false
	m.ti.Blur()
}

func (m *model) activeFilter() string {
	switch m.active {
	case pageEvidence:
		return m.evFilter
	case pageProposals:
		return m.prFilter
	}
	return ""
}

// filteredEvidence applies the Evidence filter (case-insensitive substring over
// type, summary, kind, loop, host, and actor).
func (m *model) filteredEvidence() []evidenceItem {
	items := m.evidenceItems()
	f := strings.ToLower(strings.TrimSpace(m.evFilter))
	if f == "" {
		return items
	}
	out := items[:0:0]
	for _, it := range items {
		hay := strings.ToLower(it.title + " " + it.summary + " " + it.kind)
		if it.event != nil {
			hay += " " + strings.ToLower(it.event.LoopName()+" "+it.event.HostName()+" "+it.event.Actor)
		}
		if strings.Contains(hay, f) {
			out = append(out, it)
		}
	}
	return out
}

// filteredProposals applies the Proposals filter (case-insensitive substring over
// id, status, route, risk, and title).
func (m *model) filteredProposals() []read.Proposal {
	ps := m.orderedProposals()
	f := strings.ToLower(strings.TrimSpace(m.prFilter))
	if f == "" {
		return ps
	}
	out := ps[:0:0]
	for _, p := range ps {
		hay := strings.ToLower(p.ID + " " + p.Status + " " + p.Route + " " + p.Risk + " " + p.Title)
		if strings.Contains(hay, f) {
			out = append(out, p)
		}
	}
	return out
}
