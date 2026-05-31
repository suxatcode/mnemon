package ui

import "strings"

// viewport windows a slice of pre-rendered rows to height h, keeping the row at
// index sel visible, and pads the result to exactly h lines so the layout stays
// stable as the selection moves.
func viewport(rows []string, sel, h int) string {
	if h < 1 {
		h = 1
	}
	start := 0
	if len(rows) > h {
		// Keep the selection roughly centered, clamped to the ends.
		start = sel - h/2
		if start < 0 {
			start = 0
		}
		if start > len(rows)-h {
			start = len(rows) - h
		}
	}
	end := start + h
	if end > len(rows) {
		end = len(rows)
	}
	visible := rows[start:end]
	out := make([]string, 0, h)
	out = append(out, visible...)
	for len(out) < h {
		out = append(out, "")
	}
	return strings.Join(out, "\n")
}

// emptyPane renders a centered-ish cold-start / unavailable message filling the
// pane height.
func (m *model) emptyPane(title, msg string, h int) string {
	lines := []string{
		m.th.paneTitle.Render(title),
		"",
		m.th.muted.Render(msg),
	}
	return viewport(lines, 0, h)
}

// kv renders a "label: value" detail line.
func (m *model) kv(label, value string) string {
	return m.th.detailLabel.Render(label+": ") + m.th.detailValue.Render(orDash(value))
}

// section renders a detail section header.
func (m *model) section(title string) string {
	return m.th.groupHeader.Render(title)
}
