package manifest

import (
	"fmt"
	"io/fs"
	"path"
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

func LoadLoop(fsys fs.FS, loop string) (LoopManifest, error) {
	var manifest LoopManifest
	if err := readManifest(fsys, path.Join("loops", loop, "loop.json"), &manifest); err != nil {
		return LoopManifest{}, err
	}
	return manifest, nil
}

func LoadBinding(fsys fs.FS, host, loop string) (BindingManifest, error) {
	var manifest BindingManifest
	if err := readManifest(fsys, path.Join("bindings", host+"."+loop+".json"), &manifest); err != nil {
		return BindingManifest{}, err
	}
	return manifest, nil
}

func BindingsForHost(fsys fs.FS, host string) ([]BindingManifest, error) {
	matches, err := fs.Glob(fsys, "bindings/*.json")
	if err != nil {
		return nil, fmt.Errorf("glob binding manifests: %w", err)
	}
	var bindings []BindingManifest
	for _, manifestPath := range matches {
		var binding BindingManifest
		if err := readManifest(fsys, manifestPath, &binding); err != nil {
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

func LoopsForHost(fsys fs.FS, host string) ([]string, error) {
	bindings, err := BindingsForHost(fsys, host)
	if err != nil {
		return nil, err
	}
	loops := make([]string, 0, len(bindings))
	for _, binding := range bindings {
		loops = append(loops, binding.Loop)
	}
	return loops, nil
}
