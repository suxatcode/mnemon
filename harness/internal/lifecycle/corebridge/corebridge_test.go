package corebridge

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/mnemon-dev/mnemon/harness/core/contract"
	"github.com/mnemon-dev/mnemon/harness/internal/lifecycle/schema"
)

func strp(s string) *string { return &s }

// fullEvent is a schema.Event with every field populated (incl. host-only fields) so the
// round-trip exercises the typed payload extension end to end.
func fullEvent() schema.Event {
	return schema.Event{
		SchemaVersion: 1,
		ID:            "evt_memory_x_20260606",
		TS:            "2026-06-06T12:00:00Z",
		Type:          "memory.hot_write_observed",
		Loop:          strp("memory"),
		Host:          strp("claude-code"),
		Actor:         "host-agent",
		Source:        "mnemon.event_emit",
		CorrelationID: "memory:ins-1",
		CausedBy:      strp("evt_parent"),
		Payload:       map[string]any{"insight_id": "ins-1", "weight": 0.7},
		ProjectRoot:   "/repo",
		Store:         "default",
		Scope:         map[string]any{"type": "project", "id": "project"},
		Severity:      "info",
		Privacy:       map[string]any{"redacted": false},
		ArtifactRefs:  []schema.RawObject{{"uri": "mnemon://a/1"}},
		StatusRef:     map[string]any{"uri": "mnemon://status/1"},
		ProposalRef:   map[string]any{"uri": "mnemon://proposal/1"},
		AuditRef:      map[string]any{"uri": "mnemon://audit/1"},
		Hashes:        map[string]any{"content": "sha256:abc"},
	}
}

func TestSchemaEventEnvelopeRoundTrip(t *testing.T) {
	orig := fullEvent()
	env, err := ToEnvelope(orig)
	if err != nil {
		t.Fatalf("ToEnvelope: %v", err)
	}
	if env.Source != contract.ActorID(orig.Source) {
		t.Fatalf("envelope source = %q, want %q", env.Source, orig.Source)
	}
	if env.ExternalID != orig.ID {
		t.Fatalf("envelope ExternalID = %q, want the lifecycle event id %q", env.ExternalID, orig.ID)
	}
	if env.Event.Type != orig.Type || env.Event.CorrelationID != orig.CorrelationID {
		t.Fatalf("canonical event lost type/correlation: %+v", env.Event)
	}
	if _, ok := env.Event.Payload[HostExtensionKey]; !ok {
		t.Fatalf("canonical payload must carry the host extension under %q", HostExtensionKey)
	}
	if env.Event.Payload["insight_id"] != "ins-1" {
		t.Fatalf("domain payload keys must ride alongside the host extension")
	}

	// Simulate the canonical event after it has passed through the kernel's JSON log:
	// marshal + unmarshal so payload values become their JSON forms (the real read path).
	data, err := json.Marshal(env.Event)
	if err != nil {
		t.Fatalf("marshal canonical event: %v", err)
	}
	var logged contract.Event
	if err := json.Unmarshal(data, &logged); err != nil {
		t.Fatalf("unmarshal canonical event: %v", err)
	}

	back, err := FromEvent(logged)
	if err != nil {
		t.Fatalf("FromEvent: %v", err)
	}

	// The domain payload survives a JSON round-trip with number drift (0.7 stays a number);
	// compare via JSON to normalize int/float representation, then assert structural identity
	// of everything else.
	if !reflect.DeepEqual(jsonNorm(t, orig.Payload), jsonNorm(t, back.Payload)) {
		t.Fatalf("domain payload not preserved:\n orig=%v\n back=%v", orig.Payload, back.Payload)
	}
	back.Payload = nil
	orig2 := orig
	orig2.Payload = nil
	if !reflect.DeepEqual(orig2, back) {
		t.Fatalf("host-lifecycle fields not preserved on round-trip:\n orig=%+v\n back=%+v", orig2, back)
	}
}

func TestToEnvelopeRejectsReservedKey(t *testing.T) {
	ev := fullEvent()
	ev.Payload = map[string]any{HostExtensionKey: "collision"}
	if _, err := ToEnvelope(ev); err == nil {
		t.Fatalf("ToEnvelope must reject a domain payload that uses the reserved key %q", HostExtensionKey)
	}
}

func jsonNorm(t *testing.T, v any) any {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var out any
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return out
}
