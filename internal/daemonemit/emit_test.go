package daemonemit

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestEmitAppendsHarnessEvent(t *testing.T) {
	root := t.TempDir()
	event, path, err := Emit(Options{
		Root:          root,
		Topic:         "memory.hot_write_observed",
		Payload:       map[string]any{"insight_id": "ins-1"},
		CorrelationID: "memory:ins-1",
		Loop:          "memory",
		Host:          "mnemon",
		Now:           time.Date(2026, 5, 28, 12, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("Emit returned error: %v", err)
	}
	if path != filepath.Join(root, ".mnemon", "events.jsonl") {
		t.Fatalf("unexpected event path: %s", path)
	}
	if event.Type != "memory.hot_write_observed" {
		t.Fatalf("unexpected event: %#v", event)
	}
	file, err := os.Open(path)
	if err != nil {
		t.Fatalf("open eventlog: %v", err)
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	if !scanner.Scan() {
		t.Fatalf("expected eventlog line")
	}
	var decoded Event
	if err := json.Unmarshal(scanner.Bytes(), &decoded); err != nil {
		t.Fatalf("decode event line: %v", err)
	}
	if decoded.CorrelationID != "memory:ins-1" || decoded.Payload["insight_id"] != "ins-1" {
		t.Fatalf("unexpected decoded event: %#v", decoded)
	}
}

func TestPayloadFromJSON(t *testing.T) {
	payload, err := PayloadFromJSON(`{"k":"v"}`)
	if err != nil {
		t.Fatalf("PayloadFromJSON returned error: %v", err)
	}
	if payload["k"] != "v" {
		t.Fatalf("unexpected payload: %#v", payload)
	}
}
