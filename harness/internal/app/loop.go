package app

import (
	"context"
	"fmt"
	"io"

	"github.com/mnemon-dev/mnemon/harness/internal/hostsurface"
	"github.com/mnemon-dev/mnemon/harness/internal/manifest"
)

// LoopValidate validates the harness loop/host/binding declarations under the
// facade root and returns the human-readable report lines.
func (h *Harness) LoopValidate() ([]string, error) {
	result, err := manifest.ValidateHarness(h.root)
	if err != nil {
		return nil, err
	}
	return result.Lines, nil
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
			DeclarationRoot: h.root,
			ProjectRoot:     projectRoot,
			Loops:           loops,
			HostArgs:        hostArgs,
			Stdout:          out,
			Stderr:          errw,
		})
	case "claude-code":
		return hostsurface.RunClaudeProjector(ctx, action, hostsurface.ClaudeOptions{
			DeclarationRoot: h.root,
			ProjectRoot:     projectRoot,
			Loops:           loops,
			HostArgs:        hostArgs,
			Stdout:          out,
			Stderr:          errw,
		})
	default:
		return fmt.Errorf("unsupported host %q; setup supports codex and claude-code", host)
	}
}
