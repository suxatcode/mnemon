# 为什么热门 Agent 采用 Markdown 记忆

## 结论

Hermes、Claude Code、OpenClaw 都大量使用 Markdown，不是因为 Markdown 是最强的数据库，而是因为它是最适合 LLM 和人共同维护的行为层。

Markdown 解决的是自进化早期最重要的问题：

```text
让模型能读懂
让模型能修改
让人能审查
让 git 能 diff
让安装不依赖厚 adapter
让行为资产能被移植
```

复杂数据库、向量索引、schema、adapter 可以解决容量和检索问题，但它们不适合作为第一层行为表达。Mnemon 更合理的方向是：Markdown 作为控制面，filesystem/数据库/索引作为容量面。

## 三个系统的共同模式

| 系统 | Markdown 载体 | 作用 |
|---|---|---|
| Hermes | `MEMORY.md`、`USER.md`、`SKILL.md`、curator reports | durable facts、用户偏好、procedural skills、review 输出 |
| Claude Code | `CLAUDE.md`、auto memory、`.claude/rules/*.md`、skills | 项目/用户/组织指令、自动学习、路径规则、按需技能 |
| OpenClaw | `MEMORY.md`、`DREAMS.md`、`memory/YYYY-MM-DD.md`、bootstrap files | 长期记忆、dream diary、daily notes、agent bootstrap |

这些系统都没有把“对 agent 的长期行为指导”首先设计成不可读的二进制状态或只存在数据库里的记录。它们都保留了 md 文件作为可见事实来源。

## Markdown 的核心优势

### 1. LLM-native

LLM 对 Markdown 的标题、列表、代码块、表格、引用非常敏感。结构清楚的 md 文件可以直接进入 prompt，不需要额外解释 schema。

这对自进化很重要：模型不仅要读取记忆，还要修改记忆。如果底层是复杂 schema，模型需要学习 adapter 的操作语义；如果底层是 Markdown，它可以直接提出 diff。

### 2. Human-reviewable

自进化最大的风险是 silent drift。Markdown 能让用户看到：

- 新增了什么偏好。
- 哪个流程被改了。
- 哪个旧 skill 被合并。
- 哪条记忆被 demote。
- 哪个 hook prompt 被调整。

Hermes curator 写 `REPORT.md`，OpenClaw dreaming 写 `DREAMS.md`，本质上都是把后台整理过程变成人能读的审查面。

### 3. Git-friendly

Markdown 文件天然适合版本管理。它们可以走 PR、code review、revert、blame、branch compare。

这对 Mnemon 很关键，因为用户已经在讨论 branch、commit、force push。Mnemon 的自进化成果如果能表现为 md diff，就能直接嵌入现有 git 工作流。

### 4. Agent-installable

用户希望用 `INSTALL.md` 描述如何安装 hooks 和 guideline，然后让对应 agent 自己安装。这只有在安装指令本身是模型可读的 Markdown 时才自然。

如果 Mnemon 第一阶段依赖 runtime-specific adapter，那么每个 agent 都需要专门实现。相反，Markdown 让安装变成：

```text
读 INSTALL.md
识别当前 agent 平台
安装对应 hooks
引用 GUIDELINE.md
启用相关 skills
生成审查报告
```

### 5. Progressive disclosure

Markdown 可以很容易拆成：

- 入口文件：短。
- topic 文件：按需。
- skill：按任务。
- support files：长参考。

Claude Code 的 `.claude/rules/` 和 imports、Hermes 的 skill support directories、OpenClaw 的 daily notes 都是这种模式。重点是不要让所有 md 都在每轮进 prompt。

## 为什么不先做复杂工程化记忆

复杂工程化记忆有价值，但不适合作为自进化的第一表达层。

| 工程化方案 | 优势 | 问题 |
|---|---|---|
| 关系数据库 | 强 schema、事务、查询 | 模型不可直接理解，变更需要 adapter |
| 向量数据库 | 语义召回、容量大 | 难审查，容易召回噪音，不能表达流程 |
| 图数据库 | 关系表达强 | 写入和合并规则复杂，维护成本高 |
| 事件流 | provenance 完整 | 需要总结、压缩、索引才能被模型使用 |
| 自定义 runtime adapter | 控制强 | 跨 agent 移植差，安装成本高 |

