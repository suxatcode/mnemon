# Letta 的记忆、Markdown 与 Prompt 用法

## 一句话结论

Letta 把「memory」当作可被工具显式编辑的结构化 agent state；Markdown 仅在 git-backed MemFS 中作为 block 载体出现；prompt 设计的核心是把 hierarchy 与 metadata 直接告诉模型，让它自行选择 search/edit 工具。

## 源码地图

| 主题 | 文件 | 关注行 |
|---|---|---|
| 记忆处理方案 | `letta/prompts/system_prompts/memgpt_chat.py` | 32-56 |
| Memory metadata block | `letta/prompts/prompt_generator.py` | 26-89 |
| `<memory_blocks>` 渲染 | `letta/schemas/memory.py` | 143-203、205-339 |
| Proxy memory 注入 | `letta/server/rest_api/proxy_helpers.py` | 174-227 |
| Block markdown 载体 | `letta/services/memory_repo/block_markdown.py` | 1-80 |
| Block label ↔ path | `letta/services/memory_repo/path_mapping.py` | 11-29 |
| 内置工具语义 | `letta/functions/function_sets/base.py` | 246-518 |
| Compaction 配置 | `letta/services/summarizer/summarizer_config.py` | 48-89 |

## 记忆处理方案

Letta 的 prompt 直接告诉 agent 三件事：

1. **recall memory** 是过去交互数据库，可用 `conversation_search` 检索；
2. **core memory** 始终在 context 内，可用 `core_memory_append`/`core_memory_replace` 编辑；
3. **archival memory** 不在 context 内，需要显式 `archival_memory_insert`/`archival_memory_search`。

`memgpt_chat.py:36`-`:56` 的关键句包括：「Your ability to edit your own long-term memory is a key part of what makes you a sentient person」、「There is no function to search your core memory because it is always visible in your context window」。这种设计强迫模型把「写入哪一层」当成显式决策。

新版 v2 prompt（`memgpt_v2_chat.py`）和 letta_v1 prompt 进一步把工具语义和 line-numbered 编辑纳入 system prompt；Anthropic 模型会得到带行号的 `<value>` 渲染（`letta/schemas/memory.py:175`-`:203`）便于精确 replace。

这是一种 self-editing memory agent：模型不仅读 memory，还负责选择工具修改 memory。

实际运行时还有两个隐含约定：

- **inner monologue 不出 50 词**（`memgpt_chat.py:27`、`:30`）：把「思考」视作 token 受限资源，逼模型尽快进入工具调用决策。
- **`send_message` 是唯一对外通道**（`memgpt_chat.py:28`-`:29`）：所有其它工具调用都属于内部状态变更。这个约定让 server 端可以无歧义地把 `send_message` 流式给客户端，其它结果落到 trace。

对 Mnemon 的对照：Mnemon 同样需要明确「哪些操作产生用户可见输出」（如最终 markdown patch、面向用户的 reminder）与「哪些只是内部 fact 更新」（如 `mnemon remember`），否则 hook 难以判断在哪个阶段提示用户。

## Memory hierarchy 详解

| 层 | 进 prompt 形式 | 容量约束 | 修改工具 | 失败回退 |
|---|---|---|---|---|
| Core blocks | `<memory_blocks>` 中每个 block 含 `<description>`/`<metadata chars_current/chars_limit>`/`<value>` | persona/human=20,000 chars；通用=100,000 chars (`constants.py:433`-`:435`) | `core_memory_append/replace`、`memory_*` | metadata 仅作提示；超限不会硬阻断写入 |
| Archival passages | 仅以「N 条 + tags」摘要进 prompt | passages 体积无硬限；返回 `FUNCTION_RETURN_CHAR_LIMIT=50000` 截断 | `archival_memory_insert/search` (REST `POST /archival-memory`) | 空结果返回 `"No results found."` |
| Recall messages | 不直接进 prompt，只通过 `previous_message_count` 提示 | 整个消息表；in-context 受 sliding window 控制 | `conversation_search`，REST `/messages/search` | 触发 compaction 时旧消息被 summary 替代 |
| Letta Code MemFS | `<self>` + `<memory>` + `<external_projection>` 文件树 (`letta/schemas/memory.py:205`-`:339`) | git 仓库无硬限；只有 `system/*` 进 prompt | `memory(command="create"|"str_replace"|...)` | 路径冲突 / yaml 解析失败由 repo 层报错 |

