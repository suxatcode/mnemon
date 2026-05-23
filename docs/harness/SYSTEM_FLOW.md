# System Flow

Chinese version: [SYSTEM_FLOW.md](../zh/harness/SYSTEM_FLOW.md)

Site version: [System Flow](../site/system-flow/index.html)

This document explains the end-to-end Mnemon lifecycle from the user's point of
view: starting with a bare host agent, installing Mnemon, opening a session,
sending queries, and letting daemon-driven feedback improve future sessions.

The key point is that Mnemon is not a linear pipeline. It is a feedback system
between four planes:

```text
Host Execution Plane       user dialogue, ReAct loop, hooks, skills
Lifecycle Control Plane    daemon, events, reactors, jobs, governance
Canonical State Plane      .mnemon events, state, reports, proposals, audit
Projection Plane           .codex/.claude hooks, skills, env, job specs
```

## Bare HostAgent

Before Mnemon is installed, the user only has a host such as Codex, Claude Code,
OpenClaw, or a future agent runtime.

The host owns:

```text
conversation loop
model calls
tool routing
permission model
prompt assembly
native hook / skill / subagent surfaces when available
UI and session lifecycle
```

There is no `.mnemon` state, no projected hooks, no projected skills, no
lifecycle events, and no daemon-driven maintenance. The host can complete tasks,
but durable memory, skill evolution, eval evidence, proposal review, and audit
are not governed capabilities yet.

## Bootstrap

Mnemon is installed or projected into the project or user scope:

```bash
mnemon harness install --host codex --loop memory --loop skill --loop eval
mnemon daemon start
```

The exact command may change, but the bootstrap responsibilities stay stable.

First, Mnemon creates canonical lifecycle state:

```text
.mnemon/
├── manifest.json
├── events.jsonl
├── harness/
│   ├── memory/status.json
│   ├── skill/status.json
│   └── eval/status.json
├── memory/
├── skills/
│   ├── active/
│   ├── stale/
│   └── archived/
├── reports/
├── proposals/
├── audit/
└── hosts/
    └── codex/manifest.json
```

Second, Mnemon binds loop templates to the host:

```text
harness/loops/memory
harness/loops/skill
harness/loops/eval
        |
        v
codex.memory / codex.skill / codex.eval bindings
```

Third, Mnemon renders host projections:

```text
.codex/
├── skills/
├── mnemon-memory/env.sh
├── mnemon-skill/env.sh
└── projected instructions / job specs / manifests
```

For Claude Code, the projection may instead target `.claude/hooks`,
`.claude/skills`, `.claude/agents`, and host settings. The rule is the same:
`.mnemon` is canonical state; host directories are generated projections.

## Runtime Planes

After bootstrap, four planes run together.

```text
                         +------------------------------+
                         |        User / Query           |
                         +---------------+--------------+
                                         |
                                         v
+----------------------------------------------------------------+
| Host Execution Plane                                            |
| Codex / Claude Code / OpenClaw                                  |
|                                                                |
|  ReAct loop                                                     |
|  prompt assembly                                                |
|  tool routing                                                   |
|  native hooks / skills                                          |
|                                                                |
|  prime / remind / nudge / compact                               |
+---------------+-------------------------------^----------------+
                |                               |
                | observations / protocol calls | projected surfaces
                v                               |
+----------------------------------------------------------------+
| Projection Plane                                                |
| .codex / .claude / host config                                  |
|                                                                |
|  projected hooks                                                |
|  projected protocol skills                                      |
|  projected subagent/job specs                                   |
|  projected env / manifests                                      |
+---------------^-------------------------------+----------------+
                |                               |
                | repair / regenerate           | host reads
                |                               v
+----------------------------------------------------------------+
| Canonical State Plane                                           |
| .mnemon                                                         |
|                                                                |
|  events.jsonl                                                   |
|  memory / MEMORY.md                                             |
|  skills active/stale/archived                                   |
|  eval reports                                                   |
|  proposals / reviews / audit                                    |
|  host manifests / status                                        |
+---------------^-------------------------------+----------------+
                |                               |
                | materialize / apply / audit   | watch / query
                |                               v
+----------------------------------------------------------------+
| Lifecycle Control Plane                                         |
| mnemon-daemon                                                   |
|                                                                |
|  event watcher                                                  |
|  scheduler                                                      |
|  deterministic reactors                                         |
|  HostAgent job dispatcher                                       |
|  validator                                                      |
|  governance gate                                                |
+---------------+-------------------------------^----------------+
                |                               |
                | LLM-supervised jobs           | structured results
                v                               |
        +-----------------------------------------------+
        | HostAgent Runner                              |
        | Codex app-server / Claude subagent / future   |
        | reads job spec + GUIDE + state + events       |
        +-----------------------------------------------+
```

