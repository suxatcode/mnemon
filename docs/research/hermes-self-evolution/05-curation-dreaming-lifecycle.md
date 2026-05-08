# Curation、Dreaming 与长期生命周期

## 结论

长期记忆一定会增长，增长之后就必须有整理机制。常见系统有两种代表路线：

| 路线 | 代表 | 特点 |
|---|---|---|
| Curator | Hermes | 聚焦 skill library，空闲触发，合并、patch、归档、报告、备份 |
| Dreaming | OpenClaw | 聚焦长期记忆 consolidation，阶段化处理 daily notes、recall signals、promotion |

Mnemon 应该吸收两者，但第一阶段更接近 Hermes：先做 reviewable curator，再逐步引入 dreaming 式 promotion。

## Hermes Curator 的设计

Hermes curator 的目标不是整理所有记忆，而是治理 agent-created skills。它解决的问题是：self-improvement loop 会不断生成 skill，如果不维护，skill catalog 会被窄小、重复、过时的条目污染。

### 触发方式

Hermes curator 是 inactivity-triggered，不是普通 cron daemon。文档描述为：CLI session start 和 gateway cron-ticker thread 会检查是否满足两个条件：

- 距离上次运行超过 `interval_hours`，默认 7 天。
- agent idle 超过 `min_idle_hours`，默认 2 小时。

满足后启动后台 fork 的 `AIAgent`。该 fork 使用自己的 prompt cache，不触碰活跃会话。

### 默认生命周期

| 状态 | 进入条件 | 行为 |
|---|---|---|
| active | 正常 skill | 可被查看、使用、patch |
| stale | 长期未使用，默认 30 天 | 仍保留，但进入整理候选 |
| archived | 更长期未使用，默认 90 天 | 移入 `.archive/`，可恢复 |

curator 不自动删除。最坏动作是 archive。

### 运行阶段

Hermes curator 一次运行有两阶段：

1. Deterministic transitions：不用 LLM，根据时间和状态把 unused skills 转为 stale 或 archived。
2. LLM review：辅助模型读取 agent-created skills，决定 keep、patch、consolidate 或 archive。

关键不是“让 LLM 清理文件”，而是让 LLM 在强约束下做 umbrella-building：

- 不碰 bundled/hub skills。
- 不碰 pinned skills。
- 不把 use_count 作为拒绝合并的理由。
- 不因为触发场景不同就拒绝合并。
- 优先构造 class-level skill。
- 可把窄内容降级到 `references/`、`templates/`、`scripts/`。
- 输出结构化 report。

### 报告与备份

Hermes curator 写：

- `~/.hermes/logs/curator/<timestamp>/run.json`
- `~/.hermes/logs/curator/<timestamp>/REPORT.md`

真实运行前还会备份 skill 目录。dry-run 可以输出同类报告但不变更文件。

这是 Mnemon 应该复制的核心能力：维护动作必须可审查，可回滚。

## Hermes Self-Improvement Nudge

Hermes 的运行时 self-improvement loop 会在任务后判断是否需要保存或更新 memory/skill。它的重点包括：

- 复杂任务后建议沉淀 skill。
- 用户纠正是强信号。
- 已有 skill 不准确时优先 patch。
- workflow 和 procedure 应进 skill。
- fact 和 preference 应进 memory。
- 背景 review agent 的工具集被限制在 memory/skills 相关范围内。

这说明 Hermes 的 curator 不是孤立模块。curator 只治理已经生成的 skill；生成和修正 skill 的入口发生在日常任务回合里。

## OpenClaw Dreaming 的设计

OpenClaw dreaming 是更重的长期记忆巩固系统。它把 daily notes、recall traces、phase signals、promotion candidates、dream diary 和 long-term `MEMORY.md` 连接起来。

### 输出形态

OpenClaw dreaming 写两类内容：

| 输出 | 用途 |
|---|---|
| `memory/.dreams/` | machine state、recall store、phase signals、locks |
| `DREAMS.md` 和 phase reports | 人类可读 diary/report |
| `MEMORY.md` | deep phase promotion 的长期记忆目标 |

注意：dream diary 本身不作为 promotion source。只有 grounded memory snippets 能提升到 `MEMORY.md`。

### 阶段模型

| 阶段 | 目的 | 是否写长期记忆 |
|---|---|---|
| Light | 整理和 stage 最近短期材料 | 否 |
| REM | 反思主题和信号，写 diary/report | 否 |
| Deep | score、gate、promote durable candidates | 是，写 `MEMORY.md` |

