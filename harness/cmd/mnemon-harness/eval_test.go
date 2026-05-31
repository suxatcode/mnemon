package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mnemon-dev/mnemon/harness/internal/app"
	harnesseval "github.com/mnemon-dev/mnemon/harness/internal/eval"
	"github.com/mnemon-dev/mnemon/harness/internal/lifecycle/eventlog"
	"github.com/mnemon-dev/mnemon/harness/internal/lifecycle/proposal"
	"github.com/mnemon-dev/mnemon/harness/internal/lifecycle/proposalstore"
	runnercodex "github.com/mnemon-dev/mnemon/harness/internal/lifecycle/runner/codex"
	"github.com/mnemon-dev/mnemon/harness/internal/lifecycle/schema"
)

func TestEvalPlanCommand(t *testing.T) {
	root := t.TempDir()
	suiteDir := filepath.Join(root, "harness", "loops", "eval", "suites")
	if err := os.MkdirAll(suiteDir, 0o755); err != nil {
		t.Fatalf("mkdir suite dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(suiteDir, "default.json"), []byte(`{
  "name": "default",
  "description": "fixture suite",
  "host": "codex",
  "runner": "codex-app-server",
  "scenario_ids": ["memory-focused-recall"]
}`), 0o644); err != nil {
		t.Fatalf("write suite: %v", err)
	}
	restoreEvalFlags(t)
	evalRoot = root
	evalPlanSuite = "default"

	cmd, output := testCommand()
	if err := runEvalPlan(cmd, nil); err != nil {
		t.Fatalf("runEvalPlan returned error: %v", err)
	}
	for _, want := range []string{"Eval suite default", "Runner: codex-app-server", "- memory-focused-recall"} {
		if !strings.Contains(output.String(), want) {
			t.Fatalf("expected %q in output:\n%s", want, output.String())
		}
	}
}

func TestEvalRunCommandProjectsDeclaredLoopBeforeGate(t *testing.T) {
	root := t.TempDir()
	writeEvalRunFixture(t, root)
	restoreEvalFlags(t)
	evalRoot = root
	evalRunSuite = "default"
	evalRunScenario = "eval-smoke"
	evalRunCommand = "definitely-not-a-codex-command"
	evalRunTimeout = time.Second

	cmd, output := testCommand()
	if err := runEvalRun(cmd, nil); err != nil {
		t.Fatalf("runEvalRun returned error: %v", err)
	}
	for _, want := range []string{
		"eval run: blocked",
		"scenario: eval-smoke",
		"host: codex",
		"runner: codex-app-server",
		"projected loops: eval",
		"run-id:",
	} {
		if !strings.Contains(output.String(), want) {
			t.Fatalf("expected %q in output:\n%s", want, output.String())
		}
	}
	matches, err := filepath.Glob(filepath.Join(root, ".mnemon", "harness", "runs", "codex-app-server", "*", "workspace", ".codex", "skills", "eval-run", "SKILL.md"))
	if err != nil {
		t.Fatalf("glob projected eval skill: %v", err)
	}
	if len(matches) != 1 {
		t.Fatalf("expected one projected eval skill, got %v", matches)
	}
	factMatches, err := filepath.Glob(filepath.Join(root, ".mnemon", "harness", "runs", "codex-app-server", "*", "workspace", "FACTS.md"))
	if err != nil {
		t.Fatalf("glob setup facts: %v", err)
	}
	if len(factMatches) != 1 {
		t.Fatalf("expected one setup FACTS.md, got %v", factMatches)
	}
	if _, err := os.Stat(filepath.Join(root, ".mnemon", "harness", "status", "jobs", "eval_default_eval_smoke.json")); err != nil {
		t.Fatalf("expected eval job status: %v", err)
	}
}

func TestEvalABTestCommandBlocksWithoutCostGate(t *testing.T) {
	root := t.TempDir()
	writeEvalRunFixture(t, root)
	restoreEvalFlags(t)
	evalRoot = root
	evalABSuite = "default"
	evalABScenarios = []string{"eval-smoke"}
	evalABTrialsPerArm = 1
	evalABCommand = "definitely-not-a-codex-command"
	evalABTimeout = time.Second
	evalABTreatmentSetupJSON = `{"candidate_id":"dogfood-s3-4-no-console-log-guide","summary":"guide candidate under test"}`

	cmd, output := testCommand()
	if err := runEvalABTest(cmd, nil); err != nil {
		t.Fatalf("runEvalABTest returned error: %v", err)
	}
	for _, want := range []string{
		"abtest:",
		"suite: default",
		"scenarios: eval-smoke",
		"trials: 2",
		"control pass rate: 0.00",
		"treatment pass rate: 0.00",
		"real turns: blocked",
	} {
		if !strings.Contains(output.String(), want) {
			t.Fatalf("expected %q in output:\n%s", want, output.String())
		}
	}
	matches, err := filepath.Glob(filepath.Join(root, ".mnemon", "harness", "reports", "abtest", "*.json"))
	if err != nil {
		t.Fatalf("glob abtest report: %v", err)
	}
	if len(matches) != 1 {
		t.Fatalf("expected one abtest report, got %v", matches)
	}
	data, err := os.ReadFile(matches[0])
	if err != nil {
		t.Fatalf("read abtest report: %v", err)
	}
	var report struct {
		Kind    string `json:"kind"`
		Request struct {
			TreatmentSetup map[string]any `json:"treatment_setup"`
		} `json:"request"`
		Trials []struct {
			Status  string `json:"status"`
			Outcome string `json:"outcome"`
		} `json:"trials"`
	}
	if err := json.Unmarshal(data, &report); err != nil {
		t.Fatalf("parse abtest report: %v", err)
	}
	if report.Kind != "ABTestResult" || len(report.Trials) != 2 {
		t.Fatalf("unexpected report: %#v", report)
	}
	if report.Request.TreatmentSetup["candidate_id"] != "dogfood-s3-4-no-console-log-guide" {
		t.Fatalf("expected treatment setup in report, got %#v", report.Request.TreatmentSetup)
	}
	for _, trial := range report.Trials {
		if trial.Status != "blocked" || trial.Outcome != "invalid" {
			t.Fatalf("expected blocked invalid trial, got %#v", trial)
		}
	}
}

func TestEvalAssertCommandRoutesFailedFindingToProposalDraft(t *testing.T) {
	root := t.TempDir()
	writeEvalRunFixture(t, root)
	writeFile(t, root, "harness/loops/eval/suites/router-fixture.json", `{
  "name": "router-fixture",
  "host": "codex",
  "runner": "assertion-only",
  "scenario_ids": ["memory-router-failed-finding"]
}`)
	writeFile(t, root, "harness/loops/eval/scenarios/codex-app.json", `{
  "schema_version": 1,
  "name": "codex-app",
  "scenarios": [
    {
      "id": "memory-router-failed-finding",
      "area": "memory",
      "loops": ["memory"],
      "setup_handler": "setup_memory_polluted",
      "assertion_handler": "assert_memory_no_pollution",
      "prompts": ["Assertion-only router fixture."]
    }
  ]
}`)
	writeFile(t, root, "scripts/codex_app_server_eval.py", `#!/usr/bin/env python3
import json
print(json.dumps({"assertions":[{"name":"memory file skipped transient token","passed":False,"rejected":"742913"}]}))
`)
	if err := os.Chmod(filepath.Join(root, "scripts", "codex_app_server_eval.py"), 0o755); err != nil {
		t.Fatalf("chmod assertion script: %v", err)
	}
	restoreEvalFlags(t)
	evalRoot = root
	evalAssertSuite = "router-fixture"
	evalAssertScenario = "memory-router-failed-finding"
	evalAssertRunID = "assert-router-fixture"

	cmd, output := testCommand()
	if err := runEvalAssert(cmd, nil); err != nil {
		t.Fatalf("runEvalAssert returned error: %v", err)
	}
	for _, want := range []string{
		"eval assert: fail",
		"suite: router-fixture",
		"scenario: memory-router-failed-finding",
		"proposal: eval-memory-memory-router-failed-finding-assert-router-fixture route=memory status=draft",
	} {
		if !strings.Contains(output.String(), want) {
			t.Fatalf("expected %q in output:\n%s", want, output.String())
		}
	}
	if _, err := os.Stat(filepath.Join(root, ".mnemon", "harness", "proposals", "draft", "eval-memory-memory-router-failed-finding-assert-router-fixture", "proposal.json")); err != nil {
		t.Fatalf("expected proposal draft file: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, ".mnemon", "harness", "reports", "runner", "assert-router-fixture-codex-app-server-semantic-run.json")); err != nil {
		t.Fatalf("expected assertion-only report: %v", err)
	}
}

func TestFinalizeEvalRunRoutesFailureToProposalDraft(t *testing.T) {
	root := t.TempDir()
	runID := "run-routing"
	workspace := filepath.Join(root, "workspace")
	if err := os.MkdirAll(filepath.Join(workspace, ".mnemon"), 0o755); err != nil {
		t.Fatalf("mkdir workspace: %v", err)
	}
	writeFile(t, root, "scripts/codex_app_server_eval.py", `#!/usr/bin/env python3
import json
print(json.dumps({"assertions":[{"name":"memory stayed clean","passed":False,"expected":"no temporary token"}]}))
`)
	if err := os.Chmod(filepath.Join(root, "scripts", "codex_app_server_eval.py"), 0o755); err != nil {
		t.Fatalf("chmod assertion script: %v", err)
	}
	writeFile(t, root, ".mnemon/harness/reports/runner/"+runID+"-codex-app-server-semantic-run.json", `{
  "schema_version": 1,
  "kind": "CodexAppServerSemanticRunReport",
  "run_id": "run-routing",
  "runner_id": "codex-app-server",
  "job_id": "eval_memory_deep_memory_no_pollution",
  "job_spec": "eval.memory-no-pollution",
  "loop": "eval",
  "status": "ready",
  "message": "ok",
  "artifact_refs": [
    {"id": "artifact:jsonrpc-transcript", "kind": "transcript", "uri": ".mnemon/harness/runs/codex-app-server/run-routing/artifacts/jsonrpc-transcript.jsonl", "media_type": "application/jsonl", "privacy": "project"}
  ]
}`)
	writeFile(t, root, ".mnemon/harness/runs/codex-app-server/"+runID+"/artifacts/jsonrpc-transcript.jsonl", `{"direction":"client","payload":{"id":1,"method":"thread/start","params":{}}}
{"direction":"server","payload":{"id":1,"result":{"thread":{"id":"thread-routing"}}}}
`)

	post, err := app.FinalizeEvalRun(nil, root, harnesseval.RunPlan{
		Suite:      harnesseval.Suite{Name: "memory-deep"},
		ScenarioID: "memory-no-pollution",
		Scenario: &harnesseval.Scenario{
			ID:               "memory-no-pollution",
			Loops:            []string{"memory"},
			AssertionHandler: "assert_memory_no_pollution",
		},
		ProjectLoops: []string{"eval", "memory"},
	}, runnercodex.RunResult{
		RunID:     runID,
		Status:    runnercodex.StatusReady,
		Workspace: workspace,
	})
	if err != nil {
		t.Fatalf("finalizeEvalRun returned error: %v", err)
	}
	if post.Outcome != harnesseval.OutcomeFail || len(post.Proposals) != 1 {
		t.Fatalf("expected failed outcome with one proposal, got %#v", post)
	}
	item := post.Proposals[0]
	if item.Route != proposal.RouteMemory || item.Status != proposal.StatusDraft {
		t.Fatalf("unexpected proposal route/status: %#v", item)
	}
	if len(item.Evidence) < 2 || item.Evidence[0].Type != "eval_report" {
		t.Fatalf("expected eval report evidence refs: %#v", item.Evidence)
	}
	if _, err := os.Stat(filepath.Join(root, ".mnemon", "harness", "proposals", "draft", item.ID, "proposal.json")); err != nil {
		t.Fatalf("expected proposal draft file: %v", err)
	}
}

func TestEvalPromoteCommandAppendsEvent(t *testing.T) {
	root := t.TempDir()
	writeEvalRunFixture(t, root)
	proposalID := createEvalCommandApprovedProposal(t, root, "eval-promote-cli")
	restoreEvalFlags(t)
	evalRoot = root
	evalPromoteSuite = "default"
	evalPromoteTarget = "candidate"
	evalPromoteProposalRef = proposalID
	evalPromoteEventID = "evt_eval_promote_cli"

	cmd, output := testCommand()
	if err := runEvalPromote(cmd, nil); err != nil {
		t.Fatalf("runEvalPromote returned error: %v", err)
	}
	for _, want := range []string{
		"eval asset promoted: suite default",
		"to: candidate",
		"proposal: eval-promote-cli",
		"event: evt_eval_promote_cli",
	} {
		if !strings.Contains(output.String(), want) {
			t.Fatalf("expected %q in output:\n%s", want, output.String())
		}
	}
	store, err := eventlog.New(root)
	if err != nil {
		t.Fatalf("eventlog.New returned error: %v", err)
	}
	events, err := store.ReadAll()
	if err != nil {
		t.Fatalf("ReadAll returned error: %v", err)
	}
	var event schema.Event
	for _, candidate := range events {
		if candidate.ID == "evt_eval_promote_cli" {
			event = candidate
			break
		}
	}
	if event.ID == "" || event.Type != "eval.asset_promoted" || event.Payload["asset_kind"] != "suite" {
		t.Fatalf("expected eval.asset_promoted event, got %#v", event)
	}
}

func TestEvalReportCommandReadsRunnerReport(t *testing.T) {
	root := t.TempDir()
	writeEvalRunFixture(t, root)
	restoreEvalFlags(t)
	evalRoot = root
	evalRunSuite = "default"
	evalRunScenario = "eval-smoke"
	evalRunCommand = "definitely-not-a-codex-command"
	evalRunTimeout = time.Second

	runCmd, _ := testCommand()
	if err := runEvalRun(runCmd, nil); err != nil {
		t.Fatalf("runEvalRun returned error: %v", err)
	}
	matches, err := filepath.Glob(filepath.Join(root, ".mnemon", "harness", "reports", "runner", "*-codex-app-server-semantic-run.json"))
	if err != nil {
		t.Fatalf("glob runner reports: %v", err)
	}
	if len(matches) != 1 {
		t.Fatalf("expected one runner report, got %v", matches)
	}
	evalReportRunID = strings.TrimSuffix(filepath.Base(matches[0]), "-codex-app-server-semantic-run.json")
	evalReportFormat = "text"

	reportCmd, output := testCommand()
	if err := runEvalReport(reportCmd, nil); err != nil {
		t.Fatalf("runEvalReport returned error: %v", err)
	}
	for _, want := range []string{
		"Eval report " + evalReportRunID,
		"Status: blocked",
		"Job: eval_default_eval_smoke (eval.eval-smoke)",
		"Turns: 0",
	} {
		if !strings.Contains(output.String(), want) {
			t.Fatalf("expected %q in output:\n%s", want, output.String())
		}
	}
}

func TestEvalReplayCommand(t *testing.T) {
	root := t.TempDir()
	writeEvalReplayCommandFixture(t, root)
	restoreEvalFlags(t)
	evalRoot = root
	evalReplayTier = "1,2"

	cmd, output := testCommand()
	if err := runEvalReplay(cmd, nil); err != nil {
		t.Fatalf("runEvalReplay returned error: %v", err)
	}
	for _, want := range []string{"regression replay: pass", "tiers: 1,2", "checks: 4", "report:"} {
		if !strings.Contains(output.String(), want) {
			t.Fatalf("expected %q in output:\n%s", want, output.String())
		}
	}
	matches, err := filepath.Glob(filepath.Join(root, ".mnemon", "harness", "reports", "regression", "replay-*.json"))
	if err != nil {
		t.Fatalf("glob replay report: %v", err)
	}
	if len(matches) != 1 {
		t.Fatalf("expected one replay report, got %v", matches)
	}
}

func restoreEvalFlags(t *testing.T) {
	t.Helper()
	oldRoot := evalRoot
	oldSuite := evalPlanSuite
	oldFormat := evalPlanFormat
	oldRunSuite := evalRunSuite
	oldRunScenario := evalRunScenario
	oldRunHost := evalRunHost
	oldRunCommand := evalRunCommand
	oldRunTimeout := evalRunTimeout
	oldRunTurnTimeout := evalRunTurnTimeout
	oldRunMaxTurns := evalRunMaxTurns
	oldRunIsolatedHome := evalRunIsolatedHome
	oldRunAgentTurn := evalRunAgentTurn
	oldRunAcknowledgeCost := evalRunAcknowledgeModelCost
	oldAssertSuite := evalAssertSuite
	oldAssertScenario := evalAssertScenario
	oldAssertRunID := evalAssertRunID
	oldABSuite := evalABSuite
	oldABScenarios := append([]string(nil), evalABScenarios...)
	oldABTrialsPerArm := evalABTrialsPerArm
	oldABCommand := evalABCommand
	oldABTimeout := evalABTimeout
	oldABTurnTimeout := evalABTurnTimeout
	oldABMaxTurns := evalABMaxTurns
	oldABIsolatedHome := evalABIsolatedHome
	oldABAgentTurn := evalABAgentTurn
	oldABAcknowledgeCost := evalABAcknowledgeModelCost
	oldABControlSetupJSON := evalABControlSetupJSON
	oldABTreatmentSetupJSON := evalABTreatmentSetupJSON
	oldPromoteScenario := evalPromoteScenario
	oldPromoteSuite := evalPromoteSuite
	oldPromoteRubric := evalPromoteRubric
	oldPromoteTarget := evalPromoteTarget
	oldPromoteFrom := evalPromoteFrom
	oldPromoteProposalRef := evalPromoteProposalRef
	oldPromoteAuditRef := evalPromoteAuditRef
	oldPromoteEventID := evalPromoteEventID
	oldPromoteCorrelationID := evalPromoteCorrelationID
	oldPromoteCausedBy := evalPromoteCausedBy
	oldReportRunID := evalReportRunID
	oldReportFormat := evalReportFormat
	oldReplayTier := evalReplayTier
	oldReplayFormat := evalReplayFormat
	t.Cleanup(func() {
		evalRoot = oldRoot
		evalPlanSuite = oldSuite
		evalPlanFormat = oldFormat
		evalRunSuite = oldRunSuite
		evalRunScenario = oldRunScenario
		evalRunHost = oldRunHost
		evalRunCommand = oldRunCommand
		evalRunTimeout = oldRunTimeout
		evalRunTurnTimeout = oldRunTurnTimeout
		evalRunMaxTurns = oldRunMaxTurns
		evalRunIsolatedHome = oldRunIsolatedHome
		evalRunAgentTurn = oldRunAgentTurn
		evalRunAcknowledgeModelCost = oldRunAcknowledgeCost
		evalAssertSuite = oldAssertSuite
		evalAssertScenario = oldAssertScenario
		evalAssertRunID = oldAssertRunID
		evalABSuite = oldABSuite
		evalABScenarios = oldABScenarios
		evalABTrialsPerArm = oldABTrialsPerArm
		evalABCommand = oldABCommand
		evalABTimeout = oldABTimeout
		evalABTurnTimeout = oldABTurnTimeout
		evalABMaxTurns = oldABMaxTurns
		evalABIsolatedHome = oldABIsolatedHome
		evalABAgentTurn = oldABAgentTurn
		evalABAcknowledgeModelCost = oldABAcknowledgeCost
		evalABControlSetupJSON = oldABControlSetupJSON
		evalABTreatmentSetupJSON = oldABTreatmentSetupJSON
		evalPromoteScenario = oldPromoteScenario
		evalPromoteSuite = oldPromoteSuite
		evalPromoteRubric = oldPromoteRubric
		evalPromoteTarget = oldPromoteTarget
		evalPromoteFrom = oldPromoteFrom
		evalPromoteProposalRef = oldPromoteProposalRef
		evalPromoteAuditRef = oldPromoteAuditRef
		evalPromoteEventID = oldPromoteEventID
		evalPromoteCorrelationID = oldPromoteCorrelationID
		evalPromoteCausedBy = oldPromoteCausedBy
		evalReportRunID = oldReportRunID
		evalReportFormat = oldReportFormat
		evalReplayTier = oldReplayTier
		evalReplayFormat = oldReplayFormat
	})
	evalRoot = "."
	evalPlanSuite = "default"
	evalPlanFormat = "text"
	evalRunSuite = "default"
	evalRunScenario = ""
	evalRunHost = ""
	evalRunCommand = "codex"
	evalRunTimeout = 5 * time.Minute
	evalRunTurnTimeout = 3 * time.Minute
	evalRunMaxTurns = 0
	evalRunIsolatedHome = false
	evalRunAgentTurn = false
	evalRunAcknowledgeModelCost = false
	evalAssertSuite = "default"
	evalAssertScenario = ""
	evalAssertRunID = ""
	evalABSuite = "default"
	evalABScenarios = nil
	evalABTrialsPerArm = 1
	evalABCommand = "codex"
	evalABTimeout = 5 * time.Minute
	evalABTurnTimeout = 3 * time.Minute
	evalABMaxTurns = 0
	evalABIsolatedHome = false
	evalABAgentTurn = false
	evalABAcknowledgeModelCost = false
	evalABControlSetupJSON = ""
	evalABTreatmentSetupJSON = ""
	evalPromoteScenario = ""
	evalPromoteSuite = ""
	evalPromoteRubric = ""
	evalPromoteTarget = "promoted"
	evalPromoteFrom = ""
	evalPromoteProposalRef = ""
	evalPromoteAuditRef = ""
	evalPromoteEventID = ""
	evalPromoteCorrelationID = ""
	evalPromoteCausedBy = ""
	evalReportRunID = ""
	evalReportFormat = "text"
	evalReplayTier = "1"
	evalReplayFormat = "text"
}

func createEvalCommandApprovedProposal(t *testing.T, root, id string) string {
	t.Helper()
	store, err := proposalstore.New(root)
	if err != nil {
		t.Fatalf("proposalstore.New returned error: %v", err)
	}
	now := time.Date(2026, 5, 27, 11, 0, 0, 0, time.UTC)
	if _, err := store.Create(proposalstore.CreateOptions{
		ID:      id,
		Route:   proposal.RouteEval,
		Risk:    proposal.RiskLow,
		Title:   "Promote eval suite",
		Summary: "Approve a fixture eval suite promotion.",
		Change: proposal.ChangeRequest{
			Summary: "Promote eval suite.",
			Targets: []proposal.TargetRef{{
				Type: "eval_asset",
				URI:  "harness/loops/eval/suites/default.json",
			}},
		},
		ValidationPlan: proposal.ValidationPlan{Summary: "Run CLI promotion test."},
		Now:            now,
	}); err != nil {
		t.Fatalf("Create proposal returned error: %v", err)
	}
	for index, status := range []proposal.Status{proposal.StatusOpen, proposal.StatusInReview, proposal.StatusApproved} {
		if _, err := store.Transition(proposalstore.TransitionOptions{
			ID:     id,
			Status: status,
			Now:    now.Add(time.Duration(index+1) * time.Second),
		}); err != nil {
			t.Fatalf("Transition proposal to %s returned error: %v", status, err)
		}
	}
	return id
}

func writeEvalReplayCommandFixture(t *testing.T, root string) {
	t.Helper()
	suiteDir := filepath.Join(root, "harness", "loops", "eval", "suites")
	scenarioDir := filepath.Join(root, "harness", "loops", "eval", "scenarios")
	for _, dir := range []string{suiteDir, scenarioDir, filepath.Join(scenarioDir, "ops")} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}
	if err := os.WriteFile(filepath.Join(suiteDir, "smoke.json"), []byte(`{
  "name": "smoke",
  "scenarios": ["ops/host-projection-smoke"]
}`), 0o644); err != nil {
		t.Fatalf("write smoke suite: %v", err)
	}
	if err := os.WriteFile(filepath.Join(suiteDir, "regression.json"), []byte(`{
  "name": "regression",
  "scenario_ids": ["memory-focused-recall"]
}`), 0o644); err != nil {
		t.Fatalf("write regression suite: %v", err)
	}
	if err := os.WriteFile(filepath.Join(scenarioDir, "ops", "host-projection-smoke.md"), []byte("# Host Projection Smoke\n"), 0o644); err != nil {
		t.Fatalf("write markdown scenario: %v", err)
	}
	if err := os.WriteFile(filepath.Join(scenarioDir, "codex-app.json"), []byte(`{
  "scenarios": [
    {
      "id": "memory-focused-recall",
      "loops": ["memory"],
      "prompts": ["Recall the seeded project preference."]
    }
  ]
}`), 0o644); err != nil {
		t.Fatalf("write scenario catalog: %v", err)
	}
}

func writeEvalRunFixture(t *testing.T, root string) {
	t.Helper()
	loopDir := filepath.Join(root, "harness", "loops", "eval")
	scenarioDir := filepath.Join(loopDir, "scenarios")
	hostDir := filepath.Join(root, "harness", "hosts", "codex")
	bindingDir := filepath.Join(root, "harness", "bindings")
	for _, dir := range []string{
		filepath.Join(loopDir, "hook-prompts"),
		filepath.Join(loopDir, "skills", "eval-run"),
		filepath.Join(loopDir, "suites"),
		scenarioDir,
		hostDir,
		bindingDir,
	} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}
	for _, path := range []string{
		filepath.Join(loopDir, "GUIDE.md"),
		filepath.Join(loopDir, "env.sh"),
		filepath.Join(loopDir, "README.md"),
		filepath.Join(loopDir, "hook-prompts", "prime.md"),
		filepath.Join(loopDir, "hook-prompts", "remind.md"),
		filepath.Join(loopDir, "hook-prompts", "nudge.md"),
		filepath.Join(loopDir, "hook-prompts", "compact.md"),
		filepath.Join(loopDir, "skills", "eval-run", "SKILL.md"),
	} {
		if err := os.WriteFile(path, []byte("fixture\n"), 0o644); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}
	if err := os.WriteFile(filepath.Join(loopDir, "suites", "default.json"), []byte(`{
  "name": "default",
  "host": "codex",
  "runner": "codex-app-server",
  "scenario_ids": ["eval-smoke"]
}`), 0o644); err != nil {
		t.Fatalf("write suite: %v", err)
	}
	if err := os.WriteFile(filepath.Join(scenarioDir, "codex-app.json"), []byte(`{
  "schema_version": 1,
  "name": "codex-app",
  "scenarios": [
    {
      "id": "eval-smoke",
      "area": "eval",
      "loops": ["eval"],
      "setup_handler": "setup_local_fact",
      "assertion_handler": "assert_eval_smoke",
      "prompts": ["Use the declared eval smoke prompt."]
    }
  ]
}`), 0o644); err != nil {
		t.Fatalf("write scenario catalog: %v", err)
	}
	if err := os.WriteFile(filepath.Join(loopDir, "loop.json"), []byte(`{
  "schema_version": 2,
  "name": "eval",
  "control_model": {
    "state": [],
    "intent": "fixture",
    "reality": [],
    "reconcile": []
  },
  "entity_profiles": {},
  "surfaces": {
    "projection": [],
    "observation": []
  },
  "assets": {
    "guide": "GUIDE.md",
    "env": "env.sh",
    "runtime_files": ["README.md"],
    "hook_prompts": {
      "prime": "hook-prompts/prime.md",
      "remind": "hook-prompts/remind.md",
      "nudge": "hook-prompts/nudge.md",
      "compact": "hook-prompts/compact.md"
    },
    "skills": ["skills/eval-run/SKILL.md"],
    "subagents": []
  },
  "host_adapters": {
    "codex": "../../hosts/codex"
  }
}`), 0o644); err != nil {
		t.Fatalf("write loop manifest: %v", err)
	}
	if err := os.WriteFile(filepath.Join(hostDir, "host.json"), []byte(`{
  "schema_version": 2,
  "name": "codex",
  "surfaces": {
    "projection": [".codex/skills"],
    "observation": []
  },
  "lifecycle_mapping": {}
}`), 0o644); err != nil {
		t.Fatalf("write host manifest: %v", err)
	}
	if err := os.WriteFile(filepath.Join(bindingDir, "codex.eval.json"), []byte(`{
  "schema_version": 1,
  "name": "codex.eval",
  "host": "codex",
  "loop": "eval",
  "projection_path": ".codex",
  "runtime_surface": ".codex/mnemon-eval",
  "lifecycle_mapping": {},
  "reconcile": []
}`), 0o644); err != nil {
		t.Fatalf("write binding manifest: %v", err)
	}
}

func writeFile(t *testing.T, root, rel, content string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
