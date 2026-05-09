# OpenClaw 架构观察

## 一句话结论

OpenClaw 是本次调研中最重工程化的 agent runtime：它有 plugin SDK、workspace bootstrap、tool registry、memory slot、active-memory 子 agent、memory wiki、dreaming consolidation、compaction hooks。它适合作为能力上限参考，但不适合作为 Mnemon 第一阶段的实现模板。

## 源码地图

本地源码快照：`/tmp/mnemon-agent-research-sources/openclaw`

| 主题 | 文件 | 关键行 |
|---|---|---|
| Plugin hook 列表 | `docs/concepts/agent-loop.md` | 89-115 |
| 默认 chunk 常量 | `src/agents/memory-search.ts` | 103-104 |
| hybrid 检索权重 | `src/agents/memory-search.ts` | 108-117 |
| memory tools 注册 | `extensions/memory-core/src/tools.ts` | 238、402 |
| memory-core dreaming controller | `extensions/memory-core/src/dreaming.ts` | 50-172、534-672 |
| dreaming 三阶段实现 | `extensions/memory-core/src/dreaming-phases.ts` | 74-107、1601-1751 |
| promotion 评分权重 | `extensions/memory-core/src/short-term-promotion.ts` | 56-63、1280-1289 |
| promotion 阈值 | `extensions/memory-core/src/short-term-promotion.ts` | 24-26 |
| active-memory 限制 | `extensions/active-memory/index.ts` | 28-51 |
| active-memory prompt style | `extensions/active-memory/index.ts` | 97-103、909-928 |
| sqlite-vec 加载 | `packages/memory-host-sdk/src/host/sqlite-vec.ts` | 10-50 |
| FTS5 schema | `packages/memory-host-sdk/src/host/memory-schema.ts` | 43-66 |
| chunkMarkdown 实现 | `packages/memory-host-sdk/src/host/internal.ts` | 362-419 |
| multimodal 文件上限 | `packages/memory-host-sdk/src/host/multimodal.ts` | 23-56 |
| 默认 cron 表达式占位 | `extensions/memory-core/openclaw.plugin.json` | 21 |
| preemptive compaction | `src/agents/pi-embedded-runner/run/preemptive-compaction.ts` | 11-119 |

## 架构层次详解

OpenClaw 的运行时不是单层 plugin，而是四个分工明确的子系统协作：

```text
┌─────────────────────────────────────────────────────────────┐
│ channel / UI / gateway                                      │
│   ↓                                                          │
│ agent session（pi-embedded-runner）                          │
│   ↓ before_prompt_build hook                                 │
│ ┌─────────────┐    ┌───────────────────────────────────┐     │
│ │ active-     │ →  │ memory-core (memory_search /       │     │
│ │ memory      │    │ memory_get tools, FTS+vector)      │     │
│ │ subagent    │    └───────────────────────────────────┘     │
│ └─────────────┘                ↑              ↑              │
│   ↓ summary or NONE            │              │              │
│ prompt build                   │              │              │
│   ↓                            │              │              │
│ LLM + tools (memory_get etc.)  │              │              │
│   ↓                            │              │              │
│ before_compaction hook ─ silent flush turn → 写 MEMORY.md     │
│   ↓                                                           │
│ session_end → short-term recall store                        │
│                                                                │
│ 后台 cron (memory-core 自管):                                 │
│   light → REM → deep dreaming → 候选 promotion → MEMORY.md    │
│                                                                │
│ 离线编译:                                                     │
│   memory-wiki: 把 MEMORY.md / sessions 编译成 vault           │
│                claims、freshness、contradiction、provenance    │
└─────────────────────────────────────────────────────────────┘
```

四层职责：

1. **memory-core**：file-backed memory backend、FTS5+sqlite-vec 混合检索、chunkMarkdown、`memory_search` 与 `memory_get` 工具、short-term recall 簿记、dreaming controller、cron 注册。位置 `extensions/memory-core/src/`。
2. **active-memory**：在主回复之前作为 blocking subagent 运行，仅调用 memory tools，输出紧凑 summary 或字面 `NONE`。位置 `extensions/active-memory/index.ts`。
3. **memory-wiki**：把 `MEMORY.md`、daily memory、session transcripts 编译成 wiki vault，带 claim、freshness、contradiction、provenance。位置 `extensions/memory-wiki/src/`。
4. **dreaming**：light/REM/deep 三阶段巩固。light/REM 写 daily 与 `DREAMS.md`，deep 评分排名后 append 到 `MEMORY.md`。位置 `extensions/memory-core/src/dreaming-phases.ts`。

