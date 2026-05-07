# Agno memory lifecycle 细节

## 核心判断

Agno 是应用框架式 memory：开发者通过 `MemoryManager`、database、agent flags 和 tools 决定 memory 何时生成、是否进入上下文、是否由 agent 显式操作。它不像 Hermes 那样以 Markdown skills 为中心，也不像 OpenClaw 那样内置 dreaming runtime。

对 Mnemon 来说，Agno 主要提供两个经验：memory 可后台更新但不必自动注入上下文；当 memories 积累到一定数量后，需要显式 optimization。

## 生命周期详表

| 维度 | 观察 |
|---|---|
| 主要记忆载体 | DB 中的 `UserMemory`；session history；session summary；knowledge chunks。 |
| 写路径 | `update_memory_on_run=True` 时后台更新；`enable_agentic_memory=True` 时 agent 获得 `update_user_memory(task)` tool；也可使用 MemoryTools。 |
| 读路径 | `add_memories_to_context=True` 自动注入；或使用 memory tools 显式搜索/读取。 |
| 默认历史 | 如果 `num_history_messages` 和 `num_history_runs` 都未设置，默认 `num_history_runs=3`。两者都设置时使用 `num_history_runs` 并告警。 |
| 长度限制 | 未发现全局 memory char hard cap；受 DB、retrieval limit、history settings、model context 和 knowledge chunk size 约束。 |
| knowledge chunk | Markdown chunk 默认 `chunk_size=5000` chars，`overlap=0`，默认不按 headings 拆分。 |
| 搜索限制 | `search_user_memories(query=None, limit=None, retrieval_method=None)`；支持 `last_n`、`first_n`、`agentic`。 |
| 超出处理 | 自动注入 memories 会增加 token cost；官方建议用户 50+ memories、昂贵操作前、长期应用周期维护时运行 memory optimization。 |
| 整理方式 | `optimize_memories(strategy=SUMMARIZE, apply=True)` 读取全部 user memories，生成优化列表，清空并重写。 |
| 后台任务 | 非 agentic memory update 通过 thread/async task 在 run 期间后台执行；不是 cron。 |
| 定时能力 | `SchedulerTools` 可让 agent 创建 cron-like schedules，但它是通用调度工具，依赖 DB、AgentOS server、SchedulePoller，不是 memory 专用。 |
| 安全/隐私 | MemoryManager 可自定义 model 和 additional instructions，例如不保存真实姓名。 |

## 写入模式

Agno 有两种典型写入模式：

1. 后台模式：`update_memory_on_run=True`，每轮运行后由 MemoryManager 从用户消息中提取可保存信息。
2. Agentic 模式：`enable_agentic_memory=True`，agent 通过 tool 显式决定 add/update/delete/clear。

后台模式的优点是上下文干扰少；agentic 模式的优点是可解释和可控。Mnemon 的 hook 设计更接近 agentic 模式：hook 提醒 agent 判断是否值得保存，然后输出候选。

## 读取与上下文预算

Agno 允许把 memories 自动加入上下文，也允许 `add_memories_to_context=False` 只收集不注入。官方文档明确提到：当希望保持 agent context lean，或让 agent 显式搜索 memory 时，可以关闭自动注入。

这点对 Mnemon 很重要。Mnemon 不应默认把全部 memory 放进 prompt，而应按任务召回少量相关内容，且允许无相关内容时返回 `NONE`。

## 整理与 optimization

Agno memory optimization 的触发建议：

- 用户已有 50+ memories。
- 即将执行高成本操作。
- 长期运行应用的周期维护。

源码路径上，`optimize_memories` 会获取用户全部 memories，调用策略模型生成优化结果；`apply=True` 时会清空现有 memories 并写入优化后的列表。这个行为很强，适合应用框架，但在 Mnemon 中应改成 dry-run patch，而不是默认覆盖。

## Session summary 与历史

Agno 同时提供 session summary：

- `enable_session_summaries=False` 默认关闭。
- `add_session_summary_to_context` 可把摘要注入上下文。
- summary manager 可限制 `last_n_runs` 和 `conversation_limit`。

这说明「历史摘要」和「用户 memory」应分开。Mnemon 可以对应为：

- session summary：短期连续性；
- memory：稳定事实；
- skill：可复用流程；
- guideline：行为规则。

## 对 Mnemon 的启发

- 自动保存和自动注入应分开配置。
- 50+ memories 是一个实用的整理信号，但 Mnemon 可使用更小阈值或按字符/条目数阈值。
- optimization 应默认预览，不应直接覆盖。
- session summary 不应污染 durable memory。
- Scheduler 可作为可选安装项，不是核心依赖。

## 参考来源

- 官方文档: [Agno Working with Memories](https://docs.agno.com/memory/working-with-memories/overview)
- 本地源码: `/tmp/mnemon-agent-research-sources/agno/libs/agno/agno/memory/manager.py`
- 本地源码: `/tmp/mnemon-agent-research-sources/agno/libs/agno/agno/agent/agent.py`
- 本地源码: `/tmp/mnemon-agent-research-sources/agno/libs/agno/agno/agent/_messages.py`
- 本地源码: `/tmp/mnemon-agent-research-sources/agno/libs/agno/agno/session/summary.py`
- 本地源码: `/tmp/mnemon-agent-research-sources/agno/libs/agno/agno/knowledge/chunking/markdown.py`
- 本地源码: `/tmp/mnemon-agent-research-sources/agno/libs/agno/agno/tools/scheduler.py`
