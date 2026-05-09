# Agno memory lifecycle 细节

## 核心判断

Agno 是应用框架式 memory：开发者通过 `MemoryManager`、database、agent flags 和 tools 决定 memory 何时生成、是否进入上下文、是否由 agent 显式操作。它不像 Hermes 那样以 Markdown skills 为中心，也不像 OpenClaw 那样内置 dreaming runtime。

对 Mnemon 来说，Agno 主要提供两个经验：memory 可后台更新但不必自动注入上下文；当 memories 积累到一定数量后，需要显式 optimization。

## 源码地图

| 关注点 | 文件:行 | 观察 |
|---|---|---|
| Agent 默认 flags | `libs/agno/agno/agent/agent.py:104-126` | summary/agentic/update 全部默认 False |
| history 默认 3 runs | `libs/agno/agno/agent/agent.py:557-563` | 二者都未设时硬写 `num_history_runs = 3` |
| set_memory_manager | `libs/agno/agno/agent/_init.py:99-114` | 默认构造 manager；自动决定 `add_memories_to_context` |
| 后台 future（同步线程） | `libs/agno/agno/agent/_managers.py:180-215` | `start_memory_future` 提交 `make_memories` 到 `agent.background_executor` |
| 后台 task（async） | `libs/agno/agno/agent/_managers.py:139-177` | `astart_memory_task` 走 `asyncio.create_task` |
| make_memories 实际写入 | `libs/agno/agno/agent/_managers.py:29-81` | 仅在 `update_memory_on_run=True` 且非 agentic 模式触发 |
| run 编排（同步流） | `libs/agno/agno/agent/_run.py:473-553` | 第 7 步启动 memory future，第 11 步等待并合并 metrics |
| run 编排（async stream） | `libs/agno/agno/agent/_run.py:1556-1687` | `_arun_stream` 的对应步骤 |
| MemoryManager.create_user_memories | `libs/agno/agno/memory/manager.py:368-421` | 把当前 message + existing memories 喂给 LLM 决定写入 |
| MemoryManager.search_user_memories | `libs/agno/agno/memory/manager.py:588-638` | 三种 retrieval method |
| MemoryManager.optimize_memories | `libs/agno/agno/memory/manager.py:793-862` | `apply=True` 时 `clear_user_memories` 后批量 upsert |
| SummarizeStrategy | `libs/agno/agno/memory/strategies/summarize.py:15-119` | 把所有 memory 合成单一第三人称叙述 |
| MemoryOptimizationStrategyType | `libs/agno/agno/memory/strategies/types.py:8-12` | 当前只有 `SUMMARIZE` 一种 |
| SessionSummaryManager | `libs/agno/agno/session/summary.py:62-102` | `last_n_runs` / `conversation_limit` 双切片旋钮 |
| MarkdownChunking 默认 5000 | `libs/agno/agno/knowledge/chunking/markdown.py:29` | 默认 chunk_size，不按 headings 拆分 |
| AgenticChunking MAX_CHUNK_SIZE | `libs/agno/agno/knowledge/chunking/agentic.py:11` | 上限 5000 |
| SchedulerTools | `libs/agno/agno/tools/scheduler.py:29-90` | 通用 cron，依赖 AgentOS + SchedulePoller |
| Memory prompt（preference） | `libs/agno/agno/agent/_messages.py:299-306` | 当前对话优先于历史 memory |

## 生命周期详表

| 维度 | 观察 |
|---|---|
| 主要记忆载体 | DB 中的 `UserMemory`；session history；session summary；knowledge chunks。 |
| 写路径 | `update_memory_on_run=True` 时后台更新（`_managers.py:180`）；`enable_agentic_memory=True` 时 agent 获得 `update_user_memory(task)` tool（`_default_tools.py:38`）；亦可显式装载 `MemoryTools`（`tools/memory.py:13`）。 |
| 读路径 | `add_memories_to_context=True` 自动注入（`_messages.py:286-302`）；或使用 `search_user_memories` 显式搜索（`manager.py:588`）。 |
| 默认历史 | `num_history_messages` 与 `num_history_runs` 都未设时默认 `num_history_runs=3`（`agent.py:557-563`）。两者都设时使用 `num_history_runs` 并 warning。 |
| 长度限制 | 未发现全局 memory char hard cap；受 DB、retrieval limit、history settings、model context 和 knowledge chunk size 约束。 |
| knowledge chunk | Markdown chunk 默认 `chunk_size=5000` chars，`overlap=0`，默认不按 headings 拆分（`chunking/markdown.py:29`）。 |
| 搜索限制 | `search_user_memories(query, limit, retrieval_method)`，支持 `last_n` / `first_n` / `agentic`（`manager.py:588-638`）。 |
| 超出处理 | 自动注入 memories 会增加 token cost；官方建议用户 50+ memories、昂贵操作前、长期应用周期维护时运行 memory optimization。 |
| 整理方式 | `optimize_memories(strategy=SUMMARIZE, apply=True)`：读取全部 memory，生成优化列表，清空并重写（`manager.py:793-862`）。 |
| 后台任务 | 非 agentic memory update 通过 thread/async task 在 run 期间后台执行（`_managers.py:139-215`）；不是 cron。 |
| 定时能力 | `SchedulerTools` 可让 agent 创建 cron-like schedules（`tools/scheduler.py:29-90`），但是通用调度，依赖 DB、AgentOS server、SchedulePoller。 |
| 安全/隐私 | MemoryManager 可自定义 `additional_instructions`（`manager.py:55`），例如要求不保存真实姓名。 |

