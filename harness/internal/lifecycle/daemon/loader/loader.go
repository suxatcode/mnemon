package loader

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"

	"github.com/mnemon-dev/mnemon/harness/internal/declaration"
	"go.yaml.in/yaml/v3"
)

// strictYAML decodes a single YAML document, rejecting unknown fields so a typo
// (e.g. cost-usd vs cost_usd) errors at load/dry-run instead of being silently
// dropped. An empty document is treated as no fields.
func strictYAML(data []byte, v any) error {
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(v); err != nil {
		if errors.Is(err, io.EOF) {
			return nil
		}
		return err
	}
	return nil
}

type Options struct {
	AcknowledgeModelCost bool
}

func Load(root string, opts Options) (Catalog, error) {
	if root == "" {
		root = "."
	}
	root = filepath.Clean(root)
	catalog := Catalog{}
	global, warnings, err := loadGlobal(filepath.Join(root, "harness", "control", "daemon.yaml"))
	if err != nil {
		return Catalog{}, err
	}
	catalog.GlobalBudget = global
	catalog.Warnings = append(catalog.Warnings, warnings...)

	lifted, err := liftControllers(root)
	if err != nil {
		return Catalog{}, err
	}
	byID := map[string]Definition{}
	for _, def := range lifted {
		byID[def.ID] = def
	}

	explicit, warnings, err := loadExplicit(root, opts, catalog.GlobalBudget)
	if err != nil {
		return Catalog{}, err
	}
	catalog.Warnings = append(catalog.Warnings, warnings...)
	for _, def := range explicit {
		byID[def.ID] = def
	}

	for _, def := range byID {
		catalog.Jobs = append(catalog.Jobs, def)
	}
	sort.Slice(catalog.Jobs, func(i, j int) bool {
		return catalog.Jobs[i].ID < catalog.Jobs[j].ID
	})
	return catalog, nil
}

func loadGlobal(path string) (GlobalBudget, []string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return GlobalBudget{}, nil, nil
		}
		return GlobalBudget{}, nil, fmt.Errorf("read daemon global budget: %w", err)
	}
	var cfg GlobalConfig
	if err := strictYAML(data, &cfg); err != nil {
		return GlobalBudget{}, nil, fmt.Errorf("decode daemon global budget %s: %w", path, err)
	}
	return cfg.GlobalBudget, nil, nil
}

func loadExplicit(root string, opts Options, global GlobalBudget) ([]Definition, []string, error) {
	dir := filepath.Join(root, "harness", "control", "jobs")
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil, nil
		}
		return nil, nil, fmt.Errorf("read daemon jobs dir: %w", err)
	}
	seen := map[string]string{}
	var defs []Definition
	var warnings []string
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".yaml" {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, nil, fmt.Errorf("read daemon job %s: %w", path, err)
		}
		var def Definition
		if err := strictYAML(data, &def); err != nil {
			return nil, nil, fmt.Errorf("decode daemon job %s: %w", path, err)
		}
		def.Source = Source{Path: path, Kind: "yaml"}
		jobWarnings, err := validateDefinition(&def, validateContext{
			globalBudget:          global,
			acknowledgeModelCost:  opts.AcknowledgeModelCost,
			checkSpawnRunnerGate:  true,
			allowLiftedController: false,
			sourcePath:            path,
		})
		if err != nil {
			return nil, nil, err
		}
		if previous, ok := seen[def.ID]; ok {
			return nil, nil, fmt.Errorf("duplicate daemon job id %q in %s and %s", def.ID, previous, path)
		}
		seen[def.ID] = path
		warnings = append(warnings, jobWarnings...)
		defs = append(defs, def)
	}
	sort.Slice(defs, func(i, j int) bool { return defs[i].ID < defs[j].ID })
	return defs, warnings, nil
}

func liftControllers(root string) ([]Definition, error) {
	loopsDir := filepath.Join(root, "harness", "loops")
	entries, err := os.ReadDir(loopsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read loop declarations: %w", err)
	}
	var defs []Definition
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		loop, err := declaration.LoadLoop(root, entry.Name())
		if err != nil {
			return nil, err
		}
		for _, controller := range loop.Controllers {
			spec, ok := loop.Jobs[controller.Enqueue]
			if !ok {
				return nil, fmt.Errorf("controller %s references missing job %s", controller.Name, controller.Enqueue)
			}
			def := Definition{
				ID:          controller.Name,
				Description: controller.Reason,
				When:        triggerFromWatches(controller.Watches),
				Do:          Action{Subagent: controller.Enqueue},
				Budget:      Budget{MaxTurns: spec.MaxTurns},
				Metadata: map[string]any{
					"loop":        loop.Name,
					"controller":  controller.Name,
					"job":         controller.Enqueue,
					"source_kind": "loop_controller",
				},
				Source: Source{
					Path:       filepath.Join(root, "harness", "loops", entry.Name(), "loop.json"),
					Kind:       "loop_controller",
					Loop:       loop.Name,
					Controller: controller.Name,
				},
			}
			if _, err := validateDefinition(&def, validateContext{
				allowLiftedController: true,
				sourcePath:            def.Source.Path,
			}); err != nil {
				return nil, err
			}
			defs = append(defs, def)
		}
	}
	sort.Slice(defs, func(i, j int) bool { return defs[i].ID < defs[j].ID })
	return defs, nil
}

func triggerFromWatches(watches []string) Trigger {
	if len(watches) == 1 {
		return Trigger{Event: watches[0]}
	}
	var any []Trigger
	for _, watch := range watches {
		any = append(any, Trigger{Event: watch})
	}
	return Trigger{Any: any}
}
