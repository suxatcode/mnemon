package wasm

import (
	"strings"
	"testing"
)

func goodManifest() Manifest {
	return Manifest{
		ID:           "memory.admission.v1",
		Kind:         "rule",
		Version:      "0.1.0",
		ABIVersion:   ABIVersionRuleV0,
		WASMSHA256:   "207a6da006b5c5bba1414f8ee5164f07f2230cf510b5d340186a3cc60037aacf",
		Handles:      []string{"memory.write_candidate_observed"},
		Emits:        []string{"memory.write.proposed"},
		Capabilities: []string{"read_state_view"},
		Resources: ManifestResources{
			Reads:    []string{"memory/project"},
			Proposes: []string{"memory/project"},
		},
		Limits: ManifestLimits{
			TimeoutMS:      50,
			MemoryPages:    16,
			MaxInputBytes:  65536,
			MaxOutputBytes: 65536,
		},
	}
}

func TestManifestValidatesGoodPlugin(t *testing.T) {
	inspection, err := ValidateManifest(goodManifest(), readBytes(t, "testdata/rule_allow_if_evidence.wasm"))
	if err != nil {
		t.Fatalf("valid manifest rejected: %v", err)
	}
	if inspection.SHA256 != goodManifest().WASMSHA256 {
		t.Fatalf("inspection hash mismatch: %+v", inspection)
	}
	for _, want := range []string{"memory", "alloc", "evaluate"} {
		if !containsString(inspection.Exports, want) {
			t.Fatalf("inspection missing export %q: %+v", want, inspection.Exports)
		}
	}
}

func TestSampleManifestValidates(t *testing.T) {
	manifest, wasmBytes, err := LoadManifest("../../../wasm/plugins/memory-admission/manifest.json")
	if err != nil {
		t.Fatalf("load sample manifest: %v", err)
	}
	if _, err := ValidateManifest(manifest, wasmBytes); err != nil {
		t.Fatalf("sample manifest must validate: %v", err)
	}
}

func TestManifestRejectsWideningAndSmuggling(t *testing.T) {
	goodBytes := readBytes(t, "testdata/rule_allow_if_evidence.wasm")
	for _, tc := range []struct {
		name string
		edit func(*Manifest)
		want string
	}{
		{"missing-id", func(m *Manifest) { m.ID = "" }, "id"},
		{"handled-proposed", func(m *Manifest) { m.Handles = []string{"memory.write.proposed"} }, "handle"},
		{"bad-emit", func(m *Manifest) { m.Emits = []string{"goal.write.proposed"} }, "emit"},
		{"undeclared-propose", func(m *Manifest) { m.Resources.Proposes = nil }, "propose"},
		{"capability-expansion", func(m *Manifest) { m.Capabilities = append(m.Capabilities, "network") }, "capability"},
		{"hash-mismatch", func(m *Manifest) { m.WASMSHA256 = strings.Repeat("0", 64) }, "sha256"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := goodManifest()
			tc.edit(&m)
			_, err := ValidateManifest(m, goodBytes)
			if err == nil || !strings.Contains(strings.ToLower(err.Error()), tc.want) {
				t.Fatalf("expected %q rejection, got %v", tc.want, err)
			}
		})
	}
}

func TestManifestRejectsExtraImports(t *testing.T) {
	m := goodManifest()
	m.WASMSHA256 = "dd7c633babfcdfaa04ed9a9726fa8261a62099217faf3b442b9f7d5604387c5f"
	if _, err := ValidateManifest(m, readBytes(t, "testdata/two_imports.wasm")); err == nil {
		t.Fatal("manifest validation must reject a module importing beyond declared capabilities")
	}
}

func TestManifestRejectsMalformedImportSmuggling(t *testing.T) {
	m := goodManifest()
	m.WASMSHA256 = "27cc3eb17755cada739664be198373f27ed8630f8821be4831f62dd50be64241"
	if _, err := ValidateManifest(m, readBytes(t, "testdata/two_import_sections.wasm")); err == nil {
		t.Fatal("manifest validation must reject malformed import-section smuggling")
	}
}

func containsString(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}
