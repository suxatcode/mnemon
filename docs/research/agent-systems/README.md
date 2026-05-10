# Agent Systems Research

本目录保留 Mnemon self-evolution harness 设计的来源索引与研究摘要。详细分项目调研已经浓缩进 [Self-Evolution Harness 设计](../../design/SELF_EVOLUTION_HARNESS.md)，不再维护多份长研究笔记。

## Scope

研究对象：

| System | Research focus |
|---|---|
| Claude Code | Markdown memory, `CLAUDE.md`, hooks, skills/commands, scheduled tasks |
| Codex | `AGENTS.md`, hooks, skills, generated memories, local configuration |
| OpenClaw | active memory, memory wiki, dreaming, plugin hooks |
| Hermes | bounded Markdown memory, skills, curator, background review, usage sidecar |
| Letta | stateful agent memory, core/archival/recall memory, compaction |
| ALMA | meta-learning memory design and memory-structure experimentation |
| Agno | framework-level memory manager, session summaries, explicit memory optimization |

## Cross-System Conclusions

1. Markdown is the most portable behavior control plane across current agent systems.
2. Skills are the natural carrier for procedural memory.
3. Prompt-facing memory must stay small, bounded, and reviewable.
4. Long-term memory needs retrieval, evidence links, and consolidation rather than full prompt loading.
5. Background maintenance needs provenance, reports, backups, and hard write boundaries.
6. Host-specific adapters should be convenience scripts, not core architecture.

## Source Snapshots

Local source snapshots used during the design process:

| Source | Local snapshot |
|---|---|
| Hermes Agent | `/tmp/mnemon-agent-research-sources/hermes-agent`, HEAD `04918345ea31b1106d2ee6d4f42822f4f57616ee` |
| Hermes Self-Evolution | `/tmp/mnemon-agent-research-sources/hermes-agent-self-evolution`, HEAD `4693c8f0eed21e39f065c6f38d98d2a403a04095` |
| Codex | `/tmp/mnemon-agent-research-sources/codex` |
| OpenClaw | `/tmp/mnemon-agent-research-sources/openclaw` |
| Agno | `/tmp/mnemon-agent-research-sources/agno` |
| Letta | `/tmp/mnemon-agent-research-sources/letta`, HEAD `bb52a8900a79cf1378e6e9cdecf244b673a13a72` |
| ALMA meta | `/tmp/mnemon-agent-research-sources/alma-meta` |
| ALMA-memory | `/tmp/mnemon-agent-research-sources/alma-memory` |

## Public References

- OpenAI Codex docs: [AGENTS.md](https://developers.openai.com/codex/guides/agents-md), [Memories](https://developers.openai.com/codex/memories), [Hooks](https://developers.openai.com/codex/hooks), [Config reference](https://developers.openai.com/codex/config-reference)
- Claude Code docs: [Memory](https://code.claude.com/docs/en/memory), [Context window](https://code.claude.com/docs/en/context-window), [Scheduled tasks](https://code.claude.com/docs/en/scheduled-tasks), [Subagents](https://code.claude.com/docs/en/sub-agents), [Hooks](https://code.claude.com/docs/en/hooks), [Skills / custom commands](https://code.claude.com/docs/en/slash-commands), [Settings](https://code.claude.com/docs/en/settings)
- Hermes public site: [hermes-ai.net](https://hermes-ai.net/)
- OpenClaw docs: [Memory overview](https://docs.openclaw.ai/concepts/memory), [Dreaming](https://docs.openclaw.ai/concepts/dreaming), [Compaction](https://docs.openclaw.ai/concepts/compaction), [Active memory](https://docs.openclaw.ai/concepts/active-memory)
- Letta docs: [Stateful agents](https://docs.letta.com/guides/core-concepts/stateful-agents), [Memory blocks](https://docs.letta.com/guides/core-concepts/memory/memory-blocks), [Compaction](https://docs.letta.com/guides/core-concepts/messages/compaction), [Letta Code Memory](https://docs.letta.com/letta-code/memory/), [Archival memory](https://docs.letta.com/guides/core-concepts/memory/archival-memory), [MemGPT paper](https://arxiv.org/abs/2310.08560)
- ALMA paper page: [Learning to Continually Learn via Meta-learning Agentic Memory Designs](https://arxiv.org/abs/2602.07755)
- Agno docs: [Working with Memories](https://docs.agno.com/memory/working-with-memories/overview), [Memory](https://docs-v1.agno.com/agents/memory), [Agent reference](https://docs.agno.com/reference/agents/agent)

## Research Policy

- Source and official docs are preferred over community summaries.
- Community discussions are practice signals, not normative facts.
- Architecture terms belong to Mnemon; external system names appear here only as references.
- Earlier per-system long notes remain available in git history before the v0.2 documentation consolidation.
