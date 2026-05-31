package eval

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

type AssertionBackend string

const (
	AssertionBackendPython AssertionBackend = "python"
	AssertionBackendGo     AssertionBackend = "go"
)

type AssertionRuntime struct {
	Root          string
	PythonCommand string
	PythonScript  string
	GoHandlers    map[string]AssertionHandler
}

type AssertionRunOptions struct {
	Backend      AssertionBackend
	ScenarioID   string
	Handler      string
	Report       map[string]any
	WorkspaceDir string
	MnemonDir    string
	Env          map[string]string
}

func (runtime AssertionRuntime) Run(ctx context.Context, opts AssertionRunOptions) ([]AssertionResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	switch opts.Backend {
	case "", AssertionBackendPython:
		return runtime.runPython(ctx, opts)
	case AssertionBackendGo:
		return runtime.runGo(ctx, opts)
	default:
		return nil, fmt.Errorf("unsupported assertion backend %q", opts.Backend)
	}
}

func (runtime AssertionRuntime) runGo(ctx context.Context, opts AssertionRunOptions) ([]AssertionResult, error) {
	handlerID := strings.TrimSpace(opts.Handler)
	if handlerID == "" {
		handlerID = strings.TrimSpace(opts.ScenarioID)
	}
	if handlerID == "" {
		return nil, errors.New("assertion handler is required for go backend")
	}
	handler, ok := runtime.GoHandlers[handlerID]
	if !ok {
		return nil, fmt.Errorf("go assertion handler %q not registered", handlerID)
	}
	results, err := handler.Assert(ctx, AssertionContext{
		Report:       nonNilReport(opts.Report),
		WorkspaceDir: opts.WorkspaceDir,
		MnemonDir:    opts.MnemonDir,
		Env:          opts.Env,
	})
	if err != nil {
		return nil, err
	}
	if err := ValidateAssertionResults(results); err != nil {
		return nil, err
	}
	return results, nil
}

func (runtime AssertionRuntime) runPython(ctx context.Context, opts AssertionRunOptions) ([]AssertionResult, error) {
	if strings.TrimSpace(opts.ScenarioID) == "" {
		return nil, errors.New("scenario id is required for python assertion backend")
	}
	root := cleanRoot(runtime.Root)
	python := runtime.PythonCommand
	if python == "" {
		python = "python3"
	}
	script := runtime.PythonScript
	if script == "" {
		script = filepath.Join(root, "scripts", "codex_app_server_eval.py")
	}
	reportPath, cleanup, err := writeAssertionReport(nonNilReport(opts.Report))
	if err != nil {
		return nil, err
	}
	defer cleanup()

	args := []string{
		script,
		"--assertion-only",
		"--scenario", opts.ScenarioID,
		"--report", reportPath,
	}
	if strings.TrimSpace(opts.WorkspaceDir) != "" {
		args = append(args, "--workspace", opts.WorkspaceDir)
	}
	if strings.TrimSpace(opts.MnemonDir) != "" {
		args = append(args, "--mnemon-dir", opts.MnemonDir)
	}
	for _, item := range envPairs(opts.Env) {
		args = append(args, "--env", item)
	}

	command := exec.CommandContext(ctx, python, args...)
	command.Dir = root
	command.Env = append(os.Environ(), envPairs(opts.Env)...)
	var stderr bytes.Buffer
	command.Stderr = &stderr
	output, err := command.Output()
	if err != nil {
		message := strings.TrimSpace(stderr.String())
		if message == "" {
			message = strings.TrimSpace(string(output))
		}
		if message == "" {
			message = err.Error()
		}
		return nil, fmt.Errorf("python assertion backend failed: %s", message)
	}

	var decoded struct {
		Assertions []AssertionResult `json:"assertions"`
	}
	if err := json.Unmarshal(output, &decoded); err != nil {
		return nil, fmt.Errorf("parse python assertion output: %w", err)
	}
	if err := ValidateAssertionResults(decoded.Assertions); err != nil {
		return nil, err
	}
	return decoded.Assertions, nil
}

func writeAssertionReport(report map[string]any) (string, func(), error) {
	file, err := os.CreateTemp("", "mnemon-assertion-report-*.json")
	if err != nil {
		return "", func() {}, fmt.Errorf("create assertion report: %w", err)
	}
	cleanup := func() {
		_ = os.Remove(file.Name())
	}
	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(report); err != nil {
		_ = file.Close()
		cleanup()
		return "", func() {}, fmt.Errorf("write assertion report: %w", err)
	}
	if err := file.Close(); err != nil {
		cleanup()
		return "", func() {}, fmt.Errorf("close assertion report: %w", err)
	}
	return file.Name(), cleanup, nil
}

func envPairs(env map[string]string) []string {
	pairs := make([]string, 0, len(env))
	for key, value := range env {
		if strings.TrimSpace(key) == "" {
			continue
		}
		pairs = append(pairs, key+"="+value)
	}
	sort.Strings(pairs)
	return pairs
}

func nonNilReport(report map[string]any) map[string]any {
	if report == nil {
		return map[string]any{}
	}
	return report
}
