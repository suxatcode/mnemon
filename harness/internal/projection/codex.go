package projection

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/mnemon-dev/mnemon/harness/internal/declaration"
	"github.com/mnemon-dev/mnemon/harness/internal/lifecycle/coordination"
	"github.com/mnemon-dev/mnemon/harness/internal/lifecycle/eventlog"
	"github.com/mnemon-dev/mnemon/harness/internal/lifecycle/profile"
)

// profileFragmentFile is the scoped profile fragment the projector writes onto a
// host's runtime surface so the next run pulls the durable, reviewed profile
// entries targeted at that host+loop (the pull side of the memory loop).
const profileFragmentFile = "PROFILE.json"

// coordinationFragmentFile is the host-scoped coordination fragment the projector
// writes onto a host's runtime surface so the next run inherits its current
// claims, group membership, conflicts, and merge decisions.
const coordinationFragmentFile = "COORDINATION.json"

// scopedCoordinationFragment derives the coordination topology and filters it to
// what a host needs: its owned tasks (including merge decisions that joined its
// work elsewhere), the groups it belongs to, and conflicts / merge candidates
// touching its tasks. ok is false when nothing concerns this host. Read-only.
func scopedCoordinationFragment(projectRoot, host string) (coordination.View, bool, error) {
	store, err := eventlog.New(projectRoot)
	if err != nil {
		return coordination.View{}, false, err
	}
	events, _ := store.ReadAll() // best-effort over the readable log
	full := coordination.DeriveView(events)
	host = strings.TrimSpace(host)

	frag := coordination.View{}
	owned := map[string]bool{}
	for _, t := range full.Tasks {
		if t.Owner == host {
			frag.Tasks = append(frag.Tasks, t)
			owned[t.ID] = true
		}
	}
	for _, g := range full.Groups {
		for _, m := range g.Members {
			if m == host {
				frag.Groups = append(frag.Groups, g)
				break
			}
		}
	}
	for _, c := range full.Conflicts {
		for _, tk := range c.Between {
			if owned[tk] {
				frag.Conflicts = append(frag.Conflicts, c)
				break
			}
		}
	}
	for _, mc := range full.MergeCandidates {
		for _, tk := range mc.Tasks {
			if owned[tk] {
				frag.MergeCandidates = append(frag.MergeCandidates, mc)
				break
			}
		}
	}
	if len(frag.Tasks)+len(frag.Groups)+len(frag.Conflicts) == 0 {
		return coordination.View{}, false, nil
	}
	return frag, true, nil
}

type CodexOptions struct {
	DeclarationRoot string
	ProjectRoot     string
	Loops           []string
	HostArgs        []string
	Stdout          io.Writer
	Stderr          io.Writer
}

type codexHostOptions struct {
	global            bool
	configDir         string
	configDirExplicit bool
	storeName         string
	hostSkillsDir     string
	dryRun            bool
	purgeMemory       bool
	purgeLibrary      bool
}

type codexProjector struct {
	projectorCore
	hostOptions codexHostOptions
}

type hostProjectionManifest struct {
	SchemaVersion int                         `json:"schema_version"`
	Host          string                      `json:"host"`
	UpdatedAt     string                      `json:"updated_at,omitempty"`
	ProjectRoot   string                      `json:"project_root,omitempty"`
	MnemonDir     string                      `json:"mnemon_dir,omitempty"`
	Store         string                      `json:"store,omitempty"`
	Loops         map[string]hostManifestLoop `json:"loops,omitempty"`
}

type hostManifestLoop struct {
	LoopPath         string              `json:"loop_path"`
	LoopVersion      string              `json:"loop_version,omitempty"`
	StatePath        string              `json:"state_path"`
	IntentPolicy     string              `json:"intent_policy"`
	StatusPath       string              `json:"status_path"`
	Projection       map[string]any      `json:"projection"`
	Reality          map[string]any      `json:"reality"`
	Reconcile        map[string]any      `json:"reconcile"`
	ControlModel     map[string]any      `json:"control_model,omitempty"`
	EntityProfiles   map[string]any      `json:"entity_profiles,omitempty"`
	LifecycleMapping map[string]string   `json:"lifecycle_mapping"`
	Surfaces         map[string]string   `json:"surfaces"`
	Ownership        projectionOwnership `json:"ownership"`
}

