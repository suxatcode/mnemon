# Letta memory lifecycle 细节

## 核心判断

Letta 是 stateful agent runtime。它把 always-visible memory blocks、archival memory、conversation recall、built-in memory tools、compaction 和 Letta Code 的 MemFS/dream reflection 组合成完整状态系统。

对 Mnemon 来说，Letta 的关键价值是 memory hierarchy 与 compaction 细节；但它比 Mnemon 当前目标重很多。Mnemon 第一阶段不应复制 server-side state runtime，而应把 hierarchy 思想翻译成 Markdown guideline、skills、external recall 和 reviewable patches。

## 生命周期详表

| 维度 | 观察 |
|---|---|
| 主要记忆载体 | core memory blocks、archival memory、conversation history/recall、summary messages、Letta Code MemFS markdown files。 |
| in-context memory | Memory blocks always visible，保留在 agent context 中，不需要 retrieval。 |
| out-of-context memory | Archival memory 是长期 searchable memory，需要工具搜索后进入上下文。 |
| block 限制 | 源码常量：persona/human block char limit 20,000；通用 core memory block char limit 100,000；官方示例 block metadata 可显示 `chars_current` 和 `chars_limit`。 |
| 工具返回限制 | 源码常量：function return char limit 50,000；tool return truncation chars 5,000。 |
| context 限制 | 默认 context window 128,000；min context window 4,096；全局 max context window limit 128,000。 |
| compaction 触发 | conversation history 太长无法放入 context 时自动 compacts older messages；源码/配置中常见 trigger threshold 为 context window 的 0.9。 |
| compaction 默认 | 官方文档：mode `sliding_window`；provider-specific summarizer default；sliding window percentage 0.3；summary limit 50,000 chars。 |
| compaction 超出处理 | 如果保留 70% 仍超预算，summarized portion 会以约 10% step 增加；也可用 `all`、`self_compact_sliding_window`、`self_compact_all`。 |
| Letta Code MemFS | v0.15+ 新 agents 默认启用 MemFS；git-backed context repository，由 Markdown files + frontmatter 组成。 |
| Letta Code reflection | `/sleeptime` 配置 dream/reflection subagents；触发器包括 Off、Step count、Compaction event。 |
| 定时任务 | core server memory lifecycle 主要是事件/溢出驱动；Letta Code 有 background dream/reflection subagents，推荐 MemFS 下由 compaction event 触发。 |
| 安全/一致性 | read-only blocks、block labels/descriptions、tool schema 控制 agent 可编辑范围；memory block limit 更像元数据和 prompt 约束，部分更新路径并非硬截断。 |

## Memory hierarchy

Letta 的 hierarchy 可以理解为三层：

1. Core memory blocks：始终进 prompt，适合 persona、human profile、关键策略、当前状态。
2. Archival memory：长期外部记忆，适合大量 facts、documents、历史知识。
3. Recall/conversation memory：过去消息，可搜索或被 compaction summary 替代。

Letta Code 新增 MemFS 后，memory 也有 Markdown 文件系统形态：

```text
memfs/
  system/
    *.md   # pinned to context
  ...      # tree visible, full content not always injected
```

其中 `system/` 顶层文件 pinned 到上下文，其他文件在 memory tree 中可见但不会完整进入 prompt。这和 Mnemon 的 `GUIDELINE.md` + skills + external recall 非常接近。

## 超出与 compaction

Letta 对超出的处理非常明确：

- 如果 conversation history 无法放入上下文，自动 summarization。
- 默认 sliding window 总结较旧消息，保留较新消息。
- summary 默认最多 50,000 chars。
- 默认总结约 30% messages，保留约 70%；不够时更激进。
- 支持 self-compaction 以提高 prompt cache 命中。
- 如果 system prompt/memory blocks 自身过大，会要求减少 system prompt、memory blocks 或增加 context window。

这说明 Mnemon 不能只依赖「长期记忆文件很大也没关系」。真正常驻上下文的内容必须小；大内容应转为按需 recall。

## 整理与 reflection

Letta core 的整理主要体现在 memory tools 和 compaction。Letta Code 则引入更接近 Mnemon 设想的 background reflection：

- `/sleeptime` 配置 reflection。
- Step count 可每 N 个 user messages 启动反思 subagent。
- Compaction event 可在上下文 compact/summarize 时启动反思 subagent，官方对 MemFS 推荐这个触发器。
- dream subagent 在后台运行，通常会多步编辑 memory。

这说明「在 compaction 事件触发 memory reflection」是社区成熟方向之一。Mnemon 可在 INSTALL 中要求支持该事件的 agent 安装 pre/post compaction hook；不支持的 agent 则退化为 Stop hook。

## 对 Mnemon 的启发

- 把 always-visible 内容严格控制在很小范围：`GUIDELINE.md` 和安装后的 hook reminder。
- 大量 memory 放外部 store，通过 recall 进入上下文。
- summary 和 durable memory 分开。
- compaction event 是最好的 reflection 触发点之一。
- Markdown MemFS 证明「md + LLM 直接维护」是可行路线，但需要 frontmatter、read-only、description、limit 等元数据。

## 参考来源

- 官方文档: [Letta Memory Blocks](https://docs.letta.com/guides/core-concepts/memory/memory-blocks)
- 官方文档: [Letta Compaction](https://docs.letta.com/guides/core-concepts/messages/compaction)
- 官方文档: [Letta Code Memory](https://docs.letta.com/letta-code/memory/)
- 官方文档: [Letta Archival Memory](https://docs.letta.com/guides/core-concepts/memory/archival-memory)
- 本地源码: `/tmp/mnemon-agent-research-sources/letta/letta/constants.py`
- 本地源码: `/tmp/mnemon-agent-research-sources/letta/letta/schemas/block.py`
- 本地源码: `/tmp/mnemon-agent-research-sources/letta/letta/services/summarizer/`
- 本地源码: `/tmp/mnemon-agent-research-sources/letta/letta/agents/letta_agent_v3.py`
