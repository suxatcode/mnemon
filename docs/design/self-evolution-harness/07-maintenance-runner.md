# 07. Optional Maintenance Runner

Harness core does not need a daemon. A daemon is only justified for maintenance work that is periodic, low-priority, evidence-heavy, and unsafe to run inside an active user turn. The right abstraction is therefore not an agent runtime, but a **maintenance runner**:

```text
cron / host scheduler / manual CLI
  -> runner tick
  -> lease
  -> budget
  -> scoped job
  -> report / proposal / allowlisted apply
  -> ledger
```

The runner is optional. L0/L1 installs should not include it. L2 can usually rely on host lifecycle hooks. L3/L4 may install it when the host lacks a scheduler or when dreaming/index/eval jobs need a durable execution surface.

## Architectural Position

The runner lives outside the host agent loop.

| Surface | Owner | Runner role |
|---|---|---|
| User conversation | host | none |
| Main system prompt | host | none |
| Tool routing | host | none |
| Permission approval | host | none |
| LLM client | host | calls declared host command only when configured |
| Hook bus | host | consumes queued maintenance jobs only |
| Maintenance state | harness | read/write through declared schemas |
| Reports/proposals | harness | write audit records |

This changes the earlier "no runtime" rule into a more precise rule:

```text
No mandatory agent runtime.
Optional maintenance runtime is allowed.
The optional runtime must not become an agent.
```

## Why It Exists

Some self-evolution tasks are bad foreground work:

| Workload | Why foreground is poor | Runner value |
|---|---|---|
| Dreaming | large cold evidence, long context, weak relevance to current user turn | run when idle, summarize, propose promotion |
| Curator | scans many skills/memory files, requires snapshots | controlled dry-run/apply loop |
| Post-turn review fallback | some hosts cannot run immediate `Stop` hooks | process queued session summaries later |
| Cold index rebuild | deterministic but potentially expensive | rebuild outside conversation |
| Eval batch | needs repeated checks and held-out examples | write PR-style proposal |
| Backup rotation | unrelated to active task | bounded housekeeping |

The runner is not required for Hermes-style post-turn review when the host already supports a background review agent. In that case the harness only provides the reflection prompt, provenance schema, and write policy.

## Non-Goals

The runner must not:

- handle user messages;
- assemble the main prompt;
- inject memory directly into live turns;
- intercept host LLM calls;
- hold a separate model API key by default;
- route arbitrary tools;
- maintain host session state;
- approve dangerous actions;
- watch the whole filesystem and mutate files opportunistically;
- install host adapters at runtime;
- become a plugin system.

If a proposed feature needs any of these, it belongs in the host agent or in an explicit host binding, not in the harness runner.

## Runner Components

| Component | Responsibility | Constraint |
|---|---|---|
| Job loader | load `runner/jobs/*.yaml` and queued JSON jobs | schema validation required |
| Trigger evaluator | decide whether a job is due | no busy loop required |
| Lease manager | avoid concurrent mutation | stale-safe locks |
| Budget manager | runtime, file, token/char, LLM-call limits | fail closed |
| Executor | run a scoped script/prompt/host command | declared command only |
| Validator | validate outputs and target paths | before writes |
| Ledger | append durable job records | every attempt |
| Reporter | write Markdown + machine-readable report | report-first |

The smallest valid implementation can be a CLI invoked by cron:

```text
mnemon-runner tick --root .mnemon
```

A resident process is only an optimization. The semantics must stay the same as one tick.

## Job Descriptor

`runner/jobs/*.yaml` declares recurring jobs. Defaults should be disabled until installation explicitly enables them.

```yaml
job:
  id: dreaming-nightly
  type: dreaming.deep
  enabled: false
  trigger:
    kind: schedule
    interval_hours: 24
    min_idle_minutes: 30
  mode: dry-run
  inputs:
    - memory/warm/**
    - memory/cold/evidence/**
    - state/usage.json
    - state/pins.json
  outputs:
    - reports/dreaming/**
    - memory/warm/candidates/**
  write_allowlist:
    - reports/dreaming/**
    - memory/warm/candidates/**
    - state/jobs/**
  budgets:
    max_runtime_seconds: 1800
    max_llm_calls: 8
    max_input_chars: 200000
    max_output_chars: 30000
    max_files_touched: 50
  locking:
    resources:
      - memory
      - usage
    stale_after_seconds: 7200
  kill_switch:
    file: state/runner.disabled
```

