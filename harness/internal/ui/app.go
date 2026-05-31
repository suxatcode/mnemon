// Package ui implements the mnemon-harness cognition console: a terminal UI
// layered on the internal/app facade. The screen is the governed improvement
// loop — scope, evidence, proposals (review + apply), audit, next run.
//
// This package owns the bubbletea/lipgloss/bubbles dependency; those libraries
// must not leak into other harness packages or the stable mnemon binary. The
// surface depends only on the facade (ring 6): reads decode facade JSON via the
// read/ subpackage, and writes (U2) route through the bind/ subpackage. The UI
// never writes stores, the event log, or audit directly.
package ui

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mnemon-dev/mnemon/harness/internal/ui/bind"
	"github.com/mnemon-dev/mnemon/harness/internal/ui/read"
)

// pollInterval is how often the console checks the event log for appended events
// so the Evidence stream stays live without a manual refresh.
const pollInterval = 2 * time.Second

// Run launches the cognition harness console bound to the given project root and
// blocks until the user quits. The caller is responsible for confirming an
// interactive terminal is attached; Run assumes a TTY.
func Run(root string) error {
	p := tea.NewProgram(newModel(root), tea.WithAltScreen())
	_, err := p.Run()
	return err
}

type pageID int

const (
	pageScope pageID = iota
	pageEvidence
	pageProposals
	pageProfile
	pageTrace
	pageHosts
	pageCoord
	pageCount
)

var pageNames = [pageCount]string{"SCOPE", "EVIDENCE", "PROPOSALS", "PROFILE", "TRACE", "HOSTS", "COORD"}

const railWidth = 13

// snapshotMsg delivers a freshly loaded read.Snapshot to the model.
type snapshotMsg struct{ snap read.Snapshot }

// model is the root bubbletea model: scope header + loop ribbon + left-rail nav +
// page router. It owns the snapshot; pages keep only their own view state
// (selection, detail-open) and read data from the snapshot.
type model struct {
	root   string
	th     theme
	snap   read.Snapshot
	loaded bool

	active        pageID
	width, height int
	help          bool

	confirm *confirmState

	toast    string
	toastErr bool
	toastSeq int

	// filtering
	ti        textinput.Model
	filtering bool
	evFilter  string
	prFilter  string

	// live-poll baseline (event log size + mod time)
	pollSize int64
	pollMod  int64

	// per-page view state
	scopeSel    int
	scopeDetail bool
	evSel       int
	evDetail    bool
	prSel       int
	prDetail    bool
	prSelected  map[string]bool // proposals multi-selected for bulk review/apply
	pfSel       int
	pfDetail    bool

	// Trace page: focal proposal id whose lineage is shown, and the selection
	// among that lineage's navigable steps.
	traceID  string
	traceSel int

	// Hosts page: selection among host identities derived from the event log.
	hostsSel int

	// Coordination page: selection among tasks in the materialized topology.
	coordSel int
}

func newModel(root string) model {
	if strings.TrimSpace(root) == "" {
		root = "."
	}
	ti := textinput.New()
	ti.Prompt = "/"
	ti.CharLimit = 80
	return model{
		root:   root,
		th:     newTheme(),
		active: pageScope,
		width:  80,
		height: 24,
		ti:     ti,
	}
}

func (m model) Init() tea.Cmd { return tea.Batch(m.loadCmd(), m.pollCmd()) }

func (m model) loadCmd() tea.Cmd {
	root := m.root
	return func() tea.Msg { return snapshotMsg{snap: read.Load(root)} }
}

// pollMsg is a periodic tick used to detect appended events.
type pollMsg struct{}

func (m model) pollCmd() tea.Cmd {
	return tea.Tick(pollInterval, func(time.Time) tea.Msg { return pollMsg{} })
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	cmd := (&m).update(msg)
	return m, cmd
}

