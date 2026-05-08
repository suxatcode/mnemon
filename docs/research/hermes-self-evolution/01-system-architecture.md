# 自进化的系统架构要求

## 结论

自进化不是一个单独的 memory 模块，而是一套系统工程。Hermes 最有参考价值的地方不在于它有 `MEMORY.md`，而在于它把多个能力串成了闭环：

```text
运行时经验
  -> turn-level self-improvement nudge
  -> durable fact memory 或 procedural skill
  -> skill 使用统计和 provenance
  -> idle-triggered curator
  -> consolidation / archive / report / backup
  -> 外部 self-evolution pipeline 用 eval 和 gate 生成 PR
```

Mnemon 如果要实现 memory-driven 的自进化，第一原则应该是：不要把记忆系统当作一个被动数据库，而要把记忆、skill、hook、review、安装方式、回滚方式都设计成系统表面。

## Hermes 的架构形态

Hermes 当前至少有三层自进化能力：

| 层次 | 机制 | 作用 |
|---|---|---|
| 运行时沉淀 | `memory` tool、`skill_manage`、self-improvement prompt | 在解决问题后把稳定事实或可复用流程保存下来 |
| 长期治理 | curator、usage sidecar、active/stale/archived 状态 | 防止 agent-created skills 无限堆积和重复 |
| 离线演化 | Hermes Self-Evolution 的 DSPy + GEPA pipeline | 基于 eval、trace、constraint gate 优化 skills、tool descriptions、prompt sections、code |

这三层的风险不同：

- 事实记忆的风险是污染未来上下文。
- skill 的风险是让错误流程被重复调用。
- prompt/tool/code 演化的风险是改变全局行为。

因此 Hermes 没有把所有东西交给一个后台 agent 自动改写。curator 只处理 agent-created skills，不触碰 bundled/hub skills；自进化 repo 通过测试、大小限制、benchmark gate 和 PR 流程交付候选，不直接改当前会话。

## 自进化需要的架构面

Mnemon 的自进化架构至少要暴露以下表面。

| 表面 | 目的 | 不具备时的失败模式 |
|---|---|---|
| 可演化 artifacts | 明确什么能被改：`SKILL.md`、`GUIDELINE.md`、hook prompt、安装文档、索引元数据 | 模型把所有上下文都当成可重写对象 |
| 不可演化边界 | 明确什么不能被后台改：用户当前指令、原始 evidence、secrets、运行时 schema | 旧记忆覆盖当前事实，或后台任务误改配置 |
| 触发点 | 在 session start、pre prompt、post tool、pre compact、session end、cron 等阶段运行 recall/flush/review | 只能靠模型主观想起要记忆 |
| 记忆分层 | 热记忆给模型直接读，冷记忆由工程层存储和召回 | 单个 md 越写越长，最终被截断或污染 prompt |
| provenance | 知道条目来自用户确认、工具观察、模型推断、curator 合并还是外部导入 | 无法判断可信度和是否该覆盖 |
| 使用统计 | 记录 skill/view/use/patch 等信号 | 无法知道什么该保留、合并或归档 |
| 审查与回滚 | diff、dry-run、报告、备份、archive | 自进化变成不可解释的后台改写 |
| 评估 gate | size、测试、benchmark、LLM judge、golden cases | 优化只凭模型感觉，难以防回归 |

这也是为什么 self-evolution 应该是 framework-level capability，而不是 `memory.add()` 的增强版。

## Hermes 的关键约束

Hermes 的实现给出了一组很实际的边界。

| 约束 | Hermes 做法 | 对 Mnemon 的意义 |
|---|---|---|
| 活跃会话隔离 | curator 使用后台 fork，不污染 active conversation 和主 prompt cache | 维护任务不能在用户任务中热替换上下文 |
| first-run defer | curator 第一次只记录时间，不立即改 skill library | 安装后应先给用户审查机会 |
| dry-run | `hermes curator run --dry-run` 只输出报告不变更 | Mnemon 的 review/dream 应先产生 proposal |
| recoverable archive | curator 最坏动作是移入 `.archive/`，不是删除 | 长期整理应可恢复 |
| bundled/hub 保护 | curator 不碰外部安装或内置 skills | Mnemon 应区分用户、agent、package、project 来源 |
| pinned 保护 | pinned skill 跳过自动转移和归档 | 用户可以显式冻结重要行为资产 |
| aux model | curator 可使用辅助模型 | 自进化维护可和主会话模型分离 |
| report | curator 写 `run.json` 和 `REPORT.md` | 后台维护必须留下可审查记录 |

