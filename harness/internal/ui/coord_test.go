package ui

import (
	"strings"
	"testing"

	"github.com/mnemon-dev/mnemon/harness/internal/ui/read"
)

func topologySnapshot() read.Snapshot {
	return read.Snapshot{
		Coordination: read.Coordination{
			Tasks: []read.CoordTask{
				{ID: "T1", Owner: "codex", Status: "claimed", EvidenceRefs: []string{"E7"}, LastEventID: "ev1"},
				{ID: "T2", Owner: "claude-code", Status: "forked", ForkedFrom: "T1", LastEventID: "ev2"},
			},
			Groups:          []read.CoordGroup{{ID: "G1", Members: []string{"codex", "claude-code"}}},
			Conflicts:       []read.CoordConflict{{Between: []string{"T1", "T2"}, Reason: "overlap"}},
			MergeCandidates: []read.CoordMerge{{EvidenceRef: "E7", Tasks: []string{"T1", "T2"}}},
		},
		Events: []read.Event{
			{ID: "ev1", TS: "2026-05-30T10:00:00Z", Type: "task.claimed", Host: sp("codex"), Raw: "{}"},
			{ID: "ev2", TS: "2026-05-30T11:00:00Z", Type: "task.forked", Host: sp("claude-code"), Raw: "{}"},
		},
	}
}

// TestCoordViewShowsTopology proves the Band 2 gate surface: the read-only
// coordination page shows ownership, fork lineage, groups, conflicts, and merge
// candidates from the materialized view.
func TestCoordViewShowsTopology(t *testing.T) {
	m := withSnapshot(topologySnapshot())
	m = send(m, "7")
	if m.active != pageCoord {
		t.Fatalf("7 should open the Coordination page, active=%d", m.active)
	}
	out := m.View()
	for _, want := range []string{"TASKS (2)", "T1", "T2", "owner codex", "forked from T1", "GROUPS (1)", "G1", "CONFLICTS (1)", "overlap", "MERGE CANDIDATES (1)", "E7"} {
		if !strings.Contains(out, want) {
			t.Errorf("coordination view missing %q:\n%s", want, out)
		}
	}
}

// TestCoordJumpsToTaskEvent proves the page is navigable: enter on a task lands on
// the Evidence page focused on that task's latest event.
func TestCoordJumpsToTaskEvent(t *testing.T) {
	m := withSnapshot(topologySnapshot())
	m = send(m, "7")
	m = send(m, "enter") // task T1 -> its last event ev1
	if m.active != pageEvidence {
		t.Fatalf("enter on a task should land on Evidence, active=%d", m.active)
	}
	if !m.evDetail {
		t.Error("the task's latest event should open in detail")
	}
}

func TestCoordViewEmpty(t *testing.T) {
	m := withSnapshot(read.Snapshot{})
	m = send(m, "7")
	if !strings.Contains(m.View(), "no coordination yet") {
		t.Errorf("empty coordination view should explain the empty state:\n%s", m.View())
	}
}