func (m *model) update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return nil
	case snapshotMsg:
		m.snap = msg.snap
		m.loaded = true
		// Set the poll baseline from the stat the load actually observed (carried on
		// the snapshot), not a later re-stat — otherwise a concurrent append during
		// the load could be silently swallowed.
		m.pollSize, m.pollMod = msg.snap.EventLogSize, msg.snap.EventLogMod
		m.clampSelections()
		return nil
	case pollMsg:
		return m.handlePoll()
	case clearToastMsg:
		// Only clear if this is still the toast we scheduled (a newer toast owns
		// its own expiry).
		if msg.seq == m.toastSeq {
			m.toast = ""
			m.toastErr = false
		}
		return nil
	case bind.Result:
		return m.handleWriteResult(msg)
	case tea.KeyMsg:
		return m.handleKey(msg)
	default:
		// While filtering, route non-key messages (e.g. the textinput cursor-blink
		// tick) to the input so its cursor lifecycle keeps running.
		if m.filtering {
			var cmd tea.Cmd
			m.ti, cmd = m.ti.Update(msg)
			return cmd
		}
	}
	return nil
}

// toastTTL is how long a result/error toast stays before auto-clearing.
const toastTTL = 5 * time.Second

// clearToastMsg requests clearing the toast identified by seq.
type clearToastMsg struct{ seq int }

func (m model) clearToastCmd(seq int) tea.Cmd {
	return tea.Tick(toastTTL, func(time.Time) tea.Msg { return clearToastMsg{seq: seq} })
}

// eventLogChanged reports whether the event log differs from the last-loaded
// baseline (size or mod time), without mutating it.
func (m *model) eventLogChanged() bool {
	size, mod, ok := read.EventLogStat(m.root)
	return ok && (size != m.pollSize || mod != m.pollMod)
}

// handlePoll checks the event log for appended events and reloads the snapshot
// when it has grown or changed, keeping the Evidence stream live without a
// keypress. It always reschedules the next tick. It does NOT advance the baseline
// here — the reload's snapshotMsg sets the baseline from the stat that reload
// actually observed, so an append racing this tick is never swallowed.
func (m *model) handlePoll() tea.Cmd {
	if m.eventLogChanged() {
		return tea.Batch(m.loadCmd(), m.pollCmd())
	}
	return m.pollCmd()
}

// handleWriteResult records the facade's output as a toast and, on success,
// reloads the snapshot so the new status + freshly written audit_refs appear.
func (m *model) handleWriteResult(r bind.Result) tea.Cmd {
	// A bulk apply consumes the selection; clear it so the queue isn't left with
	// stale marks on now-applied proposals.
	if strings.HasPrefix(r.Action, "bulk apply") {
		m.prSelected = nil
	}
	if !r.OK() {
		out := r.Output
		if out == "" {
			out = r.Err.Error()
		}
		return m.setToast(r.Action+" — "+firstLine(out), true)
	}
	out := firstLine(r.Output)
	if out == "" {
		out = r.Action + " ok"
	}
	return tea.Batch(m.setToast(out, false), m.loadCmd())
}

