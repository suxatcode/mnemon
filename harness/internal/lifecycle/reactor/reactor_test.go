package reactor

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/mnemon-dev/mnemon/harness/internal/lifecycle/eventlog"
	"github.com/mnemon-dev/mnemon/harness/internal/lifecycle/schema"
)

func TestDefaultRegistryListsAndRunsStatusRefresh(t *testing.T) {
	root := t.TempDir()
	store, err := eventlog.New(root)
	if err != nil {
		t.Fatalf("eventlog.New returned error: %v", err)
	}
	loop := "memory"
	host := "codex"
	if err := store.Append(schema.Event{
		SchemaVersion: schema.Version,
		ID:            "evt_reactor_001",
		TS:            "2026-05-24T08:30:00Z",
		Type:          "memory.hot_write_observed",
		Loop:          &loop,
		Host:          &host,
		Actor:         "host-agent",
		Source:        "fixture",
		CorrelationID: "corr_fixture",
		Payload:       map[string]any{"reason": "fixture"},
	}); err != nil {
		t.Fatalf("append event: %v", err)
	}

	registry := DefaultRegistry()
	if reactor, ok := registry.Get(StatusRefreshID); !ok || reactor.Type() != "deterministic" {
		t.Fatalf("expected registered deterministic %s reactor", StatusRefreshID)
	}
	result, err := registry.Run(context.Background(), StatusRefreshID, Context{
		Root: root,
		Now:  time.Date(2026, 5, 24, 9, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if result.ReactorID != StatusRefreshID || result.Outcome != "completed" {
		t.Fatalf("unexpected result: %#v", result)
	}
}

func TestRegistryRunUnknownReactor(t *testing.T) {
	_, err := DefaultRegistry().Run(context.Background(), "missing.reactor", Context{})
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}
