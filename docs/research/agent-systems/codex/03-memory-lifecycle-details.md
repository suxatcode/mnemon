# Codex memory lifecycle 细节

## 核心判断

Codex 的 memories 是「线程提取 + 后台合并 + 生成式文件系统 memory」路线。官方文档强调 memories 默认关闭，启用后从 eligible prior threads 中提取稳定上下文，并在后台更新本地 memory files。源码快照显示它进一步分成 phase 1 extraction 和 phase 2 consolidation。

对 Mnemon 来说，Codex 证明了一个重要边界：必须规则放 `AGENTS.md` 或仓库文档，generated memories 只作为 recall layer。Mnemon 的 `GUIDELINE.md`/`INSTALL.md` 也应是受审查的规则层，memory 只提出候选。

## 生命周期详表

| 维度 | 观察 |
|---|---|
| 主要记忆载体 | `~/.codex/memories/` 下的 generated memory files，包含 summaries、durable entries、recent inputs、supporting evidence。 |
| 项目规则载体 | `AGENTS.md`、checked-in docs、skills、hooks。官方明确 required team guidance 不应只放 memories。 |
| 启用方式 | `[features] memories = true`；memory feature 默认关闭。 |
| 线程级控制 | `/memories` 可控制当前 thread 是否使用既有 memories、是否允许当前 thread 生成未来 memories。 |
| 写入触发 | 后台处理 eligible prior threads；跳过 active 或 short-lived sessions；不会在线程结束时立刻强制写。 |
| 速率保护 | 当 Codex rate-limit remaining percentage 低于配置阈值时，后台 memory generation 可跳过。 |
| 长度/数量限制 | 官方配置：`max_raw_memories_for_consolidation` 默认 256、cap 4096；`max_rollouts_per_startup` 默认 16、cap 128；`max_rollout_age_days` 默认 30、clamp 0-90；`max_unused_days` 默认 30、clamp 0-365。 |
| 上下文限制 | `model_auto_compact_token_limit` 控制自动历史压缩阈值；`model_context_window` 可声明模型上下文；`tool_output_token_limit` 限制单个工具输出进入历史的 token budget；`history.max_bytes` 可裁剪本地历史文件。 |
| 项目文档限制 | `project_doc_max_bytes` 限制读取 `AGENTS.md` 的最大字节数。 |
| 整理方式 | phase 2 consolidation agent 把 raw memories 合并成 `MEMORY.md`、`memory_summary.md`、`skills/`、`rollout_summaries/` 等文件。 |
| 超出处理 | raw memory 候选按数量、年龄、unused days、usage/recentness 选择；上下文通过 history compaction；工具输出通过 token limit 截断或限制进入历史。 |
| 定时/后台 | 不是 cron；在 startup/resume 等时机异步后台处理，且需要 thread idle 足够久。 |
| 安全边界 | 生成字段会 redacts secrets；可配置 `disable_on_external_context` 避免把使用 MCP/web/tool search 的 thread 纳入 memory generation。 |

## 源码快照中的双阶段机制

本地 Codex 源码快照中的 memories pipeline 更细：

1. root session start 时，如果 memories enabled、非 ephemeral、非 subagent、state DB 可用，就启动后台任务。
2. phase 1 选择 eligible rollout，把线程内容送入 extraction prompt，输出结构化 raw memory。
3. extraction prompt 有 no-op gate，优先稳定偏好、重复 workflow、项目约定、环境坑点，排除 secrets、大段输出和短期任务进度。
4. phase 2 持有全局锁，选择近期 raw memories，写入 staging workspace。
5. consolidation agent 在受限环境中把 raw memories 合并成长期 memory 文件、skills 和 summary。
6. read path 要求主 agent 先做快速 memory pass，并在使用 memory 时输出 citation block。

这套设计非常完整，但也明显比 Mnemon 第一阶段重。Mnemon 不需要复制 state DB、lease、internal consolidation agent 和 generated workspace，只需要借鉴「候选提取 -> Markdown patch -> 审查安装」。

## 超出与整理策略

Codex 对超出的处理不是单点截断，而是多层预算：

- thread eligibility：年龄、idle 时间、active 状态、startup 处理数量。
- raw memory pool：最多保留近期 raw memories，且会忽略太久未使用的 memory。
- project instructions：`AGENTS.md` 有读取字节上限。
- history：自动 compaction、工具输出 token limit、本地 history file size。
- consolidation：把多个 raw observations 合并到更短的 durable form。

这说明 memory-driven framework 需要先定义「什么值得保留」，再定义「如何在超出时合并」。只追加不整理会很快失败。

## Hooks 与 Mnemon 四阶段

Codex hooks 支持 `SessionStart`、`UserPromptSubmit`、`PreToolUse`、`PermissionRequest`、`PostToolUse`、`Stop`。其中最适合 Mnemon 的四阶段可以映射为：

| Mnemon 阶段 | Codex hook 对应 | 作用 |
|---|---|---|
| 启动召回 | `SessionStart` | 注入 guideline、项目 memory 索引、最近关键状态。 |
| 输入前判定 | `UserPromptSubmit` | 判断本轮是否需要 recall、是否有隐私/安全风险。 |
| 工具后采样 | `PostToolUse` | 记录命令结果、失败原因、可复用 workflow 证据。 |
| 结束沉淀 | `Stop` | 要求 agent 总结候选 memory/skill/guideline patch，必要时继续一轮。 |

## 对 Mnemon 的启发

- `memories` 默认应是辅助召回，不替代 `GUIDELINE.md`。
- 安装层应通过 `INSTALL.md` 让 agent 自己配置 hooks。
- 每个 hook 只做轻量提醒或产出候选，不应强行接管 agent loop。
- memory 需要 no-op gate、secret redaction、evidence、scope 和 outdated handling。
- 长流程沉淀成 `SKILL.md`，事实和偏好沉淀成 bounded memory，规范沉淀到 `GUIDELINE.md`。

## 参考来源

- 官方文档: [Codex Memories](https://developers.openai.com/codex/memories)
- 官方文档: [Codex Hooks](https://developers.openai.com/codex/hooks)
- 官方文档: [Codex Config Reference](https://developers.openai.com/codex/config-reference)
- 本地源码: `/tmp/mnemon-agent-research-sources/codex/codex-rs/memories/README.md`
- 本地源码: `/tmp/mnemon-agent-research-sources/codex/codex-rs/memories/read/templates/memories/read_path.md`
- 本地源码: `/tmp/mnemon-agent-research-sources/codex/codex-rs/memories/write/templates/memories/stage_one_system.md`
- 本地源码: `/tmp/mnemon-agent-research-sources/codex/codex-rs/memories/write/templates/memories/consolidation.md`
