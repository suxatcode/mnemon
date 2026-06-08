package manifest

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

type ValidationResult struct {
	Lines []string
}

func ValidateHarness(root string) (ValidationResult, error) {
	if root == "" {
		root = "."
	}
	root = filepath.Clean(root)
	validator := harnessValidator{
		root:        root,
		loopsDir:    filepath.Join(root, "harness", "loops"),
		hostsDir:    filepath.Join(root, "harness", "hosts"),
		bindingsDir: filepath.Join(root, "harness", "bindings"),
	}
	return validator.validate()
}

type harnessValidator struct {
	root        string
	loopsDir    string
	hostsDir    string
	bindingsDir string
	lines       []string
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
	entries, err := os.ReadDir(v.loopsDir)
	if err != nil {
		return fmt.Errorf("read loops directory: %w", err)
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if err := v.validateLoop(filepath.Join(v.loopsDir, entry.Name())); err != nil {
			return err
		}
	}
	return nil
}

func (v *harnessValidator) validateLoop(loopDir string) error {
	manifest := filepath.Join(loopDir, "loop.json")
	if _, err := os.Stat(manifest); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("missing loop manifest: %s", manifest)
		}
		return fmt.Errorf("stat loop manifest: %w", err)
	}

	var data map[string]json.RawMessage
	if err := readManifest(manifest, &data); err != nil {
		return err
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
	for _, field := range []string{"control_model", "entity_profiles", "surfaces"} {
		if !hasField(data, field) {
			return fmt.Errorf("loop manifest missing %s: %s", field, manifest)
		}
	}

	controlModel, err := objectField(data, "control_model")
	if err != nil {
		return fmt.Errorf("loop manifest invalid control_model: %s: %w", manifest, err)
	}
	for _, field := range []string{"state", "intent", "reality", "reconcile"} {
		if !hasField(controlModel, field) {
			return fmt.Errorf("loop control_model missing %s: %s", field, manifest)
		}
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
	assetPaths, err := loopAssetPaths(assets)
	if err != nil {
		return fmt.Errorf("loop manifest invalid assets: %s: %w", manifest, err)
	}
	for _, rel := range assetPaths {
		if rel == "" {
			continue
		}
		if _, err := os.Stat(filepath.Join(loopDir, rel)); err != nil {
			if os.IsNotExist(err) {
				return fmt.Errorf("missing %s asset: %s", name, rel)
			}
			return fmt.Errorf("stat %s asset %s: %w", name, rel, err)
		}
	}

	jobs, err := loopJobSpecs(data, loopDir)
	if err != nil {
		return fmt.Errorf("loop manifest invalid jobs: %s: %w", manifest, err)
	}
	controllers, err := loopControllers(data)
	if err != nil {
		return fmt.Errorf("loop manifest invalid controllers: %s: %w", manifest, err)
	}
	for _, controller := range controllers {
		if _, ok := jobs[controller.Enqueue]; !ok {
			return fmt.Errorf("loop controller %s references missing job %s: %s", controller.Name, controller.Enqueue, manifest)
		}
	}

	hostAdapters, err := stringMapField(data, "host_adapters")
	if err != nil {
		return fmt.Errorf("loop manifest invalid host_adapters: %s: %w", manifest, err)
	}
	for _, rel := range hostAdapters {
		if rel == "" {
			continue
		}
		if _, err := os.Stat(filepath.Join(loopDir, rel)); err != nil {
			if os.IsNotExist(err) {
				return fmt.Errorf("missing %s host adapter path: %s", name, rel)
			}
			return fmt.Errorf("stat %s host adapter path %s: %w", name, rel, err)
		}
	}

	v.lines = append(v.lines, fmt.Sprintf("ok %s", name))
	return nil
}

func (v *harnessValidator) validateHosts() error {
	matches, err := filepath.Glob(filepath.Join(v.hostsDir, "*", "host.json"))
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
	if err := readManifest(manifest, &data); err != nil {
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
	matches, err := filepath.Glob(filepath.Join(v.bindingsDir, "*.json"))
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
	if err := readManifest(manifest, &data); err != nil {
		return "", err
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
	if _, err := os.Stat(filepath.Join(v.hostsDir, host, "host.json")); err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("binding references missing host: %s", manifest)
		}
		return "", fmt.Errorf("stat binding host reference: %w", err)
	}
	if _, err := os.Stat(filepath.Join(v.loopsDir, loop, "loop.json")); err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("binding references missing loop: %s", manifest)
		}
		return "", fmt.Errorf("stat binding loop reference: %w", err)
	}
	loopDir := filepath.Join(v.loopsDir, loop)
	switch schemaVersion {
	case 1:
		if err := validateBindingV1(data, loopDir); err != nil {
			return "", fmt.Errorf("binding manifest invalid v1 shape: %s: %w", manifest, err)
		}
	case 2:
		if err := validateBindingV2(data, loopDir); err != nil {
			return "", fmt.Errorf("binding manifest invalid v2 shape: %s: %w", manifest, err)
		}
	default:
		return "", fmt.Errorf("binding manifest schema_version must be 1 or 2: %s", manifest)
	}
	v.lines = append(v.lines, fmt.Sprintf("ok binding %s", name))
	return name, nil
}

func validateBindingV1(data map[string]json.RawMessage, loopDir string) error {
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
	return validateRunnerBindings(data, loopDir)
}

