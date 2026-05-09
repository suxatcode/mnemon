# 社区讨论与外部文章索引

> 本文件收集公开社区讨论和外部文章。它们用于观察实践倾向，不作为源码或官方规范事实。结论仍以官方文档和开源源码为主。

## Claude Code

| 来源 | 相关信号 |
|---|---|
| [Claude Code is a build system, not a chatbot](https://www.reddit.com/r/ClaudeCode/comments/1swcwb6/claude_code_is_a_build_system_not_a_chatbot_13/) | 社区实践偏向短 `CLAUDE.md`、长标准文档、少量 hooks、subagents 做隔离任务 |
| [CLAUDE.md, rules, hooks, agents, commands, skills...](https://www.reddit.com/r/ClaudeCode/comments/1pxou18/claudemd_rules_hooks_agents_commands_skills/) | 开发者正在讨论何时用 `CLAUDE.md`、skill、command、subagent、hook 分层 |
| [Anthropic best practices discussion](https://www.reddit.com/r/ClaudeCode/comments/1k2rz7l/claude_code_best_practices_for_agentic_coding/) | 社区围绕官方 best practices 总结 agentic coding 工作流 |

观察：Claude Code 社区并不倾向把所有规则放进一个巨大 prompt，而是用 Markdown 资产分层。

## Hermes

| 来源 | 相关信号 |
|---|---|
| [Hermes Agent public site](https://hermes-ai.net/) | 官方宣传 closed learning loop：memory、skills、session search、user modeling |
| [How Skills Work in Hermes Agent](https://www.reddit.com/r/hermesagent/comments/1smlqdt/how_skills_work_in_hermes_agent/) | 社区明确把 skills 称为 procedural memory，memory 存 facts，sessions 存 history |
| [Hermes Agent Self-Evolution discussion](https://www.reddit.com/r/hermesagent/comments/1t5ifvg/nous_research_just_dropped_hermes_agent/) | 社区测试 DSPy + GEPA 对 skills 做迭代优化，印证「skill 文件自演化」路线 |
| [HermesAgent accumulate persistent skills](https://www.reddit.com/r/hermesagent/comments/1t62ii2/hermesagent_accumulate_persistent_skills_instead/) | 社区把 skill compounding 看作跨任务学习核心 |

观察：Hermes 社区实践非常接近 Mnemon 当前思路：facts、sessions、skills 分层，技能复利比单纯聊天记忆更重要。

## OpenClaw

| 来源 | 相关信号 |
|---|---|
| [OpenClaw Active memory](https://docs.openclaw.ai/concepts/active-memory) | active memory 是主回复前的 bounded blocking memory sub-agent |
| [OpenClaw Dreaming explained](https://openclawdc.com/blog/openclaw-dreaming-memory/) | dreaming 被解释为 idle-time consolidation，把旧 daily notes 变成 durable/searchable memory |
| [OpenClaw dreaming guide](https://openclawlaunch.com/guides/openclaw-dreaming) | 社区文档强调 Dream Diary 对调试和审查 memory evolution 有用 |

观察：OpenClaw 社区与文档偏向完整 memory runtime，包括 active recall、dreaming、wiki、review trail。它是能力上限，不是轻量起点。

## ALMA

| 来源 | 相关信号 |
|---|---|
| [ALMA paper](https://arxiv.org/abs/2602.07755) | 研究问题是让 agent 自动 meta-learn memory designs |
| [Hugging Face paper page](https://huggingface.co/papers/2602.07755) | 社区摘要强调减少人工 hand-engineered memory designs |
| [ALMA-memory Reddit release](https://www.reddit.com/r/artificial/comments/1qshlln/i_have_built_alma_a_memory_framework_that_can/) | 工程社区关注 scoped learning、anti-pattern、多 agent sharing |

观察：ALMA 代表「让记忆机制本身演化」的重型研究线，应放在 Mnemon 后续研究阶段。

## Agno

| 来源 | 相关信号 |
|---|---|
| [Agno Memory docs](https://docs-v1.agno.com/agents/memory) | user memories、session summaries、agentic memory 都是可选参数 |
| [Agno Session Summaries](https://docs.agno.com/sessions/session-summaries) | session summary 被定位为降低 token 成本和保持 continuity |
| [Agno production memory best practices](https://docs.agno.com/context/memory/best-practices) | 建议 agentic memory 用较便宜模型，主对话保持强模型 |
| [SurrealDB + Agno memory discussion](https://surrealdb.com/blog/agents-with-memory-how-agno-and-surrealdb-enable-reliable-ai-systems) | 工程讨论集中在 production memory stack、storage、context reliability |

观察：Agno 社区/文档更偏 framework capability 和 production storage，不是 Markdown 行为自演化。

## Letta / MemGPT

| 来源 | 相关信号 |
|---|---|
| [Letta stateful agents](https://docs.letta.com/guides/core-concepts/stateful-agents) | Letta 把 memory blocks、messages 和 tools 作为 stateful agent 的核心组成 |
| [Letta memory blocks](https://docs.letta.com/guides/core-concepts/memory/memory-blocks) | memory blocks 是始终在 context 中、可被 agent 更新的结构化记忆 |
| [Letta archival memory](https://docs.letta.com/guides/core-concepts/memory/archival-memory) | archival memory 是按需检索的外部长期记忆层 |
| [MemGPT is now part of Letta](https://www.letta.com/blog/memgpt-and-letta) | Letta 将 MemGPT 作为 agent design pattern，Letta 作为 framework |
| [Memory Blocks](https://www.letta.com/blog/memory-blocks) | memory blocks 被描述为 agentic context management 的关键 |
| [MemGPT paper](https://arxiv.org/abs/2310.08560) | 操作系统式 memory hierarchy 与 function-mediated paging |

观察：Letta/MemGPT 是强结构化 memory runtime，重点是 agent 自编辑 memory state，而不是 Markdown skill/guideline 自演化。

## 通用 agent memory 研究

| 来源 | 相关信号 |
|---|---|
| [MemSkill](https://arxiv.org/abs/2602.02474) | 把 skill 与 memory evolution 联系起来，支持「procedure 作为可演化记忆」的方向 |
| [MemoryArena](https://arxiv.org/abs/2602.16313) | 评估多 session interdependent agentic tasks 中的 memory |
| [AI Agents Need Memory Control Over More Context](https://arxiv.org/abs/2601.11653) | 关注 bounded internal state 替代 transcript replay |
| [Agent memory mechanisms survey](https://arxiv.org/abs/2603.07670) | 讨论 write-path filtering、contradiction handling、latency budget、privacy governance |

## 对 Mnemon 的总体判断

社区信号与源码观察基本一致：

- 最实用的早期路线是 Markdown 资产 + agent judgment + hooks/reminders。
- 真正有复利的是 procedural memory，即 skills、rules、install notes、eval cases。
- 重型自演化应先输出 reviewable artifacts，不应直接改 runtime 内核。
- 任何自动 memory 写入都需要 no-op gate、scope、provenance、stale handling。