这些方案更适合“冷记忆”和“检索层”，不适合直接承载 `GUIDELINE.md`、`INSTALL.md`、`SKILL.md` 这类行为资产。

Hermes 的做法很说明问题：它有 SQLite session search、有 usage sidecar、有 curator，但 agent 行为资产仍然是 Markdown skill 和 memory 文件。

## Markdown 的真实限制

Markdown 的问题也很明确：

| 限制 | 表现 | 典型后果 |
|---|---|---|
| 上下文预算 | 文件太长不能全部进 prompt | 旧内容被忽略或降低遵循度 |
| 线性结构 | 难表达复杂关系 | 同义、冲突、重复难发现 |
| 缺少强 schema | 格式漂移 | agent 写法逐渐不一致 |
| 冲突处理弱 | 多个后台任务同时写 | 覆盖、重复、错序 |
| 过时内容难识别 | 没有 last_used/provenance | 旧规则压过新事实 |
| 检索能力弱 | 一个大文件不好查 | 模型读太多或读不到 |

因此“Markdown-first”不等于“只有一个 Markdown 文件”。它应该演化为：

```text
短热记忆 md
  + topic capsules
  + skill library
  + filesystem evidence
  + usage metadata
  + index/search
  + curator/dreaming
```

## 长度限制带来的启示

Hermes 对 `MEMORY.md` 和 `USER.md` 设置了硬字符限制。Claude Code 的 auto memory 在启动时只加载前 200 行或 25KB。Claude Code 文档也建议 `CLAUDE.md` 目标控制在 200 行以下，因为太长会消耗上下文并降低遵循度。

这些数字说明一件事：主流系统并不假设“一个 md 可以无限增长”。它们都在通过限制、拆分、按需加载或整理机制控制热记忆。

这直接支持 Mnemon 的冷热分层设计：

- 热记忆必须短。
- 冷记忆可以大。
- 热记忆不能承担全部历史。
- 大量历史必须通过召回、整理、promotion 进入热层。

## Markdown 与自进化的关系

自进化需要可被模型编辑的对象。Markdown 的好处是可以让模型输出非常具体的 patch：

```text
更新 skills/research/SKILL.md：
- 增加 "source verification" 步骤。
- 把社区帖子降级为 practice signal。
- 新增 "do not cite leaked source" 规则。
```

相比之下，如果系统只暴露 `memory.add("...")`，模型很容易不断追加事实，而不是改进方法。

因此 Mnemon 应把自进化的主要产物定义成：

- `SKILL.md` patch。
- `GUIDELINE.md` patch。
- `INSTALL.md` hook 安装说明 patch。
- memory hot capsule patch。
- curation report。

而不是只定义成“新增一条 memory”。

## 设计判断

社区大量使用 Markdown 的原因不是缺乏工程能力，而是因为 agent 行为资产需要：

- 可解释。
- 可审查。
- 可迁移。
- 可由 LLM 修改。
- 可在没有专用 adapter 时安装。

但 Markdown 的容量上限是真问题。Mnemon 最好的路线不是否定 Markdown，而是把 Markdown 放在正确层级：

```text
Markdown = 热层控制面和可审查 artifact
Filesystem = 中间层组织和证据落盘
传统记忆模型 = 冷层容量、索引、召回、promotion/demotion
```

这样既保留热门 agent 的实践优势，也避免长期增长把一个 md 文件撑爆。

## 参考来源

- Claude Code memory 文档: <https://code.claude.com/docs/en/memory>
- Claude Code context window 文档: <https://code.claude.com/docs/en/context-window>
- Hermes memory 文档: <https://hermes-agent.nousresearch.com/docs/user-guide/features/memory>
- Hermes curator 文档: <https://hermes-agent.nousresearch.com/docs/user-guide/features/curator>
- OpenClaw dreaming 文档: <https://docs.openclaw.ai/concepts/dreaming>
- OpenClaw hooks 文档: <https://docs.openclaw.ai/automation/hooks>