## 完整数据流

一次 `agent.run()` 内的 memory 数据流（取自 `_run.py:335-553`）：

1. 入口 `_run` 拿到 `run_messages` 与 `user_id`；
2. 第 7 步显式调用 `_managers.start_memory_future(agent, run_messages, user_id, existing_future=memory_future)`（`_run.py:476`），后者：
   - 检查 `has_content`（user_message 或 extra_messages 非空）；
   - 检查 `agent.memory_manager is not None`；
   - 检查 `agent.update_memory_on_run`；
   - 检查 `not agent.enable_agentic_memory`；
   - 满足才把 `make_memories` 提交给 `agent.background_executor`；
3. 主线程继续生成响应。如果走 agentic 路径，模型期间可能调用 `update_user_memory(task)`（`_default_tools.py:38`），同步进入 `MemoryManager.update_memory_task`（`manager.py:481`），该路径不在后台；
4. 第 11 步等待 memory_future 完成（`_run.py:590-598`），把模型 metrics 合并；
5. 出错时 `_run.py:698-700` 取消所有 background futures（memory / cultural_knowledge / learning）。

`make_memories`（`_managers.py:29-81`）的实际工作：

- 拿到 user_message 字符串，若非空且 `update_memory_on_run=True`，调用 `MemoryManager.create_user_memories(message=..., user_id=..., agent_id=agent.id)`；
- 处理 `extra_messages` 时先过滤空内容，然后再次走相同 manager 调用；
- 整个过程通过 `RunMetrics` collector 报告 token 与延迟。

`MemoryManager.create_user_memories`（`manager.py:368-421`）流程：

1. 读取该 user 的现有 memory；
2. 把 existing memories 投影成 `[{memory_id, memory}]`；
3. 调用 `create_or_update_memories`（`manager.py:1040-1107`）；
4. `create_or_update_memories` 拼装系统提示（`manager.py:958-1038`）+ 子工具（`add_memory` / `update_memory` / `delete_memory`）+ user message，让 LLM 输出 tool calls；
5. 工具被 framework 反向 dispatch 到 `_upsert_db_memory`（`manager.py:561`）或 `_delete_db_memory`（`manager.py:572`）；
6. `read_from_db` 再次刷新缓存。

整个流程的关键约束在 `_managers.py:172`：「`update_memory_on_run` 与 `enable_agentic_memory` 互斥」，避免双写。

## 写入模式

Agno 有两种典型写入模式：

1. **后台模式**：`update_memory_on_run=True`，每轮运行后由 MemoryManager 从用户消息中提取可保存信息（`_managers.py:38-50`）。
2. **Agentic 模式**：`enable_agentic_memory=True`，agent 通过 `update_user_memory` tool 显式决定 add/update/delete/clear（`_default_tools.py:38` + `_messages.py:315-325`）。

后台模式的优点是上下文干扰少；agentic 模式的优点是可解释和可控。Mnemon 的 hook 设计更接近 agentic 模式：hook 提醒 agent 判断是否值得保存，然后输出候选。

## 读取与上下文预算

Agno 允许把 memories 自动加入上下文（`_messages.py:286-302`），也允许 `add_memories_to_context=False` 只收集不注入。`set_memory_manager`（`_init.py:111-114`）的默认推断是「只要 manager 存在就开自动注入」，开发者要主动关。

`search_user_memories`（`manager.py:588-638`）支持：

- `retrieval_method="last_n"`：按 `updated_at` 倒序取最后 N 条；
- `retrieval_method="first_n"`：按 `updated_at` 正序取前 N 条；
- `retrieval_method="agentic"`：把全部 memory 给 LLM，让模型挑出最相关的（`manager.py:656-669`）。

官方文档明确提到：当希望保持 agent context lean，或让 agent 显式搜索 memory 时，可以关闭自动注入。

这点对 Mnemon 很重要。Mnemon 不应默认把全部 memory 放进 prompt，而应按任务召回少量相关内容，且允许无相关内容时返回 `NONE`。

## 整理与 optimization

