# AI-Native Lifecycle Architecture

英文版本：[LIFECYCLE_RUNTIME.md](../../harness/LIFECYCLE_RUNTIME.md)

站点版本：[AI-Native Lifecycle Architecture](../../site/lifecycle-runtime/index.html)

端到端用户/session 运行流：[System Flow](SYSTEM_FLOW.md)。

本文把 memory loop、skill loop、eval loop、lifecycle control plane、
event-sourced runtime、daemon、Codex app server 和 subagent/job-spec 的讨论，
收束成一个整体架构方向。

Mnemon 是挂载在现有宿主 Agent 外围的事件溯源生命周期层，而不是替代宿主的
Agent Runtime。它为已有宿主增加持久 memory、skill evolution、eval、policy、
proposal 和 audit 生命周期能力，但不接管任务执行。

它不是 daemon-only 设计。daemon 是重要运行时组件，但完整架构更大：

```text
Concept model
  -> event-sourced lifecycle substrate
  -> host projection
  -> AI-native execution surfaces
  -> deterministic and LLM-supervised reactors
  -> governed materialized state
```

## 核心判断

Mnemon 应该继续作为外置 lifecycle architecture，挂在已有 agent runtime 之外。它不替换宿主的 ReAct loop、模型运行时、UI、权限系统或原生工具执行。

边界要保持清楚：

```text
Mnemon does not orchestrate task execution.
Mnemon orchestrates lifecycle capabilities.
Host surfaces are projections; .mnemon owns canonical lifecycle state.
```

核心架构动作是：

```text
用确定性机器处理 lifecycle structure。
用 HostAgent / LLM supervision 处理 semantic judgment。
用 append-only lifecycle events 让两者都可审计。
```

最终得到的是 AI-native lifecycle system：

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

## 分层架构

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

## 概念模型

概念模型不变：

```text
State
Intent
Projection
Reality
Evidence
Reconcile
Governance
```

event-sourced runtime 给这些概念提供工程落地路线：

| 概念 | 架构形态 |
| --- | --- |
| State | `.mnemon` 下由 loop 拥有的 materialized data。 |
| Intent | `GUIDE.md`、`loop.json`、bindings、policies、suites、rubrics。 |
| Projection | `.codex`、`.claude` 等 host-readable generated surfaces。 |
| Reality | Host prompts、tool results、file state、context pressure、eval transcripts。 |
| Evidence | Append-only events、reports、status、eval artifacts。 |
| Reconcile | Deterministic 和 LLM-supervised reactors。 |
| Governance | Proposals、audits、diffs、review gates、rollback points。 |

## 运行数据流

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

这个流程对 memory、skill、eval 和未来 loops 都相同。

## 各运行时组件的角色

### Host Runtime

Host runtime 仍然是 execution runtime。它拥有：

```text
conversation loop
prompt assembly
model calls
tool routing
permission model
native hooks / skills / subagents when available
UI
```

Mnemon 不应该重新实现这些。

### Host Projection

Projection 把 canonical loop intent 变成 host-readable surfaces：

```text
.codex/skills/*
.codex/mnemon-<loop>/env.sh
.claude/hooks/*
.claude/agents/*
host manifest
runtime env files
```

Projection 是生成出来的，可修复，不是 canonical state。

### Event Substrate

Events 是 lifecycle fact source：

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

Reports 和 status files 应该引用 events，而不是替代 event log。

Event substrate 是 runtime contract，不只是 observability：

```text
lifecycle events are append-only
materialized files, status, reports, and projections reference events
reactors emit started / completed / failed / skipped / proposed / applied
replay rebuilds lifecycle state from events
fork and diff become governance tools for alternate policies or proposals
```

### Lifecycle Runtime

Lifecycle runtime 是 Mnemon 拥有的基础设施：

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

daemon 是这个 runtime 的常驻形态。在 daemon 可用之前，手动命令也可以执行同一组 contracts。

这个常驻形态不是语义 agent，也不是隐藏的宿主替代物。它的角色要更窄：

```text
mnemon-daemon = event-sourced lifecycle kernel
              + scheduler
              + materializer
              + validator
              + HostAgent job dispatcher
              + governance gate
```

daemon 直接运行确定性的 lifecycle 工作。当工作需要语义判断时，它把 lifecycle
job 派发给 HostAgent runner，然后校验结构化结果，再决定记录、应用或生成
proposal。

daemon 不应：

- 和用户对话
- 接管 ReAct loop
- 自己判断 memory 是否有长期价值
- 自己判断 skill 是否应该 retired
- 自己语义分析 eval failure
- 绕过 proposal 或 review gate
- 在 Mnemon 内嵌一个新的 LLM runtime

### Reactor System

Reactors 分为两类。

Deterministic reactors：

```text
projection repair
status update
schema validation
event materialization
threshold check
report indexing
lock / queue maintenance
```

LLM-supervised reactors：

```text
memory dreaming
skill curator review
skill authoring
eval analyze / improve
policy proposal
ambiguous deletion review
```

第一类可以由 daemon 直接运行。第二类应该通过 HostAgent runner 运行。

核心闭环是：

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

Codex app server 是 LLM-supervised reactors 的 reference HostAgent runner。它让 lifecycle runtime 能够运行语义 job，而不需要 Mnemon 内嵌新的 LLM runtime。

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

在这个架构里，Codex app server 不只是 eval tool。它是 LLM-supervised lifecycle job execution 的默认模式。

### Job Specs

Subagent specs 变成 portable lifecycle job specs：

```text
harness/loops/memory/subagents/dreaming.md
harness/loops/skill/subagents/curator.md
harness/loops/eval/subagents/evaluator.md
```

它们可以通过以下方式运行：

```text
Claude Code native subagents
Codex app-server tasks
manual HostAgent prompts
future daemon runner adapters
```

这保留了 AI-native subagent 思路，但不把架构绑定到某个 host 的特性上。

## Loop Plugin Contract

每个 loop 通过定义以下内容接入同一架构：

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

新增 loop 意味着新增 plugin surfaces，而不是新增 runtime architecture。

## 示例：Memory Loop

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

Dreaming：

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

## 示例：Skill Loop

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

低风险确定性动作可以直接应用：

```text
projection repair
status refresh
report indexing
schema-normalized state refresh
```

语义动作需要 LLM-supervised：

```text
memory consolidation
skill curation
eval analysis
policy proposal
```

高风险语义动作应该变成 proposals：

```text
delete durable memory
retire active skill
modify GUIDE.md or loop policy
cross-project memory promotion
apply weak eval evidence to core behavior
```

默认规则：

```text
deterministic low-risk -> apply
semantic judgment -> LLM-supervised
high-risk semantic -> proposal
ambiguous -> defer
```

## 实现阶段

### Phase 1: Evented Manual Runtime

```text
events.jsonl
manual reactor commands
reports
status
projection repair command
```

先证明 contract，不要求 daemon。

### Phase 2: Daemon Scheduler

```text
watch event log
watch projection drift
check thresholds
enqueue jobs
run deterministic reactors
write status
```

让 loops 获得产品级自动收敛能力。

### Phase 3: HostAgent Job Runner

```text
daemon dispatches LLM-supervised jobs
Codex app server runs job specs
daemon validates outputs
daemon applies or proposes changes
```

让 daemon 成为 AI-native，而不是隐藏的 semantic orchestrator。

### Phase 4: Cross-Loop Self-Evolution

```text
memory, skill, and eval reports share event lineage
eval findings create improvement proposals
skill curator uses usage evidence
memory dreaming uses recent lifecycle events
governance coordinates risky changes
```

这是更大的 self-evolution layer。

## 设计原则

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
