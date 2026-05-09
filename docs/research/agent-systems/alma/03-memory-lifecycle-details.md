# ALMA memory lifecycle 细节

一句话结论：alma-meta 用 reward + softmax + visit penalty 在 archive 中演化 memory structure 代码；alma-memory 用 BudgetConfig（4000 tokens）+ tiered priority + retrieval mode + learning thresholds + 显式 consolidate / forget / checkpoint MCP 工具管理 typed memory 内容；Mnemon 第一阶段只能借鉴其中很小一部分（typed memory 概念、升级门槛、retrieval budget、dry-run consolidate），其余暂不引入。

## 两条线对照速览

| 维度 | alma-meta | alma-memory |
|---|---|---|
| 核心对象 | memory structure 代码候选（`memo_structure_<sha>.py`） | typed memory 实例：Heuristic / Outcome / DomainKnowledge / AntiPattern / UserPreference |
| 写路径 | `MetaAgent.forward` → analyze → generate code → examine → evaluate → archive | `ALMA.learn` / `ALMA.add_preference` / `ALMA.add_knowledge` / ingestion / MCP tools |
| 读路径 | benchmark task 的 retrieve / update 由候选结构提供 | `RetrievalEngine.retrieve` 按 query / agent / user / project / mode 检索；可叠 `BudgetedRetriever` |
| 默认召回 | `select_structure(maximum_size=5, tau=0.5)`；softmax 抽样 | `top_k=5`，BudgetedRetriever 内部取 `top_k * 2`（budget.py:520）；mode 提供 3-50 |
| 长度限制 | 由实验 prompt + 容器 + LLM token budget 控制 | `BudgetConfig(max_tokens=4000)`；`max_content_chars=500`；MemoryStack `to_prompt(max_tokens=2000)` |
| 超出处理 | softmax + visit penalty 抑制重复探索 | tier 超预算丢入 `excluded`；MemoryStack 截断并加 "[truncated — token budget reached]" |
| 整理方式 | archive 持久 reward / parent / visit / final_score；`forward` 步进生成新候选 | `alma_consolidate(dry_run=True)`、`alma_forget(older_than_days=90, below_confidence=0.3)`、`alma_checkpoint` |
| 定时 | 无 cron；`forward(steps=10)` 是实验 driver | 无内置 cron；MCP 工具可由调用方 schedule |
| 安全边界 | LLM 生成 Python + 容器执行；需 sandbox 与 examine | DB / vector / MCP；适合应用集成 |

## alma-meta 细节

`MetaAgent` 流程（`core/meta_agent.py`）：

1. 读取并分析现有 memory structure（`analyze_memo_structure`，line 64）。
2. 生成新 Python memory structure 代码（`generate_new_code`，line 84）。
3. examine 新代码，最多尝试 3 次反思 / 修复（`examine_new_code`，line 100；`self.examine_trial = 3`，line 41）。
4. 在 evaluation 容器中跑任务（`memo_manager.execute_memo_structure`）。
5. 记录 reward / parent / visit count（memo_manager.py:158-180）。
6. 通过 softmax over `final_score` 选择下一批结构（`select_structure`，memo_manager.py:182）。

重要默认参数（来源见括号）：

- `forward(steps=10, max_concurrent=5, train_size=30, batch_max_update_concurrent=10, batch_max_retrieve_concurrent=10)`（meta_agent.py:205）。
- archive root：`memo_archive/<task_type>`（memo_manager.py:27）。
- 每轮选择 `maximum_size=5` 个结构（memo_manager.py:182）。
- 选择 temperature `tau=0.5`（memo_manager.py:182）。
- visit penalty `alpha=0.5`，`final_score = normalized_reward - 0.5 * log1p(visit_time)`（memo_manager.py:170）。
- 归一化 reward：`sigmoid(reward - no_memo_reward)`（memo_manager.py:165）。
- examine_trial=3，失败则 `RuntimeError`（meta_agent.py:141）。
- `meta_model='gpt-4.1'`、`execution_model='gpt-4o-mini'`（meta_agent.py:33）。
- task_type 支持 alfworld / minihack / textworld / babaisai（meta_agent.py:25）。

这是研究型 self-evolution。它适合探索「什么 memory design 更好」，但不适合作为 Mnemon 当前的安装机制。

## alma-memory 细节

`alma-memory` 是可用 library。生命周期相关默认值：

