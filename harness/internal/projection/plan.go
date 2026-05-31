package projection

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/mnemon-dev/mnemon/harness/internal/declaration"
)

type PlanOptions struct {
	DeclarationRoot string
	ProjectRoot     string
	Host            string
	Loops           []string
}

type Plan struct {
	SchemaVersion   int        `json:"schema_version"`
	Kind            string     `json:"kind"`
	Host            string     `json:"host"`
	Backend         string     `json:"backend"`
	DeclarationRoot string     `json:"declaration_root"`
	ProjectRoot     string     `json:"project_root"`
	Loops           []LoopPlan `json:"loops"`
}

type LoopPlan struct {
	Binding string       `json:"binding"`
	Loop    string       `json:"loop"`
	Actions []PlanAction `json:"actions"`
}

type PlanAction struct {
	Op     string `json:"op"`
	Source string `json:"source,omitempty"`
	Target string `json:"target,omitempty"`
	Detail string `json:"detail,omitempty"`
}

func BuildPlan(opts PlanOptions) (Plan, error) {
	if opts.DeclarationRoot == "" {
		opts.DeclarationRoot = "."
	}
	declarationRoot, err := filepath.Abs(opts.DeclarationRoot)
	if err != nil {
		return Plan{}, fmt.Errorf("resolve declaration root: %w", err)
	}
	if opts.ProjectRoot == "" {
		opts.ProjectRoot, err = os.Getwd()
		if err != nil {
			return Plan{}, fmt.Errorf("resolve project root: %w", err)
		}
	}
	projectRoot, err := filepath.Abs(opts.ProjectRoot)
	if err != nil {
		return Plan{}, fmt.Errorf("resolve project root: %w", err)
	}
	if opts.Host == "" {
		return Plan{}, errors.New("--host is required")
	}
	if _, err := declaration.ValidateHarness(declarationRoot); err != nil {
		return Plan{}, err
	}
	host, err := declaration.LoadHost(declarationRoot, opts.Host)
	if err != nil {
		return Plan{}, err
	}

	loops := append([]string(nil), opts.Loops...)
	if len(loops) == 0 {
		loops, err = declaration.LoopsForHost(declarationRoot, opts.Host)
		if err != nil {
			return Plan{}, err
		}
		if len(loops) == 0 {
			return Plan{}, fmt.Errorf("no bindings found for host %q", opts.Host)
		}
	}
	sort.Strings(loops)

	backend := "legacy-projector"
	if opts.Host == "codex" || opts.Host == "claude-code" {
		backend = "go-projector"
	}
	plan := Plan{
		SchemaVersion:   1,
		Kind:            "ProjectionPlan",
		Host:            opts.Host,
		Backend:         backend,
		DeclarationRoot: declarationRoot,
		ProjectRoot:     projectRoot,
	}
	for _, loopName := range loops {
		loop, err := declaration.LoadLoop(declarationRoot, loopName)
		if err != nil {
			return Plan{}, err
		}
		binding, err := declaration.LoadBinding(declarationRoot, opts.Host, loopName)
		if err != nil {
			return Plan{}, err
		}
		plan.Loops = append(plan.Loops, buildLoopPlan(declarationRoot, host, loop, binding))
	}
	return plan, nil
}

func WritePlanText(w io.Writer, plan Plan) error {
	if _, err := fmt.Fprintf(w, "Projection plan for host %s\n", plan.Host); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "Backend: %s\n", plan.Backend); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "Declaration root: %s\n", plan.DeclarationRoot); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "Project root: %s\n", plan.ProjectRoot); err != nil {
		return err
	}
	for _, loop := range plan.Loops {
		if _, err := fmt.Fprintf(w, "\n%s:\n", loop.Binding); err != nil {
			return err
		}
		for _, action := range loop.Actions {
			line := "- " + action.Op
			if action.Source != "" || action.Target != "" {
				line += ": "
				if action.Source != "" && action.Target != "" {
					line += action.Source
					line += " -> " + action.Target
				} else if action.Source != "" {
					line += action.Source
				} else {
					line += action.Target
				}
			}
			if action.Detail != "" {
				line += " (" + action.Detail + ")"
			}
			if _, err := fmt.Fprintln(w, line); err != nil {
				return err
			}
		}
	}
	return nil
}

func WritePlanJSON(w io.Writer, plan Plan) error {
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(plan)
}

