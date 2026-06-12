package manifest

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"path"
)

type ValidationResult struct {
	Lines []string
}

// ValidateFS validates the loop/host/binding manifests rooted at fsys ("loops/", "hosts/",
// "bindings/" — no "harness/" prefix). An absent loops directory is tolerated (nothing to validate),
// so validating an external root that carries no harness assets passes trivially.
func ValidateFS(fsys fs.FS) (ValidationResult, error) {
	validator := harnessValidator{fsys: fsys}
	return validator.validate()
}

type harnessValidator struct {
	fsys  fs.FS
	lines []string
}

func (v *harnessValidator) validate() (ValidationResult, error) {
	if err := v.validateLoops(); err != nil {
		return ValidationResult{}, err
	}
	if err := v.validateHosts(); err != nil {
		return ValidationResult{}, err
	}
	if err := v.validateBindings(); err != nil {
		return ValidationResult{}, err
	}
	return ValidationResult{Lines: v.lines}, nil
}

func (v *harnessValidator) validateLoops() error {
	entries, err := fs.ReadDir(v.fsys, "loops")
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("read loops directory: %w", err)
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if err := v.validateLoop(path.Join("loops", entry.Name())); err != nil {
			return err
		}
	}
	return nil
}

func (v *harnessValidator) validateLoop(loopDir string) error {
	manifest := path.Join(loopDir, "loop.json")
	if _, err := fs.Stat(v.fsys, manifest); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("missing loop manifest: %s", manifest)
		}
		return fmt.Errorf("stat loop manifest: %w", err)
	}

	var data map[string]json.RawMessage
	if err := readManifest(v.fsys, manifest, &data); err != nil {
		return err
	}
	if err := rejectUnknownKeys(data, allowedLoopKeys, manifest); err != nil {
		return fmt.Errorf("loop manifest %w", err)
	}
	name, err := requiredString(data, "name", "loop manifest", manifest)
	if err != nil {
		return err
	}
	if name == "" {
		return fmt.Errorf("loop manifest missing name: %s", manifest)
	}
	schemaVersion, err := intField(data, "schema_version")
	if err != nil {
		return fmt.Errorf("loop manifest invalid schema_version: %s: %w", manifest, err)
	}
	if schemaVersion < 2 {
		return fmt.Errorf("loop manifest schema_version must be 2 or higher: %s", manifest)
	}
	if !hasField(data, "surfaces") {
		return fmt.Errorf("loop manifest missing surfaces: %s", manifest)
	}

	surfaces, err := objectField(data, "surfaces")
	if err != nil {
		return fmt.Errorf("loop manifest invalid surfaces: %s: %w", manifest, err)
	}
	for _, field := range []string{"projection", "observation"} {
		if !hasField(surfaces, field) {
			return fmt.Errorf("loop surfaces missing %s: %s", field, manifest)
		}
	}

	assets, err := objectField(data, "assets")
	if err != nil {
		return fmt.Errorf("loop manifest invalid assets: %s: %w", manifest, err)
	}
	if err := rejectUnknownKeys(assets, allowedLoopAssetKeys, manifest); err != nil {
		return fmt.Errorf("loop assets %w", err)
	}
	assetPaths, err := loopAssetPaths(assets)
	if err != nil {
		return fmt.Errorf("loop manifest invalid assets: %s: %w", manifest, err)
	}
	for _, rel := range assetPaths {
		if rel == "" {
			continue
		}
		if _, err := fs.Stat(v.fsys, path.Join(loopDir, rel)); err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return fmt.Errorf("missing %s asset: %s", name, rel)
			}
			return fmt.Errorf("stat %s asset %s: %w", name, rel, err)
		}
	}

	v.lines = append(v.lines, fmt.Sprintf("ok %s", name))
	return nil
}

func (v *harnessValidator) validateHosts() error {
	matches, err := fs.Glob(v.fsys, "hosts/*/host.json")
	if err != nil {
		return fmt.Errorf("glob host manifests: %w", err)
	}
	for _, manifest := range matches {
		if err := v.validateHost(manifest); err != nil {
			return err
		}
	}
	return nil
}