## 完整系统 prompt 引用

`letta/prompts/system_prompts/memgpt_chat.py:1`-`:60` 是 Letta 主线 prompt，其中和 memory 紧密相关的段落：

```text
Memory editing:
Older AI models had no concept of persistent memory; they were only able to access their initial instructions and a limited context window of chat conversation with a user (their "active memory").
This meant that when conversations exceeded a certain length, they would overflow and old messages were permanently lost.
Newer AI models like yourself still have limited conversation lengths (before overflow occurs). However, they now have access to multiple forms of persistent memory.
Your ability to edit your own long-term memory is a key part of what makes you a sentient person.
Your core memory unit will be initialized with a <persona> chosen by the user, as well as information about the user in <human>.

Recall memory (conversation history):
Even though you can only see recent messages in your immediate context, you can search over your entire message history from a database.
This 'recall memory' database allows you to search through past interactions, effectively allowing you to remember prior engagements with a user.
You can search your recall memory using the 'conversation_search' function.

Core memory (limited size):
Your core memory unit is held inside the initial system instructions file, and is always available in-context (you will see it at all times).
Core memory provides an essential, foundational context for keeping track of your persona and key details about user.
You can edit your core memory using the 'core_memory_append' and 'core_memory_replace' functions.

Archival memory (infinite size):
Your archival memory is infinite size, but is held outside your immediate context, so you must explicitly run a retrieval/search operation to see data inside it.
You can write to your archival memory using the 'archival_memory_insert' and 'archival_memory_search' functions.
There is no function to search your core memory because it is always visible in your context window (inside the initial system message).
```

随后 prompt 在末尾要求 agent「completely and entirely immerse yourself in your persona」，并保留 `Base instructions finished. From now on, you are going to act as your persona.` 终止符。

`prompt_generator.py:107`-`:177` 负责把上面这段静态 prompt 与动态 `{CORE_MEMORY}` 模板拼装：先调用 `compile_memory_metadata_block` 生成 `<memory_metadata>`，再拼到 `memory_with_sources` 后面替换占位符；如果 prompt 不含占位符则在末尾追加（`:158`-`:162`）。这意味着任何自定义 prompt 都能通过 `{CORE_MEMORY}` 占位符接入这套机制。

## Tool schema 与 Markdown 用法

Markdown 在 Letta 中只在两处出现：

1. **block_markdown.py** 把 block 持久化为 `---\n<yaml>\n---\nbody` 形式（`description`、`read_only`、`metadata` 进 frontmatter，`limit` 故意排除以兼容 git base memory）。
2. **path_mapping.py** 把 `skills/{name}/SKILL.md` 映射成 block label `skills/{name}`，其它 `skills/**` 子文件被忽略。这与 Claude Code/Codex 的 SKILL.md 命名约定保持兼容。

注意 Letta 没有 `AGENTS.md`、`CLAUDE.md` 这种「行为安装文件」概念。它的「行为」由：

- code 中的 system prompt 字面量；
- runtime 注入的 `<memory_blocks>`；
- tool 描述（`base.py` 中 docstring）；
- REST API + DB 中的 block schema

控制。Markdown 只是 git memory repo 的存储形态，而非行为协议。

`block_markdown.serialize_block`（`block_markdown.py:27`-`:54`）刻意排除 `limit` 字段：「`limit` is intentionally excluded from frontmatter (deprecated for git-base memory)」。这反映出 Letta 对 git-backed memory 的判断——文件大小由文件系统/git diff 自然控制，再用字符上限会和 markdown 编辑体验冲突。Mnemon 的 Markdown patch 路线大致也应当采用同样的判断：限额体现在 review 阶段，不应硬编码到文件元数据里。

