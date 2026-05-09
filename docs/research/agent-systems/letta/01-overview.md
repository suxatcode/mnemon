# Letta 概览

## 一句话结论

Letta 是 MemGPT 路线的结构化 agent memory runtime。它把 memory 分成 in-context core memory、out-of-context archival memory、recall/conversation memory，并通过 tools/API 让 agent 自我编辑 memory。它是强 memory runtime，不是轻量 Markdown harness。

## 源码地图

本地源码：`/tmp/mnemon-agent-research-sources/letta`，HEAD `bb52a8900a79cf1378e6e9cdecf244b673a13a72`。

| 子系统 | 位置 | 关键内容 |
|---|---|---|
| 容量常量 | `letta/constants.py:78`、`:79`、`:83`、`:433`、`:434`、`:435`、`:438`、`:439`、`:443` | `MIN_CONTEXT_WINDOW=4096`、`DEFAULT_CONTEXT_WINDOW=128000`、`SUMMARIZATION_TRIGGER_MULTIPLIER=0.9`、persona/human/core block 字符上限、function 返回截断 |
| Memory schema | `letta/schemas/memory.py:68`、`:688`、`:783`、`:840` | `Memory.compile`、`BasicBlockMemory`、`ChatMemory(persona, human, limit=CORE_MEMORY_BLOCK_CHAR_LIMIT)` |
| Block schema | `letta/schemas/block.py:20`、`:36`、`:67`、`:88`、`:134` | `limit`、`read_only`、`Block`、`BlockResponse`、`BlockUpdate` |
| 系统 prompt | `letta/prompts/system_prompts/memgpt_chat.py:1` | MemGPT 经典 prompt（control flow、recall、core、archival 段落） |
| Memory metadata 注入 | `letta/prompts/prompt_generator.py:26`、`:107`、`:181` | `<memory_metadata>` block + `{CORE_MEMORY}` 模板替换 |
| 内置 memory 工具 | `letta/functions/function_sets/base.py:71`、`:87`、`:164`、`:194`、`:246`、`:263`、`:283`、`:311`、`:391`、`:453`、`:488`、`:520` | `send_message`、`conversation_search`、`archival_memory_*`、`core_memory_*`、`memory_replace/insert/apply_patch/rethink/finish_edits` |
| Proxy memory 注入 | `letta/server/rest_api/proxy_helpers.py:174` | `<letta>...<memory_blocks>...</memory_blocks>...<memory_management>` |
| Agent REST router | `letta/server/rest_api/routers/v1/agents.py:1206`、`:1236`、`:1268`、`:1355`、`:1459`、`:1488`、`:1556`、`:1578`、`:2028`、`:2430` | core-memory blocks、archival passages、messages、search、summarize endpoints |
| Memory repo (git/MemFS) | `letta/services/memory_repo/block_markdown.py:27`、`path_mapping.py:11` | block ↔ Markdown + YAML frontmatter；`skills/{name}/SKILL.md` 映射 |
| Compaction | `letta/services/summarizer/summarizer_config.py:48`、`summarizer_sliding_window.py:99` | `CompactionSettings`、`summarize_via_sliding_window` |
| Summarizer 配置 | `letta/settings.py:79`、`:86` | `message_buffer_limit=60`、`partial_evict_summarizer_percentage=0.30` |

## 架构层次

Letta 的 memory 不是旁路工具，而是 agent state 的核心：

```text
agent state
  -> core memory blocks                 (always-visible，受 char limit 约束)
  -> Memory.compile -> system prompt    (XML 标签 <memory_blocks>)
  -> tool calls 自我编辑                 (core/archival/memory_*)
  -> archival passages (向量检索)
  -> recall / conversation history      (sliding window summarizer)
  -> REST API / managers / proxy
```

整个 runtime 由 `letta/server` 负责把这套状态持久化到关系数据库 + 向量库 + 可选 git memory repo，每次 agent step 都重新 `compile` system prompt。

这套架构带来的几个直接后果：