| 机制 | 细节 | 出处 |
|---|---|---|
| RetrievalEngine | `cache_ttl_seconds=300`、`max_cache_entries=1000`、`recency_half_life_days=30`、`min_score_threshold=0.2` | `alma/retrieval/engine.py:51` |
| 默认评分 | similarity 0.4、recency 0.3、success_rate 0.2、confidence 0.1 | `alma/retrieval/scoring.py:23` |
| 检索模式 | BROAD top_k=15、PRECISE top_k=5、DIAGNOSTIC top_k=10、LEARNING top_k=20、RECALL top_k=3、BENCHMARK top_k=50 | `alma/retrieval/modes.py:69-149` |
| BudgetConfig | `max_tokens=4000`；MUST_SEE 40%、SHOULD_SEE 35%、FETCH_ON_DEMAND 25% | `alma/retrieval/budget.py:49-58` |
| 数量限制 | `max_heuristics=10`、`max_outcomes=10`、`max_knowledge=5`、`max_anti_patterns=5`、`max_preferences=5` | `alma/retrieval/budget.py:61-65` |
| Token 估算 | `chars_per_token=4`；`truncate_long_content=True`；`max_content_chars=500` | `alma/retrieval/budget.py:68-72` |
| MemoryStack | L0 identity 始终加载；L1 essential story 限 800 tokens；L2 on-demand 限 500 tokens；L3 deep search | `alma/context/memory_stack.py:53-114` |
| wake_up | 加载 L0+L1，约 600-900 tokens；L1 by confidence top_k 10 | `alma/context/memory_stack.py:151-195` |
| to_prompt | `max_tokens=2000`，超过预算输出 "[truncated — token budget reached]" | `alma/context/memory_stack.py:255-307` |
| LearningProtocol | heuristic 阈值 `min_occurrences=3`、`confidence > 0.5`；anti-pattern 阈值 `>=2` 次相似 failure | `alma/learning/protocols.py:161, 186, 241` |
| Forget | `older_than_days=90`、`below_confidence=0.3` | `alma/mcp/tools/learning.py:198-221` |
| Consolidate | `memory_type='heuristics'`、`similarity_threshold=0.85`、`dry_run=True`；引擎默认 `use_llm=False` | `alma/mcp/tools/learning.py:237-303`、`alma/consolidation/engine.py:93` |
| Checkpoint | `skip_if_unchanged=True`；按 `run_id` + `node_id` + `state` 创建 | `alma/mcp/tools/workflow.py:17-77` |
| Pruning | `prune_below_confidence=0.1`（更激进的内部阈值，区别于 forget MCP 默认） | `alma/learning/forgetting.py:718` |

### Budget-aware retrieval 截断逻辑

`RetrievalBudget.apply_budget`（budget.py:320）：

1. 接受一个 `MemorySlice`（来自 RetrievalEngine 拉的 raw 结果）。
2. 把每个 item 按类型映射到 PriorityTier：
   - `heuristic / anti_pattern / preference` → MUST_SEE（budget.py:339-343）
   - `outcome / domain_knowledge` → SHOULD_SEE
3. 按 tier 顺序填充：MUST_SEE 先（preferences、anti-patterns、heuristics），然后 SHOULD_SEE，最后 FETCH_ON_DEMAND。
4. 每个 tier 的预算 = `max_tokens * tier_pct`（budget.py:74-82）。
5. 单 item 超过 `max_content_chars=500` 截断；总预算超 4000 tokens 之后的 item 被丢入 `excluded`，记入 BudgetReport。

`RetrievalBudget.can_include`（budget.py:257）展示了双重检查：

```python
def can_include(self, item, priority=PriorityTier.SHOULD_SEE):
    if priority == PriorityTier.EXCLUDE:
        return False
    estimated = self.estimator.estimate(item)
    tier_budget = self.config.get_tier_budget(priority)
    tier_used = self._tier_usage.get(priority, 0)
    if tier_used + estimated > tier_budget:
        return False
    if self._used_tokens + estimated > self.config.max_tokens:
        return False
    return True
```

这意味着即使总预算还有余，单个 tier 用满后也不能再塞同 tier 的 item。`include` 方法（line 280）支持 `force=True` 用于 MUST_SEE 项，可超 tier 预算但仍受 `max_tokens` 总限。

`BudgetedRetriever.retrieve_with_budget`（budget.py:499）会先用 `top_k * 2` 调 RetrievalEngine 拿 raw 结果，然后调 `apply_budget`，输出 `(MemorySlice, BudgetReport)`。BudgetReport 保留 used / remaining / per-tier 用量、`items_dropped`、`utilization_pct`。

### MemoryStack 4 层与 to_prompt 截断

`MemoryStack` 在 `alma/context/memory_stack.py:104` 定义：

- L0 identity：从文本文件加载，约 100 tokens。
- L1 essential story：confidence 排序的 top memories，预算 `_DEFAULT_L1_MAX_TOKENS=800`（line 53）。
- L2 on-demand：按 topic / domain 调 retrieve，预算 `_DEFAULT_L2_MAX_TOKENS=500`（line 54）。
- L3 deep search：调 ALMA `retrieve` 全文。

