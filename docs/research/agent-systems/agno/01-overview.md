# Agno 概览

## 一句话结论

Agno 是 agent framework/library，不是一个以 Markdown 行为资产为中心的 coding runtime。它的 memory 主要通过 `MemoryManager`、agent config flags、session summaries 和 knowledge readers 实现。它适合作为「库式 memory capability」参考，但不如 Hermes/Codex/Claude Code 贴近 Mnemon 的 Markdown harness 方向。

## 关键源码证据

本地源码：`/tmp/mnemon-agent-research-sources/agno`

| 位置 | 观察 |
|---|---|
| `libs/agno/agno/agent/_init.py` | 设置 `MemoryManager`，根据 memory flags 添加 memory references |
| `libs/agno/agno/agent/_default_tools.py` | 定义 `update_user_memory` tool |
| `libs/agno/agno/agent/_messages.py` | system message 中指导何时调用 memory tool |
| `libs/agno/agno/memory/manager.py` | memory add/delete/create/search/update task 的核心管理器 |
| `libs/agno/agno/session/summary.py` | session summary prompt 和结构化摘要 |
| `libs/agno/agno/knowledge/chunking/markdown.py` | Markdown chunking 作为 knowledge ingestion |
| `libs/agno/agno/os/routers/agents/schema.py` | API schema 中 `enable_agentic_memory`、`update_memory_on_run` 等默认关闭 |

## 架构层次

Agno 典型 agent 由以下能力组合：

- model；
- tools；
- storage；
- memory；
- session summary；
- knowledge base；
- markdown output rendering；
- OS/API routers。

Memory 是一个可选 capability。开发者通过参数启用：

- `enable_user_memories`
- `enable_session_summaries`
- `enable_agentic_memory`
- `update_memory_on_run`
- `add_history_to_messages`

## 记忆模式

Agno 有两类主要记忆：

1. **User memories**：用户偏好、持久个人信息、可由 agentic tool 更新。
2. **Session summaries**：对 session history 的摘要，用于跨轮或跨 session 压缩上下文。

当启用 agentic memory 时，Agno 会把 memory update tool 加给 agent，让模型决定写入/更新/删除用户 memory。

## Markdown 用法

Agno 中 Markdown 不是核心行为控制层，主要用于：

- response rendering；
- knowledge reader；
- markdown chunking；
- docs/source ingestion；
- UI/API 输出格式。

这与 Mnemon 目标不同：Mnemon 希望 Markdown 同时承担 install contract、skill、guideline 和 reviewed evolution artifact。

## 对 Mnemon 的启发

可参考：

- memory flags 默认关闭；
- agentic memory tool 明确暴露；
- session summary 与 user memory 分离；
- Markdown chunking 用于知识库 ingestion。

不适合作为第一阶段模板：

- memory 由 framework 参数和 Python object 控制；
- 缺少通用 `INSTALL.md`/`GUIDELINE.md` 风格行为契约；
- 自进化更多依赖开发者工程集成，而非 agent 自己读 Markdown 安装。

## 参考来源

- 本地源码: `libs/agno/agno/agent/_init.py`
- 本地源码: `libs/agno/agno/agent/_default_tools.py`
- 本地源码: `libs/agno/agno/memory/manager.py`
- 本地源码: `libs/agno/agno/session/summary.py`
- 本地源码: `libs/agno/agno/knowledge/chunking/markdown.py`
- 官方文档: [Agno Memory](https://docs-v1.agno.com/agents/memory)
- 官方文档: [Agno Agent reference](https://docs.agno.com/reference/agents/agent)
