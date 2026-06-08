package app

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/mnemon-dev/mnemon/harness/internal/assets"
	"github.com/mnemon-dev/mnemon/harness/internal/hostsurface"
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
	if h.root != "" {
		external, err := manifest.ValidateFS(os.DirFS(h.root))
		if err != nil {
			return nil, err
		}
		lines = append(lines, external.Lines...)
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