## Job Taxonomy

| Type | Uses LLM | Default write mode | Output |
|---|---:|---|---|
| `reflect.deferred` | yes | proposal | `reports/reflection/*`, optional proposal patch |
| `curator.transitions` | no | apply to state only | usage state transitions, stale markers |
| `curator.review` | yes | dry-run/proposal | consolidation/archive proposal |
| `dreaming.light` | no/optional | warm candidate write | candidate extraction from recent evidence |
| `dreaming.rem` | yes | report-only | theme report |
| `dreaming.deep` | yes | proposal | promotion/demotion proposals |
| `cold.index.incremental` | no | apply to index only | FTS/vector metadata |
| `cold.index.rebuild` | no | apply to index only | rebuilt index |
| `eval.batch` | yes/optional | proposal | eval report / PR text |
| `snapshot.rotate` | no | apply | backup manifest cleanup |
| `archive.compress` | no | apply to archive only | cold archive compaction |

LLM jobs are always optional. If the host does not expose an approved LLM invocation command, LLM jobs stay manual or proposal-only.

## LLM Invocation Contract

The runner must not embed its own agent loop. When a job needs language-model judgment, it calls a host-declared command:

```yaml
host_llm:
  command: ["claude", "-p"]
  stdin: prompt
  timeout_seconds: 600
  output_schema: schemas/proposal.schema.json
  allowed_tools: []
```

Rules:

- prompts are scoped job prompts, not full agent prompts;
- no arbitrary tool use unless the host command explicitly exposes a safe mode;
- output must validate before any apply step;
- failed schema validation writes a report and stops;
- missing host command downgrades the job to report-only/manual.

This keeps the runner from becoming a second agent while still allowing Hermes-style review or OpenClaw-style dreaming where the host supports it.

Stronger rule:

```text
one job step -> one scoped prompt -> one bounded LLM response -> schema validation
```

Multi-step jobs must be declared as explicit steps:

```yaml
steps:
  - id: extract-candidates
    llm: false
  - id: consolidate-themes
    llm: true
    prompt: prompts/dreaming-rem.md
  - id: score-promotions
    llm: true
    prompt: prompts/dreaming-deep.md
```

The runner cannot run an open-ended observe/think/act loop. It cannot ask the model to choose arbitrary tools. Each step has declared inputs, outputs, budgets, and schema.

## Queued Jobs

Hosts with limited hook support can enqueue maintenance work instead of running it inline.

```text
state/jobs/
  queue/
    reflect/
      <session-id>.json
  running/
  done/
    2026-05-08/
  failed/
```

Queued reflection job:

```json
{
  "schema_version": 1,
  "job_type": "reflect.deferred",
  "session_id": "abc",
  "created_at": "2026-05-08T00:00:00Z",
  "cwd": "/repo",
  "summary_ref": "memory/warm/sessions/abc.md",
  "allowed_targets": ["memory/hot/**", "skills/**", "reports/**"],
  "mode": "proposal"
}
```

The queue stores summaries and references, not raw unbounded transcripts. Raw transcripts remain cold evidence and are summarized before LLM use.

## Lease And Locking

The runner uses file leases, not in-memory locks.

```json
{
  "resource": "memory",
  "holder": "host:pid:job-id",
  "acquired_at": "2026-05-08T00:00:00Z",
  "expires_at": "2026-05-08T00:30:00Z",
  "heartbeat_at": "2026-05-08T00:05:00Z"
}
```

Lock rules:

- acquire resources in deterministic order;
- foreground host actions have priority over maintenance;
- stale locks can be broken only after `expires_at`;
- lock failure skips the job and records `skipped_locked`;
- apply mode requires exclusive lock over every mutated resource;
- report-only mode can run with read locks.

Foreground activity can be signaled by:

```text
state/host_activity.json
```

