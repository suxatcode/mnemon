package hostsurface

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/mnemon-dev/mnemon/harness/internal/assets"
	"github.com/mnemon-dev/mnemon/harness/internal/manifest"
)

type ClaudeOptions struct {
	ProjectRoot string
	Loops       []string
	HostArgs    []string
	Stdout      io.Writer
	Stderr      io.Writer
}

type claudeHostOptions struct {
	global            bool
	configDir         string
	configDirExplicit bool
	storeName         string
	hostSkillsDir     string
	remind            bool
	remindSet         bool
	nudge             bool
	compact           bool
	purgeMemory       bool
	purgeLibrary      bool
}

type claudeProjector struct {
	projectorCore
	hostOptions claudeHostOptions
}

func newClaudeProjector(opts ClaudeOptions) (claudeProjector, []string, error) {
	var err error
	if opts.ProjectRoot == "" {
		opts.ProjectRoot, err = os.Getwd()
		if err != nil {
			return claudeProjector{}, nil, fmt.Errorf("resolve project root: %w", err)
		}
	}
	projectRoot, err := filepath.Abs(opts.ProjectRoot)
	if err != nil {
		return claudeProjector{}, nil, fmt.Errorf("resolve project root: %w", err)
	}
	hostOptions, err := parseClaudeHostOptions(opts.HostArgs)
	if err != nil {
		return claudeProjector{}, nil, err
	}
	if opts.Stdout == nil {
		opts.Stdout = io.Discard
	}
	if opts.Stderr == nil {
		opts.Stderr = io.Discard
	}
	if _, err := manifest.ValidateFS(assets.FS); err != nil {
		return claudeProjector{}, nil, err
	}
	loops := append([]string(nil), opts.Loops...)
	if len(loops) == 0 {
		return claudeProjector{}, nil, errors.New("at least one --loop is required")
	}
	sort.Strings(loops)
	return claudeProjector{
		projectorCore: projectorCore{
			host:        "claude-code",
			projectRoot: projectRoot,
			paths:       claudeProjectorPaths(hostOptions),
			stdout:      opts.Stdout,
			stderr:      opts.Stderr,
			managed:     newManagedState(),
		},
		hostOptions: hostOptions,
	}, loops, nil
}

func RunClaudeProjector(ctx context.Context, action string, opts ClaudeOptions) error {
	if action != "install" && action != "uninstall" {
		return fmt.Errorf("unsupported Claude Code projector action: %s", action)
	}
	projector, loops, err := newClaudeProjector(opts)
	if err != nil {
		return err
	}
	for _, loopName := range loops {
		loop, err := manifest.LoadLoop(assets.FS, loopName)
		if err != nil {
			return err
		}
		binding, err := manifest.LoadBinding(assets.FS, "claude-code", loopName)
		if err != nil {
			return err
		}
		switch action {
		case "install":
			if err := projector.installLoop(ctx, loop, binding); err != nil {
				return fmt.Errorf("install claude-code/%s: %w", loopName, err)
			}
		case "uninstall":
			if err := projector.uninstallLoop(loop, binding); err != nil {
				return fmt.Errorf("uninstall claude-code/%s: %w", loopName, err)
			}
		}
	}
	return nil
}

// RunClaudeProjectorReport installs/re-projects the Claude Code projection under the no-clobber policy
// and returns the managed files it preserved because the user edited them.
func RunClaudeProjectorReport(ctx context.Context, opts ClaudeOptions) (Report, error) {
	projector, loops, err := newClaudeProjector(opts)
	if err != nil {
		return Report{}, err
	}
	for _, loopName := range loops {
		loop, err := manifest.LoadLoop(assets.FS, loopName)
		if err != nil {
			return Report{}, err
		}
		binding, err := manifest.LoadBinding(assets.FS, "claude-code", loopName)
		if err != nil {
			return Report{}, err
		}
		if err := projector.installLoop(ctx, loop, binding); err != nil {
			return Report{}, fmt.Errorf("install claude-code/%s: %w", loopName, err)
		}
	}
	return Report{Conflicts: projector.managed.conflicts}, nil
}