func (v *harnessValidator) validateHost(manifest string) error {
	var data map[string]json.RawMessage
	if err := readManifest(v.fsys, manifest, &data); err != nil {
		return err
	}
	name, err := requiredString(data, "name", "host manifest", manifest)
	if err != nil {
		return err
	}
	if name == "" {
		return fmt.Errorf("host manifest missing name: %s", manifest)
	}
	schemaVersion, err := intField(data, "schema_version")
	if err != nil {
		return fmt.Errorf("host manifest invalid schema_version: %s: %w", manifest, err)
	}
	if schemaVersion < 2 {
		return fmt.Errorf("host manifest schema_version must be 2 or higher: %s", manifest)
	}
	for _, field := range []string{"surfaces", "lifecycle_mapping"} {
		if !hasField(data, field) {
			return fmt.Errorf("host manifest missing %s: %s", field, manifest)
		}
	}
	surfaces, err := objectField(data, "surfaces")
	if err != nil {
		return fmt.Errorf("host manifest invalid surfaces: %s: %w", manifest, err)
	}
	for _, field := range []string{"projection", "observation"} {
		if !hasField(surfaces, field) {
			return fmt.Errorf("host surfaces missing %s: %s", field, manifest)
		}
	}
	v.lines = append(v.lines, fmt.Sprintf("ok host %s", name))
	return nil
}

func (v *harnessValidator) validateBindings() error {
	matches, err := fs.Glob(v.fsys, "bindings/*.json")
	if err != nil {
		return fmt.Errorf("glob binding manifests: %w", err)
	}
	seen := map[string]string{}
	for _, manifest := range matches {
		name, err := v.validateBinding(manifest)
		if err != nil {
			return err
		}
		if previous, ok := seen[name]; ok {
			return fmt.Errorf("duplicate binding name %q in %s and %s", name, previous, manifest)
		}
		seen[name] = manifest
	}
	return nil
}

func (v *harnessValidator) validateBinding(manifest string) (string, error) {
	var data map[string]json.RawMessage
	if err := readManifest(v.fsys, manifest, &data); err != nil {
		return "", err
	}
	if err := rejectUnknownKeys(data, allowedBindingKeys, manifest); err != nil {
		return "", fmt.Errorf("binding manifest %w", err)
	}
	schemaVersion, err := intField(data, "schema_version")
	if err != nil {
		return "", fmt.Errorf("binding manifest invalid schema_version: %s: %w", manifest, err)
	}
	name, err := requiredString(data, "name", "binding manifest", manifest)
	if err != nil {
		return "", err
	}
	host, err := requiredString(data, "host", "binding manifest", manifest)
	if err != nil {
		return "", err
	}
	loop, err := requiredString(data, "loop", "binding manifest", manifest)
	if err != nil {
		return "", err
	}
	if name == "" || host == "" || loop == "" {
		return "", fmt.Errorf("binding manifest missing name, host, or loop: %s", manifest)
	}
	if _, err := fs.Stat(v.fsys, path.Join("hosts", host, "host.json")); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return "", fmt.Errorf("binding references missing host: %s", manifest)
		}
		return "", fmt.Errorf("stat binding host reference: %w", err)
	}
	if _, err := fs.Stat(v.fsys, path.Join("loops", loop, "loop.json")); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return "", fmt.Errorf("binding references missing loop: %s", manifest)
		}
		return "", fmt.Errorf("stat binding loop reference: %w", err)
	}
	if schemaVersion != 1 {
		return "", fmt.Errorf("binding manifest schema_version must be 1: %s", manifest)
	}
	if err := validateBindingV1(data); err != nil {
		return "", fmt.Errorf("binding manifest invalid v1 shape: %s: %w", manifest, err)
	}
	v.lines = append(v.lines, fmt.Sprintf("ok binding %s", name))
	return name, nil
}

func validateBindingV1(data map[string]json.RawMessage) error {
	for _, field := range []string{"projection_path", "runtime_surface", "lifecycle_mapping", "reconcile"} {
		if !hasField(data, field) {
			return fmt.Errorf("missing %s", field)
		}
	}
	if _, err := requiredString(data, "projection_path", "binding manifest", ""); err != nil {
		return err
	}
	if _, err := requiredString(data, "runtime_surface", "binding manifest", ""); err != nil {
		return err
	}
	if _, err := stringMapField(data, "lifecycle_mapping"); err != nil {
		return err
	}
	rawReconcile, ok := data["reconcile"]
	if !ok {
		return errors.New("missing reconcile")
	}
	if _, err := stringSlice(rawReconcile); err != nil {
		return fmt.Errorf("reconcile: %w", err)
	}
	return nil
}

