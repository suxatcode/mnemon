package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWasmInspectPrintsSafeMetadata(t *testing.T) {
	manifestPath := writeWasmManifestForTest(t)
	cmd, output := testCommand()
	if err := runWasmInspect(cmd, []string{manifestPath}); err != nil {
		t.Fatalf("wasm inspect: %v", err)
	}
	got := output.String()
	for _, want := range []string{
		"Plugin: memory.admission.v1",
		"Kind: rule",
		"Version: 0.1.0",
		"Handles: memory.write_candidate_observed",
		"Emits: memory.write.proposed",
		"Status: valid",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("wasm inspect output missing %q:\n%s", want, got)
		}
	}
	for _, blocked := range []string{"kernel", "runtime", "sync cursor", "token"} {
		if strings.Contains(strings.ToLower(got), blocked) {
			t.Fatalf("wasm inspect leaked %q:\n%s", blocked, got)
		}
	}
}

func TestWasmCommandGroupIncludesPromotionSpine(t *testing.T) {
	got := map[string]bool{}
	for _, cmd := range wasmCmd.Commands() {
		got[cmd.Name()] = true
	}
	for _, want := range []string{"inspect", "test", "shadow", "promote"} {
		if !got[want] {
			t.Fatalf("wasm command group missing %q; got %+v", want, got)
		}
	}
}

func writeWasmManifestForTest(t *testing.T) string {
	t.Helper()
	root := cmdRepoRoot(t)
	wasmPath := filepath.Join(root, "harness", "core", "rule", "wasm", "testdata", "rule_allow_if_evidence.wasm")
	doc := map[string]any{
		"id":          "memory.admission.v1",
		"kind":        "rule",
		"version":     "0.1.0",
		"abi_version": "mnemon-wasm-rule-v0",
		"wasm_path":   wasmPath,
		"wasm_sha256": "207a6da006b5c5bba1414f8ee5164f07f2230cf510b5d340186a3cc60037aacf",
		"handles":     []string{"memory.write_candidate_observed"},
		"emits":       []string{"memory.write.proposed"},
		"resources": map[string]any{
			"reads":    []string{"memory/project"},
			"proposes": []string{"memory/project"},
		},
		"capabilities": []string{"read_state_view"},
		"limits": map[string]any{
			"timeout_ms":       50,
			"memory_pages":     16,
			"max_input_bytes":  65536,
			"max_output_bytes": 65536,
		},
	}
	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "manifest.json")
	if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil {
		t.Fatalf("write wasm manifest: %v", err)
	}
	return path
}
