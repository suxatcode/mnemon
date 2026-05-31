# Mnemon Eval Loop Harness

This directory is the canonical eval loop template. It is a feedback-facing loop:
it designs and runs realistic harness scenarios, collects evidence, and turns
stable failures into curated improvement candidates.

The eval loop is not a parent of memory or skill. It is a peer loop
that can evaluate interface-facing loops, host projection, setup, documentation
workflow, commit discipline, and its own eval assets.

## File Tree

```text
harness/loops/eval/
├── README.md
├── loop.json
├── env.sh
├── GUIDE.md
├── hook-prompts/
├── skills/
│   ├── eval-plan/
│   │   └── SKILL.md
│   ├── eval-run/
│   │   └── SKILL.md
│   ├── eval-analyze/
│   │   └── SKILL.md
│   └── eval-improve/
│       └── SKILL.md
├── subagents/
├── scenarios/
├── suites/
└── rubrics/
```

## Core Parts

| Part | Role |
| --- | --- |
| Scenario | A reproducible task pressure case with target, setup, prompt, evidence, and expected observations. |
| Suite | A named group of scenarios with host and loop configuration. |
| Rubric | Review criteria used to judge behavior, stability, and improvement value. |
| Runner | Host-specific machinery that starts isolated workspaces and drives a HostAgent. Codex app server is the current primary runner. |
| Report | Durable output containing transcript references, diffs, loop state, judgement, and next actions. |

## Eval Asset Lifecycle

Eval assets are stricter than skill assets because they define how the project
judges improvement. New assets should not become canonical immediately.

```text
ephemeral -> candidate -> promoted -> canonical -> retired
```

- `ephemeral`: one-off exploration in `scratch`; no review required.
- `candidate`: generated or proposed asset with initial evidence.
- `promoted`: curated asset suitable for local regression.
- `canonical`: stable asset suitable for long-term comparison or gates.
- `retired`: obsolete, flaky, or superseded asset kept for audit.

## Runtime Directory Protocol

Installed runtime state resolves through one environment config:

```text
$MNEMON_EVAL_LOOP_DIR/
├── env.sh
├── GUIDE.md
├── scratch/
├── candidates/
├── reports/
├── artifacts/
└── retired/
```

`env.sh` defines:

```bash
MNEMON_EVAL_LOOP_ENV=<canonical-state>/harness/eval/env.sh
MNEMON_EVAL_LOOP_DIR=<canonical-state>/harness/eval
MNEMON_EVAL_LOOP_SCRATCH_DIR=$MNEMON_EVAL_LOOP_DIR/scratch
MNEMON_EVAL_LOOP_CANDIDATES_DIR=$MNEMON_EVAL_LOOP_DIR/candidates
MNEMON_EVAL_LOOP_REPORTS_DIR=$MNEMON_EVAL_LOOP_DIR/reports
MNEMON_EVAL_LOOP_ARTIFACTS_DIR=$MNEMON_EVAL_LOOP_DIR/artifacts
MNEMON_EVAL_LOOP_RETIRED_DIR=$MNEMON_EVAL_LOOP_DIR/retired
```

## Codex Install

Install into the current project:

```bash
bash harness/ops/install.sh --host codex --loop eval
```

Check status:

```bash
bash harness/ops/status.sh --host codex --loop eval
```

Remove the installed Codex integration while preserving reports and candidates:

```bash
bash harness/ops/uninstall.sh --host codex --loop eval
```

Existing project-local Codex app-server eval commands remain available through
`make codex-app-eval-suite`, `make codex-memory-deep-eval`, and
`make codex-skill-deep-eval`.

Codex app-server suite membership lives in `suites/*.json` as `scenario_ids`.
Scenario runtime metadata for the compatibility runner lives in
`scenarios/codex-app.json`: prompts, loop requirements, expected skills, and
the Python setup/assertion handler names that still provide compatibility
checks. The Go harness CLI can plan and start a gated runner workspace from the
same declarations:

```bash
mnemon-harness eval run --suite default --scenario memory-focused-recall
mnemon-harness eval report --run-id <run-id>
```
