# Letta 概览

## 一句话结论

Letta 是 MemGPT 路线的结构化 agent memory runtime。它把 memory 分成 in-context core memory、out-of-context archival memory、recall/conversation memory，并通过 tools/API 让 agent 自我编辑 memory。它是强 memory runtime，不是轻量 Markdown harness。

## 关键源码证据

本地源码：`/tmp/mnemon-agent-research-sources/letta`, HEAD `bb52a8900a79cf1378e6e9cdecf244b673a13a72`

| 位置 | 观察 |
|---|---|
| `README.md` | 创建 agent 时可传 `memory_blocks` |
| `letta/schemas/memory.py` | `Memory.compile()`、`BasicBlockMemory` 等 memory block model |
| `letta/functions/function_sets/base.py` | `archival_memory_insert/search`、`core_memory_append/replace`、`memory_insert/replace` |
| `letta/prompts/system_prompts/memgpt_chat.py` | core/recall/archival memory system prompt |
| `letta/prompts/prompt_generator.py` | 注入 memory metadata：previous messages、archival size、tags |
| `letta/server/rest_api/proxy_helpers.py` | `<memory_blocks>` 格式化并注入 proxy context |
| `letta/server/rest_api/routers/v1/agents.py` | core-memory 与 archival-memory API endpoints |
| `letta/services/memory_repo/` | block markdown/git 表示 |

## 架构层次

Letta 的 memory 不是旁路工具，而是 agent state 的核心：

```text
agent state
  -> core memory blocks
  -> prompt compilation
  -> tool-call memory edits
  -> archival passages
  -> recall/conversation search
  -> REST API / server managers
```

## Memory hierarchy

MemGPT/Letta 的关键抽象：

| 层 | 位置 | 用途 |
|---|---|---|
| Core memory | in-context blocks | 人格、用户事实、当前任务核心状态，可编辑 |
| Archival memory | out-of-context storage | 长期资料、反思、较大知识，通过 search/insert tools 访问 |
| Recall memory | conversation history | 过去交互，可通过 conversation search 检索 |

系统 prompt 明确告诉 agent：core memory 可用 `core_memory_append` / `core_memory_replace` 编辑；archival memory 无限但不在当前 context，需要显式 search。

## Tool/API 设计

Letta 暴露的关键工具：

- `core_memory_append`
- `core_memory_replace`
- `memory_insert`
- `memory_replace`
- `archival_memory_insert`
- `archival_memory_search`
- `conversation_search`

REST API 也提供 core-memory blocks 和 archival-memory 的 list/insert/search/update。

## 对 Mnemon 的启发

可参考：

- memory hierarchy 清晰；
- core vs archival 的 context budget 思想；
- agent 自编辑 memory 需要精确工具；
- memory metadata 可进入 prompt，具体内容按需 search。

不适合作为当前模板：

- Letta 是完整 runtime；
- memory schema 与 server 深度耦合；
- Markdown 不是主要行为安装协议；
- 自进化主要是 memory blocks 自编辑，不是 Markdown skill/guideline 演化。

## 参考来源

- 本地源码: `letta/prompts/system_prompts/memgpt_chat.py`
- 本地源码: `letta/functions/function_sets/base.py`
- 本地源码: `letta/prompts/prompt_generator.py`
- 本地源码: `letta/server/rest_api/routers/v1/agents.py`
- 官方文档: [Letta stateful agents](https://docs.letta.com/guides/core-concepts/stateful-agents)
- 官方文档: [Letta memory blocks](https://docs.letta.com/guides/core-concepts/memory/memory-blocks)
- 官方文档: [Letta archival memory](https://docs.letta.com/guides/core-concepts/memory/archival-memory)
- 论文: [MemGPT](https://arxiv.org/abs/2310.08560)
