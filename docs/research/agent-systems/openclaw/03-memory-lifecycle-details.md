# OpenClaw memory lifecycle 细节

## 核心判断

OpenClaw 是本轮调研中工程化程度最高的 memory runtime。它把 Markdown 文件、semantic search、active recall、compaction 前 flush、dreaming consolidation、wiki compiler 与 cron sweep 组合成一套完整系统。

这给 Mnemon 的启发是「上限参考」而非「第一阶段照搬」。Mnemon 应学习它的 reviewable artifacts、compaction 前保存、阶段化 consolidation 与 promotion lock，但暂不复制 active-memory hidden subagent、wiki compiler 与 dreaming scheduler。

## 源码地图

| 主题 | 文件 | 关键行 |
|---|---|---|
| active-memory 配置 | `extensions/active-memory/index.ts` | 28-51、97-103 |
| active-memory subagent runner | `extensions/active-memory/index.ts` | 2423-2591 |
| dreaming controller | `extensions/memory-core/src/dreaming.ts` | 50-172、233-409、534-672 |
| dreaming 三阶段 | `extensions/memory-core/src/dreaming-phases.ts` | 74-107、1601-1751 |
| short-term recall store | `extensions/memory-core/src/short-term-promotion.ts` | 65-104 |
| promotion 评分公式 | `extensions/memory-core/src/short-term-promotion.ts` | 1211-1330 |
| promotion lock 文件 | `extensions/memory-core/src/short-term-promotion.ts` | 27-44 |
| memory tools | `extensions/memory-core/src/tools.ts` | 238、402 |
| hybrid retrieval 默认 | `src/agents/memory-search.ts` | 103-117 |
| chunk 实现 | `packages/memory-host-sdk/src/host/internal.ts` | 362-419 |
| FTS5 + sqlite-vec schema | `packages/memory-host-sdk/src/host/memory-schema.ts` | 43-66 |
| sqlite-vec 加载 | `packages/memory-host-sdk/src/host/sqlite-vec.ts` | 10-50 |
| preemptive compaction | `src/agents/pi-embedded-runner/run/preemptive-compaction.ts` | 11-119 |
| plugin hooks | `docs/concepts/agent-loop.md` | 89-115 |

## 生命周期详表

| 维度 | 观察 |
|---|---|
| 主要记忆载体 | `MEMORY.md`、`memory/YYYY-MM-DD.md`、`DREAMS.md`、`memory/.dreams/`、可选 wiki vault |
| 存储位置 | agent workspace，默认 `~/.openclaw/workspace`；sqlite 索引默认 `<state>/memory/<agentId>.sqlite`（`memory-search.ts:142-149`） |
| 加载路径 | `MEMORY.md` 在每个 DM session start 加载；today/yesterday daily notes 自动加载；更多历史通过 tools 搜索/读取 |
| 工具路径 | `memory_search` 做 broad/semantic recall；`memory_get` 精确读取文件或行范围 |
| 后台召回 | `active-memory` 在主回复前 blocking subagent，输出紧凑 summary 或 `NONE` |
| 长度限制 | 单个 `MEMORY.md` 无公共硬限制；实际由上下文预算、chunk、active-memory 输出上限、tool timeout 与 compaction 控制 |
| active-memory summary 上限 | 220 chars（`index.ts:30`）；可调范围 40-1000（line 833） |
| active-memory turn 摘要 | user 220 chars（line 33）、assistant 180 chars（line 34） |
| active-memory timeout | 默认 15,000 ms（line 28）；最低 250 ms（line 38） |
| active-memory partial transcript | 32,000 chars（line 47） |
| transcript read | max 2,000 lines、50 MB（line 48-49） |
| search query | max 480 chars（line 51） |
| recall cache | max 1,000 entries（line 36） |
| circuit breaker | 连续 3 次超时（line 43）打开，跳过后续 turn |
| 默认 chunk | 400 tokens × 80 overlap（`memory-search.ts:103-104`）|
| hybrid 检索 | vector 0.7 + text 0.3，候选 4×top-K，top-K 默认 6，min score 0.35（`memory-search.ts:108-117`）|
| MMR 多样化 | 默认 disabled，lambda 0.7（line 114-115）|
| 时间衰减 | 默认 disabled，half life 30 天（line 116-117）|
| embedding context | 默认 4096 tokens（`embeddings.types.ts:45`）|
| multimodal 上限 | 10 MB / 文件（`multimodal.ts:26`）|
| 超出处理 | session 接近 context window 时 auto-compaction；compaction 前可运行 silent memory flush turn |
| 整理方式 | Dreaming light/REM/deep 三阶段；memory-wiki 离线编译 |
| 定时任务 | Dreaming opt-in，默认 disabled；启用后 `memory-core` auto-manages cron job，默认 `0 3 * * *`（`openclaw.plugin.json:21`）|
| promotion 阈值 | min score 0.75、min recall count 3、min unique queries 2（`short-term-promotion.ts:24-26`），可配 max age days |
| promotion 锁 | `memory/.dreams/short-term-promotion.lock`（`short-term-promotion.ts:32`），避免并发覆写 `MEMORY.md` |
| 安全边界 | transcript ingestion 会 redaction；Dream Diary/report artifacts 不作为 promotion source；长期 promotion 仅写 `MEMORY.md` |

