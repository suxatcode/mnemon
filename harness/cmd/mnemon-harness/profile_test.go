package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mnemon-dev/mnemon/harness/internal/lifecycle/eventlog"
)

func TestProfileCommandSmoke(t *testing.T) {
	root := t.TempDir()
	restoreProfileFlags(t)
	profileRoot = root
	profileID = "personal-default"
	profileEntryID = "focused-commits"
	profileEntryType = "work_style"
	profileSummary = "Prefer focused harness-only commits"
	profileContent = "Keep harness changes staged and avoid stable mnemon release paths."
	profileEvidence = []string{"manual=plan:E2=User boundary instruction"}
	profileProjectTo = []string{"codex/memory"}

	addCmd, addOutput := testCommand()
	if err := runProfileEntryAdd(addCmd, nil); err != nil {
		t.Fatalf("runProfileEntryAdd returned error: %v", err)
	}
	if !strings.Contains(addOutput.String(), "recorded profile entry focused-commits") {
		t.Fatalf("unexpected add output: %s", addOutput.String())
	}
	path := filepath.Join(root, ".mnemon", "harness", "profiles", "personal-default", "profile.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read profile: %v", err)
	}
	for _, want := range []string{
		`"scope_type": "personal"`,
		`"evidence"`,
		`"projection_targets"`,
		`"host": "codex"`,
		`"loop": "memory"`,
	} {
		if !strings.Contains(string(data), want) {
			t.Fatalf("expected %s in profile:\n%s", want, string(data))
		}
	}

	profileFormat = "text"
	profileHost = "codex"
	profileLoop = "memory"
	showCmd, showOutput := testCommand()
	if err := runProfileShow(showCmd, nil); err != nil {
		t.Fatalf("runProfileShow returned error: %v", err)
	}
	if !strings.Contains(showOutput.String(), "entries: 1") || !strings.Contains(showOutput.String(), "focused-commits") {
		t.Fatalf("unexpected show output: %s", showOutput.String())
	}

	profileHost = "claude"
	profileLoop = "skill"
	filteredCmd, filteredOutput := testCommand()
	if err := runProfileShow(filteredCmd, nil); err != nil {
		t.Fatalf("filtered runProfileShow returned error: %v", err)
	}
	if !strings.Contains(filteredOutput.String(), "entries: 0") {
		t.Fatalf("expected filtered profile to have no entries: %s", filteredOutput.String())
	}

	events, err := eventlog.New(root)
	if err != nil {
		t.Fatalf("eventlog.New returned error: %v", err)
	}
	allEvents, err := events.ReadAll()
	if err != nil {
		t.Fatalf("ReadAll returned error: %v", err)
	}
	if len(allEvents) != 1 || allEvents[0].Type != "profile.entry_recorded" {
		t.Fatalf("expected one profile.entry_recorded event, got %#v", allEvents)
	}
	if allEvents[0].Scope["profile_ref"] != "profile:personal/personal-default" {
		t.Fatalf("expected profile_ref scope, got %#v", allEvents[0].Scope)
	}
}

func TestProfileEntryAddRequiresEvidence(t *testing.T) {
	restoreProfileFlags(t)
	profileRoot = t.TempDir()
	profileEntryType = "preference"
	profileSummary = "Evidence required"
	profileContent = "Do not record profile entries without evidence."

	err := runProfileEntryAdd(mustTestCommand(t), nil)
	if err == nil || !strings.Contains(err.Error(), "entry evidence is required") {
		t.Fatalf("expected evidence error, got %v", err)
	}
}

func restoreProfileFlags(t *testing.T) {
	t.Helper()
	oldRoot := profileRoot
	oldID := profileID
	oldEntryID := profileEntryID
	oldType := profileEntryType
	oldSummary := profileSummary
	oldContent := profileContent
	oldEvidence := profileEvidence
	oldProjectTo := profileProjectTo
	oldHost := profileHost
	oldLoop := profileLoop
	oldFormat := profileFormat
	t.Cleanup(func() {
		profileRoot = oldRoot
		profileID = oldID
		profileEntryID = oldEntryID
		profileEntryType = oldType
		profileSummary = oldSummary
		profileContent = oldContent
		profileEvidence = oldEvidence
		profileProjectTo = oldProjectTo
		profileHost = oldHost
		profileLoop = oldLoop
		profileFormat = oldFormat
	})
	clearProfileFlags()
}

func clearProfileFlags() {
	profileRoot = "."
	profileID = defaultProfileID
	profileEntryID = ""
	profileEntryType = ""
	profileSummary = ""
	profileContent = ""
	profileEvidence = nil
	profileProjectTo = nil
	profileHost = ""
	profileLoop = ""
	profileFormat = "text"
}
