package status

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/mnemon-dev/mnemon/harness/internal/lifecycle/coordination"
	"github.com/mnemon-dev/mnemon/harness/internal/lifecycle/eventlog"
	"github.com/mnemon-dev/mnemon/harness/internal/lifecycle/layout"
	"github.com/mnemon-dev/mnemon/harness/internal/lifecycle/schema"
)

type Result struct {
	EventCount          int
	LastIncludedEventID string
	Written             []string
}

type document struct {
	SchemaVersion int            `json:"schema_version"`
	Kind          string         `json:"kind"`
	Metadata      map[string]any `json:"metadata"`
	Status        map[string]any `json:"status"`
}

// Scope is the recorded context the project is acting under, derived from the
// most recent scoped events. It is the single source of "current scope":
// materialized into the project status document by Refresh and exposed read-only
// through the app facade, so surfaces decode it instead of re-walking the log.
type Scope struct {
	Store         string `json:"store,omitempty"`
	Host          string `json:"host,omitempty"`
	Loop          string `json:"loop,omitempty"`
	ProfileRef    string `json:"profile_ref,omitempty"`
	BindingScope  string `json:"binding_scope,omitempty"`
	LastWriteback string `json:"last_writeback,omitempty"`
}

func Refresh(root string, now time.Time) (Result, error) {
	paths, err := layout.EnsureProject(root)
	if err != nil {
		return Result{}, err
	}
	store, err := eventlog.New(paths.Root)
	if err != nil {
		return Result{}, err
	}
	events, err := store.ReadAll()
	if err != nil {
		return Result{}, err
	}

	result := Result{EventCount: len(events)}
	if len(events) == 0 {
		return result, nil
	}
	result.LastIncludedEventID = events[len(events)-1].ID

	loopEvents := map[string][]schema.Event{}
	hostEvents := map[string][]schema.Event{}
	jobEvents := map[string][]schema.Event{}
	projectionEvents := map[string][]schema.Event{}
	runnerEvents := map[string][]schema.Event{}
	var daemonEvents []schema.Event

	for _, event := range events {
		if strings.HasPrefix(event.Type, "daemon.") {
			daemonEvents = append(daemonEvents, event)
		}
		if strings.HasPrefix(event.Type, "runner.") {
			if runnerID := payloadString(event.Payload, "runner_id"); runnerID != "" {
				runnerEvents[runnerID] = append(runnerEvents[runnerID], event)
			}
		}
		if event.Loop != nil && *event.Loop != "" {
			loopEvents[*event.Loop] = append(loopEvents[*event.Loop], event)
		}
		if event.Host != nil && *event.Host != "" {
			hostEvents[*event.Host] = append(hostEvents[*event.Host], event)
		}
		if jobID := payloadString(event.Payload, "job_id"); jobID != "" {
			jobEvents[jobID] = append(jobEvents[jobID], event)
		} else if jobID := nestedPayloadString(event.Payload, "target", "job_id"); jobID != "" {
			jobEvents[jobID] = append(jobEvents[jobID], event)
		}
		if strings.HasPrefix(event.Type, "projection.") {
			binding := payloadString(event.Payload, "binding")
			if binding == "" {
				binding = nestedPayloadString(event.Payload, "target", "binding")
			}
			if binding == "" && event.Host != nil && event.Loop != nil {
				binding = *event.Host + "." + *event.Loop
			}
			if binding != "" {
				projectionEvents[binding] = append(projectionEvents[binding], event)
			}
		}
	}

	if path, err := writeStatus(paths, "project.json", projectStatus(events, now)); err != nil {
		return result, err
	} else {
		result.Written = append(result.Written, path)
	}
	for _, loop := range sortedKeys(loopEvents) {
		rel := filepath.Join("loops", loop+".json")
		if path, err := writeStatus(paths, rel, loopStatus(loop, loopEvents[loop], now)); err != nil {
			return result, err
		} else {
			result.Written = append(result.Written, path)
		}
	}
	for _, host := range sortedKeys(hostEvents) {
		rel := filepath.Join("hosts", host+".json")
		if path, err := writeStatus(paths, rel, hostStatus(host, hostEvents[host], now)); err != nil {
			return result, err
		} else {
			result.Written = append(result.Written, path)
		}
	}
	for _, job := range sortedKeys(jobEvents) {
		rel := filepath.Join("jobs", job+".json")
		if path, err := writeStatus(paths, rel, jobStatus(job, jobEvents[job], now)); err != nil {
			return result, err
		} else {
			result.Written = append(result.Written, path)
		}
	}
	for _, binding := range sortedKeys(projectionEvents) {
		rel := filepath.Join("projections", binding+".json")
		if path, err := writeStatus(paths, rel, projectionStatus(binding, projectionEvents[binding], now)); err != nil {
			return result, err
		} else {
			result.Written = append(result.Written, path)
		}
	}
	if len(daemonEvents) > 0 {
		if path, err := writeStatus(paths, "daemon.json", daemonStatus(daemonEvents, now, latestTickLog(paths))); err != nil {
			return result, err
		} else {
			result.Written = append(result.Written, path)
		}
	}
	for _, runner := range sortedKeys(runnerEvents) {
		rel := filepath.Join("runners", runner+".json")
		if path, err := writeStatus(paths, rel, runnerStatus(runner, runnerEvents[runner], now)); err != nil {
			return result, err
		} else {
			result.Written = append(result.Written, path)
		}
	}
	// Materialize the coordination topology only when collaboration events exist,
	// so non-coordinating projects keep a clean status dir.
	if view := coordination.DeriveView(events); len(view.Tasks) > 0 || len(view.Groups) > 0 || len(view.Conflicts) > 0 {
		if path, err := writeStatus(paths, "coordination.json", coordinationDocument(view, events, now)); err != nil {
			return result, err
		} else {
			result.Written = append(result.Written, path)
		}
	}
	// Writeback verifier: materialize per-host readback when projections exist.
	if rb := DeriveReadback(events); len(rb) > 0 {
		if path, err := writeStatus(paths, "readback.json", readbackDocument(rb, events, now)); err != nil {
			return result, err
		} else {
			result.Written = append(result.Written, path)
		}
	}

	sort.Strings(result.Written)
	return result, nil
}

