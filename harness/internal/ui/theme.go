package ui

import "github.com/charmbracelet/lipgloss"

// theme holds the console's lipgloss styles. One clean palette; status colors are
// consistent across pages so a proposal's state reads the same in a list, a
// detail pane, or the loop ribbon.
type theme struct {
	// chrome
	headerTitle lipgloss.Style
	scopeKey    lipgloss.Style
	scopeVal    lipgloss.Style
	ribbonOn    lipgloss.Style
	ribbonOff   lipgloss.Style
	ribbonArrow lipgloss.Style
	railTitle   lipgloss.Style
	railOn      lipgloss.Style
	railOff     lipgloss.Style
	footer      lipgloss.Style
	divider     lipgloss.Style

	// content
	paneTitle    lipgloss.Style
	listSelected lipgloss.Style
	listNormal   lipgloss.Style
	groupHeader  lipgloss.Style
	detailLabel  lipgloss.Style
	detailValue  lipgloss.Style
	muted        lipgloss.Style
	good         lipgloss.Style
	warn         lipgloss.Style
	bad          lipgloss.Style
	toastOK      lipgloss.Style
	toastErr     lipgloss.Style
	hint         lipgloss.Style
}

const (
	colAccent = lipgloss.Color("75")  // soft blue — selection / active
	colText   = lipgloss.Color("252") // primary text
	colMuted  = lipgloss.Color("245") // secondary text
	colDim    = lipgloss.Color("240") // dividers / faint
	colGood   = lipgloss.Color("78")  // green
	colWarn   = lipgloss.Color("214") // amber
	colBad    = lipgloss.Color("203") // red
	colHeader = lipgloss.Color("153") // header title
)

func newTheme() theme {
	base := lipgloss.NewStyle()
	return theme{
		headerTitle: base.Foreground(colHeader).Bold(true),
		scopeKey:    base.Foreground(colMuted),
		scopeVal:    base.Foreground(colText),
		ribbonOn:    base.Foreground(colAccent).Bold(true),
		ribbonOff:   base.Foreground(colMuted),
		ribbonArrow: base.Foreground(colDim),
		railTitle:   base.Foreground(colMuted).Bold(true),
		railOn:      base.Foreground(colAccent).Bold(true),
		railOff:     base.Foreground(colMuted),
		footer:      base.Foreground(colDim),
		divider:     base.Foreground(colDim),

		paneTitle:    base.Foreground(colHeader).Bold(true),
		listSelected: base.Foreground(colAccent).Bold(true),
		listNormal:   base.Foreground(colText),
		groupHeader:  base.Foreground(colMuted).Bold(true),
		detailLabel:  base.Foreground(colMuted),
		detailValue:  base.Foreground(colText),
		muted:        base.Foreground(colMuted),
		good:         base.Foreground(colGood),
		warn:         base.Foreground(colWarn),
		bad:          base.Foreground(colBad),
		toastOK:      base.Foreground(colGood).Bold(true),
		toastErr:     base.Foreground(colBad).Bold(true),
		hint:         base.Foreground(colDim),
	}
}

// statusStyle maps a proposal status to a consistent color.
func (t theme) statusStyle(status string) lipgloss.Style {
	switch status {
	case "approved", "applied":
		return t.good
	case "open", "in_review":
		return lipgloss.NewStyle().Foreground(colAccent)
	case "request_changes", "blocked":
		return t.warn
	case "rejected", "expired", "withdrawn", "superseded":
		return t.bad
	default: // draft and anything unknown
		return t.muted
	}
}

// goalStatusStyle maps a goal lifecycle status to a color.
func (t theme) goalStatusStyle(status string) lipgloss.Style {
	switch status {
	case "complete":
		return t.good
	case "active", "verifying", "planned":
		return lipgloss.NewStyle().Foreground(colAccent)
	case "blocked":
		return t.bad
	case "paused":
		return t.warn
	default:
		return t.muted
	}
}
