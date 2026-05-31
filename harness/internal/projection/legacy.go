package projection

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/mnemon-dev/mnemon/harness/internal/declaration"
)

type LegacyOptions struct {
	DeclarationRoot string
	ProjectRoot     string
	Host            string
	Loops           []string
	HostArgs        []string
	Stdout          io.Writer
	Stderr          io.Writer
}

func RunLegacyProjector(ctx context.Context, action string, opts LegacyOptions) error {
	if opts.DeclarationRoot == "" {
		opts.DeclarationRoot = "."
	}
	declarationRoot, err := filepath.Abs(opts.DeclarationRoot)
	if err != nil {
		return fmt.Errorf("resolve declaration root: %w", err)
	}
	if opts.ProjectRoot == "" {
		opts.ProjectRoot, err = os.Getwd()
		if err != nil {
			return fmt.Errorf("resolve project root: %w", err)
		}
	}
	projectRoot, err := filepath.Abs(opts.ProjectRoot)
	if err != nil {
		return fmt.Errorf("resolve project root: %w", err)
	}
	if opts.Host == "" {
		return errors.New("--host is required")
	}
	loops := append([]string(nil), opts.Loops...)
	if len(loops) == 0 {
		if action != "status" {
			return errors.New("at least one --loop is required")
		}
		loops, err = declaration.LoopsForHost(declarationRoot, opts.Host)
		if err != nil {
			return err
		}
		if len(loops) == 0 {
			return fmt.Errorf("no bindings found for host %q", opts.Host)
		}
	}

	projector := filepath.Join(declarationRoot, "harness", "hosts", opts.Host, "projector.sh")
	info, err := os.Stat(projector)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("unsupported host or missing projector: %s", opts.Host)
		}
		return fmt.Errorf("stat projector: %w", err)
	}
	if info.Mode()&0o111 == 0 {
		return fmt.Errorf("projector is not executable: %s", projector)
	}

	for _, loop := range loops {
		args := []string{action, "--loop", loop}
		args = append(args, opts.HostArgs...)
		command := exec.CommandContext(ctx, projector, args...)
		command.Dir = projectRoot
		command.Stdout = opts.Stdout
		command.Stderr = opts.Stderr
		if err := command.Run(); err != nil {
			return fmt.Errorf("%s %s/%s: %w", action, opts.Host, loop, err)
		}
	}
	return nil
}
