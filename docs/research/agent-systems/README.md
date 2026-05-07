# Agent 记忆与自进化系统调研

> 本目录记录 Mnemon 设计讨论所需的外部系统调研。所有正文使用中文。Claude Code 部分只基于公开官方文档与公开社区讨论，不下载、引用或复现泄漏源码。

## 研究对象

| 系统 | 文档 | 研究重点 |
|---|---|---|
| Claude Code | [架构](claude-code/01-architecture.md), [记忆与 Markdown](claude-code/02-memory-evolution-markdown-prompts.md), [生命周期详表](claude-code/03-memory-lifecycle-details.md) | `CLAUDE.md`、settings、hooks、subagents、skills、commands |
| Codex | [架构](codex/01-architecture.md), [记忆与 Markdown](codex/02-memory-evolution-markdown-prompts.md), [生命周期详表](codex/03-memory-lifecycle-details.md) | `AGENTS.md`、hooks、skills、memories、本地源码结构 |
| OpenClaw | [架构](openclaw/01-architecture.md), [记忆与 Markdown](openclaw/02-memory-evolution-markdown-prompts.md), [生命周期详表](openclaw/03-memory-lifecycle-details.md) | memory-core、active-memory、memory-wiki、dreaming、plugin hooks |
| Hermes | [架构](hermes/01-architecture.md), [记忆与 Markdown](hermes/02-memory-evolution-markdown-prompts.md), [生命周期详表](hermes/03-memory-lifecycle-details.md) | `MEMORY.md`/`USER.md`、skills、session search、self-evolution |
| ALMA | [概览](alma/01-overview.md), [记忆与演化](alma/02-memory-evolution-markdown-prompts.md), [生命周期详表](alma/03-memory-lifecycle-details.md) | ALMA meta-learning memory design 与 ALMA-memory library 两条线 |
| Agno | [概览](agno/01-overview.md), [记忆与 Markdown](agno/02-memory-evolution-markdown-prompts.md), [生命周期详表](agno/03-memory-lifecycle-details.md) | MemoryManager、agentic memory、session summary、knowledge markdown |
| Letta | [概览](letta/01-overview.md), [记忆与 Markdown](letta/02-memory-evolution-markdown-prompts.md), [生命周期详表](letta/03-memory-lifecycle-details.md) | MemGPT memory hierarchy、core/archival/recall memory、memory tools |

补充资料：[社区讨论与外部文章索引](community-discussions.md) 汇总 Reddit、博客、论文和第三方文章，只作为实践信号，不作为规范事实。

## 生命周期横向速览

| 系统 | 长度/容量控制 | 超出处理 | 整理/定时机制 |
|---|---|---|---|
| Claude Code | `CLAUDE.md` 无公开字符硬上限；skill body compaction 后每个 5,000 tokens、总 25,000 tokens | `/compact` 或自动 compaction；root 指令和 auto memory 从磁盘重注入，path-scoped 内容需再次触发 | 人工/agent 整理 Markdown；scheduled tasks 是通用自动化，不是专门 memory scheduler |
| Codex | raw memories consolidation 默认 256、cap 4096；rollouts/startup 默认 16、cap 128；有 project doc/history/tool output 限制 | idle/age/rate-limit eligibility；history compaction；工具输出 token budget | 后台 thread extraction + global consolidation，不是 cron；required rules 仍进 `AGENTS.md` |
| OpenClaw | active-memory summary 220 chars；partial transcript 32,000 chars；read 2,000 lines/50MB；search query 480 chars | auto-compaction 默认开；compaction 前可 silent memory flush | Dreaming opt-in，cron 默认 `0 3 * * *`；light/REM/deep promotion |
| Hermes | `MEMORY.md` 2,200 chars；`USER.md` 1,375 chars；skills 目标 <=15KB | add 超限返回错误和现有 entries，agent 需 replace/remove/consolidate | 超过 80% 建议 consolidation；Autonomous Curator 默认 7-day cycle |
| ALMA | `BudgetConfig(max_tokens=4000)`；MemoryStack prompt 默认 2,000 tokens；多种 retrieval top_k | budget-aware retrieval 排除超预算项；MemoryStack 到预算后截断 | explicit consolidate/forget/checkpoint；alma-meta 是实验 driver，无核心 cron |
| Agno | 无全局 memory char hard cap；Markdown chunk 默认 5,000 chars；默认 history 3 runs | 关闭 auto context injection；50+ memories 或高成本操作前 optimize | run 内后台 memory update；`optimize_memories` 显式合并；SchedulerTools 是通用调度 |
| Letta | block metadata limit；源码常量 persona/human 20,000 chars、core block 100,000 chars；context 默认 128,000 tokens | 自动 compaction；sliding window 默认总结约 30%，不够则更激进 | core 事件/溢出驱动；Letta Code MemFS 可用 step count 或 compaction event 触发 reflection |

## 方法边界

- 源码优先：对开源系统优先读取本地源码快照，记录关键文件路径。
- 官方文档优先：对 Codex 和 Claude Code，使用官方文档核验当前行为。
- 生命周期详表：对每个系统单独检查记忆长度/容量限制、超出处理、整理/合并方式、后台或定时任务、读写路径和安全边界。
- 社区讨论只作信号：Reddit、博客、第三方文章用于观察实践倾向，不作为规范事实。
- 不处理泄漏源码：Claude Code 架构分析只基于公开文档、公开可见行为和社区实践。