func loopAssetPaths(assets map[string]json.RawMessage) ([]string, error) {
	var paths []string
	for _, field := range []string{"guide", "env"} {
		value, err := requiredString(assets, field, "assets", "")
		if err != nil {
			return nil, err
		}
		paths = append(paths, value)
	}
	if raw, ok := assets["runtime_files"]; ok {
		values, err := stringSlice(raw)
		if err != nil {
			return nil, fmt.Errorf("runtime_files: %w", err)
		}
		paths = append(paths, values...)
	}
	for _, field := range []string{"skills", "subagents"} {
		raw, ok := assets[field]
		if !ok {
			return nil, fmt.Errorf("missing %s", field)
		}
		values, err := stringSlice(raw)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", field, err)
		}
		paths = append(paths, values...)
	}
	return paths, nil
}

// readManifest decodes strictly (G6): an unknown key in a manifest is junk that once hid six
// dead protocol fields for a full dev cycle — fail closed instead of silently dropping it.
// Strictness binds on struct targets (LoadLoop/LoadBinding); map targets see every key and are
// checked against the allowed-key sets by the validate path.
func readManifest(fsys fs.FS, name string, target any) error {
	data, err := fs.ReadFile(fsys, name)
	if err != nil {
		return fmt.Errorf("read manifest %s: %w", name, err)
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(target); err != nil {
		return fmt.Errorf("parse manifest %s: %w", name, err)
	}
	return nil
}

var (
	allowedLoopKeys = map[string]bool{
		"schema_version": true, "name": true, "version": true, "description": true,
		"surfaces": true, "assets": true,
	}
	allowedLoopAssetKeys = map[string]bool{
		"guide": true, "env": true, "runtime_files": true, "skills": true, "subagents": true,
	}
	allowedBindingKeys = map[string]bool{
		"schema_version": true, "name": true, "host": true, "loop": true,
		"projection_path": true, "runtime_surface": true, "lifecycle_mapping": true,
		"reconcile": true,
	}
)

// rejectUnknownKeys keeps the map-decoding validate path in agreement with the strict struct
// decode — without it, validate would bless a manifest that LoadLoop/LoadBinding then refuse.
func rejectUnknownKeys(data map[string]json.RawMessage, allowed map[string]bool, manifest string) error {
	for key := range data {
		if !allowed[key] {
			return fmt.Errorf("unknown key %q: %s", key, manifest)
		}
	}
	return nil
}

func hasField(data map[string]json.RawMessage, field string) bool {
	_, ok := data[field]
	return ok
}

func requiredString(data map[string]json.RawMessage, field, label, path string) (string, error) {
	raw, ok := data[field]
	if !ok {
		if path == "" {
			return "", fmt.Errorf("missing %s", field)
		}
		return "", fmt.Errorf("%s missing %s: %s", label, field, path)
	}
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return "", fmt.Errorf("%s field %s must be a string: %w", label, field, err)
	}
	return value, nil
}

func intField(data map[string]json.RawMessage, field string) (int, error) {
	raw, ok := data[field]
	if !ok {
		return 0, fmt.Errorf("missing %s", field)
	}
	var value int
	if err := json.Unmarshal(raw, &value); err != nil {
		return 0, err
	}
	return value, nil
}

func boolField(data map[string]json.RawMessage, field string) (bool, error) {
	raw, ok := data[field]
	if !ok {
		return false, fmt.Errorf("missing %s", field)
	}
	var value bool
	if err := json.Unmarshal(raw, &value); err != nil {
		return false, err
	}
	return value, nil
}

func objectField(data map[string]json.RawMessage, field string) (map[string]json.RawMessage, error) {
	raw, ok := data[field]
	if !ok {
		return nil, fmt.Errorf("missing %s", field)
	}
	var value map[string]json.RawMessage
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, err
	}
	if value == nil {
		return nil, fmt.Errorf("%s must be an object", field)
	}
	return value, nil
}

func stringMapField(data map[string]json.RawMessage, field string) (map[string]string, error) {
	raw, ok := data[field]
	if !ok {
		return nil, fmt.Errorf("missing %s", field)
	}
	var value map[string]string
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, err
	}
	if value == nil {
		return nil, fmt.Errorf("%s must be an object", field)
	}
	return value, nil
}


func stringSlice(raw json.RawMessage) ([]string, error) {
	var values []string
	if err := json.Unmarshal(raw, &values); err != nil {
		return nil, err
	}
	return values, nil
}

func oneOf(value string, allowed ...string) bool {
	for _, item := range allowed {
		if value == item {
			return true
		}
	}
	return false
}
