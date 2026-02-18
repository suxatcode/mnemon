# 4. 与 MAMGA/memcp 实现的对比分析

## 4.1 三方概况

| 维度 | MAMGA (官方实现) | memcp | Mnemon |
|------|-----------------|-------|--------|
| **定位** | 学术研究 benchmark | Claude Code MCP 插件 | 独立记忆守护进程 |
| **语言** | Python | Python 3.10+ | Go |
| **存储** | NetworkX MultiDiGraph + FAISS (内存) → JSON 持久化 | SQLite (WAL) + 文件系统 | SQLite (WAL) |
| **LLM 依赖** | gpt-4o-mini (必需, 嵌入管道) | Claude Haiku/Sonnet 子代理 (可选) | CLI 引擎层 LLM (Claude Code 等, 外置于二进制) |
| **Embedding** | all-MiniLM-L6-v2 (384d) / OpenAI text-embedding-3-small | Model2Vec (256d) / FastEmbed (384d) | Ollama nomic-embed-text (768d) |
| **向量存储** | FAISS IndexFlatL2 | numpy .npz brute-force cosine | SQLite BLOB brute-force cosine |
| **接口** | Python API | MCP (21 tools) | CLI (11 commands) |
| **外部依赖** | OpenAI API Key 必需 | 核心仅 mcp+pydantic | 二进制零外部依赖 (纯 Go)，LLM 能力由引擎层 CLI 提供 |
| **测试** | LoCoMo/LongMemEval | 341 tests + 77 benchmarks | 125 e2e assertions |
| **生产就绪** | 否 (研究代码) | 部分 (MCP 插件限制) | 是 (单二进制可部署) |

## 4.2 写入管道对比

### 内容提取

| | MAMGA | memcp | Mnemon |
|---|-------|-------|--------|
| **主提取** | LLM (gpt-4o-mini, structured JSON, 3 retries) | regex + 可选 LLM 子代理 (Haiku) | regex + 字典 (140+ 术语) |
| **提取内容** | entities, topic, dates, summary, semantic_facts, relationships, activities, context_keywords | entities (files, modules, URLs, CamelCase) | entities (CamelCase, ALLCAPS, paths, URLs, @mention, 中文, 字典) |
| **LLM 增强** | 核心路径 (必需, 管道内) | 可选 (entity-extractor 子代理) | 引擎层 (Claude `--entities` flag, Opus/Sonnet 级别) |
| **成本** | 每条 ~$0.001 (gpt-4o-mini) | 可选 ~$0.0003 (Haiku) | 二进制层 $0，LLM 成本由引擎层承担（已含在 Claude 会话中） |

### 边生成

| 边类型 | MAMGA | memcp | Mnemon |
|--------|-------|-------|--------|
| **Temporal** | backbone 链 + 24h proximity (lookahead=10, w=1/(1+hours)) | 30min 窗口 (w=max(0.1, 1-delta_min/30), top-20) | backbone 链 + 24h proximity (max 10, w=1/(1+hours)) |
| **Semantic** | 种子 top-3 cosine > 0.5 | cosine >= 0.3 或 keyword overlap >= 0.1, top-3 | auto cosine >= 0.50 max 3, 候选 >= 0.30 |
| **Causal** | LLM 推断 (slow path, 2-hop BFS → gpt-4o-mini) | regex pattern + token overlap >= 3 tokens (ratio >= 0.15), max 1 | regex + token overlap >= 0.15 + 方向推断 + sub_type |
| **Entity** | LLM 提取 → 共现 (SAME_ENTITY conf=0.9) | 精确匹配 (case-insensitive), weight=1.0 | 共现匹配, max 5/entity |

### 异步处理

| | MAMGA | memcp | Mnemon |
|---|-------|-------|--------|
| **双流机制** | Fast Path (同步) + Slow Path (异步 consolidation_queue) | 同步写入 | 同步写入 |
| **异步因果** | ✅ 2-hop BFS → LLM 推断 | ❌ | ❌ |
| **异步整合** | ✅ episode 分割, session 摘要 | ❌ | ❌ |

## 4.3 查询管道对比

### 搜索层

| 搜索方式 | MAMGA | memcp | Mnemon |
|----------|-------|-------|--------|
| **向量搜索** | FAISS L2, k=15-30 | numpy cosine brute-force | SQLite BLOB cosine brute-force |
| **关键词搜索** | bigram index + threshold=0.3 | 5 层：keyword → BM25 → fuzzy → semantic → hybrid | token overlap scoring |
| **全扫描** | 是 (20-40) | 否 | 否 |
| **融合** | RRF (k=60) | hybrid alpha=0.6 | RRF (k=60) |

