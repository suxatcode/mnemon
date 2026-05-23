# AI-Native Lifecycle Architecture

Chinese version: [LIFECYCLE_RUNTIME.md](../zh/harness/LIFECYCLE_RUNTIME.md)

Site version: [AI-Native Lifecycle Architecture](../site/lifecycle-runtime/index.html)

End-to-end user/session flow: [System Flow](SYSTEM_FLOW.md).

This document consolidates the architecture direction that emerges from the
memory-loop, skill-loop, eval-loop, lifecycle control-plane, event-sourced
runtime, daemon, Codex app-server, and subagent/job-spec discussions.

Mnemon is an event-sourced lifecycle layer for agents you already use, not a
replacement agent runtime. It gives existing hosts durable memory, skill
evolution, eval, policy, proposal, and audit lifecycles without taking over
task execution.

It is not a daemon-only design. The daemon is one important runtime component,
but the architecture is larger:

```text
Concept model
  -> event-sourced lifecycle substrate
  -> host projection
  -> AI-native execution surfaces
  -> deterministic and LLM-supervised reactors
  -> governed materialized state
```

## Thesis

Mnemon should remain an external lifecycle architecture for existing agent
runtimes. It should not replace the host's ReAct loop, model runtime, UI,
permission system, or native tool execution.

The boundary is deliberately sharp:

```text
Mnemon does not orchestrate task execution.
Mnemon orchestrates lifecycle capabilities.
Host surfaces are projections; .mnemon owns canonical lifecycle state.
```

The core architectural move is:

```text
Use deterministic machinery for lifecycle structure.
Use HostAgent / LLM supervision for semantic judgment.
Use append-only lifecycle events to make both auditable.
```

The result is an AI-native lifecycle system:

```text
host-native hooks / skills / subagents / app-server sessions
        +
event-sourced lifecycle state
        +
daemon-backed scheduling and materialization
        +
LLM-supervised job execution
        +
governed proposals, reports, and eval evidence
```

## Layered Architecture

```text
+------------------------------------------------------------+
| Host Agent Runtime                                         |
| Codex, Claude Code, OpenClaw, Nanobot, future hosts        |
| Owns ReAct loop, model calls, tools, permissions, UI       |
+--------------------------+---------------------------------+
                           |
                           | hooks / skills / app-server / CLI
                           v
+------------------------------------------------------------+
| Host Projection Layer                                      |
| Generated .codex, .claude, hooks, skills, env, job specs   |
| Host-readable, repairable, not canonical state             |
+--------------------------+---------------------------------+
                           |
                           | observed lifecycle activity
                           v
+------------------------------------------------------------+
| Lifecycle Event Substrate                                  |
| append-only events, correlation, caused_by, lineage        |
| source of truth for lifecycle changes                      |
+--------------------------+---------------------------------+
                           |
                           | materialize / schedule / dispatch
                           v
+------------------------------------------------------------+
| Lifecycle Runtime                                          |
| daemon, queues, locks, deterministic reactors, validators  |
| watches events, checks thresholds, repairs projections     |
+--------------------------+---------------------------------+
                           |
         +-----------------+------------------+
         |                                    |
         v                                    v
+----------------------+          +-------------------------+
| Deterministic         |          | LLM-Supervised          |
| Reactors              |          | Reactors                |
| repair/status/schema  |          | dreaming/curator/eval   |
| direct daemon work    |          | via HostAgent runner    |
+----------+-----------+          +-----------+-------------+
           |                                  |
           v                                  v
+------------------------------------------------------------+
| Governed Materialized State                                |
| .mnemon state, MEMORY.md, skill library, eval reports,     |
| proposals, audit, status, host manifests                   |
+------------------------------------------------------------+
```

## Concept Model

The conceptual model is unchanged:

```text
State
Intent
Projection
Reality
Evidence
Reconcile
Governance
```

The event-sourced runtime gives these concepts an implementation route:

| Concept | Architecture Shape |
| --- | --- |
| State | Materialized loop-owned data under `.mnemon`. |
| Intent | `GUIDE.md`, `loop.json`, bindings, policies, suites, rubrics. |
| Projection | Generated host-readable surfaces under `.codex`, `.claude`, etc. |
| Reality | Host prompts, tool results, file state, context pressure, eval transcripts. |
| Evidence | Append-only events, reports, status, eval artifacts. |
| Reconcile | Deterministic and LLM-supervised reactors. |
| Governance | Proposals, audits, diffs, review gates, rollback points. |