If the host is active, expensive jobs should defer unless explicitly manual.

## Budgets And Backoff

Budgets are part of the safety model, not performance tuning.

Required budgets:

- max runtime;
- max LLM calls;
- max input chars;
- max output chars;
- max files scanned;
- max files mutated;
- max report size;
- retry count and backoff window.

Failure behavior:

| Failure | Behavior |
|---|---|
| Budget exceeded | stop, write partial report, no apply |
| Schema invalid | stop, write validation error |
| Protected target requested | downgrade to proposal |
| Lock unavailable | skip with ledger record |
| Repeated transient errors | pause job until manual review |
| Kill switch present | skip all jobs |

Kill switches:

```text
state/runner.disabled
state/runner.disabled.<job-type>
state/maintenance_disabled
```

## Write Safety

Apply is allowed only when all gates pass:

```text
job.enabled == true
AND mode == apply
AND lease acquired
AND backup succeeded
AND output schema valid
AND target in job write_allowlist
AND target in global allowlist
AND target not protected
AND target not pinned
AND provenance allows automated mutation
```

Protected by default:

- `INSTALL.md`
- `GUIDELINE.md`
- `harness.yaml`
- `install/**`
- `hooks/**`
- `schemas/**`
- `eval/**`
- package-provided skills
- user-created skills and memory

The default result of high-risk work is a proposal report.

## Ledger

Every attempt writes a machine-readable ledger entry:

```json
{
  "schema_version": 1,
  "job_id": "dreaming-nightly",
  "job_type": "dreaming.deep",
  "status": "proposal_written",
  "mode": "dry-run",
  "started_at": "2026-05-08T00:00:00Z",
  "finished_at": "2026-05-08T00:12:00Z",
  "inputs": ["memory/warm/**", "memory/cold/evidence/**"],
  "outputs": ["reports/dreaming/2026-05-08.md"],
  "budgets": {
    "llm_calls": 3,
    "input_chars": 84500,
    "output_chars": 9400
  },
  "mutations": [],
  "warnings": []
}
```

Reports are for humans; ledger is for later curator/eval.

## Dreaming Through Runner

Dreaming is the strongest runner use case because it is not a foreground capability.

```text
Light:
  recent cold evidence + warm sessions
    -> candidate facts/workflows/topics
    -> memory/warm/candidates/*

REM:
  candidates + usage + recent reports
    -> theme consolidation
    -> reports/dreaming/*

Deep:
  candidates + evidence links + usage frequency
    -> promotion/demotion proposals
    -> reports/dreaming/*
```

Dreaming promotion rules:

- raw evidence is never promoted directly;
- every proposed hot-memory entry links evidence;
- procedures become skill proposals, not memory;
- high-risk guideline/hook/install changes are proposal-only;
- hot memory writes require explicit apply or human approval.

## Review-Agent Skill Creation Through Runner

Hermes uses background review to create or patch skills after a turn. In the harness architecture, that behavior is represented as a `reflect.deferred` job or host-native post-turn hook:

```text
completed turn summary
  -> reflection prompt
  -> classify: memory vs skill vs session note
  -> patch existing skill if possible
  -> create new skill only for reusable workflow
  -> write report
  -> apply only low-risk allowlisted targets
```

The runner can execute this only from queued summaries. It must not reopen or mutate the active conversation.

## Installation Modes

Preferred order:

1. Host-native scheduler or hook.
2. External cron/CI invoking `runner tick`.
3. Optional local runner process.
4. Manual `curate` / `dreaming` / `reflect` skills.

The architecture should be specified so mode 2 and mode 3 are equivalent. If a resident daemon behaves differently from a cron tick, the daemon has too much authority.

## Acceptance Criteria

The runner design is acceptable only if:

1. disabling the runner does not disable recall/reflect/curate skills;
2. all LLM work can degrade to proposal-only;
3. every write has report and ledger evidence;
4. host foreground work can preempt maintenance;
5. no job owns arbitrary tool routing;
6. no job writes outside declared targets;
7. uninstalling the runner preserves memory/reports/state;
8. a generic agent can still install L0/L1 with only Markdown.
