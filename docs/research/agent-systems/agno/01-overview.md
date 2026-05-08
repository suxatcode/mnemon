# Agno 概览

## 一句话结论

Agno 是 agent framework/library，不是一个以 Markdown 行为资产为中心的 coding runtime。它的 memory 主要通过 `MemoryManager`、agent config flags、session summaries 和 knowledge readers 实现。它适合作为「库式 memory capability」参考，但不如 Hermes/Codex/Claude Code 贴近 Mnemon 的 Markdown harness 方向。

## 源码地图

本地源码：`/tmp/mnemon-agent-research-sources/agno`，所有 file:line 引用以本快照为准。

| 关注点 | 文件:行 | 观察 |
|---|---|---|
| MemoryManager 类 | `libs/agno/agno/memory/manager.py:45` | dataclass，封装 read/write/search/optimize 全部行为 |
| MemoryManager.__init__ | `libs/agno/agno/memory/manager.py:76` | 默认 `delete_memories=False`、`add_memories=True`、`update_memories=True`、`clear_memories=False` |
| MemoryManager.update_memory_task | `libs/agno/agno/memory/manager.py:481` | agentic memory 的总入口，被 `update_user_memory` tool 调用 |
| MemoryManager.optimize_memories | `libs/agno/agno/memory/manager.py:793` | 显式合并策略，`apply=True` 时清空并重写 |
| MemoryManager.search_user_memories | `libs/agno/agno/memory/manager.py:588` | 支持 `last_n` / `first_n` / `agentic` 三种检索 |
| Memory 系统提示模板 | `libs/agno/agno/memory/manager.py:958` | 含 `<memories_to_capture>`、`<existing_memories>` 段落与第三人称写入规则 |
| 后台 memory future | `libs/agno/agno/agent/_managers.py:180` | `start_memory_future` 提交 `make_memories` 到 thread pool |
| 后台 memory async task | `libs/agno/agno/agent/_managers.py:139` | `astart_memory_task` 走 `asyncio.create_task` |
| make_memories 写入逻辑 | `libs/agno/agno/agent/_managers.py:29` | 仅当 `update_memory_on_run=True` 才调用 `create_user_memories` |
| update_user_memory tool | `libs/agno/agno/agent/_default_tools.py:38` | agent 主动写入入口，task 字符串透传给 `update_memory_task` |
| MemoryTools 工具集 | `libs/agno/agno/tools/memory.py:13` | 暴露 `think` / `get_memories` / `add_memory` / `update_memory` / `delete_memory` / `analyze` |
| 系统消息中 memory 注入 | `libs/agno/agno/agent/_messages.py:286` | `add_memories_to_context=True` 时把 `<memories_from_previous_interactions>` 写入 system prompt |
| agentic memory 提示注入 | `libs/agno/agno/agent/_messages.py:315` | 加入 `<updating_user_memories>` 块解释何时调用 `update_user_memory` |
| set_memory_manager | `libs/agno/agno/agent/_init.py:99` | 没传 manager 时构造默认 `MemoryManager(model=agent.model, db=agent.db)` |
| Agent flags 默认值 | `libs/agno/agno/agent/agent.py:104-126` | `enable_session_summaries=False`、`enable_agentic_memory=False`、`update_memory_on_run=False` |
| history 默认 3 runs | `libs/agno/agno/agent/agent.py:556-563` | 当 `num_history_runs` 与 `num_history_messages` 都未设置时硬编码 `num_history_runs = 3` |
| SessionSummaryManager | `libs/agno/agno/session/summary.py:62` | 支持 `last_n_runs`、`conversation_limit`，需要 `enable_session_summaries=True` |
| Markdown chunking | `libs/agno/agno/knowledge/chunking/markdown.py:29` | `chunk_size=5000`、`overlap=0`、`split_on_headings=False` |
| 通用 chunking 默认 5000 | `libs/agno/agno/knowledge/chunking/{document,recursive,fixed}.py:10` | 多种 chunker 共用 5000 字符默认 |
| AgenticChunking 上限 | `libs/agno/agno/knowledge/chunking/agentic.py:11` | `MAX_CHUNK_SIZE = 5000` |
| Memory 优化策略枚举 | `libs/agno/agno/memory/strategies/types.py:8` | 当前只有 `SUMMARIZE` 一种 |
| SummarizeStrategy | `libs/agno/agno/memory/strategies/summarize.py:15` | 把所有 memory 合并成一条第三人称叙述 |
| SchedulerTools | `libs/agno/agno/tools/scheduler.py:29` | 通用 cron 调度工具，依赖 AgentOS 与 SchedulePoller |

## 架构层次

Agno 典型 agent 由以下能力组合：

