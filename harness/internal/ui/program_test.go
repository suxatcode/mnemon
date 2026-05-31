package ui

import (
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/exp/teatest"
)

// TestProgramLaunchesAndQuits drives the root model through the real bubbletea
// program loop (via teatest's simulated terminal), proving the console launches,
// renders, and quits on q — the U0 acceptance gate, deterministically and without
// a flaky real-pty dependency.
func TestProgramLaunchesAndQuits(t *testing.T) {
	tm := teatest.NewTestModel(t, newModel("."), teatest.WithInitialTermSize(80, 24))
	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	tm.WaitFinished(t, teatest.WithFinalTimeout(5*time.Second))
}