func validateBindingV2(data map[string]json.RawMessage, loopDir string) error {
	spec, err := objectField(data, "spec")
	if err != nil {
		return err
	}
	scope, err := requiredString(spec, "scope", "binding spec", "")
	if err != nil {
		return err
	}
	if scope != BindingScopeProject {
		return fmt.Errorf("spec.scope must be %s", BindingScopeProject)
	}
	if _, err := boolField(spec, "enabled"); err != nil {
		return fmt.Errorf("spec.enabled: %w", err)
	}
	hookMode, err := requiredString(spec, "hook_mode", "binding spec", "")
	if err != nil {
		return err
	}
	if !oneOf(hookMode, "native", "prompt", "manual", "none") {
		return fmt.Errorf("spec.hook_mode %q is not allowed", hookMode)
	}
	projection, err := objectField(spec, "projection")
	if err != nil {
		return fmt.Errorf("spec.projection: %w", err)
	}
	if _, err := requiredString(projection, "path", "binding spec.projection", ""); err != nil {
		return err
	}
	if _, err := requiredString(projection, "runtime_surface", "binding spec.projection", ""); err != nil {
		return err
	}
	if _, err := stringMapField(spec, "lifecycle_mapping"); err != nil {
		return fmt.Errorf("spec.lifecycle_mapping: %w", err)
	}
	rawReconcile, ok := spec["reconcile"]
	if !ok {
		return errors.New("spec missing reconcile")
	}
	if _, err := stringSlice(rawReconcile); err != nil {
		return fmt.Errorf("spec.reconcile: %w", err)
	}
	if err := validateRunnerBindings(spec, loopDir); err != nil {
		return fmt.Errorf("spec.runner_bindings: %w", err)
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
	hookPromptsRaw, ok := assets["hook_prompts"]
	if !ok {
		return nil, errors.New("missing hook_prompts")
	}
	hookPrompts, err := stringMapValues(hookPromptsRaw)
	if err != nil {
		return nil, fmt.Errorf("hook_prompts: %w", err)
	}
	paths = append(paths, hookPrompts...)
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

func loopJobSpecs(data map[string]json.RawMessage, loopDir string) (map[string]JobSpec, error) {
	raw, ok := data["jobs"]
	if !ok {
		return map[string]JobSpec{}, nil
	}
	var jobs map[string]JobSpec
	if err := json.Unmarshal(raw, &jobs); err != nil {
		return nil, err
	}
	if jobs == nil {
		return nil, errors.New("jobs must be an object")
	}
	for name, spec := range jobs {
		if name == "" {
			return nil, errors.New("job name must be non-empty")
		}
		if spec.Type != "deterministic" && spec.Type != "semantic" {
			return nil, fmt.Errorf("job %s type must be deterministic or semantic", name)
		}
		if spec.Spec != "" {
			if _, err := os.Stat(filepath.Join(loopDir, spec.Spec)); err != nil {
				if os.IsNotExist(err) {
					return nil, fmt.Errorf("job %s references missing spec asset: %s", name, spec.Spec)
				}
				return nil, fmt.Errorf("stat job %s spec asset %s: %w", name, spec.Spec, err)
			}
		}
		if spec.MaxTurns < 0 {
			return nil, fmt.Errorf("job %s max_turns must not be negative", name)
		}
	}
	return jobs, nil
}

func loopControllers(data map[string]json.RawMessage) ([]LoopController, error) {
	raw, ok := data["controllers"]
	if !ok {
		return nil, nil
	}
	var controllers []LoopController
	if err := json.Unmarshal(raw, &controllers); err != nil {
		return nil, err
	}
	for _, controller := range controllers {
		if controller.Name == "" {
			return nil, errors.New("controller name must be non-empty")
		}
		if len(controller.Watches) == 0 {
			return nil, fmt.Errorf("controller %s must watch at least one event type", controller.Name)
		}
		for _, watch := range controller.Watches {
			if watch == "" {
				return nil, fmt.Errorf("controller %s has empty watch event type", controller.Name)
			}
		}
		if controller.Enqueue == "" {
			return nil, fmt.Errorf("controller %s enqueue must be non-empty", controller.Name)
		}
	}
	return controllers, nil
}

func validateRunnerBindings(data map[string]json.RawMessage, loopDir string) error {
	raw, ok := data["runner_bindings"]
	if !ok {
		return nil
	}
	var bindings map[string]RunnerBinding
	if err := json.Unmarshal(raw, &bindings); err != nil {
		return err
	}
	if bindings == nil {
		return errors.New("runner_bindings must be an object")
	}
	for name, binding := range bindings {
		if name == "" {
			return errors.New("runner binding name must be non-empty")
		}
		switch binding.Mode {
		case "app_server":
			if binding.Runner == "" {
				return fmt.Errorf("runner binding %s app_server mode requires runner", name)
			}
		case "native_subagent":
			if binding.Agent == "" {
				return fmt.Errorf("runner binding %s native_subagent mode requires agent", name)
			}
		default:
			return fmt.Errorf("runner binding %s mode must be app_server or native_subagent", name)
		}
		if binding.PromptFrom != "" {
			if _, err := os.Stat(filepath.Join(loopDir, binding.PromptFrom)); err != nil {
				if os.IsNotExist(err) {
					return fmt.Errorf("runner binding %s references missing prompt asset: %s", name, binding.PromptFrom)
				}
				return fmt.Errorf("stat runner binding %s prompt asset %s: %w", name, binding.PromptFrom, err)
			}
		}
	}
	return nil
}

func readManifest(path string, target any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read manifest %s: %w", path, err)
	}
	if err := json.Unmarshal(data, target); err != nil {
		return fmt.Errorf("parse manifest %s: %w", path, err)
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

func stringMapValues(raw json.RawMessage) ([]string, error) {
	var object map[string]string
	if err := json.Unmarshal(raw, &object); err == nil && object != nil {
		keys := make([]string, 0, len(object))
		for key := range object {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		values := make([]string, 0, len(keys))
		for _, key := range keys {
			values = append(values, object[key])
		}
		return values, nil
	}
	return stringSlice(raw)
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
