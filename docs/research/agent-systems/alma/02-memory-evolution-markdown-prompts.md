# ALMA 的记忆、演化与 Prompt 用法

一句话结论：ALMA 的两条线对「演化」的定义完全不同——alma-meta 演化的是 memory structure 的 Python 代码（meta-learning loop），alma-memory 演化的是 typed memory 内容（learn / consolidate / forget）；Markdown 在两者中都不是 runtime artifact，而是 prompt / 文档载体；Mnemon 第一阶段需要的演化形态比 alma-meta 轻，比 alma-memory 简单，更接近「Markdown candidate + review + 安装」。

## 两条线对照：演化对象与演化机制

| 维度 | alma-meta | alma-memory |
|---|---|---|
| 演化对象 | memory structure 代码（继承 `MemoStructure` 与 `Sub_memo_layer`） | typed memory 内容：heuristics、outcomes、anti-patterns、preferences、domain knowledge |
| 触发 | `MetaAgent.forward(steps=10)` 每步 select + analyze + generate + examine + evaluate | `ALMA.learn` 写 outcome；满足阈值后自动升级为 heuristic / anti-pattern |
| 学习信号 | benchmark `benchmark_overall_eval_score`，再用 `sigmoid(reward - no_memo_reward)` 归一化（memo_manager.py:165） | `success` flag + 相似策略累积；`min_occurrences=3` 升级 heuristic（protocols.py:161），`>= 2` 升级 anti-pattern（protocols.py:241） |
| 选择机制 | softmax over `final_score = normalized_reward - alpha * log1p(visit_time)`，`alpha=0.5, tau=0.5, maximum_size=5`（memo_manager.py:158-204） | retrieval scoring 默认 `similarity 0.4 / recency 0.3 / success 0.2 / confidence 0.1`（scoring.py:23） |
| 写入边界 | examine_trial=3 失败抛 `RuntimeError` 不入 archive（meta_agent.py:113-141） | confidence 阈值；consolidate 默认 dry-run；forget 限于过期或低置信项 |
| Prompt 角色 | senior engineer 写代码 / 反思修复 / 分析 schema | LLM 不一定参与；MemoryStack `to_prompt(max_tokens=2000)` 直接注入 |

## alma-meta：让 LLM 重写 memory structure 代码

`MetaAgent` 在 `core/meta_agent.py:32` 起步。每个 task_type 一个 archive 目录（`memo_archive/<task_type>`）。主入口是 `forward`：

```text
forward(steps=10, max_concurrent=5, train_size=30)
  if no checkpoint:
    跑 baseline (target_sha='no_mem') 算 no_memo_reward
    generate_new_code → examine_new_code（最多 3 次反思）→ execute_memo_structure 评估
    update_reward 写入第一个候选
  for step in range(steps):
    memo_SHA_list = memo_manager.select_structure()  # softmax 抽最多 5 个
    并发 run_single_memo
      update_visit_time
      analyze_memo_structure (analysis_agent ask)
      generate_new_code (gen_code_agent ask)
      examine_new_code (尝试 examine_trial=3 次)
      execute_memo_structure (eval)
      update_reward
```

关键点是「analyze + generate + examine + evaluate」全部由 LLM 调用驱动，而 LLM 输出的是 Python 源码。Memo_Manager 把代码哈希成 SHA，落盘成 `memo_structure_<sha>.py`，并维护 `memo_db` 字典记录 reward / parent / visit_time / final_score / analysis suggestion。

select_structure（memo_manager.py:182）的归一化是关键：

```text
logits = scores / 0.5
probs = softmax(logits)
selected = numpy.random.choice(len(scores), size=min(5, n), replace=False, p=probs)
```

`tau=0.5` 让分布更尖；`alpha=0.5` 的 visit penalty 防止反复采样同一结构。这是非常典型的 explore-exploit。

## alma-meta 的 prompt 模式

`core/meta_agent_prompt.py` 给三种角色：