func (m *model) handleKey(msg tea.KeyMsg) tea.Cmd {
	key := msg.String()

	// ctrl+c is the unconditional hard quit (even mid-filter / mid-confirm).
	if key == "ctrl+c" {
		return tea.Quit
	}

	// Filter input mode captures typing until committed or cancelled.
	if m.filtering {
		switch key {
		case "enter":
			m.commitFilter()
			return nil
		case "esc":
			m.cancelFilter()
			return nil
		default:
			var cmd tea.Cmd
			m.ti, cmd = m.ti.Update(msg)
			return cmd
		}
	}

	// A pending confirm modal captures input until resolved — including q, so a
	// governed write is never one stray keystroke from being abandoned by quitting.
	if m.confirm != nil {
		switch key {
		case "y", "enter":
			cmd := m.confirm.cmd
			m.confirm = nil
			return cmd
		case "n", "esc":
			m.confirm = nil
		}
		return nil
	}

	// Global quit (no modal/filter active).
	if key == "q" {
		return tea.Quit
	}
	// Help overlay swallows keys until dismissed.
	if key == "?" {
		m.help = !m.help
		return nil
	}
	if m.help {
		if key == "esc" {
			m.help = false
		}
		return nil
	}
	switch key {
	case "/":
		if m.active == pageEvidence || m.active == pageProposals {
			return m.startFilter()
		}
		return nil
	case "r":
		m.toast = ""
		return m.loadCmd()
	case "tab":
		m.switchPage((m.active + 1) % pageCount)
		return nil
	case "shift+tab":
		m.switchPage((m.active + pageCount - 1) % pageCount)
		return nil
	case "1":
		m.switchPage(pageScope)
		return nil
	case "2":
		m.switchPage(pageEvidence)
		return nil
	case "3":
		m.switchPage(pageProposals)
		return nil
	case "4":
		m.switchPage(pageProfile)
		return nil
	case "5":
		m.openTrace("")
		return nil
	case "6":
		m.switchPage(pageHosts)
		return nil
	case "7":
		m.switchPage(pageCoord)
		return nil
	case "t":
		// Trace the lineage of the focal proposal (the one highlighted on the
		// Proposals page) from anywhere — evidence → proposal → audit → projection.
		m.openTrace("")
		return nil
	case "p":
		m.confirm = &confirmState{
			title: "pause daemon", call: "app.DaemonPause", effect: "active → paused",
			notes: []string{"stops new enqueueing; running jobs are unaffected"},
			cmd:   bind.DaemonPause(m.root, "paused from console"),
		}
		return nil
	case "P":
		m.confirm = &confirmState{
			title: "resume daemon", call: "app.DaemonResume", effect: "paused → active",
			cmd: bind.DaemonResume(m.root),
		}
		return nil
	}

	switch m.active {
	case pageScope:
		return m.updateScope(msg)
	case pageEvidence:
		return m.updateEvidence(msg)
	case pageProposals:
		return m.updateProposals(msg)
	case pageProfile:
		return m.updateProfile(msg)
	case pageTrace:
		return m.updateTrace(msg)
	case pageHosts:
		return m.updateHosts(msg)
	case pageCoord:
		return m.updateCoord(msg)
	}
	return nil
}

func (m *model) switchPage(p pageID) {
	if p == m.active {
		return
	}
	m.active = p
	m.toast = ""
	// Switching pages always lands on the list, never a stale detail pane.
	m.closeAllDetails()
}

// closeAllDetails collapses every page's detail view back to its list. Used on
// page switch and before a cross-page link jump so the source page is not left
// showing a stale detail when the operator returns to it.
func (m *model) closeAllDetails() {
	m.scopeDetail = false
	m.evDetail = false
	m.prDetail = false
	m.pfDetail = false
}

// clampSelections keeps each page's selection within the bounds of freshly
// loaded data, and collapses any open detail whose underlying item no longer
// exists (e.g. a goal completed/removed, or a store errored, between reloads).
func (m *model) clampSelections() {
	m.scopeSel = clampIdx(m.scopeSel, len(m.snap.Goals))
	m.evSel = clampIdx(m.evSel, len(m.filteredEvidence()))
	m.prSel = clampIdx(m.prSel, len(m.filteredProposals()))
	m.pfSel = clampIdx(m.pfSel, len(m.snap.Profile.Entries))
	m.traceSel = clampIdx(m.traceSel, m.traceNavCount())
	m.hostsSel = clampIdx(m.hostsSel, len(m.hostRows()))
	m.coordSel = clampIdx(m.coordSel, len(m.snap.Coordination.Tasks))

	if m.scopeDetail && len(m.snap.Goals) == 0 {
		m.scopeDetail = false
	}
	if m.evDetail && len(m.filteredEvidence()) == 0 {
		m.evDetail = false
	}
	if m.prDetail && len(m.filteredProposals()) == 0 {
		m.prDetail = false
	}
	if m.pfDetail && len(m.snap.Profile.Entries) == 0 {
		m.pfDetail = false
	}
}