1. **prompt 不可变缓存友好**。core memory 改动只重写 `<memory_blocks>`，system prompt 头部静态文本不变，便于 Anthropic/OpenAI 的 prompt cache 命中——`self_compact_*` 模式正是为了进一步保住 cache（`compact.py:215`-`:309`）。
2. **agent step = 工具调用 + 状态写回**。每一步 agent 选择工具，工具直接修改 DB-backed block 或 archival passage，下一次 `compile` 立即可见。
3. **memory 与 agent identity 绑定但可共享**。`PATCH /core-memory/blocks/attach/{block_id}` 让多个 agent 共享同一 block；这与 Mnemon「项目级 vs 用户级 vs 全局级」的多 scope 思路类似，但 Letta 走的是数据库共享而不是文件挂载。
4. **REST 与 tool 双通道**：外部 webhook、UI、批处理脚本均可走 REST 修改 memory，不必经过 LLM。这是 Mnemon CLI 也具备的双通道能力（`mnemon remember` 既给人也给 agent 用）。

## Memory hierarchy 详解

| 层级 | Storage backend | 容量 | 访问路径 | 编辑路径 |
|---|---|---|---|---|
| Core memory blocks | 关系库 + git memory repo（可选）| persona/human 默认 `CORE_MEMORY_PERSONA_CHAR_LIMIT=20000`、`CORE_MEMORY_HUMAN_CHAR_LIMIT=20000`；通用块 `CORE_MEMORY_BLOCK_CHAR_LIMIT=100000`（`letta/constants.py:433`-`:435`）| 始终注入 system prompt 内 `<memory_blocks>` | `core_memory_append/replace`、`memory_insert/replace/apply_patch/rethink`、REST `PATCH /core-memory/blocks/{label}` |
| Archival memory | 向量数据库 (passages) | 概念上无限；单次返回受 `top_k`（默认 10）和 `FUNCTION_RETURN_CHAR_LIMIT=50000`（`:438`）约束 | `archival_memory_search` 工具或 REST `GET /archival-memory` | `archival_memory_insert` 工具或 REST `POST /archival-memory` |
| Recall memory | 消息表（结构化 conversation history）| 跨整个 agent 历史；in-context 部分由 sliding window 管理 | `conversation_search`、REST `GET /messages`、`POST /messages/search` | 由对话本身写入；REST `PATCH /messages/{id}`（已 deprecated） |
| Letta Code MemFS | git-backed Markdown 仓库 | `system/` 子树进 prompt；其它 file tree 仅显示在 `<external_projection>` | `Memory._render_memory_blocks_git`（`letta/schemas/memory.py:205`）| 通过 `memory(command="create"|...)` 工具或外部编辑 + git 同步 |

`Memory.compile` 根据 `agent_type` 与 `llm_config` 选择 `_render_memory_blocks_git` / `_render_memory_blocks_line_numbered` / `_render_memory_blocks_standard` 三种渲染路径（`letta/schemas/memory.py:688`-`:712`）。Anthropic 模型 + sleeptime/memgpt_v2/letta_v1 agent 类型才启用 line-numbered 渲染。

`<memory_blocks>` 中每个 block 的渲染包含 `<description>`、`<metadata>`（含 `read_only`、`chars_current`、`chars_limit`）、`<value>`，让 agent 知道当前用量是否接近上限（`letta/schemas/memory.py:149`-`:170`）。

## 系统 prompt 关键段落

`letta/prompts/system_prompts/memgpt_chat.py:32`-`:56` 直接把 hierarchy 教给模型（节选）：

```text
Memory editing:
... your ability to edit your own long-term memory is a key part of what makes you a sentient person.
Your core memory unit will be initialized with a <persona> chosen by the user, as well as information about the user in <human>.

Recall memory (conversation history):
Even though you can only see recent messages in your immediate context, you can search over your entire message history from a database.
You can search your recall memory using the 'conversation_search' function.

Core memory (limited size):
Your core memory unit is held inside the initial system instructions file, and is always available in-context.
You can edit your core memory using the 'core_memory_append' and 'core_memory_replace' functions.

Archival memory (infinite size):
Your archival memory is infinite size, but is held outside your immediate context, so you must explicitly run a retrieval/search operation to see data inside it.
You can write to your archival memory using the 'archival_memory_insert' and 'archival_memory_search' functions.
```