func readbackDocument(rb []HostReadback, events []schema.Event, now time.Time) document {
	last := events[len(events)-1]
	var observed, mismatch, unattributed, silent, stale int
	for _, r := range rb {
		switch r.State {
		case ReadbackObserved:
			observed++
		case ReadbackMismatch:
			mismatch++
		case ReadbackUnattributed:
			unattributed++
		case ReadbackSilent:
			silent++
		}
		if r.Stale {
			stale++
		}
	}
	return document{
		SchemaVersion: 1,
		Kind:          "ReadbackStatus",
		Metadata: map[string]any{
			"name": "readback",
		},
		Status: baseStatus(phaseFor(events), now, last.ID, map[string]any{
			"hosts": rb,
			"counters": map[string]any{
				"observed":               observed,
				"mismatch":               mismatch,
				"acted_but_unattributed": unattributed,
				"silent":                 silent,
				"stale":                  stale,
			},
		}, events),
	}
}

func coordinationDocument(view coordination.View, events []schema.Event, now time.Time) document {
	last := events[len(events)-1]
	return document{
		SchemaVersion: 1,
		Kind:          "CoordinationStatus",
		Metadata: map[string]any{
			"name": "coordination",
		},
		Status: baseStatus(phaseFor(events), now, last.ID, map[string]any{
			"topology": view,
			"counters": map[string]any{
				"tasks":            len(view.Tasks),
				"groups":           len(view.Groups),
				"conflicts":        len(view.Conflicts),
				"merge_candidates": len(view.MergeCandidates),
			},
		}, events),
	}
}

func projectStatus(events []schema.Event, now time.Time) document {
	phase := phaseFor(events)
	loopCount := map[string]struct{}{}
	hostCount := map[string]struct{}{}
	for _, event := range events {
		if event.Loop != nil && *event.Loop != "" {
			loopCount[*event.Loop] = struct{}{}
		}
		if event.Host != nil && *event.Host != "" {
			hostCount[*event.Host] = struct{}{}
		}
	}
	last := events[len(events)-1]
	return document{
		SchemaVersion: 1,
		Kind:          "ProjectStatus",
		Metadata: map[string]any{
			"name": "project",
		},
		Status: baseStatus(phase, now, last.ID, map[string]any{
			"counters": map[string]any{
				"events": len(events),
				"loops":  len(loopCount),
				"hosts":  len(hostCount),
			},
			"scope": DeriveScope(events),
		}, events),
	}
}

