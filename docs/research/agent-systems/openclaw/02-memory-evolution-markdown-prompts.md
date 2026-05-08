# OpenClaw 的记忆、Markdown 与 Prompt 用法

## 一句话结论

OpenClaw memory 是多组件协作的 runtime：file-backed `MEMORY.md` 配合 sqlite-vec/FTS5 索引，用 active-memory subagent 在主回复前完成 bounded recall，用 dreaming 在后台把高频候选 promotion 到长期记忆，用 memory-wiki 把 durable knowledge 编译成 reviewable vault。这套模式可解释、可审查，但工程复杂度高。

## 源码地图

| 主题 | 文件 | 关键行 |
|---|---|---|
| memory tools 注册 | `extensions/memory-core/src/tools.ts` | 238、402 |
| short-term recall 簿记 | `extensions/memory-core/src/short-term-promotion.ts` | 56-105 |
| promotion 评分 | `extensions/memory-core/src/short-term-promotion.ts` | 1280-1289 |
| promotion 默认阈值 | `extensions/memory-core/src/short-term-promotion.ts` | 24-26 |
| dreaming 三阶段 | `extensions/memory-core/src/dreaming-phases.ts` | 74-107、1601-1751 |
| dreaming controller | `extensions/memory-core/src/dreaming.ts` | 50-172、534-672 |
| REM evidence collection | `extensions/memory-core/src/rem-evidence.ts` | – |
| REM harness | `extensions/memory-core/src/rem-harness.ts` | – |
| narrative prompt | `extensions/memory-core/src/dreaming-narrative.ts` | – |
| concept vocabulary | `extensions/memory-core/src/concept-vocabulary.ts` | – |
| public artifacts | `extensions/memory-core/src/public-artifacts.ts` | – |
| active-memory 限制 | `extensions/active-memory/index.ts` | 28-51 |
| active-memory prompt style | `extensions/active-memory/index.ts` | 97-103、909-928 |
| chunkMarkdown | `packages/memory-host-sdk/src/host/internal.ts` | 362-419 |
| hybrid retrieval | `src/agents/memory-search.ts` | 75-117、290-380 |
| memory-wiki claim health | `extensions/memory-wiki/src/claim-health.ts` | – |
| memory-wiki ingest | `extensions/memory-wiki/src/ingest.ts` | – |

## 记忆处理方案

OpenClaw memory 是多组件协作：

| 组件 | 作用 |
|---|---|
| `memory-core` | 默认 file-backed memory backend、`memory_search` / `memory_get` tools、dreaming 调度 |
| `active-memory` | 主回复前的 blocking recall sub-agent |
| `memory-wiki` | 编译知识 vault，保留 provenance、claim、freshness |
| `memory-lancedb`、QMD 等 | 可选 backend |
| `DREAMS.md` | dreaming diary 与 phase summaries |

`memory_search` 是 broad recall，`memory_get` 是精确读取。`MEMORY.md` 与 `memory/*.md` 被切成 chunk（见下文 chunk 实现），embedding provider 存在时做 hybrid search。

## 检索 pipeline

`src/agents/memory-search.ts` 定义了完整 hybrid retrieval pipeline。默认值（line 103-118）：

| 维度 | 默认值 | 含义 |
|---|---|---|
| `DEFAULT_CHUNK_TOKENS` | 400 | 每个 chunk 的 token 数 |
| `DEFAULT_CHUNK_OVERLAP` | 80 | 相邻 chunk 的 token 重叠 |
| `DEFAULT_MAX_RESULTS` | 6 | top-K |
| `DEFAULT_MIN_SCORE` | 0.35 | 分数阈值 |
| `DEFAULT_HYBRID_VECTOR_WEIGHT` | 0.7 | vector 部分权重 |
| `DEFAULT_HYBRID_TEXT_WEIGHT` | 0.3 | BM25/FTS5 部分权重 |
| `DEFAULT_HYBRID_CANDIDATE_MULTIPLIER` | 4 | 取候选数 = top-K × 4 |
| `DEFAULT_MMR_ENABLED` | false | MMR 多样化默认关闭 |
| `DEFAULT_MMR_LAMBDA` | 0.7 | MMR 相关性权重（与多样性权衡） |
| `DEFAULT_TEMPORAL_DECAY_ENABLED` | false | 时间衰减默认关闭 |
| `DEFAULT_TEMPORAL_DECAY_HALF_LIFE_DAYS` | 30 | 时间半衰期 |