func parseClaudeHostOptions(args []string) (claudeHostOptions, error) {
	parsed := claudeHostOptions{
		configDir: ".claude",
		nudge:     true,
		compact:   true,
	}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch arg {
		case "--global":
			parsed.global = true
		case "--config-dir":
			if i+1 >= len(args) {
				return parsed, errors.New("missing value for --config-dir")
			}
			parsed.configDir = args[i+1]
			parsed.configDirExplicit = true
			i++
		case "--store":
			if i+1 >= len(args) {
				return parsed, errors.New("missing value for --store")
			}
			parsed.storeName = args[i+1]
			i++
		case "--host-skills-dir":
			if i+1 >= len(args) {
				return parsed, errors.New("missing value for --host-skills-dir")
			}
			parsed.hostSkillsDir = args[i+1]
			i++
		case "--with-remind":
			parsed.remind = true
			parsed.remindSet = true
		case "--no-remind":
			parsed.remind = false
			parsed.remindSet = true
		case "--no-nudge":
			parsed.nudge = false
		case "--no-compact":
			parsed.compact = false
		case "--purge-memory":
			parsed.purgeMemory = true
		case "--purge-library":
			parsed.purgeLibrary = true
		default:
			return parsed, fmt.Errorf("unsupported Claude Code host option: %s", arg)
		}
	}
	return parsed, nil
}

func claudeProjectorPaths(opts claudeHostOptions) corePaths {
	if opts.global && !opts.configDirExplicit {
		home := os.Getenv("HOME")
		configDir := filepath.Join(home, ".claude")
		mnemonDir := os.Getenv("MNEMON_HARNESS_STATE_DIR")
		if mnemonDir == "" {
			mnemonDir = filepath.Join(home, ".mnemon")
		}
		return corePaths{configDir: filepath.ToSlash(configDir), mnemonDir: filepath.ToSlash(mnemonDir)}
	}
	mnemonDir := os.Getenv("MNEMON_HARNESS_STATE_DIR")
	if mnemonDir == "" {
		mnemonDir = ".mnemon"
	}
	return corePaths{configDir: filepath.ToSlash(opts.configDir), mnemonDir: filepath.ToSlash(mnemonDir)}
}

func (p claudeProjector) installLoop(ctx context.Context, loop manifest.LoopManifest, binding manifest.BindingManifest) error {
	switch loop.Name {
	case "memory", "skill":
	default:
		return fmt.Errorf("unsupported loop for Claude Code: %s", loop.Name)
	}
	p.beginManaged(loop.Name)
	if err := p.copyCommonCanonicalAssets(loop); err != nil {
		return err
	}
	if err := p.prepareLoopState(loop); err != nil {
		return err
	}
	if err := p.writeRuntimeEnv(loop, binding); err != nil {
		return err
	}
	if err := p.projectManaged(p.loopAsset(loop, loop.Assets.Guide), pathJoin(binding.RuntimeSurface, "GUIDE.md"), 0o644); err != nil {
		return err
	}
	if err := p.projectSkills(loop, binding); err != nil {
		return err
	}
	if err := p.projectAgents(loop, binding); err != nil {
		return err
	}
	if err := p.projectHooks(loop, binding); err != nil {
		return err
	}
	if loop.Name == "memory" || loop.Name == "skill" {
		if err := p.patchSettings(loop.Name); err != nil {
			return err
		}
	}
	if loop.Name == "memory" && p.hostOptions.storeName != "" {
		if err := p.ensureStore(ctx, p.hostOptions.storeName); err != nil {
			return err
		}
	}
	ownership := p.loopOwnership(loop, binding)
	ownership.Hashes = p.managed.next
	ownership.MarkerVersion = managedMarkerVersion
	if err := p.writeHostManifest(loop, binding, ownership); err != nil {
		return err
	}
	if err := p.writeLoopStatus(loop, binding); err != nil {
		return err
	}
	p.printf("Installed Mnemon %s loop for Claude Code.\n", loop.Name)
	p.printf("Config:       %s\n", p.paths.configDir)
	p.printf("State:        %s\n", p.stateDir(loop.Name))
	if loop.Name == "memory" {
		p.printf("Memory:       %s\n", pathJoin(p.stateDir(loop.Name), "MEMORY.md"))
	}
	if hostSkills := p.hostSkillsDir(loop.Name); hostSkills != "" {
		p.printf("Host skills:  %s\n", hostSkills)
	}
	return nil
}

