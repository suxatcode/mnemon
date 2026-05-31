package schema

import (
	"strings"
	"testing"
)

func TestDecodeEventValidatesRequiredEnvelope(t *testing.T) {
	data := []byte(`{
		"schema_version": 1,
		"id": "evt_fixture_memory_001",
		"ts": "2026-05-24T08:30:00Z",
		"type": "memory.hot_write_observed",
		"loop": "memory",
		"host": "codex",
		"actor": "host-agent",
		"source": "fixture",
		"correlation_id": "corr_fixture",
		"caused_by": null,
		"payload": {"reason": "fixture"}
	}`)

	event, err := DecodeEvent(data)
	if err != nil {
		t.Fatalf("DecodeEvent returned error: %v", err)
	}
	if event.ID != "evt_fixture_memory_001" {
		t.Fatalf("event id mismatch: %q", event.ID)
	}
}

func TestDecodeEventRejectsMissingRequiredField(t *testing.T) {
	data := []byte(`{
		"schema_version": 1,
		"id": "evt_fixture_memory_001",
		"ts": "2026-05-24T08:30:00Z",
		"type": "memory.hot_write_observed",
		"loop": "memory",
		"host": "codex",
		"actor": "host-agent",
		"source": "fixture",
		"correlation_id": "corr_fixture",
		"payload": {"reason": "fixture"}
	}`)

	_, err := DecodeEvent(data)
	if err == nil || !strings.Contains(err.Error(), "caused_by") {
		t.Fatalf("expected missing caused_by error, got %v", err)
	}
}

func TestDecodeEventRejectsSemanticInvalidEnvelope(t *testing.T) {
	data := []byte(`{
		"schema_version": 1,
		"id": "evt_fixture_memory_001",
		"ts": "not-a-date",
		"type": "Memory.Bad",
		"loop": "memory",
		"host": "codex",
		"actor": "agent",
		"source": "fixture",
		"correlation_id": "corr_fixture",
		"caused_by": null,
		"payload": {}
	}`)

	_, err := DecodeEvent(data)
	if err == nil {
		t.Fatal("expected validation error")
	}
	for _, want := range []string{"ts must be RFC3339", "type must be lower-case", "actor"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("expected error to contain %q, got %v", want, err)
		}
	}
}

func TestProjectScopeMap(t *testing.T) {
	scope := ProjectScopeWithProfile("/repo", "default", "codex", "eval", "").Map()
	for key, want := range map[string]any{
		"id":            "project",
		"type":          "project",
		"project_root":  "/repo",
		"store":         "default",
		"host":          "codex",
		"loop":          "eval",
		"binding_scope": "project",
	} {
		if scope[key] != want {
			t.Fatalf("scope[%s] = %#v, want %#v in %#v", key, scope[key], want, scope)
		}
	}
}

func TestProjectScopeWithProfileMap(t *testing.T) {
	scope := ProjectScopeWithProfile("/repo", "default", "codex", "memory", "profile:personal/default").Map()
	if scope["profile_ref"] != "profile:personal/default" {
		t.Fatalf("profile_ref missing from scope: %#v", scope)
	}
	if scope["binding_scope"] != "project" || scope["type"] != "project" {
		t.Fatalf("expected project scope defaults: %#v", scope)
	}
}
