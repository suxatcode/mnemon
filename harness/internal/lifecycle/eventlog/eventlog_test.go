package eventlog

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/mnemon-dev/mnemon/harness/internal/lifecycle/layout"
	"github.com/mnemon-dev/mnemon/harness/internal/lifecycle/schema"
)

// TestConcurrentTwoHostWritebackKeepsLedgerConsistent is the Band 1 substrate
// proof: two host identities, each driving two concurrent writers with their own
// Store handle (as separate processes would), append host-tagged events to one
// ledger. The append lock (O_EXCL + same-pid-alive detection) and the
// rebuildable index must yield a ledger with no lost, duplicated, or inconsistent
// events — every event present exactly once, each carrying its writer's host.
func TestConcurrentTwoHostWritebackKeepsLedgerConsistent(t *testing.T) {
	root := t.TempDir()
	if _, err := layout.EnsureProject(root); err != nil {
		t.Fatalf("EnsureProject returned error: %v", err)
	}

	hosts := []string{"codex", "claude-code"}
	const writersPerHost = 2
	const eventsPerWriter = 30
	want := len(hosts) * writersPerHost * eventsPerWriter

	var wg sync.WaitGroup
	errCh := make(chan error, len(hosts)*writersPerHost)
	for _, host := range hosts {
		for w := 0; w < writersPerHost; w++ {
			wg.Add(1)
			go func(host string, w int) {
				defer wg.Done()
				store, err := New(root) // each writer its own handle, like a separate process
				if err != nil {
					errCh <- err
					return
				}
				for i := 0; i < eventsPerWriter; i++ {
					id := fmt.Sprintf("evt_%s_w%d_%03d", host, w, i)
					if err := store.Append(fixtureEvent(id, "memory.hot_write_observed", "memory", host)); err != nil {
						errCh <- fmt.Errorf("append %s: %w", id, err)
						return
					}
				}
			}(host, w)
		}
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		if err != nil {
			t.Fatalf("concurrent writeback failed: %v", err)
		}
	}

	store, err := New(root)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	events, err := store.ReadAll()
	if err != nil {
		t.Fatalf("ReadAll returned error: %v", err)
	}

	// No lost or extra events.
	if len(events) != want {
		t.Fatalf("ledger has %d events, want %d (lost or duplicated under concurrent writeback)", len(events), want)
	}
	// No duplicates; host identity carried end to end; per-host counts intact.
	seen := map[string]bool{}
	hostCount := map[string]int{}
	for _, ev := range events {
		if seen[ev.ID] {
			t.Fatalf("duplicate event id %q", ev.ID)
		}
		seen[ev.ID] = true
		if ev.Host == nil {
			t.Fatalf("event %q lost its host identity", ev.ID)
		}
		hostCount[*ev.Host]++
	}
	for _, host := range hosts {
		if got := hostCount[host]; got != writersPerHost*eventsPerWriter {
			t.Fatalf("host %q: %d events, want %d", host, got, writersPerHost*eventsPerWriter)
		}
	}
	// The rebuildable index stays consistent with the canonical log.
	if records := readIndexRecords(t, root); len(records) != want {
		t.Fatalf("index drift: %d records for %d ledger events", len(records), want)
	}
}

func TestAppendReadAndRejectDuplicateEvent(t *testing.T) {
	root := t.TempDir()
	store, err := New(root)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	event := fixtureEvent("evt_memory_001", "memory.hot_write_observed", "memory", "codex")

	if err := store.Append(event); err != nil {
		t.Fatalf("Append returned error: %v", err)
	}
	if err := store.Append(event); err == nil {
		t.Fatal("expected duplicate event id error")
	} else if !IsDuplicateEventID(err) {
		t.Fatalf("expected typed duplicate event id error, got %v", err)
	}

	events, err := store.ReadAll()
	if err != nil {
		t.Fatalf("ReadAll returned error: %v", err)
	}
	if len(events) != 1 || events[0].ID != event.ID {
		t.Fatalf("unexpected events: %#v", events)
	}
	records := readIndexRecords(t, root)
	if len(records) != 1 || records[0].ID != event.ID || records[0].Offset != 0 || records[0].NextOffset <= records[0].Offset {
		t.Fatalf("unexpected index records: %#v", records)
	}
}

func TestAppendJSONRejectsInvalidCandidate(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	_, err = store.AppendJSON([]byte(`{"schema_version":1}`))
	if err == nil {
		t.Fatal("expected validation error")
	}
}

func TestReadAllReturnsPartialEventsOnCorruptLine(t *testing.T) {
	root := t.TempDir()
	paths, err := layout.EnsureProject(root)
	if err != nil {
		t.Fatalf("EnsureProject returned error: %v", err)
	}
	first := fixtureEvent("evt_memory_001", "memory.hot_write_observed", "memory", "codex")
	data, err := json.Marshal(first)
	if err != nil {
		t.Fatalf("marshal event: %v", err)
	}
	if err := os.WriteFile(paths.EventLog, append(append(data, '\n'), []byte("{bad json}\n")...), 0o644); err != nil {
		t.Fatalf("write event log: %v", err)
	}

	store, err := New(root)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	events, err := store.ReadAll()
	if err == nil {
		t.Fatal("expected corrupt log error")
	}
	var corrupt *CorruptLogError
	if !errors.As(err, &corrupt) || corrupt.Line != 2 {
		t.Fatalf("expected corrupt line 2, got %v", err)
	}
	if len(events) != 1 || events[0].ID != first.ID {
		t.Fatalf("expected partial event before corrupt line, got %#v", events)
	}
}

