package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mnemon-dev/mnemon/harness/internal/app"
	"github.com/mnemon-dev/mnemon/harness/internal/capability"
	"github.com/mnemon-dev/mnemon/harness/internal/runtime"
)

func TestLocalStatusReportsProductBoundary(t *testing.T) {
	root := t.TempDir()
	restoreLocalFlags(t)
	localRoot = root

	cmd, output := testCommand()
	if err := runLocalStatus(cmd, nil); err != nil {
		t.Fatalf("runLocalStatus returned error: %v", err)
	}
	got := output.String()
	for _, want := range []string{
		"Local Mnemon: ready",
		"Remote Workspace: disconnected",
		"Mode: local",
		filepath.Join(root, runtime.DefaultStorePath),
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("local status missing %q:\n%s", want, got)
		}
	}
	for _, blocked := range []string{"channel", "runtime", "kernel", "outbox", "cursor"} {
		if strings.Contains(strings.ToLower(got), blocked) {
			t.Fatalf("local status leaked %q:\n%s", blocked, got)
		}
	}
}

func TestLocalBootAutoDiscoversSetupConfig(t *testing.T) {
	projectRoot := t.TempDir()
	setupProductIntegration(t, projectRoot)
	restoreLocalFlags(t)
	localRoot = projectRoot

	boot, err := resolveLocalBoot()
	if err != nil {
		t.Fatalf("resolve local boot from setup config: %v", err)
	}
	if !boot.Configured {
		t.Fatal("local boot must use setup config when --bindings is omitted")
	}
	if boot.StorePath != filepath.Join(projectRoot, runtime.DefaultStorePath) {
		t.Fatalf("store path = %q, want project default", boot.StorePath)
	}
	if len(boot.Loaded.Tokens) == 0 {
		t.Fatal("local boot must load setup token credentials")
	}
	cfg, err := app.LocalRuntimeConfigFromBindings(boot.Loaded.Bindings)
	if err != nil {
		t.Fatalf("boot config: %v", err)
	}
	var handlesMemory, handlesSkill bool
	for _, r := range cfg.Rules.Rules() {
		handlesMemory = handlesMemory || r.Handles(capability.MemoryWriteCandidateObserved)
		handlesSkill = handlesSkill || r.Handles(capability.SkillWriteCandidateObserved)
	}
	if !handlesMemory || !handlesSkill {
		t.Fatalf("local boot must enable memory and skill rules; memory=%v skill=%v", handlesMemory, handlesSkill)
	}
}

func TestLocalBootMissingSetupShowsProductRemediation(t *testing.T) {
	restoreLocalFlags(t)
	localRoot = t.TempDir()
	_, err := resolveLocalBoot()
	if err == nil {
		t.Fatal("local boot without setup must fail")
	}
	for _, want := range []string{
		"Local Mnemon is not set up.",
		"mnemon-harness setup --host codex --memory --skills",
	} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("missing remediation %q in error:\n%v", want, err)
		}
	}
	for _, blocked := range []string{"binding", "channel", "runtime", "kernel", "token file"} {
		if strings.Contains(strings.ToLower(err.Error()), blocked) {
			t.Fatalf("local boot remediation leaked %q:\n%v", blocked, err)
		}
	}
}

func restoreLocalFlags(t *testing.T) {
	t.Helper()
	oldRoot := localRoot
	oldAddr := localAddr
	oldStore := localStorePath
	oldBindings := localBindingsPath
	t.Cleanup(func() {
		localRoot = oldRoot
		localAddr = oldAddr
		localStorePath = oldStore
		localBindingsPath = oldBindings
	})
	localRoot = "."
	localAddr = "127.0.0.1:8787"
	localStorePath = ""
	localBindingsPath = ""
}

// setup 写入的 endpoint 必须驱动 local run 的监听地址(显式 --addr 优先;
// endpoint 缺失/不可解析时回落默认)——否则非默认端口下 hooks/bindings
// 指向的地址无人监听,破坏"一次 setup + local run"承诺。
func TestListenAddrFromEndpoint(t *testing.T) {
	cases := []struct {
		name, endpoint, fallback, want string
	}{
		{"derives host:port", "http://127.0.0.1:9001", "127.0.0.1:8787", "127.0.0.1:9001"},
		{"empty endpoint falls back", "", "127.0.0.1:8787", "127.0.0.1:8787"},
		{"unparsable falls back", "::not-a-url::", "127.0.0.1:8787", "127.0.0.1:8787"},
		{"schemeless host:port falls back (no host parsed)", "127.0.0.1:9001", "127.0.0.1:8787", "127.0.0.1:8787"},
	}
	for _, c := range cases {
		if got := listenAddrFromEndpoint(c.endpoint, c.fallback); got != c.want {
			t.Fatalf("%s: listenAddrFromEndpoint(%q,%q) = %q, want %q", c.name, c.endpoint, c.fallback, got, c.want)
		}
	}
}

// mirror_mode 驱动 driver 的镜像再生:缺省 prime-refresh(写入即见);
// manual 退回仅 prime 再生;unknown 值 fail-closed。
func TestReadLocalConfigMirrorMode(t *testing.T) {
	root := t.TempDir()
	write := func(body string) {
		p := filepath.Join(root, ".mnemon", "harness", "local")
		if err := os.MkdirAll(p, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(p, "config.json"), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(`{"schema_version":1,"mode":"local"}`) // 旧安装:缺省
	cfg, err := readLocalConfig(root)
	if err != nil || cfg.MirrorMode != "prime-refresh" {
		t.Fatalf("absent mirror_mode must default to prime-refresh; got %q err=%v", cfg.MirrorMode, err)
	}
	write(`{"schema_version":1,"mode":"local","mirror_mode":"manual"}`)
	if cfg, err = readLocalConfig(root); err != nil || cfg.MirrorMode != "manual" {
		t.Fatalf("manual must round-trip; got %q err=%v", cfg.MirrorMode, err)
	}
	write(`{"schema_version":1,"mode":"local","mirror_mode":"bogus"}`)
	if _, err = readLocalConfig(root); err == nil {
		t.Fatal("unknown mirror_mode must fail closed")
	}
}