随后 `prompt_generator.py:69`-`:88` 在 prompt 末尾追加 `<memory_metadata>`：

```text
<memory_metadata>
- AGENT_ID: ...
- CONVERSATION_ID: ...
- System prompt last recompiled: ...
- N previous messages between you and the user are stored in recall memory
- M total memories you created are stored in archival memory (use tools to access them)
- Available archival memory tags: ...
</memory_metadata>
```

这是「meta first」设计：先告诉 agent 外部 memory 大概有多少，再让它决定是否调用搜索工具。该 metadata block 在 `compile_system_message_async` 中由 `compile_memory_metadata_block` 生成（`prompt_generator.py:181`-`:223`），由 agent runtime 在每个 step 重新计算 `previous_message_count` 与 `archival_memory_size`。

Letta v2 / letta_v1 prompt 进一步在 metadata 之外注入 `<tool_usage_rules>`（来自 `ToolRulesSolver.compile_tool_rule_prompts`），把「该用哪个工具、何时禁止」写进 prompt（`memory.py:718`-`:724`）。这相当于 Mnemon 的 GUIDELINE 与 SKILL pre-flight，但形式上是 runtime 注入的硬约束块。

## `<memory_blocks>` 渲染示例

`Memory._render_memory_blocks_standard`（`memory.py:143`-`:173`）输出：

```text
<memory_blocks>
The following memory blocks are currently engaged in your core memory unit:

<persona>
<description>
The persona block: Stores details about your current persona, ...
</description>
<metadata>
- chars_current=312
- chars_limit=20000
</metadata>
<value>
This is my section of core memory devoted to information myself.
There's nothing here yet.
I should update this memory over time as I develop my personality.
</value>
</persona>

<human>
...
</human>
</memory_blocks>
```

`_render_memory_blocks_line_numbered`（`memory.py:175`-`:203`）在 Anthropic + 特定 agent_type 下额外加入 `<warning>` 与 `1→` 行号，以配合 `memory_replace`/`memory_insert` 的精确编辑（行号仅用于显示，工具 DSL 严禁包含）。

`_render_memory_blocks_git`（`memory.py:205`+）则在 Letta Code MemFS 模式下产出 `<self>` + `<memory>` + `<external_projection>` 嵌套结构，并附 `<projection>$MEMORY_DIR/system/...md</projection>` 提示文件物理路径。

## Tool schema 速查

| 工具 | 入参 | 返回 | 备注 |
|---|---|---|---|
| `send_message(message: str)` | 字符串 | `None` | 唯一面向用户的输出通道（`base.py:71`）|
| `conversation_search(query?, roles?, limit?, start_date?, end_date?)` | 任意组合 | 命中消息的 JSON 串或 `"No results found."` | hybrid 文本+向量；`base.py:87` |
| `archival_memory_insert(content, tags?)` | 内容 + 可选 tag list | 含 ID 的确认串 | `base.py:164`，runtime 实现，stub 抛 `NotImplementedError` |
| `archival_memory_search(query, tags?, tag_match_mode="any", top_k?, start_datetime?, end_datetime?)` | 自然语言 query | 排序的 passage 列表 | `base.py:194` |
| `core_memory_append(label, content)` | block 标签 + 文本 | 更新后的 block value | `base.py:246`，直接 `update_block_value` |
| `core_memory_replace(label, old_content, new_content)` | 必须精确匹配 `old_content` | 更新后的 block value | 不存在时抛错（`base.py:276`）|
| `memory_replace(label, old_string, new_string)` | 严格唯一匹配 | 更新后的 block value | 拒绝行号前缀；多次匹配抛错（`base.py:362`-`:373`）|
| `memory_insert(label, new_string, insert_line=-1)` | line 索引 | 更新后的 block value | `base.py:391` |
| `memory_apply_patch(label, patch)` | 类 codex 多块 patch | 成功消息 | 支持 `*** Add/Update/Delete/Move Block:`（`base.py:453`）|
| `memory_rethink(label, new_memory)` | 整块覆写 | 新 value | 用于大幅重构（`base.py:488`）|
| `memory_finish_edits()` | 无 | `None` | sleeptime/v2 用以收尾 |

