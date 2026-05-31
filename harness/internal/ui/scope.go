package ui

import (
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mnemon-dev/mnemon/harness/internal/ui/bind"
	"github.com/mnemon-dev/mnemon/harness/internal/ui/read"
)

// Scope is the home page: under what context am I acting, and what is the state
// of the loop? It renders three strips — Active Goals (selectable), Recent
// Evidence, and Open Proposals — over the persistent scope header + ribbon.

func (m *model) updateScope(msg tea.KeyMsg) tea.Cmd {
	goals := m.snap.Goals
	switch msg.String() {
	case "j", "down":
		if !m.scopeDetail {
			m.scopeSel = clampIdx(m.scopeSel+1, len(goals))
		}
	case "k", "up":
		if !m.scopeDetail {
			m.scopeSel = clampIdx(m.scopeSel-1, len(goals))
		}
	case "enter":
		if len(goals) == 0 {
			return nil
		}
		m.scopeDetail = !m.scopeDetail
	case "n":
		if len(goals) > 0 {
			g := goals[m.scopeSel]
			m.confirm = &confirmState{
				title:  "nudge goal",
				call:   "app.GoalNudge",
				effect: "record a nudge",
				notes:  []string{"goal: " + g.ID},
				cmd:    bind.GoalNudge(m.root, g.ID, "nudged from console"),
			}
		}
	case "esc":
		m.scopeDetail = false
	}
	return nil
}

func (m *model) viewScope(w, h int) string {
	if m.scopeDetail && m.scopeSel < len(m.snap.Goals) {
		return m.viewGoalDetail(m.snap.Goals[m.scopeSel], w, h)
	}

	var rows []string

	// Active Goals (selectable strip).
	if m.snap.Err.Goals != nil {
		rows = append(rows, m.th.paneTitle.Render("ACTIVE GOALS"),
			m.th.muted.Render("  unavailable: "+m.snap.Err.Goals.Error()))
	} else {
		rows = append(rows, m.th.paneTitle.Render(fmt.Sprintf("ACTIVE GOALS (%d)", len(m.snap.Goals))))
		if len(m.snap.Goals) == 0 {
			rows = append(rows, m.th.muted.Render("  no goals yet"))
		}
		for i, g := range m.snap.Goals {
			obj := truncPlain(g.Objective, w-28)
			if i == m.scopeSel {
				rows = append(rows, m.th.listSelected.Render(fmt.Sprintf("▸ %s  %s  %s", pad(g.Status, 10), pad(g.ID, 14), obj)))
			} else {
				rows = append(rows, "  "+m.th.goalStatusStyle(g.Status).Render(pad(g.Status, 10))+"  "+
					m.th.detailValue.Render(pad(g.ID, 14))+"  "+m.th.muted.Render(obj))
			}
		}
	}
	selRow := m.scopeSel + 1 // account for the title row

	// Recent Evidence strip (read-only).
	rows = append(rows, "")
	ev := m.evidenceItems()
	rows = append(rows, m.th.paneTitle.Render("RECENT EVIDENCE")+m.th.hint.Render("  (2 to open)"))
	if len(ev) == 0 {
		rows = append(rows, m.th.muted.Render("  none"))
	}
	for i := 0; i < len(ev) && i < 5; i++ {
		rows = append(rows, "  "+m.th.muted.Render(pad(relTime(ev[i].ts, time.Now()), 9))+"  "+
			m.th.detailValue.Render(truncPlain(ev[i].title+"  "+ev[i].summary, w-12)))
	}

	// Open Proposals strip (read-only).
	rows = append(rows, "")
	rows = append(rows, m.th.paneTitle.Render("OPEN PROPOSALS")+m.th.hint.Render("  (3 to review)"))
	open := 0
	for _, p := range m.orderedProposals() {
		if p.Status != "open" && p.Status != "in_review" && p.Status != "draft" {
			continue
		}
		if open >= 5 {
			break
		}
		open++
		rows = append(rows, "  "+m.th.statusStyle(p.Status).Render(pad(p.Status, 12))+"  "+
			m.th.detailValue.Render(truncPlain(p.Title, w-16)))
	}
	if open == 0 {
		rows = append(rows, m.th.muted.Render("  none pending"))
	}

	return viewport(rows, selRow, h)
}

func (m *model) viewGoalDetail(g read.GoalView, w, h int) string {
	var lines []string
	add := func(s string) { lines = append(lines, s) }
	add(m.th.paneTitle.Render("goal " + g.ID))
	add(m.th.detailLabel.Render("status: ") + m.th.goalStatusStyle(g.Status).Render(g.Status))
	add(m.kv("objective", g.Objective))
	add(m.kv("report", g.ReportStatus))
	add(m.kv("evidence", fmt.Sprintf("%d records", g.EvidenceCount)))
	add(m.kv("completion ready", fmt.Sprintf("%t", g.Ready)))
	if g.Plan != nil {
		add("")
		add(m.section("plan"))
		add(m.kv("summary", g.Plan.Summary))
		for i, step := range g.Plan.Steps {
			add(fmt.Sprintf("  %d. %s", i+1, m.th.detailValue.Render(truncPlain(step, w-6))))
		}
	}
	add("")
	add(m.kv("path", g.Path))
	return viewport(lines, 0, h)
}
