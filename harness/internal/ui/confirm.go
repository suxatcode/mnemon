package ui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mnemon-dev/mnemon/harness/internal/ui/bind"
	"github.com/mnemon-dev/mnemon/harness/internal/ui/read"
)

// confirmState is a pending governed write awaiting the operator's y/n. It names
// the exact facade call and its effect so a write is never one keystroke away by
// accident — the console mediates governance, it does not bypass it.
type confirmState struct {
	title  string
	call   string   // the facade call, e.g. app.ProposalTransition("id", "approved")
	effect string   // human effect, e.g. "in_review → approved"
	notes  []string // extra lines (e.g. what the apply emits)
	cmd    tea.Cmd  // the bind command dispatched on confirm
}

// confirmTransition builds a confirm modal for a proposal status transition.
func (m *model) confirmTransition(id, status, label string) *confirmState {
	cur := ""
	for _, p := range m.snap.Proposals {
		if p.ID == id {
			cur = p.Status
			break
		}
	}
	return &confirmState{
		title:  label + " proposal",
		call:   "app.ProposalTransition",
		effect: cur + " → " + status,
		notes:  []string{"id: " + id},
		cmd:    bind.ProposalTransition(m.root, id, status, label),
	}
}

// confirmApply builds a confirm modal for applying an approved proposal. It shows
// the deterministic review class (advisory), the diff (what the apply will do),
// and the reason — so the human decides with the change in front of them.
func (m *model) confirmApply(p read.Proposal) *confirmState {
	notes := []string{"id: " + p.ID, "route: " + p.Route}
	cls := read.ClassifyProposal(p)
	notes = append(notes, "class: "+cls.Label+" ("+cls.Reason+") — advisory triage, not auto-apply")
	for _, op := range p.Change.Operations {
		diff := "diff: " + op.Type + " → " + op.Target
		if op.Summary != "" {
			diff += "  (" + op.Summary + ")"
		}
		notes = append(notes, diff)
	}
	if p.Summary != "" {
		notes = append(notes, "reason: "+p.Summary)
	}
	switch p.Route {
	case "memory":
		notes = append(notes, "emits: profile.entry_recorded + audit.recorded; writes audit_refs")
	case "eval":
		notes = append(notes, "emits: eval.asset_promoted + audit.recorded; writes audit_refs")
	case "coordination":
		notes = append(notes, "emits: coordination event(s) + audit.recorded; writes audit_refs")
	default:
		notes = append(notes, "route not implemented for apply — surfaces the facade's boundary audit")
	}
	return &confirmState{
		title:  "apply proposal",
		call:   "app.ProposalApply",
		effect: "approved → applied",
		notes:  notes,
		cmd:    bind.ProposalApply(m.root, p.ID),
	}
}

// confirmApplyBatch builds a confirm modal for bulk-applying several approved
// proposals. Each still goes through the governed app.ProposalApply — the human
// presses apply once for the reviewed batch; nothing auto-applies.
func (m *model) confirmApplyBatch(ps []read.Proposal) *confirmState {
	ids := make([]string, 0, len(ps))
	notes := []string{fmt.Sprintf("%d approved proposal(s) — each applied through the governed apply path:", len(ps))}
	for _, p := range ps {
		ids = append(ids, p.ID)
		cls := read.ClassifyProposal(p)
		notes = append(notes, fmt.Sprintf("  [%s] %s  %s", cls.Label, p.ID, truncPlain(p.Title, 48)))
	}
	return &confirmState{
		title:  "bulk apply selected proposals",
		call:   fmt.Sprintf("app.ProposalApply ×%d", len(ids)),
		effect: "approved → applied",
		notes:  notes,
		cmd:    bind.ProposalApplyBatch(m.root, ids),
	}
}

// viewConfirm renders the confirm modal as a bordered box filling the content
// pane.
func (m *model) viewConfirm(w, h int) string {
	c := m.confirm
	inner := []string{
		m.th.paneTitle.Render(c.title),
		"",
		m.th.detailLabel.Render("facade call: ") + m.th.detailValue.Render(c.call),
		m.th.detailLabel.Render("effect:      ") + m.th.statusStyle(lastToken(c.effect)).Render(c.effect),
	}
	for _, n := range c.notes {
		inner = append(inner, m.th.muted.Render("  "+n))
	}
	inner = append(inner, "", m.th.good.Render("y / enter")+m.th.muted.Render(" confirm    ")+m.th.bad.Render("n / esc")+m.th.muted.Render(" cancel"))

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colAccent).
		Padding(0, 1).
		Width(minInt(w-2, 70))
	rendered := box.Render(joinLines(inner))
	return lipgloss.Place(w, h, lipgloss.Center, lipgloss.Center, rendered)
}

func joinLines(lines []string) string {
	out := ""
	for i, l := range lines {
		if i > 0 {
			out += "\n"
		}
		out += l
	}
	return out
}

func lastToken(s string) string {
	// effect is "from → to"; color by the target status.
	for i := len(s) - 1; i >= 0; i-- {
		if s[i] == ' ' {
			return s[i+1:]
		}
	}
	return s
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
