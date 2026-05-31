package read

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/mnemon-dev/mnemon/harness/internal/app"
)

// maxEvents caps how many of the most recent events the snapshot retains, keeping
// render and load responsive on a large event log.
const maxEvents = 2000

// Snapshot is an immutable view of the project's .mnemon state at one refresh.
// Each section carries its own error so a missing or locked store degrades only
// that pane; the rest of the console keeps working.
type Snapshot struct {
	Root     string
	LoadedAt time.Time

	Scope        Scope
	Goals        []GoalView
	Proposals    []Proposal
	Profile      Profile
	Coordination Coordination
	Readback     []HostReadback
	Events       []Event       // reverse-chronological (newest first), capped to maxEvents
	Audits       []AuditRecord // newest first by record name

	// EventLogSize/Mod are the size and mod-time (unix nanos) of events.jsonl as
	// observed at the moment its content was read. The poll baseline is set from
	// these (not a later re-stat) so a concurrent append during the load can never
	// be silently swallowed: the baseline is <= the content actually loaded, so the
	// next poll always notices later growth.
	EventLogSize int64
	EventLogMod  int64

	Err SectionErrors
}

// SectionErrors records the first error encountered loading each section. A nil
// error means the section loaded (possibly empty).
type SectionErrors struct {
	Goals        error
	Proposals    error
	Profile      error
	Coordination error
	Readback     error
	Events       error
	Audit        error
}

// Scope is the context the operator acts under, derived from the project root and
// the most recent scoped event.
type Scope struct {
	ProjectRoot       string
	Store             string
	Host              string
	Loop              string
	ProfileRef        string
	BindingScope      string
	EventLogPath      string
	ProjectionHealth  string // "ok", "N issue(s)", or "unavailable"
	AuditHealth       string // audit↔event integrity: "ok", "N issue(s)", or "unavailable"
	AntipatternHealth string // anti-pattern scan: "ok", "N finding(s)", or "unavailable"
	LastWriteback     string // RFC3339 ts of the latest event, or ""
}

// GoalView is a goal's facade status plus its objective and plan, recovered from
// goal.json (the flat facade status view drops those richer fields).
type GoalView struct {
	app.GoalStatusView
	Objective string
	Plan      *GoalPlan
}

// Load reads the full snapshot for the project rooted at root. It never returns
// an error: per-section failures are captured in Snapshot.Err so the caller can
// render each pane independently. A passive UI refresh must not mutate the store,
// so Load only reads (it never calls EnsureProject or any writer).
func Load(root string) Snapshot {
	if strings.TrimSpace(root) == "" {
		root = "."
	}
	absRoot := root
	if a, err := filepath.Abs(root); err == nil {
		absRoot = a
	}

	snap := Snapshot{Root: absRoot, LoadedAt: time.Now()}
	h := app.New(root)

	snap.Events, snap.EventLogSize, snap.EventLogMod, snap.Err.Events = loadEvents(absRoot)
	snap.Proposals, snap.Err.Proposals = loadProposals(h)
	snap.Profile, snap.Err.Profile = loadProfile(h)
	snap.Coordination, snap.Err.Coordination = loadCoordination(h)
	snap.Readback, snap.Err.Readback = loadReadback(h)
	snap.Audits, snap.Err.Audit = loadAudits(h)
	snap.Goals, snap.Err.Goals = loadGoals(h, absRoot)
	snap.Scope = loadScope(h, absRoot)

	return snap
}

// EventLogPath is the on-disk path of the raw event stream for a project root.
func EventLogPath(absRoot string) string {
	return filepath.Join(absRoot, ".mnemon", "events.jsonl")
}

// EventLogStat reports the size and modification time (unix nanos) of the project
// event log, resolving root the same way Load does. ok is false when the log is
// absent. The console polls this cheaply to detect appended events without
// re-reading the whole log every tick.
func EventLogStat(root string) (size int64, modNanos int64, ok bool) {
	if strings.TrimSpace(root) == "" {
		root = "."
	}
	absRoot := root
	if a, err := filepath.Abs(root); err == nil {
		absRoot = a
	}
	info, err := os.Stat(EventLogPath(absRoot))
	if err != nil {
		return 0, 0, false
	}
	return info.Size(), info.ModTime().UnixNano(), true
}

