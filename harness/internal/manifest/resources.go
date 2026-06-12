package manifest

import (
	"fmt"
	"io/fs"
	"path"
	"sort"
)

type LoopManifest struct {
	SchemaVersion int               `json:"schema_version"`
	Name          string            `json:"name"`
	Version       string            `json:"version,omitempty"`
	Description   string            `json:"description,omitempty"`
	Surfaces      Surfaces          `json:"surfaces"`
	Assets        LoopAssets        `json:"assets"`
	// Store, when present and Native, declares that the loop is backed by a native `mnemon` store
	// (the projector ensures it via the mnemon CLI when --store is set). Declarative replacement for
	// the hardcoded loop.Name == "memory" gate (PD4); absent = not store-backed.
	Store *LoopStore `json:"store,omitempty"`
	// StateDirs are loop state directories the projector creates under the loop's state dir at
	// install (declarative replacement for the hardcoded skill-loop scaffolding; PD4). Each is a
	// safe relative path (no absolute, no "..").
	StateDirs []string `json:"state_dirs,omitempty"`
	// Env are the loop's extra runtime env vars, rendered into the runtime env.sh as
	// `export NAME="VALUE"` (declarative replacement for the hardcoded per-loop env switch; PD4).
	// Names are namespaced (^MNEMON_...) and values use a CLOSED shell-safe grammar — closed
	// projector variables (${state_dir}, ${host_skills_dir}) the projector substitutes, runtime bash
	// refs ${VAR} / ${VAR:-default}, and safe literals — so an external package can never inject
	// shell into a sourced file (the env injection lock).
	Env []EnvVar `json:"env,omitempty"`
}

type EnvVar struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type LoopStore struct {
	Native bool `json:"native"`
}

type LoopAssets struct {
	Guide        string   `json:"guide"`
	Env          string   `json:"env"`
	RuntimeFiles []string `json:"runtime_files,omitempty"`
	Skills       []string `json:"skills"`
	Subagents    []string `json:"subagents"`
}

type Surfaces struct {
	Projection  []string `json:"projection"`
	Observation []string `json:"observation"`
}

type BindingManifest struct {
	SchemaVersion    int               `json:"schema_version"`
	Name             string            `json:"name"`
	Host             string            `json:"host"`
	Loop             string            `json:"loop"`
	ProjectionPath   string            `json:"projection_path"`
	RuntimeSurface   string            `json:"runtime_surface"`
	LifecycleMapping map[string]string `json:"lifecycle_mapping"`
	Reconcile        []string          `json:"reconcile"`
}

func LoadLoop(fsys fs.FS, loop string) (LoopManifest, error) {
	var manifest LoopManifest
	if err := readManifest(fsys, path.Join("loops", loop, "loop.json"), &manifest); err != nil {
		return LoopManifest{}, err
	}
	// Env injection lock on the struct decode path too (G6 — both paths agree, fail-closed).
	for _, e := range manifest.Env {
		if err := validateEnvVar(e.Name, e.Value); err != nil {
			return LoopManifest{}, fmt.Errorf("loop %s: %w", loop, err)
		}
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
