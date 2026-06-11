package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mnemon-dev/mnemon/harness/internal/app"
	"github.com/mnemon-dev/mnemon/harness/internal/capability"
	"github.com/mnemon-dev/mnemon/harness/internal/channel"
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
	cfg, err := app.LocalRuntimeConfigFromBindings(boot.Loaded.Bindings, nil)
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

// T1 回环地板:非回环监听地址 fail-closed,--allow-nonloopback 显式越权。
func TestValidateListenAddrLoopbackOnly(t *testing.T) {
	for _, ok := range []string{"127.0.0.1:8787", "localhost:8787", "[::1]:8787"} {
		if err := validateListenAddr(ok, false); err != nil {
			t.Fatalf("%s must be allowed: %v", ok, err)
		}
	}
	for _, bad := range []string{"0.0.0.0:8787", "192.168.1.10:8787", ":8787"} {
		if err := validateListenAddr(bad, false); err == nil {
			t.Fatalf("%s must be refused without --allow-nonloopback", bad)
		}
		if err := validateListenAddr(bad, true); err != nil {
			t.Fatalf("%s must pass with explicit override: %v", bad, err)
		}
	}
}

// rotate:以 bindings.json 的 credential_ref 为唯一目标,强制重写;新 token 经 LoadBindingFile
// 生效映射,旧值不再在 Tokens 中(重启生效语义由命令输出与 USAGE 声明)。
func TestRotateTokenInvalidatesOldValue(t *testing.T) {
	root := t.TempDir()
	setupProductIntegration(t, root)
	tokPath := filepath.Join(root, ".mnemon", "harness", "channel", "credentials", "codex-project.token")
	oldRaw, err := os.ReadFile(tokPath)
	if err != nil {
		t.Fatal(err)
	}
	ref, err := rotateToken(root, "codex@project")
	if err != nil {
		t.Fatalf("rotate: %v", err)
	}
	if ref == "" {
		t.Fatal("rotate must report the credential_ref it rewrote")
	}
	newRaw, err := os.ReadFile(tokPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(newRaw) == string(oldRaw) {
		t.Fatal("rotate must change the token content")
	}
	if st, _ := os.Stat(tokPath); st.Mode().Perm() != 0o600 {
		t.Fatalf("rotated token mode %o, want 0600", st.Mode().Perm())
	}
	loaded, err := channel.LoadBindingFile(root, filepath.Join(root, ".mnemon", "harness", "channel", "bindings.json"))
	if err != nil {
		t.Fatal(err)
	}
	oldTok := strings.TrimSpace(string(oldRaw))
	newTok := strings.TrimSpace(string(newRaw))
	if _, stale := loaded.Tokens[oldTok]; stale {
		t.Fatal("old token must no longer map to any principal")
	}
	if p := loaded.Tokens[newTok]; p != "codex@project" {
		t.Fatalf("new token must map to the principal, got %q", p)
	}
	if _, err := rotateToken(root, "ghost@nowhere"); err == nil {
		t.Fatal("rotate for an unknown principal must error clearly")
	}
}