- `build_analysis_prompt`（line 194）：让 LLM 扮演 Senior Agent Construction Engineer，读 `source_code` + `examples` + `benchmark_eval_score` + 可选 `improve_example`，输出结构化 analysis schema（包含 prioritized suggestions、High/Medium/Low）。
- `build_generate_new_code_prompt`（line 333）：让 LLM 扮演 senior AI software engineer，依据 analysis 结果 + 当前 source code + recorder 接口产出新的 `MemoStructure` Python 代码。
- `build_reflection_prompt`（line 469）：把执行错误 `error_msg` 注入 system prompt 作为 code repair。

这些 prompt 共同点：

- 强角色化（senior engineer / repair expert）；
- 给 schema / interaction protocol / class 接口约束；
- 强制 JSON schema 输出（analysis）或 Python 代码块（generate / reflection）；
- 用过往 `improve_example` 显式作为 in-context few-shot，让模型从 `improve_score` 推断「什么修改能涨分」。

`build_generate_new_code_prompt` 在系统提示里塞了相当多的工程上下文（meta_agent_prompt.py:333-465）：

- `<BACKBONE_CODE>` 块：`evals/agents/memo_structure.py` 的源码，定义 `Sub_memo_layer` 与 `MemoStructure` 抽象。
- `<CODE_INPUT>` 块：`Basic_Recorder` 的属性 metadata（dict 含 init / steps / reward 等字段）。
- `<CODE_USAGE>` 块：明确 `general_retrieve` 在任务前调、`general_update` 在任务后调；retrieve 输出 JSON 直接喂给下游 agent。
- `<GRAPH_DATABASE_INTERACTION>` / `<CHROMA_DATABASE_INTERACTION>` / `<OTHER_TOOLS>` 块：把 NetworkX 与 Chroma 的 cheat sheet 直接放进 prompt。
- 任务专属 `TASK_DESCRIPTION[task_type]` 描述 alfworld / minihack / textworld / babaisai 的任务结构。

这种 prompt 与 Codex / Claude Code 的 `AGENTS.md` / `CLAUDE.md` 不在一个层面：alma-meta 的 prompt 是一次性的、面向代码生成的，结果保存为可执行 `.py`；而 Markdown-based 系统的 prompt 是长期的、面向行为对齐的，结果保存为人类可读 doc。

这套 prompt 的目标是「自动改 memory structure 代码」，不适合 Mnemon 第一阶段。Mnemon 真正需要的 prompt 模式更接近：让 LLM 总结一段经验、提出 candidate 安装到 `SKILL.md`，由人 review 后落盘。

## alma-memory：让 typed memory 自然演化

ALMA-memory 的 learn 路径在 `alma/learning/protocols.py:59`：

```text
LearningProtocol.learn(task, strategy_used, success, outcome, scope, ...)
  写 Outcome 记录
  if success:
    _maybe_create_heuristic
      取最近 outcomes，过滤同 strategy
      if len(same_strategy) >= min_occurrences (默认 3, 可被 scope 覆盖):
        confidence = success_count / total
        if confidence > 0.5: 写 Heuristic
  else:
    _maybe_create_anti_pattern
      取最近 outcomes，过滤同 error
      if len(similar) >= 2: 写 AntiPattern
```

这条路径的「演化」是隐式的：任何 outcome 都可能在累计 3 次后升级为 heuristic，2 次相似 failure 后升级为 anti-pattern。它不需要 LLM 写代码，只要 storage 能查询、similarity 能算。

`_maybe_create_heuristic` 的关键代码（protocols.py:181-209）：

```python
if len(same_strategy) >= min_occurrences:
    success_count = sum(1 for o in same_strategy if o.success)
    confidence = success_count / len(same_strategy)
    if confidence > 0.5:
        heuristic = Heuristic(
            condition=f"task type: {task_type}",
            strategy=strategy,
            confidence=confidence,
            occurrence_count=len(same_strategy),
            success_count=success_count,
            ...
        )
        self.storage.save_heuristic(heuristic)
```

`_maybe_create_anti_pattern` 的对应代码（protocols.py:225-263）拉最近 10 个 outcome，过滤 error message 相似的失败项；只要相似失败数 `>= 2`，就生成一条 `AntiPattern`，但 `better_alternative` 字段先填占位 `"[To be determined from successful outcomes]"`，后续可由其他工具补全。

并行的 `add_preference`（line 265）和 `add_domain_knowledge`（line 285）则是显式 API：用户或 ingestion pipeline 直接写入，不走门槛检查。这给 Mnemon 一个清晰的分工启示：

