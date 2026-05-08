# 热记忆、冷记忆与 Filesystem

## 结论

Mnemon 更合适的长期方案是把记忆分成模型层和工程层：

```text
模型层：热记忆
  - 小
  - 明确
  - 当前任务相关
  - 直接进入 prompt 或 hook 注入

工程层：冷记忆
  - 大
  - 可索引
  - 可追溯
  - 可长期积累
  - 通过 recall/promote/demote 与热层交换
```

这能同时吸收 Hermes 的 Markdown-first 实践和 OpenClaw 的高容量 memory runtime 思路。核心不是在二者之间二选一，而是让热层服务 LLM，让冷层服务长期容量。

## 为什么需要冷热分层

单个 Markdown 文件短期足够好，但长期会出现三个问题：

1. **容量问题。** 文件太长后无法全部进入上下文，或者进入后挤压任务上下文。
2. **质量问题。** 新旧事实、过时流程、一次性进度、重复经验混在一起。
3. **控制问题。** 模型不知道哪些记忆是用户确认的，哪些是推断的，哪些已被新事实覆盖。

Hermes 选择硬限制和 curator，Claude Code 对 auto memory 启动加载做限制，OpenClaw 选择 daily notes、search、compaction flush、dreaming promotion。共同结论是：热层必须被控制，长期积累必须进入另一个层。

## 三层模型

建议 Mnemon 使用 hot / warm / cold 三层，而不是简单二分。

| 层 | 直接给模型吗 | 典型内容 | 存储形态 |
|---|---|---|---|
| Hot | 是 | 当前用户偏好、当前项目 capsule、活跃 guideline、少量相关 facts、当前 task reminders | 小 Markdown 文件或 hook 注入片段 |
| Warm | 按需 | topic capsules、session summaries、active skills、recent daily notes、curated examples | filesystem Markdown、skill support files |
| Cold | 否，需召回 | raw transcripts、tool evidence、历史报告、embedding index、usage events、archived memories | filesystem + sqlite/vector/full-text index |

热层是模型的工作记忆扩展。冷层是系统的长期记忆。温层是两者之间的人类可审查整理层。

## Filesystem 的角色

Filesystem 不只是存文件，它是自进化的控制面。

建议的概念结构：

```text
.mnemon/
  hot/
    profile.md
    project.md
    active-guideline.md
    reminders.md
  warm/
    topics/
    sessions/
    capsules/
  cold/
    evidence/
    transcripts/
    imports/
    archive/
  index/
    memory.sqlite
    embeddings/
  reports/
    review/
    curator/
    dreaming/
```

可以把 filesystem 看成“可审查真相层”，把 sqlite/vector 看成“召回加速层”。重要事实最终应该能落到可读 artifact 上，而不是只存在 embedding 里。

## 热记忆的规则

热记忆必须遵循严格预算。建议规则：

| 规则 | 说明 |
|---|---|
| 小于固定预算 | 例如每个 hot capsule 目标 100 到 300 行以内，或按 token 预算控制 |
| 高置信度 | 用户确认、重复命中、最近验证、项目事实 |
| 当前相关 | 与当前 cwd、分支、任务、打开文件、用户身份相关 |
| 无一次性进度 | “刚刚做了什么”不应长期进入热层 |
| 指令少而明确 | 避免让旧记忆变成不可取消的系统命令 |
| 有 provenance | 至少知道来源和更新时间 |

热记忆的目标不是完整，而是减少模型当前决策成本。

## 冷记忆的规则

冷记忆可以大，但必须可检索、可整理、可回溯。

冷层应保存：

- 原始 session transcript 或压缩版本。
- tool call evidence。
- 用户纠正和 preference signals。
- 被拒绝的 memory proposal。
- 已归档 skill。
- curation report。
- 旧版本 hot capsule。
- embedding / FTS index。

冷层不应该直接污染 prompt。它通过 recall 工具或 hook 产生候选上下文，并通过 `NONE` gate 避免无关注入。

## 冷热更替模式

冷热更替可以定义为两个方向：promotion 和 demotion。

### Promotion: cold/warm -> hot

触发条件：

- 用户重复纠正同一问题。
- 某条事实在多个任务中被召回并验证。
- 某流程被成功复用。
- 当前任务和冷层 evidence 高相关。
- pre prompt hook 检测到任务类型需要某个 capsule。

promotion 输出应该是 proposal，而不是直接无限追加：

```text
candidate:
  target: hot/project.md
  reason: "被最近 3 次任务复用，且用户确认过"
  evidence:
    - cold/transcripts/2026-05-01.md#...
    - reports/review/2026-05-04.md#...
  patch:
    - add concise fact
```