OpenClaw 的 deep promotion 通常会考虑 relevance、frequency、query diversity、recency、consolidation、conceptual richness 等信号。它比 Hermes curator 更像传统记忆模型。

### Compaction 前 flush

OpenClaw 的另一个关键点是 compaction 前 silent memory flush。上下文接近窗口时，系统可先运行一个保存 durable notes 的维护 turn，再 compact。这样降低“旧上下文被压缩前没有落盘”的风险。

对 Mnemon 来说，pre-compact hook 的价值很高。它不是为了每轮都记忆，而是为了在上下文即将损失细节前捕获关键连续性。

## Claude Code 的参照

Claude Code 更偏轻量。它有：

- `CLAUDE.md` 和 auto memory。
- rules 和 skills 用于拆分长期指令和任务能力。
- `/compact` 和自动 compaction 处理上下文窗口。
- scheduled tasks 可让 prompt 按计划运行。
- hooks 可在 `SessionStart`、`UserPromptSubmit`、`PreToolUse`、`PostToolUse`、`PreCompact`、`PostCompact` 等阶段触发。

Claude Code 没有把 memory consolidation 做成 OpenClaw 式 dreaming runtime，但它提供了足够 hook，让用户或项目实现轻量 review/flush/remind。

这对 Mnemon 的启发是：不要把所有系统都假设有内置 dreaming。Mnemon 可以用 `INSTALL.md` 为不同 agent 安装近似能力。

## Mnemon 的生命周期建议

### 第一阶段：Reviewable Curator

先做轻量 curator，目标是治理 Markdown artifacts。

输入：

- recent session summaries。
- hot memory。
- active skills。
- user corrections。
- tool failures。
- current guideline。

输出：

- memory patch proposal。
- skill patch proposal。
- new skill proposal。
- archive/demote proposal。
- report。

默认只 dry-run。用户确认后写入。

### 第二阶段：Pre-Compact Flush

如果目标 agent 支持 compaction hook，安装 pre-compact hook：

```text
当前任务目标是什么？
哪些文件/命令/决策必须保留？
哪些用户要求不能丢？
是否有 durable fact 或 skill update 候选？
写入 warm/session capsule，不直接污染 hot memory。
```

这样能减少压缩导致的连续性损失。

### 第三阶段：Dreaming 式 Promotion

当 cold/warm 层积累足够多后，再引入 dreaming：

1. Light：把 recent sessions 和 evidence 拆成候选。
2. REM：按主题聚合，写人类可读报告。
3. Deep：对高频、高置信、近期、跨任务复用的候选做 promotion proposal。

promotion 仍应先 proposal，再写 hot memory 或 skill。

## 长期增长的处理策略

| 问题 | 策略 |
|---|---|
| hot memory 太长 | demote 到 warm topic capsule 或 skill support file |
| skill 太多 | curator 合并为 umbrella skill |
| skill 太长 | 拆出 `references/`、`templates/`、`scripts/` |
| old facts 过时 | 标记 superseded，等待 review 删除或 demote |
| raw history 太多 | cold archive + index，按需召回 |
| recall 噪音 | `NONE` gate 和最小相关度阈值 |
| 后台写冲突 | lock + report + atomic patch |
| 高风险变更 | 只输出 PR/proposal |

## 设计判断

Mnemon 不应直接照搬 OpenClaw 的全量 dreaming，也不应只做 Hermes 的 skill curator。更合适的是：

```text
短期：Hermes-style curator for skills/hot memory
中期：pre-compact flush + warm capsules
长期：OpenClaw-style dreaming promotion over cold memory
```

这样可以从轻量 Markdown-first 起步，又为高容量长期记忆留下工程路径。

## 参考来源

- Hermes curator: <https://hermes-agent.nousresearch.com/docs/user-guide/features/curator>
- Hermes Self-Evolution: <https://github.com/NousResearch/hermes-agent-self-evolution>
- OpenClaw Dreaming: <https://docs.openclaw.ai/concepts/dreaming>
- OpenClaw Compaction: <https://docs.openclaw.ai/concepts/compaction>
- Claude Code scheduled tasks: <https://code.claude.com/docs/en/scheduled-tasks>
- Claude Code hooks: <https://code.claude.com/docs/en/hooks>
- 本地源码: `/tmp/mnemon-agent-research-sources/hermes-agent/agent/curator.py`
- 本地源码: `/tmp/mnemon-agent-research-sources/hermes-agent/tools/skill_usage.py`
- 本地源码: `/tmp/mnemon-agent-research-sources/openclaw/extensions/memory-core/src/dreaming.ts`