## 文件层级

OpenClaw 的 memory 文件非常接近 Mnemon 讨论中的 Markdown-first 形态：

```text
workspace/
  MEMORY.md
  DREAMS.md
  memory/
    YYYY-MM-DD.md
    .dreams/
      short-term-promotion.lock
      <phase>-state.json
    dreaming/<phase>/YYYY-MM-DD.md
  AGENTS.md / SOUL.md / TOOLS.md / IDENTITY.md / USER.md / ...
```

关键区别：OpenClaw 不把所有 Markdown 都直接放进 context。`MEMORY.md` 是长期 root，daily notes 是短期工作记忆，历史通过 `memory_search` 与 `memory_get` 按需进入上下文。

## Dreaming 流程详解

Dreaming 是 OpenClaw 的核心记忆巩固机制，三阶段实现位于 `dreaming-phases.ts` 与 `dreaming.ts`。

### 阶段总览

| 阶段 | 读取 | 写入 | promotion |
|---|---|---|---|
| Light | recent daily memory、recall traces、redacted transcripts | candidate lines、phase signals | 否 |
| REM | short-term traces、theme signals | `DREAMS.md` 的反思块 | 否 |
| Deep | staged candidates、recall evidence、phase reinforcement | promoted entries 到 `MEMORY.md` | 是 |

### 阶段实现细节

**Light**（`dreaming-phases.ts:1601-1670`）使用 `LIGHT_SLEEP_EVENT_TEXT = "__openclaw_memory_core_light_sleep__"`（line 74）作为 internal session marker。它聚合 recall 信号，把候选 line 写入 daily file 的 `<!-- openclaw:dreaming:light:start --> ... <!-- openclaw:dreaming:light:end -->` 块（line 103-104），随后调用 `recordDreamingPhaseSignals` 累积 lightHits。

**REM**（`dreaming-phases.ts:1691-1751`）使用 `REM_SLEEP_EVENT_TEXT`（line 75）作为 marker。它从最近的 memory traces 中抽取主题，过滤 `REM_REFLECTION_TAG_BLACKLIST`（line 203，含 `assistant/user/system/subagent/the`）后生成反思块，写入 daily file 的 `## REM Sleep`（line 107）以及 `DREAMS.md` 的 dream diary。narrative prompt 由 `dreaming-narrative.ts` 生成。

**Deep**（`dreaming.ts:534-672`）是唯一写 `MEMORY.md` 的阶段。流程：

1. 读取 short-term recall store（`short-term-promotion.ts:65-104` 定义 `ShortTermRecallStore`）。
2. 对每个 entry 计算 score：
   ```text
   score = 0.30 * relevance(avgRecallScore)
         + 0.24 * frequency(log1p(signalCount)/log1p(10))
         + 0.15 * diversity(uniqueQueries / recallDays)
         + 0.15 * recency(halfLife=14 day)
         + 0.10 * consolidation(light/REM 强化)
         + 0.06 * conceptual(concept-vocabulary 命中)
   ```
