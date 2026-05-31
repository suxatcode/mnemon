package app

import (
	"context"
	"fmt"
	"io"

	"github.com/mnemon-dev/mnemon/harness/internal/declaration"
	"github.com/mnemon-dev/mnemon/harness/internal/projection"
)

// LoopValidate validates the harness loop/host/binding declarations under the
// facade root and returns the human-readable report lines.
func (h *Harness) LoopValidate() ([]string, error) {
	result, err := declaration.ValidateHarness(h.root)
	if err != nil {
		return nil, err
	}
	return result.Lines, nil
}

// LoopPlan builds the projection plan for a host and writes it to out in the
// requested format ("text"/"" or "json").
func (h *Harness) LoopPlan(out io.Writer, projectRoot, host string, loops []string, format string) error {
	plan, err := projection.BuildPlan(projection.PlanOptions{
		DeclarationRoot: h.root,
		ProjectRoot:     projectRoot,
		Host:            host,
		Loops:           loops,
	})
	if err != nil {
		return err
	}
	switch format {
	case "text", "":
		return projection.WritePlanText(out, plan)
	case "json":
		return projection.WritePlanJSON(out, plan)
	default:
		return fmt.Errorf("unsupported --format %q", format)
	}
}

// LoopProject runs a projector action (install/diff/reconcile/status/uninstall)
// against a host runtime, streaming host output to out/errw. Reconcile output is
// formatted here so the surface never touches projection result types.
func (h *Harness) LoopProject(ctx context.Context, out, errw io.Writer, action, projectRoot, host string, loops, hostArgs []string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	switch host {
	case "codex":
		if action == "reconcile" {
			result, err := projection.RunCodexReconcile(ctx, projection.CodexOptions{
				DeclarationRoot: h.root,
				ProjectRoot:     projectRoot,
				Loops:           loops,
				HostArgs:        hostArgs,
				Stdout:          out,
				Stderr:          errw,
			})
			if err != nil {
				return err
			}
			writeReconcileText(out, result)
			return nil
		}
		return projection.RunCodexProjector(ctx, action, projection.CodexOptions{
			DeclarationRoot: h.root,
			ProjectRoot:     projectRoot,
			Loops:           loops,
			HostArgs:        hostArgs,
			Stdout:          out,
			Stderr:          errw,
		})
	case "claude-code":
		if action == "reconcile" {
			return fmt.Errorf("reconcile is not supported for host %q", host)
		}
		return projection.RunClaudeProjector(ctx, action, projection.ClaudeOptions{
			DeclarationRoot: h.root,
			ProjectRoot:     projectRoot,
			Loops:           loops,
			HostArgs:        hostArgs,
			Stdout:          out,
			Stderr:          errw,
		})
	default:
		if action == "reconcile" {
			return fmt.Errorf("reconcile is not supported for host %q", host)
		}
		return projection.RunLegacyProjector(ctx, action, projection.LegacyOptions{
			DeclarationRoot: h.root,
			ProjectRoot:     projectRoot,
			Host:            host,
			Loops:           loops,
			HostArgs:        hostArgs,
			Stdout:          out,
			Stderr:          errw,
		})
	}
}

func writeReconcileText(out io.Writer, result projection.ReconcileResult) {
	if len(result.Items) == 0 {
		fmt.Fprintf(out, "Codex reconcile: no drift\n")
		fmt.Fprintf(out, "event: %s\n", result.EventID)
		return
	}
	fmt.Fprintf(out, "Codex reconcile: repaired %d drift item(s)\n", len(result.Repaired))
	for _, item := range result.Repaired {
		fmt.Fprintf(out, "  repaired %s\n", item.Text())
	}
	fmt.Fprintf(out, "event: %s\n", result.EventID)
}
