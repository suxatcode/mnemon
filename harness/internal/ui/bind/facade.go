// Package bind wraps the internal/app facade's governed write operations as
// bubbletea commands for the cognition console. It is the write half of the
// surface and imports ONLY the app facade (ring 6) and stdlib — never a store,
// the event log, or audit directly. Every write therefore goes through the same
// facade the CLI uses, which emits the domain event + audit.recorded + proposal
// audit_refs; the console relies on that and never mutates governed state itself.
package bind

import (
	"bytes"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mnemon-dev/mnemon/harness/internal/app"
)

// Result is the outcome of a governed facade write, delivered as a tea.Msg. The
// model captures the facade's human-readable output verbatim and shows it as a
// result toast, then reloads the snapshot so the new status + audit_refs appear.
type Result struct {
	Action string // human label, e.g. "approve" / "apply"
	Call   string // the facade call named in the confirm modal
	Output string // facade's captured human-readable output
	Err    error
}

// OK reports whether the write succeeded.
func (r Result) OK() bool { return r.Err == nil }

// ProposalTransition wraps app.ProposalTransition(id, status).
func ProposalTransition(root, id, status, action string) tea.Cmd {
	return func() tea.Msg {
		var buf bytes.Buffer
		err := app.New(root).ProposalTransition(&buf, id, status)
		return Result{
			Action: action,
			Call:   fmt.Sprintf("app.ProposalTransition(%q, %q)", id, status),
			Output: strings.TrimSpace(buf.String()),
			Err:    err,
		}
	}
}

// ProposalApply wraps app.ProposalApply(id). Apply is implemented for route=eval
// and route=memory; other routes return the facade's not-implemented result
// (plus the boundary audit it writes), which the UI surfaces verbatim.
func ProposalApply(root, id string) tea.Cmd {
	return func() tea.Msg {
		var buf bytes.Buffer
		err := app.New(root).ProposalApply(&buf, id)
		out := strings.TrimSpace(buf.String())
		if err != nil && out == "" {
			out = err.Error()
		}
		return Result{
			Action: "apply",
			Call:   fmt.Sprintf("app.ProposalApply(%q)", id),
			Output: out,
			Err:    err,
		}
	}
}

// ProposalApplyBatch applies several approved proposals, each through the same
// governed app.ProposalApply call (no batch fast-path that bypasses governance).
// It aggregates per-proposal outcomes; Err is non-nil if any apply failed, so the
// UI flags the batch, while Output lists each result.
func ProposalApplyBatch(root string, ids []string) tea.Cmd {
	return func() tea.Msg {
		h := app.New(root)
		var b strings.Builder
		var firstErr error
		ok := 0
		for _, id := range ids {
			var buf bytes.Buffer
			err := h.ProposalApply(&buf, id)
			if err != nil {
				if firstErr == nil {
					firstErr = err
				}
				msg := strings.TrimSpace(buf.String())
				if msg == "" {
					msg = err.Error()
				}
				fmt.Fprintf(&b, "x %s: %s\n", id, firstLine(msg))
				continue
			}
			ok++
			fmt.Fprintf(&b, "ok %s applied\n", id)
		}
		return Result{
			Action: fmt.Sprintf("bulk apply (%d/%d)", ok, len(ids)),
			Call:   fmt.Sprintf("app.ProposalApply x%d", len(ids)),
			Output: strings.TrimSpace(b.String()),
			Err:    firstErr,
		}
	}
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

// GoalNudge wraps app.GoalNudge for a single goal.
func GoalNudge(root, id, summary string) tea.Cmd {
	return func() tea.Msg {
		results, err := app.New(root).GoalNudge(id, false, 0, summary)
		out := ""
		for _, r := range results {
			if r.Skipped {
				out += fmt.Sprintf("skipped %s (%s) ", r.GoalID, r.Reason)
			} else {
				out += fmt.Sprintf("nudged %s ", r.GoalID)
			}
		}
		return Result{
			Action: "nudge",
			Call:   fmt.Sprintf("app.GoalNudge(%q)", id),
			Output: strings.TrimSpace(out),
			Err:    err,
		}
	}
}

// DaemonPause wraps app.DaemonPause(reason).
func DaemonPause(root, reason string) tea.Cmd {
	return func() tea.Msg {
		var buf bytes.Buffer
		err := app.New(root).DaemonPause(&buf, reason)
		return Result{
			Action: "daemon pause",
			Call:   fmt.Sprintf("app.DaemonPause(%q)", reason),
			Output: strings.TrimSpace(buf.String()),
			Err:    err,
		}
	}
}

// DaemonResume wraps app.DaemonResume().
func DaemonResume(root string) tea.Cmd {
	return func() tea.Msg {
		var buf bytes.Buffer
		err := app.New(root).DaemonResume(&buf)
		return Result{
			Action: "daemon resume",
			Call:   "app.DaemonResume()",
			Output: strings.TrimSpace(buf.String()),
			Err:    err,
		}
	}
}
