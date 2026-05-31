package ui

import (
	"fmt"
	"sort"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mnemon-dev/mnemon/harness/internal/ui/read"
)

// The Hosts page shows who is active on this ledger, when each host last wrote
// back, and the loop it is currently in. It is derived purely from the existing
// event stream (no new event types); the first newest event a host appears in is
// its current state.

type hostRow struct {
	host          string
	lastWriteback string // newest event ts for this host (RFC3339)
	loop          string // newest event's loop
	events        int
	newestEventID string // focus target for the Evidence jump
}

// hostRows folds the event stream (newest-first) into one row per host identity,
// most-recently-active first.
func (m *model) hostRows() []hostRow {
	idx := map[string]int{}
	var rows []hostRow
	for i := range m.snap.Events {
		ev := &m.snap.Events[i]
		h := ev.HostName()
		if h == "" {
			continue
		}
		if j, ok := idx[h]; ok {
			rows[j].events++
			continue
		}
		// First occurrence is the newest (events are newest-first): current state.
		idx[h] = len(rows)
		rows = append(rows, hostRow{
			host:          h,
			lastWriteback: ev.TS,
			loop:          ev.LoopName(),
			events:        1,
			newestEventID: ev.ID,
		})
	}
	sort.SliceStable(rows, func(i, j int) bool { return rows[i].lastWriteback > rows[j].lastWriteback })
	return rows
}

func (m *model) updateHosts(msg tea.KeyMsg) tea.Cmd {
	rows := m.hostRows()
	switch msg.String() {
	case "j", "down":
		m.hostsSel = clampIdx(m.hostsSel+1, len(rows))
	case "k", "up":
		m.hostsSel = clampIdx(m.hostsSel-1, len(rows))
	case "enter":
		if m.hostsSel >= 0 && m.hostsSel < len(rows) {
			if m.gotoEventByID(rows[m.hostsSel].newestEventID) {
				return nil
			}
			return m.setToast("host's latest event not loaded", true)
		}
	}
	return nil
}

func (m *model) viewHosts(w, h int) string {
	rows := m.hostRows()
	if m.snap.Err.Events != nil && len(rows) == 0 {
		return m.emptyPane("HOSTS", "unavailable: "+m.snap.Err.Events.Error(), h)
	}
	if len(rows) == 0 {
		return m.emptyPane("HOSTS", "no host has written back yet — events carry the host identity.", h)
	}

	rb := m.readbackByHost()
	now := time.Now()
	lines := []string{m.th.paneTitle.Render(fmt.Sprintf("HOSTS (%d)  ·  readback: observed / acted-but-unattributed / silent", len(rows)))}
	for i, r := range rows {
		when := relTime(r.lastWriteback, now)
		state, ok := rb[r.host]
		if i == m.hostsSel {
			lines = append(lines, m.th.listSelected.Render(fmt.Sprintf("▸ %s  loop %s  last %s  %d events  %s",
				pad(r.host, 16), pad(orDash(r.loop), 10), pad(when, 12), r.events, readbackLabel(state, ok))))
			continue
		}
		line := "  " + m.th.detailValue.Render(pad(r.host, 16)) + "  " +
			m.th.muted.Render("loop ") + m.th.detailValue.Render(pad(orDash(r.loop), 10)) + "  " +
			m.th.muted.Render("last ") + m.th.detailValue.Render(pad(when, 12)) + "  " +
			m.th.muted.Render(fmt.Sprintf("%d events", r.events)) + "  " +
			m.readbackBadge(state, ok)
		lines = append(lines, line)
	}
	return viewport(lines, m.hostsSel+1, h)
}

// readbackByHost indexes the writeback-verifier readback by host.
func (m *model) readbackByHost() map[string]read.HostReadback {
	out := make(map[string]read.HostReadback, len(m.snap.Readback))
	for _, r := range m.snap.Readback {
		out[r.Host] = r
	}
	return out
}

func readbackLabel(rb read.HostReadback, ok bool) string {
	if !ok {
		return "no-projection"
	}
	if rb.Stale {
		return rb.State + " (stale)"
	}
	return rb.State
}

// readbackBadge styles a host's writeback-verification state: observed green,
// stale/unattributed warn, silent bad, no-projection muted.
func (m *model) readbackBadge(rb read.HostReadback, ok bool) string {
	label := readbackLabel(rb, ok)
	switch {
	case !ok:
		return m.th.muted.Render(label)
	case rb.State == ReadbackObserved && !rb.Stale:
		return m.th.good.Render(label)
	case rb.State == ReadbackSilent:
		return m.th.bad.Render(label)
	default: // acted-but-unattributed, or observed-but-stale
		return m.th.warn.Render(label)
	}
}

// Readback state labels mirrored from status (the UI cannot import the inner pkg).
const (
	ReadbackObserved = "observed"
	ReadbackSilent   = "silent"
)