- model；
- tools；
- storage（`db`，可同步或异步）；
- memory（`MemoryManager`）；
- session summary（`SessionSummaryManager`）；
- knowledge base（reader + chunking + vectordb + embedder）；
- markdown output rendering；
- OS/API routers。

memory 是一个可选 capability。开发者通过几组参数决定写入与读取路径：

- `update_memory_on_run`（`agent.py:122`）：每轮结束后由 framework 后台抽取并写入 user memory。
- `enable_agentic_memory`（`agent.py:120`）：注册 `update_user_memory` tool，由 agent 主动决定写入。
- `add_memories_to_context`（`agent.py:126`）：把现有 memory 自动注入 system message。
- `enable_session_summaries`（`agent.py:104`）：启用 session 级摘要管理器。
- `add_history_to_context` + `num_history_runs/num_history_messages`（`agent.py:134-138`）：把最近若干轮原始消息塞进 prompt。

## MemoryManager 与 agentic memory 的区分

Agno 的 memory 写路径有两条互斥的入口：

1. **MemoryManager 自动写**：`update_memory_on_run=True` 时，每次 run 内由 `_managers.start_memory_future`（`_managers.py:180`）或 `astart_memory_task`（`_managers.py:139`）启动后台任务，调用 `make_memories` → `MemoryManager.create_user_memories`。该路径在 `_managers.py:172` 与 `_managers.py:210` 显式判断 `not agent.enable_agentic_memory`，即 agentic 模式启用时 framework 不再自动写。
2. **Agent 主动写**：`enable_agentic_memory=True` 时，`get_update_user_memory_function`（`_default_tools.py:38`）把 `update_user_memory(task)` 注册为可调用工具，agent 通过自然语言 task 触发 `MemoryManager.update_memory_task`（`manager.py:481`），后者再调度 `add_memory` / `update_memory` / `delete_memory` / `clear_memory` 子工具（提示模板见 `manager.py:1013-1020`）。

二者的关键差异：

- 自动模式不暴露给模型，模型不知道什么被写入；
- agentic 模式有完整工具调用记录，可以审计；
- 自动模式只能从 user message 抽取（`_managers.py:36-50`），agentic 模式可以基于完整对话决定。

Mnemon 的 hook 设计更接近 agentic 模式：在关键阶段提醒 LLM 自己生成 candidate 写入，而不是 framework 偷偷写。

## 启动路径

Agent 初始化由 `initialize_agent`（`_init.py:240-264`）按固定顺序触发：

1. `set_default_model`（`_init.py:66`）：未提供则用 `OpenAIResponses(id="gpt-5.4")`；
2. `set_debug` / `set_id` / `set_telemetry`；
3. `set_memory_manager`（`_init.py:99`）：仅当 `update_memory_on_run` / `enable_agentic_memory` / 用户已传 manager 三者之一为真时；
4. `set_culture_manager`、`set_session_summary_manager`、`set_compression_manager`、`set_learning_machine`：各自独立 flags 控制；
5. `add_history_to_context` 与 `num_history_runs/num_history_messages` 在 `agent.py` 构造期已经处理。

这种「按需构造」让默认 agent 几乎无后台开销。Mnemon 的 install 流程也可以借鉴：默认不开启 reflection/scheduling，明确 install 阶段才触发。

## 记忆类别

Agno 把可保留状态分成至少四层，对应不同 manager：

1. **User memories**：`UserMemory` schema，存于 `db.upsert_user_memory`（`manager.py:566`），第三人称偏好与事实。
2. **Session summaries**：`SessionSummary`（`session/summary.py`），结构化摘要，含 `summary` 与 `topics`。
3. **Session history**：原始消息，按 `num_history_runs` / `num_history_messages` 注入。
4. **Knowledge chunks**：长文档经 chunking + embedder + vectordb 提供检索，与 user memory 不混合。

此外还有 cultural knowledge（`CultureManager`）和 learning machine（`LearningMachine`），后者在 `_init.py:117` 被设置为可选组件。

## 默认提示模板速查

为了便于 Mnemon 设计 prompt 时直接对照，下面把 Agno 在三种 flag 组合下的 system prompt 关键差异汇总到一张表（实际拼接见 `_messages.py:286-326`）：

| 组合 | system prompt 是否含 `<memories_from_previous_interactions>` | 是否含 `<updating_user_memories>` | 后台是否抽取 memory |
|---|---|---|---|
| 默认（全 False） | 否 | 否 | 否 |
| 仅 `add_memories_to_context=True` | 是 | 否 | 否 |
| 仅 `update_memory_on_run=True` | 是（`set_memory_manager` 自动开 `add_memories_to_context`） | 否 | 是 |
| 仅 `enable_agentic_memory=True` | 是 | 是 | 否（被 `_managers.py:172` 排他） |
| `update_memory_on_run=True` 且 `enable_agentic_memory=True` | 是 | 是 | **否**（agentic 排他后台路径） |