执行顺序大致为：

```text
query → chunkMarkdown(400/80) → 候选生成 (4×top-K)
     → vector(0.7) + BM25(0.3) 融合分数
     → 可选 MMR 多样化（lambda=0.7）
     → 可选时间衰减（halfLife=30d）
     → 阈值过滤 (>0.35)
     → top-6
```

底层存储是 sqlite，索引由 `packages/memory-host-sdk/src/host/memory-schema.ts:43-66` 创建：FTS5 虚拟表 + sqlite-vec 扩展（`sqlite-vec.ts:10-50`）。

`chunkMarkdown` 实现（`packages/memory-host-sdk/src/host/internal.ts:362-419`）按行流式累积，达到 `tokens × CHARS_PER_TOKEN_ESTIMATE` 触发 flush，并保留 `overlap × CHARS_PER_TOKEN_ESTIMATE` 字符进入下一段。这是经典的 token-budget chunker，没有语义分段。

## 常量定位

OpenClaw 内常被引用的具体数字，全部来自源码：

| 数字 | 含义 | 源码 |
|---|---|---|
| 220 | active-memory summary max chars | `extensions/active-memory/index.ts:30` |
| 220 | recent user turn chars | `extensions/active-memory/index.ts:33` |
| 180 | recent assistant turn chars | `extensions/active-memory/index.ts:34` |
| 32,000 | partial transcript max chars | `extensions/active-memory/index.ts:47` |
| 2,000 | transcript read max lines | `extensions/active-memory/index.ts:48` |
| 50 MB | transcript read max bytes | `extensions/active-memory/index.ts:49` |
| 480 | active-memory search query max chars | `extensions/active-memory/index.ts:51` |
| 15,000 ms | default timeout | `extensions/active-memory/index.ts:28` |
| 1,000 | recall cache max entries | `extensions/active-memory/index.ts:36` |
| 3 | circuit breaker timeout 阈值 | `extensions/active-memory/index.ts:43` |
| 4096 | embedding context window 默认 | `packages/memory-host-sdk/src/host/embeddings.types.ts:45` |
| 10 MB | multimodal max file bytes | `packages/memory-host-sdk/src/host/multimodal.ts:26` |
| 400 | default chunk tokens | `src/agents/memory-search.ts:103` |
| 80 | default chunk overlap tokens | `src/agents/memory-search.ts:104` |
| 0.7 / 0.3 | hybrid vector / text 权重 | `src/agents/memory-search.ts:111-112` |
| 0.35 | min score | `src/agents/memory-search.ts:109` |
| 30 | temporal decay half life days | `src/agents/memory-search.ts:117` |
| 0.75 | promotion min score | `extensions/memory-core/src/short-term-promotion.ts:24` |
| 3 | promotion min recall count | `extensions/memory-core/src/short-term-promotion.ts:25` |
| 2 | promotion min unique queries | `extensions/memory-core/src/short-term-promotion.ts:26` |
| `0 3 * * *` | 默认 cron 占位（每日凌晨 3 点） | `extensions/memory-core/openclaw.plugin.json:21` |

## Active Memory Prompt 形态

`extensions/active-memory/index.ts` 中的 recall prompt 形态很关键：

- 它明确告诉子 agent：另一个模型会生成最终回答；
- 子 agent 只能用 memory tools；
- 输出必须是 `NONE` 或紧凑 plain-text summary（≤ 220 chars）；
- 有 timeout（15s）、cache（≤ 1000 entries）、circuit breaker（连续 3 次超时跳过）；
- 支持 5 种 prompt style，由 `resolvePromptStyle`（line 909-928）解析：
  - `balanced`：默认；
  - `strict`：偏保守，只返回明确事实；
  - `contextual`：当前会话上下文相关；
  - `recall-heavy`：偏向召回；
  - `precision-heavy`：偏向精确；
  - `preference-only`：仅返回偏好类信息；
- 会保存 hidden subagent transcript 供调试。

这比 Mnemon 当前需要的提醒重很多，但其中的 bounded output 与 `NONE` gate 值得借鉴。

## Markdown 文件用法

