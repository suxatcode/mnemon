# Letta memory lifecycle 细节

## 核心判断

Letta 是 stateful agent runtime。它把 always-visible memory blocks、archival memory、conversation recall、built-in memory tools、compaction 和 Letta Code 的 MemFS/dream reflection 组合成完整状态系统。

对 Mnemon 来说，Letta 的关键价值是 memory hierarchy 与 compaction 细节；但它比 Mnemon 当前目标重很多。Mnemon 第一阶段不应复制 server-side state runtime，而应把 hierarchy 思想翻译成 Markdown guideline、skills、external recall 和 reviewable patches。

## 源码地图

| 主题 | 文件 | 关注行 |
|---|---|---|
| 容量常量 | `letta/constants.py` | 78-83、433-443、488 |
| Block schema 默认值 | `letta/schemas/block.py` | 20、36、67、103 |
| `Memory.compile` 渲染分支 | `letta/schemas/memory.py` | 688-712 |
| `<memory_blocks>` 标准渲染 | `letta/schemas/memory.py` | 143-203 |
| Git/MemFS 渲染 | `letta/schemas/memory.py` | 205-339 |
| 内置 memory 工具 | `letta/functions/function_sets/base.py` | 246-518 |
| Memory metadata block | `letta/prompts/prompt_generator.py` | 26-89 |
| Compaction 入口 | `letta/services/summarizer/compact.py` | 18-369 |
| Sliding window 主体 | `letta/services/summarizer/summarizer_sliding_window.py` | 99-232 |
| Compaction 默认设置 | `letta/services/summarizer/summarizer_config.py` | 48-89 |
| Self-summarize | `letta/services/summarizer/self_summarizer.py` | 154-225 |
| Summarizer 全局参数 | `letta/settings.py` | 79-86 |
| REST 入口 | `letta/server/rest_api/routers/v1/agents.py` | 1206-2430 |
| Memory repo (markdown/git) | `letta/services/memory_repo/block_markdown.py`、`path_mapping.py` | 1-80、11-29 |

## 生命周期详表

| 维度 | 观察 |
|---|---|
| 主要记忆载体 | core memory blocks、archival memory passages、conversation history/recall messages、summary message[1]、Letta Code MemFS markdown files。 |
| in-context memory | Memory blocks always visible（`Memory.compile` 渲染进 `<memory_blocks>` 或 git `<memory>`）；不需要 retrieval。 |
| out-of-context memory | Archival memory 是长期 searchable memory，需要 `archival_memory_search` 进入上下文；recall messages 通过 `conversation_search` 取回。 |
| block 限制 | `CORE_MEMORY_PERSONA_CHAR_LIMIT=20000`、`CORE_MEMORY_HUMAN_CHAR_LIMIT=20000`、`CORE_MEMORY_BLOCK_CHAR_LIMIT=100000`（`constants.py:433`-`:435`）；block metadata 在 prompt 中显示 `chars_current` 与 `chars_limit`。 |
| 工具返回限制 | `FUNCTION_RETURN_CHAR_LIMIT=50000`、`BASE_FUNCTION_RETURN_CHAR_LIMIT=50000`、`TOOL_RETURN_TRUNCATION_CHARS=5000`（`constants.py:438`-`:443`）；超出时由 `FUNCTION_RETURN_VALUE_TRUNCATED` 包装提示。 |
| context 限制 | `MIN_CONTEXT_WINDOW=4096`、`DEFAULT_CONTEXT_WINDOW=128000`（`constants.py:78`-`:79`）；`LLM_MAX_CONTEXT_WINDOW` 表（`:251`）按模型映射上限。 |
| compaction 触发 | step 估算 token 超过 `context_window * SUMMARIZATION_TRIGGER_MULTIPLIER (0.9)` 时触发（`constants.py:83`）。 |
| compaction 默认 | `mode="sliding_window"`、`sliding_window_percentage=0.30`、`clip_chars=50000`、`prompt_acknowledgement=False`（`summarizer_config.py:48`-`:89`、`settings.py:86`）。 |
| compaction 步进 | 找最近 assistant message 作切点；若仍超目标，eviction_percentage += 0.10，最多到 1.0（`summarizer_sliding_window.py:163`-`:198`）。 |
| compaction 替代模式 | `all`（全部压缩）、`self_compact_sliding_window`、`self_compact_all`（用 agent 自身模型，`compact.py:215`-`:309`），可通过 `POST /agents/{id}/summarize` 主动触发。 |
| Letta Code MemFS | v0.15+ 默认启用；git-backed Markdown + YAML frontmatter（`block_markdown.py:27`-`:54`），`system/*` 子树注入 `<memory>`，其它 file tree 仅以 `<external_projection>` 显示。 |
| Letta Code reflection | `/sleeptime` 配置 dream/reflection subagent，触发器：`Off`、`Step count`、`Compaction event`；MemFS 推荐 `Compaction event`。 |
| 定时任务 | core runtime 主要是事件/溢出驱动；Letta Code 在后台跑 dream subagent，不是 cron。 |
| 安全/一致性 | `read_only` block + `description` + tool schema 控制 agent 可编辑范围；`memory_replace` 拒绝行号前缀、要求唯一匹配；REST `PATCH` 走 BlockManager 经数据库持久化。 |