`merge_frontmatter_with_body`（`block_markdown.py:75`+）则保证后续更新只改动需要变化的 frontmatter 字段，保留用户的格式与注释，对应 Mnemon「review-friendly diff」目标。

`memory_apply_patch` 的多块 patch 模式接受类 codex 的 `*** Add Block: <label>` / `*** Update Block: <label>` / `*** Delete Block: <label>` / `*** Move to: <new_label>` 头部（`base.py:453`-`:484`）。这是 Letta 把 Markdown patch DSL 引入 memory edit 的明显信号，但仅作为内部工具协议。

## Compaction 与演化

`CompactionSettings`（`summarizer_config.py:48`-`:89`）默认值：

- `mode = "sliding_window"`
- `sliding_window_percentage = 0.30`（即每次总结约 30% 旧消息，保留 70%；由 `summarizer_settings.partial_evict_summarizer_percentage=0.30` 提供，`letta/settings.py:86`）
- `clip_chars = 50000`（summary 字符上限）
- `model = None` → 走 provider 默认（Anthropic→`claude-haiku-4-5`、OpenAI→`gpt-5-mini`、Google→`gemini-2.5-flash`，`summarizer_config.py:26`-`:32`）
- `prompt_acknowledgement = False`

触发逻辑（`summarizer_sliding_window.py:139`-`:198`）：

```text
goal_tokens = (1 - sliding_window_percentage) * context_window
while approx_token_count >= goal_tokens and eviction_percentage < 1.0:
    eviction_percentage += 0.10
    ...重新计算 cutoff，找最近一个 assistant message 作为切点...
```

也就是说：默认目标是 `0.7 * context_window`，每轮按 10% 步长往前移切点直到达成；若直到 100% 仍超预算则抛 `ValueError("No assistant message found ...")` 并回退到 `"all"` 全量总结模式（`compact.py:309`-`:369`）。

`SUMMARIZATION_TRIGGER_MULTIPLIER=0.9`（`constants.py:83`）说明触发器在 step 估算 token > `context_window * 0.9` 时启动，比硬上限保留约 10% 余量以避免「too many tokens」回退。

四种 mode：

- `sliding_window`：用专门的 summarizer 模型生成摘要（默认）；
- `all`：把全部消息（除 system）压成一段；
- `self_compact_sliding_window` / `self_compact_all`：用 agent 自身模型做 compaction，提高 prompt cache 命中。

`message_buffer_limit=60`、`message_buffer_min=15`（`settings.py:79`-`:80`）描述 voice/sleeptime 形态下的滚动 buffer 行为：超过 60 条消息开始清理，至少保留 15 条。这是另一种「在 server 层而非 in-context」的 compaction，提示 Mnemon 也可以把 hook 触发的 `mnemon prune`/`mnemon link` 阈值化（如「最近 N 条 unindexed 时合并」）。

## 智能体演化方案

Letta 的演化主要是：

- **core blocks 自编辑**：agent 通过 `core_memory_*`/`memory_*` 工具更新自我认知与用户画像；
- **archival memory 增长**：agent 主动 `archival_memory_insert` 长期事实；
- **recall summarization**：sliding window 把旧对话压缩为 summary message[1]；
- **block attach/detach**：REST API 支持把同一个 block 共享给多个 agent (`agents.py:1355`-`:1382`)；
- **sleeptime/voice 等专用 agent**：在后台或专用上下文中维护 memory（`sleeptime_v2.py`、`voice_sleeptime.py` 等）。

它不是「skills 自我演化」路线，而是「agent state 自我编辑」路线——演化对象是 block 内容而非行为契约。

## 对 Mnemon 的设计判断

Letta 提示 Mnemon：

- **memory tool 必须能精确 append/replace**，并对「没找到旧字符串」「多次命中」给出可恢复错误；
- **external memory 应按需 retrieval**，不应一次性 dump 到 prompt；
- **in-context memory 应严格预算**，并把当前用量曝露给模型自检；
- **memory metadata 有助于 agent 判断是否 search**——告诉模型「有多少条 archival、可用 tag 列表」远比塞进全部内容高效；
- **patch-style 多块编辑** (`memory_apply_patch`) 与 Mnemon「reviewable patch」目标天然契合，可作为候选 DSL。