3. 三重 gate：`score >= 0.75` AND `recallCount >= 3` AND `uniqueQueries >= 2`，可选 `ageDays <= maxAge`。
4. 取 promotion lock（line 32 的 `.lock` 文件，超时 timeout）。
5. append 到 `MEMORY.md`，注释 `<!-- openclaw-memory-promotion:... -->` 标记 provenance（line 27、282）。
6. 释放 lock，记录 `promotedAt`。
7. 生成 deep phase 的 narrative 写入 `DREAMS.md`。

dreaming controller（`dreaming.ts:233-409`）从 `cron` 服务读取已注册 job（line 233），如发现 legacy phase job 则 `migrate`（line 247-258），统一切换到 unified controller，避免重复执行。`isolated heartbeat`（line 365）允许 cron 在 sibling `:heartbeat` session 跑，避免污染主会话。

### Dreaming 失败模式

- 单 workspace 失败被记录但不影响其他 workspace（`dreaming.ts:667`）；
- 缺少 `cron` 服务时不抛错，整个 dreaming 关闭（`dreaming.ts:342-351`）；
- promotion lock 被持有时阻塞至 timeout；
- `limit=0` 跳过整个 promotion（line 539）。

## 检索 pipeline 详解

`memory_search` 的 hybrid 实现（`memory-search.ts:75-117、290-380`）：

```text
chunk     = chunkMarkdown(content, {tokens: 400, overlap: 80})
candidates = top(4 × maxResults) by combined score
combined  = normalizedVectorWeight * vec(chunk, query) + textWeight * fts5(chunk, query)
if mmr.enabled:
  re-rank by lambda * relevance - (1-lambda) * maxSimToSelected
if temporalDecay.enabled:
  combined *= 0.5 ^ (ageDays / halfLifeDays)
filter by combined >= 0.35
return top(6)
```

vector / text 权重在加和不为 1 时归一化（line 320-322）。`vectorWeight + textWeight = 1` 的设计与社区 hybrid retrieval 经验一致：纯向量易漏低频专有名词，纯 BM25 易漏语义近义。

底层存储：FTS5 虚拟表 + sqlite-vec extension。schema 由 `memory-schema.ts:43-66` 创建，包括 `embeddingCacheTable`（`memory-schema.ts:43-55`）允许命中重复内容跳过 embedding 调用。

## 超出与 compaction 处理

`preemptive-compaction.ts:41-119` 在 prompt 提交前估算 token 用量。决策路由（line 100-108）：

| 路由 | 触发条件 |
|---|---|
| `fits` | overflow ≤ 0 |
| `compact_only` | overflow > 0，无可削减的 tool result |
| `truncate_tool_results_only` | tool result 可削减 ≥ 1.5 × overflow + buffer |
| `compact_then_truncate` | 介于两者之间 |

`SAFETY_MARGIN`（`compaction.ts`）在估算时乘上保险系数；`MIN_PROMPT_BUDGET_TOKENS` 与 `MIN_PROMPT_BUDGET_RATIO`（`pi-compaction-constants.ts`）保证 reserve 不会吃掉所有 prompt 空间。

无法削减时抛出 `Context overflow: prompt too large for the model (precheck).`（line 11）。

OpenClaw 对上下文超出的策略：

1. session 接近上下文窗口或 provider 返回 overflow；
2. 走 preemptive route，决定 compact / truncate / 混合；
3. compaction 前可运行 silent memory flush turn，提醒 agent 把关键 durable context 写入 memory files；
4. 使用 compacted context retry 原请求；
5. 原始 conversation 仍保留在磁盘，compaction 只影响下一次模型上下文。

这点对 Mnemon 非常重要：memory hook 不应只在 turn end 运行，也应有 pre-compact / pre-stop 的「连续性捕获」职责。

## 定时与后台任务

OpenClaw 中两类后台能力：

- **active-memory**：主回复前的同步/阻塞召回，适合在每轮回答前补上下文；
- **dreaming**：启用后由 cron 定期运行 full sweep，默认每天 03:00（`openclaw.plugin.json:21`）。controller 自动迁移 legacy phase job，统一为单一 dreaming job。