## Memory hierarchy

Letta 的 hierarchy 三层：

1. **Core memory blocks**：始终进 prompt，适合 persona、human profile、关键策略、当前状态。渲染在 `<memory_blocks>` 中，包含 `<description>`、`<metadata>`（`read_only`/`chars_current`/`chars_limit`）、`<value>`。
2. **Archival memory**：长期外部记忆，向量检索；适合大量 facts、documents、历史知识；通过 metadata block 告诉模型条目数与可用 tag。
3. **Recall/conversation memory**：过去消息，可搜索（`conversation_search`）或被 sliding window summary 替代。

Letta Code 新增 MemFS 后，memory 也有 Markdown 文件系统形态：

```text
memfs/
  system/
    persona.md     # 渲染为 <self>
    human.md       # 渲染为 <memory><human>...</human></memory>
    {others}.md    # 嵌套渲染为 <memory> 子树
  skills/
    {name}/SKILL.md  # block label = skills/{name}
  ...                # 其它路径 -> <memory><external_projection> 文件树
```

`system/` 顶层 pinned 进 prompt；`skills/{name}/SKILL.md` 通过 `path_mapping.memory_block_label_from_markdown_path` 映射成 block label `skills/{name}`；其它路径仅在 file tree 中可见，不会完整进 prompt。这和 Mnemon 的 `GUIDELINE.md` + skills + external recall 非常接近。

## 关键容量速查

| 常量 | 值 | 来源 | 含义 |
|---|---|---|---|
| `MIN_CONTEXT_WINDOW` | 4096 | `constants.py:78` | 最小允许的 context window |
| `DEFAULT_CONTEXT_WINDOW` | 128000 | `constants.py:79` | 缺省 context window |
| `SUMMARIZATION_TRIGGER_MULTIPLIER` | 0.9 | `constants.py:83` | 触发 compaction 的相对阈值 |
| `CORE_MEMORY_PERSONA_CHAR_LIMIT` | 20000 | `constants.py:433` | persona block 字符上限 |
| `CORE_MEMORY_HUMAN_CHAR_LIMIT` | 20000 | `constants.py:434` | human block 字符上限 |
| `CORE_MEMORY_BLOCK_CHAR_LIMIT` | 100000 | `constants.py:435` | 通用 core block 字符上限 |
| `FUNCTION_RETURN_CHAR_LIMIT` | 50000 | `constants.py:438` | 函数返回值最大字符 |
| `BASE_FUNCTION_RETURN_CHAR_LIMIT` | 50000 | `constants.py:439` | base 函数返回值最大字符 |
| `TOOL_RETURN_TRUNCATION_CHARS` | 5000 | `constants.py:443` | 工具返回截断粒度 |
| `DEFAULT_CORE_MEMORY_SOURCE_CHAR_LIMIT` | 50000 | `constants.py:488` | 来源块字符上限 |
| `summarizer.partial_evict_summarizer_percentage` | 0.30 | `settings.py:86` | 默认 sliding window 比例 |
| `CompactionSettings.clip_chars` | 50000 | `summarizer_config.py:72` | summary 字符上限 |
| `summarizer.message_buffer_limit` | 60 | `settings.py:79` | voice/sleeptime buffer 上限 |
| `summarizer.message_buffer_min` | 15 | `settings.py:80` | voice/sleeptime buffer 下限 |

