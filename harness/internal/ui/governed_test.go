package ui

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mnemon-dev/mnemon/harness/internal/app"
)

// runCmd executes a tea.Cmd with a short deadline. Timer commands (tea.Tick for
// the poll and toast-expiry) block for seconds when invoked directly; the real
// runtime fires them asynchronously, so for synchronous test stepping we simply
// skip any cmd that doesn't return promptly.
func runCmd(cmd tea.Cmd) tea.Msg {
	if cmd == nil {
		return nil
	}
	ch := make(chan tea.Msg, 1)
	go func() { ch <- cmd() }()
	select {
	case msg := <-ch:
		return msg
	case <-time.After(300 * time.Millisecond):
		return nil
	}
}

// drain executes a command chain to completion, feeding each produced message
// back through Update — a synchronous stand-in for the bubbletea event loop so a
// governed write (bind.Result → reload → snapshotMsg) settles within one step. It
// unpacks tea.BatchMsg the way the real runtime would.
func drain(m model, cmd tea.Cmd) model {
	queue := []tea.Cmd{cmd}
	for steps := 0; len(queue) > 0 && steps < 64; steps++ {
		c := queue[0]
		queue = queue[1:]
		msg := runCmd(c)
		if msg == nil {
			continue
		}
		if batch, ok := msg.(tea.BatchMsg); ok {
			queue = append(queue, batch...)
			continue
		}
		nm, next := m.Update(msg)
		m = nm.(model)
		if next != nil {
			queue = append(queue, next)
		}
	}
	return m
}

func step(m model, key string) model {
	nm, cmd := m.Update(keyOf(key))
	return drain(nm.(model), cmd)
}

func loadModel(t *testing.T, root string) model {
	t.Helper()
	m := newModel(root)
	nm, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = nm.(model)
	return drain(m, m.loadCmd())
}

// createMemoryProposal seeds a route=memory proposal whose apply succeeds (one
// profile_entry target + one matching profile.entry.add op + evidence).
func createMemoryProposal(t *testing.T, root, id string) {
	t.Helper()
	uri := "profile:personal/personal-default"
	content := app.ProposalContent{
		Title:         "Record concise-response preference",
		Summary:       "Add a durable preference entry from review evidence.",
		ChangeSummary: "Add one evidence-backed profile entry.",
		Targets:       []string{"profile_entry=" + uri},
		Operations: []string{
			`profile.entry.add=` + uri + `=Add preference={"entry_id":"ui-demo-pref","entry_type":"preference","summary":"Prefer concise responses","content":"The user prefers concise, direct responses.","project_to":["codex/memory"]}`,
		},
		Evidence:          []string{"eval_report=.mnemon/harness/reports/demo.json=demo evidence"},
		ValidationSummary: "Verify the entry projects to codex/memory.",
	}
	var buf bytes.Buffer
	if err := app.New(root).ProposalCreate(&buf, id, "memory", "low", content); err != nil {
		t.Fatalf("seed memory proposal: %v", err)
	}
}

func eventTypes(t *testing.T, root string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, ".mnemon", "events.jsonl"))
	if err != nil {
		return ""
	}
	return string(data)
}