Mnemon 第一阶段不应做长期驻留 scheduler。更好的做法是让 INSTALL 文档说明：如果目标 agent 支持 scheduled tasks，可以可选安装一个「weekly memory review」或「pre-compact save」任务；默认只依赖 hooks 与手动命令。

## 失败模式总览

| 故障点 | OpenClaw 行为 |
|---|---|
| active-memory 超时 | 返回 `timeout` / `timeout_partial`，连续 3 次开启 circuit breaker 跳过 |
| partial transcript 截断 | summary 返回 partial 标记，下一 turn 可 retry，且 `not persisted`（`index.ts:1362`） |
| compaction 拒绝 | overflow 不可削减时抛 precheck 错误，由上层退化或重试 |
| dreaming 单 workspace 失败 | 仅记录日志，不影响其他 workspace |
| promotion lock 超时 | 抛 `Timed out waiting for short-term promotion lock`（line 748） |
| sqlite-vec 缺失 | 给出 hint：`Set agents.defaults.memorySearch.store.vector.extensionPath`（`sqlite-vec.ts:12`） |
| embedding provider 不可用 | 退化为纯 FTS5，hybrid 仍工作 |

## 对 Mnemon 的具体启发

可借鉴：

- 采用 `NONE` gate：没有相关记忆时明确不注入，避免噪音。
- 把 daily notes、long-term facts、review diary 分开。
- 在 compaction 前保存关键状态。
- promotion 必须有 evidence、recency、frequency 或用户确认。
- 用 lock 文件避免后台任务并发改写 root memory。
- preemptive compaction 路由：先看 tool result 能否截断，再考虑全量 compaction。

值得警惕的过度工程化：

- 三阶段 dreaming + cron 调度，第一阶段 Mnemon 用户负担过大。
- 五种 prompt style + circuit breaker + cache，runtime 太多状态。
- FTS5 + sqlite-vec + reindex state 是 indexer 工程，建议 Mnemon 让具体 agent 自己接（CLI 提供 markdown / sqlite store 的简单形态即可）。
- memory wiki 的 claim health、freshness、contradiction 分析在 review 流程中真实有用，但实现成本高，应作为 v2+ 选项。

## 参考来源

- 官方文档: [OpenClaw Memory Overview](https://docs.openclaw.ai/concepts/memory)
- 官方文档: [OpenClaw Dreaming](https://docs.openclaw.ai/concepts/dreaming)
- 官方文档: [OpenClaw Compaction](https://docs.openclaw.ai/concepts/compaction)
- 官方文档: [OpenClaw Active memory](https://docs.openclaw.ai/concepts/active-memory)
- 本地源码: `/tmp/mnemon-agent-research-sources/openclaw/extensions/active-memory/index.ts`
- 本地源码: `/tmp/mnemon-agent-research-sources/openclaw/extensions/memory-core/src/dreaming.ts`
- 本地源码: `/tmp/mnemon-agent-research-sources/openclaw/extensions/memory-core/src/dreaming-phases.ts`
- 本地源码: `/tmp/mnemon-agent-research-sources/openclaw/extensions/memory-core/src/short-term-promotion.ts`
- 本地源码: `/tmp/mnemon-agent-research-sources/openclaw/extensions/memory-core/src/tools.ts`
- 本地源码: `/tmp/mnemon-agent-research-sources/openclaw/src/agents/memory-search.ts`
- 本地源码: `/tmp/mnemon-agent-research-sources/openclaw/src/agents/pi-embedded-runner/run/preemptive-compaction.ts`
- 本地源码: `/tmp/mnemon-agent-research-sources/openclaw/packages/memory-host-sdk/src/host/internal.ts`
- 本地源码: `/tmp/mnemon-agent-research-sources/openclaw/packages/memory-host-sdk/src/host/memory-schema.ts`
- 本地源码: `/tmp/mnemon-agent-research-sources/openclaw/packages/memory-host-sdk/src/host/sqlite-vec.ts`