## 完整工具签名（lifecycle 视角）

| 工具 | 参数 | 副作用 | lifecycle 角色 |
|---|---|---|---|
| `core_memory_append(label, content)` | label, content | `current + "\n" + content` 写回 block | 增长 core 内容（`base.py:246`） |
| `core_memory_replace(label, old, new)` | 精确匹配 | 字符串替换 | 修订 core；`old` 不存在抛错（`:276`） |
| `memory_replace(label, old_string, new_string)` | 唯一匹配；拒绝行号前缀 | 字符串替换 | 行号渲染下的精确编辑（`:311`） |
| `memory_insert(label, new_string, insert_line=-1)` | 行索引 | 在指定行后插入 | 结构化追加（`:391`） |
| `memory_apply_patch(label, patch)` | 多块 patch | 增删改 block | 大规模重组（`:453`） |
| `memory_rethink(label, new_memory)` | 整块覆写 | 整体替换 | sleep-time agent 重构（`:488`） |
| `memory_finish_edits()` | 无 | 信号 | 标记编辑会话结束（`:520`） |
| `archival_memory_insert(content, tags)` | 文本 + tags | 写入向量库 | 长期事实 |
| `archival_memory_search(query, tags?, top_k?, ...)` | 自然语言 query | 读出 passages | 长期检索 |
| `conversation_search(query?, roles?, limit?, dates?)` | 任意组合 | 读出消息 | recall |
| `send_message(message)` | 字符串 | 唯一面向用户输出 | 对外通信 |

## 超出与 compaction

Letta 对超出的处理路径（`summarizer_sliding_window.py:99`-`:232`、`compact.py`）：

1. step 估算 token 超过 `0.9 * context_window` → 触发 sliding window 总结。
2. `goal_tokens = (1 - 0.30) * context_window`（默认 70% 保留）。
3. 从 `eviction_percentage = 0.30` 开始，找 cutoff 处最近 assistant message，让保留段 `[system_prompt, *messages[cutoff:]]` token 数 ≤ `goal_tokens`；不够则 `+= 0.10`。
4. 调 summarizer 模型（默认 provider 轻量模型）生成 summary；若 `len(summary) > clip_chars (50000)`，截断并追加 `"... [summary truncated to fit]"`。
5. summary 作为 message[1] 写回，新的 in-context = `[system_prompt, summary, *messages[cutoff:]]`。
6. 若 eviction_percentage 到 1.0 仍超预算 → 抛 `ValueError`，回退到 `"all"` 全量压缩或要求扩大 context window。

`self_summarize_sliding_window`（`self_summarizer.py:154`-`:225`）走相似逻辑但用 agent 自身模型，复用 prompt cache。

如果 system prompt + memory blocks 自身已经超预算（与消息无关），Letta 会直接报错并要求减少 system prompt、memory blocks 或增加 context window；compaction 不会缩减 core memory。

这说明 Mnemon 不能只依赖「长期记忆文件很大也没关系」。真正常驻上下文的内容必须小；大内容应转为按需 recall。

## 整理与 reflection

Letta core 的整理主要体现在 memory tools 和 compaction。Letta Code 则引入更接近 Mnemon 设想的 background reflection：

- `/sleeptime` 配置 reflection；
- **Step count** trigger：每 N 个 user messages 启动反思 subagent；
- **Compaction event** trigger：在 sliding window 触发时联动反思 subagent，官方对 MemFS 推荐这个触发器；
- dream subagent 在后台运行，通常会多步编辑 `system/*` 与 archival passages。

