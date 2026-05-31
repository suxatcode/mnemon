package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mnemon-dev/mnemon/internal/model"
	"github.com/spf13/cobra"
)

func TestEventEmitCommand(t *testing.T) {
	root := t.TempDir()
	restoreEventFlags(t)
	eventRoot = root
	eventPayload = `{"k":"v"}`
	eventCorrelationID = "corr-test"
	eventLoop = "memory"
	eventHost = "mnemon"
	cmd, output := eventTestCommand()
	if err := eventEmitCmd.RunE(cmd, []string{"memory.hot_write_observed"}); err != nil {
		t.Fatalf("event emit returned error: %v", err)
	}
	if !strings.Contains(output.String(), "emitted") {
		t.Fatalf("unexpected output: %s", output.String())
	}
	data, err := os.ReadFile(filepath.Join(root, ".mnemon", "events.jsonl"))
	if err != nil {
		t.Fatalf("read eventlog: %v", err)
	}
	if !strings.Contains(string(data), `"correlation_id":"corr-test"`) {
		t.Fatalf("eventlog missing correlation: %s", string(data))
	}
	if !strings.Contains(string(data), `"loop":"memory"`) || !strings.Contains(string(data), `"host":"mnemon"`) {
		t.Fatalf("eventlog missing loop/host metadata: %s", string(data))
	}
}

func TestRememberEventEmitIsFeatureFlagged(t *testing.T) {
	root := t.TempDir()
	t.Setenv("MNEMON_HARNESS_EVENTLOG", filepath.Join(root, "events.jsonl"))
	t.Setenv("MNEMON_HARNESS_EVENT_EMIT", "1")
	restoreRootFlags(t)
	storeName = "test_store"
	emitRememberEvent(&model.Insight{
		ID:         "ins-1",
		Category:   model.CategoryInsight,
		Importance: 4,
	}, "added")
	data, err := os.ReadFile(filepath.Join(root, "events.jsonl"))
	if err != nil {
		t.Fatalf("read eventlog: %v", err)
	}
	if !strings.Contains(string(data), `"type":"memory.hot_write_observed"`) || !strings.Contains(string(data), `"store":"test_store"`) {
		t.Fatalf("unexpected remember event: %s", string(data))
	}
}

func eventTestCommand() (*cobra.Command, *bytes.Buffer) {
	output := &bytes.Buffer{}
	cmd := &cobra.Command{}
	cmd.SetOut(output)
	cmd.SetErr(output)
	return cmd, output
}

func restoreEventFlags(t *testing.T) {
	t.Helper()
	oldRoot := eventRoot
	oldPayload := eventPayload
	oldCorrelationID := eventCorrelationID
	oldLoop := eventLoop
	oldHost := eventHost
	t.Cleanup(func() {
		eventRoot = oldRoot
		eventPayload = oldPayload
		eventCorrelationID = oldCorrelationID
		eventLoop = oldLoop
		eventHost = oldHost
	})
	eventRoot = "."
	eventPayload = "{}"
	eventCorrelationID = ""
	eventLoop = ""
	eventHost = ""
}

func restoreRootFlags(t *testing.T) {
	t.Helper()
	oldStoreName := storeName
	oldDataDir := dataDir
	t.Cleanup(func() {
		storeName = oldStoreName
		dataDir = oldDataDir
	})
	storeName = ""
	dataDir = t.TempDir()
}