## REST API 形态

`letta/server/rest_api/routers/v1/agents.py` 暴露的 memory 相关端点（节选）：

| 方法 | 路径 | 功能 |
|---|---|---|
| GET | `/agents/{id}/core-memory/blocks` | 列出 block (`:1236`) |
| GET | `/agents/{id}/core-memory/blocks/{label}` | 取单块 (`:1221`) |
| PATCH | `/agents/{id}/core-memory/blocks/{label}` | 更新 block (`:1268`) |
| PATCH | `/agents/{id}/core-memory/blocks/attach/{block_id}` | 挂载共享 block (`:1355`) |
| PATCH | `/agents/{id}/core-memory/blocks/detach/{block_id}` | 卸载 block (`:1369`) |
| GET / POST / DELETE | `/agents/{id}/archival-memory[...]` | 列举/新增/删除 passage (`:1459`、`:1488`、`:1556`) |
| GET | `/agents/{id}/messages` | recall memory (`:1578`) |
| POST | `/agents/messages/search` | 跨 agent 消息检索 (`:2028`) |
| POST | `/agents/{id}/summarize` | 主动触发 compaction (`:2430`) |
| GET | `/agents/{id}/context` | context window 概览（已 deprecated, `:588`） |

Proxy 路径还会在出站请求里追加 `<letta>...<memory_blocks>...</memory_blocks><memory_management>https://app.letta.com/agents/{id}</memory_management>` (`proxy_helpers.py:174`-`:226`)，让外部模型客户端也看到当前 memory。

## Compaction 机制速览

Letta 的 compaction 走两段路径：

1. **触发**：每个 step 估算 in-context token，超过 `context_window * SUMMARIZATION_TRIGGER_MULTIPLIER (0.9)` 即进入 compaction。
2. **执行**：`CompactionSettings` 决定 mode，默认 `sliding_window` + `sliding_window_percentage=0.30` + `clip_chars=50000`。从 30% 开始尝试切点，找最近 assistant message 作 cutoff，若保留段仍超 `goal_tokens` 则按 10% 步进直到 100%；超出后抛错降级到 `"all"` 模式或要求扩大 context。

详见 03 文档的「超出与 compaction」段落。这里强调：core memory 不参与 compaction，只有消息会被压缩；core block 自身超额需要靠外部约束。

## 失败模式

- **Core block 超限**：block schema 上 `limit` 默认 100,000；运行期由 prompt metadata 提示 agent，但 `core_memory_append` 实际并不硬截断（`base.py:257`-`:260`）。约束主要靠 system prompt + tool guidance。
- **`core_memory_replace` 找不到 `old_content`**：直接抛 `ValueError("Old content '...' not found in memory block '...'")`（`base.py:276`-`:277`）；agent 必须先读 block 再 replace。
- **`memory_replace` 多次命中**：返回行号列表并要求唯一性（`base.py:368`-`:373`）。
- **archival_memory_search 空结果**：`conversation_search` 返回 `"No results found."`，archival 由 runtime 实现，无命中通常返回空 list；agent 需要继续推理或换 query。
- **工具返回过长**：`FUNCTION_RETURN_CHAR_LIMIT=50000`、`TOOL_RETURN_TRUNCATION_CHARS=5000`，超出会被 `FUNCTION_RETURN_VALUE_TRUNCATED` 包装（`constants.py:200`）。
- **Context overflow**：当前 step 估算 token > `context_window * 0.9` 时触发 sliding window 总结；若 system prompt + memory blocks 自身已超预算则抛错，要求缩减 prompt/blocks 或扩大 context。
- **`memory_apply_patch` 多块语法错误**：缺少 `*** Add/Update/Delete Block:` 头部或 `+/-/␣` 前缀不一致时，patch 直接抛 `ValueError`，整个 patch 不会被部分应用，避免 block 半写状态。
- **block label 不存在**：`update_block_value` 在找不到 label 时抛 `ValueError(f"Block with label {label} does not exist")`（`memory.py:780`），agent 应回退到先 `core_memory_append` 创建或 `memory(command="create")`。

