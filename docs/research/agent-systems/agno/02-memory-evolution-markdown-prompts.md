# Agno 的记忆、Markdown 与 Prompt 用法

## 记忆处理方案

Agno memory 的核心是 framework-managed：

```text
Agent config flags
  -> MemoryManager
  -> existing user memories inserted into prompt
  -> optional update_user_memory tool
  -> session summary manager
  -> storage backend
```

源码中的 prompt 示例显示，历史 memories 会以 `<memories_from_previous_interactions>` 形式进入 prompt，并提醒 agent 当前对话优先于过去 memory。

## Agentic memory tool

`update_user_memory(task)` 是 Agno 的关键工具：

- agent 可根据对话历史创建/更新/删除/清空 memory；
- prompt 指导 agent 保存 observations、preferences、context；
- tool 层把自然语言 task 交给 `MemoryManager.update_memory_task`；
- `enable_agentic_memory` 或相关 flags 启用后才加入。

这与 Mnemon 的 `remember` 有相似点，但 Agno 更像内置 tool，而 Mnemon 是外部 CLI/protocol。

## Session summary prompt

`session/summary.py` 维护 session summary system prompt，并支持 structured output。它的作用是压缩 session history，而不是替代 durable memory。

Mnemon 可借鉴这一点：Compact phase 应保存关键连续性，不应机械保存完整 transcript。

## Markdown 用法

Agno 的 Markdown 用途更偏数据处理：

- `MarkdownReader` 读取 `.md`/`.markdown`；
- `MarkdownChunking` 按 heading/paragraph 分块；
- print response 可用 rich markdown；
- API schema 有 markdown output flag。

这说明 Agno 不把 Markdown 作为 agent 自我安装和自我演化的主要协议。

## 智能体演化方案

Agno 没有像 Hermes 那样把「成功 workflow -> skill」作为内置闭环。它的演化更像：

- memory manager 根据对话更新 user memory；
- session summary 压缩上下文；
- knowledge base 通过外部数据更新；
- developer 修改 agent code/config。

所以 Agno 对 Mnemon 的启发更偏「memory capability API」，不是「memory-driven self-evolving framework」。

## 对 Mnemon 的设计判断

Agno 强化了几个 guardrail：

- memory feature 应可开关；
- 当前对话和当前事实应优先于过去 memory；
- session summary 与 durable memory 要分层；
- markdown ingestion 和 markdown behavior contract 是两回事。

## 参考来源

- 本地源码: `libs/agno/agno/agent/_messages.py`
- 本地源码: `libs/agno/agno/agent/_default_tools.py`
- 本地源码: `libs/agno/agno/session/summary.py`
- 本地源码: `libs/agno/agno/knowledge/reader/markdown_reader.py`
- 本地源码: `libs/agno/agno/knowledge/chunking/markdown.py`
- 官方文档: [Agno Memory](https://docs-v1.agno.com/agents/memory)
