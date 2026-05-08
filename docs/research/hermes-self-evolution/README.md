# Hermes 自进化能力专题研究

本目录面向 Mnemon 当前的 memory-driven framework 设计，做一次更聚焦的研究。主对象是 Hermes Agent 的自进化能力，OpenClaw 和 Claude Code 只作为辅助参照。

本次不按项目分文件，而按自进化系统需要的层次组织：

| 层次 | 文档 | 关注点 |
|---|---|---|
| 系统架构 | [01-system-architecture.md](01-system-architecture.md) | 为什么自进化不是单一模块，而是需要架构层支持 |
| Everything is skill | [02-everything-is-skill.md](02-everything-is-skill.md) | 为什么 Hermes 把流程性经验沉淀为 skill，而不是放进事实记忆 |
| Markdown 记忆 | [03-markdown-memory-rationale.md](03-markdown-memory-rationale.md) | 为什么热门 agent 普遍选择 md + LLM，而不是先做厚工程化记忆 |
| 冷热记忆 | [04-hot-cold-memory-filesystem.md](04-hot-cold-memory-filesystem.md) | 如何用热记忆服务模型，用冷记忆解决长期容量问题 |
| 整理与 dreaming | [05-curation-dreaming-lifecycle.md](05-curation-dreaming-lifecycle.md) | Hermes curator 与 OpenClaw dreaming 对长期增长的处理 |
| Hook / nudge / remind | [06-hooks-nudges-reminders.md](06-hooks-nudges-reminders.md) | 触发点如何支撑 recall、reflect、flush、curate |
| Mnemon 启示 | [07-mnemon-design-implications.md](07-mnemon-design-implications.md) | 对 Mnemon 当前设计的具体建议 |

## 核心结论

1. **Hermes 的自进化不是一个 memory 模块。** 它由 bounded memory、skill library、self-improvement nudge、curator、cron、hooks、辅助模型、报告与回滚策略共同构成。把它复制成一个 adapter 会丢掉重点。
2. **Everything is skill 是架构约束，不只是组织习惯。** Hermes 把稳定事实放进 `MEMORY.md`/`USER.md`，把流程、工具坑点、可复用方法放进 `SKILL.md`，再用 curator 把过窄 skill 合并成 umbrella skill。
3. **Markdown 是 agent 可直接操作的行为层。** Claude Code 的 `CLAUDE.md`/auto memory、Hermes 的 `MEMORY.md`/skills、OpenClaw 的 `MEMORY.md`/`DREAMS.md` 都说明，md 的价值在于 LLM 可读、可写、可审查、可 diff、可由 agent 自行安装。
4. **Markdown 不解决长期容量。** 当记忆长期增长，单个 md 文件会遇到上下文预算、冲突、过时、噪音和被截断的问题。Claude Code 对 auto memory 有启动加载上限，Hermes 对 `MEMORY.md`/`USER.md` 有硬字符限制，OpenClaw 则引入 dreaming、索引和 promotion。
5. **更适合 Mnemon 的路线是热冷分层。** 模型直接消费小而清晰的热记忆；工程层负责冷记忆落盘、索引、证据、历史、召回、promotion 与 demotion。filesystem 是可审查的控制面，传统记忆模型是容量面。
6. **hook 是自进化的触发底座。** 没有 session start、pre prompt、post tool、pre compact、session end、scheduled review 这些触发点，自进化只能靠模型偶尔想起，不能成为系统能力。

## 主要参考来源

- Hermes Agent curator 文档: <https://hermes-agent.nousresearch.com/docs/user-guide/features/curator>
- Hermes Agent memory 文档: <https://hermes-agent.nousresearch.com/docs/user-guide/features/memory>
- Hermes Agent hooks 文档: <https://hermes-agent.nousresearch.com/docs/user-guide/features/hooks>
- Hermes Agent cron 文档: <https://hermes-agent.nousresearch.com/docs/user-guide/features/cron>
- Hermes Agent Self-Evolution: <https://github.com/NousResearch/hermes-agent-self-evolution>
- OpenClaw Dreaming: <https://docs.openclaw.ai/concepts/dreaming>
- OpenClaw Compaction: <https://docs.openclaw.ai/concepts/compaction>
- OpenClaw Hooks: <https://docs.openclaw.ai/automation/hooks>
- Claude Code Memory: <https://code.claude.com/docs/en/memory>
- Claude Code Context Window: <https://code.claude.com/docs/en/context-window>
- Claude Code Scheduled Tasks: <https://code.claude.com/docs/en/scheduled-tasks>
- Claude Code Hooks: <https://code.claude.com/docs/en/hooks>

本地源码快照也被用于核对实现细节，尤其是 Hermes 的 `tools/memory_tool.py`、`tools/skill_manager_tool.py`、`agent/curator.py`、`tools/skill_usage.py`、`agent/prompt_builder.py`、`cron/scheduler.py`，以及 Hermes Self-Evolution 的 `PLAN.md`、`evolution/core/config.py`、`evolution/core/constraints.py`。
