package ui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// The Coordination page renders the materialized multi-agent topology read-only:
// who owns what, fork lineage, groups, conflicts, and merge candidates. It is the
// collaboration accountability surface — derived purely from coordination events,
// never mutated here.

func (m *model) updateCoord(msg tea.KeyMsg) tea.Cmd {
	tasks := m.snap.Coordination.Tasks
	switch msg.String() {
	case "j", "down":
		m.coordSel = clampIdx(m.coordSel+1, len(tasks))
	case "k", "up":
		m.coordSel = clampIdx(m.coordSel-1, len(tasks))
	case "enter":
		if m.coordSel >= 0 && m.coordSel < len(tasks) {
			if id := tasks[m.coordSel].LastEventID; id != "" {
				if m.gotoEventByID(id) {
					return nil
				}
				return m.setToast("task's latest event not loaded", true)
			}
		}
	}
	return nil
}

func (m *model) viewCoord(w, h int) string {
	c := m.snap.Coordination
	if m.snap.Err.Coordination != nil {
		return m.emptyPane("COORD", "unavailable: "+m.snap.Err.Coordination.Error(), h)
	}
	if len(c.Tasks)+len(c.Groups)+len(c.Conflicts) == 0 {
		return m.emptyPane("COORD", "no coordination yet — claim/fork/group/conflict events build the topology.", h)
	}

	var rows []string

	// Tasks (selectable): who owns what + fork/join lineage + evidence.
	rows = append(rows, m.th.paneTitle.Render(fmt.Sprintf("TASKS (%d)", len(c.Tasks))))
	for i, t := range c.Tasks {
		lineage := ""
		if t.ForkedFrom != "" {
			lineage += " forked from " + t.ForkedFrom
		}
		if t.JoinedInto != "" {
			lineage += " joined into " + t.JoinedInto
		}
		ev := ""
		if len(t.EvidenceRefs) > 0 {
			ev = fmt.Sprintf("  %d evidence", len(t.EvidenceRefs))
		}
		plain := fmt.Sprintf("%s  %s  owner %s%s%s",
			pad(t.ID, 14), pad(t.Status, 9), pad(orDash(t.Owner), 14), lineage, ev)
		if i == m.coordSel {
			rows = append(rows, m.th.listSelected.Render("▸ "+plain))
		} else {
			rows = append(rows, "  "+m.th.detailValue.Render(plain))
		}
	}
	selRow := m.coordSel + 1

	rows = append(rows, "", m.th.paneTitle.Render(fmt.Sprintf("GROUPS (%d)", len(c.Groups))))
	if len(c.Groups) == 0 {
		rows = append(rows, m.th.muted.Render("  none"))
	}
	for _, g := range c.Groups {
		rows = append(rows, "  "+m.th.detailValue.Render(pad(g.ID, 14))+"  "+m.th.muted.Render(strings.Join(g.Members, ", ")))
	}

	rows = append(rows, "", m.th.paneTitle.Render(fmt.Sprintf("CONFLICTS (%d)", len(c.Conflicts))))
	if len(c.Conflicts) == 0 {
		rows = append(rows, m.th.muted.Render("  none"))
	}
	for _, cf := range c.Conflicts {
		rows = append(rows, "  "+m.th.warn.Render(strings.Join(cf.Between, " x "))+m.th.muted.Render("  "+cf.Reason))
	}

	rows = append(rows, "", m.th.paneTitle.Render(fmt.Sprintf("MERGE CANDIDATES (%d)", len(c.MergeCandidates))))
	if len(c.MergeCandidates) == 0 {
		rows = append(rows, m.th.muted.Render("  none"))
	}
	for _, mc := range c.MergeCandidates {
		rows = append(rows, "  "+m.th.muted.Render(mc.EvidenceRef+" -> ")+m.th.detailValue.Render(strings.Join(mc.Tasks, ", ")))
	}

	return viewport(rows, selRow, h)
}
