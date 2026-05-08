# Agno 的记忆、Markdown 与 Prompt 用法

## 一句话结论

Agno memory 的核心是 framework-managed：开发者通过 flags 决定写路径与读路径，prompt 模板与 tool schema 都由 framework 拼接，Markdown 只承担 knowledge ingestion 这一面，不参与行为契约。

## 源码地图

| 关注点 | 文件:行 | 观察 |
|---|---|---|
| `<memories_from_previous_interactions>` 注入 | `libs/agno/agno/agent/_messages.py:299-302` | 列表化展开所有 user memory，并提示「当前对话优先于过去 memory」 |
| 当前对话优先提示 | `libs/agno/agno/agent/_messages.py:303-306` | 显式写入 `You should always prefer information from this conversation over the past memories.` |
| `<updating_user_memories>` 注入 | `libs/agno/agno/agent/_messages.py:315-325` | `enable_agentic_memory=True` 时把 `update_user_memory` 工具说明写入 system prompt |
| update_user_memory tool | `libs/agno/agno/agent/_default_tools.py:38-75` | 把自然语言 task 转交给 `MemoryManager.update_memory_task` |
| MemoryManager 系统提示 | `libs/agno/agno/memory/manager.py:980-1038` | 第三人称写入规则、避免重复、用户撤回信息时的处理 |
| 默认 memory 抓取规则 | `libs/agno/agno/memory/manager.py:969-978` | personal facts / opinions / life events / context 四类 |
| MemoryTools 工具集 | `libs/agno/agno/tools/memory.py:13-65` | 显式版 think / get_memories / add_memory / update_memory / delete_memory / analyze |
| MemoryTools.think | `libs/agno/agno/tools/memory.py:66-95` | 把 chain-of-thought 写入 `session_state["memory_thoughts"]` |
| Session summary 系统提示 | `libs/agno/agno/session/summary.py:104-149` | 默认提示要求生成 `summary` + `topics` |
| Session summary 默认请求 | `libs/agno/agno/session/summary.py:72` | `summary_request_message = "Provide the summary of the conversation."` |
| MarkdownChunking | `libs/agno/agno/knowledge/chunking/markdown.py:29` | `chunk_size=5000`、`overlap=0`、`split_on_headings=False` |
| MarkdownReader | `libs/agno/agno/knowledge/reader/markdown_reader.py:23` | 把 `.md`/`.markdown` 转成 `Document` 输入 chunker |

## 记忆处理方案

Agno memory 的核心是 framework-managed：

```text
Agent config flags
  -> set_memory_manager (_init.py:99)
  -> MemoryManager (memory/manager.py:45)
  -> existing user memories inserted into prompt (_messages.py:286-326)
  -> optional update_user_memory tool (_default_tools.py:38)
  -> MemoryTools (tools/memory.py:13) for explicit operations
  -> SessionSummaryManager (session/summary.py:62)
  -> storage backend (BaseDb / AsyncBaseDb)
```

源码中的 prompt 拼装 (`_messages.py:286-326`) 显示：

- `add_memories_to_context=True` 时，所有 user memory 以 `<memories_from_previous_interactions>` 段落形式插入；
- 之后立刻附一句「always prefer information from this conversation over the past memories」，是 framework 写死的 guardrail；
- `enable_agentic_memory=True` 时再追加 `<updating_user_memories>` 段，向模型解释 `update_user_memory` 工具的语义；
- 自动后台写入路径不在 prompt 中体现，模型对其无感知。

## Agentic memory tool

`update_user_memory(task)` 是 agentic 路径的关键工具：

- agent 可根据对话历史创建/更新/删除/清空 memory；
- prompt 指导 agent 保存 observations、preferences、context（`_messages.py:320`）；
- tool 层把自然语言 task 交给 `MemoryManager.update_memory_task`（`manager.py:481`）；
- `update_memory_task` 内部还会把 `add_memory` / `update_memory` / `delete_memory` / `clear_memory` 子工具组合给 LLM 选择（`manager.py:1013-1020`），是「先用大 task 描述意图，再让模型自己分发」的两层结构。

与之并列的还有 `MemoryTools`（`tools/memory.py:13`）这一更显式的工具集：暴露 `think` / `get_memories` / `add_memory` / `update_memory` / `delete_memory` / `analyze`，把 chain-of-thought 显式写到 `session_state["memory_thoughts"]`（`tools/memory.py:81-83`），让 memory 操作过程也可审计。

这与 Mnemon 的 `remember` 有相似点，但 Agno 同时提供「task 透传」和「显式工具」两条路径，Mnemon 当前 `remember` 偏向后者：直接产生 candidate，再由 review/install 决定是否落盘。

## Session summary prompt

`session/summary.py:62` 维护 `SessionSummaryManager`，其默认行为：