`set_memory_manager`（`_init.py:111-114`）的逻辑是：只要 `update_memory_on_run` 或 `enable_agentic_memory` 或者用户已传 `memory_manager` 三者任一为真，就把 `add_memories_to_context` 默认置为 True。开发者要显式 `add_memories_to_context=False` 才能关掉自动注入。

## Markdown 用法

Agno 中 Markdown 不是核心行为控制层，它的位置主要是数据 pipeline：

- `MarkdownReader`（`libs/agno/agno/knowledge/reader/markdown_reader.py:23`）读取 `.md`/`.markdown` 文件；
- `MarkdownChunking`（`chunking/markdown.py:16`）把内容按结构切块，默认 `chunk_size=5000`、`overlap=0`、`split_on_headings=False`；
- response 渲染允许 markdown 输出；
- API schema 中有 markdown flag 控制返回格式。

这与 Mnemon 目标不同：Mnemon 希望 Markdown 同时承担 install contract、skill、guideline 和 reviewed evolution artifact，是行为契约，而不是一种数据格式。

## 对 Mnemon 的具体启发

可参考：

- memory flags 默认关闭（`agent.py:104,120,122`），开发者必须显式开启，避免「装上 framework 就开始写」的副作用；
- agentic memory tool 明确暴露给 agent（`_default_tools.py:38`），可被审计、可被禁用；
- 自动写入路径排他于 agentic（`_managers.py:172`），避免双写冲突；
- session summary 与 user memory 分层（`_init.py:159` 与 `_init.py:99`），短期连续性与稳定事实由不同 manager 负责；
- Markdown chunking 默认 5000 chars，作为知识检索的合理切片大小，可作为 Mnemon 引入 markdown ingestion 时的参考阈值；
- `optimize_memories` 提供一种「显式整理」的 API（`manager.py:793`），与「写入时不整理、整理时显式触发」理念一致。

不适合作为第一阶段模板：

- memory 由 framework 参数和 Python object 控制，不暴露给非 Python runtime；
- 缺少通用 `INSTALL.md`/`GUIDELINE.md` 风格的行为契约；
- `optimize_memories(apply=True)` 默认会清空再写（`manager.py:847`），强但激进，Mnemon 应改成 dry-run patch；
- 自进化更多依赖开发者工程集成（修改 agent 代码、调 manager），而非 agent 自行读取 Markdown 安装新行为。

## UserMemory schema 与存储约束

Agno 的 user memory 落到 `UserMemory`（`db.schemas`），关键字段包括 `memory_id`、`memory`、`topics`、`user_id`、`agent_id`、`team_id`、`updated_at`。`MemoryManager.add_user_memory`（`manager.py:211-242`）对这些字段的处理：

- `memory_id` 缺省时由 `uuid4()` 生成（`manager.py:225-228`）；
- `user_id` 缺省时使用字符串 `"default"`（`manager.py:230-232`），意味着多用户场景必须显式传 user_id，否则会汇到一个用户名下；
- `updated_at` 缺省时取 `now_epoch_s()`（`manager.py:234-235`），用于 `last_n` / `first_n` 排序。

`MAX_UNIX_TS = 2**63 - 1`（`manager.py:774`）作为 sentinel：在 `_get_last_n_memories` 排序时，没有 `updated_at` 的 memory 视为最新，避免因为缺时间戳被排到最旧。Mnemon 设计字段时也应当有类似的「未知 = 最新」或「未知 = 最旧」的明确约定。

## SchedulerTools 与 Mnemon 定时能力对照

`SchedulerTools`（`tools/scheduler.py:29-90`）通过 `create_schedule(cron, ...)` / `list_schedules` / `update_schedule` / `delete_schedule` 提供给 agent 创建 cron 任务的能力，但它的运行依赖：

- 数据库（`scheduler` 相关表）；
- AgentOS server；
- `SchedulePoller`（`agno.scheduler.manager.ScheduleManager` 系列）。

这意味着 Agno 的「自动定时整理」其实需要一整套服务化基础设施。对于 Mnemon 这类单机 CLI，可以借鉴 `SchedulerTools` 的工具命名，但实现可以是 `cron` / `launchd` / 手动 `mnemon dream` 命令，不必引入持续轮询进程。

## 失败模式

Agno 在以下场景容易失败或行为不直观：