`to_prompt(max_tokens=2000)`（line 255）按优先级拼接：

```text
始终包含 L0
如果 token 预算允许 → 加 L1
依次加 active recalls (L2/L3)
  如果某层放不下 → 取剩余预算（>50 tokens 才尝试）
                   截断该层并附 "[truncated — token budget reached]"
                   break
```

如果 `max_tokens` 不足 50，剩余层直接丢弃。

### MemoryStack to_prompt 的具体截断流程

`to_prompt(max_tokens=2000, model=None)`（memory_stack.py:255）按下列顺序输出：

```python
sections = []
tokens_used = 0
# L0 always
if l0.is_loaded:
    tokens_used += l0.token_count
    sections.append(l0.content)
# L1 if budget allows
if l1.is_loaded and tokens_used + l1.token_count <= max_tokens:
    tokens_used += l1.token_count
    sections.append(l1.content)
# Active recalls (L2/L3) one by one
for recall_layer in self._active_recalls:
    if tokens_used + recall_layer.token_count <= max_tokens:
        tokens_used += recall_layer.token_count
        sections.append(recall_layer.content)
    else:
        remaining = max_tokens - tokens_used
        if remaining > 50:
            truncated = estimator.truncate_to_token_limit(
                recall_layer.content,
                max_tokens=remaining,
                suffix="\n[truncated — token budget reached]",
            )
            sections.append(truncated)
        break
return "\n\n".join(sections)
```

要点：

- L0 永远不被截断；
- L1 要么完整加入，要么完全跳过；
- L2 / L3 按加入顺序贪心填充，第一次放不下就尝试截断该层并 break，剩余层全部丢弃；
- 剩余预算 < 50 tokens 时整层丢弃，不输出截断标记。

### Consolidate / Forget / Checkpoint 工具签名

`alma_forget(alma, agent=None, older_than_days=90, below_confidence=0.3)` → `{success, pruned_count, message}`（learning.py:198）。

`alma_consolidate(alma, agent, memory_type='heuristics', similarity_threshold=0.85, dry_run=True)` → `{success, dry_run, merged_count, groups_found, memories_processed, merge_details, errors}`（learning.py:237）。注意：

- 默认 `dry_run=True`。只有显式传 `False` 才落盘。
- `use_llm=False` 写死在 MCP 调用中（learning.py:301）；引擎本身支持 LLM merge 但 MCP 默认走「保留最高 confidence」的合并策略。
- `dry_run=False` 且 merged_count > 0 时调 `alma.retrieval.invalidate_cache`（learning.py:307）。

`alma_checkpoint(alma, run_id, node_id, state, branch_id=None, parent_checkpoint_id=None, metadata=None, skip_if_unchanged=True)` → `{success, checkpoint: {id, run_id, node_id, sequence_number, branch_id, state_hash, created_at}}`（workflow.py:17）。`skip_if_unchanged=True` 时，state 哈希未变就跳过写入。

`async_alma_*` 是 asyncio 包装，签名相同（mcp/__init__.py:68-115）。

### 触发关系

| 工具 | 谁来触发 | 默认安全策略 |
|---|---|---|
| `learn` | agent 完成任务后 | 始终写 outcome；heuristic / anti-pattern 升级走阈值 |
| `forget` | 调用方按需调（无 cron） | 只删 90 天前 outcome 与 confidence < 0.3 heuristic |
| `consolidate` | 调用方按需调 | 默认 dry-run；返回 merge plan 等待 review |
| `checkpoint` | 工作流节点显式调 | `skip_if_unchanged=True` 默认开 |

ALMA-memory 没有内置 scheduler。运维侧把它接到 cron 或 agent 自检循环里即可。

## 失败模式与防御

alma-meta：

- 候选代码 import 错或 runtime 崩溃 → reflection 修复，最多 3 次（meta_agent.py:113-141）。
- 候选代码 reward 低 → 落进 archive 但 final_score 低，不会被反复抽中（softmax 抑制）。
- 同一 SHA 被多次访问 → visit_time 累计，penalty 抑制（memo_manager.py:173）。
- benchmark 评估失败 → log warning，结构正常入 archive 但 reward 缺失（forward.sem_task try/except，meta_agent.py:286-300）。
- meta-evaluation 评估结果 JSON 不存在 → `FileNotFoundError("can't find: {json_path}, examination failed with unknown error.")`（memo_manager.py:103），调用方需要重跑或丢弃。

alma-memory：