func (p claudeProjector) uninstallLoop(loop manifest.LoopManifest, binding manifest.BindingManifest) error {
	p.beginManaged(loop.Name) // load recorded ownership so uninstall preserves user-edited/foreign skills
	if loop.Name == "memory" || loop.Name == "skill" {
		if err := p.unpatchSettings(loop.Name); err != nil {
			return err
		}
	}
	hostSkillsDir := p.installedHostSkillsDir(loop.Name, binding)
	if loop.Name == "skill" {
		if err := p.removeGeneratedSkillViews(hostSkillsDir); err != nil {
			return err
		}
	}
	for _, skill := range loop.Assets.Skills {
		if err := p.removeManagedSkill(pathJoin(hostSkillsDir, skillID(skill), "SKILL.md")); err != nil {
			return err
		}
	}
	for _, subagent := range loop.Assets.Subagents {
		if err := p.removeManagedFile(pathJoin(p.paths.configDir, "agents", agentFile(loop.Name, subagent))); err != nil {
			return fmt.Errorf("remove projected agent: %w", err)
		}
	}
	if err := p.removeManagedTree(pathJoin(p.paths.configDir, "hooks", "mnemon-"+loop.Name)); err != nil {
		return err
	}
	if err := p.removeManagedTree(binding.RuntimeSurface); err != nil {
		return err
	}
	if err := p.removeCanonicalState(loop); err != nil {
		return err
	}
	if err := p.removeHostManifestLoop(loop.Name); err != nil {
		return err
	}
	p.printf("Removed Mnemon %s loop from %s.\n", loop.Name, p.paths.configDir)
	return nil
}

func (p claudeProjector) copyCommonCanonicalAssets(loop manifest.LoopManifest) error {
	for _, asset := range []struct {
		rel  string
		name string
		mode os.FileMode
	}{
		{rel: loop.Assets.Guide, name: "GUIDE.md", mode: 0o644},
		{rel: loop.Assets.Env, name: "env.sh", mode: 0o755},
		{rel: "loop.json", name: "loop.json", mode: 0o644},
	} {
		if err := p.copyFile(p.loopAsset(loop, asset.rel), pathJoin(p.stateDir(loop.Name), asset.name), asset.mode); err != nil {
			return err
		}
	}
	return nil
}

func (p claudeProjector) prepareLoopState(loop manifest.LoopManifest) error {
	switch loop.Name {
	case "memory":
		for _, runtimeFile := range loop.Assets.RuntimeFiles {
			if err := p.copyFileIfMissing(p.loopAsset(loop, runtimeFile), pathJoin(p.stateDir(loop.Name), runtimeFile), 0o644); err != nil {
				return err
			}
		}
	case "skill":
		for _, dir := range []string{"skills/active", "skills/stale", "skills/archived", "proposals", "reports"} {
			if err := os.MkdirAll(p.resolve(pathJoin(p.stateDir(loop.Name), dir)), 0o755); err != nil {
				return fmt.Errorf("mkdir %s: %w", dir, err)
			}
		}
	}
	return nil
}