四层之间的数据流：active-memory 通过 memory-core 的 tools 访问数据；memory-core 在 turn 结束写 short-term recall 簿记；dreaming 读取该簿记并产生 promotion 候选；memory-wiki 单独从磁盘读 markdown，不参与 hot path。

## Dreaming 流程速览

dreaming 是 OpenClaw 最有特色的子系统，详细流程见第 03 篇。简述如下：

- **light**（`dreaming-phases.ts:1601-1670`）：每日聚合短期 recall 信号，写入 daily file 的 `<!-- openclaw:dreaming:light:start/end -->` 块；不动 `MEMORY.md`。light 阶段只做「记录候选」。
- **REM**（`dreaming-phases.ts:1691-1751`）：在 daily file 与 `DREAMS.md` 写反思块（`## REM Sleep`），过滤无意义 tag；只做「主题关联」。
- **deep**（`dreaming.ts:534-672`）：按 6 维评分（relevance 0.30 / frequency 0.24 / diversity 0.15 / recency 0.15 / consolidation 0.10 / conceptual 0.06），通过三重 gate（score≥0.75、recall≥3、unique queries≥2）后 append 到 `MEMORY.md`，唯一会写 root memory 的阶段。

每阶段都有 narrative prompt（`dreaming-narrative.ts`）生成可读的 review 文本，写到 `DREAMS.md`。这让长期演化可被人审查、可被回滚。

## 检索 pipeline 速览

`memory_search` 不是单纯向量查询，而是 hybrid pipeline：

```text
chunk(400/80)
  → 候选生成（4 × top-K）
  → vector(0.7) + BM25/FTS5(0.3) 融合
  → 可选 MMR 多样化（lambda=0.7，默认 disabled）
  → 可选时间衰减（halfLife=30d，默认 disabled）
  → 阈值过滤（>0.35）
  → top-6
```

vector / text 权重在加和不为 1 时归一化。底层用 sqlite FTS5 + sqlite-vec 扩展，schema 在 `packages/memory-host-sdk/src/host/memory-schema.ts:43-66`。embedding 命中 cache 时跳过外部调用，节省成本。详细参数与公式见第 02 篇「检索 pipeline」章节。

## Plugin hook 模型

OpenClaw 公开两类挂钩点。Gateway hooks（`agent:bootstrap`、`/new` `/reset` `/stop` 等命令事件）面向 shell 集成与 workspace 级自动化；plugin hooks 面向 agent loop。memory-core 与 active-memory 都是基于 plugin hooks 实现：

- active-memory 在 `before_prompt_build` 注入 recall summary；
- memory-core 在 `before_compaction` 触发 silent flush，把待固化的 context 写到 daily memory；
- memory-core 在 `session_end` 更新 short-term recall store；
- memory-core 在 `gateway_start` 注册 dreaming cron job；
- memory-wiki 在 `before_prompt_build` 注入 wiki prompt section（如启用）。

这给 Mnemon 的提示是：`mnemon` CLI 只需暴露与这些 hook 等价的轻量挂钩点（pre-compact、pre-stop、user-prompt-submit、post-tool），具体 agent 怎么调度由 harness 决定。

## Workspace Markdown Bootstrap

OpenClaw 文档 `docs/concepts/system-prompt.md` 显示 bootstrap 会识别固定文件名：

- `AGENTS.md`
- `SOUL.md`
- `TOOLS.md`
- `IDENTITY.md`
- `USER.md`
- `HEARTBEAT.md`
- `BOOTSTRAP.md`
- `MEMORY.md`

`memory/*.md` daily files 不属于普通 bootstrap context，通常通过 `memory_search` 与 `memory_get` 按需访问。这是 OpenClaw 的关键边界：稳定规则自动进 prompt，长期记忆按需检索。

## Memory 多层栈

OpenClaw 的 memory 至少分五层：

1. **root memory**：`MEMORY.md` 表达 long-term durable facts，每个 DM session 启动时载入。
2. **daily memory**：`memory/YYYY-MM-DD.md`，按需 search/get。
3. **active-memory**：在主回复前运行 bounded sub-agent，只允许 memory tools。
4. **memory-wiki**：把 durable memory 编译成 wiki vault，支持 claims、dashboard、provenance。
5. **dreaming**：后台 consolidation，把强短期信号推广到 `MEMORY.md`，输出 `DREAMS.md` 与 phase reports。

