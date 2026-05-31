package codex

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/mnemon-dev/mnemon/harness/internal/lifecycle/eventlog"
	"github.com/mnemon-dev/mnemon/harness/internal/lifecycle/schema"
)

func TestCheckReportsCommandMissing(t *testing.T) {
	result, err := Check(context.Background(), t.TempDir(), CheckOptions{
		Command: "definitely-not-a-codex-command",
		Now:     fixtureNow(),
		RunID:   "missing-command",
	})
	if err != nil {
		t.Fatalf("Check returned error: %v", err)
	}
	if result.Status != StatusBlocked || result.FailureClass != FailureCommandMissing {
		t.Fatalf("unexpected result: %#v", result)
	}
	assertFileExists(t, result.ReportPath)
	assertFileExists(t, result.StatusPath)
}

func TestCheckReportsProtocolUnavailable(t *testing.T) {
	result, err := Check(context.Background(), t.TempDir(), CheckOptions{
		Command: os.Args[0],
		Args:    []string{"-test.run=TestFakeCodexAppServer", "--"},
		Env:     []string{"MNEMON_FAKE_CODEX_APPSERVER=bad-json"},
		Now:     fixtureNow(),
		RunID:   "bad-protocol",
	})
	if err != nil {
		t.Fatalf("Check returned error: %v", err)
	}
	if result.Status != StatusDegraded || result.FailureClass != FailureProtocolUnavailable {
		t.Fatalf("unexpected result: %#v", result)
	}
}

func TestCheckReportsAuthQuotaUnavailable(t *testing.T) {
	result, err := Check(context.Background(), t.TempDir(), CheckOptions{
		Command: os.Args[0],
		Args:    []string{"-test.run=TestFakeCodexAppServer", "--"},
		Env:     []string{"MNEMON_FAKE_CODEX_APPSERVER=auth-error"},
		Now:     fixtureNow(),
		RunID:   "auth-error",
	})
	if err != nil {
		t.Fatalf("Check returned error: %v", err)
	}
	if result.Status != StatusBlocked || result.FailureClass != FailureAuthQuotaUnavailable {
		t.Fatalf("unexpected result: %#v", result)
	}
}

func TestCheckReadyWritesReportAndRunnerStatus(t *testing.T) {
	root := t.TempDir()
	result, err := Check(context.Background(), root, CheckOptions{
		Command: os.Args[0],
		Args:    []string{"-test.run=TestFakeCodexAppServer", "--"},
		Env:     []string{"MNEMON_FAKE_CODEX_APPSERVER=ready"},
		Now:     fixtureNow(),
		RunID:   "ready",
	})
	if err != nil {
		t.Fatalf("Check returned error: %v", err)
	}
	if result.Status != StatusReady || result.FailureClass != FailureNone {
		t.Fatalf("unexpected result: %#v", result)
	}
	assertFileExists(t, result.ReportPath)
	assertFileExists(t, result.StatusPath)
	assertFileExists(t, filepath.Join(result.RunDir, "workspace", ".mnemon"))
	assertFileExists(t, filepath.Join(result.RunDir, "workspace", ".codex"))

	events := readReadinessEvents(t, root)
	if len(events) != 1 || events[0].Type != "runner.readiness_passed" {
		t.Fatalf("unexpected readiness events: %#v", events)
	}
	if _, err := Check(context.Background(), root, CheckOptions{
		Command: os.Args[0],
		Args:    []string{"-test.run=TestFakeCodexAppServer", "--"},
		Env:     []string{"MNEMON_FAKE_CODEX_APPSERVER=ready"},
		Now:     fixtureNow().Add(time.Minute),
		RunID:   "ready-again",
	}); err != nil {
		t.Fatalf("second Check returned error: %v", err)
	}
	events = readReadinessEvents(t, root)
	if len(events) != 1 {
		t.Fatalf("ready phase should not append duplicate runner event, got %#v", events)
	}
}

func TestFakeCodexAppServer(t *testing.T) {
	mode := os.Getenv("MNEMON_FAKE_CODEX_APPSERVER")
	if mode == "" {
		return
	}
	switch mode {
	case "bad-json":
		fmt.Println("not json")
		return
	case "protocol-spam":
		for i := 0; i < 128; i++ {
			fmt.Println("{")
		}
		return
	}

	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		var msg map[string]any
		if err := json.Unmarshal(scanner.Bytes(), &msg); err != nil {
			fmt.Fprintf(os.Stdout, `{"id":1,"error":{"message":"bad request"}}`+"\n")
			continue
		}
		id, _ := msg["id"].(float64)
		method, _ := msg["method"].(string)
		if id == 0 {
			continue
		}
		switch method {
		case "initialize":
			fmt.Fprintf(os.Stdout, `{"id":%d,"result":{"userAgent":"fake-codex","codexHome":"/tmp/fake"}}`+"\n", int(id))
		case "skills/list":
			fmt.Fprintf(os.Stdout, `{"id":%d,"result":{"skills":[]}}`+"\n", int(id))
		case "model/list":
			if mode == "auth-error" {
				fmt.Fprintf(os.Stdout, `{"id":%d,"error":{"message":"auth login required or quota unavailable"}}`+"\n", int(id))
			} else {
				fmt.Fprintf(os.Stdout, `{"id":%d,"result":{"models":[]}}`+"\n", int(id))
			}
		case "thread/start":
			fmt.Fprintf(os.Stdout, `{"id":%d,"result":{"thread":{"id":"thread_fake"}}}`+"\n", int(id))
		case "turn/start":
			fmt.Fprintf(os.Stdout, `{"id":%d,"result":{"turn":{"id":"turn_fake"}}}`+"\n", int(id))
			if mode == "turn-failed" {
				fmt.Fprintln(os.Stdout, `{"method":"turn/completed","params":{"threadId":"thread_fake","turn":{"id":"turn_fake","status":"failed","error":{"message":"unexpected status 401 Unauthorized: Missing bearer authentication"}}}}`)
			} else {
				fmt.Fprintln(os.Stdout, `{"method":"turn/completed","params":{"threadId":"thread_fake","turnId":"turn_fake","status":"completed"}}`)
			}
		default:
			fmt.Fprintf(os.Stdout, `{"id":%d,"result":{}}`+"\n", int(id))
		}
		_ = os.Stdout.Sync()
	}
	os.Exit(0)
}

func fixtureNow() time.Time {
	return time.Date(2026, 5, 24, 9, 30, 0, 0, time.UTC)
}

func readReadinessEvents(t *testing.T, root string) []schema.Event {
	t.Helper()
	store, err := eventlog.New(root)
	if err != nil {
		t.Fatalf("eventlog.New returned error: %v", err)
	}
	events, err := store.ReadAll()
	if err != nil {
		t.Fatalf("ReadAll returned error: %v", err)
	}
	var readiness []schema.Event
	for _, event := range events {
		switch event.Type {
		case "runner.readiness_passed", "runner.readiness_blocked", "runner.readiness_degraded":
			readiness = append(readiness, event)
		}
	}
	return readiness
}

func assertFileExists(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected %s to exist: %v", path, err)
	}
}