func (p claudeProjector) writeRuntimeEnv(loop manifest.LoopManifest, binding manifest.BindingManifest) error {
	stateDir := p.stateDir(loop.Name)
	lines := []string{
		"#!/usr/bin/env bash",
		exportLine(loopEnvName(loop.Name), pathJoin(stateDir, "env.sh")),
		exportLine(loopDirVarName(loop.Name), stateDir),
	}
	switch loop.Name {
	case "memory":
		lines = append(lines, `export MNEMON_MEMORY_LOOP_MAX_NON_EMPTY_LINES="${MNEMON_MEMORY_LOOP_MAX_NON_EMPTY_LINES:-200}"`)
	case "skill":
		hostSkillsDir := p.hostSkillsDir(loop.Name)
		lines = append(lines,
			exportLine("MNEMON_SKILL_LOOP_LIBRARY_DIR", pathJoin(stateDir, "skills")),
			exportLine("MNEMON_SKILL_LOOP_ACTIVE_DIR", pathJoin(stateDir, "skills/active")),
			exportLine("MNEMON_SKILL_LOOP_STALE_DIR", pathJoin(stateDir, "skills/stale")),
			exportLine("MNEMON_SKILL_LOOP_ARCHIVED_DIR", pathJoin(stateDir, "skills/archived")),
			exportLine("MNEMON_SKILL_LOOP_USAGE_FILE", pathJoin(stateDir, "skills/.usage.jsonl")),
			exportLine("MNEMON_SKILL_LOOP_PROPOSALS_DIR", pathJoin(stateDir, "proposals")),
			exportLine("MNEMON_SKILL_LOOP_HOST_SKILLS_DIR", hostSkillsDir),
			`export MNEMON_SKILL_LOOP_REVIEW_MIN_EVENTS="${MNEMON_SKILL_LOOP_REVIEW_MIN_EVENTS:-20}"`,
			`export MNEMON_SKILL_LOOP_PROTECTED_SKILLS="${MNEMON_SKILL_LOOP_PROTECTED_SKILLS:-skill-observe,skill-curate,skill-author,skill-manage,memory-get,memory-set}"`,
		)
	}
	content := strings.Join(lines, "\n") + "\n"
	return p.writeFile(pathJoin(binding.RuntimeSurface, "env.sh"), []byte(content), 0o755)
}

func (p claudeProjector) projectSkills(loop manifest.LoopManifest, binding manifest.BindingManifest) error {
	hostSkillsDir := p.hostSkillsDir(loop.Name)
	for _, skill := range loop.Assets.Skills {
		target := pathJoin(hostSkillsDir, skillID(skill), "SKILL.md")
		if err := p.projectManaged(p.loopAsset(loop, skill), target, 0o644); err != nil {
			return err
		}
	}
	return nil
}

func (p claudeProjector) projectAgents(loop manifest.LoopManifest, binding manifest.BindingManifest) error {
	for _, subagent := range loop.Assets.Subagents {
		target := pathJoin(binding.ProjectionPath, "agents", agentFile(loop.Name, subagent))
		if err := p.projectManaged(p.loopAsset(loop, subagent), target, 0o644); err != nil {
			return err
		}
	}
	return nil
}

func (p claudeProjector) projectHooks(loop manifest.LoopManifest, binding manifest.BindingManifest) error {
	for phase := range loop.Assets.HookPrompts {
		source := path.Join("hosts", "claude-code", loop.Name, "hooks", phase+".sh")
		if _, err := fs.Stat(assets.FS, source); errors.Is(err, fs.ErrNotExist) {
			continue
		} else if err != nil {
			return fmt.Errorf("stat hook %s: %w", phase, err)
		}
		target := pathJoin(binding.ProjectionPath, "hooks", "mnemon-"+loop.Name, phase+".sh")
		if err := p.projectManaged(source, target, 0o755); err != nil {
			return err
		}
	}
	return nil
}

func (p claudeProjector) patchSettings(loopName string) error {
	return patchClaudeSettings(p.resolve(pathJoin(p.paths.configDir, "settings.json")), p.paths.configDir, "mnemon-"+loopName, p.hookOptions(loopName))
}

func (p claudeProjector) unpatchSettings(loopName string) error {
	return unpatchClaudeSettings(p.resolve(pathJoin(p.paths.configDir, "settings.json")), "mnemon-"+loopName)
}

