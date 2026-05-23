# 系统运行流

英文版本：[SYSTEM_FLOW.md](../../harness/SYSTEM_FLOW.md)

站点版本：[System Flow](../../site/system-flow/index.html)

本文从用户视角解释 Mnemon lifecycle 的端到端路径：从一个裸 HostAgent 开始，
安装 Mnemon，启动 session，发起 query，再由 daemon 驱动反馈，让未来 session
持续改进。

关键点是：Mnemon 不是线性 pipeline。它是四个平面之间的反馈系统：

```text
Host Execution Plane       用户对话、ReAct loop、hooks、skills
Lifecycle Control Plane    daemon、events、reactors、jobs、governance
Canonical State Plane      .mnemon events、state、reports、proposals、audit
Projection Plane           .codex/.claude hooks、skills、env、job specs
```

## 裸 HostAgent

安装 Mnemon 之前，用户只有 Codex、Claude Code、OpenClaw 或未来某个 agent
runtime。

宿主拥有：

```text
conversation loop
model calls
tool routing
permission model
prompt assembly
native hook / skill / subagent surfaces when available
UI and session lifecycle
```

此时没有 `.mnemon` state，没有 projected hooks，没有 projected skills，没有
lifecycle events，也没有 daemon-driven maintenance。宿主可以完成任务，但持久
memory、skill evolution、eval evidence、proposal review 和 audit 还不是可治理能力。

## Bootstrap

用户把 Mnemon 安装或投影到 project/user scope：

```bash
mnemon harness install --host codex --loop memory --loop skill --loop eval
mnemon daemon start
```

具体命令可以演进，但 bootstrap 的职责保持稳定。

第一，Mnemon 创建 canonical lifecycle state：

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

第二，Mnemon 把 loop templates 绑定到宿主：

```text
harness/loops/memory
harness/loops/skill
harness/loops/eval
        |
        v
codex.memory / codex.skill / codex.eval bindings
```

第三，Mnemon 渲染 host projections：

```text
.codex/
├── skills/
├── mnemon-memory/env.sh
├── mnemon-skill/env.sh
└── projected instructions / job specs / manifests
```

如果是 Claude Code，projection 可能写入 `.claude/hooks`、`.claude/skills`、
`.claude/agents` 和 host settings。规则一致：`.mnemon` 是 canonical state；
host directories 是 generated projections。

## 运行平面

Bootstrap 之后，四个平面同时运行。

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

各平面职责：

| 平面 | 拥有什么 | 读什么 | 写什么 | 反馈到哪里 |
| --- | --- | --- | --- | --- |
| Host Execution | ReAct loop、tool routing、UI、prompt assembly | Projection、recall、GUIDE | observations、protocol outputs | `.mnemon` events |
| Projection | `.codex`、`.claude`、hooks、skills、env | `.mnemon` materialized state | host-readable files | HostAgent |
| Canonical State | events、memory、skills、reports、proposals、audit | Host observations、daemon results | durable state | daemon 和 projection |
| Lifecycle Control | daemon、reactors、scheduler、validator | `.mnemon` events 和 state | events、status、proposals、projection repairs | `.mnemon` 和 HostAgent runner |
| HostAgent Runner | semantic job execution | job spec、GUIDE、state、events | structured result | daemon |

## 用户启动 Session

用户启动宿主 agent 时，宿主的 session-start boundary 在支持时触发 Prime。

```text
HostAgent session starts
        |
        v
prime hook reads projected env and surfaces
        |
        v
HostAgent sees GUIDE, hot memory, active skills, and protocols
```

Prime 应保持轻量。它暴露 lifecycle policy 和当前 projected surfaces，不运行重型
memory consolidation、skill curation 或 eval analysis。

## 用户发起 Query

用户发送 query 后，宿主 prompt boundary 可以触发 Remind：

```text
user query
        |
        v
remind hook
        |
        v
HostAgent decides whether lifecycle context is needed
```

如果 query 需要历史项目上下文，HostAgent 可以加载 `memory_get.md`：

```text
HostAgent calls memory_get
        |
        v
bounded recall from Mnemon / .mnemon state
        |
        v
recall context enters current reasoning
```

