package ui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mnemon-dev/mnemon/harness/internal/ui/read"
)

// Profile is the durable-behavior page: what is carried forward, and where does
// it project? Read-only in this plan — new entries arrive only via an approved +
// applied route=memory proposal (Proposals page), keeping growth governed.

func (m *model) updateProfile(msg tea.KeyMsg) tea.Cmd {
	entries := m.snap.Profile.Entries
	switch msg.String() {
	case "j", "down":
		if !m.pfDetail {
			m.pfSel = clampIdx(m.pfSel+1, len(entries))
		}
	case "k", "up":
		if !m.pfDetail {
			m.pfSel = clampIdx(m.pfSel-1, len(entries))
		}
	case "enter":
		if len(entries) == 0 {
			return nil
		}
		m.pfDetail = !m.pfDetail
	case "esc":
		m.pfDetail = false
	}
	return nil
}

func (m *model) viewProfile(w, h int) string {
	prof := m.snap.Profile
	if m.snap.Err.Profile != nil {
		return m.emptyPane("PROFILE",
			"no profile yet — approve & apply a route=memory proposal to record the first entry.\n("+m.snap.Err.Profile.Error()+")", h)
	}
	if len(prof.Entries) == 0 {
		return m.emptyPane("PROFILE", "no profile entries yet — they arrive via an applied route=memory proposal.", h)
	}
	if m.pfDetail && m.pfSel < len(prof.Entries) {
		return m.viewProfileEntryDetail(prof, prof.Entries[m.pfSel], w, h)
	}

	rows := []string{m.th.paneTitle.Render(fmt.Sprintf("PROFILE %s (%d entries)", prof.ID, len(prof.Entries)))}
	for i, e := range prof.Entries {
		if i == m.pfSel {
			rows = append(rows, m.th.listSelected.Render(fmt.Sprintf("▸ %s  %s", pad(e.Type, 12), truncPlain(e.Summary, w-18))))
			continue
		}
		rows = append(rows, "  "+m.th.warn.Render(pad(e.Type, 12))+"  "+m.th.listNormal.Render(truncPlain(e.Summary, w-18)))
	}
	return viewport(rows, m.pfSel+1, h)
}

func (m *model) viewProfileEntryDetail(prof read.Profile, e read.ProfileEntry, w, h int) string {
	var lines []string
	add := func(s string) { lines = append(lines, s) }
	add(m.th.paneTitle.Render(truncPlain(e.Summary, w)))
	add(m.kv("id", e.ID))
	add(m.kv("type", e.Type))
	add(m.kv("profile", prof.ID+" ("+prof.ScopeType+")"))
	add("")
	add(m.section("content"))
	add(m.th.detailValue.Render(wrap(e.Content, w)))
	if len(e.Evidence) > 0 {
		add("")
		add(m.section("evidence"))
		for _, ev := range e.Evidence {
			add("  " + m.th.muted.Render(ev.Type+" ") + m.th.detailValue.Render(truncPlain(ev.Ref, w-10)))
		}
	}
	add("")
	add(m.section("projects to"))
	if len(e.ProjectionTargets) == 0 {
		add(m.th.muted.Render("  (no projection targets)"))
	}
	for _, t := range e.ProjectionTargets {
		add("  " + m.th.detailValue.Render(orDash(t.Host)+" / "+orDash(t.Loop)))
	}
	add("")
	add(m.kv("created", absTime(e.CreatedAt)+"  updated "+absTime(e.UpdatedAt)))
	return viewport(lines, 0, h)
}
