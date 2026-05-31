package status

import (
	"testing"

	"github.com/mnemon-dev/mnemon/harness/internal/lifecycle/schema"
)

func projAppliedEvent(id, host, ref, digest, ts string) schema.Event {
	h := host
	loop := "memory"
	return schema.Event{
		SchemaVersion: schema.Version,
		ID:            id,
		TS:            ts,
		Type:          "projection.applied",
		Loop:          &loop,
		Host:          &h,
		Actor:         "projector",
		Source:        "mnemon-harness.projection",
		CorrelationID: "projection:" + host,
		Payload:       map[string]any{"host": host, "context_digest": digest, "projection_ref": ref},
	}
}

func hostWriteback(id, host, ts string, payload map[string]any, causedBy string) schema.Event {
	h := host
	loop := "memory"
	ev := schema.Event{
		SchemaVersion: schema.Version,
		ID:            id,
		TS:            ts,
		Type:          "memory.hot_write_observed",
		Loop:          &loop,
		Host:          &h,
		Actor:         "host-agent",
		Source:        "host",
		CorrelationID: "c-" + id,
		Payload:       payload,
	}
	if causedBy != "" {
		ev.CausedBy = &causedBy
	}
	return ev
}

// TestDeriveReadbackThreeStatesAndStaleness is the A2 gate: synthetic events drive
// all three readback states + a staleness lag; a host that wrote back without
// echoing is acted-but-unattributed, never falsely silent; an echo via caused_by
// is attributed.
func TestDeriveReadbackThreeStatesAndStaleness(t *testing.T) {
	events := []schema.Event{
		// observed (echo via payload).
		projAppliedEvent("p-obs", "codex", ".codex/mnemon-memory/PROFILE.json", "sha256:D1", "2026-05-31T10:00:00Z"),
		hostWriteback("w-obs", "codex", "2026-05-31T10:01:00Z", map[string]any{"observed_projection_ref": "sha256:D1", "reason": "acted"}, ""),
		// observed (echo via caused_by pointing at the projection.applied event).
		projAppliedEvent("p-ref", "openclaw", ".openclaw/mnemon-memory/PROFILE.json", "sha256:DR", "2026-05-31T10:00:00Z"),
		hostWriteback("w-ref", "openclaw", "2026-05-31T10:02:00Z", map[string]any{"reason": "acted"}, "p-ref"),
		// acted-but-unattributed: wrote back, no echo.
		projAppliedEvent("p-un", "claude-code", ".claude/mnemon-memory/PROFILE.json", "sha256:DU", "2026-05-31T10:00:00Z"),
		hostWriteback("w-un", "claude-code", "2026-05-31T10:03:00Z", map[string]any{"reason": "acted, no echo"}, ""),
		// silent: projection, no writeback.
		projAppliedEvent("p-si", "hermes", ".hermes/mnemon-memory/PROFILE.json", "sha256:DS", "2026-05-31T10:00:00Z"),
		// stale: echoed an OLD digest; a newer projection is live.
		projAppliedEvent("p-st1", "robusta", ".robusta/mnemon-memory/PROFILE.json", "sha256:OLD", "2026-05-31T10:00:00Z"),
		hostWriteback("w-st", "robusta", "2026-05-31T10:01:00Z", map[string]any{"observed_projection_ref": "sha256:OLD"}, ""),
		projAppliedEvent("p-st2", "robusta", ".robusta/mnemon-memory/PROFILE.json", "sha256:NEW", "2026-05-31T10:05:00Z"),
	}
	byHost := map[string]HostReadback{}
	for _, r := range DeriveReadback(events) {
		byHost[r.Host] = r
	}

	if r := byHost["codex"]; r.State != ReadbackObserved || r.Stale {
		t.Errorf("codex should be observed (current), got %#v", r)
	}
	if r := byHost["openclaw"]; r.State != ReadbackObserved || r.Stale {
		t.Errorf("openclaw (caused_by echo) should be observed, got %#v", r)
	}
	if r := byHost["claude-code"]; r.State != ReadbackUnattributed {
		t.Errorf("claude-code wrote back without echo → must be acted-but-unattributed, never silent; got %#v", r)
	}
	if r := byHost["hermes"]; r.State != ReadbackSilent {
		t.Errorf("hermes never wrote back → silent; got %#v", r)
	}
	if r := byHost["robusta"]; r.State != ReadbackObserved || !r.Stale || r.LiveDigest != "sha256:NEW" || r.ObservedDigest != "sha256:OLD" {
		t.Errorf("robusta should be observed+stale (echoed OLD, live NEW); got %#v", r)
	}
}