- **enable_agentic_memory + update_memory_on_run 同时为 True**：自动后台路径会被 `_managers.py:172` 显式跳过，但开发者经常以为两者叠加，结果发现自动模式静默失效。`_managers.py:210` 同步路径同样有这一判断，行为一致。
- **未提供 db**：`set_memory_manager` 在 `_init.py:101` 仅 `log_warning("Database not provided. Memories will not be stored.")`，不抛错，结果是 manager 创建出来但 `add_user_memory` 全部走 `log_warning` 分支并返回 None（`manager.py:241`）。所有读路径返回 `[]`，agent 的对话不会出错，但 memory 静默丢失。
- **add_memories_to_context 未关闭 + 50+ memories**：所有 memory 直接拼到 system prompt（`_messages.py:300` 在 `for _memory in user_memories: system_message_content += f"\n- {_memory.memory}"`），token 成本线性增长，必须人工调用 `optimize_memories`。
- **`apply=True` 的 optimize**：`manager.py:847` 先 `clear_user_memories` 再 upsert 优化结果，过程中崩溃会丢数据，没有事务回退。`SUMMARIZE` 是当前唯一策略（`strategies/types.py:11`），不可选保留高频 memory。
- **同时设置 `num_history_runs` 与 `num_history_messages`**：`agent.py:557-561` 会 warning 并强制使用 `num_history_runs`，把 `num_history_messages` 置为 None。开发者预期的 message 数量被忽略。
- **同步 manager 调异步 db**：`manager.py:488-491`、`manager.py:816-819` 等多处显式 `raise ValueError` 要求改用 `aupdate_memory_task`、`aoptimize_memories`，不会自动适配。
- **agentic memory tool 但模型不调用**：当 prompt 中加入 `<updating_user_memories>` 块（`_messages.py:315-325`）后，模型仍可能选择不调用 `update_user_memory`，无法用 framework 强制。

## 后台执行模型

Agno 支持两套并发模型，由 sync/async 路径决定：

- **同步路径**：`agent.background_executor` 是 `concurrent.futures.ThreadPoolExecutor`，`start_memory_future`（`_managers.py:213`）调用 `submit`，主线程在 `_run.py:590` 用 `wait_for_open_threads` 等待；
- **异步路径**：用 `asyncio.create_task`（`_managers.py:175`），主协程在 `_run.py:1679` 等待。

错误处理：`_run.py:698-700` 在主流程异常时显式 `cancel()` 所有 background futures，但同步线程 future 的 `cancel()` 只对未启动的有效，已启动的 memory 写入会继续执行——可能导致「主流程失败但 memory 已落库」的情况。Mnemon 的 hook 阶段如果异步执行 reflection，应当显式记录哪些写入已生效，避免这种孤儿状态。

## 与 Mnemon 现有设计的对照

Mnemon 的 hook 阶段（experience → remember/recall/link → reflection → candidate patch）相比 Agno 有几个对应关系：

| Mnemon 概念 | Agno 对应 | 差异 |
|---|---|---|
| `mnemon remember` CLI | `update_user_memory` tool（`_default_tools.py:38`） | Agno 是进程内函数，Mnemon 是子进程 CLI，跨 runtime |
| `mnemon recall` CLI | `search_user_memories`（`manager.py:588`） | Agno 由 framework 注入 system prompt，Mnemon 由 agent 显式查 |
| `INSTALL.md` / `GUIDELINE.md` | system prompt + `additional_instructions`（`manager.py:55`） | Mnemon 是 reviewable 文档，Agno 是 Python 字符串 |
| `SKILL.md` | 无直接对应（`Skills`/`agno.skills` 是 Python class） | Agno 把 skill 工程化成对象，Mnemon 把 skill markdown 化 |
| review/install 闸门 | 无 | Agno 后台直接写库，没有人工 review 阶段 |
| candidate patch | 无 | Agno 直接覆盖，无 dry-run patch 概念 |

这表明 Agno 适合「服务化 agent runtime」，Mnemon 适合「单机 markdown harness」。两者目标不同，但 Agno 的 prompt guardrail、写入路径互斥、显式 optimization API 都可以直接迁移到 Mnemon 的设计语言里。

## 参考来源

- 本地源码: `libs/agno/agno/agent/_init.py`
- 本地源码: `libs/agno/agno/agent/_default_tools.py`
- 本地源码: `libs/agno/agno/agent/_managers.py`
- 本地源码: `libs/agno/agno/agent/_messages.py`
- 本地源码: `libs/agno/agno/agent/agent.py`
- 本地源码: `libs/agno/agno/memory/manager.py`
- 本地源码: `libs/agno/agno/memory/strategies/summarize.py`
- 本地源码: `libs/agno/agno/memory/strategies/types.py`
- 本地源码: `libs/agno/agno/session/summary.py`
- 本地源码: `libs/agno/agno/knowledge/chunking/markdown.py`
- 本地源码: `libs/agno/agno/tools/memory.py`
- 本地源码: `libs/agno/agno/tools/scheduler.py`
- 官方文档: [Agno Memory](https://docs-v1.agno.com/agents/memory)
- 官方文档: [Agno Working with Memories](https://docs.agno.com/memory/working-with-memories/overview)
- 官方文档: [Agno Agent reference](https://docs.agno.com/reference/agents/agent)
