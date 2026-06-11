package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
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

// setup 重跑不得覆盖用户手选的 mirror_mode(setup 无该 flag,覆盖即静默推翻用户决策);
// 全新安装写出显式缺省 prime-refresh。
func TestSetupPreservesMirrorModeAcrossReruns(t *testing.T) {
	root := t.TempDir()
	setupHost(t, root, "codex")
	cfgPath := filepath.Join(root, ".mnemon", "harness", "local", "config.json")
	raw, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"mirror_mode": "prime-refresh"`) {
		t.Fatalf("fresh setup must write the explicit default; got:\n%s", raw)
	}
	edited := strings.Replace(string(raw), `"mirror_mode": "prime-refresh"`, `"mirror_mode": "manual"`, 1)
	if err := os.WriteFile(cfgPath, []byte(edited), 0o644); err != nil {
		t.Fatal(err)
	}
	setupHost(t, root, "codex") // rerun
	raw, err = os.ReadFile(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"mirror_mode": "manual"`) {
		t.Fatalf("setup rerun must preserve the user-chosen manual mode; got:\n%s", raw)
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
	rt, err := OpenLocalRuntime(storePath, loaded, []string{"memory"}, nil)
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

	d := driver.New(rt, serveReproject(rt, loaded, map[string][]string{"codex": {"memory"}}, root, "prime-refresh"), 0)
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
	if _, drained, err := rt.DrainOutbox(); err != nil || drained != 0 {
		t.Fatalf("driver tick must have drained the invalidation; re-drain found %d (err %v)", drained, err)
	}
}

// 阶段一核心验收:accepted write → driver tick → MEMORY.md 镜像已含新内容,全程不跑 prime;
// user-edited 定义文件在多个"真实再生"周期下持续不被触碰(I10 时间窗:每轮注入新候选,
// 保证 ≥3 次重投影真的发生)。
func TestDriverTickRegeneratesMemoryMirror(t *testing.T) {
	root := t.TempDir()
	setupHost(t, root, "codex")
	loaded, err := channel.LoadBindingFile(root, filepath.Join(root, ".mnemon", "harness", "channel", "bindings.json"))
	if err != nil {
		t.Fatal(err)
	}
	rt, err := OpenLocalRuntime(filepath.Join(root, ".mnemon", "harness", "local", "governed.db"), loaded, []string{"memory"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close()

	guide := filepath.Join(root, ".codex", "mnemon-memory", "GUIDE.md")
	if err := os.WriteFile(guide, []byte("# USER EDIT\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	d := driver.New(rt, serveReproject(rt, loaded, map[string][]string{"codex": {"memory"}}, root, "prime-refresh"), 0)
	for i := 1; i <= 3; i++ { // 每轮一个新 accepted write → 每轮一次真实重投影
		if _, _, err := rt.API().Ingest("codex@project", contract.ObservationEnvelope{
			ExternalID: fmt.Sprintf("m%d", i),
			Event: contract.Event{Type: "memory.write_candidate.observed",
				Payload: map[string]any{"content": fmt.Sprintf("driver mirror fact %d", i), "source": "s", "confidence": "high"}},
		}); err != nil {
			t.Fatal(err)
		}
		if _, err := rt.Tick(); err != nil {
			t.Fatal(err)
		}
		if err := d.Tick(context.Background()); err != nil {
			t.Fatalf("driver tick %d: %v", i, err)
		}
	}

	mirror, err := os.ReadFile(filepath.Join(root, ".codex", "mnemon-memory", "MEMORY.md"))
	if err != nil {
		t.Fatal(err)
	}
	for i := 1; i <= 3; i++ {
		if !strings.Contains(string(mirror), fmt.Sprintf("driver mirror fact %d", i)) {
			t.Fatalf("driver must regenerate the mirror with governed content (fact %d missing):\n%s", i, mirror)
		}
	}
	if after, _ := os.ReadFile(guide); !strings.HasPrefix(string(after), "# USER EDIT") {
		t.Fatal("guarded definition file touched across real re-projection cycles")
	}
}

// manual 模式:driver 排空照常,但镜像保持种子态(仅 prime 再生)。
func TestDriverManualModeSkipsMirror(t *testing.T) {
	root := t.TempDir()
	setupHost(t, root, "codex")
	loaded, err := channel.LoadBindingFile(root, filepath.Join(root, ".mnemon", "harness", "channel", "bindings.json"))
	if err != nil {
		t.Fatal(err)
	}
	rt, err := OpenLocalRuntime(filepath.Join(root, ".mnemon", "harness", "local", "governed.db"), loaded, []string{"memory"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close()
	if _, _, err := rt.API().Ingest("codex@project", contract.ObservationEnvelope{
		ExternalID: "m1",
		Event: contract.Event{Type: "memory.write_candidate.observed",
			Payload: map[string]any{"content": "must not appear", "source": "s", "confidence": "high"}},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := rt.Tick(); err != nil {
		t.Fatal(err)
	}
	d := driver.New(rt, serveReproject(rt, loaded, map[string][]string{"codex": {"memory"}}, root, "manual"), 0)
	if err := d.Tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	mirror, err := os.ReadFile(filepath.Join(root, ".codex", "mnemon-memory", "MEMORY.md"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(mirror), "must not appear") {
		t.Fatal("manual mode must not regenerate the mirror from the driver")
	}
}

// reproject 错误绝不杀死 driver:包装器记日志吞错,排空与修剪长存。
func TestSwallowReprojectErrorsKeepsDriverAlive(t *testing.T) {
	var log bytes.Buffer
	wrapped := swallowReprojectErrors(func([]contract.ResourceRef) error {
		return fmt.Errorf("transient mirror failure")
	}, &log)
	if err := wrapped(nil); err != nil {
		t.Fatalf("wrapper must swallow reproject errors, got %v", err)
	}
	if !strings.Contains(log.String(), "transient mirror failure") {
		t.Fatalf("the swallowed error must be logged, got %q", log.String())
	}
}

// T1 权限地板:setup 后私密目录 0700、token 0600;预先以 0755 存在的目录在重跑时被校正
// (local run 先于 setup 的窗口);同用户读写不受影响(本测试自身即同用户)。
func TestSetupTightensPrivateDirPermissions(t *testing.T) {
	root := t.TempDir()
	// 模拟 local run 先行:channel 目录先以宽权限存在
	pre := filepath.Join(root, ".mnemon", "harness", "channel")
	if err := os.MkdirAll(pre, 0o755); err != nil {
		t.Fatal(err)
	}
	setupHost(t, root, "codex")
	for _, rel := range []string{
		".mnemon/harness", ".mnemon/harness/local", ".mnemon/harness/channel",
		".mnemon/harness/channel/credentials",
	} {
		st, err := os.Stat(filepath.Join(root, filepath.FromSlash(rel)))
		if err != nil {
			t.Fatalf("%s: %v", rel, err)
		}
		if st.Mode().Perm() != 0o700 {
			t.Fatalf("%s: mode %o, want 0700", rel, st.Mode().Perm())
		}
	}
	tok := filepath.Join(root, ".mnemon", "harness", "channel", "credentials", "codex-project.token")
	if st, err := os.Stat(tok); err != nil || st.Mode().Perm() != 0o600 {
		t.Fatalf("token mode: %v %o, want 0600", err, st.Mode().Perm())
	}
}