Agno memory optimization 的触发建议（来自官方 `working-with-memories/overview` 文档）：

- 用户已有 50+ memories；
- 即将执行高成本操作；
- 长期运行应用的周期维护。

源码 `optimize_memories`（`manager.py:793-862`）行为：

1. `get_user_memories(user_id)` 拉取全部；
2. 用 `MemoryOptimizationStrategyFactory.create_strategy(SUMMARIZE)`（`strategies/types.py:18-31`）拿到 `SummarizeStrategy`；
3. 调用 `strategy_instance.optimize(memories, model)`（`summarize.py:44-119`）：把每条 memory 编号合并成 prompt，让 LLM 写一段第三人称叙述，topics 取并集，agent_id/team_id 在一致时保留；
4. 若 `apply=True`：先 `clear_user_memories(user_id)`（`manager.py:299-332`），再批量 `db.upsert_user_memory`（`manager.py:850-857`）；
5. 返回优化后的 memory 列表。

注意 `apply=True` 是默认值，意味着开发者一不小心就会把所有 memory 折叠成一条。`SUMMARIZE` 是当前唯一策略（`strategies/types.py:11`）。

这个行为很强，适合应用框架，但在 Mnemon 中应改成 dry-run patch，而不是默认覆盖。

## Session summary 与历史

Agno 同时提供 session summary（`session/summary.py`）：

- `enable_session_summaries=False` 默认关闭（`agent.py:104`）；
- `add_session_summary_to_context` 可把摘要注入上下文（`agent.py:106`）；
- `SessionSummaryManager.last_n_runs` 与 `conversation_limit` 控制摘要范围（`summary.py:78-87`）；
- `create_session_summary` / `acreate_session_summary`（`summary.py:227, 263`）按需生成；
- summary 默认结构化为 `summary` + `topics`（`summary.py:23-27`）。

这说明「历史摘要」和「用户 memory」应分开。Mnemon 可以对应为：

- session summary：短期连续性；
- memory：稳定事实；
- skill：可复用流程；
- guideline：行为规则。

## 失败模式

源码层面可观测的失败模式：

- **50+ memories 触发 optimize 失败**：`SummarizeStrategy.optimize` 把全部 memory 字符串拼到一个 user message（`summarize.py:88-94`），数量大时单 prompt 体积可能超 model context。失败后 `optimize_memories` 仍然会先 `clear_user_memories`（`manager.py:847`）吗？不会——`apply=True` 分支在 strategy 抛错时会向上传递，`clear` 在 strategy 之后调用，所以原 memory 还在。但若 strategy 部分成功后在 `db.upsert_user_memory` 阶段断网，则会出现「清空成功、写入失败」的中间态。
- **context injection 关闭场景**：`add_memories_to_context=False` 时 `_messages.py:287` 跳过整段注入，agent 不知道 memory 存在，必须主动调 `MemoryTools.get_memories` 或 `search_user_memories`，否则 memory 形同不存在。
- **enable_agentic_memory 与 update_memory_on_run 同时为 True**：`_managers.py:172` 与 `_managers.py:210` 显式排他，自动后台路径会被静默跳过，开发者预期的「双重保险」失效。
- **db 是 AsyncBaseDb 但调用 sync API**：`optimize_memories` 在 `manager.py:816-819` 直接抛 `ValueError`；`update_memory_task` 在 `manager.py:488-491` 同样抛错。开发者必须显式选 sync/async API。
- **memory_capture_instructions 自定义后默认提示丢失**：`manager.py:969` 用 `or` 选择，自定义后默认四类（personal facts / opinions / life events / context）就不再生效，需要把默认条款手动并入。
- **空 db**：`set_memory_manager` 仅 warning（`_init.py:101`），但所有 add/delete 走 `log_warning` 后返回 None，没有显式 fail-fast。

## Run 编排时序图

以同步 `run` 流程（`_run.py:335-700`）为例：

```text
agent.run(input)
  |
  +-- _run() (line 335)
  |     |
  |     +-- 1. resolve session, hooks, dependencies
  |     +-- 2. build run_messages (system + history + user)
  |     +-- 3. iterate model + tool loop
  |     +-- 7. start_memory_future(agent, run_messages, user_id)  (line 476)
  |     |        --> agent.background_executor.submit(make_memories, ...)
  |     |              --> if update_memory_on_run and not enable_agentic_memory:
  |     |                    MemoryManager.create_user_memories(...)
  |     |                      --> create_or_update_memories(...)
  |     |                        --> deepcopy(model).response(messages, tools=[add/update/delete_memory])
  |     |                          --> _upsert_db_memory / _delete_db_memory
  |     +-- 8. start_cultural_knowledge_future
  |     +-- 9. start_learning_future
  |     +-- 10. emit run output
  |     +-- 11. wait for memory_future + cultural + learning  (line 590-598)
  |     |        --> merge_background_metrics
  |     +-- 12. persist session
  |
  +-- on error: cancel all futures (line 698-700)
```

