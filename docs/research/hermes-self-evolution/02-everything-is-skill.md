# Everything Is Skill

## 结论

Hermes 最值得 Mnemon 学习的一点是：它没有把所有长期经验都塞进 memory，而是强制把“怎么做某类事”沉淀成 skill。

这背后的设计原则可以概括为：

```text
事实、偏好、环境细节 -> memory
流程、工具经验、反复出现的任务模式 -> skill
一次性进度、临时 TODO、当前会话状态 -> session artifact
```

因此 “everything is skill” 不是说一切都进 `SKILL.md`，而是说自进化的主要表达单元应该是可调用、可审查、可合并、可归档的 skill。memory 不应该承载 workflow。

## 为什么 skill 是自进化的主单元

自进化要解决的问题不是“记住更多”，而是“未来做得更好”。这更像能力资产管理，而不是事实存储。

| 需求 | memory 是否适合 | skill 是否适合 |
|---|---|---|
| 用户偏好 | 适合 | 通常不适合 |
| 项目固定事实 | 适合 | 只在形成操作流程时适合 |
| 一段可复用调试流程 | 不适合 | 适合 |
| 某类任务的验证 checklist | 不适合 | 适合 |
| 工具错误的规避方法 | 简短事实可进 memory，完整方法应进 skill | 适合 |
| 模板、脚本、参考文件 | 不适合 | 适合 |
| 多步骤安装流程 | 不适合 | 适合 |
| 当前任务进度 | 不适合 | 不适合，应放 session summary |

Skill 的优势在于它天然有结构：

- `name` 和 `description` 可用于检索与选择。
- `SKILL.md` 可写详细步骤和判断条件。
- `references/` 可放长说明。
- `templates/` 可放可复用模板。
- `scripts/` 可放可执行辅助程序。
- `assets/` 可放非文本资源。

这比把流程压缩成一条 memory 更适合长期演化。

## Hermes 的 skill 机制

Hermes 的 `skill_manage` 工具把 skill 当成一等可变 artifact。它支持 create、edit、patch、delete、write_file、remove_file。agent 可以创建 `~/.hermes/skills/<skill>/SKILL.md`，也可以写入支持文件。

Hermes 的关键设计点：

| 机制 | 作用 |
|---|---|
| frontmatter | 让 skill 有 name、description 等可检索元数据 |
| 支持目录白名单 | `references/`、`templates/`、`scripts/`、`assets/` |
| size limit | 防止单个 skill 膨胀成不可读仓库 |
| patch 优先 | 对已有 skill 增量修正，而不是每次新建 |
| agent-created provenance | curator 只治理 agent 自己创建的 skill |
| usage sidecar | 记录 view/use/patch/state/pinned/archive 信息 |
| curator | 把过窄、重复、过期的 skill 合并或归档 |

这套设计让 skill 成为可治理对象。没有这些元数据和治理面，skill 也会膨胀成无边界的 Markdown 垃圾堆。

## Class-First 而不是 one-session-one-skill

Hermes curator 的 review prompt 非常强调 umbrella-building。它不是被动找重复文件，而是主动把一堆窄 skill 归并为类级别能力。

一个坏模式是：

```text
fix-nextjs-port-3000
fix-nextjs-port-3001
fix-vite-port-5173
recover-node-dev-server
debug-dev-server-already-running
```

更好的 skill 是：

```text
dev-server-troubleshooting
  - port occupied
  - stale process
  - env mismatch
  - framework-specific commands
  - verification checklist
```

这对 Mnemon 特别重要。自进化不能把每次任务都变成一个 skill。更合理的是：

1. 先 patch 已有 skill。
2. 已有 skill 不够时，把长内容放入 `references/`。
3. 只有出现真正新类别时，才创建新 skill。
4. 周期性把窄 skill 合并成 umbrella skill。

## Skill 与 memory 的边界

Hermes 的 prompt guidance 把 memory 定义为 declarative facts，而不是 instructions。原因是：指令式 memory 会在未来被重复解释成全局命令，覆盖当前用户意图。

更合适的边界是：

| 内容 | 放哪里 | 理由 |
|---|---|---|
| “用户偏好简洁回答” | memory | 稳定偏好 |
| “以后所有回答必须简洁” | 不建议 | 容易覆盖当前请求 |
| “这个项目用 pnpm test” | memory 或 project guideline | 稳定事实 |
| “运行测试前先启动 redis，再跑 pnpm test:integration” | skill | 多步骤流程 |
| “上次 migration 失败是因为缺 env X” | memory 或 issue note | 可复用事实 |
| “如何诊断 migration 失败” | skill | 方法论 |
| “本轮已经改了三个文件” | session summary | 临时状态 |