func buildLoopPlan(root string, host declaration.HostManifest, loop declaration.LoopManifest, binding declaration.BindingManifest) LoopPlan {
	stateDir := path.Join(".mnemon", "harness", loop.Name)
	hostManifest := path.Join(".mnemon", "hosts", host.Name, "manifest.json")
	statusFile := path.Join(".mnemon", "harness", loop.Name, "status.json")
	loopDir := path.Join("harness", "loops", loop.Name)
	hostProjector := path.Join("harness", "hosts", host.Name, "projector.sh")

	actions := []PlanAction{
		{Op: "validate_declarations", Detail: "loop, host, and binding manifests"},
		{Op: "ensure_state_dir", Target: stateDir, Detail: "canonical loop runtime state"},
		{Op: "copy_canonical_asset", Source: path.Join(loopDir, "GUIDE.md"), Target: path.Join(stateDir, "GUIDE.md")},
		{Op: "copy_canonical_asset", Source: path.Join(loopDir, "env.sh"), Target: path.Join(stateDir, "env.sh")},
		{Op: "copy_canonical_asset", Source: path.Join(loopDir, "loop.json"), Target: path.Join(stateDir, "loop.json")},
	}
	for _, runtimeFile := range loop.Assets.RuntimeFiles {
		actions = append(actions, PlanAction{
			Op:     "copy_runtime_seed",
			Source: path.Join(loopDir, runtimeFile),
			Target: path.Join(stateDir, runtimeFile),
			Detail: "preserve existing target when projector policy requires it",
		})
	}
	actions = append(actions,
		PlanAction{Op: "write_runtime_env", Target: path.Join(binding.RuntimeSurface, "env.sh")},
		PlanAction{Op: "copy_runtime_guide", Source: path.Join(loopDir, loop.Assets.Guide), Target: path.Join(binding.RuntimeSurface, "GUIDE.md")},
	)
	for _, skill := range loop.Assets.Skills {
		actions = append(actions, PlanAction{
			Op:     "project_skill",
			Source: path.Join(loopDir, skill),
			Target: path.Join(binding.ProjectionPath, "skills", skillID(skill), "SKILL.md"),
		})
	}
	for _, subagent := range loop.Assets.Subagents {
		if hostHasProjection(host, "agents") {
			actions = append(actions, PlanAction{
				Op:     "project_agent",
				Source: path.Join(loopDir, subagent),
				Target: path.Join(binding.ProjectionPath, "agents", agentFile(loop.Name, subagent)),
			})
		} else {
			actions = append(actions, PlanAction{
				Op:     "skip_agent",
				Source: path.Join(loopDir, subagent),
				Detail: "host does not declare an agent projection surface",
			})
		}
	}
	actions = append(actions, phaseActions(root, host, loop, binding)...)
	actions = append(actions,
		PlanAction{Op: "write_loop_status", Target: statusFile},
		PlanAction{Op: "write_host_manifest", Target: hostManifest},
	)
	switch host.Name {
	case "codex":
		actions = append(actions, PlanAction{Op: "go_apply_backend", Detail: "declaration-driven Codex projection engine"})
	case "claude-code":
		actions = append(actions, PlanAction{Op: "go_apply_backend", Detail: "declaration-driven Claude Code projection engine"})
	default:
		actions = append(actions, PlanAction{Op: "legacy_apply_backend", Source: hostProjector, Detail: "temporary backend until Go projection engine replaces host projector scripts"})
	}
	return LoopPlan{
		Binding: binding.Name,
		Loop:    loop.Name,
		Actions: actions,
	}
}

func phaseActions(root string, host declaration.HostManifest, loop declaration.LoopManifest, binding declaration.BindingManifest) []PlanAction {
	var phases []string
	for phase := range loop.Assets.HookPrompts {
		phases = append(phases, phase)
	}
	sort.Strings(phases)
	var actions []PlanAction
	for _, phase := range phases {
		prompt := loop.Assets.HookPrompts[phase]
		hostHookRel := path.Join("harness", "hosts", host.Name, loop.Name, "hooks", phase+".sh")
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(hostHookRel))); err == nil {
			actions = append(actions, PlanAction{
				Op:     "project_native_hook",
				Source: hostHookRel,
				Target: path.Join(binding.ProjectionPath, "hooks", "mnemon-"+loop.Name, phase+".sh"),
				Detail: binding.LifecycleMapping[phase],
			})
			continue
		}
		actions = append(actions, PlanAction{
			Op:     "map_phase_prompt",
			Source: path.Join("harness", "loops", loop.Name, prompt),
			Detail: phase + " -> " + binding.LifecycleMapping[phase],
		})
	}
	if hostHasProjection(host, "settings.json") {
		actions = append(actions, PlanAction{
			Op:     "patch_host_settings",
			Target: path.Join(binding.ProjectionPath, "settings.json"),
			Detail: "register owned native hooks when projected",
		})
	} else if hostHasProjection(host, "hooks.json") {
		actions = append(actions, PlanAction{
			Op:     "patch_host_hooks",
			Target: path.Join(binding.ProjectionPath, "hooks.json"),
			Detail: "register owned native hooks when projected",
		})
	}
	return actions
}

func hostHasProjection(host declaration.HostManifest, needle string) bool {
	for _, surface := range host.Surfaces.Projection {
		if strings.Contains(surface, needle) {
			return true
		}
	}
	return false
}

func skillID(skillPath string) string {
	dir := path.Dir(skillPath)
	if dir == "." || dir == "/" {
		return strings.TrimSuffix(path.Base(skillPath), path.Ext(skillPath))
	}
	return path.Base(dir)
}

func agentFile(loopName, subagentPath string) string {
	base := strings.TrimSuffix(path.Base(subagentPath), path.Ext(subagentPath))
	switch loopName + "." + base {
	case "skill.curator":
		return "mnemon-skill-curator.md"
	default:
		return "mnemon-" + base + ".md"
	}
}