agentic 路径不在此时序图里——它是模型在主 loop 内调用 `update_user_memory(task)`，同步执行，会阻塞当前轮，但可被审计。

## 关键常量定位

| 常量 | 值 | 出处 |
|---|---|---|
| 默认 history runs | 3 | `agent.py:563` 中 `self.num_history_runs = 3` |
| Markdown chunk_size | 5000 | `chunking/markdown.py:29` |
| Markdown overlap | 0 | `chunking/markdown.py:29` |
| Markdown split_on_headings | False | `chunking/markdown.py:29` |
| Document chunk_size | 5000 | `chunking/document.py:10` |
| Recursive chunk_size | 5000 | `chunking/recursive.py:11` |
| Fixed chunk_size | 5000 | `chunking/fixed.py:10` |
| Code chunk_size | 2048 | `chunking/code.py:30` |
| Agentic MAX_CHUNK_SIZE | 5000 | `chunking/agentic.py:11` |
| chunk_by_title new_after_n_chars | 0.8 × chunk_size | `chunking/markdown.py:208` |
| chunk_by_title combine_text_under_n_chars | chunk_size | `chunking/markdown.py:209` |
| chunk_by_title overlap | 0（强制） | `chunking/markdown.py:210` |
| MemoryManager 默认 delete | False | `manager.py:83` |
| MemoryManager 默认 clear | False | `manager.py:86` |
| MemoryManager 默认 add | True | `manager.py:85` |
| MemoryManager 默认 update | True | `manager.py:84` |
| optimize_memories `apply` 默认 | True | `manager.py:799` |
| optimize 唯一策略 | SUMMARIZE | `strategies/types.py:11` |
| 50+ memories 优化阈值 | 文档建议 | docs.agno.com/memory/working-with-memories/overview |

50 memories 这个阈值不在源码里——它是官方文档的运营建议。Mnemon 应当根据自己 user memory 的字符密度选择更小的阈值（例如 30 条或 8KB 字符）。

## 对 Mnemon 的启发

- 自动保存和自动注入应分开配置（对应 `update_memory_on_run` vs `add_memories_to_context`）。
- 50+ memories 是一个实用的整理信号，但 Mnemon 可使用更小阈值或按字符/条目数阈值。
- optimization 应默认预览，不应直接覆盖（与 `apply=True` 默认相反）。
- session summary 不应污染 durable memory，沿用 Agno 的双 manager 分层。
- Scheduler 可作为可选安装项，不是核心依赖（`SchedulerTools` 强依赖 AgentOS）。
- 「当前对话优先于历史 memory」这一条 prompt 级 guardrail（`_messages.py:303-306`）值得直接复用。
- agentic 与自动写入两条路径必须互斥，避免双写竞争。

## 参考来源

- 官方文档: [Agno Working with Memories](https://docs.agno.com/memory/working-with-memories/overview)
- 官方文档: [Agno Agent reference](https://docs.agno.com/reference/agents/agent)
- 本地源码: `/tmp/mnemon-agent-research-sources/agno/libs/agno/agno/memory/manager.py`
- 本地源码: `/tmp/mnemon-agent-research-sources/agno/libs/agno/agno/memory/strategies/summarize.py`
- 本地源码: `/tmp/mnemon-agent-research-sources/agno/libs/agno/agno/memory/strategies/types.py`
- 本地源码: `/tmp/mnemon-agent-research-sources/agno/libs/agno/agno/agent/agent.py`
- 本地源码: `/tmp/mnemon-agent-research-sources/agno/libs/agno/agno/agent/_init.py`
- 本地源码: `/tmp/mnemon-agent-research-sources/agno/libs/agno/agno/agent/_managers.py`
- 本地源码: `/tmp/mnemon-agent-research-sources/agno/libs/agno/agno/agent/_messages.py`
- 本地源码: `/tmp/mnemon-agent-research-sources/agno/libs/agno/agno/agent/_run.py`
- 本地源码: `/tmp/mnemon-agent-research-sources/agno/libs/agno/agno/agent/_default_tools.py`
- 本地源码: `/tmp/mnemon-agent-research-sources/agno/libs/agno/agno/session/summary.py`
- 本地源码: `/tmp/mnemon-agent-research-sources/agno/libs/agno/agno/knowledge/chunking/markdown.py`
- 本地源码: `/tmp/mnemon-agent-research-sources/agno/libs/agno/agno/knowledge/chunking/agentic.py`
- 本地源码: `/tmp/mnemon-agent-research-sources/agno/libs/agno/agno/tools/memory.py`
- 本地源码: `/tmp/mnemon-agent-research-sources/agno/libs/agno/agno/tools/scheduler.py`
