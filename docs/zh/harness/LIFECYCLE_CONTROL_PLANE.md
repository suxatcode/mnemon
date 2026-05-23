# 生命周期控制平面

English version: [LIFECYCLE_CONTROL_PLANE.md](../../harness/LIFECYCLE_CONTROL_PLANE.md)

本文定义 Mnemon Harness 背后的轻量控制模型。可视化版本见
[Lifecycle Control Plane](../../site/lifecycle-control-plane/index.html)。

Mnemon 不需要一个重型分布式控制系统。Mnemon 需要的是一套一致的模型，用来让
agent 生命周期能力变得持久、可观测、可迁移、可治理。

这个控制平面围绕宿主 agent 展开，而不是替代宿主。Mnemon 不编排任务执行；
Mnemon 编排 lifecycle capabilities，例如 memory consolidation、skill promotion、
eval evidence、policy proposal、projection repair 和 audit。

## 最小定义

Mnemon 保存 `State`，声明 `Intent`，观察 `Reality`，并通过 `Reconcile` 把
Reality 拉回 Intent。结果重新写回 State。

```text
State -> Intent -> Reality -> Reconcile -> State
```

这是稳定内核。具体文件、skills、hooks、host adapters、evals 和 proposals，都通过
profile 进入这个内核。

## 核心模型

| 概念 | 含义 |
| --- | --- |
| State | Mnemon 拥有的持久事实，例如 `.mnemon` 下的 memory、skills、reports、proposals、audit 和 status。 |
| Intent | Mnemon 希望系统呈现的生命周期形态。 |
| Reality | 宿主、项目、工具、eval 和运行时当前真实发生的状态。 |
| Reconcile | 比较 Intent 与 Reality，并把结果写回 State 的对齐机制。 |

Execution surfaces 不属于核心模型。它们属于执行层：它们说明 Mnemon 如何触达宿主现实。

在 event-sourced runtime 中，State 由 lifecycle events materialize 出来，宿主
surfaces 仍然只是 projections。`.mnemon` 拥有 canonical lifecycle state；
`.codex`、`.claude`、hooks、skills 和 subagents 都是生成或可修复的 view。

## Entity Profiles

实体不是模型本身。每个实体只是在模型中声明自己的 profile。

| Profile | 含义 | 示例 |
| --- | --- | --- |
| Template | 可复用定义，不一定被持续 reconcile。 | `Loop` |
| Controlled | 需要持续对齐 Intent 与 Reality。 | `LoopBinding`、`EvalRun`、未来 `Goal` |
| Surface | 表达或触达宿主能力。 | `HostCapability`、`Projection` |
| Evidence | 来自 Reality 的观测事实，不是声明对象。 | `Observation`、runtime status |
| Governance | review、risk 和 audit 边界。 | `Proposal`、`Review`、`Audit` |

只有 controlled entities 需要完整的 `spec/status/reconcile` 形态。其他 profile
以不同方式参与 reconcile。

## 当前实体

| Entity | Profile | 作用 |
| --- | --- | --- |
| `Loop` | Template | 可复用 lifecycle capability package，例如 memory、skill、eval。 |
| `Binding` | Controlled | 把某个 `Loop` 绑定到某个 host；适合作为第一个完整 controlled object 样本。 |
| `HostCapability` | Surface | 描述宿主可以暴露的静态或动态能力。 |
| `Projection` | Surface | 让 HostAgent 看见 Mnemon 的 Intent。 |
| `Observation` | Evidence | 让 Mnemon 看见 HostAgent 的 Reality。 |
| `Proposal` / `Review` / `Audit` | Governance | 当 Reconcile 无法安全自动完成时，保存 proposal、decision 和不可变记录。 |

## Execution Surfaces

Execution surfaces 说明 Mnemon 如何触达宿主，而不把这个机制混进核心模型。

### Projection

Projection 是静态方向：把 Intent 渲染成 host-readable view。

