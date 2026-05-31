// Package coordination is the read model for multi-agent collaboration topology.
//
// It rides the existing kernel: collaboration is modeled as governed events on
// schema.Event (no new event struct, no DB), and the topology is a materialized
// fold over the append-only log — exactly the pattern status uses for
// ProjectStatus. These are teamwork *semantics* (claim/fork/merge/...), not
// chatter: the events are canonical, and the view is replayable from the log.
//
// This package defines the coordination vocabulary and fold. Governed mutations
// emit these events through the route=coordination apply executor, using the same
// proposal -> review -> apply -> audit path as the eval and memory routes.
package coordination

import (
	"sort"
	"strings"

	"github.com/mnemon-dev/mnemon/harness/internal/lifecycle/schema"
)

// Coordination event types — the minimal vocabulary on the kernel. Each is a
// teamwork operator, not a message.
const (
	EventTaskClaimed      = "task.claimed"
	EventTaskReleased     = "task.released"
	EventTaskForked       = "task.forked"
	EventTaskJoined       = "task.joined"
	EventGroupCreated     = "group.created"
	EventGroupMemberAdded = "group.member_added"
	EventEvidenceLinked   = "evidence.linked"
	EventConflictDetected = "conflict.detected"

	// Compensating (inverse) events — undo a link / membership via a new governed
	// event, never by deleting history (the log is append-only).
	EventEvidenceUnlinked   = "evidence.unlinked"
	EventGroupMemberRemoved = "group.member_removed"
)

// Payload field conventions for coordination events.
const (
	FieldTaskID       = "task_id"
	FieldOwner        = "owner"       // host; defaults to the event's host
	FieldForkedFrom   = "forked_from" // parent task id
	FieldJoinedInto   = "joined_into" // task id this one merged into
	FieldGroupID      = "group_id"
	FieldMember       = "member"        // host added to a group
	FieldEvidenceRef  = "evidence_ref"  // evidence linked to a task
	FieldConflictWith = "conflict_with" // task id in conflict
	FieldReason       = "reason"
)

// IsCoordinationType reports whether an event type is part of the coordination
// vocabulary (so readers can fold only collaboration operators).
func IsCoordinationType(t string) bool {
	switch t {
	case EventTaskClaimed, EventTaskReleased, EventTaskForked, EventTaskJoined,
		EventGroupCreated, EventGroupMemberAdded, EventEvidenceLinked, EventConflictDetected,
		EventEvidenceUnlinked, EventGroupMemberRemoved:
		return true
	}
	return false
}

// Task is one unit of claimable work and its current ownership/lineage.
type Task struct {
	ID           string   `json:"id"`
	Owner        string   `json:"owner,omitempty"` // host currently holding the claim
	Status       string   `json:"status"`          // claimed | released | forked | joined
	ForkedFrom   string   `json:"forked_from,omitempty"`
	JoinedInto   string   `json:"joined_into,omitempty"`
	EvidenceRefs []string `json:"evidence_refs,omitempty"`
	LastEventID  string   `json:"last_event_id,omitempty"`
	LastTS       string   `json:"last_ts,omitempty"`
}

// Group is a set of hosts collaborating under one banner.
type Group struct {
	ID      string   `json:"id"`
	Members []string `json:"members,omitempty"`
	LastTS  string   `json:"last_ts,omitempty"`
}

// Conflict is a detected clash between two tasks (overlap, duplicate, contention).
type Conflict struct {
	Between      []string `json:"between"` // task ids
	Reason       string   `json:"reason,omitempty"`
	EvidenceRefs []string `json:"evidence_refs,omitempty"`
	LastEventID  string   `json:"last_event_id,omitempty"`
	LastTS       string   `json:"last_ts,omitempty"`
}

// MergeCandidate is a set of tasks linked to the same evidence — likely
// duplicate or mergeable work surfaced for review (not auto-merged).
type MergeCandidate struct {
	EvidenceRef string   `json:"evidence_ref"`
	Tasks       []string `json:"tasks"`
}

// View is the materialized coordination topology: who owns what, fork lineage,
// groups, conflicts, and merge candidates — all derived from the event log.
type View struct {
	Tasks           []Task           `json:"tasks,omitempty"`
	Groups          []Group          `json:"groups,omitempty"`
	Conflicts       []Conflict       `json:"conflicts,omitempty"`
	MergeCandidates []MergeCandidate `json:"merge_candidates,omitempty"`
}

