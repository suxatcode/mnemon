# ALMA 概览

一句话结论：ALMA 在调研中实际上是两条独立的线，一条让 LLM 演化 memory structure 的代码（alma-meta），另一条是带 budget、scoring、consolidation、forget 工具的 typed memory library（alma-memory）；前者太重，后者的 budget 和 typed memory 思路对 Mnemon 有借鉴价值，但其库式 DB/MCP 集成与 Mnemon 第一阶段的 Markdown framework 路线并不一致。

## 命名说明

调研中存在两个相关但不同的 ALMA：

1. **ALMA meta-learning memory design**：论文 / 源码 `zksha/alma`，全称 Automated meta-Learning of Memory designs for Agentic systems。它的目标不是「记住更多事实」，而是让 meta-learning loop 自动搜索更好的 memory 结构代码。
2. **ALMA-memory library**：`RBKunnela/ALMA-memory` 风格的工程库，提供 typed memory（heuristics、outcomes、anti-patterns、preferences、domain knowledge）、verified retrieval、budget-aware injection、forget / consolidate / checkpoint 工具，和 MCP / Python / TypeScript SDK。

两者都纳入本文，但它们不共享代码、也不共享论文目标。

## 两条线对照表

| 维度 | alma-meta | alma-memory |
|---|---|---|
| 演化对象 | memory structure 的 Python 代码 | typed memory 内容 |
| 主循环 | analyze → generate code → examine/repair → evaluate → archive | retrieve → execute task → learn outcome → consolidate / forget |
| 主入口 | `MetaAgent.forward(steps=10, max_concurrent=5, train_size=30)` | `ALMA.retrieve / ALMA.learn / ALMA.forget / ALMA.checkpoint` |
| 学习信号 | benchmark reward（成功率），sigmoid 归一化 | success / failure outcome、相似策略累积 |
| 选择策略 | softmax over `final_score`，含 visit penalty `alpha * log1p(visit_time)` | scoring weights：similarity 0.4 / recency 0.3 / success_rate 0.2 / confidence 0.1 |
| 候选数量 | 每轮 `maximum_size=5` | retrieval 默认 `top_k=5`，BROAD 15、LEARNING 20、BENCHMARK 50 |
| 长度控制 | 容器内 LLM token budget，由实验 prompt 决定 | `BudgetConfig(max_tokens=4000)`，MemoryStack `to_prompt(max_tokens=2000)` |
| 整理 | archive 候选并保留 reward / parent / visit | `alma_consolidate` / `alma_forget` / `alma_checkpoint` MCP 工具 |
| 安全边界 | LLM 生成 Python 代码 + 容器执行 | DB / 向量索引 / MCP 工具 |
| 适合的位置 | 研究：搜索更好的 memory 设计 | 工程：给应用 agent 加可用的 memory 层 |

ALMA meta 的核心是「记忆机制演化」；ALMA-memory 的核心是「记忆内容管理」。Mnemon 第一阶段的「Markdown 行为资产沉淀」正好处在两者之间，更接近 ALMA-memory 的轻量子集，远离 ALMA meta 的 runtime 代码生成。

## 源码地图

alma-meta 关键位置（`/tmp/mnemon-agent-research-sources/alma-meta`）：