这已经超过「memory tool」范畴，是完整 memory runtime。

## Hook 模型

OpenClaw 有两类 hook：内部 gateway hooks（`agent:bootstrap`、command hooks 如 `/new` `/reset` `/stop`）与 plugin hooks（在 agent loop 内）。plugin hooks 来自 `docs/concepts/agent-loop.md:89-115`：

| Hook | 触发时机 | memory plugin 用途 |
|---|---|---|
| `before_model_resolve` | session 加载前 | 切换 provider |
| `before_prompt_build` | session 加载后、prompt 提交前 | 注入 active-memory recall、prompt section |
| `before_agent_reply` | 内联动作之后、LLM 调用之前 | 短路 turn 用合成回复 |
| `before_compaction` / `after_compaction` | compaction 前后 | silent flush、补注 |
| `before_tool_call` / `after_tool_call` | 工具调用前后 | 拦截 memory tool 参数 |
| `tool_result_persist` | 工具结果写入 transcript 前 | 同步变换 |
| `agent_end` | 完成后 | 检查最终消息列表 |
| `session_start` / `session_end` | session 边界 | dreaming sweep 触发 |
| `gateway_start` / `gateway_stop` | gateway 生命周期 | cron 注册 |

`before_tool_call`、`before_install`、`message_sending` 的 `block` / `cancel` 是终端语义：true 终结后续 handler，false 不清除上一个 block。

这证明 Mnemon 的四 phase hook（pre-compact、pre-stop、post-tool、user-prompt-submit）是合理的，但也警告：hook 太重会让系统复杂度快速上升。

## 失败模式

- **active-memory 超时**：`extensions/active-memory/index.ts:28` 默认 15s timeout，超过后返回 `timeout`、`timeout_partial` 或 `unavailable`。连续 3 次超时打开 circuit breaker（line 43），后续 turn 跳过 recall。
- **partial transcript 截断**：超过 32,000 chars 触发 partial 模式（line 47），下一个 turn 仍可 retry。
- **compaction 拒绝**：preemptive route 包括 `compact_only`、`truncate_tool_results_only`、`compact_then_truncate`、`fits` 四种（`preemptive-compaction.ts:100-108`）；overflow 无法削减时仍可能抛 `Context overflow: prompt too large for the model (precheck).`（line 11）。
- **dreaming 失败**：单个 workspace 失败被记录（`dreaming.ts:667`），不影响其他 workspace；migration 错误也被独立日志（line 247）。
- **promotion lock**：`short-term-promotion.ts:32` 有 `.dreams/short-term-promotion.lock`，避免并发改写 `MEMORY.md`。

## 对 Mnemon 的具体启发

可吸收：

- 固定 Markdown bootstrap 文件名与「root memory 自动载入、daily 按需检索」的二分法。
- `memory_search` / `memory_get` 工具分离：broad recall 与精确读取使用不同 tool。
- active recall 的 bounded 输出与 `NONE` gate（无相关时不注入噪音）。
- compaction 前 silent flush，把关键连续性沉淀到 markdown。
- promotion lock 文件，避免并发改写 long-term memory。
- circuit breaker：连续超时跳过非关键路径。

不应照搬：

- 多 memory plugin slot（runtime 级抽象）。
- wiki compiler（freshness、contradiction、claim health 等离线分析）。
- dreaming cron 与三阶段 phase engine。
- 大型 plugin SDK（`packages/plugin-sdk` 与 `memory-host-sdk` 都是独立 npm 包）。
- runtime 内部嵌入完整 memory engine（FTS5 + sqlite-vec + 嵌入 cache + reindex state）。

Mnemon 第一阶段更适合先做可安装 Markdown harness：把 heavy capabilities 留作未来可选层，Mnemon CLI 自身保留简洁 API。

## 参考来源

- 本地源码: `docs/concepts/agent-loop.md`
- 本地源码: `docs/concepts/memory.md`
- 本地源码: `docs/concepts/dreaming.md`
- 本地源码: `extensions/memory-core/`
- 本地源码: `extensions/active-memory/`
- 本地源码: `extensions/memory-wiki/`
- 本地源码: `packages/memory-host-sdk/`
- 官方/公开文档: [Active memory](https://docs.openclaw.ai/concepts/active-memory)
- 官方/公开文档: [Memory overview](https://docs.openclaw.ai/concepts/memory)
- 官方/公开文档: [Dreaming](https://docs.openclaw.ai/concepts/dreaming)