- **隐式升级**靠 outcome 累积，要求 storage 支持 `top_k` 查询和 strategy / error 相似度判断；
- **显式写入**靠 API（preference、knowledge），适合人工录入和高置信源。

对应到 Mnemon：`mnemon remember` 是显式入口，可以直接落 fact / preference；而 `mnemon learn`（如果未来增加）则应是隐式升级入口，需要先有 outcome 数据。

## Markdown 在两条线中的角色

ALMA meta：Markdown 主要承载 prompt 和文档；LLM 输出按 Markdown code fence 抽出 Python 后保存为 `memo_structure_<sha>.py`；它没有 `SKILL.md` / `AGENTS.md` 风格的行为资产。

具体看 `Memo_Manager.execute_memo_structure`（memo_manager.py:67-69）：

```python
match = re.search(r"```(?:python)?(.*?)```", code_str, re.DOTALL)
code = match.group(1).strip() if match else code_str.strip()
```

也就是说 Markdown 只是 LLM 输出与 Python 文件之间的胶水，没有任何长期 Markdown artifact 落到 archive。

ALMA-memory：库自身使用 Markdown 文档（`README.md`、`GUIDE.md`、`mkdocs.yml` 站点），但 runtime 行为通过 Python / TypeScript SDK、MCP tools 和 typed memory 对象表达，而不是「在仓库里写个 `SKILL.md`」。

两条线都不是 Markdown-driven。Markdown 只是工程交付载体。这与 Hermes、Codex、Claude Code 显著不同：后者把 Markdown 视作 agent 行为资产，要求 agent 在结束任务后向 Markdown 增量、并由 framework 在下一次启动时再加载。

## 失败模式与对应 prompt 行为

alma-meta 的失败处理：

- 如果 generate_new_code 写出的 Python 不可执行，`examine_new_code` 抓住异常，把 `error_msg` 喂给 reflection prompt（meta_agent_prompt.py:469），让 LLM 修复；最多 3 次（meta_agent.py:113）。
- 如果 3 次都失败，抛 `RuntimeError("Fail to revise code in {self.examine_trial} attempt.")`（meta_agent.py:141）。这条候选不入 archive。
- 这种 fail-fast + reflection 模式让 archive 里只保留可执行结构，但代价是 LLM 调用成本翻倍。

alma-memory 的失败处理：

- learn 时如果 storage 写失败由调用方处理；不会自动重试。
- consolidate 默认 `dry_run=True`，先输出 merge plan，由调用方决定是否落盘（learning.py:242, consolidation/engine.py:170）。
- forget 默认保留 `older_than_days=90` 内的 outcome 与 `below_confidence=0.3` 以上的 heuristic（learning.py:201）；阈值偏保守。

## Consolidation 与 forget：另一种「演化」

ALMA-memory 还提供两类后期演化：

- **Consolidate**：通过 `alma_consolidate(agent, memory_type='heuristics', similarity_threshold=0.85, dry_run=True)`（learning.py:237），用 cosine similarity 将相似 typed memory 分组合并。`ConsolidationEngine.consolidate`（consolidation/engine.py:93）默认 `use_llm=False`，靠最高 confidence 选代表 item；如果传 `use_llm=True` 则用 LLM merge。注意 MCP 调用在 learning.py:301 写死 `use_llm=False`，意味着 MCP 默认走非-LLM 路径。
- **Forget**：通过 `alma_forget(agent, older_than_days=90, below_confidence=0.3)`（learning.py:198），调用底层 `forgetting_engine.prune`，按时间和置信度阈值删除。`ForgettingEngine` 内部还支持 decay-based pruning（forgetting.py:469-560，`compute_decay_score` + `identify_candidates`）；decay function 可选 ExponentialDecay（half-life 30 天）、LinearDecay、StepDecay。

这两个动作不是「记忆内容生长」，而是「记忆内容修剪」——和 alma-meta 的 selection（让低分结构 visit penalty 后被替代）形成对称：alma-memory 通过删除让 memory pool 保持质量。

对应 prompt：consolidate 的 LLM merge prompt 在 `alma/consolidation/prompts.py`，但默认不启用；这是为了保证操作可审计（dry-run + 可观察 merge plan）。

