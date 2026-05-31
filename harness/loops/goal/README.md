# Mnemon Goal Loop Harness

This directory is the canonical goal loop template. It gives a host agent a
small skill for using project-scoped Mnemon goal state without replacing the
host's own continuation mechanism.

The goal loop is a governance loop. It records objective, plan, evidence,
verification, completion, host links, and blocked/paused state under
`.mnemon/harness`.

## File Tree

```text
harness/loops/goal/
├── README.md
├── loop.json
├── env.sh
├── GUIDE.md
├── hook-prompts/
├── skills/
│   └── mnemon-goal/
│       └── SKILL.md
└── subagents/
    └── cross-goal-consolidator.md
```

## Runtime Directory Protocol

Installed runtime state resolves through one environment config:

```text
$MNEMON_GOAL_LOOP_DIR/
├── env.sh
├── GUIDE.md
└── loop.json
```

Goal records live separately because `mnemon-harness goal` owns their layout:

```text
.mnemon/harness/goals/<goal-id>/
├── goal.json
├── GOAL.md
├── PLAN.md
├── EVIDENCE.jsonl
└── REPORT.md
```

## Host Boundary

Codex `/goal` and Claude Code continuation behavior remain host-owned. Mnemon
stores durable project goal state and completion evidence. The host agent still
does the work.

## Install

Install into Codex:

```bash
bash harness/ops/install.sh --host codex --loop goal
```

Install into Claude Code:

```bash
bash harness/ops/install.sh --host claude-code --loop goal
```