// DeriveView folds the coordination events in the log (oldest first, as the event
// log returns them) into the topology. It is pure and replayable: the same log
// always yields the same view.
func DeriveView(events []schema.Event) View {
	tasks := map[string]*Task{}
	var taskOrder []string
	groups := map[string]*Group{}
	var groupOrder []string
	var conflicts []Conflict
	// evidenceRef -> ordered task ids linked to it (for merge candidates).
	evidenceTasks := map[string][]string{}

	ensureTask := func(id string) *Task {
		if id == "" {
			return nil
		}
		t, ok := tasks[id]
		if !ok {
			t = &Task{ID: id}
			tasks[id] = t
			taskOrder = append(taskOrder, id)
		}
		return t
	}
	ensureGroup := func(id string) *Group {
		if id == "" {
			return nil
		}
		g, ok := groups[id]
		if !ok {
			g = &Group{ID: id}
			groups[id] = g
			groupOrder = append(groupOrder, id)
		}
		return g
	}
	addMember := func(g *Group, host string) {
		if g == nil || host == "" {
			return
		}
		for _, m := range g.Members {
			if m == host {
				return
			}
		}
		g.Members = append(g.Members, host)
	}

	for _, ev := range events {
		if !IsCoordinationType(ev.Type) {
			continue
		}
		host := derefHost(ev)
		switch ev.Type {
		case EventTaskClaimed:
			if t := ensureTask(field(ev, FieldTaskID)); t != nil {
				t.Owner = firstNonEmpty(field(ev, FieldOwner), host)
				t.Status = "claimed"
				stamp(t, ev)
			}
		case EventTaskReleased:
			if t := ensureTask(field(ev, FieldTaskID)); t != nil {
				t.Status = "released"
				stamp(t, ev)
			}
		case EventTaskForked:
			if t := ensureTask(field(ev, FieldTaskID)); t != nil {
				t.ForkedFrom = field(ev, FieldForkedFrom)
				t.Owner = firstNonEmpty(field(ev, FieldOwner), host)
				t.Status = "forked"
				stamp(t, ev)
			}
		case EventTaskJoined:
			if t := ensureTask(field(ev, FieldTaskID)); t != nil {
				t.JoinedInto = field(ev, FieldJoinedInto)
				t.Status = "joined"
				stamp(t, ev)
			}
		case EventGroupCreated:
			if g := ensureGroup(field(ev, FieldGroupID)); g != nil {
				addMember(g, firstNonEmpty(field(ev, FieldOwner), host))
				g.LastTS = ev.TS
			}
		case EventGroupMemberAdded:
			if g := ensureGroup(field(ev, FieldGroupID)); g != nil {
				addMember(g, firstNonEmpty(field(ev, FieldMember), host))
				g.LastTS = ev.TS
			}
		case EventEvidenceLinked:
			ref := field(ev, FieldEvidenceRef)
			if t := ensureTask(field(ev, FieldTaskID)); t != nil && ref != "" {
				t.EvidenceRefs = appendUnique(t.EvidenceRefs, ref)
				stamp(t, ev)
				evidenceTasks[ref] = appendUnique(evidenceTasks[ref], t.ID)
			}
		case EventEvidenceUnlinked:
			// Compensation: undo a prior link in the materialized view. The linked
			// and unlinked events both remain in the log; the fold reflects the net.
			ref := field(ev, FieldEvidenceRef)
			if t := ensureTask(field(ev, FieldTaskID)); t != nil && ref != "" {
				t.EvidenceRefs = removeString(t.EvidenceRefs, ref)
				stamp(t, ev)
				evidenceTasks[ref] = removeString(evidenceTasks[ref], t.ID)
			}
		case EventGroupMemberRemoved:
			if g := ensureGroup(field(ev, FieldGroupID)); g != nil {
				g.Members = removeString(g.Members, firstNonEmpty(field(ev, FieldMember), host))
				g.LastTS = ev.TS
			}
		case EventConflictDetected:
			a := field(ev, FieldTaskID)
			b := field(ev, FieldConflictWith)
			between := nonEmpty(a, b)
			if len(between) > 0 {
				c := Conflict{Between: between, Reason: field(ev, FieldReason), LastEventID: ev.ID, LastTS: ev.TS}
				if ref := field(ev, FieldEvidenceRef); ref != "" {
					c.EvidenceRefs = []string{ref}
				}
				conflicts = append(conflicts, c)
			}
		}
	}

	view := View{}
	for _, id := range taskOrder {
		view.Tasks = append(view.Tasks, *tasks[id])
	}
	for _, id := range groupOrder {
		view.Groups = append(view.Groups, *groups[id])
	}
	view.Conflicts = conflicts

	// Merge candidates: any evidence linked to two or more tasks is duplicate /
	// mergeable work — surfaced for review, never auto-merged.
	var refs []string
	for ref, ids := range evidenceTasks {
		if len(ids) >= 2 {
			refs = append(refs, ref)
		}
	}
	sort.Strings(refs)
	for _, ref := range refs {
		view.MergeCandidates = append(view.MergeCandidates, MergeCandidate{EvidenceRef: ref, Tasks: evidenceTasks[ref]})
	}
	return view
}

func stamp(t *Task, ev schema.Event) {
	t.LastEventID = ev.ID
	t.LastTS = ev.TS
}

func field(ev schema.Event, key string) string {
	if ev.Payload == nil {
		return ""
	}
	if s, ok := ev.Payload[key].(string); ok {
		return strings.TrimSpace(s)
	}
	return ""
}

func derefHost(ev schema.Event) string {
	if ev.Host == nil {
		return ""
	}
	return strings.TrimSpace(*ev.Host)
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func nonEmpty(vals ...string) []string {
	var out []string
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			out = append(out, v)
		}
	}
	return out
}

func appendUnique(list []string, v string) []string {
	for _, x := range list {
		if x == v {
			return list
		}
	}
	return append(list, v)
}

func removeString(list []string, v string) []string {
	out := list[:0:0]
	for _, x := range list {
		if x != v {
			out = append(out, x)
		}
	}
	return out
}