## Runtime Flow

```text
Reality happens in a host
        |
        v
Host surface records or exposes an observation
        |
        v
Lifecycle event is appended
        |
        v
Runtime evaluates intent, state, evidence, and thresholds
        |
        +------------------------------+
        |                              |
        v                              v
deterministic reactor            LLM-supervised reactor
direct daemon execution          HostAgent/app-server job
        |                              |
        v                              v
derived events                    structured job result
        |                              |
        +---------------+--------------+
                        |
                        v
validate / apply / propose / no-op
                        |
                        v
materialized state + reports + projection
```

This flow is the same for memory, skill, eval, and future loops.

## The Role Of Each Runtime Component

### Host Runtime

The host runtime is still the execution runtime. It owns:

```text
conversation loop
prompt assembly
model calls
tool routing
permission model
native hooks / skills / subagents when available
UI
```

Mnemon must not reimplement this.

### Host Projection

Projection turns canonical loop intent into host-readable surfaces:

```text
.codex/skills/*
.codex/mnemon-<loop>/env.sh
.claude/hooks/*
.claude/agents/*
host manifest
runtime env files
```

Projection is generated and repairable. It is not canonical state.

### Event Substrate

Events are the lifecycle fact source:

```json
{
  "id": "evt_...",
  "ts": "2026-05-23T00:00:00Z",
  "type": "memory.dreaming_requested",
  "loop": "memory",
  "host": "codex",
  "actor": "mnemon-daemon",
  "caused_by": "evt_...",
  "correlation_id": "job_...",
  "payload": {}
}
```

Reports and status files should reference events instead of replacing them.

The event substrate is a runtime contract, not just an observability aid:

```text
lifecycle events are append-only
materialized files, status, reports, and projections reference events
reactors emit started / completed / failed / skipped / proposed / applied
replay rebuilds lifecycle state from events
fork and diff become governance tools for alternate policies or proposals
```

### Lifecycle Runtime

The lifecycle runtime is Mnemon-owned infrastructure:

```text
event append
event materialization
status writing
projection repair
threshold checks
queues and locks
deterministic reactor execution
LLM job dispatch
schema validation
governance enforcement
```

The daemon is the long-running form of this runtime. Manual commands can execute
the same contracts before the daemon is available.

That long-running form is not a semantic agent and not a hidden replacement for
the host. Its role is deliberately narrower:

```text
mnemon-daemon = event-sourced lifecycle kernel
              + scheduler
              + materializer
              + validator
              + HostAgent job dispatcher
              + governance gate
```

The daemon directly runs deterministic lifecycle work. When work requires
semantic judgment, it dispatches a lifecycle job to a HostAgent runner and then
validates the structured result before recording, applying, or proposing
changes.

The daemon must not:

- converse with users
- take over the ReAct loop
- decide durable memory value by itself
- decide whether a skill should be retired by itself
- analyze eval failures semantically by itself
- bypass proposal or review gates
- embed a new LLM runtime inside Mnemon

### Reactor System

Reactors split into two classes.

Deterministic reactors:

```text
projection repair
status update
schema validation
event materialization
threshold check
report indexing
lock / queue maintenance
```

LLM-supervised reactors:

```text
memory dreaming
skill curator review
skill authoring
eval analyze / improve
policy proposal
ambiguous deletion review
```

The first class can run directly in the daemon. The second class should run
through a HostAgent runner.

The core loop is:

```text
lifecycle event accumulates
        |
        v
daemon detects due work
        |
        v
daemon appends job.requested
        |
        v
HostAgent runner executes portable job spec
        |
        v
LLM produces structured result
        |
        v
daemon validates result
        |
        +-----------------------------+
        |                             |
        v                             v
safe deterministic apply          proposal / review needed
        |                             |
        v                             v
events appended                   proposal.created
status/materialized state         audit/report updated
```

### HostAgent Runner

Codex app server is the reference HostAgent runner for LLM-supervised reactors.
It gives the lifecycle runtime a way to run semantic jobs without embedding a
new LLM runtime inside Mnemon.