### 图遍历

| 维度 | MAMGA | memcp | Mnemon |
|------|-------|-------|--------|
| **算法** | BFS (主) + beam search (存在但未启用) | BFS | beam search |
| **BFS 参数** | sim_threshold 0.10-0.30, max_depth 3-12, max_nodes 500-800 | 简单 BFS 遍历 | — |
| **Beam 参数** | beam=10, max_visited=50, λ₁=0.6, λ₂=0.4 | — | beam=10-15, maxVisited=400-500, λ₁=1.0, λ₂=0.4 |
| **Intent 自适应** | 8 种查询类型, per-type 参数 | 4 种 (why/when/who/what) | 4 种 (WHY/WHEN/ENTITY/GENERAL) |
| **Intent 权重** | WHY: {CAUSAL:0.7, TEMPORAL:0.2} | total_score = keyword×0.7 + edge_boost×0.3 | WHY: {causal:0.70, temporal:0.20, entity:0.05, semantic:0.05} |

### Reranking

| 维度 | MAMGA | memcp | Mnemon |
|------|-------|-------|--------|
| **因子** | keyword, entity, temporal, phrase, similarity, person boost | keyword×0.7 + edge_boost×0.3 | keyword, entity, similarity, graph |
| **特殊** | similarity×10 放大, person_boost=20.0 | — | 有/无 embedding 两组权重 |
| **因果排序** | 无 | 无 | WHY → Kahn 拓扑排序 |

### 高级查询功能

| 功能 | MAMGA | memcp | Mnemon |
|------|-------|-------|--------|
| **Multi-hop 分解** | ✅ LLM 分解子问题 → 合成 | ❌ | ❌ (Claude 外部分解) |
| **Best-of-N** | ✅ N=3 答案选择 | ❌ | ❌ |
| **Intent override** | ❌ | ❌ | ✅ --intent flag |
| **Signals 元数据** | ❌ | ❌ | ✅ 每结果 signals 字段 |
| **Sparse hint** | ❌ | ❌ | ✅ hint="sparse_results" |

## 4.4 生命周期管理对比

| 维度 | MAMGA | memcp | Mnemon |
|------|-------|-------|--------|
| **衰减模型** | 无 | 3-zone (Active→Archive@30d→Purge@180d) + importance decay (half-life 30d) | continuous EI decay (half-life 30d) |
| **衰减公式** | — | base×log(1+access)×decay | base×log(1+access)×0.5^(days/30)×(1+0.1×edges) |
| **免疫规则** | — | critical/high importance 或 access>=3 或 pinned tags | importance>=4 或 access>=3 |
| **自动清理** | ❌ | ✅ auto-prune at 10000 | ✅ auto-prune at 1000 |
| **手动 GC** | ❌ | ✅ retention_preview + retention_run + restore | ✅ gc --threshold / --keep |
| **软删除** | ❌ (内存图) | ✅ (3-zone) | ✅ (deleted_at 时间戳) |

## 4.5 LLM 角色对比（关键差异）

这是三个实现之间最本质的区别。

### MAMGA：LLM-in-the-loop（LLM 在管道内部）

```
用户输入 → [管道内: LLM 提取] → 存入图 → [管道内: LLM 因果推断]
查询 → 搜索+遍历 → [管道内: LLM 分解问题] → [管道内: LLM 生成答案 ×3] → 选最佳
```

- LLM 是管道的**必要组成部分**，没有 LLM 管道无法运行
- 每次写入至少 1 次 LLM 调用（提取），可能 2 次（+因果推断）
- 每次查询至少 1 次 LLM 调用（答案生成），复杂查询 4+ 次（分解+多答案+选择）
- LLM 选择受限于管道设计（gpt-4o-mini，参数固定）

### memcp：LLM-as-subagent（LLM 作为子代理）

```
Claude 主会话 ←→ MCP 工具调用
                   ↓
              memcp 服务端
                   ↓
            [可选: LLM 子代理]
            · Analyzer (Haiku)
            · Mapper (Haiku ×N)
            · Synthesizer (Sonnet)
            · Entity-Extractor (Haiku)
```

- LLM 子代理是**可选**的，核心管道可以无 LLM 运行
- 子代理使用 Haiku（成本低 1/60），仅在需要时调用
- MCP 协议限制：memcp 作为 Claude Code 插件，生命周期绑定到 Claude 会话
- PreCompact hook 确保 context compact 前保存

### Mnemon：CLI-in-the-loop（CLI 就是 LLM 引擎）