// TestDeriveReadbackMismatch is the T1 gate: a host that echoes a digest we never
// projected is mismatch — distinct from acted-but-unattributed (echoed nothing).
// The negative control flips the same fixture to the live digest → observed,
// proving mismatch is not a false alarm. Regression A (empty echo → unattributed)
// and regression B (known-older echo → observed+stale) are locked by
// TestDeriveReadbackThreeStatesAndStaleness, which must stay green under this insert.
func TestDeriveReadbackMismatch(t *testing.T) {
	mismatchEvents := []schema.Event{
		projAppliedEvent("p-m", "codex", ".codex/mnemon-memory/PROJECTION.json", "sha256:LIVE", "2026-05-31T10:00:00Z"),
		hostWriteback("w-m", "codex", "2026-05-31T10:01:00Z", map[string]any{"observed_projection_ref": "sha256:GARBAGE"}, ""),
	}
	byHost := map[string]HostReadback{}
	for _, r := range DeriveReadback(mismatchEvents) {
		byHost[r.Host] = r
	}
	if r := byHost["codex"]; r.State != ReadbackMismatch || r.Stale || r.ObservedDigest != "sha256:GARBAGE" || r.LiveDigest != "sha256:LIVE" {
		t.Errorf("wrong/unknown echo → must be mismatch (not unattributed); got %#v", r)
	}

	// Negative control: same fixture, echo the LIVE digest → observed (no false mismatch).
	liveEvents := []schema.Event{
		projAppliedEvent("p-m", "codex", ".codex/mnemon-memory/PROJECTION.json", "sha256:LIVE", "2026-05-31T10:00:00Z"),
		hostWriteback("w-m", "codex", "2026-05-31T10:01:00Z", map[string]any{"observed_projection_ref": "sha256:LIVE"}, ""),
	}
	for _, r := range DeriveReadback(liveEvents) {
		if r.Host == "codex" && (r.State != ReadbackObserved || r.Stale) {
			t.Errorf("negative control: live-digest echo must be observed, got %#v", r)
		}
	}

	// Empty-ledger guard: the pure fold over zero events yields no rows (so
	// readbackDocument's events[len-1] is never reached on an empty project).
	if got := DeriveReadback(nil); len(got) != 0 {
		t.Errorf("empty ledger must yield no readback rows, got %#v", got)
	}
}

// TestDeriveReadbackEchoViaContextDigestField proves the host can echo the digest
// it read from PROJECTION.json under the observed_context_digest key (not only the
// legacy observed_projection_ref) and still be scored observed.
func TestDeriveReadbackEchoViaContextDigestField(t *testing.T) {
	events := []schema.Event{
		projAppliedEvent("p-env", "codex", ".codex/mnemon-memory/PROJECTION.json", "sha256:ENV", "2026-05-31T10:00:00Z"),
		hostWriteback("w-env", "codex", "2026-05-31T10:01:00Z", map[string]any{"observed_context_digest": "sha256:ENV"}, ""),
	}
	for _, r := range DeriveReadback(events) {
		if r.Host == "codex" {
			if r.State != ReadbackObserved || r.Stale {
				t.Fatalf("echo via observed_context_digest should be observed, got %#v", r)
			}
			return
		}
	}
	t.Fatal("no codex readback derived")
}
