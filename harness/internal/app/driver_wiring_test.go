package app

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/mnemon-dev/mnemon/harness/internal/channel"
	"github.com/mnemon-dev/mnemon/harness/internal/contract"
	"github.com/mnemon-dev/mnemon/harness/internal/driver"
	"github.com/mnemon-dev/mnemon/harness/internal/store"
)

func setupHost(t *testing.T, root, host string) {
	t.Helper()
	var out, errw bytes.Buffer
	if _, err := New(root).Setup(context.Background(), &out, &errw, SetupOptions{
		Host:        host,
		Loops:       []string{"memory"},
		Principal:   "codex@project",
		ControlURL:  "http://127.0.0.1:8787",
		ProjectRoot: root,
	}); err != nil {
		t.Fatalf("setup %s: %v\n%s", host, err, errw.String())
	}
}

// setup records the per-host projected loops in localConfig — the background driver's
// re-projection authority — merging across reruns and across hosts.
func TestSetupRecordsHostsInLocalConfig(t *testing.T) {
	root := t.TempDir()
	setupHost(t, root, "codex")
	setupHost(t, root, "claude-code")

	raw, err := os.ReadFile(filepath.Join(root, ".mnemon", "harness", "local", "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	var cfg struct {
		Hosts map[string][]string `json:"hosts"`
	}
	if err := json.Unmarshal(raw, &cfg); err != nil {
		t.Fatal(err)
	}
	want := map[string][]string{"codex": {"memory"}, "claude-code": {"memory"}}
	if !reflect.DeepEqual(cfg.Hosts, want) {
		t.Fatalf("hosts = %v, want %v", cfg.Hosts, want)
	}
}

// Plan 3.6 acceptance shape: boot over a real setup, admit a write, then ONE driver tick
// out-of-band — it drains the invalidation, re-projects the host surface under no-clobber
// (a user edit is preserved), prunes the acked rows, and no second store opener exists.
func TestDriverTickDrainsReprojectsAndPrunes(t *testing.T) {
	root := t.TempDir()
	setupHost(t, root, "codex")

	loaded, err := channel.LoadBindingFile(root, filepath.Join(root, ".mnemon", "harness", "channel", "bindings.json"))
	if err != nil {
		t.Fatal(err)
	}
	storePath := filepath.Join(root, ".mnemon", "harness", "local", "governed.db")
	rt, err := OpenLocalRuntime(storePath, loaded, []string{"memory"})
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close()

	// single-writer: while the runtime holds the store, a second opener must be refused.
	if _, err := store.OpenStore(storePath); err == nil {
		t.Fatal("a second store opener must be refused while the runtime serves")
	}

	if _, _, err := rt.API().Ingest("codex@project", contract.ObservationEnvelope{
		ExternalID: "m1",
		Event: contract.Event{Type: "memory.write_candidate.observed",
			Payload: map[string]any{"content": "driver fact", "source": "s", "confidence": "high"}},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := rt.Tick(); err != nil {
		t.Fatal(err)
	}

	// hand-edit a managed definition file; the driver's re-projection must preserve it.
	guide := filepath.Join(root, ".codex", "mnemon-memory", "GUIDE.md")
	prior, err := os.ReadFile(guide)
	if err != nil {
		t.Fatal(err)
	}
	edited := "# USER EDIT\n" + string(prior)
	if err := os.WriteFile(guide, []byte(edited), 0o644); err != nil {
		t.Fatal(err)
	}

	d := driver.New(rt, reprojectForHosts(map[string][]string{"codex": {"memory"}}, root), 0)
	if err := d.Tick(context.Background()); err != nil {
		t.Fatalf("driver tick: %v", err)
	}

	after, err := os.ReadFile(guide)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(after), "# USER EDIT") {
		t.Fatal("driver re-projection clobbered a user-edited managed file")
	}
	if n, err := rt.DrainOutbox(); err != nil || n != 0 {
		t.Fatalf("driver tick must have drained the invalidation; re-drain found %d (err %v)", n, err)
	}
}