Responsibilities by plane:

| Plane | Owns | Reads | Writes | Feeds back to |
| --- | --- | --- | --- | --- |
| Host Execution | ReAct loop, tool routing, UI, prompt assembly | Projection, recall, GUIDE | observations, protocol outputs | `.mnemon` events |
| Projection | `.codex`, `.claude`, hooks, skills, env | `.mnemon` materialized state | host-readable files | HostAgent |
| Canonical State | events, memory, skills, reports, proposals, audit | Host observations, daemon results | durable state | daemon and projection |
| Lifecycle Control | daemon, reactors, scheduler, validator | `.mnemon` events and state | events, status, proposals, projection repairs | `.mnemon` and HostAgent runner |
| HostAgent Runner | semantic job execution | job spec, GUIDE, state, events | structured result | daemon |

## User Session

When the user starts the host agent, the host's session-start boundary triggers
Prime when the host supports it.

```text
HostAgent session starts
        |
        v
prime hook reads projected env and surfaces
        |
        v
HostAgent sees GUIDE, hot memory, active skills, and protocols
```

Prime should stay light. It exposes the lifecycle policy and current projected
surfaces. It should not run heavy memory consolidation, skill curation, or eval
analysis.

## User Query

When the user sends a query, the host prompt boundary may trigger Remind:

```text
user query
        |
        v
remind hook
        |
        v
HostAgent decides whether lifecycle context is needed
```

For a query that needs prior project context, the HostAgent may load a protocol
skill such as `memory_get.md`:

```text
HostAgent calls memory_get
        |
        v
bounded recall from Mnemon / .mnemon state
        |
        v
recall context enters current reasoning
```

For a query where local context is sufficient, Remind should no-op. Mnemon does
not inject every memory into every prompt.

The same query is not a single line of execution. Several planes may be active
at once:

```text
Host Plane:
  - prompt boundary triggers Remind
  - HostAgent decides whether to call memory_get
  - HostAgent performs normal ReAct work

Projection Plane:
  - HostAgent reads projected skills, hooks, env, and job specs
  - visible capability is determined by the last projection repair

Canonical State Plane:
  - memory_get queries .mnemon
  - memory_set / skill_observe write events or evidence
  - reports, proposals, and status can be read

Control Plane:
  - daemon may be processing previous events in the background
  - daemon may repair projection drift
  - daemon may schedule dreaming, curator, or eval jobs
```

The user experiences one conversation. Internally, host execution and Mnemon
lifecycle control are coupled feedback planes.

## Online Work

The HostAgent then runs its normal execution loop:

```text
reason
read files
call tools
edit files
run tests
inspect results
respond
```

Mnemon does not replace planning, tool routing, permissions, or the UI. It
provides projected protocols the HostAgent may use when lifecycle signals are
relevant:

```text
memory_set       -> durable memory candidate
skill_observe    -> skill usage or missing-skill evidence
eval_plan/run    -> eval scenario planning or execution
```

At turn end, Nudge asks whether the work created durable signals:

```text
turn end
        |
        v
nudge hook
        |
        v
HostAgent checks memory, skill, eval, policy, or proposal evidence
        |
        v
append event / write evidence / no-op
```

Compact performs the same role at a context-save boundary, with higher emphasis
on preserving continuity before context is lost.

## Daemon Feedback

The daemon watches `.mnemon` and the event log. It turns scattered lifecycle
signals into governed state.

