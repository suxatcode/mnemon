package coordination

import (
	"testing"

	"github.com/mnemon-dev/mnemon/harness/internal/lifecycle/schema"
)

func coEvent(id, typ, host string, payload map[string]any) schema.Event {
	h := host
	return schema.Event{
		SchemaVersion: schema.Version,
		ID:            id,
		TS:            "2026-05-30T10:00:00Z",
		Type:          typ,
		Host:          &h,
		Actor:         "host-agent",
		Source:        "test",
		CorrelationID: "c",
		Payload:       payload,
	}
}

// TestDeriveViewFoldsTopology proves the coordination fold reconstructs ownership,
// fork lineage, groups, conflicts, and merge candidates from the event log alone —
// replayable, no DB.
func TestDeriveViewFoldsTopology(t *testing.T) {
	events := []schema.Event{
		coEvent("e1", EventTaskClaimed, "codex", map[string]any{FieldTaskID: "T1"}),
		coEvent("e2", EventTaskForked, "claude-code", map[string]any{FieldTaskID: "T2", FieldForkedFrom: "T1"}),
		coEvent("e3", EventGroupCreated, "codex", map[string]any{FieldGroupID: "G1"}),
		coEvent("e4", EventGroupMemberAdded, "codex", map[string]any{FieldGroupID: "G1", FieldMember: "claude-code"}),
		coEvent("e5", EventEvidenceLinked, "codex", map[string]any{FieldTaskID: "T1", FieldEvidenceRef: "E7"}),
		coEvent("e6", EventEvidenceLinked, "claude-code", map[string]any{FieldTaskID: "T2", FieldEvidenceRef: "E7"}),
		coEvent("e7", EventConflictDetected, "codex", map[string]any{FieldTaskID: "T1", FieldConflictWith: "T2", FieldReason: "overlap"}),
		// A non-coordination event must be ignored by the fold.
		coEvent("e8", "memory.hot_write_observed", "codex", map[string]any{"reason": "noise"}),
	}
	v := DeriveView(events)

	// Ownership + fork lineage.
	tasks := map[string]Task{}
	for _, tk := range v.Tasks {
		tasks[tk.ID] = tk
	}
	if len(v.Tasks) != 2 {
		t.Fatalf("want 2 tasks, got %d: %#v", len(v.Tasks), v.Tasks)
	}
	if tasks["T1"].Owner != "codex" || tasks["T1"].Status != "claimed" {
		t.Errorf("T1 ownership/status wrong: %#v", tasks["T1"])
	}
	if tasks["T2"].Owner != "claude-code" || tasks["T2"].ForkedFrom != "T1" || tasks["T2"].Status != "forked" {
		t.Errorf("T2 fork lineage wrong: %#v", tasks["T2"])
	}

	// Group membership.
	if len(v.Groups) != 1 || v.Groups[0].ID != "G1" {
		t.Fatalf("want group G1, got %#v", v.Groups)
	}
	if got := v.Groups[0].Members; !(len(got) == 2 && got[0] == "codex" && got[1] == "claude-code") {
		t.Errorf("G1 members wrong: %#v", got)
	}

	// Conflict.
	if len(v.Conflicts) != 1 || v.Conflicts[0].Reason != "overlap" ||
		len(v.Conflicts[0].Between) != 2 || v.Conflicts[0].Between[0] != "T1" || v.Conflicts[0].Between[1] != "T2" {
		t.Errorf("conflict wrong: %#v", v.Conflicts)
	}

	// Merge candidate: T1 and T2 both linked to E7.
	if len(v.MergeCandidates) != 1 || v.MergeCandidates[0].EvidenceRef != "E7" ||
		len(v.MergeCandidates[0].Tasks) != 2 {
		t.Errorf("merge candidate wrong: %#v", v.MergeCandidates)
	}
}

// TestDeriveViewCompensatingEvents proves the inverse events undo a link /
// membership in the materialized view while both events remain in the log
// (compensation, never deletion).
func TestDeriveViewCompensatingEvents(t *testing.T) {
	events := []schema.Event{
		coEvent("e1", EventTaskClaimed, "codex", map[string]any{FieldTaskID: "T1"}),
		coEvent("e2", EventEvidenceLinked, "codex", map[string]any{FieldTaskID: "T1", FieldEvidenceRef: "E1"}),
		coEvent("e3", EventEvidenceUnlinked, "codex", map[string]any{FieldTaskID: "T1", FieldEvidenceRef: "E1"}),
		coEvent("e4", EventGroupCreated, "codex", map[string]any{FieldGroupID: "G1"}),
		coEvent("e5", EventGroupMemberAdded, "codex", map[string]any{FieldGroupID: "G1", FieldMember: "claude-code"}),
		coEvent("e6", EventGroupMemberRemoved, "codex", map[string]any{FieldGroupID: "G1", FieldMember: "claude-code"}),
	}
	v := DeriveView(events)
	for _, tk := range v.Tasks {
		if tk.ID == "T1" && len(tk.EvidenceRefs) != 0 {
			t.Errorf("unlink should remove the evidence, got %#v", tk.EvidenceRefs)
		}
	}
	if len(v.MergeCandidates) != 0 {
		t.Errorf("no merge candidate after unlink, got %#v", v.MergeCandidates)
	}
	for _, g := range v.Groups {
		if g.ID != "G1" {
			continue
		}
		for _, m := range g.Members {
			if m == "claude-code" {
				t.Errorf("member_removed should drop claude-code, got %#v", g.Members)
			}
		}
		if len(g.Members) != 1 || g.Members[0] != "codex" {
			t.Errorf("G1 should retain only its creator codex, got %#v", g.Members)
		}
	}
}

func TestDeriveViewEmpty(t *testing.T) {
	v := DeriveView(nil)
	if len(v.Tasks)+len(v.Groups)+len(v.Conflicts)+len(v.MergeCandidates) != 0 {
		t.Errorf("empty log should derive empty view, got %#v", v)
	}
}

// TestTaskReleaseAndJoin proves later operators update the same task in place.
func TestTaskReleaseAndJoin(t *testing.T) {
	events := []schema.Event{
		coEvent("e1", EventTaskClaimed, "codex", map[string]any{FieldTaskID: "T1"}),
		coEvent("e2", EventTaskReleased, "codex", map[string]any{FieldTaskID: "T1"}),
		coEvent("e3", EventTaskClaimed, "claude-code", map[string]any{FieldTaskID: "T2"}),
		coEvent("e4", EventTaskJoined, "claude-code", map[string]any{FieldTaskID: "T2", FieldJoinedInto: "T1"}),
	}
	v := DeriveView(events)
	tasks := map[string]Task{}
	for _, tk := range v.Tasks {
		tasks[tk.ID] = tk
	}
	if tasks["T1"].Status != "released" {
		t.Errorf("T1 should be released, got %q", tasks["T1"].Status)
	}
	if tasks["T2"].Status != "joined" || tasks["T2"].JoinedInto != "T1" {
		t.Errorf("T2 should be joined into T1, got %#v", tasks["T2"])
	}
}