// View renders the whole console.
func (m model) View() string {
	if m.width <= 0 {
		m.width = 80
	}
	if m.height <= 0 {
		m.height = 24
	}
	if m.help {
		return m.th.helpText()
	}

	header := m.renderHeader()
	ribbon := m.renderRibbon()
	div := m.th.divider.Render(strings.Repeat("─", m.width))

	topLines := 4 // header(2) + ribbon(1) + divider(1)
	footerLines := 2
	contentH := m.height - topLines - footerLines
	if contentH < 1 {
		contentH = 1
	}
	contentW := m.width - railWidth - 1
	if contentW < 10 {
		contentW = 10
	}

	rail := m.renderRail(contentH)
	content := m.viewContent(contentW, contentH)
	body := lipgloss.JoinHorizontal(lipgloss.Top, rail, m.th.divider.Render("│"), content)

	footer := m.renderFooter()
	return strings.Join([]string{header, ribbon, div, body, footer}, "\n")
}

func (m *model) renderHeader() string {
	sc := m.headerScope()
	field := func(label, val string) string {
		if val == "" {
			val = "—"
		}
		return m.th.scopeKey.Render(label+" ") + m.th.scopeVal.Render(val)
	}
	// health renders one scope-health signal, green when ok, muted when
	// unknown/unavailable, warn otherwise — shared by projection/audit/patterns.
	health := func(label, val string) string {
		var styled string
		switch {
		case val == "ok":
			styled = m.th.good.Render(val)
		case val == "" || val == "…" || val == "unavailable":
			styled = m.th.muted.Render(orDash(val))
		default:
			styled = m.th.warn.Render(val)
		}
		return m.th.scopeKey.Render(label+" ") + styled
	}

	line1 := strings.Join([]string{
		m.th.headerTitle.Render("mnemon-harness"),
		field("project", filepath.Base(sc.ProjectRoot)),
		field("host", sc.Host),
		field("loop", sc.Loop),
		field("profile", sc.ProfileRef),
		health("projection", sc.ProjectionHealth),
		health("audit", sc.AuditHealth),
		health("patterns", sc.AntipatternHealth),
	}, m.th.divider.Render(" · "))

	writeback := "—"
	if sc.LastWriteback != "" {
		writeback = relTime(sc.LastWriteback, time.Now())
	}
	logPath := sc.EventLogPath
	if sc.ProjectRoot != "" {
		if rel := strings.TrimPrefix(logPath, sc.ProjectRoot+string(filepath.Separator)); rel != logPath {
			logPath = rel
		}
	}
	line2 := strings.Join([]string{
		field("root", sc.ProjectRoot),
		field("log", logPath),
		m.th.scopeKey.Render("last writeback ") + m.th.scopeVal.Render(writeback),
	}, m.th.divider.Render(" · "))

	return truncate(line1, m.width) + "\n" + truncate(line2, m.width)
}

func (m *model) renderRibbon() string {
	evCount := len(m.snap.Events)
	openCount := 0
	for _, p := range m.snap.Proposals {
		if p.Status == "open" {
			openCount++
		}
	}
	stages := []struct {
		label string
		page  pageID
		on    bool
	}{
		{fmt.Sprintf("evidence(%d)", evCount), pageEvidence, m.active == pageEvidence},
		{fmt.Sprintf("proposals(%d open)", openCount), pageProposals, m.active == pageProposals},
		{"apply", pageProposals, false},
		{"audit", pageEvidence, false},
		{"next run", pageProfile, m.active == pageProfile},
	}
	parts := make([]string, 0, len(stages)*2)
	for i, s := range stages {
		if i > 0 {
			parts = append(parts, m.th.ribbonArrow.Render(" ▸ "))
		}
		if s.on {
			parts = append(parts, m.th.ribbonOn.Render(s.label))
		} else {
			parts = append(parts, m.th.ribbonOff.Render(s.label))
		}
	}
	return truncate(strings.Join(parts, ""), m.width)
}