| 位置 | 观察 |
|---|---|
| `core/meta_agent.py:32` | `MetaAgent` 入口；持有 `examine_trial = 3`（meta_agent.py:41）和 `meta_model='gpt-4.1'` 默认值 |
| `core/meta_agent.py:64` | `analyze_memo_structure` 调 `build_analysis_prompt` 产出结构化 analysis schema |
| `core/meta_agent.py:84` | `generate_new_code` 用 senior engineer prompt 生成新 memory structure 代码 |
| `core/meta_agent.py:100` | `examine_new_code` 在容器中 try / fix 最多 3 次 |
| `core/meta_agent.py:205` | `forward(steps=10, max_concurrent=5, train_size=30)` 主循环 |
| `core/memo_manager.py:23` | `Memo_Manager` 管 archive root `memo_archive/<task_type>` |
| `core/memo_manager.py:158` | `update_reward` 用 `sigmoid(reward - no_memo_reward)` 归一化 |
| `core/memo_manager.py:182` | `select_structure(maximum_size=5, tau=0.5)` softmax 选择 |
| `core/meta_agent_prompt.py:194` | `build_analysis_prompt` 构造 analysis schema |
| `core/meta_agent_prompt.py:333` | `build_generate_new_code_prompt` 构造代码生成 prompt |
| `core/meta_agent_prompt.py:469` | `build_reflection_prompt` 构造修复 prompt |
| `evals/agents/memo_structure.py:7` | `Sub_memo_layer` 抽象 `retrieve` / `update` |
| `evals/agents/memo_structure.py:28` | `MemoStructure` 抽象 `general_retrieve` / `general_update` |

alma-memory 关键位置（`/tmp/mnemon-agent-research-sources/alma-memory`）：

| 位置 | 观察 |
|---|---|
| `alma/core.py:68` | `class ALMA` 是顶层 facade |
| `alma/core.py:175` | `ALMA.retrieve` 是默认入口 |
| `alma/core.py:238` | `ALMA.learn` 写 outcome、可能升级为 heuristic / anti-pattern |
| `alma/core.py:384` | `ALMA.forget` 触发 `forgetting_engine.prune` |
| `alma/core.py:474` | `ALMA.checkpoint` 写工作流 checkpoint |
| `alma/retrieval/budget.py:49` | `BudgetConfig(max_tokens=4000)` |
| `alma/retrieval/budget.py:56` | tier 分配：MUST_SEE 40%、SHOULD_SEE 35%、FETCH_ON_DEMAND 25% |
| `alma/retrieval/budget.py:72` | `max_content_chars=500` 单 item 截断 |
| `alma/retrieval/budget.py:499` | `BudgetedRetriever.retrieve_with_budget(top_k=10)`，内部取 `top_k * 2` 做过滤 |
| `alma/retrieval/modes.py:69` | mode 表：BROAD 15 / PRECISE 5 / DIAGNOSTIC 10 / LEARNING 20 / RECALL 3 / BENCHMARK 50 |
| `alma/retrieval/scoring.py:23` | 默认权重 similarity 0.4 / recency 0.3 / success 0.2 / confidence 0.1 |
| `alma/retrieval/engine.py:51` | RetrievalEngine 默认 `cache_ttl_seconds=300`、`max_cache_entries=1000`、`recency_half_life_days=30`、`min_score_threshold=0.2` |
| `alma/context/memory_stack.py:53` | `_DEFAULT_L1_MAX_TOKENS=800`、`_DEFAULT_L2_MAX_TOKENS=500` |
| `alma/context/memory_stack.py:255` | `MemoryStack.to_prompt(max_tokens=2000)` 截断逻辑 |
| `alma/learning/protocols.py:161` | heuristic 升级阈值 `min_occurrences=3` |
| `alma/learning/protocols.py:241` | anti-pattern 阈值 `>= 2` 次相似失败 |
| `alma/mcp/tools/learning.py:198` | `alma_forget(older_than_days=90, below_confidence=0.3)` |
| `alma/mcp/tools/learning.py:237` | `alma_consolidate(memory_type='heuristics', similarity_threshold=0.85, dry_run=True)` |
| `alma/mcp/tools/workflow.py:17` | `alma_checkpoint(run_id, node_id, state, skip_if_unchanged=True)` |
| `alma/consolidation/engine.py:93` | `ConsolidationEngine.consolidate(similarity_threshold=0.85, use_llm=False, dry_run=False)` |

## ALMA meta 架构总览

ALMA meta 把记忆 structure 当作可演化代码，循环大致是：

