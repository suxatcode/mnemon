# Agent 记忆与自进化系统调研

> 本目录记录 Mnemon 设计讨论所需的外部系统调研。所有正文使用中文。Claude Code 部分只基于公开官方文档与公开社区讨论，不下载、引用或复现泄漏源码。

## 研究对象

| 系统 | 文档 | 研究重点 |
|---|---|---|
| Claude Code | [架构](claude-code/01-architecture.md), [记忆与 Markdown](claude-code/02-memory-evolution-markdown-prompts.md) | `CLAUDE.md`、settings、hooks、subagents、skills、commands |
| Codex | [架构](codex/01-architecture.md), [记忆与 Markdown](codex/02-memory-evolution-markdown-prompts.md) | `AGENTS.md`、hooks、skills、memories、本地源码结构 |
| OpenClaw | [架构](openclaw/01-architecture.md), [记忆与 Markdown](openclaw/02-memory-evolution-markdown-prompts.md) | memory-core、active-memory、memory-wiki、dreaming、plugin hooks |
| Hermes | [架构](hermes/01-architecture.md), [记忆与 Markdown](hermes/02-memory-evolution-markdown-prompts.md) | `MEMORY.md`/`USER.md`、skills、session search、self-evolution |
| ALMA | [概览](alma/01-overview.md), [记忆与演化](alma/02-memory-evolution-markdown-prompts.md) | ALMA meta-learning memory design 与 ALMA-memory library 两条线 |
| Agno | [概览](agno/01-overview.md), [记忆与 Markdown](agno/02-memory-evolution-markdown-prompts.md) | MemoryManager、agentic memory、session summary、knowledge markdown |
| Letta | [概览](letta/01-overview.md), [记忆与 Markdown](letta/02-memory-evolution-markdown-prompts.md) | MemGPT memory hierarchy、core/archival/recall memory、memory tools |

补充资料：[社区讨论与外部文章索引](community-discussions.md) 汇总 Reddit、博客、论文和第三方文章，只作为实践信号，不作为规范事实。

## 方法边界

- 源码优先：对开源系统优先读取本地源码快照，记录关键文件路径。
- 官方文档优先：对 Codex 和 Claude Code，使用官方文档核验当前行为。
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
- Claude Code docs: [Memory](https://code.claude.com/docs/en/memory), [Subagents](https://code.claude.com/docs/en/sub-agents), [Hooks](https://code.claude.com/docs/en/hooks), [Skills / custom commands](https://code.claude.com/docs/en/slash-commands), [Settings](https://code.claude.com/docs/en/settings)
- Hermes public site: [hermes-ai.net](https://hermes-ai.net/)
- OpenClaw docs: [Active memory](https://docs.openclaw.ai/concepts/active-memory), local `docs/concepts/memory.md`, local `docs/concepts/dreaming.md`
- Letta docs: [Stateful agents](https://docs.letta.com/guides/core-concepts/stateful-agents), [Memory blocks](https://docs.letta.com/guides/core-concepts/memory/memory-blocks), [Archival memory](https://docs.letta.com/guides/core-concepts/memory/archival-memory), [MemGPT paper](https://arxiv.org/abs/2310.08560)
- ALMA paper page: [Learning to Continually Learn via Meta-learning Agentic Memory Designs](https://arxiv.org/abs/2602.07755)
- Agno docs: [Memory](https://docs-v1.agno.com/agents/memory), [Agent reference](https://docs.agno.com/reference/agents/agent)
