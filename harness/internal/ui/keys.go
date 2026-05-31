package ui

import "strings"

// Key handling uses tea.KeyMsg.String() directly (e.g. "j", "tab", "enter").
// This file centralizes the key→meaning mapping for the help overlay and the
// contextual footer so the documented keymap and the behavior stay in one place.

// globalKeyHelp lists keys that work on every page.
var globalKeyHelp = [][2]string{
	{"1-7 / tab", "switch page"},
	{"j / k, ↑ / ↓", "move selection"},
	{"enter", "drill into detail · follow link"},
	{"esc", "back / close detail"},
	{"t", "trace selected proposal's lineage"},
	{"/", "filter"},
	{"r", "refresh snapshot"},
	{"?", "toggle this help"},
	{"q", "quit"},
}

// proposalKeyHelp lists the governed proposal actions (live in U2).
var proposalKeyHelp = [][2]string{
	{"o", "open (draft → open)"},
	{"v", "submit review (open → in_review)"},
	{"a", "approve (in_review → approved)"},
	{"c", "request changes"},
	{"x", "reject"},
	{"b", "block"},
	{"A", "apply (approved → applied)"},
	{"w", "withdraw"},
	{"space", "select for bulk review"},
	{"B", "bulk-apply selected approved (each governed)"},
}

// optionalKeyHelp lists the safe non-proposal governance controls.
var optionalKeyHelp = [][2]string{
	{"n", "nudge selected goal (Scope page)"},
	{"p", "pause daemon"},
	{"P", "resume daemon"},
}

// helpText renders the full-screen help overlay body.
func (t theme) helpText() string {
	var b strings.Builder
	b.WriteString(t.paneTitle.Render("mnemon-harness — cognition console") + "\n")
	b.WriteString(t.muted.Render("the screen is the loop: scope → evidence → proposals → audit → next run") + "\n\n")

	b.WriteString(t.railTitle.Render("global") + "\n")
	for _, kv := range globalKeyHelp {
		b.WriteString("  " + t.listSelected.Render(pad(kv[0], 14)) + t.detailValue.Render(kv[1]) + "\n")
	}
	b.WriteString("\n" + t.railTitle.Render("proposals page — governed actions") + "\n")
	for _, kv := range proposalKeyHelp {
		b.WriteString("  " + t.listSelected.Render(pad(kv[0], 14)) + t.detailValue.Render(kv[1]) + "\n")
	}
	b.WriteString("\n" + t.railTitle.Render("optional controls") + "\n")
	for _, kv := range optionalKeyHelp {
		b.WriteString("  " + t.listSelected.Render(pad(kv[0], 14)) + t.detailValue.Render(kv[1]) + "\n")
	}
	b.WriteString("\n" + t.muted.Render("every governed action opens a confirm modal naming the exact facade call.") + "\n")
	b.WriteString(t.hint.Render("press ? or esc to close") + "\n")
	return b.String()
}

// footerHint returns the contextual key hint line for a page.
func footerHint(active pageID, detail bool) string {
	if detail {
		return "enter follow link · esc back · r refresh · ? help · q quit"
	}
	switch active {
	case pageProposals:
		return "j/k move · space select · B bulk-apply · enter detail · t trace · o v a c x b A w actions · / filter · ? help · q quit"
	case pageScope:
		return "j/k move · enter detail · 1-7 pages · r refresh · ? help · q quit"
	case pageTrace:
		return "j/k step · enter jump to record · esc back · 1-7 pages · ? help · q quit"
	case pageHosts:
		return "j/k move · enter → host's latest event · 1-7 pages · r refresh · ? help · q quit"
	case pageCoord:
		return "j/k move · enter → task's latest event · 1-7 pages · r refresh · ? help · q quit"
	default:
		return "j/k move · enter detail · t trace · / filter · 1-7 pages · r refresh · ? help · q quit"
	}
}

func pad(s string, n int) string {
	if len(s) >= n {
		return s
	}
	return s + strings.Repeat(" ", n-len(s))
}