### Demotion: hot -> warm/cold

触发条件：

- 热记忆超过预算。
- 条目长期未被召回。
- 条目被新事实覆盖。
- 条目太细，适合进入 topic capsule。
- 条目是流程，应转成 skill。

demotion 不能简单删除。更好的做法是：

```text
hot/project.md 删除短条目
warm/topics/build.md 保留详细说明
cold/evidence/... 保留原始来源
reports/curator/... 记录迁移原因
```

## 传统记忆模型如何接入

传统记忆模型不应该替代 Markdown 控制面，而应提供容量能力：

| 能力 | 用途 |
|---|---|
| full-text search | 找专有名词、文件路径、命令、错误信息 |
| vector search | 找语义相似经验 |
| recency/frequency scoring | 判断哪些信号值得 promotion |
| provenance graph | 追踪事实来自哪里、被谁确认 |
| decay | 降低旧而未用的条目权重 |
| consolidation | 合并重复 memory 或 skill |
| conflict detection | 找出互相矛盾的规则和事实 |

OpenClaw 的 hybrid retrieval、promotion scoring、dreaming 和 compaction 前 flush 是这里的上限参考。Hermes 的 hard cap 和 curator 是轻量参考。

## Hook 在冷热层中的位置

冷热记忆需要 hook 才能成为系统能力。

| 阶段 | Hook 做什么 | 记忆层动作 |
|---|---|---|
| session start | 读取 guideline、active hot capsules、安装状态 | hot load |
| pre prompt | 根据当前输入召回 cold/warm，注入短上下文 | cold -> hot ephemeral |
| post tool | 记录错误、成功命令、环境事实候选 | evidence append |
| pre compact | 在上下文压缩前保存关键连续性 | hot/warm flush |
| session end | 总结候选 durable facts 和 skill patches | warm proposal |
| scheduled/idle | 执行 curator/dream/review | promotion/demotion |

这里的关键是：pre prompt 注入可以是 ephemeral，不必永久写 hot 文件。只有被验证、复用或用户确认后才 promotion。

## 与 Hermes 的对应关系

Hermes 当前更偏轻量：

- `MEMORY.md`/`USER.md` 是热事实层，有硬限制。
- `SKILL.md` 是 procedural hot/warm 层，按需加载。
- session search 是冷历史召回。
- curator 是 skill warm/cold 治理。
- self-evolution pipeline 是离线能力演化。

Mnemon 可以在 Hermes 模式上补一个明确的 filesystem cold layer，让长期增长有地方落盘，不把压力都放在 `MEMORY.md` 或 skill catalog 上。

## 与 OpenClaw 的对应关系

OpenClaw 更接近完整冷热系统：

- `MEMORY.md` 是长期 root。
- daily notes 是近期 warm 记忆。
- dreaming state 和 recall store 是 cold/working store。
- semantic search 和 FTS 是检索层。
- compaction 前 silent memory flush 是热层丢失前的保存机制。
- dreaming deep phase 负责 promotion 到 `MEMORY.md`。

Mnemon 不必复制全部 OpenClaw 工程，但应复制其分层思想：不是所有历史都进入 prompt，只有经过召回和整理的内容进入热层。

## 设计判断

Mnemon 的 memory-driven framework 可以采用这样的原则：

1. `GUIDELINE.md` 和活跃 hot memory 给模型直接读。
2. `skills/` 承载可复用行为。
3. `memory/warm/` 承载整理后的 topic/session capsules。
4. `memory/cold/` 承载原始证据和长期历史。
5. index/search 只负责召回，不作为唯一真相。
6. promotion/demotion 必须产生 report。
7. hook 负责触发，不负责无审查地永久改写。

这比“一个 md 无限增长”更可持续，也比“上来就厚 adapter”更适合当前 Mnemon。

## 参考来源

- Hermes memory 文档: <https://hermes-agent.nousresearch.com/docs/user-guide/features/memory>
- Hermes curator 文档: <https://hermes-agent.nousresearch.com/docs/user-guide/features/curator>
- OpenClaw Dreaming: <https://docs.openclaw.ai/concepts/dreaming>
- OpenClaw Compaction: <https://docs.openclaw.ai/concepts/compaction>
- Claude Code Memory: <https://code.claude.com/docs/en/memory>
- 本地源码: `/tmp/mnemon-agent-research-sources/hermes-agent/tools/memory_tool.py`
- 本地源码: `/tmp/mnemon-agent-research-sources/hermes-agent/tools/session_search_tool.py`
- 本地源码: `/tmp/mnemon-agent-research-sources/openclaw/extensions/memory-core/src/short-term-promotion.ts`