// DeriveScope walks events newest-first and fills each scope field from the first
// event that carries it — the live context the operator is acting under. events
// arrive oldest-first as the event log returns them, so the walk runs in reverse.
// This is the single home of scope derivation; surfaces read it via the facade.
func DeriveScope(events []schema.Event) Scope {
	var sc Scope
	if len(events) == 0 {
		return sc
	}
	sc.LastWriteback = events[len(events)-1].TS
	for i := len(events) - 1; i >= 0; i-- {
		ev := events[i]
		sc.Store = firstNonEmpty(sc.Store, scopeField(ev, "store"))
		sc.Host = firstNonEmpty(sc.Host, scopeField(ev, "host"), deref(ev.Host))
		sc.Loop = firstNonEmpty(sc.Loop, scopeField(ev, "loop"), deref(ev.Loop))
		sc.ProfileRef = firstNonEmpty(sc.ProfileRef, scopeField(ev, "profile_ref"))
		sc.BindingScope = firstNonEmpty(sc.BindingScope, scopeField(ev, "binding_scope"))
		if sc.Store != "" && sc.Host != "" && sc.Loop != "" && sc.ProfileRef != "" && sc.BindingScope != "" {
			break
		}
	}
	return sc
}

func scopeField(ev schema.Event, key string) string {
	if ev.Scope == nil {
		return ""
	}
	if s, ok := ev.Scope[key].(string); ok {
		return strings.TrimSpace(s)
	}
	return ""
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return strings.TrimSpace(*s)
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func loopStatus(loop string, events []schema.Event, now time.Time) document {
	phase := phaseFor(events)
	last := events[len(events)-1]
	return document{
		SchemaVersion: 1,
		Kind:          "LoopStatus",
		Metadata: map[string]any{
			"name": loop,
			"loop": loop,
		},
		Status: baseStatus(phase, now, last.ID, map[string]any{
			"counters": map[string]any{
				"events":         len(events),
				"open_proposals": countTypePrefix(events, "proposal.created"),
				"failed_jobs":    countTypePrefix(events, "job.failed"),
			},
		}, events),
	}
}

func hostStatus(host string, events []schema.Event, now time.Time) document {
	phase := phaseFor(events)
	last := events[len(events)-1]
	return document{
		SchemaVersion: 1,
		Kind:          "HostStatus",
		Metadata: map[string]any{
			"name": host,
			"host": host,
		},
		Status: baseStatus(phase, now, last.ID, map[string]any{
			"capabilities": map[string]string{
				"host.app_server.run": "unknown",
			},
			"counters": map[string]any{"events": len(events)},
		}, events),
	}
}

func jobStatus(job string, events []schema.Event, now time.Time) document {
	phase := phaseFor(events)
	last := events[len(events)-1]
	return document{
		SchemaVersion: 1,
		Kind:          "JobStatus",
		Metadata: map[string]any{
			"name": job,
			"job":  job,
		},
		Status: baseStatus(phase, now, last.ID, map[string]any{
			"counters": map[string]any{"events": len(events)},
		}, events),
	}
}

func projectionStatus(binding string, events []schema.Event, now time.Time) document {
	phase := "current"
	if phaseFor(events) == "blocked" {
		phase = "blocked"
	} else if phaseFor(events) == "degraded" {
		phase = "degraded"
	}
	last := events[len(events)-1]
	return document{
		SchemaVersion: 1,
		Kind:          "ProjectionStatus",
		Metadata: map[string]any{
			"name":    binding,
			"binding": binding,
		},
		Status: baseStatus(phase, now, last.ID, map[string]any{
			"last_projection_event_id": last.ID,
			"drift": map[string]any{
				"state":             driftState(events),
				"observed_event_id": last.ID,
				"details":           []any{},
			},
		}, events),
	}
}

func daemonStatus(events []schema.Event, now time.Time, tick map[string]any) document {
	last := events[len(events)-1]
	phase := payloadString(last.Payload, "to_phase")
	if phase == "" {
		phase = phaseFor(events)
	}
	extra := map[string]any{
		"last_processed_event_id": payloadString(last.Payload, "last_processed_event_id"),
		"counters": map[string]any{
			"events": len(events),
		},
	}
	if tick != nil {
		extra["last_tick"] = tick
	}
	return document{
		SchemaVersion: 1,
		Kind:          "DaemonStatus",
		Metadata: map[string]any{
			"name": "project-daemon",
		},
		Status: baseStatus(phase, now, last.ID, extra, events),
	}
}

func runnerStatus(runner string, events []schema.Event, now time.Time) document {
	last := events[len(events)-1]
	phase := payloadString(last.Payload, "to_phase")
	if phase == "" {
		phase = phaseFor(events)
	}
	extra := map[string]any{
		"counters": map[string]any{
			"events": len(events),
		},
	}
	if reportRef, ok := last.Payload["report_ref"].(map[string]any); ok {
		extra["last_report_ref"] = reportRef
	}
	if failureClass := payloadString(last.Payload, "failure_class"); failureClass != "" {
		extra["failure_class"] = failureClass
	}
	if last.Host != nil && *last.Host != "" {
		extra["host"] = *last.Host
	}
	return document{
		SchemaVersion: 1,
		Kind:          "RunnerStatus",
		Metadata: map[string]any{
			"name":      runner,
			"runner_id": runner,
		},
		Status: baseStatus(phase, now, last.ID, extra, events),
	}
}

func baseStatus(phase string, now time.Time, lastEventID string, extra map[string]any, events []schema.Event) map[string]any {
	status := map[string]any{
		"phase":                  phase,
		"last_refreshed_at":      now.UTC().Format(time.RFC3339),
		"last_included_event_id": lastEventID,
		"conditions":             conditionsFor(phase, now, lastEventID, events),
	}
	for key, value := range extra {
		status[key] = value
	}
	return status
}

func conditionsFor(phase string, now time.Time, lastEventID string, events []schema.Event) []schema.Condition {
	ts := now.UTC().Format(time.RFC3339)
	switch phase {
	case "blocked":
		return []schema.Condition{{
			Type:             "Blocked",
			Status:           "true",
			Reason:           "LifecycleBlocked",
			Message:          "One or more lifecycle events report a blocked condition.",
			LastTransitionTS: ts,
			LastEventID:      lastEventID,
		}}
	case "degraded":
		return []schema.Condition{{
			Type:             "Degraded",
			Status:           "true",
			Reason:           "LifecycleDegraded",
			Message:          "One or more lifecycle events report a failed or degraded condition.",
			LastTransitionTS: ts,
			LastEventID:      lastEventID,
		}}
	default:
		_ = events
		return []schema.Condition{{
			Type:             "Ready",
			Status:           "true",
			Reason:           "EventsMaterialized",
			LastTransitionTS: ts,
			LastEventID:      lastEventID,
		}}
	}
}

func phaseFor(events []schema.Event) string {
	phase := "ready"
	for _, event := range events {
		if strings.Contains(event.Type, "blocked") || event.Severity == "critical" {
			return "blocked"
		}
		if strings.Contains(event.Type, "failed") || event.Severity == "error" {
			phase = "degraded"
		}
	}
	return phase
}

func driftState(events []schema.Event) string {
	for i := len(events) - 1; i >= 0; i-- {
		switch events[i].Type {
		case "projection.drift_observed":
			return "drifted"
		case "projection.repaired":
			return "none"
		}
	}
	return "unknown"
}

func countTypePrefix(events []schema.Event, prefix string) int {
	var count int
	for _, event := range events {
		if strings.HasPrefix(event.Type, prefix) {
			count++
		}
	}
	return count
}

func payloadString(payload map[string]any, key string) string {
	value, ok := payload[key]
	if !ok {
		return ""
	}
	text, _ := value.(string)
	return text
}

func nestedPayloadString(payload map[string]any, parent, key string) string {
	value, ok := payload[parent]
	if !ok {
		return ""
	}
	object, ok := value.(map[string]any)
	if !ok {
		return ""
	}
	return payloadString(object, key)
}

func latestTickLog(paths layout.Paths) map[string]any {
	file, err := os.Open(filepath.Join(paths.HarnessDir, "daemon", "tick-log.jsonl"))
	if err != nil {
		return nil
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	var latest map[string]any
	for scanner.Scan() {
		var record map[string]any
		if err := json.Unmarshal(scanner.Bytes(), &record); err == nil && record != nil {
			latest = record
		}
	}
	return latest
}

func sortedKeys[T any](items map[string]T) []string {
	keys := make([]string, 0, len(items))
	for key := range items {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func writeStatus(paths layout.Paths, rel string, doc document) (string, error) {
	path := filepath.Join(paths.StatusDir, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", fmt.Errorf("create status parent: %w", err)
	}
	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal status: %w", err)
	}
	data = append(data, '\n')
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return "", fmt.Errorf("create temp status: %w", err)
	}
	tmpPath := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		return "", fmt.Errorf("write temp status: %w", err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return "", fmt.Errorf("close temp status: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath)
		return "", fmt.Errorf("replace status: %w", err)
	}
	return path, nil
}