Mnemon 的 `GUIDELINE.md` 应把这个边界写得很清楚。否则 memory 会不断变成隐式规则，最后和当前任务冲突。

## 为什么 skill 比 adapter 更适合第一阶段

用户当前的直觉是对的：harness framework 本身，大多数能力可以通过 skill 方式表达，不需要复杂 adapter。

原因有四个：

1. **跨 agent 更容易安装。** 每个 agent 都懂 Markdown，但不一定能接同一套 runtime adapter。
2. **LLM 可以自我解释。** `INSTALL.md` 告诉 agent 在哪个阶段装什么 hook，agent 可以根据自己的平台完成安装。
3. **review 成本低。** skill diff 能被人读懂，adapter 行为通常要读代码和日志。
4. **演化路径自然。** 先让 skill 改进流程，再在必要时把稳定模式固化为代码或工具。

这和 Hermes 的路径一致：运行时经验先进入 skill library；curator 负责治理；更激进的 Self-Evolution pipeline 再通过 eval 和 PR 改进 skill/prompt/tool/code。

## Mnemon 的 skill 设计建议

Mnemon 可以采用下面的规则。

### Skill 分类

| 分类 | 示例 | 是否应自进化 |
|---|---|---|
| workflow skill | release、debug、review、research、install | 是 |
| memory skill | recall、reflect、curate、promote、demote | 是，但需谨慎 |
| platform skill | Claude Code hooks、Codex skills、Hermes hooks | 是，按平台拆支持文件 |
| policy skill | secret handling、safe git、review gate | 只允许用户确认后变更 |
| project skill | 本项目特定流程 | 是，但仅在项目范围 |

### Skill frontmatter

建议至少包含：

```yaml
---
name: memory-review
description: Review recent work and propose durable memory or skill updates.
scope: project
created_by: agent
risk: medium
---
```

`created_by` 和 `risk` 很重要。curator 可以只自动处理 `created_by: agent` 且 `risk` 不高的 skill。高风险 skill 只输出 proposal。

### Skill 文件结构

```text
skills/
  memory-review/
    SKILL.md
    references/
      rubric.md
      examples.md
    templates/
      report.md
    scripts/
      check-memory-budget.sh
```

`SKILL.md` 应保持短而可执行；长解释、例子、历史报告放支持文件。这样既保留 Markdown-first，也避免一个文件膨胀。

## 自进化 skill 的生命周期

建议 Mnemon 借鉴 Hermes 的状态机：

```text
candidate
  -> active
  -> stale
  -> archived
```

每个 skill 记录：

- 创建来源：user、agent、package、project、imported。
- 最近使用时间。
- 最近查看时间。
- 最近 patch 时间。
- 被哪些 skill 吸收。
- 是否 pinned。
- 风险等级。
- 关联 evidence。

自动化规则可以很保守：

- agent-created 且长期 unused，可以 stale。
- stale 很久后，只 archive，不 delete。
- pinned 永不 archive。
- bundled/package skill 不自动变更。
- 所有合并输出 report。

## 设计判断

Mnemon 的第一阶段应该把“自进化能力”主要定义为 skill library 的生成、修正、合并和安装，而不是定义为记忆数据库。

这意味着：

- `GUIDELINE.md` 写记忆原则。
- `INSTALL.md` 写 hook 安装和平台差异。
- `skills/` 写实际可复用能力。
- memory 只保存必要事实和偏好。
- curator 只管理 skill 和热记忆候选，不直接动原始证据。

Everything is skill 的最终价值是：让系统演化的对象保持人类可读、agent 可执行、工程可治理。

## 参考来源

- Hermes skills and curator 文档: <https://hermes-agent.nousresearch.com/docs/user-guide/features/curator>
- Hermes Self-Evolution: <https://github.com/NousResearch/hermes-agent-self-evolution>
- Claude Code memory 文档关于 `CLAUDE.md`、rules、skills 的分工: <https://code.claude.com/docs/en/memory>
- 本地源码: `/tmp/mnemon-agent-research-sources/hermes-agent/tools/skill_manager_tool.py`
- 本地源码: `/tmp/mnemon-agent-research-sources/hermes-agent/tools/skill_usage.py`
- 本地源码: `/tmp/mnemon-agent-research-sources/hermes-agent/agent/curator.py`
- 本地源码: `/tmp/mnemon-agent-research-sources/hermes-agent/agent/prompt_builder.py`
