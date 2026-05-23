# Mnemon Harness

Mnemon Harness 是 Mnemon modular self-evolution harness 的正式中文文档入口。

Mnemon 建立在 memory-driven 原则之上：持久 agent 应该把经验转化为可治理的
长期状态，并用这些状态改进未来行为。

Mnemon 不替换宿主 agent runtime，而是通过 hooks、skills、subagents、文件系统资产和环境配置，把外置 evolution loop 挂载到已有 agent 上。

这里的核心判断是：当宿主已经拥有 ReAct loop 和可读扩展面时，大量行为层面的
agent 能力都可以外置实现。Mnemon 把这些能力包装成 harness loops，而不是
重新实现一个 runtime。

Mnemon 也不只是 skill 集合。它拥有自己的 harness runtime substrate：loop
layout、ops、environment、state、reports、proposals、locks、queues、
host surface projection，以及可选的 daemon scheduling。

## 核心定位

| 主题 | 设计 |
| --- | --- |
| Modular Agent Harness | [中文](modular-agent/DESIGN.md) / [EN](../../harness/modular-agent/DESIGN.md) |
| Loop Standard | [中文](LOOP_STANDARD.md) / [EN](../../harness/LOOP_STANDARD.md) |
| Host Projection | [中文](HOST_PROJECTION.md) / [EN](../../harness/HOST_PROJECTION.md) |
| Harness Roadmap | [中文](ROADMAP.md) / [EN](../../harness/ROADMAP.md) |
| YC Evolving 设计哲学 | [中文](YC_EVOLVING_DESIGN_PHILOSOPHY.md) / [EN](../../harness/YC_EVOLVING_DESIGN_PHILOSOPHY.md) |
| Lifecycle Control Plane | [中文](LIFECYCLE_CONTROL_PLANE.md) / [EN](../../harness/LIFECYCLE_CONTROL_PLANE.md) / [site](../../site/lifecycle-control-plane/index.html) |
| AI-Native Lifecycle Runtime | [中文](LIFECYCLE_RUNTIME.md) / [EN](../../harness/LIFECYCLE_RUNTIME.md) / [site](../../site/lifecycle-runtime/index.html) |
| System Flow | [中文](SYSTEM_FLOW.md) / [EN](../../harness/SYSTEM_FLOW.md) / [site](../../site/system-flow/index.html) |
| Memory Loop | [中文](memory/DESIGN.md) / [EN](../../harness/memory/DESIGN.md) / [site](../../site/memory/index.html) |
| Skill Loop | [中文](skill/DESIGN.md) / [EN](../../harness/skill/DESIGN.md) / [site](../../site/skill/index.html) |
| Eval Loop | [中文](eval/DESIGN.md) / [EN](../../harness/eval/DESIGN.md) |

## 可安装资产

| Harness Loop | 实现 |
| --- | --- |
| Memory Loop | [harness/loops/memory](../../../harness/loops/memory/README.md) |
| Skill Loop | [harness/loops/skill](../../../harness/loops/skill/README.md) |
| Eval Loop | [harness/loops/eval](../../../harness/loops/eval/README.md) |

## 仓库布局

| 目录 | 作用 |
| --- | --- |
| `harness/loops/` | Canonical、host-agnostic loop templates。 |
| `harness/hosts/` | Host projection adapters，例如 Claude Code，以及后续 Codex 支持。 |
| `harness/bindings/` | Loop x host binding definitions。 |
| `harness/control/` | Shared control-plane contracts。 |
| `harness/ops/` | 统一 install、status 和 uninstall 入口，用来组合 loops 与 hosts。 |

## 词汇

| 概念 | 含义 |
| --- | --- |
| loop template | 一个可挂载 harness loop 的标准包结构。 |
| GUIDE | Markdown policy，用来判断某个 loop 何时应该行动。 |
| ops | 安装、status、validate 和 uninstall 操作。 |
| hook | Prime、Remind、Nudge、Compact 等宿主生命周期时机。 |
| protocol | 定义可复用操作的 Markdown skill。 |
| subagent | 用于较重 review 或 consolidation 的后台维护 agent。 |
| projection | 把 canonical loop assets 渲染到 `.claude`、`.codex` 或其他 runtime surface 的宿主特定过程。 |
| host manifest | 机器可读记录，描述已投影 loops、paths、lifecycle mappings 和 host capabilities。 |
| daemon | 可选的 harness maintenance runner，用于调度 loop 后台工作。 |
| substrate | Mnemon 拥有的运行时基座，用于 loop state、ops、projection、scheduling 和跨 loop 协议。 |
| system flow | 从裸 HostAgent 到 bootstrap、hooks、daemon reconcile、`.mnemon` state 和 host projection 的端到端反馈路径。 |

## 边界

宿主 agent 保留 ReAct loop、prompt assembly、tool routing、native skill runtime、权限模型和 UI。Mnemon 提供可挂载的 harness loop，让宿主 agent 获得更持久、更可自进化的能力。

简言之：宿主 agent 是 execution runtime；Mnemon 是 harness runtime substrate。

Claude Code 是第一个 reference host，因为它提供 hooks、skills 和 subagents。这个架构的目标不局限于 Claude Code。

`mnemon-daemon` 后续可以作为 harness loop 的后台维护 runner。它属于
harness layer，不是宿主 agent runtime。