这说明「在 compaction 事件触发 memory reflection」是社区成熟方向之一。Mnemon 可在 INSTALL 中要求支持该事件的 agent 安装 pre/post compaction hook；不支持的 agent 则退化为 Stop hook。

进一步的 lifecycle 时序（Letta Code MemFS）：

```text
[step] user message
   |
   |-- agent step (tool calls) --+
   |                              |
   |-- token check --> trigger?   |
   |     yes -> sliding_window    |
   |             |                |
   |             |-- summary written to message[1]
   |             |-- (if MemFS) compaction event ---+
   |                                                 |
   |                                                 v
   |                                        sleeptime/dream subagent
   |                                          - reads compacted region
   |                                          - 多步 memory_* 编辑
   |                                          - git commit MemFS 变更
   |
   |-- next step
```

`agents.py:2430` 的 `POST /agents/{id}/summarize` 让运维方可以主动诱发该 lifecycle，便于在 CI/批处理里复现整理流程。

## REST API 形态（lifecycle 用法）

| 阶段 | endpoint | 用法 |
|---|---|---|
| 创建/查看 | `POST /agents/`、`GET /agents/{id}` | 提供 `memory_blocks` 列表初始化 core；`/{id}/context` 查看 token 占用（已 deprecated） |
| 读 core | `GET /agents/{id}/core-memory/blocks[/{label}]` | 不经过 LLM 直接读 block |
| 写 core | `PATCH /agents/{id}/core-memory/blocks/{label}` | 外部系统直接更新（绕过 tool） |
| 共享 core | `PATCH /agents/{id}/core-memory/blocks/(attach|detach)/{block_id}` | 让多个 agent 共享同一 block |
| 读/写 archival | `GET|POST|DELETE /agents/{id}/archival-memory[...]` | 不经过 agent 操作长期记忆 |
| 读 recall | `GET /agents/{id}/messages`、`POST /agents/messages/search` | 全量/搜索消息 |
| 主动 compaction | `POST /agents/{id}/summarize` | 触发 sliding window 或 self-compact |
| 重新编译 system prompt | `POST /agents/{id}/...recompile...` (`agents.py:1291`、`:1326`) | block 变更后 force recompile |
| 重置 | `PATCH /agents/{id}/reset-messages` (`:2329`) | 清空 conversation history |

外部模型代理路径还会用 `proxy_helpers.format_memory_blocks`（`proxy_helpers.py:174`-`:227`）把 `<memory_blocks>` 注入到对外请求中，并附带 `https://app.letta.com/agents/{id}` 的链接。

## 失败模式

- **core block 超限**：metadata 提示 `chars_current >= chars_limit`，但 `core_memory_append` 不硬阻断。需要靠 prompt 引导或外部校验。
- **archival_search 空结果**：`conversation_search` 返回 `"No results found."`；archival 由 runtime 实现。Agent 必须能容忍空结果并尝试更宽 query 或落到 `core` 已知信息。
- **`*_replace` 找不到 / 多次匹配**：抛 `ValueError`，提示行号；agent 应先 read，再 retry。
- **summary 截断**：超过 `clip_chars=50000` 追加 `"... [summary truncated to fit]"`，agent 看到的将是不完整摘要。
- **context overflow**：sliding window 失败 → 退回 `"all"` mode 或抛错，要求人工介入；这与 Mnemon 不应让重要 fact 仅存于 recall 一致。
- **自定义 prompt 缺 `{CORE_MEMORY}` 占位符**：`prompt_generator.py:158`-`:162` 自动 append；但若使用 mustache 模板会抛 `NotImplementedError`（`:175`）。
- **block 共享并发写**：无显式锁，最后写入胜出；多 agent 协作时需要应用层协调。

## 对 Mnemon 的启发

可借鉴：

