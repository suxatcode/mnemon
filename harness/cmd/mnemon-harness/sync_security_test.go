package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// `sync connect` is the WRITE-time half of the T2 downgrade gate (v1.1 #3): a plaintext
// non-loopback endpoint never enters remotes.json unless --allow-insecure-remote explicitly
// overrides; loopback plaintext (same-machine hub) stays allowed.
func TestSyncConnectRefusesPlaintextNonLoopbackEndpoint(t *testing.T) {
	restoreSyncFlags(t)
	syncRoot = t.TempDir()
	syncRemoteURL = "http://hub.example.test:9787"
	syncRemoteToken = "tok"
	cmd := mustTestCommand(t)
	if err := runSyncConnect(cmd, []string{"team"}); err == nil || !strings.Contains(err.Error(), "plaintext sync endpoint") {
		t.Fatalf("plaintext non-loopback connect must be refused, got %v", err)
	}
	if _, err := os.Stat(filepath.Join(syncRoot, ".mnemon", "harness", "sync", "remotes.json")); err == nil {
		t.Fatal("a refused connect must not write remotes.json")
	}

	syncAllowInsecure = true
	var out bytes.Buffer
	cmd = mustTestCommand(t)
	cmd.SetOut(&out)
	if err := runSyncConnect(cmd, []string{"team"}); err != nil {
		t.Fatalf("explicit --allow-insecure-remote must permit the connect: %v", err)
	}

	restoreSyncFlags(t)
	syncRoot = t.TempDir()
	syncRemoteURL = "http://127.0.0.1:9787"
	syncRemoteToken = "tok"
	cmd = mustTestCommand(t)
	if err := runSyncConnect(cmd, []string{"local"}); err != nil {
		t.Fatalf("loopback plaintext connect must stay allowed: %v", err)
	}
}

// --ca-file records the pinned TLS root into remotes.json (relative ref, resolved at read time),
// and resolveSyncRemote surfaces it for client construction.
func TestSyncConnectRecordsCAFile(t *testing.T) {
	restoreSyncFlags(t)
	syncRoot = t.TempDir()
	syncRemoteURL = "https://hub.example.test:9787"
	syncRemoteToken = "tok"
	syncCAFile = "certs/hub-ca.pem"
	cmd := mustTestCommand(t)
	if err := runSyncConnect(cmd, []string{"team"}); err != nil {
		t.Fatalf("connect with --ca-file: %v", err)
	}
	config := string(mustReadCmd(t, filepath.Join(syncRoot, ".mnemon", "harness", "sync", "remotes.json")))
	if !strings.Contains(config, `"ca_file": "certs/hub-ca.pem"`) {
		t.Fatalf("remotes.json must record ca_file:\n%s", config)
	}
	syncRemoteURL = ""
	syncRemoteToken = ""
	syncCAFile = ""
	remote, err := resolveSyncRemote()
	if err != nil {
		t.Fatalf("resolve remote with ca_file: %v", err)
	}
	want := filepath.Join(syncRoot, "certs", "hub-ca.pem")
	if remote.CAFile != want {
		t.Fatalf("ca_file must resolve against the project root: got %q want %q", remote.CAFile, want)
	}
}
