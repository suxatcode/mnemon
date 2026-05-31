---
name: mnemon-goal
description: Manage project-scoped Mnemon goal state, evidence, verification, completion, blockers, and host goal links.
---

# mnemon-goal

Use this skill when a task should be tracked as a durable Mnemon project goal
or when an existing goal needs plan, evidence, verification, completion,
blocked, paused, resumed, or host-link updates.

## Boundary

This skill uses `mnemon-harness goal` commands. It does not replace Codex
`/goal`, Claude Code continuation behavior, or any host-owned planning state.
It must not write Codex internal sqlite state, Claude internal state, or other
private host runtime databases.

Mnemon owns project goal records under `.mnemon/harness/goals`. The host agent
owns the work.

## Runtime

If `MNEMON_GOAL_LOOP_ENV` is set and the expected variables are missing, source
it before running commands:

```bash
source "$MNEMON_GOAL_LOOP_ENV"
```

Useful variables:

```text
MNEMON_GOAL_LOOP_ROOT
MNEMON_GOAL_LOOP_GOALS_DIR
MNEMON_GOAL_LOOP_STATUS_DIR
```

Default to the current repository root when variables are unavailable.

## Create

Create a goal when the work is multi-step, evidence-sensitive, or likely to
span handoff/compaction:

```bash
mnemon-harness goal init --root . --objective "<objective>"
```

Use `--goal-id <id>` only when the user or existing state requires a stable id.

## Plan

Record or update the plan before substantial work:

```bash
mnemon-harness goal plan --root . --goal-id <goal-id> \
  --summary "<short plan summary>" \
  --step "<step 1>" \
  --step "<step 2>"
```

Add refs when useful:

```bash
--memory-ref "<memory ref>"
--memory-recall "<recall request>"
--skill-ref "<skill/workflow ref>"
--eval-ref "<eval/report ref>"
```

## Record Evidence

Record evidence when a durable result is produced:

```bash
mnemon-harness goal evidence append --root . --goal-id <goal-id> \
  --type manual \
  --status accepted \
  --summary "<what changed and why it satisfies part of the goal>"
```

Attach refs when they exist:

```bash
--artifact-ref "<path or artifact ref>"
--eval-report-ref "<report ref>"
--audit-ref "<audit ref>"
--proposal-ref "<proposal ref>"
--host-evidence-ref "<host thread or public goal ref>"
```

Do not record raw secrets or private host database paths as evidence.

## Verify And Complete

Before claiming completion:

```bash
mnemon-harness goal verify --root . --goal-id <goal-id> \
  --gate "<gate name>" \
  --summary "<verification result>"
```

Then complete only after accepted evidence and verification exist:

```bash
mnemon-harness goal complete --root . --goal-id <goal-id>
```

After a successful completion, emit a best-effort daemon event so declarative
daemon jobs can react:

```bash
mnemon event emit goal.completed \
  --loop goal \
  --payload '{"goal_id":"<goal-id>","source":"mnemon-goal"}'
```

If emit fails or `mnemon` is unavailable, continue without retrying; the
Mnemon goal completion remains canonical.

Use `--block-on-failure` when a failed completion should become a durable
blocked state instead of only returning an error.

## Block, Pause, Resume

Use blocked for an impasse that needs external input or changed conditions:

```bash
mnemon-harness goal block --root . --goal-id <goal-id> --reason "<reason>"
```

Use pause/resume for intentional scheduling state:

```bash
mnemon-harness goal pause --root . --goal-id <goal-id> --reason "<reason>"
mnemon-harness goal resume --root . --goal-id <goal-id> --reason "<reason>"
```

## Host Link

Link public host identifiers only when they are available through supported
host APIs or visible user-provided refs:

```bash
mnemon-harness goal link --root . --goal-id <goal-id> \
  --host codex \
  --thread-id "<public thread id>" \
  --evidence "<source ref>"
```

Do not inspect or mutate host internal storage to discover ids.

## Codex `/goal`

For Codex, generate the host-owned `/goal` prompt snippet from Mnemon state:

```bash
mnemon-harness goal codex prompt --root . --goal-id <goal-id>
```

The generated `/goal` text delegates work to Codex while keeping Mnemon as the
durable verification and evidence plane.

## Safety

Current user instructions and repository state override stale goal text. If the
goal objective conflicts with the user, stop and ask before continuing. If
verification evidence is missing, do not mark the goal complete.