func loadEvents(absRoot string) ([]Event, int64, int64, error) {
	path := EventLogPath(absRoot)
	f, err := os.Open(path)
	if err != nil {
		return nil, 0, 0, err
	}
	defer f.Close()
	// Stat the open fd BEFORE reading: the observed size/mod is then <= the content
	// we read (a concurrent append lands after the stat), so the poll baseline can
	// never overshoot the loaded content and silently swallow that append.
	info, err := f.Stat()
	if err != nil {
		return nil, 0, 0, err
	}
	size, mod := info.Size(), info.ModTime().UnixNano()
	data, err := io.ReadAll(f)
	if err != nil {
		return nil, size, mod, err
	}
	lines := strings.Split(string(data), "\n")
	events := make([]Event, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var ev Event
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			// Skip an unparsable line rather than failing the whole stream.
			continue
		}
		ev.Raw = line
		events = append(events, ev)
	}
	// Reverse to newest-first, then cap.
	for i, j := 0, len(events)-1; i < j; i, j = i+1, j-1 {
		events[i], events[j] = events[j], events[i]
	}
	if len(events) > maxEvents {
		events = events[:maxEvents]
	}
	return events, size, mod, nil
}

func loadProposals(h *app.Harness) ([]Proposal, error) {
	var buf bytes.Buffer
	if err := h.ProposalList(&buf, nil, "json"); err != nil {
		return nil, err
	}
	var out []Proposal
	if err := decodeJSON(buf.Bytes(), &out); err != nil {
		return nil, err
	}
	return out, nil
}

func loadProfile(h *app.Harness) (Profile, error) {
	var buf bytes.Buffer
	// Empty id/host/loop -> default profile, all entries.
	if err := h.ProfileShow(&buf, "", "", "", "json"); err != nil {
		return Profile{}, err
	}
	var prof Profile
	if err := decodeJSON(buf.Bytes(), &prof); err != nil {
		return Profile{}, err
	}
	return prof, nil
}

func loadCoordination(h *app.Harness) (Coordination, error) {
	var buf bytes.Buffer
	if err := h.Coordination(&buf, "json"); err != nil {
		return Coordination{}, err
	}
	var c Coordination
	if err := decodeJSON(buf.Bytes(), &c); err != nil {
		return Coordination{}, err
	}
	return c, nil
}

func loadReadback(h *app.Harness) ([]HostReadback, error) {
	var buf bytes.Buffer
	if err := h.Readback(&buf, "json"); err != nil {
		return nil, err
	}
	var rb []HostReadback
	if err := decodeJSON(buf.Bytes(), &rb); err != nil {
		return nil, err
	}
	return rb, nil
}

func loadAudits(h *app.Harness) ([]AuditRecord, error) {
	var buf bytes.Buffer
	if err := h.AuditList(&buf, "", "json"); err != nil {
		return nil, err
	}
	var recs []AuditRecord
	if err := decodeJSON(buf.Bytes(), &recs); err != nil {
		return nil, err
	}
	// Records embed a timestamp in their name (…-20060102T150405…); name-desc sort
	// puts the newest first.
	sort.SliceStable(recs, func(i, j int) bool {
		return recs[i].Audit.Metadata.Name > recs[j].Audit.Metadata.Name
	})
	return recs, nil
}