```text
events accumulate
        |
        v
daemon detects threshold, drift, or due work
        |
        +-----------------------------+
        |                             |
        v                             v
deterministic reactor            LLM-supervised job
status, projection, schema       memory dreaming, skill curator, eval analysis
        |                             |
        v                             v
events appended                  structured result
        |                             |
        +-------------+---------------+
                      |
                      v
validate / apply / propose / audit
                      |
                      v
.mnemon state and host projections update
```

The daemon directly handles deterministic work such as projection repair, status
refresh, schema validation, report indexing, threshold checks, and queue or lock
maintenance.

Semantic work goes through a HostAgent runner such as Codex app-server or a
native Claude Code subagent:

```text
daemon appends job.requested
        |
        v
HostAgent runner executes portable job spec
        |
        v
LLM reads GUIDE, state, recent events, reports, and artifacts
        |
        v
LLM returns structured result
        |
        v
daemon validates
        |
        v
apply safe result / create proposal / record failure
```

The daemon is the governance gate. It is not the semantic agent.

## Feedback Loops

The system has three primary feedback loops.

### Online Context Feedback

```text
.mnemon state
   -> projection / recall
   -> HostAgent context
   -> task outcome / evidence
   -> .mnemon events
```

This loop lets the current conversation benefit from previous lifecycle state
and write new durable signals back into the system.

### Background Lifecycle Feedback

```text
events and state
   -> daemon threshold / drift / due-work detection
   -> deterministic reactor or HostAgent job
   -> validated result
   -> status, reports, proposals, audit, state
```

This loop turns lightweight online observations into stable lifecycle state.

### Projection Feedback

```text
.mnemon state changes
   -> projection repair
   -> .codex / .claude surfaces update
   -> next HostAgent lifecycle boundary sees new capability
   -> new usage creates new evidence
```

This loop makes governed lifecycle changes visible to the host agent again.

The shortest accurate statement is:

```text
HostAgent turns user work into lifecycle signals.
Daemon turns lifecycle signals into governed state.
.mnemon preserves canonical state and evidence.
Projection turns governed state back into HostAgent-visible capability.
HostAgent uses that capability in future work.
```

This is why the final system should not be described as:

```text
user -> hook -> daemon -> .mnemon
```

It is instead:

```text
daemon -> .mnemon -> projection -> HostAgent -> events -> daemon
```

## Example: Memory Dreaming

```text
MEMORY.md grows too large
        |
        v
daemon detects threshold
        |
        v
memory.dreaming_requested
        |
        v
Codex app-server runs dreaming job spec
        |
        v
LLM proposes consolidation, skips, risks
        |
        v
daemon validates result
        |
        +-----------------------------+
        |                             |
        v                             v
safe writes                      risky changes
        |                             |
        v                             v
memory.cold_write_applied        proposal.created
memory.hot_memory_compacted      audit/report updated
        |
        v
next Prime sees smaller, better working memory
```

## Example: Skill Evolution

```text
HostAgent repeatedly performs a workflow
        |
        v
nudge / skill_observe records evidence
        |
        v
skill.usage_observed events accumulate
        |
        v
daemon schedules curator job
        |
        v
HostAgent runner reviews evidence and skill library
        |
        v
structured proposal: create / patch / stale / archive
        |
        v
daemon validates and writes proposal
        |
        v
approved proposal updates .mnemon skill state
        |
        v
projection repairs host skill surface
        |
        v
future queries can discover and use the improved skill
```

## User Experience

The desired user experience is simple:

```text
1. Install Mnemon into a project or user scope.
2. Start mnemon-daemon.
3. Open the preferred HostAgent.
4. Talk normally.
```

Behind that simple path, Mnemon is continuously cycling:

```text
HostAgent turns work into lifecycle signals.
Daemon turns signals into governed state.
.mnemon preserves canonical facts and materialized state.
Projection turns governed state into HostAgent-visible capability.
Future HostAgent work uses that capability and creates new signals.
```

This is the complete AI-native lifecycle pattern: the host remains the execution
runtime, while Mnemon provides a durable, event-sourced, LLM-supervised
lifecycle layer around it.