func (p claudeProjector) hookOptions(loopName string) claudeHookOptions {
	remind := p.hostOptions.remind
	if !p.hostOptions.remindSet {
		remind = loopName == "memory"
	}
	return claudeHookOptions{
		Remind:  remind,
		Nudge:   p.hostOptions.nudge,
		Compact: p.hostOptions.compact,
	}
}

func (p claudeProjector) ensureStore(ctx context.Context, storeName string) error {
	mnemon, err := exec.LookPath("mnemon")
	if err != nil {
		return errors.New("mnemon binary not found in PATH; build or install it before setting a Claude Code memory store")
	}
	list := exec.CommandContext(ctx, mnemon, "store", "list")
	list.Dir = p.projectRoot
	list.Stderr = p.stderr
	output, err := list.Output()
	if err != nil {
		return fmt.Errorf("mnemon store list: %w", err)
	}
	if !storeListContains(output, storeName) {
		create := exec.CommandContext(ctx, mnemon, "store", "create", storeName)
		create.Dir = p.projectRoot
		create.Stdout = io.Discard
		create.Stderr = p.stderr
		if err := create.Run(); err != nil {
			return fmt.Errorf("mnemon store create %s: %w", storeName, err)
		}
	}
	set := exec.CommandContext(ctx, mnemon, "store", "set", storeName)
	set.Dir = p.projectRoot
	set.Stdout = io.Discard
	set.Stderr = p.stderr
	if err := set.Run(); err != nil {
		return fmt.Errorf("mnemon store set %s: %w", storeName, err)
	}
	return nil
}

func (p claudeProjector) writeLoopStatus(loop manifest.LoopManifest, binding manifest.BindingManifest) error {
	status := map[string]any{
		"schema_version":  2,
		"loop":            loop.Name,
		"host":            "claude-code",
		"phase":           "projected",
		"updated_at":      nowUTC(),
		"project_root":    p.projectRoot,
		"projection_path": binding.ProjectionPath,
		"state_path":      p.stateDir(loop.Name),
		"control_model":   nonNilMap(loop.ControlModel),
		"entity_profiles": nonNilMap(loop.EntityProfiles),
		"surfaces":        loop.Surfaces,
	}
	return p.writeJSON(pathJoin(p.stateDir(loop.Name), "status.json"), status, 0o644)
}

func (p claudeProjector) writeHostManifest(loop manifest.LoopManifest, binding manifest.BindingManifest, ownership projectionOwnership) error {
	manifestPath := p.resolve(p.hostManifestPath())
	manifest := hostProjectionManifest{
		SchemaVersion: 2,
		Host:          "claude-code",
		Loops:         map[string]hostManifestLoop{},
	}
	if data, err := os.ReadFile(manifestPath); err == nil && len(bytes.TrimSpace(data)) > 0 {
		if err := json.Unmarshal(data, &manifest); err != nil {
			return fmt.Errorf("parse host manifest %s: %w", p.hostManifestPath(), err)
		}
	}
	if manifest.Loops == nil {
		manifest.Loops = map[string]hostManifestLoop{}
	}
	manifest.SchemaVersion = 2
	manifest.Host = "claude-code"
	manifest.UpdatedAt = nowUTC()
	manifest.ProjectRoot = p.projectRoot
	manifest.MnemonDir = p.paths.mnemonDir
	if p.hostOptions.storeName != "" {
		manifest.Store = p.hostOptions.storeName
	} else {
		manifest.Store = "default"
	}
	manifest.Loops[loop.Name] = hostManifestLoop{
		LoopPath:     p.stateDir(loop.Name),
		LoopVersion:  loop.Version,
		StatePath:    p.stateDir(loop.Name),
		IntentPolicy: pathJoin(p.stateDir(loop.Name), "GUIDE.md"),
		StatusPath:   pathJoin(p.stateDir(loop.Name), "status.json"),
		Projection: map[string]any{
			"path":     binding.ProjectionPath,
			"surfaces": loop.Surfaces.Projection,
		},
		Reality: map[string]any{
			"surfaces": loop.Surfaces.Observation,
		},
		Reconcile: map[string]any{
			"actions": loop.ControlModel["reconcile"],
		},
		ControlModel:     nonNilMap(loop.ControlModel),
		EntityProfiles:   nonNilMap(loop.EntityProfiles),
		LifecycleMapping: binding.LifecycleMapping,
		Surfaces: map[string]string{
			"skills":  p.hostSkillsDir(loop.Name),
			"runtime": binding.RuntimeSurface,
		},
		Ownership: ownership,
	}
	return p.writeJSON(p.hostManifestPath(), manifest, 0o644)
}