// TestGovernedApproveApplyLoop is the U2 acceptance gate: drive a draft
// route=memory proposal open → in_review → approved → applied entirely from the
// UI, and confirm the loop closed — profile.entry_recorded + audit.recorded
// events appear and the proposal carries audit_refs.
func TestGovernedApproveApplyLoop(t *testing.T) {
	root := t.TempDir()
	id := "ui-memory-loop"
	createMemoryProposal(t, root, id)

	m := loadModel(t, root)
	m.active = pageProposals
	if len(m.orderedProposals()) != 1 || m.orderedProposals()[0].Status != "draft" {
		t.Fatalf("expected one draft proposal, got %+v", m.orderedProposals())
	}

	// Every action is mediated by a confirm modal (action key, then y).
	for _, key := range []string{"o", "v", "a", "A"} {
		m = step(m, key)
		if m.confirm == nil {
			t.Fatalf("action %q should open a confirm modal", key)
		}
		if !strings.Contains(m.confirm.call, "app.Proposal") {
			t.Fatalf("confirm should name the facade call, got %q", m.confirm.call)
		}
		m = step(m, "y")
	}

	p := m.orderedProposals()[0]
	if p.Status != "applied" {
		t.Fatalf("proposal should be applied, got %q (toast=%q)", p.Status, m.toast)
	}
	if len(p.AuditRefs) == 0 {
		t.Fatalf("applied proposal should carry audit_refs; got none")
	}

	log := eventTypes(t, root)
	if !strings.Contains(log, "profile.entry_recorded") {
		t.Error("apply should emit profile.entry_recorded")
	}
	if !strings.Contains(log, "audit.recorded") {
		t.Error("apply should emit audit.recorded")
	}
	if !strings.Contains(log, "proposal.applied") {
		t.Error("apply should emit proposal.applied")
	}

	// The detail pane surfaces the loop-closure proof: the applied status, the
	// emitted event id, and the freshly written audit_refs.
	m.prDetail = true
	out := m.View()
	if !strings.Contains(out, "loop closed") || !strings.Contains(out, "proposal.applied") {
		t.Errorf("proposal detail should show the emitted apply event; got:\n%s", out)
	}
	if !strings.Contains(out, "audit/records/proposal-"+id) {
		t.Errorf("proposal detail should show the freshly written audit_refs; got:\n%s", out)
	}
}

// TestIllegalTransitionDisabled proves illegal actions are not offered: applying
// a draft (apply is legal only from approved) does not mutate state.
func TestIllegalTransitionDisabled(t *testing.T) {
	root := t.TempDir()
	createMemoryProposal(t, root, "ui-illegal")
	m := loadModel(t, root)
	m.active = pageProposals

	m = step(m, "A") // apply from draft — illegal
	if m.confirm != nil {
		t.Fatal("apply from draft must not open a confirm modal")
	}
	if !m.toastErr {
		t.Error("an illegal action should surface a disabled-action toast")
	}
	if got := m.orderedProposals()[0].Status; got != "draft" {
		t.Errorf("illegal action must not mutate state; status now %q", got)
	}
}

// TestUnsupportedRouteApplySurfacesBoundary proves applying an unsupported route
// surfaces the facade's not-implemented result verbatim and does NOT mutate the
// proposal in the UI — the facade still writes its boundary audit.
func TestUnsupportedRouteApplySurfacesBoundary(t *testing.T) {
	root := t.TempDir()
	id := "ui-docs-unsupported"
	content := app.ProposalContent{
		Title:             "Docs change",
		Summary:           "A docs-route proposal whose apply is not implemented.",
		ChangeSummary:     "Edit a doc.",
		Targets:           []string{"docs=docs/example.md"},
		ValidationSummary: "n/a",
	}
	var buf bytes.Buffer
	if err := app.New(root).ProposalCreate(&buf, id, "docs", "low", content); err != nil {
		t.Fatalf("seed docs proposal: %v", err)
	}

	m := loadModel(t, root)
	m.active = pageProposals
	for _, key := range []string{"o", "v", "a"} {
		m = step(m, key)
		m = step(m, "y")
	}
	if got := m.orderedProposals()[0].Status; got != "approved" {
		t.Fatalf("precondition: proposal should be approved, got %q", got)
	}

	m = step(m, "A") // apply
	m = step(m, "y")

	if !m.toastErr || !strings.Contains(m.toast, "not_implemented") {
		t.Errorf("unsupported apply should surface not_implemented verbatim; toast=%q err=%t", m.toast, m.toastErr)
	}
	if got := m.orderedProposals()[0].Status; got != "approved" {
		t.Errorf("unsupported apply must not mutate the proposal in the UI; status now %q", got)
	}
	// The facade still records a boundary audit + audit.recorded event.
	if log := eventTypes(t, root); !strings.Contains(log, "audit.recorded") {
		t.Error("facade should write a boundary audit.recorded event on unsupported apply")
	}
}
