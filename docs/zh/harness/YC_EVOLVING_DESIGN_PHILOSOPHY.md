# YC Evolving 设计哲学

English version: [YC_EVOLVING_DESIGN_PHILOSOPHY.md](../../harness/YC_EVOLVING_DESIGN_PHILOSOPHY.md)

这份文档基于 YC Root Access 的演讲 "How to Build a Self-Improving Company
with AI" 以及中文文章《YC合伙人：如何打造一家自我进化的AI原生公司》整理。它不是
文章归档，而是把其中对 Mnemon harness 和 lifecycle control plane 有价值的判断，
沉淀成后续设计参考。

## 核心判断

AI 原生组织不应该只被理解为“传统层级组织 + AI 工具”。它更像一组递归、自我改进
的 loop：

```text
信号 -> 策略 -> 工具 -> 质量关卡 -> 学习
  ^                                  |
  |----------------------------------|
```

对 Mnemon 来说，这强化了 harness 的核心判断：

Mnemon 不应该变成 agent runtime、workflow engine，或者单纯的 memory store。
Mnemon 应该提供一层生命周期控制能力，让宿主 agent 能够把持久上下文、skill、
policy、反馈和执行结果，转化为可治理的自我改进 loop。

## 从 Copilot 到自我改进系统

文章中最有价值的区分是：

| 模式 | 形态 | 局限 |
| --- | --- | --- |
| Copilot | AI 帮助人更快完成已有任务。 | 组织仍然依赖人类协调和手工改进。 |
| 自我改进 loop | AI 观察结果、识别失败、提出或执行修正，并把结果反馈回系统。 | 需要可读取上下文、确定性工具、质量关卡和持久反馈。 |

Mnemon 应该服务第二种模式。宿主 agent 可以负责实际执行，但 Mnemon 应该帮助外层
系统记住发生了什么、检测漂移、改进 skill、更新生命周期状态，并保存可 review 的
证据。

## 公司大脑与 canonical context

文章里的“公司大脑”，可以直接映射到 Mnemon 的 canonical state。真正有价值的资产
不是临时 dashboard、生成脚本、聊天线程或宿主特定插件文件，而是可读取、持久、
结构化的上下文：

- goals、decisions、policies 和 constraints
- memory 和压缩后的运营知识
- skills 及其 usage evidence
- reports、proposals、audit records 和 review status
- host bindings 和 capability manifests
- validation outcomes 和 observed drift

在 Mnemon 中，这些状态应该位于 `.mnemon` 或其他 canonical state root 下。
`.codex`、`.claude` 或未来插件目录，应该被视为可再生成的 projection。

```text
canonical context
  durable memory, skills, policy, reports, proposals, audit
        |
        v
lifecycle control
  reconcile, validate, project, learn
        |
        v
host surfaces
  skills, hooks, app servers, tools, generated files
```

## 临时软件，持久上下文

文章提出，生成出来的内部软件可以是临时的，而业务上下文和 skills 才是长期资产。
这与 Mnemon 的 host projection 模型高度一致。

Mnemon 应该把宿主原生资产视为有用但可替换：

- generated dashboards
- host skill files
- hook glue
- app-server configuration
- eval runners
- temporary workflow code

真正持久的，是解释这些资产为何存在、何时过期、如何验证、是否应该重新生成的
lifecycle state。

## Loop 结构

文章中的 loop 可以转化为 Mnemon 的生命周期模型：

```text
State
  durable context, skill lifecycle state, reports, proposals, status
        |
        v
Intent
  goals, policies, desired visibility, review boundaries
        |
        v
Projection
  host-readable skills, hooks, app servers, tools, eval surfaces
        |
        v
Reality
  user intent, repo diffs, host behavior, eval results, customer feedback
        |
        v
Reconcile
  compare Intent with Reality, then record action, no-op, or proposal
        |
        v
Updated State
```

Mnemon 应该保持清晰的最小主干：

```text
State -> Intent -> Projection -> Reality -> Reconcile -> State
```

## Host Capability Surfaces

文章强调确定性工具、生成软件和质量关卡。在 Mnemon 中，这些不应该变成 Mnemon
自己的 execution runtime，而应该被表达为 host capability surfaces。

示例包括：

- Codex skills 和 project files
- Claude Code skills、hooks 和 subagents
- Codex app-server endpoints
- eval runners 和 test commands
- repository files 和 generated dashboards
- 通过宿主工具暴露的 databases、search indexes 和 external APIs

宿主拥有执行。Mnemon 拥有围绕执行展开的生命周期协调：什么应该存在、如何投影、
如何验证、哪里失败了、下一步应该改变什么。

## 质量关卡与人类边界

文章并不意味着所有事情都应该完全自治。它明确把人类放在系统边缘，用来处理高风险、
新颖、伦理复杂或情绪浓度很高的现实场景。

Mnemon 应该把这个边界显式化：

- 低风险 observation 和 reporting 可以自动化
- projection validation 可以自动化
- skill 和 memory proposal 可以自动生成
- 破坏性变更需要显式 review
- 高风险 policy、security、data 或 production 变更需要 human gate
- audit records 应该保存发生了什么以及为什么发生

这样，自我改进是可 review 的，而不是隐形发生的。

## 对 Mnemon 的设计含义

这个设计哲学支持以下 Mnemon 设计选择：

1. 把 `.mnemon` 作为 canonical lifecycle state。
2. 把 `.codex`、`.claude` 和类似目录视为 projection。
3. 每条改进路径都建模为包含 signals、policy、tools、gates 和 feedback 的 loop。
4. 宿主执行保持在 Mnemon core 之外。
5. 显式建模 Reconcile：比较 desired lifecycle state、actual host surfaces 和
   observed outcomes。
6. 把 status、failures、stale projections 和 missing capabilities 作为一等状态。
7. 优先生成或投影宿主资产，而不是维护重复真相。
8. 对高风险变更保留 human review boundary。

## 战略定位

这篇文章描述的是 Mnemon 应该服务的组织形态：通过持久上下文和递归 loop 运转的
self-improving agentic systems。

Mnemon 的差异化不只是“agent memory”。更强的定位是：

```text
Mnemon turns durable context into lifecycle-controlled agent improvement loops.
```

Memory 是连续性支点。Loop 是差异化。Control plane 是产品形态。
