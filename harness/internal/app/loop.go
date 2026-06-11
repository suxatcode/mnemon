package app

import (
	"context"
	"fmt"
	"io"
	"os"
	"sort"

	"github.com/mnemon-dev/mnemon/harness/internal/assets"
	"github.com/mnemon-dev/mnemon/harness/internal/capability"
	"github.com/mnemon-dev/mnemon/harness/internal/hostsurface"
	"github.com/mnemon-dev/mnemon/harness/internal/kernel"
	"github.com/mnemon-dev/mnemon/harness/internal/manifest"
)

// LoopValidate validates the embedded harness loop/host/binding manifests unconditionally, then —
// when root names an external tree carrying its own loops/hosts/bindings — validates that too (the
// union). A root with no harness assets (the common case, including the repo root after the assets
// moved under internal/assets) contributes nothing, so the validation passes.
func (h *Harness) LoopValidate() ([]string, error) {
	result, err := manifest.ValidateFS(assets.FS)
	if err != nil {
		return nil, err
	}
	lines := result.Lines
	// Stage-3: hooks are generated; validate renders for every embedded (host, loop) pair so a
	// broken intents/mechanics/fragment combination fails HERE, not at install time.
	hookHosts, hookLoops, err := hostsurface.EmbeddedHookUniverse()
	if err != nil {
		return nil, err
	}
	hookLines, err := hostsurface.ValidateGeneratedHooks(hookHosts, hookLoops)
	if err != nil {
		return nil, err
	}
	lines = append(lines, hookLines...)
	if h.root != "" {
		// Manifest-TREE validation (a loops/hosts/bindings tree at the root) — distinct from the
		// .mnemon/loops external CAPABILITY packages validated below.
		external, err := manifest.ValidateFS(os.DirFS(h.root))
		if err != nil {
			return nil, err
		}
		lines = append(lines, external.Lines...)
	}
	// External capability packages: run the SAME fail-closed resolution boot uses (symlink screen
	// + LoadExternal + four-axis shadowing merge), so a package that would refuse `local run`
	// fails validate too. One OK line per package — the v1 source label (status integration is
	// explicitly deferred).
	merged, err := capability.ResolveCatalog(h.root, kernel.DefaultSchemaGuard().Required)
	if err != nil {
		return nil, err
	}
	var externalNames []string
	for name := range merged {
		if _, embedded := capability.Builtins[name]; !embedded {
			externalNames = append(externalNames, name)
		}
	}
	sort.Strings(externalNames)
	for _, name := range externalNames {
		lines = append(lines, fmt.Sprintf("external capability %s: OK", name))
	}
	return lines, nil
}

// LoopProject runs the product projector action against a supported host
// runtime, streaming host output to out/errw.
func (h *Harness) LoopProject(ctx context.Context, out, errw io.Writer, action, projectRoot, host string, loops, hostArgs []string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if action != "install" && action != "uninstall" {
		return fmt.Errorf("unsupported projector action %q", action)
	}
	switch host {
	case "codex":
		return hostsurface.RunCodexProjector(ctx, action, hostsurface.CodexOptions{
			ProjectRoot: projectRoot,
			Loops:       loops,
			HostArgs:    hostArgs,
			Stdout:      out,
			Stderr:      errw,
		})
	case "claude-code":
		return hostsurface.RunClaudeProjector(ctx, action, hostsurface.ClaudeOptions{
			ProjectRoot: projectRoot,
			Loops:       loops,
			HostArgs:    hostArgs,
			Stdout:      out,
			Stderr:      errw,
		})
	default:
		return fmt.Errorf("unsupported host %q; setup supports codex and claude-code", host)
	}
}

// Refresh re-projects the managed definition files (GUIDE, hooks, skill defs) for a host loop under
// the no-clobber policy: a definition file the user has edited is preserved and reported, never
// overwritten. It does NOT touch the channel (bindings, token, config) — only the Agent Workspace
// projection. It returns the display paths it preserved.
func (h *Harness) Refresh(ctx context.Context, out, errw io.Writer, projectRoot, host string, loops, hostArgs []string) ([]string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	switch host {
	case "codex":
		rep, err := hostsurface.RunCodexProjectorReport(ctx, hostsurface.CodexOptions{
			ProjectRoot: projectRoot, Loops: loops, HostArgs: hostArgs, Stdout: out, Stderr: errw,
		})
		return rep.Conflicts, err
	case "claude-code":
		rep, err := hostsurface.RunClaudeProjectorReport(ctx, hostsurface.ClaudeOptions{
			ProjectRoot: projectRoot, Loops: loops, HostArgs: hostArgs, Stdout: out, Stderr: errw,
		})
		return rep.Conflicts, err
	default:
		return nil, fmt.Errorf("unsupported host %q; refresh supports codex and claude-code", host)
	}
}