| 文件 | 角色 |
|---|---|
| `AGENTS.md` | 稳定 standing orders |
| `USER.md` | 用户/身份上下文 |
| `MEMORY.md` | long-term memory，session 启动自动加载 |
| `memory/YYYY-MM-DD.md` | daily memory / indexed notes，按需检索 |
| `DREAMS.md` | dreaming diary，人类审查 |
| `memory/.dreams/` | dreaming 工作目录与 lock |
| `memory/dreaming/<phase>/YYYY-MM-DD.md` | phase 报告 |
| wiki vault pages | compiled durable knowledge with claims |

OpenClaw 的 key insight 是：并不是所有 Markdown 都直接进 context。`MEMORY.md` 是 root，`memory/*.md` 多数时候通过 tools 访问。这与「全部 markdown 全注入」的设计有本质区别。

## Dreaming 演化方案

Dreaming 是 OpenClaw 的自进化路径，由 `dreaming.ts` 调度、`dreaming-phases.ts` 执行：

- **light phase**（`dreaming-phases.ts:74-107`）：聚合短期 recall 信号，用 `<!-- openclaw:dreaming:light:start/end -->` 标记写入 daily file，**不**写 `MEMORY.md`。
- **REM phase**：基于 short-term traces 与 theme signals 生成反思（`## REM Sleep` 段），写入 daily file 与 `DREAMS.md`，**不** promotion。REM_REFLECTION_TAG_BLACKLIST 排除 `assistant/user/system/subagent/the` 等无意义 tag。
- **deep phase**（`dreaming.ts:534-672`）：读取 staged candidates，按权重评分，超过 `minScore=0.75` 且 `recallCount≥3` 且 `uniqueQueries≥2` 时 append 到 `MEMORY.md`，**这是唯一会写 root memory 的阶段**。

deep ranking 默认权重（`short-term-promotion.ts:56-63`）：

```text
relevance     0.30
frequency     0.24
diversity     0.15  // unique query 数量
recency       0.15  // 半衰期 14 天，PHASE_SIGNAL_HALF_LIFE_DAYS
consolidation 0.10  // 是否被 light/REM 强化
conceptual    0.06  // concept vocabulary 命中
```

公式（line 1280-1289）：

```text
score = w_freq * normalize(log1p(signalCount)/log1p(10))
      + w_rel  * avgRecallScore
      + w_div  * diversity
      + w_rec  * recencyDecay(ageDays, halfLife)
      + w_con  * consolidationSignal
      + w_cpt  * conceptualRichness
if (score < minScore) skip
```

dreaming 的好处是可解释：每个候选有评分、diary、phase 报告、promotion 记录。代价是 runtime 复杂、后台任务复杂、配置面复杂。Mnemon 第一阶段不需要这一整套，但「评分 + 阈值 + lock」的思路值得借鉴。

## 对 Mnemon 的设计判断

OpenClaw 支持一个结论：memory-driven 自进化可以很强，但工程复杂度会迅速吞噬可移植性。

Mnemon 第一阶段应吸收：

- `NONE` gate；
- provenance（每条 promotion 都带来源 path/line）；
- compaction 前 continuity capture；
- reviewable Markdown artifacts（phase 报告、dreaming diary）；
- memory tools 与 bootstrap docs 分离。

暂不吸收：

- active-memory hidden subagent runtime；
- memory wiki compiler；
- dreaming cron；
- 多 backend slot（lancedb/qmd 等）；
- sqlite-vec + FTS5 + reindex state 的完整 indexer。

## 参考来源

- 本地源码: `extensions/active-memory/index.ts`
- 本地源码: `extensions/memory-core/src/prompt-section.ts`
- 本地源码: `extensions/memory-core/src/dreaming.ts`
- 本地源码: `extensions/memory-core/src/dreaming-phases.ts`
- 本地源码: `extensions/memory-core/src/short-term-promotion.ts`
- 本地源码: `extensions/memory-wiki/src/prompt-section.ts`
- 本地源码: `extensions/memory-wiki/src/claim-health.ts`
- 本地源码: `src/agents/memory-search.ts`
- 本地源码: `packages/memory-host-sdk/src/host/internal.ts`
- 本地源码: `packages/memory-host-sdk/src/host/memory-schema.ts`
- 本地源码: `docs/concepts/dreaming.md`
- 本地源码: `docs/concepts/memory.md`
- 公开文档: [OpenClaw Active memory](https://docs.openclaw.ai/concepts/active-memory)
- 社区/博客信号: [OpenClaw Dreaming explained](https://openclawdc.com/blog/openclaw-dreaming-memory/)