```text
daemon schedules job
        |
        v
Codex app server starts HostAgent task
        |
        v
HostAgent reads job spec, GUIDE, state, recent events
        |
        v
LLM produces structured result
        |
        v
daemon validates and records accepted events
```

Codex app server is not merely an eval tool in this architecture. It is the
default pattern for LLM-supervised lifecycle job execution.

### Job Specs

Subagent specs become portable lifecycle job specs:

```text
harness/loops/memory/subagents/dreaming.md
harness/loops/skill/subagents/curator.md
harness/loops/eval/subagents/evaluator.md
```

They can run through:

```text
Claude Code native subagents
Codex app-server tasks
manual HostAgent prompts
future daemon runner adapters
```

This keeps the AI-native subagent idea without binding the architecture to one
host's feature set.

## Loop Plugin Contract

Every loop plugs into the same architecture by defining:

```text
Intent       why the loop exists and when it should no-op
Events       observed / requested / started / proposed / applied / skipped / failed / completed
State        canonical .mnemon-owned materialized data
Projection   host-readable hooks / skills / env / job specs
Reactors     deterministic or LLM-supervised reconcile units
Evidence     reports, status, eval artifacts, event lineage
Governance   proposal, audit, diff, rollback, review gates
Validation   scenarios proving behavior and no-op boundaries
```

New loop means new plugin surfaces. It should not mean new runtime architecture.

## Example: Memory Loop

```text
User or HostAgent creates durable memory signal
        |
        v
memory.hot_write_candidate
        |
        v
hot-write reactor
        |
        v
memory.hot_patch_applied
        |
        v
MEMORY.md materialized
```

Dreaming:

```text
MEMORY.md exceeds threshold
        |
        v
daemon schedules memory.dreaming_requested
        |
        v
Codex app server runs dreaming job spec
        |
        v
LLM proposes consolidation, skips, risks
        |
        v
daemon validates output and governance boundary
        |
        v
apply safe writes or create proposal
        |
        v
memory.cold_write_applied
memory.hot_patch_applied
memory.dreaming_completed
        |
        v
report + status updated
```

## Example: Skill Loop

```text
skill.usage_observed events accumulate
        |
        v
daemon detects threshold / schedule
        |
        v
skill.curator_requested
        |
        v
Codex app server runs curator job spec
        |
        v
LLM proposes promote / update / retire / no-op
        |
        v
daemon applies low-risk changes or writes proposal
        |
        v
skill.updated / skill.proposal_created / skill.skipped
```

## Governance

Low-risk deterministic actions can apply directly:

```text
projection repair
status refresh
report indexing
schema-normalized state refresh
```

Semantic actions are LLM-supervised:

```text
memory consolidation
skill curation
eval analysis
policy proposal
```

High-risk semantic actions should become proposals:

```text
delete durable memory
retire active skill
modify GUIDE.md or loop policy
cross-project memory promotion
apply weak eval evidence to core behavior
```

Default rule:

```text
deterministic low-risk -> apply
semantic judgment -> LLM-supervised
high-risk semantic -> proposal
ambiguous -> defer
```

## Implementation Phases

### Phase 1: Evented Manual Runtime

```text
events.jsonl
manual reactor commands
reports
status
projection repair command
```

This proves the contract without requiring a daemon.

### Phase 2: Daemon Scheduler

```text
watch event log
watch projection drift
check thresholds
enqueue jobs
run deterministic reactors
write status
```

This gives loops product-grade automatic convergence.

### Phase 3: HostAgent Job Runner

```text
daemon dispatches LLM-supervised jobs
Codex app server runs job specs
daemon validates outputs
daemon applies or proposes changes
```

This makes the daemon AI-native instead of a hidden semantic orchestrator.

### Phase 4: Cross-Loop Self-Evolution

```text
memory, skill, and eval reports share event lineage
eval findings create improvement proposals
skill curator uses usage evidence
memory dreaming uses recent lifecycle events
governance coordinates risky changes
```

This is the broader self-evolution layer.

## Design Principles

```text
Mnemon is not the host agent runtime.
The concept model remains stable.
Events are the lifecycle source of truth.
Files and host directories are materialized views.
Daemon is the lifecycle runtime's always-on form.
Codex app server is the reference LLM-supervised reactor runner.
Subagent specs are portable lifecycle job specs.
Governance controls high-risk self-evolution.
```
