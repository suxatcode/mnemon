package wasm

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/tetratelabs/wazero"
)

const ABIVersionRuleV0 = "mnemon-wasm-rule-v0"

type Manifest struct {
	ID           string            `json:"id"`
	Kind         string            `json:"kind"`
	Version      string            `json:"version"`
	ABIVersion   string            `json:"abi_version"`
	WASMPath     string            `json:"wasm_path,omitempty"`
	WASMSHA256   string            `json:"wasm_sha256"`
	Handles      []string          `json:"handles"`
	Emits        []string          `json:"emits"`
	Resources    ManifestResources `json:"resources"`
	Capabilities []string          `json:"capabilities"`
	Limits       ManifestLimits    `json:"limits"`
}

type ManifestResources struct {
	Reads    []string `json:"reads,omitempty"`
	Proposes []string `json:"proposes,omitempty"`
}

type ManifestLimits struct {
	TimeoutMS      int `json:"timeout_ms"`
	MemoryPages    int `json:"memory_pages"`
	MaxInputBytes  int `json:"max_input_bytes"`
	MaxOutputBytes int `json:"max_output_bytes"`
}

type Inspection struct {
	Manifest Manifest `json:"manifest"`
	SHA256   string   `json:"sha256"`
	Imports  []string `json:"imports"`
	Exports  []string `json:"exports"`
}

func LoadManifest(path string) (Manifest, []byte, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Manifest{}, nil, fmt.Errorf("read manifest: %w", err)
	}
	var manifest Manifest
	if err := json.Unmarshal(raw, &manifest); err != nil {
		return Manifest{}, nil, fmt.Errorf("parse manifest: %w", err)
	}
	wasmPath := manifest.WASMPath
	if strings.TrimSpace(wasmPath) == "" {
		wasmPath = strings.TrimSuffix(path, filepath.Ext(path)) + ".wasm"
	} else if !filepath.IsAbs(wasmPath) {
		wasmPath = filepath.Join(filepath.Dir(path), wasmPath)
	}
	wasmBytes, err := os.ReadFile(wasmPath)
	if err != nil {
		return Manifest{}, nil, fmt.Errorf("read wasm module: %w", err)
	}
	return manifest, wasmBytes, nil
}

func ValidateManifest(manifest Manifest, wasmBytes []byte) (Inspection, error) {
	if strings.TrimSpace(manifest.ID) == "" {
		return Inspection{}, fmt.Errorf("manifest id is required")
	}
	if manifest.Kind != "rule" {
		return Inspection{}, fmt.Errorf("manifest kind must be rule")
	}
	if strings.TrimSpace(manifest.Version) == "" {
		return Inspection{}, fmt.Errorf("manifest version is required")
	}
	if manifest.ABIVersion != ABIVersionRuleV0 {
		return Inspection{}, fmt.Errorf("manifest abi_version must be %s", ABIVersionRuleV0)
	}
	if strings.TrimSpace(manifest.WASMSHA256) == "" {
		return Inspection{}, fmt.Errorf("manifest wasm_sha256 is required")
	}
	sum := sha256.Sum256(wasmBytes)
	actualSHA := hex.EncodeToString(sum[:])
	if manifest.WASMSHA256 != actualSHA {
		return Inspection{}, fmt.Errorf("manifest sha256 mismatch")
	}
	if err := validateEvents(manifest); err != nil {
		return Inspection{}, err
	}
	if err := validateResources(manifest); err != nil {
		return Inspection{}, err
	}
	allowedImports, err := validateCapabilities(manifest.Capabilities)
	if err != nil {
		return Inspection{}, err
	}
	if err := validateLimits(manifest.Limits); err != nil {
		return Inspection{}, err
	}
	inspection, err := InspectModule(wasmBytes)
	if err != nil {
		return Inspection{}, fmt.Errorf("inspect wasm module: %w", err)
	}
	for _, want := range []string{"memory", "alloc", "evaluate"} {
		if !stringSet(inspection.Exports)[want] {
			return Inspection{}, fmt.Errorf("wasm module must export memory, alloc, and evaluate")
		}
	}
	imports := stringSet(inspection.Imports)
	for imp := range imports {
		if !allowedImports[imp] {
			return Inspection{}, fmt.Errorf("wasm import %q is not declared by manifest capabilities", imp)
		}
	}
	inspection.Manifest = manifest
	inspection.SHA256 = actualSHA
	return inspection, nil
}