func loadGoals(h *app.Harness, absRoot string) ([]GoalView, error) {
	goalsDir := filepath.Join(absRoot, ".mnemon", "harness", "goals")
	entries, err := os.ReadDir(goalsDir)
	if err != nil {
		return nil, err
	}
	var goals []GoalView
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		id := e.Name()
		gv := GoalView{}
		if status, serr := h.GoalStatus(id); serr == nil {
			gv.GoalStatusView = status
		} else {
			gv.GoalStatusView = app.GoalStatusView{ID: id, Status: "unknown"}
		}
		// Recover objective + plan from goal.json (facade status view drops them).
		if raw, rerr := os.ReadFile(filepath.Join(goalsDir, id, "goal.json")); rerr == nil {
			var g Goal
			if json.Unmarshal(raw, &g) == nil {
				gv.Objective = g.Objective
				gv.Plan = g.Plan
			}
		}
		goals = append(goals, gv)
	}
	// Active (non-complete) goals first, then by id for stability.
	sort.SliceStable(goals, func(i, j int) bool {
		ai, aj := isActiveGoal(goals[i].Status), isActiveGoal(goals[j].Status)
		if ai != aj {
			return ai
		}
		return goals[i].ID < goals[j].ID
	})
	return goals, nil
}

func isActiveGoal(status string) bool {
	switch status {
	case "complete", "blocked":
		return false
	default:
		return true
	}
}

// loadScope reads the live project scope through the facade as a single JSON read
// and fills the surface-local context (project root, event-log path, projection
// health). The event-walk that used to derive scope here now lives in the status
// projection (read via app.ProjectScope), so scope has a single source.
func loadScope(h *app.Harness, absRoot string) Scope {
	sc := Scope{
		ProjectRoot:       absRoot,
		EventLogPath:      EventLogPath(absRoot),
		ProjectionHealth:  projectionHealth(h),
		AuditHealth:       auditHealth(h),
		AntipatternHealth: antipatternHealth(h),
	}
	var buf bytes.Buffer
	if err := h.ProjectScope(&buf, "json"); err != nil {
		return sc
	}
	var derived struct {
		Store         string `json:"store"`
		Host          string `json:"host"`
		Loop          string `json:"loop"`
		ProfileRef    string `json:"profile_ref"`
		BindingScope  string `json:"binding_scope"`
		LastWriteback string `json:"last_writeback"`
	}
	if err := decodeJSON(buf.Bytes(), &derived); err != nil {
		return sc
	}
	sc.Store = derived.Store
	sc.Host = derived.Host
	sc.Loop = derived.Loop
	sc.ProfileRef = derived.ProfileRef
	sc.BindingScope = derived.BindingScope
	sc.LastWriteback = derived.LastWriteback
	return sc
}

// projectionHealth summarizes declaration/host-binding validity via the facade.
func projectionHealth(h *app.Harness) string {
	lines, err := h.LoopValidate()
	if err != nil {
		return "unavailable"
	}
	issues := 0
	for _, l := range lines {
		low := strings.ToLower(l)
		if strings.Contains(low, "error") || strings.Contains(low, "invalid") ||
			strings.Contains(low, "missing") || strings.Contains(low, "fail") {
			issues++
		}
	}
	if issues == 0 {
		return "ok"
	}
	return fmt.Sprintf("%d issue(s)", issues)
}

// auditHealth summarizes audit↔event integrity via the facade (read-only).
func auditHealth(h *app.Harness) string {
	issues, ok := h.AuditIntegrity()
	if !ok {
		return "unavailable"
	}
	if issues == 0 {
		return "ok"
	}
	return fmt.Sprintf("%d issue(s)", issues)
}

// antipatternHealth summarizes the anti-pattern scan via the facade (read-only;
// it never writes the report a passive refresh must not produce).
func antipatternHealth(h *app.Harness) string {
	status, findings, ok := h.AntipatternStatus()
	if !ok {
		return "unavailable"
	}
	if findings == 0 {
		if status == "" || status == "pass" {
			return "ok"
		}
		return status
	}
	return fmt.Sprintf("%d finding(s)", findings)
}

// decodeJSON unmarshals facade JSON, tolerating the trailing newline writeJSON
// and json.Encoder append.
func decodeJSON(data []byte, v any) error {
	data = bytes.TrimSpace(data)
	if len(data) == 0 {
		return nil
	}
	return json.Unmarshal(data, v)
}
