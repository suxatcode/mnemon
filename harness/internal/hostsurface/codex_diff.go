package hostsurface

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path"
	"sort"

	"github.com/mnemon-dev/mnemon/harness/internal/assets"
	"github.com/mnemon-dev/mnemon/harness/internal/manifest"
)

type codexDesiredFile struct {
	Path             string
	Content          []byte
	Mode             os.FileMode
	PreserveExisting bool
	Metadata         string
}

type DriftItem struct {
	Host   string `json:"host"`
	Loop   string `json:"loop"`
	Action string `json:"action"`
	Target string `json:"target"`
	Detail string `json:"detail,omitempty"`
	DryRun bool   `json:"dry_run,omitempty"`
}

func (p codexProjector) diffLoop(loop manifest.LoopManifest, binding manifest.BindingManifest, dryRun bool) (bool, error) {
	items, err := p.driftItems(loop, binding, dryRun)
	if err != nil {
		return false, err
	}
	if dryRun {
		p.printf("Dry-run Codex %s install:\n", loop.Name)
	} else {
		p.printf("Codex %s diff:\n", loop.Name)
	}
	for _, item := range items {
		p.printf("  %s\n", item.Text())
	}
	if len(items) == 0 {
		p.printf("  no changes\n")
	}
	return len(items) > 0, nil
}

func (p codexProjector) driftItems(loop manifest.LoopManifest, binding manifest.BindingManifest, dryRun bool) ([]DriftItem, error) {
	files, err := p.desiredLoopFiles(loop, binding)
	if err != nil {
		return nil, err
	}
	var items []DriftItem
	for _, file := range files {
		item, err := p.diffDesiredFile(file, loop.Name, dryRun)
		if err != nil {
			return nil, err
		}
		if item == nil {
			continue
		}
		items = append(items, *item)
	}
	return items, nil
}

func (p codexProjector) desiredLoopFiles(loop manifest.LoopManifest, binding manifest.BindingManifest) ([]codexDesiredFile, error) {
	var files []codexDesiredFile
	for _, asset := range []struct {
		rel  string
		name string
		mode os.FileMode
	}{
		{rel: loop.Assets.Guide, name: "GUIDE.md", mode: 0o644},
		{rel: loop.Assets.Env, name: "env.sh", mode: 0o755},
		{rel: "loop.json", name: "loop.json", mode: 0o644},
	} {
		content, err := fs.ReadFile(assets.FS, p.loopAsset(loop, asset.rel))
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", asset.rel, err)
		}
		files = append(files, codexDesiredFile{
			Path:    pathJoin(p.stateDir(loop.Name), asset.name),
			Content: content,
			Mode:    asset.mode,
		})
	}
	for _, runtimeFile := range loop.Assets.RuntimeFiles {
		content, err := fs.ReadFile(assets.FS, p.loopAsset(loop, runtimeFile))
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", runtimeFile, err)
		}
		files = append(files, codexDesiredFile{
			Path:             pathJoin(p.stateDir(loop.Name), runtimeFile),
			Content:          content,
			Mode:             0o644,
			PreserveExisting: loop.Name == "memory",
		})
	}
	guideContent, err := fs.ReadFile(assets.FS, p.loopAsset(loop, loop.Assets.Guide))
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", loop.Assets.Guide, err)
	}
	files = append(files,
		codexDesiredFile{
			Path:    pathJoin(binding.RuntimeSurface, "env.sh"),
			Content: p.runtimeEnvContent(loop, binding),
			Mode:    0o755,
		},
		codexDesiredFile{
			Path:    pathJoin(binding.RuntimeSurface, "GUIDE.md"),
			Content: guideContent,
			Mode:    0o644,
		},
	)
	if loop.Name == "memory" {
		for _, runtimeFile := range loop.Assets.RuntimeFiles {
			content, err := fs.ReadFile(assets.FS, p.loopAsset(loop, runtimeFile))
			if err != nil {
				return nil, fmt.Errorf("read %s: %w", runtimeFile, err)
			}
			files = append(files, codexDesiredFile{
				Path:    pathJoin(binding.RuntimeSurface, runtimeFile),
				Content: content,
				Mode:    0o644,
			})
		}
	}
	for _, skill := range loop.Assets.Skills {
		content, err := p.projectedSkillContent(loop, binding, skill)
		if err != nil {
			return nil, err
		}
		files = append(files, codexDesiredFile{
			Path:    pathJoin(p.hostSkillsDir(loop.Name), skillID(skill), "SKILL.md"),
			Content: content,
			Mode:    0o644,
		})
	}
	var phases []string
	for phase := range loop.Assets.HookPrompts {
		if !p.hostHookExists(loop.Name, phase) {
			continue
		}
		phases = append(phases, phase)
	}
	sort.Strings(phases)
	for _, phase := range phases {
		source := path.Join("hosts", "codex", loop.Name, "hooks", phase+".sh")
		content, err := fs.ReadFile(assets.FS, source)
		if err != nil {
			return nil, fmt.Errorf("read %s hook: %w", phase, err)
		}
		files = append(files, codexDesiredFile{
			Path:    pathJoin(binding.ProjectionPath, "hooks", "mnemon-"+loop.Name, phase+".sh"),
			Content: content,
			Mode:    0o755,
		})
	}
	if p.codexHooksEnabled(loop.Name) {
		files = append(files, codexDesiredFile{Path: pathJoin(binding.ProjectionPath, "hooks.json"), Metadata: "codex_hooks"})
	}
	files = append(files,
		codexDesiredFile{Path: pathJoin(p.stateDir(loop.Name), "status.json"), Metadata: "loop_status"},
		codexDesiredFile{Path: p.hostManifestPath(), Metadata: "host_manifest"},
	)
	return files, nil
}