如果当前本地上下文足够，Remind 应 no-op。Mnemon 不把所有 memory 主动塞进每个
prompt。

同一个 query 不是单线执行。多个平面可能同时活跃：

```text
Host Plane:
  - prompt boundary 触发 Remind
  - HostAgent 判断是否调用 memory_get
  - HostAgent 正常 ReAct 执行

Projection Plane:
  - HostAgent 读取 projected skills、hooks、env 和 job specs
  - 当前可见能力由上一次 projection repair 决定

Canonical State Plane:
  - memory_get 查询 .mnemon
  - memory_set / skill_observe 写 events 或 evidence
  - reports、proposals 和 status 可被读取

Control Plane:
  - daemon 可能同时在后台处理上一轮事件
  - daemon 可能修复 projection drift
  - daemon 可能调度 dreaming、curator 或 eval jobs
```

用户看到的是一次对话。系统内部是 Host execution 和 Mnemon lifecycle control
之间的多平面反馈耦合。

## 在线工作

随后 HostAgent 正常运行自己的 execution loop：

```text
reason
read files
call tools
edit files
run tests
inspect results
respond
```

Mnemon 不替代 planning、tool routing、permissions 或 UI。它提供 projected
protocols，让 HostAgent 在相关时写入 lifecycle signals：

```text
memory_set       -> durable memory candidate
skill_observe    -> skill usage or missing-skill evidence
eval_plan/run    -> eval scenario planning or execution
```

回合结束时，Nudge 判断是否产生 durable signal：

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

Compact 在 context-save boundary 执行类似职责，但更强调在上下文丢失前保存连续性。

## Daemon Feedback

daemon watch `.mnemon` 和 event log。它把零散 lifecycle signals 转化为可治理状态。

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

daemon 直接处理 deterministic work，例如 projection repair、status refresh、
schema validation、report indexing、threshold checks、queue 或 lock maintenance。

语义工作通过 Codex app-server 或 Claude Code native subagent 这类 HostAgent
runner 执行：

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

daemon 是 governance gate，不是 semantic agent。

## 反馈闭环

系统有三个主要反馈闭环。

### Online Context Feedback

```text
.mnemon state
   -> projection / recall
   -> HostAgent context
   -> task outcome / evidence
   -> .mnemon events
```

这个闭环让当前对话受益于已有 lifecycle state，并把新的 durable signals 写回系统。

### Background Lifecycle Feedback

```text
events and state
   -> daemon threshold / drift / due-work detection
   -> deterministic reactor or HostAgent job
   -> validated result
   -> status, reports, proposals, audit, state
```

这个闭环把在线轻量 observations 转化为稳定 lifecycle state。

### Projection Feedback

```text
.mnemon state changes
   -> projection repair
   -> .codex / .claude surfaces update
   -> next HostAgent lifecycle boundary sees new capability
   -> new usage creates new evidence
```

这个闭环让治理后的 lifecycle changes 重新变成宿主可见能力。

最短的准确表述是：

```text
HostAgent turns user work into lifecycle signals.
Daemon turns lifecycle signals into governed state.
.mnemon preserves canonical state and evidence.
Projection turns governed state back into HostAgent-visible capability.
HostAgent uses that capability in future work.
```

最终系统不应被描述为：

```text
user -> hook -> daemon -> .mnemon
```

它更准确是：

```text
daemon -> .mnemon -> projection -> HostAgent -> events -> daemon
```

## 示例：Memory Dreaming

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

## 示例：Skill Evolution

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

## 用户体验

目标用户体验很简单：

```text
1. Install Mnemon into a project or user scope.
2. Start mnemon-daemon.
3. Open the preferred HostAgent.
4. Talk normally.
```

背后 Mnemon 持续循环：

```text
HostAgent turns work into lifecycle signals.
Daemon turns signals into governed state.
.mnemon preserves canonical facts and materialized state.
Projection turns governed state into HostAgent-visible capability.
Future HostAgent work uses that capability and creates new signals.
```

这就是完整的 AI-native lifecycle pattern：宿主仍然是 execution runtime，Mnemon
在它外围提供 durable、event-sourced、LLM-supervised lifecycle layer。