func InspectModule(wasmBytes []byte) (Inspection, error) {
	ctx := context.Background()
	rt := wazero.NewRuntime(ctx)
	defer rt.Close(ctx)
	compiled, err := rt.CompileModule(ctx, wasmBytes)
	if err != nil {
		return Inspection{}, err
	}
	defer compiled.Close(ctx)

	var imports []string
	for _, fn := range compiled.ImportedFunctions() {
		if mod, name, ok := fn.Import(); ok {
			imports = append(imports, mod+"."+name)
		}
	}
	for _, mem := range compiled.ImportedMemories() {
		if mod, name, ok := mem.Import(); ok {
			imports = append(imports, mod+"."+name)
		}
	}
	sort.Strings(imports)

	var exports []string
	for name := range compiled.ExportedFunctions() {
		exports = append(exports, name)
	}
	for name := range compiled.ExportedMemories() {
		exports = append(exports, name)
	}
	sort.Strings(exports)
	return Inspection{Imports: imports, Exports: exports}, nil
}

func validateEvents(manifest Manifest) error {
	if len(manifest.Handles) == 0 {
		return fmt.Errorf("manifest handles must not be empty")
	}
	for _, handle := range manifest.Handles {
		handle = strings.TrimSpace(handle)
		if handle == "" {
			return fmt.Errorf("manifest handle is empty")
		}
		if strings.HasSuffix(handle, ".proposed") || strings.HasSuffix(handle, ".diagnostic") {
			return fmt.Errorf("manifest handle %q cannot be an internal event", handle)
		}
	}
	if len(manifest.Emits) == 0 {
		return fmt.Errorf("manifest emits must not be empty")
	}
	allowed := map[string]bool{"memory.write.proposed": true, "skill.write.proposed": true}
	for _, emit := range manifest.Emits {
		if !allowed[strings.TrimSpace(emit)] {
			return fmt.Errorf("manifest emit %q is not a governed memory/skill proposal", emit)
		}
	}
	return nil
}

func validateResources(manifest Manifest) error {
	allowed := map[string]bool{"memory/project": true, "skill/project": true}
	for _, ref := range append(append([]string(nil), manifest.Resources.Reads...), manifest.Resources.Proposes...) {
		if !allowed[strings.TrimSpace(ref)] {
			return fmt.Errorf("manifest resource %q is not allowed", ref)
		}
	}
	proposes := stringSet(manifest.Resources.Proposes)
	for _, emit := range manifest.Emits {
		switch emit {
		case "memory.write.proposed":
			if !proposes["memory/project"] {
				return fmt.Errorf("manifest must declare propose access to memory/project")
			}
		case "skill.write.proposed":
			if !proposes["skill/project"] {
				return fmt.Errorf("manifest must declare propose access to skill/project")
			}
		}
	}
	return nil
}

func validateCapabilities(capabilities []string) (map[string]bool, error) {
	allowedImports := map[string]bool{}
	for _, cap := range capabilities {
		switch strings.TrimSpace(cap) {
		case "read_state_view":
			allowedImports["env.read_state_view"] = true
		default:
			return nil, fmt.Errorf("manifest capability %q is not allowed", cap)
		}
	}
	return allowedImports, nil
}

func validateLimits(limits ManifestLimits) error {
	if limits.TimeoutMS <= 0 {
		return fmt.Errorf("manifest limits.timeout_ms must be positive")
	}
	if limits.MemoryPages <= 0 {
		return fmt.Errorf("manifest limits.memory_pages must be positive")
	}
	if limits.MaxInputBytes <= 0 || limits.MaxOutputBytes <= 0 {
		return fmt.Errorf("manifest byte limits must be positive")
	}
	return nil
}

func stringSet(items []string) map[string]bool {
	out := make(map[string]bool, len(items))
	for _, item := range items {
		out[strings.TrimSpace(item)] = true
	}
	return out
}
