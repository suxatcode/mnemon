package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// returnsQuit reports whether a tea.Cmd resolves to tea.QuitMsg.
func returnsQuit(cmd tea.Cmd) bool {
	if cmd == nil {
		return false
	}
	_, ok := cmd().(tea.QuitMsg)
	return ok
}

func TestModelQuitsOnQ(t *testing.T) {
	m := newModel(".")
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	if !returnsQuit(cmd) {
		t.Fatal("pressing q should issue tea.Quit")
	}
}

func TestModelQuitsOnCtrlC(t *testing.T) {
	m := newModel(".")
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if !returnsQuit(cmd) {
		t.Fatal("pressing ctrl+c should issue tea.Quit")
	}
}

func TestModelViewMentionsRoot(t *testing.T) {
	m := newModel("/tmp/project")
	if !strings.Contains(m.View(), "/tmp/project") {
		t.Fatalf("view should surface the bound root, got: %q", m.View())
	}
}
