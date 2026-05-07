# ALMA memory lifecycle 细节

## 核心判断

ALMA 实际有两条线：

- `alma-meta`：让 LLM 生成、评估和演化 memory structure 代码，是 memory design self-evolution。
- `alma-memory`：结构化记忆库，提供 retrieval、learning、budget、consolidation、forget、MCP tools。

它们都对 Mnemon 有研究价值，但第一阶段不应照搬。Mnemon 当前要的是 agent 可安装的 Markdown/hook framework，不是让模型生成 memory runtime 代码，也不是先引入复杂 DB schema。

## 生命周期详表

| 维度 | alma-meta | alma-memory |
|---|---|---|
| 核心对象 | memory structure 代码候选 | typed memories：heuristics、outcomes、domain knowledge、anti-patterns、preferences 等 |
| 写路径 | MetaAgent 分析旧结构、生成新代码、examine/fix、evaluate、archive | `learn`、`add_preference`、`add_knowledge`、workflow learn、ingestion、MCP tools |
| 读路径 | evaluation harness 使用候选结构执行任务 | retrieval engine 按 query、agent、user、project、mode 检索 top_k |
| 默认召回量 | 选择最多 5 个结构进入下一轮 | `retrieve(..., top_k=5)`；内部先取 `top_k * 2` 再重排 |
| 长度限制 | 无统一 memory char cap，由实验 prompt、容器、LLM token budget 和候选代码自定 | `BudgetConfig(max_tokens=4000)`；内容估算 chars/token=4；`max_content_chars=500` 用于预算报告/截断意图 |
| 超出处理 | 通过 softmax 选择结构、visit penalty、并发评估预算控制搜索空间 | Budget-aware retrieval 按 tier 分配 token；超预算 item 被排除；MemoryStack `to_prompt(max_tokens=2000)` 到预算后截断 |
| 整理方式 | 训练循环持续生成新结构并 checkpoint | consolidation tool 按 similarity grouping 合并；forget 删除旧 outcomes 和低置信 heuristics |
| 定时任务 | 无内置 cron；`forward(steps=...)` 是实验 driver | 无核心 cron；consolidate/forget/checkpoint 是显式工具/API |
| 安全边界 | 代码生成和容器评估风险高，需要 sandbox/eval gate | DB/API/MCP 工具边界，适合应用集成但比 Markdown framework 重 |

## alma-meta 细节

`alma-meta` 的 MetaAgent 流程是：

1. 读取并分析现有 memory structure。
2. 生成新的 Python memory structure 代码。
3. examine 新代码，最多尝试 3 次反思/修复。
4. 在 evaluation container 中跑任务。
5. 记录 reward、parent、visit count、checkpoint。
6. 通过 softmax over score 选择下一批结构继续演化。

重要默认参数：

- `forward(steps=10, max_concurrent=5, train_size=30, ...)`。
- archive root 为 `memo_archive/<task_type>`。
- 每轮选择 `maximum_size=5` 个结构。
- selection temperature `tau=0.5`。
- visit penalty `alpha * log1p(visit_time)`，`alpha=0.5`。
- batch update/retrieve 并发默认 10。

这是一种研究型 self-evolution。它适合探索「什么 memory design 更好」，但不适合作为 Mnemon 当前的安装机制。

## alma-memory 细节

`alma-memory` 更像可用 library：

| 机制 | 细节 |
|---|---|
| RetrievalEngine | 默认 cache TTL 300s、max cache entries 1000、recency half-life 30 days、min score threshold 0.2。 |
| 默认评分 | similarity 0.4、recency 0.3、success_rate 0.2、confidence 0.1。 |
| 检索模式 | BROAD top_k 15；PRECISE top_k 5；DIAGNOSTIC top_k 10；LEARNING top_k 20；RECALL top_k 3；BENCHMARK top_k 50。 |
| BudgetConfig | `max_tokens=4000`；MUST_SEE 40%、SHOULD_SEE 35%、FETCH_ON_DEMAND 25%。 |
| 数量限制 | max heuristics/outcomes 10，knowledge 5，anti-patterns 5，preferences 5。 |
| MemoryStack | L0 identity 始终加载；L1 essential story；L2 on-demand；L3 deep search。 |
| wake_up | 加载 L0+L1，约 600-900 tokens；L1 top_k 10。 |
| to_prompt | 默认 `max_tokens=2000`，超过预算输出截断提示。 |
| LearningProtocol | 默认 heuristic 需要相似 outcome 出现 3 次；anti-pattern 需要至少 2 个相似 failure。 |
| Forget | 默认删除 older_than_days=90 的 outcomes 和 below_confidence=0.3 的 heuristics。 |
| Consolidate | `alma_consolidate` 默认 dry_run=true，similarity_threshold 0.85，top_k=1000，默认不使用 LLM merge。 |

## 超出处理与整理策略

ALMA 的核心思想不是「把所有 memory 都塞进 prompt」，而是：

- 用 scoring 和 modes 决定召回哪些。
- 用 token budget 和 tiers 控制 prompt 注入。
- 用 learning protocol 把重复经验提升为 heuristic。
- 用 forget/consolidate 定期减少噪音。
- 用 feedback 调整未来召回权重。

这比 Markdown-only 更强，但也要求 DB、embedding、scoring、schema、MCP tools 和评估基础设施。

## 对 Mnemon 的启发

Mnemon 可吸收：

- memory 类型区分：fact、preference、outcome、anti-pattern、workflow。
- promotion 门槛：重复出现 2-3 次后再提升为 guideline/skill。
- retrieval budget：必须有 top_k、token budget 和 no-op gate。
- consolidation 默认 dry-run，输出 patch 供 review。

Mnemon 暂不吸收：

- LLM 生成 runtime code。
- 多层 DB schema。
- 自动删除低分 memory。
- 复杂 feedback scorer。

## 参考来源

- 论文页: [Learning to Continually Learn via Meta-learning Agentic Memory Designs](https://arxiv.org/abs/2602.07755)
- 本地源码: `/tmp/mnemon-agent-research-sources/alma-meta/core/meta_agent.py`
- 本地源码: `/tmp/mnemon-agent-research-sources/alma-meta/core/memo_manager.py`
- 本地源码: `/tmp/mnemon-agent-research-sources/alma-memory/alma/core.py`
- 本地源码: `/tmp/mnemon-agent-research-sources/alma-memory/alma/retrieval/`
- 本地源码: `/tmp/mnemon-agent-research-sources/alma-memory/alma/budget/`
- 本地源码: `/tmp/mnemon-agent-research-sources/alma-memory/alma/learning/`