```text
读取当前 memo SHA 的源码与评估结果
  → analyze_memo_structure 输出结构化 analysis JSON
  → generate_new_code 由 LLM 写出新结构 .py
  → examine_new_code 在容器中跑，失败则用 reflection prompt 修复，最多 3 次
  → memo_manager.execute_memo_structure 跑 benchmark
  → update_reward / update_visit_time 维护 final_score
  → select_structure 用 softmax(scores / 0.5) 抽 5 个继续演化
```

它的核心不是「记忆内容演化」，而是「记忆结构代码演化」。这是研究型自演化，依赖 LLM 写代码、容器执行、benchmark 任务集，门槛很高。

执行入口的代码细节（`memo_manager.py:50-123`）：

- 接受 `code_str`，用正则 `r"```(?:python)?(.*?)```"` 抽出 LLM 输出中的 Python 代码块；如果没有 fence 就视为纯代码。
- 计算 8 位 SHA1 前缀（基于时间戳 + uuid）作为 memo_SHA，用于命名 `memo_structure_<sha>.py`。
- 调 `run_evaluation` 在容器中执行；评估结果落到 `evals/logs/<task_type>/<sha>_<mode>.json`。
- 从结果中读 `examples`，任意 example 含 `error_info` 即视为失败。
- token usage 写入 `GLOBAL_TOKEN_TRACKER`，用于跟踪 meta-learning 总成本。

候选结构本身是 `MemoStructure` 子类（`evals/agents/memo_structure.py:28`）；结构里挂多个 `Sub_memo_layer`（line 7）；每个 layer 必须实现 `retrieve` 与 `update`；`MemoStructure.general_retrieve(recorder)` 在任务前调用，`general_update(recorder)` 在任务后调用。LLM 生成代码时拿到的 backbone 就是这两个抽象类的源码。

## ALMA-memory 架构总览

ALMA-memory 是工程化 memory layer：

- typed memory：Heuristic / Outcome / DomainKnowledge / AntiPattern / UserPreference；
- retrieval engine 带 cache、recency decay、min-score 阈值、6 种 retrieval mode；
- budget-aware retrieval 把召回结果按 tier 装入 4000 token 预算；
- learning protocol 把重复成功策略升级为 heuristic（`min_occurrences=3`），把重复失败模式升级为 anti-pattern（`>=2`）；
- MCP 工具暴露 `alma_retrieve` / `alma_learn` / `alma_consolidate` / `alma_forget` / `alma_checkpoint`；
- MemoryStack 提供 4-layer 包装（identity / essential / on-demand / deep search），`to_prompt(max_tokens=2000)` 是 prompt 注入的稳定接口。

`ALMA` 类（`alma/core.py:68`）是顶层 facade，主要方法签名：

- `retrieve(task, agent, user_id=None, top_k=5)`（line 175）：内部调 `RetrievalEngine.retrieve(query=task, agent, project_id, user_id, top_k, scope)`，并按 agent 是否定义 scope 写日志；返回 `MemorySlice`。
- `learn(agent, task, outcome, strategy_used, task_type=None, duration_ms=None, error_message=None, feedback=None)`（line 238）：写 `Outcome` 并触发 heuristic / anti-pattern 自动升级；invalidate 缓存。
- `forget(agent, older_than_days, below_confidence)`（line 384）：触发 `forgetting_engine.prune`。
- `checkpoint(run_id, node_id, state, ...)`（line 474）：写工作流 checkpoint。
- `learn_from_workflow(...)`（line 580）、`retrieve_with_scope(...)`（line 779）：scope 化版本。

它是库式 memory layer，不是 agent runtime。Mnemon 的 CLI 形态比这个更轻——后者要 DB schema、向量索引、MCP server、Python SDK 才能跑起来。

## Budget-aware retrieval 与 MemoryStack 概览

ALMA-memory 的预算控制有两层：