```
┌─────────────────────────────────────────────┐
│  引擎层 — Claude (Opus/Sonnet)               │
│  · 实体补充 (--entities)                      │
│  · 因果评估 (candidates → link)               │
│  · 语义判断 (candidates → link)               │
│  · 意图覆盖 (--intent)                        │
│  · 多步分解 (多次 recall)                     │
│  · Signals 复判 (keyword/entity/sim/graph)    │
│         ↕ CLI 调用 (stdin/stdout JSON)        │
├─────────────────────────────────────────────┤
│  自动化层 — mnemon 二进制                      │
│  · regex + 字典实体提取                        │
│  · 因果关键词检测 + token 重叠                 │
│  · RRF + beam search + reranking              │
│  · 生命周期管理 (EI decay, auto-prune)         │
└─────────────────────────────────────────────┘
```

- CLI（Claude Code）**就是 mnemon 的 LLM 引擎**，通过 CLI 调用驱动所有需要 LLM 能力的操作
- mnemon 二进制处理高频低成本的自动化计算（regex、图遍历、向量搜索），零外部依赖
- 引擎层 LLM 处理高价值判断（因果评估、语义链接、实体补充），能力远超 gpt-4o-mini
- Signals 元数据让引擎层 LLM 能做出比管道内部 LLM 更好的判断（有完整对话上下文）
- 引擎层可替换：当前用 Claude，可切换到 GPT / Gemini / 本地模型

### 三种模式的权衡

| 维度 | MAMGA (LLM-in) | memcp (LLM-subagent) | Mnemon (CLI-in-loop) |
|------|----------------|---------------------|---------------------|
| **图质量** | 高（管道内 gpt-4o-mini 提取+推断） | 中高（可选 LLM + regex） | 高（二进制自动化 + 引擎层 Opus/Sonnet 补充评估） |
| **LLM 能力** | gpt-4o-mini（管道内，固定参数） | Haiku/Sonnet（可选子代理） | Opus/Sonnet（引擎层，完整上下文，能力最强） |
| **写入延迟** | 高（每次管道内 LLM 调用 ~1-3s） | 中低（regex 为主） | 最低（二进制层纯计算 ~10ms） |
| **写入成本** | ~$0.001-0.003/条（额外 API） | ~$0-0.001/条（额外 API） | 二进制层 $0（LLM 成本已含在引擎会话中） |
| **查询质量** | 高（管道内 LLM 参与） | 中（搜索为主） | 最高（Signals 辅助引擎层 Claude 判断，有完整上下文） |
| **引擎可替换性** | 绑定 OpenAI | 绑定 Claude (MCP) | 引擎层可替换（Claude / GPT / Gemini / 本地） |
| **可部署性** | 研究代码 | Python + MCP 环境 | 单二进制 |

## 4.6 存储架构对比

| 维度 | MAMGA | memcp | Mnemon |
|------|-------|-------|--------|
| **图存储** | NetworkX MultiDiGraph (内存) | SQLite `graph.db` | SQLite `mnemon.db` |
| **持久化** | JSON 序列化到磁盘 | SQLite WAL + 文件系统 | SQLite WAL |
| **向量索引** | FAISS IndexFlatL2 (内存) | numpy .npz | SQLite BLOB |
| **并发** | 单进程 | 单进程 (MCP server) | WAL 支持多读单写 |
| **容量** | 内存受限 | 10000 insight 上限 | 1000 insight 上限 |
| **文件管理** | 无 | 文件系统 (~/.memcp/) chunk/context | 无 |

### Mnemon 的存储简化

MAMGA 使用 NetworkX 内存图，优势是遍历快但不适合生产（无持久化保证、内存受限）。memcp 使用 SQLite + 文件系统双存储。Mnemon 用纯 SQLite 实现所有功能：

- **节点**：insights 表
- **边**：edges 表（复合主键 + CHECK 约束）
- **向量**：embedding BLOB 列
- **审计**：oplog 表

这种统一存储的优势是事务一致性和原子性，劣势是向量搜索必须全表扫描（O(n)）。

## 4.7 特色功能对比

### Mnemon 独有

| 功能 | 说明 | 价值 |
|------|------|------|
| **Intent override** | `--intent WHY\|WHEN\|ENTITY\|GENERAL` | Claude 可修正意图误检 |
| **Signals 元数据** | 每条结果的 keyword/entity/similarity/graph 分数 | Claude 可复判排序 |
| **Sparse hint** | `meta.hint = "sparse_results"` | 提示 Claude 可分解查询 |
| **Causal 拓扑排序** | WHY intent → Kahn 算法排序因果链 | 原因在前、结果在后 |
| **EI edge factor** | 有效重要度含 `(1+0.1×min(edges,5))` | 图中连接度高的节点更耐衰减 |
| **Diff 命令** | 写入前去重/冲突检测 | 避免重复存储 |