type projectionOwnership struct {
	Files []string `json:"files,omitempty"`
	Dirs  []string `json:"dirs,omitempty"`
}

func RunCodexProjector(ctx context.Context, action string, opts CodexOptions) error {
	projector, loops, err := newCodexProjector(action, opts)
	if err != nil {
		return err
	}
	for _, loopName := range loops {
		loop, err := declaration.LoadLoop(projector.declarationRoot, loopName)
		if err != nil {
			return err
		}
		binding, err := declaration.LoadBinding(projector.declarationRoot, "codex", loopName)
		if err != nil {
			return err
		}
		switch action {
		case "install":
			if projector.hostOptions.dryRun {
				if _, err := projector.diffLoop(loop, binding, true); err != nil {
					return fmt.Errorf("dry-run install codex/%s: %w", loopName, err)
				}
				continue
			}
			if err := projector.installLoop(ctx, loop, binding); err != nil {
				return fmt.Errorf("install codex/%s: %w", loopName, err)
			}
		case "diff":
			if _, err := projector.diffLoop(loop, binding, false); err != nil {
				return fmt.Errorf("diff codex/%s: %w", loopName, err)
			}
		case "status":
			if err := projector.statusLoop(loop); err != nil {
				return fmt.Errorf("status codex/%s: %w", loopName, err)
			}
		case "uninstall":
			if err := projector.uninstallLoop(loop); err != nil {
				return fmt.Errorf("uninstall codex/%s: %w", loopName, err)
			}
		default:
			return fmt.Errorf("unsupported Codex projector action: %s", action)
		}
	}
	return nil
}

func newCodexProjector(action string, opts CodexOptions) (codexProjector, []string, error) {
	if opts.DeclarationRoot == "" {
		opts.DeclarationRoot = "."
	}
	declarationRoot, err := filepath.Abs(opts.DeclarationRoot)
	if err != nil {
		return codexProjector{}, nil, fmt.Errorf("resolve declaration root: %w", err)
	}
	if opts.ProjectRoot == "" {
		opts.ProjectRoot, err = os.Getwd()
		if err != nil {
			return codexProjector{}, nil, fmt.Errorf("resolve project root: %w", err)
		}
	}
	projectRoot, err := filepath.Abs(opts.ProjectRoot)
	if err != nil {
		return codexProjector{}, nil, fmt.Errorf("resolve project root: %w", err)
	}
	hostOptions, err := parseCodexHostOptions(opts.HostArgs)
	if err != nil {
		return codexProjector{}, nil, err
	}
	if opts.Stdout == nil {
		opts.Stdout = io.Discard
	}
	if opts.Stderr == nil {
		opts.Stderr = io.Discard
	}
	if _, err := declaration.ValidateHarness(declarationRoot); err != nil {
		return codexProjector{}, nil, err
	}
	loops := append([]string(nil), opts.Loops...)
	if len(loops) == 0 {
		if action != "status" && action != "diff" {
			return codexProjector{}, nil, errors.New("at least one --loop is required")
		}
		loops, err = declaration.LoopsForHost(declarationRoot, "codex")
		if err != nil {
			return codexProjector{}, nil, err
		}
		if len(loops) == 0 {
			return codexProjector{}, nil, errors.New("no bindings found for host \"codex\"")
		}
	}
	sort.Strings(loops)

	return codexProjector{
		projectorCore: projectorCore{
			host:            "codex",
			declarationRoot: declarationRoot,
			projectRoot:     projectRoot,
			paths:           codexProjectorPaths(hostOptions),
			stdout:          opts.Stdout,
			stderr:          opts.Stderr,
		},
		hostOptions: hostOptions,
	}, loops, nil
}

func parseCodexHostOptions(args []string) (codexHostOptions, error) {
	parsed := codexHostOptions{configDir: ".codex"}
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
		case "--dry-run":
			parsed.dryRun = true
		case "--purge-memory":
			parsed.purgeMemory = true
		case "--purge-library":
			parsed.purgeLibrary = true
		default:
			return parsed, fmt.Errorf("unsupported Codex host option: %s", arg)
		}
	}
	return parsed, nil
}