示例：

- `.codex/skills`
- `.claude/hooks`
- host config
- generated docs
- manifests

Projection 让 HostAgent 看见 Mnemon 的 Intent。

### Observation

Observation 是动态方向：把 Reality 转化为 status、evidence 或 proposal 的输入。

示例：

- Codex appserver
- session APIs
- eval endpoints
- tool status
- runtime errors

Observation 让 Mnemon 看见 HostAgent Reality。

## Memory-loop 给出的证据

Mnemon 的方法，是把通常被做成重外部系统的能力，通过 hooks、skills、daemon work、
canonical state 和 reconcile，重新引入宿主生命周期。

`memory` 已经用 memory 验证了这个模式：

```text
external memory service
  -> hook + skill + .mnemon state
  -> prime / remind / nudge / compact lifecycle
  -> lifecycle-native memory capability
```

lifecycle control plane 把同样模式推广到 self-improving loops：

```text
standalone self-improvement loop
  -> hook + skill + daemon + HostCapability
  -> projection / observation / reconcile
  -> governable project evolution
```

## 与 Autoresearch 的关系

Autoresearch 是有价值的参考，因为它展示了一个受约束的 self-improving loop：

```text
edit -> run -> evaluate -> keep/discard -> repeat
```

Mnemon 不复制实验平台。Mnemon 借鉴的是 self-improving loop 的纪律，并让这类 loop
变得生命周期原生、宿主可迁移、可治理。

同样的边界也适用于 event-sourced agent runtimes。那类系统可以把 log、graph 和
behaviors 做成 agent runtime 本体。Mnemon 借鉴 event-sourced discipline，但把它
应用在已有宿主 agent 外围的 lifecycle control plane。

在 Mnemon 中，决策空间不止 keep 或 discard：

- repair
- validate
- propose
- review
- audit
- no-op

## 声明式控制平面类比

最接近的基础设施类比是 Kubernetes，但 Mnemon 借鉴的是 control-plane pattern，
不是复制它的领域模型。Kubernetes 用户用 manifests 声明 desired infrastructure
state，controllers 观察 actual state，并通过 reconcile 把 reality 拉向 desired
state。新增资源用 CRD；新增行为需要 controller 或 driver。

Mnemon 把同样形态应用到 AI lifecycle capabilities：

| Kubernetes | Mnemon |
| --- | --- |
| YAML manifest | `loop.json` 加 Markdown templates |
| CRD | loop schema 和 entity profile |
| Controller | daemon reactor |
| Reconcile loop | lifecycle reconcile |
| Status subresource | `.mnemon/harness/*/status.json` |
| Events | lifecycle events |
| Admission / policy | governance 和 proposal gates |
| Runtime / kubelet | HostAgent、host adapter 和 HostAgent runner |

关键差异是，每个 Mnemon loop package 有两类读者。Framework 读取 `loop.json`、
schemas 和 event vocabulary。HostAgent 读取 `GUIDE.md`、hooks、protocol skills
和 subagent/job specs。所以 Markdown templates 是一等对象：它们是
LLM-supervised lifecycle work 的语义 surface。

扩展规则由此得到：

```text
Template and manifest for new lifecycle semantics.
Code only for new host integration, deterministic algorithms, or framework primitives.
```

## 演进层级

Mnemon 应该沿着轻量能力层级增长：

| Level | 形态 |
| --- | --- |
| Profiles | 每个实体先声明 profile，不急于成为完整 resource object。 |
| Projection | 把 Intent 投影给 HostAgent。 |
| Observation | 通过 appserver、eval、tool status 和 runtime evidence 观察 Reality。 |
| Governance | AI 可以产生 patch、report 和 proposal，由 review gate 控制风险。 |

目标不是复制一个大型控制系统，而是形成一个小而一致的 lifecycle model，从
memory 延展到自演进的 agentic projects。
