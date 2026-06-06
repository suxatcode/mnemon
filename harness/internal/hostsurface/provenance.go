package hostsurface

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"time"

	"github.com/mnemon-dev/mnemon/harness/internal/lifecycle/eventlog"
	"github.com/mnemon-dev/mnemon/harness/internal/lifecycle/schema"
)

// EventProjectionApplied records that Mnemon projected a context fragment onto a
// host surface — the PUSH side of the access loop made auditable. It carries a
// content digest so the writeback verifier (status, ring 1) can tell whether a
// host read the CURRENT projection (echoes this digest) or a stale one.
const EventProjectionApplied = "projection.applied"

// Projected-fragment kinds (the context Mnemon pushes to a host surface).
const (
	FragmentProfile      = "PROFILE"
	FragmentCoordination = "COORDINATION"
)

// fragmentDigest is a deterministic content hash of a projected fragment.
// Re-projecting identical content yields the same digest (idempotency).
func fragmentDigest(fragment any) (string, error) {
	data, err := json.Marshal(fragment)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("sha256:%x", sha256.Sum256(data)), nil
}

// recordProjectionApplied emits a projection.applied event for a projection
// written onto host surface `ref`, carrying the precomputed content `digest` that
// the writeback verifier matches the host's echo against. It is idempotent: if the
// latest projection.applied for this (host, kind, ref) already carries the same
// digest, no new event is emitted, so re-projecting unchanged context appends
// nothing.
func recordProjectionApplied(projectRoot, host, loop, kind, ref, digest string) error {
	store, err := eventlog.New(projectRoot)
	if err != nil {
		return err
	}
	events, _ := store.ReadAll() // best-effort over the readable log
	for i := len(events) - 1; i >= 0; i-- {
		ev := events[i]
		if ev.Type != EventProjectionApplied {
			continue
		}
		if projectionField(ev, "host") == host && projectionField(ev, "fragment") == kind && projectionField(ev, "projection_ref") == ref {
			if projectionField(ev, "context_digest") == digest {
				return nil // unchanged — idempotent, no new event
			}
			break // a newer projection of this ref exists with a different digest — emit
		}
	}
	now := time.Now().UTC()
	hostVal, loopVal := host, loop
	event := schema.Event{
		SchemaVersion: schema.Version,
		ID:            fmt.Sprintf("evt_projection_applied_%s_%s_%s_%d", host, loop, kind, now.UnixNano()),
		TS:            now.Format(time.RFC3339),
		Type:          EventProjectionApplied,
		Loop:          &loopVal,
		Host:          &hostVal,
		Actor:         "projector",
		Source:        "mnemon-harness.projection",
		CorrelationID: "projection:" + host + "." + loop,
		ProjectRoot:   projectRoot,
		Scope:         schema.ProjectScopeWithProfile(projectRoot, "", host, loop, "").Map(),
		Payload: map[string]any{
			"host":           host,
			"loop":           loop,
			"fragment":       kind,
			"projection_ref": ref,
			"context_digest": digest,
			"binding":        host + "." + loop,
		},
	}
	for attempt := 0; attempt < 100; attempt++ {
		if attempt > 0 {
			event.ID = fmt.Sprintf("evt_projection_applied_%s_%s_%s_%d_%d", host, loop, kind, now.UnixNano(), attempt+1)
		}
		if err := store.Append(event); err != nil {
			if eventlog.IsDuplicateEventID(err) {
				continue
			}
			return err
		}
		return nil
	}
	return fmt.Errorf("append projection.applied: exhausted duplicate id retries")
}

func projectionField(ev schema.Event, key string) string {
	if ev.Payload == nil {
		return ""
	}
	if s, ok := ev.Payload[key].(string); ok {
		return s
	}
	return ""
}