func (p codexProjector) diffDesiredFile(file codexDesiredFile, loopName string, dryRun bool) (*DriftItem, error) {
	if file.Metadata != "" {
		matches, err := p.metadataMatches(file, loopName)
		if err != nil {
			return nil, err
		}
		if matches {
			return nil, nil
		}
		if p.exists(file.Path) {
			return newDriftItem(loopName, "update", dryRun, file.Path, "metadata"), nil
		}
		return newDriftItem(loopName, "create", dryRun, file.Path, "metadata"), nil
	}
	actual, err := os.ReadFile(p.resolve(file.Path))
	if os.IsNotExist(err) {
		return newDriftItem(loopName, "create", dryRun, file.Path, ""), nil
	}
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", file.Path, err)
	}
	if file.PreserveExisting {
		return nil, nil
	}
	if bytes.Equal(actual, file.Content) {
		return nil, nil
	}
	return newDriftItem(loopName, "update", dryRun, file.Path, ""), nil
}

func (p codexProjector) metadataMatches(file codexDesiredFile, loopName string) (bool, error) {
	data, err := os.ReadFile(p.resolve(file.Path))
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("read %s: %w", file.Path, err)
	}
	switch file.Metadata {
	case "loop_status":
		var status map[string]any
		if err := json.Unmarshal(data, &status); err != nil {
			return false, nil
		}
		return status["loop"] == loopName && status["host"] == "codex" && status["phase"] == "projected", nil
	case "host_manifest":
		var manifest hostProjectionManifest
		if err := json.Unmarshal(data, &manifest); err != nil {
			return false, nil
		}
		entry, ok := manifest.Loops[loopName]
		return manifest.Host == "codex" && ok && len(entry.Ownership.Files) > 0, nil
	case "codex_hooks":
		var hooks map[string]any
		if err := json.Unmarshal(data, &hooks); err != nil {
			return false, nil
		}
		marker := "mnemon-" + loopName
		hooksDir := pathJoin(p.paths.configDir, "hooks", marker)
		opts := p.hookOptions(loopName)
		expected := map[string]string{"SessionStart": pathJoin(hooksDir, "prime.sh")}
		if opts.Remind {
			expected["UserPromptSubmit"] = pathJoin(hooksDir, "remind.sh")
		}
		if opts.Nudge {
			expected["Stop"] = pathJoin(hooksDir, "nudge.sh")
		}
		if opts.Compact {
			expected["PreCompact"] = pathJoin(hooksDir, "compact.sh")
		}
		return codexManagedHookCommandsMatch(hooks, marker, expected), nil
	default:
		return false, fmt.Errorf("unsupported metadata diff type: %s", file.Metadata)
	}
}

func codexManagedHookCommandsMatch(data map[string]any, marker string, expected map[string]string) bool {
	hooks, ok := data["hooks"].(map[string]any)
	if !ok {
		return false
	}
	seen := map[string]int{}
	for event, rawEntries := range hooks {
		entries, ok := rawEntries.([]any)
		if !ok {
			continue
		}
		for _, rawEntry := range entries {
			entry, ok := rawEntry.(map[string]any)
			if !ok {
				continue
			}
			rawHandlers, ok := entry["hooks"].([]any)
			if !ok {
				continue
			}
			entryUsesManagedHook := false
			for _, rawHandler := range rawHandlers {
				handler, ok := rawHandler.(map[string]any)
				if !ok {
					continue
				}
				command, ok := handler["command"].(string)
				if !ok || !commandUsesHookPath(command, marker) {
					continue
				}
				entryUsesManagedHook = true
				if expected[event] != command {
					return false
				}
				seen[event]++
			}
			if entryUsesManagedHook {
				if len(rawHandlers) != 1 {
					return false
				}
				handler, ok := rawHandlers[0].(map[string]any)
				if !ok || handler["type"] != "command" || handler["command"] != expected[event] {
					return false
				}
			}
		}
	}
	for event := range expected {
		if seen[event] != 1 {
			return false
		}
	}
	return true
}

func newDriftItem(loopName, action string, dryRun bool, target, detail string) *DriftItem {
	return &DriftItem{
		Host:   "codex",
		Loop:   loopName,
		Action: action,
		Target: target,
		Detail: detail,
		DryRun: dryRun,
	}
}

func (item DriftItem) Text() string {
	verb := item.Action
	if item.DryRun {
		verb = "would " + verb
	}
	if item.Detail != "" {
		return fmt.Sprintf("%s %s (%s)", verb, item.Target, item.Detail)
	}
	return fmt.Sprintf("%s %s", verb, item.Target)
}