这些约束共同说明：自进化需要“变更治理”，不只是“让 agent 写文件”。

## Hermes Self-Evolution 的位置

Hermes Self-Evolution repo 不是运行时 memory 模块，而是离线优化器。它的流程是：

```text
读取当前 skill/prompt/tool
  -> 生成或导入 eval dataset
  -> GEPA / DSPy 优化候选版本
  -> holdout 评估
  -> constraint gates
  -> 产出最佳候选
  -> PR
```

它把演化目标分成几个风险等级：

| 目标 | 风险 | 典型 gate |
|---|---|---|
| skill 文件 | 低到中 | 结构、大小、eval、测试 |
| tool description | 中 | 描述长度、参数说明、语义保持 |
| system prompt section | 中到高 | 最大增长率、行为回归、benchmark |
| tool implementation code | 高 | full tests、benchmark、human review |

这对 Mnemon 很关键：我们不应该一开始就把“自进化”定义为自动改代码。第一阶段更适合演化 Markdown 行为资产，再逐渐把评估和 PR gate 加进来。

## OpenClaw 与 Claude Code 的旁证

OpenClaw 证明了重工程化路线可以把 memory runtime 做得很完整。它有 compaction 前 silent memory flush、dreaming、promotion lock、daily notes、semantic retrieval、hook pack、cron sweep。这是高容量、长期运行系统的上限参考。

Claude Code 证明了主流 coding agent 的行为层仍然强烈依赖 Markdown。`CLAUDE.md`、auto memory、rules、skills、hooks 和 scheduled tasks 形成了可安装、可编辑、可审查的控制面，但它没有要求每个项目先实现复杂 adapter。

这两者和 Hermes 的共同点是：真正有用的不是某个“记忆模块”，而是让模型在合适阶段看见合适的行为资产，并让这些资产可以被人和 agent 一起维护。

## 对 Mnemon 的架构要求

Mnemon 的自进化 framework 可以先按下面的系统形态设计：

```text
project/
  INSTALL.md        # 如何给当前 agent 安装 hooks、skills、guidelines
  GUIDELINE.md      # 记忆与自进化的初始行为准则
  skills/           # 可复用流程，everything is skill
  memory/
    hot/            # 小而稳定，直接进入 prompt 或 hook 注入
    warm/           # md capsules、topic notes、session summaries
    cold/           # 原始 evidence、索引、历史、transcripts
  reports/
    review/         # 每次 curator/dream/review 的可审查输出
```

第一阶段不要求所有 runtime 共享同一个 adapter。更合理的安装方式是：让目标 agent 根据 `INSTALL.md` 自己安装符合其平台的 hooks，并让 hooks 在四个阶段做有限、清晰的事情：

1. recall：进入模型前召回相关热记忆。
2. observe：工具调用和用户纠正后记录候选信号。
3. reflect：turn/session 结束时生成 memory/skill proposal。
4. curate：空闲或手动运行时整理、合并、归档。

## 设计判断

Mnemon 需要学习 Hermes 的系统性，而不是复制 Hermes 的所有实现。最重要的是：

- 自进化对象要 Markdown-first。
- 运行时要 hook-first。
- 记忆要 hot/cold split。
- 维护任务要 dry-run-first。
- 高风险变更要 proposal/PR-first。
- skill 是主表达方式，memory 只保留事实和偏好。

如果不做这些约束，自进化会退化成“LLM 往一个文件里追加越来越多内容”。这短期可用，长期会失控。

## 参考来源

- Hermes curator: <https://hermes-agent.nousresearch.com/docs/user-guide/features/curator>
- Hermes hooks: <https://hermes-agent.nousresearch.com/docs/user-guide/features/hooks>
- Hermes cron: <https://hermes-agent.nousresearch.com/docs/user-guide/features/cron>
- Hermes Self-Evolution: <https://github.com/NousResearch/hermes-agent-self-evolution>
- OpenClaw compaction: <https://docs.openclaw.ai/concepts/compaction>
- Claude Code memory: <https://code.claude.com/docs/en/memory>