- `last_n_runs` 与 `conversation_limit` 决定切片范围，未设置则全量（`summary.py:78-87`）；
- 默认 prompt 要求模型返回结构化的 `summary` + `topics`（`summary.py:112-117`）；
- 支持 native structured output / json schema / json object 三种 fallback（`summary.py:89-102`）；
- summary 与 user memory 走不同 manager、不同存储字段（`AgentSession.summary` vs `db.user_memories`），互不污染。

Mnemon 可借鉴这一点：Compact phase 应保存关键连续性，不应机械保存完整 transcript，且与 durable memory 隔离。

## Markdown 用法

Agno 的 Markdown 用途偏数据处理：

- `MarkdownReader`（`knowledge/reader/markdown_reader.py:23`）读取 `.md`/`.markdown`；
- `MarkdownChunking`（`chunking/markdown.py:16`）按 heading/paragraph 分块，默认 `chunk_size=5000` chars、`overlap=0`、`split_on_headings=False`；
- chunk 内部再走 `unstructured` 库 `chunk_by_title` 与 `partition_md`（`chunking/markdown.py:199-210`）；
- `markdown=True` 时给 system prompt 加 markdown 输出指令（`agent.py:244`）；
- API schema 有 markdown output flag 控制 UI 展示。

这说明 Agno 不把 Markdown 作为 agent 自我安装和自我演化的主要协议。它的 `.md` 是输入语料，不是 install contract。

## Knowledge Markdown chunking 细节

5000 字符的默认值在多个 chunker 共享：

- `MarkdownChunking.__init__`（`chunking/markdown.py:29`）：`chunk_size: int = 5000`；
- `DocumentChunking`（`chunking/document.py:10`）：同 5000；
- `RecursiveChunking`（`chunking/recursive.py:11`）：同 5000；
- `FixedSizeChunking`（`chunking/fixed.py:10`）：同 5000；
- `AgenticChunking.MAX_CHUNK_SIZE`（`chunking/agentic.py:11`）：上限 5000，使用 LLM 找自然断点。

`MarkdownChunking.chunk` 流程（`chunking/markdown.py:238-327`）：

1. 内容长度 ≤ chunk_size 且未启用 heading 分割时直接返回单 chunk；
2. 否则进 `_partition_markdown_content`：若 `split_on_headings` 启用，走自写正则；否则调用 `unstructured.partition_md` 与 `chunk_by_title`，参数 `max_characters=chunk_size`、`new_after_n_chars=chunk_size*0.8`、`combine_text_under_n_chars=chunk_size`、`overlap=0`；
3. 大节点用 `_split_large_section`（`chunking/markdown.py:40`）按段落、再按句子、再按词强制切；
4. `overlap > 0` 时把前 chunk 末尾 `overlap` 字符前置到下一 chunk（`chunking/markdown.py:301-326`）。

embedding pipeline 的位置：chunk 产出 `Document` 后，由 `Knowledge.upsert/insert` 流水线送到 vectordb（`knowledge/knowledge.py:2453, 2466, 2492, 2505` 处理 `Could not upsert/insert embedding` 错误分支）。embedder 是 knowledge 配置的独立组件，不和 user memory 共用。

## 智能体演化方案

Agno 没有像 Hermes 那样把「成功 workflow → skill」作为内置闭环。它的演化路径更像：

- memory manager 根据对话更新 user memory（`_managers.py:29` 与 `manager.py:368`）；
- session summary 压缩上下文（`summary.py:227`）；
- knowledge base 通过外部数据更新（开发者显式 ingest）；
- `optimize_memories` 显式合并（`manager.py:793`）；
- developer 修改 agent code/config 进化 agent 自身。

`SchedulerTools`（`tools/scheduler.py:29`）提供给 agent 创建 cron 调度的能力，但它是通用调度，不是 memory 专用。它依赖 AgentOS server 与 SchedulePoller，因此对单机 CLI 这类场景成本较高。

所以 Agno 对 Mnemon 的启发更偏「memory capability API」，不是「memory-driven self-evolving framework」。

## 完整 prompt 示例

来自 `_messages.py:286-326` 的实际拼接，在 `add_memories_to_context=True` 且 `enable_agentic_memory=True` 时，system message 会包含类似：

```text
You have access to user info and preferences from previous interactions
that you can use to personalize your response:

<memories_from_previous_interactions>
- John Doe's name is John Doe.
- John Doe goes to the gym regularly.
- John Doe prefers Python over Go.
</memories_from_previous_interactions>

Note: this information is from previous interactions and may be updated
in this conversation. You should always prefer information from this
conversation over the past memories.

<updating_user_memories>
- You have access to the `update_user_memory` tool that you can use to
  add new memories, update existing memories, delete memories, or clear
  all memories.
- If the user's message includes information that should be captured as
  a memory, use the `update_user_memory` tool to update your memory
  database.
- Memories should include details that could personalize ongoing
  interactions with the user.
- Use this tool to add new memories or update existing memories that you
  identify in the conversation.
- Use this tool if the user asks to update their memory, delete a
  memory, or clear all memories.
- If you use the `update_user_memory` tool, remember to pass on the
  response to the user.
</updating_user_memories>
```