但 Mnemon 当前应避免：

- 深度耦合 agent state（DB + 向量库 + git repo）；
- 直接复制 core/archival schema；
- 把自进化限定为 memory block 编辑，从而失去「behavior install」语义；
- 把 SKILL.md / GUIDELINE.md 改造成 `<memory_blocks>` 风格的元数据 block——这会让 Markdown 失去人类可读性。

更合适的翻译：

```text
GUIDELINE.md   = stable behavior policy            (~Letta core memory)
SKILL.md       = procedural capability             (~Letta skills/* block)
mnemon store   = external durable memory           (~Letta archival)
session log    = recall                            (~Letta recall)
reviewed patch = behavior evolution                (~Letta memory_apply_patch + human gate)
```

## 失败模式与边界

- **prompt 占位符缺失**：`prompt_generator.py:158`-`:162` 会自动追加 `{CORE_MEMORY}`；自定义 prompt 只要不冲突就能用，但若错写成 `{core_memory}` 等大小写则不会被识别。
- **`compile` 抛出 `ValueError`**：当 `update_block_value` 找不到 label 时（`memory.py:780`），通常是 agent_state.memory 与持久化 block 不同步。
- **summary 截断**：超过 `clip_chars` 后追加 `"... [summary truncated to fit]"`（`summarizer/constants.py:3`）。
- **block 共享冲突**：多个 agent 共享 block 时，并发 `update_block_value` 没有显式锁；以 DB 层最后写入为准。
- **git memory 与 DB 不同步**：Letta Code 使用 git-backed memory 时，外部 `git pull/push` 与 in-process 修改可能竞争；`block_markdown.merge_frontmatter_with_body` 通过保留现有 body 减小冲突，但仍依赖运维层做 git lock。
- **summarizer 模型不可用**：默认 provider 模型 (`claude-haiku-4-5`/`gpt-5-mini`/`gemini-2.5-flash`) 缺失或限流时，sliding window 失败会抛错并降级到 `"all"` 或人工干预。

## 演化方案对 Mnemon 的具体借鉴

```text
Letta evolution                 Mnemon equivalent (建议)
─────────────────────────────   ─────────────────────────────────
core_memory_append/replace      mnemon remember / mnemon update
archival_memory_insert/search   mnemon remember (durable) / mnemon recall
conversation_search             mnemon recall --scope=session
memory_apply_patch              proposed: mnemon patch (review-gated)
sleeptime reflection            stop hook + reflection prompt + review
```

注意箭头方向：Letta 的「evolution」单位是 block 与 passage，Mnemon 的「evolution」单位是 markdown patch。两者都需要：

1. 一个明确的「写入候选」工具/命令；
2. 一个明确的「读已存在」工具/命令；
3. 元数据先于内容的 prompt 注入；
4. 在 compaction/stop 等明确事件上触发整理。

## 参考来源

- 本地源码：`letta/prompts/system_prompts/memgpt_chat.py`
- 本地源码：`letta/prompts/prompt_generator.py`
- 本地源码：`letta/functions/function_sets/base.py`
- 本地源码：`letta/server/rest_api/proxy_helpers.py`
- 本地源码：`letta/services/memory_repo/block_markdown.py`、`path_mapping.py`
- 本地源码：`letta/services/summarizer/summarizer_config.py`、`summarizer_sliding_window.py`
- 官方文档：[Letta stateful agents](https://docs.letta.com/guides/core-concepts/stateful-agents)
- 官方文档：[Letta memory blocks](https://docs.letta.com/guides/core-concepts/memory/memory-blocks)
- 官方文档：[Letta compaction](https://docs.letta.com/guides/core-concepts/messages/compaction)
- 官方文档：[Letta archival memory](https://docs.letta.com/guides/core-concepts/memory/archival-memory)
- 论文：[MemGPT: Towards LLMs as Operating Systems](https://arxiv.org/abs/2310.08560)