### memcp 独有

| 功能 | 说明 | 价值 |
|------|------|------|
| **Context-as-Variable (RLM)** | 大文档存磁盘，Claude 只看元数据 | 97% token 节省 |
| **6 种 Chunking** | auto/lines/paragraphs/headings/chars/regex | 灵活处理不同文档格式 |
| **5 层搜索降级** | keyword→BM25→fuzzy→semantic→hybrid | 渐进式搜索质量提升 |
| **Multi-project** | 自动检测 git root，命名空间隔离 | 多项目同时使用 |
| **Multi-session** | 会话时间戳和 insight 计数 | 会话级审计 |
| **3-zone 留存** | Active→Archive→Purge | 比 Mnemon 更细粒度的生命周期 |
| **PreCompact hook** | `/compact` 前阻塞直到保存完成 | 防止 context rot |

### MAMGA 独有

| 功能 | 说明 | 价值 |
|------|------|------|
| **Episode 分割** | LLM 检测对话边界 | 更好的对话级记忆组织 |
| **Session 摘要** | 生成 SESSION 节点 | 跨会话的全局视图 |
| **QA 检测** | ANSWERED_BY (conf=0.85) | 问答对关联 |
| **Multi-hop 分解** | LLM 分解 → 各自检索 → 合成 | 处理复杂推理查询 |
| **Best-of-N** | N=3 答案选择 | 提高回答质量 |
| **Person boost** | 说话人匹配 20.0 权重 | 对话场景的人物定位 |

## 4.8 给 Mnemon 的启发

### 可以从 memcp 学习的

1. **BM25 搜索**：当前 keyword search 用简单 token overlap，BM25 的 IDF 加权能提升搜索质量。Go 生态有 `blevesearch/bleve` 可用，但会增加二进制大小。**建议**：评估 TF-IDF 加权的性价比，可能一个简单的 IDF 表就够了。

2. **Multi-project 支持**：通过 `--data-dir` 已隐式支持，但缺少自动 git root 检测。**建议**：低优先级，当前 `--data-dir` 够用。

3. **Context-as-Variable (RLM)**：大文档场景下极有价值。但 Mnemon 是 CLI 工具不是 MCP server，Claude 可以直接读文件。**建议**：不需要实现。

4. **PreCompact hook 等效**：Claude Code 的 compact 会丢失上下文。memcp 通过 MCP hook 拦截。Mnemon 通过 CLAUDE.md 的 skill 提示要求 Claude 在 compact 前 remember。**建议**：当前方式足够，不需要技术拦截。

### 可以从 MAMGA 学习的

1. **KeywordEnricher**：embedding 前附加关键词。简单且有效，Go 实现成本低。**建议**：值得尝试，在 `embed.Embed()` 前拼接 entities + tags 到内容末尾。

2. **自适应 BFS**：MAMGA 的 BFS 使用 `relative_drop` 参数（相对分数下降阈值），在高置信度路径上深入，低置信度路径上剪枝。**建议**：中优先级，可以在 beam search 基础上增加 early stopping 策略。

3. **Temporal proximity lookahead**：MAMGA 对未来事件也建 temporal 边（lookahead=10）。当前 Mnemon 只对历史事件建边。**建议**：不需要。CLI 写入是 append-only，不存在"未来"事件。

4. **FAISS 或专用向量索引**：当前全表扫描在 >500 insight 时可能变慢。**建议**：观察实际性能，必要时考虑 Go 实现的 HNSW（`viterin/vek` 等），但 1000 insight 上限下全表扫描可以接受。

### 应该坚持的设计选择

1. **CLI-in-the-loop 引擎架构**：将 LLM 从管道内部提升到引擎层——比 MCP 更灵活（引擎可替换），比 LLM-in-pipe LLM 能力更强（Opus/Sonnet vs gpt-4o-mini），且引擎 LLM 有完整对话上下文
2. **二进制零外部依赖**：自动化层（regex、图遍历、向量搜索）纯本地计算，高频操作零成本零延迟——这是最大的差异化优势
3. **单二进制**：Go 编译的部署优势不可替代
4. **SQLite 统一存储**：简洁性 > 性能（在 1000 insight 规模下）
5. **Signals 元数据**：这是 MAMGA 和 memcp 都没有的，让引擎层 LLM 能做出比管道内部 LLM 更好的判断
6. **Diff 命令**：在写入前的去重是关键的数据质量保障