如果 memory 为空，会改成（`_messages.py:308-311`）：

```text
You have the capability to retain memories from previous interactions
with the user, but have not had any interactions with the user yet.
```

这种「占位」语句对模型行为可预测性很重要：模型不会因为找不到 memory 而幻觉一个用户偏好。

## MemoryManager 系统提示节选

`manager.py:980-1038` 拼接的提示在写入阶段会变成：

```text
You are a Memory Manager that is responsible for managing information
and preferences about the user. You will be provided with a criteria
for memories to capture in the <memories_to_capture> section and a list
of existing memories in the <existing_memories> section.

## When to add or update memories
- Your first task is to decide if a memory needs to be added, updated,
  or deleted based on the user's message OR if no changes are needed.
- If the user's message meets the criteria in the <memories_to_capture>
  section and that information is not already captured in the
  <existing_memories> section, you should capture it as a memory.
...

## How to add or update memories
- If you decide to add a new memory, create memories that captures key
  information, as if you were storing it for future reference.
- Memories should be a brief, third-person statements...
  - Example: If the user's message is 'I'm going to the gym', a memory
    could be `John Doe goes to the gym regularly`.
...

<memories_to_capture>
Memories should capture personal information about the user that is
relevant to the current conversation, such as:
- Personal facts: name, age, occupation, location, interests, and
  preferences
- Opinions and preferences: what the user likes, dislikes, enjoys, or
  finds frustrating
- Significant life events or experiences shared by the user
- Important context about the user's current situation, challenges, or
  goals
- Any other details that offer meaningful insight into the user's
  personality, perspective, or needs
</memories_to_capture>

## Updating memories
You will also be provided with a list of existing memories in the
<existing_memories> section. You can:
  - Decide to make no changes.
  - Decide to add a new memory, using the `add_memory` tool.
  - Decide to update an existing memory, using the `update_memory` tool.
  - Decide to delete an existing memory, using the `delete_memory` tool.
```

注意 `clear_memory` 在 `create_or_update_memories` 的提示中是 `enable_clear_memory=False`（`manager.py:1075`）传入，所以自动写入路径不会清空所有 memory；`update_memory_task`（agentic 路径）才会传 `clear_memories=self.clear_memories` 透传开发者设置。

## Prompt-level guardrail 借鉴

Agno 在 prompt 拼装上有几个值得借鉴的细节：

1. **当前对话优先**：`_messages.py:303-306` 明确写「always prefer information from this conversation over the past memories」，避免历史 memory 覆盖当前事实。
2. **空 memory 时的占位语**：`_messages.py:308-311` 在没有 memory 时也会告诉模型「我有 memory 能力但还没积累」，让模型行为可预测。
3. **第三人称写入规范**：`manager.py:992-995` 提供示例「If the user's message is 'I'm going to the gym', a memory could be 'John Doe goes to the gym regularly'」，把存储格式与对话格式解耦。
4. **避免重复与遗忘标记**：`manager.py:997-998` 要求模型用「更新」而不是「重写」，并且用户要求遗忘时不要写「The user used to like ...」。

这些都是 Mnemon 设计 candidate prompt 时可以直接借鉴的措辞。

## 对 Mnemon 的设计判断

Agno 强化了几个 guardrail：

- memory feature 应可开关（`agent.py:120,122` 默认全 False）；
- 当前对话和当前事实应优先于过去 memory（`_messages.py:303-306`）；
- session summary 与 durable memory 要分层（不同 manager、不同存储）；
- markdown ingestion 和 markdown behavior contract 是两回事，不要混；
- 写入路径要么 framework 自动、要么 agent 主动，不要并行（`_managers.py:172`）；
- 整理是显式 API（`manager.py:793`），不是 cron 副作用。

## 参考来源

- 本地源码: `libs/agno/agno/agent/_messages.py`
- 本地源码: `libs/agno/agno/agent/_default_tools.py`
- 本地源码: `libs/agno/agno/agent/_managers.py`
- 本地源码: `libs/agno/agno/memory/manager.py`
- 本地源码: `libs/agno/agno/memory/strategies/summarize.py`
- 本地源码: `libs/agno/agno/session/summary.py`
- 本地源码: `libs/agno/agno/knowledge/reader/markdown_reader.py`
- 本地源码: `libs/agno/agno/knowledge/chunking/markdown.py`
- 本地源码: `libs/agno/agno/knowledge/chunking/agentic.py`
- 本地源码: `libs/agno/agno/tools/memory.py`
- 本地源码: `libs/agno/agno/tools/scheduler.py`
- 官方文档: [Agno Memory](https://docs-v1.agno.com/agents/memory)
- 官方文档: [Agno Working with Memories](https://docs.agno.com/memory/working-with-memories/overview)
