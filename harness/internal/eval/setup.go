package eval

import (
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

type SetupRuntime struct {
	Handlers      map[string]SetupHandler
	MnemonCommand string
}

type SetupHandler interface {
	Setup(context.Context, SetupContext) error
}

type SetupFunc func(context.Context, SetupContext) error

type SetupContext struct {
	WorkspaceDir  string
	MnemonDir     string
	Env           map[string]string
	MnemonCommand string
}

type SetupOptions struct {
	Handler       string
	WorkspaceDir  string
	MnemonDir     string
	Loops         []string
	Env           map[string]string
	MnemonCommand string
}

func (fn SetupFunc) Setup(ctx context.Context, input SetupContext) error {
	if fn == nil {
		return errors.New("setup func is nil")
	}
	return fn(ctx, input)
}

func (runtime SetupRuntime) Run(ctx context.Context, opts SetupOptions) error {
	if ctx == nil {
		ctx = context.Background()
	}
	handlerID := strings.TrimSpace(opts.Handler)
	if handlerID == "" {
		handlerID = "setup_none"
	}
	handlers := runtime.Handlers
	if handlers == nil {
		handlers = BuiltinSetupHandlers()
	}
	handler, ok := handlers[handlerID]
	if !ok {
		return fmt.Errorf("setup handler %q not registered", handlerID)
	}
	env := opts.Env
	if env == nil {
		env = SetupEnv(opts.MnemonDir, opts.Loops)
	}
	mnemonCommand := opts.MnemonCommand
	if mnemonCommand == "" {
		mnemonCommand = runtime.MnemonCommand
	}
	if mnemonCommand == "" {
		mnemonCommand = "mnemon"
	}
	return handler.Setup(ctx, SetupContext{
		WorkspaceDir:  opts.WorkspaceDir,
		MnemonDir:     opts.MnemonDir,
		Env:           env,
		MnemonCommand: mnemonCommand,
	})
}

func BuiltinSetupHandlers() map[string]SetupHandler {
	return map[string]SetupHandler{
		"setup_none":                        SetupFunc(setupNone),
		"setup_memory_seed":                 SetupFunc(setupMemorySeed),
		"setup_local_fact":                  SetupFunc(setupLocalFact),
		"setup_memory_merge":                SetupFunc(setupMemoryMerge),
		"setup_memory_uncertain_preference": SetupFunc(setupMemoryUncertainPreference),
		"setup_memory_noise":                SetupFunc(setupMemoryNoise),
		"setup_memory_polluted":             SetupFunc(setupMemoryPolluted),
		"setup_skill_curate_evidence":       SetupFunc(setupSkillCurateEvidence),
		"setup_skill_active_release":        SetupFunc(setupSkillActiveRelease),
		"setup_skill_active_legacy":         SetupFunc(setupSkillActiveLegacy),
		"setup_skill_stale_release":         SetupFunc(setupSkillStaleRelease),
	}
}

func SetupEnv(mnemonDir string, loops []string) map[string]string {
	env := map[string]string{
		"MNEMON_HARNESS_STATE_DIR": mnemonDir,
		"MNEMON_DATA_DIR":          filepath.Join(mnemonDir, "data"),
	}
	seen := map[string]bool{}
	for _, loop := range loops {
		seen[loop] = true
	}
	if seen["memory"] {
		memoryDir := filepath.Join(mnemonDir, "harness", "memory")
		env["MNEMON_MEMORY_LOOP_ENV"] = filepath.Join(memoryDir, "env.sh")
		env["MNEMON_MEMORY_LOOP_DIR"] = memoryDir
	}
	if seen["skill"] {
		skillDir := filepath.Join(mnemonDir, "harness", "skill")
		env["MNEMON_SKILL_LOOP_ENV"] = filepath.Join(skillDir, "env.sh")
		env["MNEMON_SKILL_LOOP_DIR"] = skillDir
		env["MNEMON_SKILL_LOOP_LIBRARY_DIR"] = filepath.Join(skillDir, "skills")
		env["MNEMON_SKILL_LOOP_ACTIVE_DIR"] = filepath.Join(skillDir, "skills", "active")
		env["MNEMON_SKILL_LOOP_STALE_DIR"] = filepath.Join(skillDir, "skills", "stale")
		env["MNEMON_SKILL_LOOP_ARCHIVED_DIR"] = filepath.Join(skillDir, "skills", "archived")
		env["MNEMON_SKILL_LOOP_USAGE_FILE"] = filepath.Join(skillDir, "skills", ".usage.jsonl")
		env["MNEMON_SKILL_LOOP_PROPOSALS_DIR"] = filepath.Join(skillDir, "proposals")
	}
	if seen["eval"] {
		evalDir := filepath.Join(mnemonDir, "harness", "eval")
		env["MNEMON_EVAL_LOOP_ENV"] = filepath.Join(evalDir, "env.sh")
		env["MNEMON_EVAL_LOOP_DIR"] = evalDir
		env["MNEMON_EVAL_LOOP_SCRATCH_DIR"] = filepath.Join(evalDir, "scratch")
		env["MNEMON_EVAL_LOOP_CANDIDATES_DIR"] = filepath.Join(evalDir, "candidates")
		env["MNEMON_EVAL_LOOP_REPORTS_DIR"] = filepath.Join(evalDir, "reports")
		env["MNEMON_EVAL_LOOP_ARTIFACTS_DIR"] = filepath.Join(evalDir, "artifacts")
		env["MNEMON_EVAL_LOOP_RETIRED_DIR"] = filepath.Join(evalDir, "retired")
	}
	return env
}

func SetupEnvPairs(env map[string]string) []string {
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

func setupNone(ctx context.Context, input SetupContext) error {
	return nil
}

func setupMemorySeed(ctx context.Context, input SetupContext) error {
	return runMnemon(ctx, input, "remember",
		"Project decision: Mnemon harness validation should prefer the real Codex app-server for host integration checks.",
		"--cat", "decision",
		"--imp", "5",
		"--tags", "harness,codex,eval",
		"--entities", "Codex app-server,Mnemon harness",
	)
}

func setupLocalFact(ctx context.Context, input SetupContext) error {
	return writeSetupFile(filepath.Join(input.WorkspaceDir, "FACTS.md"),
		"# Local Facts\n\n"+
			"- The local release color is cerulean.\n",
	)
}

func setupMemoryMerge(ctx context.Context, input SetupContext) error {
	return appendMemory(input.MnemonDir, "- Loop optimization should prioritize broad host expansion before scenario evals. (source: user, confidence: medium)")
}

func setupMemoryUncertainPreference(ctx context.Context, input SetupContext) error {
	return appendMemory(input.MnemonDir, "- Preferred package manager for this project is npm. (source: user, confidence: high)")
}

func setupMemoryNoise(ctx context.Context, input SetupContext) error {
	memories := [][]string{
		{
			"Project decision: Mnemon should validate host integration with real Codex app-server evals before relying on adapter-only checks.",
			"decision",
			"5",
			"Codex app-server,Mnemon harness",
		},
		{
			"Temporary fact: the demo workspace color was magenta during a disposable test run.",
			"fact",
			"1",
			"demo workspace",
		},
		{
			"User preference: keep Chinese status updates concise during long-running eval work.",
			"preference",
			"4",
			"Chinese,status update",
		},
	}
	for _, memory := range memories {
		if err := runMnemon(ctx, input, "remember", memory[0], "--cat", memory[1], "--imp", memory[2], "--tags", "memory-deep", "--entities", memory[3]); err != nil {
			return err
		}
	}
	return nil
}

func setupMemoryPolluted(ctx context.Context, input SetupContext) error {
	return appendMemory(input.MnemonDir, "- Temporary task token 742913 was incorrectly stored. (source: eval router fixture, confidence: low)")
}

func setupSkillCurateEvidence(ctx context.Context, input SetupContext) error {
	for index, event := range []string{"missing", "workflow", "feedback"} {
		item := map[string]any{
			"time":    fmt.Sprintf("2026-05-15T00:0%d:00Z", index+1),
			"skill":   nil,
			"event":   event,
			"outcome": "neutral",
			"note":    "Release handoff checklist workflow repeated across eval, docs, and push tasks.",
			"source":  "agent",
		}
		if event == "missing" {
			item["outcome"] = "negative"
		}
		if err := appendSkillUsage(input.MnemonDir, item); err != nil {
			return err
		}
	}
	return nil
}

func setupSkillActiveRelease(ctx context.Context, input SetupContext) error {
	return writeSkill(skillActivePath(input.MnemonDir, "release-checklist"), "release-checklist", "Release handoff checklist fixture.")
}

func setupSkillActiveLegacy(ctx context.Context, input SetupContext) error {
	return writeSkill(skillActivePath(input.MnemonDir, "legacy-release"), "legacy-release", "Legacy release workflow fixture.")
}

func setupSkillStaleRelease(ctx context.Context, input SetupContext) error {
	return writeSkill(skillStalePath(input.MnemonDir, "release-checklist"), "release-checklist", "Stale release handoff checklist fixture.")
}

func runMnemon(ctx context.Context, input SetupContext, args ...string) error {
	command := exec.CommandContext(ctx, input.MnemonCommand, args...)
	command.Dir = input.WorkspaceDir
	command.Env = append(os.Environ(), SetupEnvPairs(input.Env)...)
	output, err := command.CombinedOutput()
	if err != nil {
		message := strings.TrimSpace(string(output))
		if message == "" {
			message = err.Error()
		}
		return fmt.Errorf("mnemon %s failed: %s", strings.Join(args, " "), message)
	}
	return nil
}

func memoryPath(mnemonDir string) string {
	return filepath.Join(mnemonDir, "harness", "memory", "MEMORY.md")
}

func appendMemory(mnemonDir, text string) error {
	path := memoryPath(mnemonDir)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer file.Close()
	_, err = fmt.Fprintf(file, "\n%s\n", strings.TrimRight(text, "\n"))
	return err
}

func skillLoopPath(mnemonDir string) string {
	return filepath.Join(mnemonDir, "harness", "skill")
}

func skillUsagePath(mnemonDir string) string {
	return filepath.Join(skillLoopPath(mnemonDir), "skills", ".usage.jsonl")
}

func skillActivePath(mnemonDir, skillID string) string {
	return filepath.Join(skillLoopPath(mnemonDir), "skills", "active", skillID, "SKILL.md")
}

func skillStalePath(mnemonDir, skillID string) string {
	return filepath.Join(skillLoopPath(mnemonDir), "skills", "stale", skillID, "SKILL.md")
}

func writeSkill(path, skillID, description string) error {
	return writeSetupFile(path,
		"---\n"+
			"name: "+skillID+"\n"+
			"description: "+description+"\n"+
			"---\n\n"+
			"# "+skillID+"\n\n"+
			"Use this skill for lifecycle eval fixtures.\n",
	)
}

func appendSkillUsage(mnemonDir string, item map[string]any) error {
	path := skillUsagePath(mnemonDir)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.Marshal(item)
	if err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer file.Close()
	if _, err := file.Write(append(data, '\n')); err != nil {
		return err
	}
	return nil
}

func writeSetupFile(path, content string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(content), 0o644)
}