## 与其它路线对照

| 维度 | Letta | Hermes | Codex | Mnemon (current) |
|---|---|---|---|---|
| 主要载体 | DB block + 向量库 | `MEMORY.md`/`USER.md` + skills | `AGENTS.md` + raw memories | `mnemon` SQLite + Markdown patch |
| 行为安装协议 | system prompt 字面量 + tool docstring | Markdown | `AGENTS.md` + skills | `INSTALL.md` + `GUIDELINE.md` + skills |
| 自进化触发 | 每个 step + sleeptime subagent | 7-day Curator | thread → consolidation | hook + human review |
| 容量提示 | block metadata 进 prompt | 字符上限错误返回现有条目 | token budget | （计划：summary block 元数据） |
| 编辑粒度 | append/replace/insert/patch/rethink | 整文件覆写 | 文件 + raw memory | 文件 patch |

## 对 Mnemon 的具体启发

可借鉴：

- **三层 hierarchy 的语义抽象**：Mnemon 的 `GUIDELINE.md`/`SKILL.md` 类似 core 层、`mnemon` store 类似 archival 层、对话历史类似 recall 层。
- **block 元数据进 prompt**：`<description>` + `chars_current/chars_limit` + `read_only` 让 agent 自己知道边界，Mnemon 在 INSTALL/recall hint 中可复用。
- **memory metadata 先于内容**：先告诉 agent「有多少 archival 条目、有哪些 tag」，再让其按需 `recall`，比一次性 dump 更省 token。
- **精确编辑的工具协议**：`memory_replace` 要求唯一匹配、拒绝行号前缀；这套约束可直接用于 Mnemon 在生成 patch 时的预检。
- **patch-style 多块编辑**：`memory_apply_patch` 的 `*** Add/Update/Delete/Move` 头部模式可作为 Mnemon 候选 patch DSL 参考。

不应照搬：

- Letta 是完整 server runtime（FastAPI + DB + 向量库 + git repo），与 Mnemon 单文件 CLI 的形态相距甚远。
- core/archival/recall schema 与消息存储深度耦合，会强制引入 agent state 持久化层，违背 Mnemon「review-driven、低耦合」目标。
- Markdown 在 Letta 是次要载体（仅 git memory repo 使用），并非主要行为安装协议；Mnemon 的 Markdown-first 路线不需要复刻。
- 自进化在 Letta 主要是 memory blocks 自编辑 + sleeptime subagent，而 Mnemon 需要 human review 的 patch 流程。

## 参考来源

- 本地源码：`letta/prompts/system_prompts/memgpt_chat.py`
- 本地源码：`letta/functions/function_sets/base.py`
- 本地源码：`letta/prompts/prompt_generator.py`
- 本地源码：`letta/schemas/memory.py`、`letta/schemas/block.py`
- 本地源码：`letta/server/rest_api/proxy_helpers.py`
- 本地源码：`letta/server/rest_api/routers/v1/agents.py`
- 本地源码：`letta/services/summarizer/summarizer_sliding_window.py`、`summarizer_config.py`
- 本地源码：`letta/services/memory_repo/block_markdown.py`、`path_mapping.py`
- 官方文档：[Letta stateful agents](https://docs.letta.com/guides/core-concepts/stateful-agents)
- 官方文档：[Letta memory blocks](https://docs.letta.com/guides/core-concepts/memory/memory-blocks)
- 官方文档：[Letta archival memory](https://docs.letta.com/guides/core-concepts/memory/archival-memory)
- 论文：[MemGPT: Towards LLMs as Operating Systems](https://arxiv.org/abs/2310.08560)