1. **BudgetConfig + RetrievalBudget**：单次召回的 token 预算与 tier 分配。`BudgetConfig(max_tokens=4000)` 在 `alma/retrieval/budget.py:49`，分配比例 MUST_SEE 40%、SHOULD_SEE 35%、FETCH_ON_DEMAND 25%。token 估算用 `chars_per_token=4` 的简单近似。
2. **MemoryStack.to_prompt(max_tokens=2000)**：把 4 层 stack（identity / essential / on-demand / deep search）按优先级塞入 prompt。L0 永远不截，L1 / L2 / L3 按预算填充，超出后输出 `[truncated — token budget reached]`（`alma/context/memory_stack.py:303`）。

MemoryStack 的 layer 默认配额：

- L0 identity：从文本文件加载，约 100 tokens（memory_stack.py:111）。
- L1 essential story：默认 800 tokens（`_DEFAULT_L1_MAX_TOKENS`，memory_stack.py:53），按 confidence 排序 top memories。
- L2 on-demand：默认 500 tokens（`_DEFAULT_L2_MAX_TOKENS`，memory_stack.py:54）。
- L3 deep search：调底层 ALMA `retrieve` 的全文。
- wake_up 加载 L0 + L1，约 600-900 tokens（memory_stack.py:13）。

这套预算/分层设计的核心思想是：把 prompt 注入和 retrieval 解耦。retrieval 负责拉候选；budget 负责决定哪些进 prompt；MemoryStack 负责按优先级拼接。Mnemon 当前 retrieval 是单层 `recall`，没有 budget 也没有分层；扩展时可以参考此模型，但建议先做最简两层（identity + essential），不必直接照搬 4 层。

## Meta-learning loop 候选选择

`select_structure`（memo_manager.py:182-204）是 alma-meta 的核心选择逻辑：

```python
def select_structure(self, maximum_size=5, seed=42, tau=0.5):
    np.random.seed(seed)
    valid_items = [(k, v["final_score"]) for k, v in self.memo_db.items() if "final_score" in v]
    if not valid_items:
        raise RuntimeError("No available memory structure for selection.")
    keys, scores = zip(*valid_items)
    scores = np.array(scores, dtype=float)
    logits = scores / tau
    exp_score = np.exp(logits - np.max(logits))
    probs = exp_score / np.sum(exp_score)
    k = min(maximum_size, len(scores))
    selected_indices = np.random.choice(len(scores), size=k, replace=False, p=probs)
    return [keys[i] for i in selected_indices]
```

`final_score` 来自 `update_reward`（memo_manager.py:158-171）：

```python
self.memo_db[memo_sha]['reward'] = reward
self.memo_db[memo_sha]['normalized_reward'] = sigmoid(reward - self.no_memo_reward)
self.memo_db[memo_sha]['visit_time'] = 0
penalty = np.log1p(self.memo_db[memo_sha]['visit_time'])
self.memo_db[memo_sha]['final_score'] = self.memo_db[memo_sha]['normalized_reward'] - alpha * penalty
```

`alpha=0.5, tau=0.5, maximum_size=5` 是写死默认。这套 selection 在数学上是 softmax 多臂 bandit + visit penalty，本质上是 explore-exploit trade-off。Mnemon 不需要这个层级的复杂度，但其「分数 + 访问惩罚」的形式给未来 retrieval 排序留了参考。

## 失败模式

alma-meta 的失败模式：

- LLM 生成的代码语法或 import 错误，进入 reflection 循环；超过 `examine_trial=3` 抛 `RuntimeError`（meta_agent.py:141）。
- benchmark 评估在容器中跑实验任务，时间长、token 成本高。
- softmax 选择会反复访问高分 structure，需要 visit penalty `alpha * log1p(visit_time)`（memo_manager.py:170）防止退化为贪心。

alma-memory 的失败模式：

- 召回项总 token 超过 `BudgetConfig.max_tokens=4000`：低优先级 tier 被丢弃，excluded list 进 BudgetReport（budget.py:121）。
- MemoryStack `to_prompt` 超过 `max_tokens=2000`：尾部 layer 被截断并附 "[truncated — token budget reached]"（memory_stack.py:303）。
- consolidate 默认 `dry_run=True`，避免误合并；只有显式传 `dry_run=False` 才修改 storage（learning.py:242）。
- forget 默认 `older_than_days=90, below_confidence=0.3`，过激进会丢失尚未升级的 outcome。

