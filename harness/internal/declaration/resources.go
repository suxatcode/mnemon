package declaration

import (
	"fmt"
	"path/filepath"
	"sort"
)

type LoopManifest struct {
	SchemaVersion  int                `json:"schema_version"`
	Name           string             `json:"name"`
	Version        string             `json:"version,omitempty"`
	Description    string             `json:"description,omitempty"`
	ControlModel   map[string]any     `json:"control_model,omitempty"`
	EntityProfiles map[string]any     `json:"entity_profiles,omitempty"`
	Surfaces       Surfaces           `json:"surfaces"`
	Assets         LoopAssets         `json:"assets"`
	Controllers    []LoopController   `json:"controllers,omitempty"`
	Jobs           map[string]JobSpec `json:"jobs,omitempty"`
	HostAdapters   map[string]string  `json:"host_adapters"`
}

type LoopAssets struct {
	Guide        string            `json:"guide"`
	Env          string            `json:"env"`
	RuntimeFiles []string          `json:"runtime_files,omitempty"`
	HookPrompts  map[string]string `json:"hook_prompts"`
	Skills       []string          `json:"skills"`
	Subagents    []string          `json:"subagents"`
}

type HostManifest struct {
	SchemaVersion int             `json:"schema_version"`
	Name          string          `json:"name"`
	DisplayName   string          `json:"display_name,omitempty"`
	Description   string          `json:"description,omitempty"`
	Surfaces      Surfaces        `json:"surfaces"`
	Supports      map[string]bool `json:"supports,omitempty"`
}

type LoopController struct {
	Name    string   `json:"name"`
	Watches []string `json:"watches"`
	Enqueue string   `json:"enqueue"`
	Reason  string   `json:"reason,omitempty"`
}

type JobSpec struct {
	Type            string `json:"type"`
	Spec            string `json:"spec,omitempty"`
	PreferredRunner string `json:"preferred_runner,omitempty"`
	FallbackRunner  string `json:"fallback_runner,omitempty"`
	Governance      string `json:"governance,omitempty"`
	Prompt          string `json:"prompt,omitempty"`
	MaxTurns        int    `json:"max_turns,omitempty"`
}

type Surfaces struct {
	Projection  []string `json:"projection"`
	Observation []string `json:"observation"`
}

type BindingManifest struct {
	SchemaVersion    int                      `json:"schema_version"`
	Name             string                   `json:"name"`
	Host             string                   `json:"host"`
	Loop             string                   `json:"loop"`
	ProjectionPath   string                   `json:"projection_path"`
	RuntimeSurface   string                   `json:"runtime_surface"`
	LifecycleMapping map[string]string        `json:"lifecycle_mapping"`
	RunnerBindings   map[string]RunnerBinding `json:"runner_bindings,omitempty"`
	Reconcile        []string                 `json:"reconcile"`
}

type BindingManifestV2 struct {
	SchemaVersion int           `json:"schema_version"`
	Name          string        `json:"name"`
	Host          string        `json:"host"`
	Loop          string        `json:"loop"`
	Spec          BindingSpecV2 `json:"spec"`
}

const BindingScopeProject = "project"

type BindingSpecV2 struct {
	Scope            string                   `json:"scope"`
	Enabled          bool                     `json:"enabled"`
	HookMode         string                   `json:"hook_mode"`
	Projection       BindingProjectionSpec    `json:"projection"`
	LifecycleMapping map[string]string        `json:"lifecycle_mapping"`
	RunnerBindings   map[string]RunnerBinding `json:"runner_bindings,omitempty"`
	Reconcile        []string                 `json:"reconcile"`
}

type BindingProjectionSpec struct {
	Path           string `json:"path"`
	RuntimeSurface string `json:"runtime_surface"`
}

type RunnerBinding struct {
	Mode           string `json:"mode"`
	Runner         string `json:"runner,omitempty"`
	Agent          string `json:"agent,omitempty"`
	PromptFrom     string `json:"prompt_from,omitempty"`
	FallbackRunner string `json:"fallback_runner,omitempty"`
}

func LoadLoop(root, loop string) (LoopManifest, error) {
	var manifest LoopManifest
	path := filepath.Join(cleanRoot(root), "harness", "loops", loop, "loop.json")
	if err := readManifest(path, &manifest); err != nil {
		return LoopManifest{}, err
	}
	return manifest, nil
}

func LoadHost(root, host string) (HostManifest, error) {
	var manifest HostManifest
	path := filepath.Join(cleanRoot(root), "harness", "hosts", host, "host.json")
	if err := readManifest(path, &manifest); err != nil {
		return HostManifest{}, err
	}
	return manifest, nil
}

func LoadBinding(root, host, loop string) (BindingManifest, error) {
	var manifest BindingManifest
	path := filepath.Join(cleanRoot(root), "harness", "bindings", host+"."+loop+".json")
	if err := readManifest(path, &manifest); err != nil {
		return BindingManifest{}, err
	}
	return manifest, nil
}

func BindingsForHost(root, host string) ([]BindingManifest, error) {
	bindingsDir := filepath.Join(cleanRoot(root), "harness", "bindings")
	matches, err := filepath.Glob(filepath.Join(bindingsDir, "*.json"))
	if err != nil {
		return nil, fmt.Errorf("glob binding manifests: %w", err)
	}
	var bindings []BindingManifest
	for _, manifestPath := range matches {
		var binding BindingManifest
		if err := readManifest(manifestPath, &binding); err != nil {
			return nil, err
		}
		if binding.Host == host && binding.Loop != "" {
			bindings = append(bindings, binding)
		}
	}
	sort.Slice(bindings, func(i, j int) bool {
		return bindings[i].Loop < bindings[j].Loop
	})
	return bindings, nil
}

func LoopsForHost(root, host string) ([]string, error) {
	bindings, err := BindingsForHost(root, host)
	if err != nil {
		return nil, err
	}
	loops := make([]string, 0, len(bindings))
	for _, binding := range bindings {
		loops = append(loops, binding.Loop)
	}
	return loops, nil
}

func cleanRoot(root string) string {
	if root == "" {
		root = "."
	}
	return filepath.Clean(root)
}