- 把 always-visible 内容严格控制在很小范围：`GUIDELINE.md` 与安装后的 hook reminder。
- 大量 memory 放外部 store，通过 recall 进入上下文；并曝露「条目数 + tag/label」给 agent，让它先决定是否搜索。
- summary 与 durable memory 分开存放：summary 是有损压缩，事实必须落到 archival 或 SKILL.md。
- compaction event 是最好的 reflection 触发点之一；Mnemon 的 hook 可在 stop / pre-compaction 阶段调用 `mnemon link` / `mnemon recall`。
- Markdown MemFS 证明「md + LLM 直接维护」是可行路线，但需要 frontmatter（`description`、`read_only`、`metadata`）来表达元信息。
- patch-style 多块编辑（`memory_apply_patch`）可作为 Mnemon 候选 patch DSL 的现成参考。

不应照搬：

- 全套 server runtime（FastAPI + DB + 向量库 + git repo + sleeptime subagent）超出 Mnemon CLI 范畴。
- core/archival/recall 的 schema 与消息存储深度耦合，会让 Mnemon 不得不维护 agent state。
- block 字符上限作为元数据提示而非硬约束，对 Mnemon「review-driven」语义来说太弱。
- self-editing memory 完全交由 agent，没有 human gate；Mnemon 必须保留 review。

## 阶段化映射建议

Mnemon 第一阶段（CLI + Markdown patch）只需吸收 Letta 以下信号：

1. memory 元数据进 prompt：在 hook 输出中告诉 agent 当前有多少条 fact、最近被引用的 tag 是什么。
2. 工具协议明确「精确匹配 + 唯一性」：在 `mnemon update` / patch DSL 上预检 `old_string` 的唯一出现，匹配失败给出行号建议。
3. compaction 事件作为 reflection 触发器：把 `mnemon link` 的运行时机从「每次 stop」收紧为「stop + 长会话或 token 接近上限」。
4. 容量提示作为引导而非硬约束：在 INSTALL 中规定 `GUIDELINE.md` 推荐 < 5KB、`SKILL.md` 推荐 < 15KB，但允许个别 patch 临时超出，由 review 决定是否拆分。

Mnemon 第二阶段（如果引入轻量 runtime adapter）才需要考虑 Letta 的：

- 持久化 + 共享 block 的多 agent 协作；
- archival vector index；
- self_compact 与 prompt cache；
- sleeptime subagent。

这些能力的运维成本明显高于第一阶段目标，应在用户实际反馈「Markdown 不够用」后再分别 opt-in。

## 参考来源

- 官方文档：[Letta Memory Blocks](https://docs.letta.com/guides/core-concepts/memory/memory-blocks)
- 官方文档：[Letta Compaction](https://docs.letta.com/guides/core-concepts/messages/compaction)
- 官方文档：[Letta Code Memory](https://docs.letta.com/letta-code/memory/)
- 官方文档：[Letta Archival Memory](https://docs.letta.com/guides/core-concepts/memory/archival-memory)
- 本地源码：`/tmp/mnemon-agent-research-sources/letta/letta/constants.py`
- 本地源码：`/tmp/mnemon-agent-research-sources/letta/letta/schemas/block.py`
- 本地源码：`/tmp/mnemon-agent-research-sources/letta/letta/schemas/memory.py`
- 本地源码：`/tmp/mnemon-agent-research-sources/letta/letta/functions/function_sets/base.py`
- 本地源码：`/tmp/mnemon-agent-research-sources/letta/letta/prompts/prompt_generator.py`
- 本地源码：`/tmp/mnemon-agent-research-sources/letta/letta/services/summarizer/`
- 本地源码：`/tmp/mnemon-agent-research-sources/letta/letta/services/memory_repo/`
- 本地源码：`/tmp/mnemon-agent-research-sources/letta/letta/server/rest_api/routers/v1/agents.py`
- 本地源码：`/tmp/mnemon-agent-research-sources/letta/letta/server/rest_api/proxy_helpers.py`
- 本地源码：`/tmp/mnemon-agent-research-sources/letta/letta/settings.py`