## 与 Mnemon 的关系

ALMA meta 是 Mnemon 的长期研究方向，不是当前路线。如果未来要让 agent 自动搜索不同 memory schema / retrieval policy / lifecycle 规则，ALMA meta 的 selection + reward + reflection loop 是参考；但当前阶段我们只需要让 agent 调 `mnemon` CLI，不打算让 agent 写代码再热加载。

ALMA-memory 是功能对比对象。它的 BudgetConfig、tiered priority、retrieval mode、learning protocol、forget / consolidate / checkpoint 工具，和「outcome 升级为 heuristic」「重复失败升级为 anti-pattern」的门槛思想，都值得 Mnemon 在 retrieval 与生命周期 API 设计上参考。但其库式集成（DB schema、MCP server、Python SDK）比 Mnemon 目标侵入度高得多，第一阶段不应原样引入。

具体到 Mnemon 当前命令面：

- `mnemon recall` 暂不引入 BudgetConfig。但可以借鉴 alma-memory 的「先取 `top_k * 2` 再 rerank / 截断」做法，避免 retrieval 把上下文打满。
- `mnemon remember` 暂不区分 typed memory，但 schema 上要为 `kind` 字段留位置（fact / preference / outcome / anti-pattern / workflow）。
- `mnemon link` 与 alma-memory 的 graph store 思路重合，可参考 `alma/graph/store.py` 的关系存储约定。
- 生命周期命令（consolidate / forget）必须默认 dry-run，输出 patch 由人 review；这与 alma-memory MCP 工具默认 `dry_run=True` 一致。

ALMA 整体提醒我们一件事：把「记忆怎么演化」做成 runtime 行为很容易陷入 alma-meta 的工程深井（容器、benchmark、reward、reflection、archive）。Mnemon 的轻量起点应该把演化暴露成显式 CLI 操作 + Markdown candidate，而不是隐式地让 LLM 写代码。

## 参考来源

- 本地源码：`/tmp/mnemon-agent-research-sources/alma-meta/core/meta_agent.py`
- 本地源码：`/tmp/mnemon-agent-research-sources/alma-meta/core/meta_agent_prompt.py`
- 本地源码：`/tmp/mnemon-agent-research-sources/alma-meta/core/memo_manager.py`
- 本地源码：`/tmp/mnemon-agent-research-sources/alma-meta/evals/agents/memo_structure.py`
- 本地源码：`/tmp/mnemon-agent-research-sources/alma-memory/alma/core.py`
- 本地源码：`/tmp/mnemon-agent-research-sources/alma-memory/alma/retrieval/budget.py`
- 本地源码：`/tmp/mnemon-agent-research-sources/alma-memory/alma/retrieval/modes.py`
- 本地源码：`/tmp/mnemon-agent-research-sources/alma-memory/alma/retrieval/scoring.py`
- 本地源码：`/tmp/mnemon-agent-research-sources/alma-memory/alma/retrieval/engine.py`
- 本地源码：`/tmp/mnemon-agent-research-sources/alma-memory/alma/context/memory_stack.py`
- 本地源码：`/tmp/mnemon-agent-research-sources/alma-memory/alma/learning/protocols.py`
- 本地源码：`/tmp/mnemon-agent-research-sources/alma-memory/alma/mcp/tools/learning.py`
- 本地源码：`/tmp/mnemon-agent-research-sources/alma-memory/alma/mcp/tools/workflow.py`
- 本地源码：`/tmp/mnemon-agent-research-sources/alma-memory/alma/consolidation/engine.py`
- 论文：[Learning to Continually Learn via Meta-learning Agentic Memory Designs](https://arxiv.org/abs/2602.07755)