func (p claudeProjector) removeCanonicalState(loop manifest.LoopManifest) error {
	stateDir := p.stateDir(loop.Name)
	switch loop.Name {
	case "memory":
		if p.hostOptions.purgeMemory {
			return os.RemoveAll(p.resolve(stateDir))
		}
		return p.removeCommonStateFiles(stateDir)
	case "skill":
		if p.hostOptions.purgeLibrary {
			return os.RemoveAll(p.resolve(stateDir))
		}
		if err := p.removeCommonStateFiles(stateDir); err != nil {
			return err
		}
		for _, dir := range []string{"reports", "proposals"} {
			_ = os.Remove(p.resolve(pathJoin(stateDir, dir)))
		}
		_ = os.Remove(p.resolve(stateDir))
	}
	return nil
}

func (p claudeProjector) loopOwnership(loop manifest.LoopManifest, binding manifest.BindingManifest) projectionOwnership {
	files := []string{
		pathJoin(p.stateDir(loop.Name), "GUIDE.md"),
		pathJoin(p.stateDir(loop.Name), "env.sh"),
		pathJoin(p.stateDir(loop.Name), "loop.json"),
		pathJoin(p.stateDir(loop.Name), "status.json"),
		pathJoin(binding.RuntimeSurface, "env.sh"),
		pathJoin(binding.RuntimeSurface, "GUIDE.md"),
	}
	if loop.Name == "memory" || loop.Name == "skill" {
		files = append(files, pathJoin(binding.ProjectionPath, "settings.json"))
	}
	for _, runtimeFile := range loop.Assets.RuntimeFiles {
		if loop.Name == "memory" {
			continue
		}
		files = append(files, pathJoin(p.stateDir(loop.Name), runtimeFile))
	}
	for _, skill := range loop.Assets.Skills {
		files = append(files, pathJoin(p.hostSkillsDir(loop.Name), skillID(skill), "SKILL.md"))
	}
	for _, subagent := range loop.Assets.Subagents {
		files = append(files, pathJoin(binding.ProjectionPath, "agents", agentFile(loop.Name, subagent)))
	}
	for phase := range loop.Assets.HookPrompts {
		hook := pathJoin(binding.ProjectionPath, "hooks", "mnemon-"+loop.Name, phase+".sh")
		if p.exists(hook) || p.hostHookExists(loop.Name, phase) {
			files = append(files, hook)
		}
	}
	sort.Strings(files)
	return projectionOwnership{
		Files: files,
		Dirs:  []string{binding.RuntimeSurface, pathJoin(binding.ProjectionPath, "hooks", "mnemon-"+loop.Name)},
	}
}

func (p claudeProjector) installedHostSkillsDir(loopName string, binding manifest.BindingManifest) string {
	envPath := pathJoin(binding.RuntimeSurface, "env.sh")
	envVar := "MNEMON_" + strings.ToUpper(strings.ReplaceAll(loopName, "-", "_")) + "_LOOP_HOST_SKILLS_DIR"
	if value, ok := p.readExportValue(envPath, envVar); ok {
		return value
	}
	return p.hostSkillsDir(loopName)
}

func (p claudeProjector) hostSkillsDir(loopName string) string {
	if p.hostOptions.hostSkillsDir != "" && loopName != "memory" {
		return filepath.ToSlash(p.hostOptions.hostSkillsDir)
	}
	return pathJoin(p.paths.configDir, "skills")
}