// TestReadAllSkipsInProgressTrailingLine proves the multi-writer read hardening:
// a final line with no terminating newline (a writer mid-append) is skipped, and
// the durable newline-terminated prefix is returned without error.
func TestReadAllSkipsInProgressTrailingLine(t *testing.T) {
	root := t.TempDir()
	paths, err := layout.EnsureProject(root)
	if err != nil {
		t.Fatalf("EnsureProject returned error: %v", err)
	}
	done := fixtureEvent("evt_done", "memory.hot_write_observed", "memory", "codex")
	data, err := json.Marshal(done)
	if err != nil {
		t.Fatalf("marshal event: %v", err)
	}
	// One complete line, then a newline-LESS partial (an append in progress).
	content := append(append(data, '\n'), []byte(`{"id":"evt_partial`)...)
	if err := os.WriteFile(paths.EventLog, content, 0o644); err != nil {
		t.Fatalf("write event log: %v", err)
	}
	store, err := New(root)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	events, err := store.ReadAll()
	if err != nil {
		t.Fatalf("ReadAll should skip an in-progress trailing line, got error: %v", err)
	}
	if len(events) != 1 || events[0].ID != "evt_done" {
		t.Fatalf("expected only the durable event, got %#v", events)
	}
}

// TestReadAllToleratesConcurrentAppend hammers reads while a writer appends to the
// same ledger: every read must succeed (no partial-line error) and return only
// fully-decoded events, and the final read must see the whole ledger.
func TestReadAllToleratesConcurrentAppend(t *testing.T) {
	root := t.TempDir()
	if _, err := layout.EnsureProject(root); err != nil {
		t.Fatalf("EnsureProject returned error: %v", err)
	}
	writer, err := New(root)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	if err := writer.Append(fixtureEvent("evt_seed", "memory.hot_write_observed", "memory", "codex")); err != nil {
		t.Fatalf("seed append: %v", err)
	}

	const total = 80
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < total; i++ {
			_ = writer.Append(fixtureEvent(fmt.Sprintf("evt_%03d", i), "memory.hot_write_observed", "memory", "claude-code"))
		}
	}()

	reader, err := New(root)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	for {
		events, err := reader.ReadAll()
		if err != nil {
			t.Fatalf("concurrent ReadAll errored (partial read not tolerated): %v", err)
		}
		for _, ev := range events {
			if ev.ID == "" || ev.Host == nil {
				t.Fatalf("concurrent ReadAll returned an inconsistent event: %#v", ev)
			}
		}
		select {
		case <-done:
			final, err := reader.ReadAll()
			if err != nil {
				t.Fatalf("final ReadAll: %v", err)
			}
			if len(final) != total+1 {
				t.Fatalf("final ledger has %d events, want %d", len(final), total+1)
			}
			return
		default:
		}
	}
}

func TestAppendCreatesLayout(t *testing.T) {
	root := t.TempDir()
	store, err := New(root)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	if err := store.Append(fixtureEvent("evt_eval_001", "eval.run_observed", "eval", "codex")); err != nil {
		t.Fatalf("Append returned error: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, ".mnemon", "events.jsonl")); err != nil {
		t.Fatalf("expected events.jsonl: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, ".mnemon", "events.index")); err != nil {
		t.Fatalf("expected events.index: %v", err)
	}
}

func TestAppendRebuildsMissingOrCorruptIndex(t *testing.T) {
	root := t.TempDir()
	store, err := New(root)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	first := fixtureEvent("evt_eval_001", "eval.run_observed", "eval", "codex")
	second := fixtureEvent("evt_eval_002", "eval.run_observed", "eval", "codex")
	if err := store.Append(first); err != nil {
		t.Fatalf("Append first returned error: %v", err)
	}
	indexPath := filepath.Join(root, ".mnemon", "events.index")
	if err := os.WriteFile(indexPath, []byte("{bad json}\n"), 0o644); err != nil {
		t.Fatalf("corrupt index: %v", err)
	}
	if err := store.Append(first); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("expected duplicate error after index rebuild, got %v", err)
	}
	records := readIndexRecords(t, root)
	if len(records) != 1 || records[0].ID != first.ID {
		t.Fatalf("expected rebuilt first index record, got %#v", records)
	}
	if err := os.Remove(indexPath); err != nil {
		t.Fatalf("remove index: %v", err)
	}
	if err := store.Append(second); err != nil {
		t.Fatalf("Append second returned error: %v", err)
	}
	records = readIndexRecords(t, root)
	if len(records) != 2 || records[0].ID != first.ID || records[1].ID != second.ID {
		t.Fatalf("expected rebuilt index with both records, got %#v", records)
	}
}

func fixtureEvent(id, typ, loop, host string) schema.Event {
	return schema.Event{
		SchemaVersion: 1,
		ID:            id,
		TS:            "2026-05-24T08:30:00Z",
		Type:          typ,
		Loop:          &loop,
		Host:          &host,
		Actor:         "host-agent",
		Source:        "fixture",
		CorrelationID: "corr_fixture",
		CausedBy:      nil,
		Payload:       map[string]any{"reason": "fixture"},
	}
}

func readIndexRecords(t *testing.T, root string) []indexRecord {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, ".mnemon", "events.index"))
	if err != nil {
		t.Fatalf("read events.index: %v", err)
	}
	var records []indexRecord
	for lineNo, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var record indexRecord
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			t.Fatalf("decode index line %d: %v", lineNo+1, err)
		}
		records = append(records, record)
	}
	return records
}