- 召回总 token 超 4000：低优先级 tier 被 drop，BudgetReport 记 `budget_exceeded`、`items_dropped`。
- 单 tier 用尽：即使总预算还有余，同 tier 后续 item 也无法纳入；MUST_SEE 可通过 `force=True` 旁路（budget.py:307）。
- MemoryStack to_prompt 超 2000：尾部 layer 截断；如果剩余 < 50 tokens，整层丢弃。
- consolidate 误判：默认 `dry_run=True` 是安全网；`similarity_threshold=0.85` 较保守。
- consolidate 后 cache 失效：当 `dry_run=False` 且 merged_count > 0，自动 `invalidate_cache`，避免读到旧索引（learning.py:307）。
- forget 误删未升级 outcome：阈值 90 天 + 0.3 confidence 是默认，调用方可以传更保守值。`ForgettingEngine` 内部还有更激进的 `prune_below_confidence=0.1`（forgetting.py:718），由 decay 计算后触发，运维侧需关注两套阈值的协同。
- meta-evaluation 类似的失败在 alma-memory 里不存在——它不演化 runtime，只演化数据。

## 对 Mnemon 的启发

可以借鉴的抽象：

- **typed memory 区分**：fact、preference、outcome、anti-pattern、workflow。Mnemon 当前 memory 是单一 namespace，未来 schema 应留这个分类位置。
- **升级门槛**：连续 N 次成功才升级为 guideline / skill；连续 N 次失败才记录 anti-pattern。N 取 2-3 与 ALMA 的实测经验吻合。
- **retrieval budget**：必须有 `top_k`、token budget 和 no-op gate。Mnemon 的 `recall` 暴露 token budget 与 mode 是合理的演进。
- **consolidation 默认 dry-run**：任何「合并 / 删除」操作都要先输出 patch / plan，由人 review。这与 Mnemon `INSTALL.md` candidate 的 review 流程一致。
- **checkpoint 抽象**：`skip_if_unchanged` + `state_hash` 是好设计，可用于 Mnemon 未来的 session 状态保存。

为什么不在第一阶段引入：

- **不引入 alma-meta**：LLM 写 runtime Python 与 Mnemon「本地优先 / 可审计」原则冲突；缺 benchmark 任务集；缺容器评估；token 成本高。
- **不引入 BudgetConfig 的全套 tier**：Mnemon 当前 retrieval 输出还没有 typed memory，做 4000-token tier 分配缺乏对象。
- **不引入自动 forget**：Mnemon 必须 human-in-the-loop，自动删低分 memory 风险大。
- **不引入复杂 feedback / trust scoring**：第一阶段连 outcome 都不强写入，没有数据驱动 trust scorer。
- **不引入 MemoryStack 的 4 层**：Mnemon 没有 identity / essential / on-demand / deep 的强分层需求；扁平 namespace + tag 已足够。

第二阶段可以考虑的最小子集：

- 给 `recall` 加 `--mode precise|broad|recall`；
- 给 `recall` 加 `--max-tokens` budget 与截断策略；
- 在 lifecycle 命令里实现 `mnemon consolidate --dry-run`；
- 暴露 `mnemon forget --older-than 90d --below-confidence 0.3` 类工具，但默认 dry-run。

## 参考来源

- 论文：[Learning to Continually Learn via Meta-learning Agentic Memory Designs](https://arxiv.org/abs/2602.07755)
- 本地源码：`/tmp/mnemon-agent-research-sources/alma-meta/core/meta_agent.py`
- 本地源码：`/tmp/mnemon-agent-research-sources/alma-meta/core/meta_agent_prompt.py`
- 本地源码：`/tmp/mnemon-agent-research-sources/alma-meta/core/memo_manager.py`
- 本地源码：`/tmp/mnemon-agent-research-sources/alma-memory/alma/core.py`
- 本地源码：`/tmp/mnemon-agent-research-sources/alma-memory/alma/retrieval/engine.py`
- 本地源码：`/tmp/mnemon-agent-research-sources/alma-memory/alma/retrieval/budget.py`
- 本地源码：`/tmp/mnemon-agent-research-sources/alma-memory/alma/retrieval/modes.py`
- 本地源码：`/tmp/mnemon-agent-research-sources/alma-memory/alma/retrieval/scoring.py`
- 本地源码：`/tmp/mnemon-agent-research-sources/alma-memory/alma/context/memory_stack.py`
- 本地源码：`/tmp/mnemon-agent-research-sources/alma-memory/alma/learning/protocols.py`
- 本地源码：`/tmp/mnemon-agent-research-sources/alma-memory/alma/learning/forgetting.py`
- 本地源码：`/tmp/mnemon-agent-research-sources/alma-memory/alma/consolidation/engine.py`
- 本地源码：`/tmp/mnemon-agent-research-sources/alma-memory/alma/mcp/tools/learning.py`
- 本地源码：`/tmp/mnemon-agent-research-sources/alma-memory/alma/mcp/tools/workflow.py`