## 总体结论

1. **最接近 Mnemon 当前设计方向的是 Hermes。** Hermes 把 durable fact 放进 bounded memory 文件，把 procedure 放进 skills，并让 agent 在复杂任务后把成功流程沉淀为 `SKILL.md`。这与 Mnemon 现在的 `SKILL.md` + `INSTALL.md` + `GUIDELINE.md` + hook phase 设计高度一致。
2. **Codex 和 Claude Code 证明 Markdown 是 agent 行为层的主流载体。** Codex 用 `AGENTS.md`、skills、hooks、generated memories；Claude Code 用 `CLAUDE.md`、skills、commands、subagents、settings hooks。二者都没有要求每个项目先实现复杂 adapter。
3. **OpenClaw 是重工程化上限。** 它把 memory-core、active-memory、memory-wiki、dreaming、plugin hooks 做成完整运行时能力。它非常强，但对 Mnemon 的第一阶段来说更像上限参考，不应照搬。
4. **Letta 和 ALMA 展示重型记忆路线。** Letta 是结构化 agent memory runtime；ALMA meta 甚至让 LLM 生成并评估新的 memory structure 代码。它们适合长期研究，但不是 Mnemon 当前轻量 harness 的起点。
5. **社区实践更偏向 md + LLM。** Claude Code/Hermes/OpenClaw 社区里常见模式是：短主指令、长 guideline、skills/commands 承载流程、hooks 在关键阶段提醒、human review 控制长期行为变更。

## 对 Mnemon 的设计启发

Mnemon 的自进化 framework 第一阶段应保持：

```text
experience
  -> mnemon remember / recall / link
  -> LLM reflection
  -> candidate patch to SKILL.md / GUIDELINE.md / INSTALL.md / project rule
  -> review
  -> installed markdown behavior
```

不应在第一阶段做：

- 为每个 runtime 写厚 adapter；
- 自动把每段对话写入 memory；
- 自动改写 agent runtime 行为；
- 把 workflow 放进 fact memory；
- 让旧 memory 覆盖当前仓库事实和当前用户指令。

## 主要来源

源码快照：

- Hermes Agent: `/tmp/mnemon-agent-research-sources/hermes-agent`, HEAD `04918345ea31b1106d2ee6d4f42822f4f57616ee`
- Hermes Self-Evolution: `/tmp/mnemon-agent-research-sources/hermes-agent-self-evolution`, HEAD `4693c8f0eed21e39f065c6f38d98d2a403a04095`
- Codex: `/tmp/mnemon-agent-research-sources/codex`
- OpenClaw: `/tmp/mnemon-agent-research-sources/openclaw`
- Agno: `/tmp/mnemon-agent-research-sources/agno`
- Letta: `/tmp/mnemon-agent-research-sources/letta`, HEAD `bb52a8900a79cf1378e6e9cdecf244b673a13a72`
- ALMA meta: `/tmp/mnemon-agent-research-sources/alma-meta`
- ALMA-memory: `/tmp/mnemon-agent-research-sources/alma-memory`

官方与公开资料：

- OpenAI Codex docs: [AGENTS.md](https://developers.openai.com/codex/guides/agents-md), [Memories](https://developers.openai.com/codex/memories), [Hooks](https://developers.openai.com/codex/hooks), [Config reference](https://developers.openai.com/codex/config-reference)
- Claude Code docs: [Memory](https://code.claude.com/docs/en/memory), [Context window](https://code.claude.com/docs/en/context-window), [Scheduled tasks](https://code.claude.com/docs/en/scheduled-tasks), [Subagents](https://code.claude.com/docs/en/sub-agents), [Hooks](https://code.claude.com/docs/en/hooks), [Skills / custom commands](https://code.claude.com/docs/en/slash-commands), [Settings](https://code.claude.com/docs/en/settings)
- Hermes public site: [hermes-ai.net](https://hermes-ai.net/)
- OpenClaw docs: [Memory overview](https://docs.openclaw.ai/concepts/memory), [Dreaming](https://docs.openclaw.ai/concepts/dreaming), [Compaction](https://docs.openclaw.ai/concepts/compaction), [Active memory](https://docs.openclaw.ai/concepts/active-memory), local `docs/concepts/memory.md`, local `docs/concepts/dreaming.md`
- Letta docs: [Stateful agents](https://docs.letta.com/guides/core-concepts/stateful-agents), [Memory blocks](https://docs.letta.com/guides/core-concepts/memory/memory-blocks), [Compaction](https://docs.letta.com/guides/core-concepts/messages/compaction), [Letta Code Memory](https://docs.letta.com/letta-code/memory/), [Archival memory](https://docs.letta.com/guides/core-concepts/memory/archival-memory), [MemGPT paper](https://arxiv.org/abs/2310.08560)
- ALMA paper page: [Learning to Continually Learn via Meta-learning Agentic Memory Designs](https://arxiv.org/abs/2602.07755)
- Agno docs: [Working with Memories](https://docs.agno.com/memory/working-with-memories/overview), [Memory](https://docs-v1.agno.com/agents/memory), [Agent reference](https://docs.agno.com/reference/agents/agent)