func codexProjectorPaths(opts codexHostOptions) corePaths {
	if opts.global && !opts.configDirExplicit {
		home := os.Getenv("HOME")
		configDir := filepath.Join(home, ".codex")
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

func (p codexProjector) installLoop(ctx context.Context, loop declaration.LoopManifest, binding declaration.BindingManifest) error {
	if err := p.copyCommonCanonicalAssets(loop); err != nil {
		return err
	}
	if err := p.prepareLoopState(loop); err != nil {
		return err
	}
	if err := p.writeRuntimeEnv(loop, binding); err != nil {
		return err
	}
	if err := p.copyFile(p.loopAsset(loop, loop.Assets.Guide), p.displayJoin(binding.RuntimeSurface, "GUIDE.md"), 0o644); err != nil {
		return err
	}
	if err := p.projectProfileFragment(loop, binding); err != nil {
		return err
	}
	if err := p.projectCoordinationFragment(loop, binding); err != nil {
		return err
	}
	if err := p.applyProjectionEnvelope(loop, binding); err != nil {
		return err
	}
	if err := p.projectSkills(loop, binding); err != nil {
		return err
	}
	if err := p.projectHooks(loop, binding); err != nil {
		return err
	}
	if p.codexHooksEnabled(loop.Name) {
		if err := p.patchHooks(loop.Name); err != nil {
			return err
		}
	}
	if loop.Name == "memory" && p.hostOptions.storeName != "" {
		if err := p.ensureStore(ctx, p.hostOptions.storeName); err != nil {
			return err
		}
	}
	ownership := p.loopOwnership(loop, binding)
	if err := p.writeHostManifest(loop, binding, ownership); err != nil {
		return err
	}
	if err := p.writeLoopStatus(loop, binding); err != nil {
		return err
	}
	p.printf("Installed Mnemon %s loop for Codex.\n", loop.Name)
	p.printf("Config:       %s\n", p.paths.configDir)
	p.printf("State:        %s\n", p.stateDir(loop.Name))
	if hostSkills := p.hostSkillsDir(loop.Name); hostSkills != "" {
		p.printf("Host skills:  %s\n", hostSkills)
	}
	return nil
}

// scopedProfileFragment loads the durable profile and filters it to the entries
// projected to (host, loop) via their projection_targets, reusing the store's
// FilterEntries. ok is false when there is no profile yet or no entry targets
// this host+loop, so the caller writes nothing. Read-only on the profile store.
func scopedProfileFragment(projectRoot, host, loop string) (profile.Profile, bool, error) {
	store, err := profile.New(projectRoot)
	if err != nil {
		return profile.Profile{}, false, err
	}
	prof, err := store.Load("")
	if errors.Is(err, profile.ErrProfileNotFound) {
		return profile.Profile{}, false, nil
	}
	if err != nil {
		return profile.Profile{}, false, err
	}
	fragment := store.FilterEntries(prof, host, loop)
	if len(fragment.Entries) == 0 {
		return profile.Profile{}, false, nil
	}
	return fragment, true, nil
}

// projectProfileFragment writes the host+loop-scoped profile fragment onto the
// Codex runtime surface so the next Codex run inherits the applied profile. It is
// a point-in-time snapshot derived from canonical profile state (data, not a
// static owned asset), so uninstall removes it with the runtime surface.
func (p codexProjector) projectProfileFragment(loop declaration.LoopManifest, binding declaration.BindingManifest) error {
	fragment, ok, err := scopedProfileFragment(p.projectRoot, "codex", loop.Name)
	if err != nil || !ok {
		return err
	}
	ref := p.displayJoin(binding.RuntimeSurface, profileFragmentFile)
	// Payload only — the projection ACT's provenance (projection.applied) is emitted
	// once by applyProjectionEnvelope over the combined context, not per fragment.
	return p.writeJSON(ref, fragment, 0o644)
}

// projectCoordinationFragment writes the host-scoped coordination fragment onto
// the Codex runtime surface so the next run inherits its claims, group
// membership, conflicts, and merge decisions. A point-in-time snapshot of the
// event-sourced topology; removed with the runtime surface on uninstall.
func (p codexProjector) projectCoordinationFragment(loop declaration.LoopManifest, binding declaration.BindingManifest) error {
	fragment, ok, err := scopedCoordinationFragment(p.projectRoot, "codex")
	if err != nil || !ok {
		return err
	}
	ref := p.displayJoin(binding.RuntimeSurface, coordinationFragmentFile)
	return p.writeJSON(ref, fragment, 0o644)
}

func (p codexProjector) statusLoop(loop declaration.LoopManifest) error {
	p.printf("Codex %s:\n", loop.Name)
	p.printf("  config:   %s\n", p.paths.configDir)
	p.printf("  state:    %s\n", p.stateDir(loop.Name))
	if p.exists(p.hostManifestPath()) {
		p.printf("  manifest: %s\n", p.hostManifestPath())
	} else {
		p.printf("  manifest: missing\n")
	}
	statusPath := p.displayJoin(p.stateDir(loop.Name), "status.json")
	if p.exists(statusPath) {
		p.printf("  status:   %s\n", statusPath)
	} else {
		p.printf("  status:   missing\n")
	}
	if p.exists(p.stateDir(loop.Name)) {
		p.printf("  loop:   installed\n")
	} else {
		p.printf("  loop:   missing\n")
	}
	return nil
}

func (p codexProjector) uninstallLoop(loop declaration.LoopManifest) error {
	binding, err := declaration.LoadBinding(p.declarationRoot, "codex", loop.Name)
	if err != nil {
		return err
	}
	if p.codexHooksEnabled(loop.Name) {
		if err := p.unpatchHooks(loop.Name); err != nil {
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
		if err := os.RemoveAll(p.resolve(p.displayJoin(hostSkillsDir, skillID(skill)))); err != nil {
			return err
		}
	}
	if err := os.RemoveAll(p.resolve(p.displayJoin(p.paths.configDir, "hooks", "mnemon-"+loop.Name))); err != nil {
		return err
	}
	if err := os.RemoveAll(p.resolve(binding.RuntimeSurface)); err != nil {
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

func (p codexProjector) copyCommonCanonicalAssets(loop declaration.LoopManifest) error {
	for _, asset := range []struct {
		rel  string
		name string
		mode os.FileMode
	}{
		{rel: loop.Assets.Guide, name: "GUIDE.md", mode: 0o644},
		{rel: loop.Assets.Env, name: "env.sh", mode: 0o755},
		{rel: "loop.json", name: "loop.json", mode: 0o644},
	} {
		if err := p.copyFile(p.loopAsset(loop, asset.rel), p.displayJoin(p.stateDir(loop.Name), asset.name), asset.mode); err != nil {
			return err
		}
	}
	return nil
}

func (p codexProjector) prepareLoopState(loop declaration.LoopManifest) error {
	switch loop.Name {
	case "memory":
		for _, runtimeFile := range loop.Assets.RuntimeFiles {
			if err := p.copyFileIfMissing(p.loopAsset(loop, runtimeFile), p.displayJoin(p.stateDir(loop.Name), runtimeFile), 0o644); err != nil {
				return err
			}
		}
	case "skill":
		for _, dir := range []string{"skills/active", "skills/stale", "skills/archived", "proposals", "reports"} {
			if err := os.MkdirAll(p.resolve(p.displayJoin(p.stateDir(loop.Name), dir)), 0o755); err != nil {
				return fmt.Errorf("mkdir %s: %w", dir, err)
			}
		}
	case "eval":
		for _, dir := range []string{"scratch", "candidates", "reports", "artifacts", "retired", "scenarios", "suites", "rubrics"} {
			if err := os.MkdirAll(p.resolve(p.displayJoin(p.stateDir(loop.Name), dir)), 0o755); err != nil {
				return fmt.Errorf("mkdir %s: %w", dir, err)
			}
		}
		for _, runtimeFile := range loop.Assets.RuntimeFiles {
			if err := p.copyFile(p.loopAsset(loop, runtimeFile), p.displayJoin(p.stateDir(loop.Name), runtimeFile), 0o644); err != nil {
				return err
			}
		}
	case "goal":
		for _, dir := range []string{
			p.displayJoin(p.paths.mnemonDir, "harness/goals"),
			p.displayJoin(p.paths.mnemonDir, "harness/status/goals"),
		} {
			if err := os.MkdirAll(p.resolve(dir), 0o755); err != nil {
				return fmt.Errorf("mkdir %s: %w", dir, err)
			}
		}
	default:
		for _, runtimeFile := range loop.Assets.RuntimeFiles {
			if err := p.copyFileIfMissing(p.loopAsset(loop, runtimeFile), p.displayJoin(p.stateDir(loop.Name), runtimeFile), 0o644); err != nil {
				return err
			}
		}
	}
	return nil
}

func (p codexProjector) writeRuntimeEnv(loop declaration.LoopManifest, binding declaration.BindingManifest) error {
	return p.writeFile(p.displayJoin(binding.RuntimeSurface, "env.sh"), p.runtimeEnvContent(loop, binding), 0o755)
}

func (p codexProjector) runtimeEnvContent(loop declaration.LoopManifest, binding declaration.BindingManifest) []byte {
	envName := loopEnvName(loop.Name)
	loopDirVar := loopDirVarName(loop.Name)
	stateDir := p.stateDir(loop.Name)
	lines := []string{
		"#!/usr/bin/env bash",
		exportLine(envName, p.displayJoin(stateDir, "env.sh")),
		exportLine(loopDirVar, stateDir),
	}
	switch loop.Name {
	case "memory":
		lines = append(lines, `export MNEMON_MEMORY_LOOP_MAX_NON_EMPTY_LINES="${MNEMON_MEMORY_LOOP_MAX_NON_EMPTY_LINES:-200}"`)
	case "skill":
		hostSkillsDir := p.hostSkillsDir(loop.Name)
		lines = append(lines,
			exportLine("MNEMON_SKILL_LOOP_LIBRARY_DIR", p.displayJoin(stateDir, "skills")),
			exportLine("MNEMON_SKILL_LOOP_ACTIVE_DIR", p.displayJoin(stateDir, "skills/active")),
			exportLine("MNEMON_SKILL_LOOP_STALE_DIR", p.displayJoin(stateDir, "skills/stale")),
			exportLine("MNEMON_SKILL_LOOP_ARCHIVED_DIR", p.displayJoin(stateDir, "skills/archived")),
			exportLine("MNEMON_SKILL_LOOP_USAGE_FILE", p.displayJoin(stateDir, "skills/.usage.jsonl")),
			exportLine("MNEMON_SKILL_LOOP_PROPOSALS_DIR", p.displayJoin(stateDir, "proposals")),
			exportLine("MNEMON_SKILL_LOOP_HOST_SKILLS_DIR", hostSkillsDir),
			`export MNEMON_SKILL_LOOP_REVIEW_MIN_EVENTS="${MNEMON_SKILL_LOOP_REVIEW_MIN_EVENTS:-20}"`,
			`export MNEMON_SKILL_LOOP_PROTECTED_SKILLS="${MNEMON_SKILL_LOOP_PROTECTED_SKILLS:-skill-observe,skill-curate,skill-author,skill-manage,memory-get,memory-set,mnemon-goal}"`,
		)
	case "eval":
		hostSkillsDir := p.hostSkillsDir(loop.Name)
		lines = append(lines,
			exportLine("MNEMON_EVAL_LOOP_SCRATCH_DIR", p.displayJoin(stateDir, "scratch")),
			exportLine("MNEMON_EVAL_LOOP_CANDIDATES_DIR", p.displayJoin(stateDir, "candidates")),
			exportLine("MNEMON_EVAL_LOOP_REPORTS_DIR", p.displayJoin(stateDir, "reports")),
			exportLine("MNEMON_EVAL_LOOP_ARTIFACTS_DIR", p.displayJoin(stateDir, "artifacts")),
			exportLine("MNEMON_EVAL_LOOP_RETIRED_DIR", p.displayJoin(stateDir, "retired")),
			exportLine("MNEMON_EVAL_LOOP_SCENARIOS_DIR", p.displayJoin(stateDir, "scenarios")),
			exportLine("MNEMON_EVAL_LOOP_SUITES_DIR", p.displayJoin(stateDir, "suites")),
			exportLine("MNEMON_EVAL_LOOP_RUBRICS_DIR", p.displayJoin(stateDir, "rubrics")),
			exportLine("MNEMON_EVAL_LOOP_HOST_SKILLS_DIR", hostSkillsDir),
			`export MNEMON_EVAL_LOOP_DEFAULT_HOST="${MNEMON_EVAL_LOOP_DEFAULT_HOST:-codex}"`,
			`export MNEMON_EVAL_LOOP_DEFAULT_SUITE="${MNEMON_EVAL_LOOP_DEFAULT_SUITE:-smoke}"`,
		)
	case "goal":
		hostSkillsDir := p.hostSkillsDir(loop.Name)
		lines = append(lines,
			exportLine("MNEMON_GOAL_LOOP_ROOT", p.projectRoot),
			exportLine("MNEMON_GOAL_LOOP_GOALS_DIR", p.displayJoin(p.paths.mnemonDir, "harness/goals")),
			exportLine("MNEMON_GOAL_LOOP_STATUS_DIR", p.displayJoin(p.paths.mnemonDir, "harness/status/goals")),
			exportLine("MNEMON_GOAL_LOOP_HOST_SKILLS_DIR", hostSkillsDir),
		)
	}
	content := strings.Join(lines, "\n") + "\n"
	return []byte(content)
}

func (p codexProjector) projectSkills(loop declaration.LoopManifest, binding declaration.BindingManifest) error {
	hostSkillsDir := p.hostSkillsDir(loop.Name)
	for _, skill := range loop.Assets.Skills {
		target := p.displayJoin(hostSkillsDir, skillID(skill), "SKILL.md")
		content, err := p.projectedSkillContent(loop, binding, skill)
		if err != nil {
			return err
		}
		if err := p.writeFile(target, content, 0o644); err != nil {
			return err
		}
	}
	return nil
}

func (p codexProjector) projectedSkillContent(loop declaration.LoopManifest, binding declaration.BindingManifest, skill string) ([]byte, error) {
	content, err := os.ReadFile(p.loopAsset(loop, skill))
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", skill, err)
	}
	note := runtimeNote(loopDirVarName(loop.Name), p.displayJoin(binding.RuntimeSurface, "env.sh"), p.stateDir(loop.Name))
	return append(content, []byte(note)...), nil
}

func (p codexProjector) projectHooks(loop declaration.LoopManifest, binding declaration.BindingManifest) error {
	for phase := range loop.Assets.HookPrompts {
		source := filepath.Join(p.declarationRoot, "harness", "hosts", "codex", loop.Name, "hooks", phase+".sh")
		if _, err := os.Stat(source); os.IsNotExist(err) {
			continue
		} else if err != nil {
			return fmt.Errorf("stat hook %s: %w", phase, err)
		}
		target := p.displayJoin(binding.ProjectionPath, "hooks", "mnemon-"+loop.Name, phase+".sh")
		if err := p.copyFile(source, target, 0o755); err != nil {
			return err
		}
	}
	return nil
}

func (p codexProjector) patchHooks(loopName string) error {
	return patchCodexHooks(p.resolve(p.displayJoin(p.paths.configDir, "hooks.json")), p.paths.configDir, "mnemon-"+loopName, p.hookOptions(loopName))
}

func (p codexProjector) unpatchHooks(loopName string) error {
	return unpatchCodexHooks(p.resolve(p.displayJoin(p.paths.configDir, "hooks.json")), "mnemon-"+loopName)
}

func (p codexProjector) hookOptions(loopName string) codexHookOptions {
	switch loopName {
	case "memory":
		return codexHookOptions{Remind: true, Nudge: true, Compact: true}
	case "skill":
		return codexHookOptions{Nudge: true, Compact: true}
	case "goal":
		return codexHookOptions{Remind: true, Nudge: true, Compact: true}
	case "eval":
		return codexHookOptions{Remind: true, Nudge: true, Compact: true}
	default:
		return codexHookOptions{}
	}
}

func (p codexProjector) codexHooksEnabled(loopName string) bool {
	return loopName == "memory" || loopName == "skill" || loopName == "goal" || loopName == "eval"
}

func (p codexProjector) ensureStore(ctx context.Context, storeName string) error {
	mnemon, err := exec.LookPath("mnemon")
	if err != nil {
		return errors.New("mnemon binary not found in PATH; build or install it before setting a Codex memory store")
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

func storeListContains(output []byte, storeName string) bool {
	scanner := bufio.NewScanner(bytes.NewReader(output))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		line = strings.TrimLeft(line, "* ")
		if strings.TrimSpace(line) == storeName {
			return true
		}
	}
	return false
}

func (p codexProjector) writeLoopStatus(loop declaration.LoopManifest, binding declaration.BindingManifest) error {
	status := map[string]any{
		"schema_version":  2,
		"loop":            loop.Name,
		"host":            "codex",
		"phase":           "projected",
		"updated_at":      nowUTC(),
		"project_root":    p.projectRoot,
		"projection_path": p.paths.configDir,
		"state_path":      p.stateDir(loop.Name),
		"control_model":   nonNilMap(loop.ControlModel),
		"entity_profiles": nonNilMap(loop.EntityProfiles),
		"surfaces":        loop.Surfaces,
	}
	return p.writeJSON(p.displayJoin(p.stateDir(loop.Name), "status.json"), status, 0o644)
}

func (p codexProjector) writeHostManifest(loop declaration.LoopManifest, binding declaration.BindingManifest, ownership projectionOwnership) error {
	manifestPath := p.resolve(p.hostManifestPath())
	manifest := hostProjectionManifest{
		SchemaVersion: 2,
		Host:          "codex",
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
	manifest.Host = "codex"
	manifest.UpdatedAt = nowUTC()
	manifest.ProjectRoot = p.projectRoot
	manifest.MnemonDir = p.paths.mnemonDir
	if p.hostOptions.storeName != "" {
		manifest.Store = p.hostOptions.storeName
	} else {
		manifest.Store = "default"
	}
	surfaces := map[string]string{
		"skills":  p.hostSkillsDir(loop.Name),
		"runtime": binding.RuntimeSurface,
	}
	if p.codexHooksEnabled(loop.Name) {
		surfaces["hooks"] = p.displayJoin(binding.ProjectionPath, "hooks", "mnemon-"+loop.Name)
	}
	manifest.Loops[loop.Name] = hostManifestLoop{
		LoopPath:    p.stateDir(loop.Name),
		LoopVersion: loop.Version,
		StatePath:   p.stateDir(loop.Name),
		IntentPolicy: p.displayJoin(
			p.stateDir(loop.Name),
			"GUIDE.md",
		),
		StatusPath: p.displayJoin(p.stateDir(loop.Name), "status.json"),
		Projection: map[string]any{
			"path":     p.paths.configDir,
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
		Surfaces:         surfaces,
		Ownership:        ownership,
	}
	return p.writeJSON(p.hostManifestPath(), manifest, 0o644)
}

func (p codexProjector) removeCanonicalState(loop declaration.LoopManifest) error {
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
			_ = os.Remove(p.resolve(p.displayJoin(stateDir, dir)))
		}
		_ = os.Remove(p.resolve(stateDir))
	case "eval":
		for _, dir := range []string{"scenarios", "suites", "rubrics"} {
			if err := os.RemoveAll(p.resolve(p.displayJoin(stateDir, dir))); err != nil {
				return err
			}
		}
		if err := p.removeCommonStateFiles(stateDir); err != nil {
			return err
		}
		for _, dir := range []string{"retired", "artifacts", "reports", "candidates", "scratch"} {
			_ = os.Remove(p.resolve(p.displayJoin(stateDir, dir)))
		}
		_ = os.Remove(p.resolve(stateDir))
	case "goal":
		if err := p.removeCommonStateFiles(stateDir); err != nil {
			return err
		}
		_ = os.Remove(p.resolve(stateDir))
	default:
		return p.removeCommonStateFiles(stateDir)
	}
	return nil
}

func (p codexProjector) installedHostSkillsDir(loopName string, binding declaration.BindingManifest) string {
	envPath := p.displayJoin(binding.RuntimeSurface, "env.sh")
	envVar := "MNEMON_" + strings.ToUpper(strings.ReplaceAll(loopName, "-", "_")) + "_LOOP_HOST_SKILLS_DIR"
	if value, ok := p.readExportValue(envPath, envVar); ok {
		return value
	}
	return p.hostSkillsDir(loopName)
}

func (p codexProjector) removeGeneratedSkillViews(hostSkillsDir string) error {
	entries, err := os.ReadDir(p.resolve(hostSkillsDir))
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read host skills dir: %w", err)
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		skillDir := p.displayJoin(hostSkillsDir, entry.Name())
		marker := p.displayJoin(skillDir, ".mnemon-skill-generated")
		if _, err := os.Stat(p.resolve(marker)); os.IsNotExist(err) {
			continue
		} else if err != nil {
			return fmt.Errorf("stat generated skill marker: %w", err)
		}
		if err := os.RemoveAll(p.resolve(skillDir)); err != nil {
			return fmt.Errorf("remove generated skill view: %w", err)
		}
	}
	return nil
}

func (p codexProjector) loopOwnership(loop declaration.LoopManifest, binding declaration.BindingManifest) projectionOwnership {
	files := []string{
		p.displayJoin(p.stateDir(loop.Name), "GUIDE.md"),
		p.displayJoin(p.stateDir(loop.Name), "env.sh"),
		p.displayJoin(p.stateDir(loop.Name), "loop.json"),
		p.displayJoin(p.stateDir(loop.Name), "status.json"),
		p.displayJoin(binding.RuntimeSurface, "env.sh"),
		p.displayJoin(binding.RuntimeSurface, "GUIDE.md"),
	}
	for _, runtimeFile := range loop.Assets.RuntimeFiles {
		if loop.Name == "memory" {
			continue
		}
		files = append(files, p.displayJoin(p.stateDir(loop.Name), runtimeFile))
	}
	for _, skill := range loop.Assets.Skills {
		files = append(files, p.displayJoin(p.hostSkillsDir(loop.Name), skillID(skill), "SKILL.md"))
	}
	if p.codexHooksEnabled(loop.Name) {
		files = append(files, p.displayJoin(binding.ProjectionPath, "hooks.json"))
	}
	for phase := range loop.Assets.HookPrompts {
		hook := p.displayJoin(binding.ProjectionPath, "hooks", "mnemon-"+loop.Name, phase+".sh")
		if p.exists(hook) || p.hostHookExists(loop.Name, phase) {
			files = append(files, hook)
		}
	}
	dirs := []string{binding.RuntimeSurface}
	if p.codexHooksEnabled(loop.Name) {
		dirs = append(dirs, p.displayJoin(binding.ProjectionPath, "hooks", "mnemon-"+loop.Name))
	}
	sort.Strings(files)
	sort.Strings(dirs)
	return projectionOwnership{Files: files, Dirs: dirs}
}

func (p codexProjector) hostSkillsDir(loopName string) string {
	if p.hostOptions.hostSkillsDir != "" && loopName != "memory" {
		return filepath.ToSlash(p.hostOptions.hostSkillsDir)
	}
	return p.displayJoin(p.paths.configDir, "skills")
}

func runtimeNote(loopDirVar, runtimeFile, canonicalLoopDir string) string {
	return fmt.Sprintf(`

## Codex Projection

This skill is projected by the Mnemon Codex host adapter.

- Canonical loop directory: %s
- Runtime env file: %s
- Before following the procedure, source the runtime env file when the expected
  environment variables are not already exported.
- The canonical loop directory is the location for GUIDE.md, runtime files,
  and loop state. Do not look for loop-owned state in the workspace root.
- If %s is not already exported, use the canonical loop directory above.
`, markdownCode(canonicalLoopDir), markdownCode(runtimeFile), markdownCode(loopDirVar))
}

func loopEnvName(loopName string) string {
	return "MNEMON_" + strings.ToUpper(strings.ReplaceAll(loopName, "-", "_")) + "_LOOP_ENV"
}

func loopDirVarName(loopName string) string {
	return "MNEMON_" + strings.ToUpper(strings.ReplaceAll(loopName, "-", "_")) + "_LOOP_DIR"
}

func exportLine(key, value string) string {
	return fmt.Sprintf("export %s=\"%s\"", key, escapeDoubleQuoted(value))
}

func escapeDoubleQuoted(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, `"`, `\"`)
	value = strings.ReplaceAll(value, "$", `\$`)
	value = strings.ReplaceAll(value, "`", "\\`")
	return value
}

func markdownCode(value string) string {
	return "`" + strings.ReplaceAll(value, "`", "\\`") + "`"
}

func nonNilMap(value map[string]any) map[string]any {
	if value == nil {
		return map[string]any{}
	}
	return value
}

func nowUTC() string {
	return time.Now().UTC().Truncate(time.Second).Format(time.RFC3339)
}
