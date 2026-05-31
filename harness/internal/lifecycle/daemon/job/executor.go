package job

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/mnemon-dev/mnemon/harness/internal/lifecycle/daemon/loader"
)

type CLIResult struct {
	ExitCode int
	Stdout   string
	Stderr   string
}

func ExecuteCLI(ctx context.Context, root string, action loader.Action, maxSec int) (CLIResult, error) {
	if action.CLI == "" {
		return CLIResult{}, fmt.Errorf("cli action is required")
	}
	if maxSec <= 0 {
		maxSec = 300
	}
	ctx, cancel := context.WithTimeout(ctx, time.Duration(maxSec)*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "sh", "-c", action.CLI)
	cmd.Dir = cliCWD(root, action.CWD)
	cmd.Env = os.Environ()
	for key, value := range action.Env {
		cmd.Env = append(cmd.Env, key+"="+value)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	result := CLIResult{Stdout: stdout.String(), Stderr: stderr.String()}
	if cmd.ProcessState != nil {
		result.ExitCode = cmd.ProcessState.ExitCode()
	}
	if ctx.Err() != nil {
		return result, ctx.Err()
	}
	if err != nil {
		return result, err
	}
	return result, nil
}

func cliCWD(root, cwd string) string {
	if root == "" {
		root = "."
	}
	if cwd == "" {
		return filepath.Clean(root)
	}
	if filepath.IsAbs(cwd) {
		return filepath.Clean(cwd)
	}
	return filepath.Join(root, cwd)
}