func (m *model) renderRail(h int) string {
	var b strings.Builder
	b.WriteString(m.th.railTitle.Render("loop") + "\n")
	for p := pageScope; p < pageCount; p++ {
		name := pageNames[p]
		if p == m.active {
			b.WriteString(m.th.railOn.Render("▸ "+name) + "\n")
		} else {
			b.WriteString(m.th.railOff.Render("  "+name) + "\n")
		}
	}
	b.WriteString(m.th.divider.Render("  ─────") + "\n")
	b.WriteString(m.th.railOff.Render("  audit") + "\n")
	// pad to content height
	body := b.String()
	style := lipgloss.NewStyle().Width(railWidth).Height(h)
	return style.Render(body)
}

func (m *model) renderFooter() string {
	div := m.th.divider.Render(strings.Repeat("─", m.width))
	if m.filtering {
		return div + "\n" + truncate(m.th.detailLabel.Render("filter ")+m.ti.View()+m.th.hint.Render("   enter apply · esc cancel"), m.width)
	}
	if m.confirm != nil {
		return div + "\n" + truncate(m.th.good.Render("y/enter")+m.th.muted.Render(" confirm · ")+m.th.bad.Render("n/esc")+m.th.muted.Render(" cancel"), m.width)
	}
	if m.toast != "" {
		style := m.th.toastOK
		if m.toastErr {
			style = m.th.toastErr
		}
		return div + "\n" + truncate(style.Render(m.toast), m.width)
	}
	detail := m.detailOpen()
	hint := footerHint(m.active, detail)
	if f := m.activeFilter(); f != "" {
		hint = "filter: " + f + "  ·  " + hint
	}
	return div + "\n" + m.th.footer.Render(truncate(hint, m.width))
}

func (m *model) detailOpen() bool {
	switch m.active {
	case pageScope:
		return m.scopeDetail
	case pageEvidence:
		return m.evDetail
	case pageProposals:
		return m.prDetail
	case pageProfile:
		return m.pfDetail
	}
	return false
}

func (m *model) viewContent(w, h int) string {
	if m.confirm != nil {
		return m.viewConfirm(w, h)
	}
	switch m.active {
	case pageScope:
		return m.viewScope(w, h)
	case pageEvidence:
		return m.viewEvidence(w, h)
	case pageProposals:
		return m.viewProposals(w, h)
	case pageProfile:
		return m.viewProfile(w, h)
	case pageTrace:
		return m.viewTrace(w, h)
	case pageHosts:
		return m.viewHosts(w, h)
	case pageCoord:
		return m.viewCoord(w, h)
	}
	return ""
}

func (m *model) headerScope() read.Scope {
	if m.loaded {
		return m.snap.Scope
	}
	abs := m.root
	if a, err := filepath.Abs(m.root); err == nil {
		abs = a
	}
	return read.Scope{ProjectRoot: abs, EventLogPath: read.EventLogPath(abs), ProjectionHealth: "…"}
}

// --- small shared render/format helpers ---

func clampIdx(v, n int) int {
	if n <= 0 {
		return 0
	}
	if v < 0 {
		return 0
	}
	if v >= n {
		return n - 1
	}
	return v
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

func orDash(s string) string {
	if strings.TrimSpace(s) == "" {
		return "—"
	}
	return s
}

func truncate(s string, w int) string {
	if w <= 0 {
		return ""
	}
	if lipgloss.Width(s) <= w {
		return s
	}
	// Trim by display width, accounting for styling by truncating the rendered
	// string conservatively.
	return lipgloss.NewStyle().MaxWidth(w).Render(s)
}

// relTime renders an RFC3339 timestamp as a compact relative duration.
func relTime(ts string, now time.Time) string {
	t, err := time.Parse(time.RFC3339, ts)
	if err != nil {
		return ts
	}
	d := now.Sub(t)
	switch {
	case d < 0:
		return "just now"
	case d < time.Minute:
		return fmt.Sprintf("%ds ago", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	}
}

// absTime renders an RFC3339 timestamp in a compact absolute form, or the raw
// string if it cannot be parsed.
func absTime(ts string) string {
	t, err := time.Parse(time.RFC3339, ts)
	if err != nil {
		return ts
	}
	return t.Format("2006-01-02 15:04")
}