## 对 Mnemon 的设计判断

ALMA 提醒我们 memory-driven self-evolution 至少有两层：

1. **行为资产演化**：skills、guidelines、install notes、project rule。Mnemon 当前阶段应聚焦此层。形态接近「LLM 反思 → Markdown candidate → review → 安装」。
2. **记忆机制演化**：schema、retrieval policy、update algorithm、reward loop。属于研究阶段，对应 alma-meta 的 selection / reward / reflection 全套。

Mnemon 当前不应直接做 alma-meta 式的代码自演化，理由：

- LLM 写 runtime 代码与 Mnemon「本地优先 / 可审计」目标冲突；
- 没有 benchmark 任务集就无法稳定算 reward；
- 没有容器评估就无法安全跑候选；
- 评估成本远超第一阶段需要的「让 agent 多记几条事实」。

更现实的路径是：

- 沿用 `mnemon recall / remember / link` 积累 evidence；
- 借鉴 alma-memory 的「重复 N 次后升级」思想，把 repeated 工作流写成 Markdown candidate；
- review 后安装为 `SKILL.md` / `GUIDELINE.md` / `INSTALL.md`；
- 等行为层稳定后，再评估是否需要把 retrieval 升级到 budget / mode / scoring 化。

借鉴 alma-memory 的具体抽象（即使不立刻引入）：

- typed memory 区分（fact / preference / outcome / anti-pattern / workflow）；
- 升级阈值（3 次成功 → heuristic，2 次失败 → anti-pattern）；
- consolidate 默认 dry-run、提供 merge plan；
- forget 用「时间 + 置信度」组合阈值；
- retrieval 应有 mode（精确 / 探索 / 诊断 / 召回）和 budget。

不借鉴的部分：

- LLM 生成 runtime 代码；
- DB / vector index / MCP server 的强工程化；
- 自动删除低分 memory（Mnemon 必须 human-in-the-loop）；
- 复杂 feedback scorer 与 trust scoring。

## 失败模式总结

| 失败 | alma-meta 表现 | alma-memory 表现 |
|---|---|---|
| 候选不可执行 | reflection 修复 3 次后抛 RuntimeError | n/a（不演化代码） |
| 评估 budget 超 | softmax + visit penalty 限制 | BudgetReport 记录 excluded |
| 召回总长超限 | 由 LLM token budget 间接控制 | MemoryStack 截断 + "[truncated — token budget reached]" |
| 误升级 / 误合并 | 无升级概念；archive 完整保留 | min_occurrences=3、similarity 0.85、dry-run 默认 |
| 误删 | 无删除概念；保留所有 archive entries | older_than_days=90、below_confidence=0.3，但仍可能删未升级 outcome |
| 评估失败 | log warning，候选无 reward | n/a |

这一对照对 Mnemon 的提示是：「演化 = 写入 + 升级 + 整理 + 修剪」是个连续光谱。Mnemon 第一阶段只覆盖「写入」一端（mnemon remember / link），二阶段需要补「升级」（candidate Markdown），更后期再考虑「整理 / 修剪」。直接把 alma-memory 的 consolidate / forget 抄过来对当前阶段没有数据支撑。

## 参考来源

- 本地源码：`/tmp/mnemon-agent-research-sources/alma-meta/core/meta_agent.py`
- 本地源码：`/tmp/mnemon-agent-research-sources/alma-meta/core/meta_agent_prompt.py`
- 本地源码：`/tmp/mnemon-agent-research-sources/alma-meta/core/memo_manager.py`
- 本地源码：`/tmp/mnemon-agent-research-sources/alma-memory/alma/learning/protocols.py`
- 本地源码：`/tmp/mnemon-agent-research-sources/alma-memory/alma/retrieval/scoring.py`
- 本地源码：`/tmp/mnemon-agent-research-sources/alma-memory/alma/retrieval/budget.py`
- 本地源码：`/tmp/mnemon-agent-research-sources/alma-memory/alma/context/memory_stack.py`
- 本地源码：`/tmp/mnemon-agent-research-sources/alma-memory/alma/mcp/tools/learning.py`
- 论文：[Learning to Continually Learn via Meta-learning Agentic Memory Designs](https://arxiv.org/abs/2602.07755)
