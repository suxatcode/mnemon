# Codex 架构观察

## 一句话结论

Codex 是一个本地优先的 coding agent runtime：配置、项目指令、skills、hooks、memories、subagents、MCP/apps 等都被组装进一次会话的开发者上下文。它非常适合验证 Mnemon 的轻量 harness 思路，因为 Codex 官方本身就把 `AGENTS.md`、skills、hooks 和 generated memories 分成不同责任层。

## 源码地图

本地源码快照：`/tmp/mnemon-agent-research-sources/codex`。所有引用都已通过 grep/read 验证。

| 主题 | 文件 | 关键行 |
|---|---|---|
| AGENTS.md 装载与合并 | `codex-rs/core/src/agents_md.rs` | `1-78` 文件头注释解释 root-to-cwd 合并；`37-39` 默认/override 文件名常量；`82-127` 拼接 user instructions；`130-141` 列出 instruction sources；`149-206` 字节预算读取；`213-303` root marker 探测与 ancestor 收集 |
| AGENTS.md 子目录提示 | `codex-rs/core/hierarchical_agents_message.md` | `1-7` 父子覆盖与 prompt 优先级说明 |
| AGENTS.md 字节预算 | `codex-rs/config/src/config_toml.rs` | `68` `DEFAULT_PROJECT_DOC_MAX_BYTES = 32 * 1024`；`78-80` default fn；`231-232` 字段定义 |
| Memory 配置类型 | `codex-rs/config/src/types.rs` | `45-54` 默认值与上下界常量；`258-287` `MemoriesToml`；`289-321` `MemoriesConfig::default`；`323-366` toml→config 的 clamp 逻辑 |
| Memory pipeline 启动 | `codex-rs/memories/write/src/start.rs` | `22-75` `start_memories_startup_task` 跳过 ephemeral/sub-agent；先 `phase1::prune` 再做 rate-limit guard，然后顺跑 phase1/phase2 |
| Phase 1 抽取 | `codex-rs/memories/write/src/phase1.rs` | `70-108` 主流程；`110-132` `prune` 老化清理；`148-183` `claim_startup_jobs`；`135-146` 输出 schema；`394-475` 过滤与脱敏序列化 |
| Phase 2 合并 | `codex-rs/memories/write/src/phase2.rs` | `45-199` 主流程含 10 步注释；`201-210` workspace 同步；`215-249` 全局锁 claim；`295-353` consolidation agent sandbox |
| Stage 常量 | `codex-rs/memories/write/src/lib.rs` | `35-44` artifact 子目录；`46-48` extension 保留 7 天；`78-101` `stage_one`；`103-110` `stage_two`；`112-116` workspace_diff 4 MiB |
| Rate-limit guard | `codex-rs/memories/write/src/guard.rs` | `9-47` 门控逻辑；`49-64` window 比较 |
| 读取注入模板 | `codex-rs/memories/read/src/lib.rs` | `16` summary token 上限 5000；`18` `memory_root` |
| Read prompt | `codex-rs/memories/read/src/prompts.rs` | `10-15` 嵌入 `read_path.md`；`28-52` 渲染 developer instructions |
| Memory MCP backend | `codex-rs/memories/mcp/src/backend.rs` | `6-10` list/search/read 上限：list=2000、search=200、read=20000 tokens |
| Hooks 事件名清单 | `codex-rs/hooks/src/lib.rs` | `18-27` `HOOK_EVENT_NAMES` 共 8 个；`34-41` 带 matcher 的 6 个 |
| Hooks 发现 | `codex-rs/hooks/src/engine/discovery.rs` | `49-78` `discover_handlers`；`255-296` `hooks.json` 加载；`298-330` config TOML hooks 加载 |
| Hooks 事件实现 | `codex-rs/hooks/src/events/{session_start,user_prompt_submit,pre_tool_use,post_tool_use,permission_request,compact,stop}.rs` | 每个事件都有 `Request`/`Outcome`/`HandlerData` 三件套 |
| Feature flags | `codex-rs/features/src/lib.rs` | `136` `MemoryTool`；`142` `ChildAgentsMd`；`80` Claude-style hooks 注释；`791-796` memories feature 描述 |
| Rollout 来源筛选 | `codex-rs/rollout/src/lib.rs` | `23-30` `INTERACTIVE_SESSION_SOURCES`：CLI/VSCode/atlas/chatgpt |

## 架构层次

| 层 | 机制 | 作用 |
|---|---|---|
| 配置层 | `~/.codex/config.toml`、project `.codex/config.toml`、MDM、session flags | feature flags、model、hooks、memories、sandbox（多层 stack 由 `ConfigLayerStack` 合并）|
| 指令层 | `AGENTS.md`、`AGENTS.override.md`、`developer_instructions`、`model_instructions_file` | 持久项目规则与开发者约束 |
| 扩展层 | `core-skills` 加载的 `SKILL.md`、plugins、MCP/apps、`memory_extensions/<name>/instructions.md` | 可复用工具说明、外部能力、第三方 memory 信号 |
| 生命周期层 | hooks（8 个事件） | `SessionStart`/`UserPromptSubmit`/`PreToolUse`/`PostToolUse`/`PermissionRequest`/`PreCompact`/`PostCompact`/`Stop` |
| 记忆层 | `~/.codex/memories/` 下的 generated artifact + state DB | helpful recall layer，绝非项目规则 |
| 多 agent 层 | worker/explorer 等 subagent + phase 2 consolidation agent | 并行探索/实现/审查 + 记忆合并 |

## `AGENTS.md` 装载模型

`codex-rs/core/src/agents_md.rs` 的注释（行 `1-17`）和实现（行 `82-303`）描述了完整流程：

1. **全局 scope**：`AgentsMdManager::load_global_instructions`（`61-78`）按顺序尝试 `~/.codex/AGENTS.override.md`、`~/.codex/AGENTS.md`，第一个非空命中即返回。该路径不会再向 cwd 走，纯属全局守则。
2. **项目 scope**：`agents_md_paths`（`213-303`）从当前 cwd 调用 `dunce::canonicalize`，再用 `project_root_markers_from_config` 取得 marker 列表（默认仅 `.git`，行 `236-243` 的 fallback 在 `default_project_root_markers()`）。
3. **root 探测**：从 cwd 的祖先逐级检查 marker；找到第一个含 marker 的目录作为 project root；找不到则 search_dirs 退化为只含当前 cwd。
4. **search dirs 收集**：`266-283` 从 cwd 向上 `parent()` 直到 root，再 `reverse()`，得到 root→cwd 顺序。
5. **per-directory 候选文件名**：`candidate_filenames`（`305-320`）依次为 `AGENTS.override.md`、`AGENTS.md`、再加用户配置的 `project_doc_fallback_filenames`。每个目录在第一个 hit 后 `break`。
6. **总字节预算**：`read_agents_md`（`149-206`）以 `project_doc_max_bytes` 作为 budget；默认 `32 * 1024 = 32768` 字节（`config_toml.rs:68`）。budget 用尽后剩余文件被截断，并发出 warning。
7. **分隔符**：`AGENTS_MD_SEPARATOR = "\n\n--- project-doc ---\n\n"`（`agents_md.rs:43`），仅在拼接 `user_instructions` 与 docs 时插入一次。
8. **child-agents 提示**：当 `Feature::ChildAgentsMd` 启用时，会在末尾追加 `hierarchical_agents_message.md`（`agents_md.rs:33-34, 115-120`），该 markdown 解释了 deeper 文件覆盖 higher 文件、prompt 永远 outrank `AGENTS.md` 的优先级。

注意：root-to-leaf 合并意味着越接近 cwd 的内容越晚出现；下游模型若取最后赢家行为，则 nested 文件实质享有更高优先级。这与官方 docs 的描述（`Custom instructions with AGENTS.md`）一致。

## Hooks 架构

Codex hooks 模块 (`codex-rs/hooks/`) 遵循事件驱动 + 多源合并：

- **事件枚举**：`HOOK_EVENT_NAMES`（`lib.rs:18-27`）为 8 个：`PreToolUse`、`PermissionRequest`、`PostToolUse`、`PreCompact`、`PostCompact`、`SessionStart`、`UserPromptSubmit`、`Stop`。其中 6 个带 matcher（`lib.rs:34-41`）。
- **配置入口**：`engine/discovery.rs` 的 `load_hooks_json`（`255-296`）与 `load_toml_hooks_from_layer`（`298-316`）。前者读 `hooks.json`，后者从任意 config layer 提取 `hooks` 表。
- **来源识别**：`hook_metadata_for_config_layer_source`（`533-`）把 layer 来源标准化为 `HookSource::User`/`Project`/`System`/`Mdm` 等，避免 hook 跨信任域。
- **匹配与执行**：`engine/dispatcher.rs` 提供 `select_handlers` / `execute_handlers`，每条匹配都会执行；事件实现见 `events/*.rs`。
- **统一返回结构**：`schema.rs:60-72` 的 `HookUniversalOutputWire` 含 `continue`、`stopReason`、`suppressOutput`、`systemMessage`，事件特定字段挂在 `hookSpecificOutput`。
- **stdout fallback**：纯文本会被当作 `additionalContext` 注入（参见 `events/session_start.rs:163-206`）。
- **feature flag**：`Feature::*` 的 `key = "hooks"` 描述为 "Claude-style lifecycle hooks loaded from hooks.json files"（`features/src/lib.rs:80, 838`）。

这给 Mnemon 的四阶段 hook 提供了直接映射：Prime 对应 `SessionStart`，Remind 对应 `UserPromptSubmit`，Nudge 对应 `Stop` 与 `PostToolUse`，Compact 可由 `PreCompact`/`PostCompact` 接管。

## Hook 事件契约速览

每个事件在 `hooks/src/events/<name>.rs` 都按同样的 4 段结构组织：

1. `XxxRequest` 结构体记录输入字段（session_id、turn_id、cwd、transcript_path、model、permission_mode 以及事件特有字段）。
2. `XxxOutcome` 记录可能的副作用：`hook_events`（用于上报）、`should_stop`、`stop_reason`、事件特有字段（`additional_contexts`、`feedback_message`、`continuation_fragments` 等）。
3. `XxxHandlerData` 是 per-handler 中间状态。
4. `parse_completed` 把命令 stdout 解释为 `XxxOutcome`：纯文本走 `additionalContext`，JSON 必须严格匹配 schema 否则记为 `Failed`。

事件触发时机（结合 `events/*.rs` 与 codex-rs/core 的调用点）：

- `SessionStart` 在 root session 启动 / resume / clear 时触发，并附带 `source` 字段标识来源；
- `UserPromptSubmit` 在用户回车提交后、模型未开始推理前触发；
- `PreToolUse` 在 tool call 解析后、执行前触发，可拒绝 / 改写决策；
- `PermissionRequest` 在工具升级到需要审批时触发，独立于 `PreToolUse`；
- `PostToolUse` 在工具结果回归后、加入 history 前触发，可附 `feedback_message` 通知模型；
- `PreCompact` / `PostCompact` 在 history compaction 流程前后触发，让外部脚本观测 / 阻断；
- `Stop` 在模型决定结束 turn 时触发，可注入 `continuation_fragments` 让 turn 继续。

`HookSource` 标签贯穿所有事件，是审计输出的核心：每条 hook 完成事件都带 source path 与 layer 信任域。Mnemon 后续若实现 hook，可直接复用这套 source/turn/run 字段。

## Memory pipeline 概览

完整 flow：

```text
session start
  -> start_memories_startup_task (write/src/start.rs:22)
  -> phase1::prune (清理过期 stage1 输出)
  -> guard::rate_limits_ok (低于阈值跳过)
  -> phase1::run
       -> claim_startup_jobs (state DB lease)
       -> 并发抽取 (CONCURRENCY_LIMIT=8, JOB_LEASE_SECONDS=3600)
       -> 写回 stage1_output 行
  -> phase2::run
       -> try_claim_global_phase2_job (全局锁)
       -> get_phase2_input_selection(max_raw, max_unused_days)
       -> sync_rollout_summaries / rebuild_raw_memories.md
       -> memory_workspace_diff (git status 判脏)
       -> 写 phase2_workspace_diff.md
       -> 起 consolidation agent (沙箱、无网络)
       -> 重置 git baseline
       -> 标记 success
```

Read 路径只触及 `memory_summary.md` 与 `MEMORY.md`：`build_memory_tool_developer_instructions`（`memories/read/src/prompts.rs:28-52`）把截断后的 `memory_summary.md` 渲染进 developer instructions，其余 artifact 由 agent 通过 MCP 工具按需检索。

## Subagent 与 multi-agent

Codex 的 `multi-agent` 与 `multi_agent_v2` feature 提供 worker / explorer 等 subagent 模式。memory pipeline 复用同一套基础设施：

- phase 2 启动的 consolidation agent 是 sub-agent 实例，通过 `ThreadManager::spawn_consolidation_agent` 创建；
- 它运行在 `SandboxPolicy::WorkspaceWrite` + 禁网（`memories/write/src/phase2.rs:320-329`），cwd 锁定为 `memory_root`；
- 它的 collab 能力被禁用，避免再次递归生成 sub-agent；
- 它的 reasoning effort 来自 `MemoriesConfig::consolidation_model` 与 `stage_two::REASONING_EFFORT = Medium`；
- 它结束后 `memory_root` 的 git baseline 会被 reset，下一轮 phase 2 又从干净 baseline 开始判脏。

这种"用受限 sub-agent 做记忆合并"的模式比"主 agent 兼职"更安全：(a) 不消耗主 agent token；(b) 沙箱与无网络隔离；(c) 失败可重试；(d) git baseline 让结果可观测。Mnemon 第一阶段不必启动专用 sub-agent，但在长期路线上可以参考这套隔离方案。

## 与 Mnemon 设计的关系

Codex 的架构支持 Mnemon 的轻量安装方式：

- `SKILL.md` 可直接放进 `~/.codex/skills/` 或 repo 的 `.codex/skills/`，被 `core-skills` loader 消费；
- `GUIDELINE.md` 应进入 `AGENTS.md`（必须规则）或 `AGENTS.override.md`（临时局部覆盖）；
- `INSTALL.md` 可指导 Codex 自己写 `~/.codex/hooks.json` 或 `.codex/config.toml` 中的 `[hooks]` 表；
- memories 是 generated state，应当作 helpful recall，不替代 checked-in rules；
- Mnemon 的 reflection 候选输出可以被 phase 2 的 consolidation 思路借鉴：先合并到 staging diff，再让 agent 决定是否提交。

## Config layer stack

Codex 的所有配置（含 hooks 与 memories）都通过 `ConfigLayerStack` 合并。其来源定义在 `codex-app-server-protocol` 的 `ConfigLayerSource`，常见 variant（用于 hook 信任分级，见 `hooks/src/engine/discovery.rs:298-330, 533+`）：

- `System { file }` — 系统级 `config.toml`；
- `User { file }` — 用户级 `~/.codex/config.toml`；
- `Project { dot_codex_folder }` — 仓库级 `.codex/config.toml`；
- `Mdm { domain, key }` — 企业 MDM 注入；
- `LegacyManagedConfigTomlFromFile { file }` 与 `LegacyManagedConfigTomlFromMdm` — 旧 managed config 兼容；
- `SessionFlags` — 单次启动的命令行覆盖。

`agents_md_paths`（`agents_md.rs:226-235`）在搜 root marker 时会跳过 `Project` layer，避免循环依赖（项目内的 marker 配置不能影响项目根的探测），其它 layer 的 marker 配置会被合并。这是一个值得 Mnemon 借鉴的细节：当配置层和被配置对象在同一目录时，需要显式断环。

## Skill 与 plugin loader

`core-skills` 加载所有 `SKILL.md`，校验 frontmatter（YAML）后注入到主 agent 的 developer instructions。`core-plugins`、`builtin-mcps`、`apps` crate 提供 plugin 与 MCP 的发现与执行；它们都和 hooks 一样基于 layer stack，所以可以在 user/project 两层独立部署。

memory MCP server (`codex-rs/memories/mcp/`) 是 read-only：

- `list` 工具枚举 `~/.codex/memories/` 内的文件（默认/上限均为 2000 项，`backend.rs:6-7`）；
- `read` 工具读单文件，token 上限 20000（`backend.rs:10`）；
- `search` 工具支持多 query 与 windowed 模式，默认/上限 200 命中（`backend.rs:8-9`）；
- 三个 tool 的 `ToolAnnotations` 都标 `read_only(true)`（`server.rs:218, 231, 246`），从协议层防止 agent 误改 generated memory。

这套读写分离对 Mnemon 也直接适用：写路径走 reflection + review，读路径只暴露 read-only 检索接口。

## 失败模式与边界

- `project_doc_max_bytes = 0` 直接禁用 `AGENTS.md`（`agents_md.rs:152, 217`）。Mnemon 若让用户禁用项目文档，需要明确告知效果。
- 项目 doc 超出 budget 时只截断当前文件而不停止累计，所以越靠 root 的内容更容易被保留，越接近 leaf 的内容反而可能丢尾——使用者需控制每层规模。
- root marker 配置为空（`!project_root_markers.is_empty()` 失败，`agents_md.rs:245`）就放弃父目录遍历，`AGENTS.md` 收集只剩当前 cwd。
- hooks 由 layer 来源分级，user/project hooks 不会从对方继承，避免敏感执行被仓库劫持。`hook_metadata_for_config_layer_source`（`discovery.rs:533+`）确保信任标签随 layer 来源固定，无法靠 config 重写。
- memories pipeline 在 `ephemeral`/`sub-agent`/无 state DB 时早退（`start.rs:30-49`），意味着子 agent 不会自我进化，靠 root agent 的 phase 2 集中合并。
- `Feature::ChildAgentsMd` 关闭时 nested `AGENTS.md` 仍按 root-to-cwd 顺序拼接，但模型不会收到 hierarchical 提示，可能误把整个串当扁平规则。
- `disable_on_external_context` 启用后，凡用过 MCP/web/tool search 的 thread 都会被标 `polluted`，phase 1 不会从这种 thread 抽取（`config/src/types.rs:262-263`）。Mnemon 类似设计应同样标记 contaminated session。

## 容量常量速览

`AGENTS.md`、history、tool output、memory selection 各自独立的 budget：

| 对象 | 默认值 | 上下界 | 源码 |
|---|---|---|---|
| `project_doc_max_bytes` (AGENTS.md 总和) | 32 KiB | 0 表示禁用 | `config/src/config_toml.rs:68, 78-80, 231-232` |
| `model_auto_compact_token_limit` | 用户配置 | 无默认 | `config/src/config_toml.rs:106` |
| `tool_output_token_limit` | 用户配置 | 无默认 | `config/src/config_toml.rs:239` |
| `history.max_bytes` | 用户配置 | — | `config/src/types.rs:171` |
| `max_raw_memories_for_consolidation` | 256 | 1-4096 | `config/src/types.rs:49, 51-52` |
| `max_rollouts_per_startup` | 2 | 1-128 | `config/src/types.rs:45, 53-54` |
| `max_rollout_age_days` | 10 | 0-90 | `config/src/types.rs:46` |
| `max_unused_days` | 30 | 0-365 | `config/src/types.rs:50` |
| `min_rollout_idle_hours` | 6 | 1-48 | `config/src/types.rs:47` |
| `min_rate_limit_remaining_percent` | 25 | 0-100 | `config/src/types.rs:48` |
| `memory_summary` 注入 token 上限 | 5000 | — | `memories/read/src/lib.rs:16` |
| MCP `list/search/read` 默认/上限 | 2000 / 200 / 20000 tokens | — | `memories/mcp/src/backend.rs:6-10` |
| stage 1 concurrency / lease | 8 / 3600s | — | `memories/write/src/lib.rs:82-83` |
| stage 1 thread scan limit | 5000 | — | `memories/write/src/lib.rs:85` |
| stage 1 rollout token fallback / window % | 150000 / 70% | — | `memories/write/src/lib.rs:93, 100` |
| stage 2 lease / heartbeat | 3600s / 90s | — | `memories/write/src/lib.rs:107, 109` |
| workspace diff size cap | 4 MiB | — | `memories/write/src/lib.rs:115` |
| extension 资源保留 | 7 days | — | `memories/write/src/lib.rs:43` |

注意：原社区文档常说 `max_rollouts_per_startup` 默认 16，但源码实际 default 为 2（cap 才是 128）。Codex 的真实启动行为相当保守。

## 信任域与读写分离

| 域 | 写者 | 读者 | 信任级 |
|---|---|---|---|
| `~/.codex/AGENTS.md` / `AGENTS.override.md` | 用户手写 | global system instructions | 高（用户级） |
| repo 内 `AGENTS.md` 链 | 仓库维护者 | project instructions | 高（团队级） |
| `.codex/hooks.json`、`config.toml` 中 hooks | 用户/团队 | hook engine | layer 决定（System/User/Project/Mdm） |
| `~/.codex/memories/MEMORY.md`、`memory_summary.md` 等 | phase 2 consolidation agent (sandboxed) | 主 agent 通过 read prompt + MCP read-only | 中（generated，需要 citation） |
| `~/.codex/memories/raw_memories.md`、`rollout_summaries/` | phase 2 sync 步骤 | consolidation agent 输入 | 低（staging，每轮重写） |
| `~/.codex/memories/extensions/<n>/instructions.md` | extension 提供方 seed | consolidation agent | 低-中（需要明示 instructions） |

Mnemon 在设计 `GUIDELINE.md`（高信任）、`SKILL.md`（中-高信任）、`mnemon` 提取的 candidate（低-中信任，需 review）时应映射类似的信任分级，避免 generated memory 直接进入高信任面。

## 对 Mnemon 的具体启发

- **AGENTS.md 风格的多层合并** 是 markdown-only 控制面的可行最小实现。Mnemon 第一阶段不需要 yaml/json frontmatter，仅靠 root-to-cwd 拼接 + hierarchical 提示就能让模型理解优先级。
- **字节预算 + 截断 + warning** 比硬错误更友好：用户可以加内容直到接近预算，超出时只丢部分。Mnemon 在拼装 always-loaded `GUIDELINE.md` 时同样建议设置预算并 warn。
- **Hooks 必须按 layer 分级签信任**：`hook_metadata_for_config_layer_source` 让 user-level hook 不会被 project hook 覆盖。Mnemon 在让 agent 自动配置 hooks 时也应区分 user/project，避免仓库代码触发用户级敏感操作。
- **read 与 write 路径分离**：write 走 sandbox + reflection；read 走 read-only MCP + injection prompt。Mnemon 的 `mnemon recall` / `mnemon remember` / `mnemon link` 自然对应这种分离。
- **selection 排序 by usage**：Codex 用 `usage_count + last_usage` 决定哪些 memory 优先合并。Mnemon 在 reflection 选 top-K 时可以借用同样的口径，避免依赖时间衰减。
- **forgetting 通过 input deletion**：删除 staging 文件 → diff 进 prompt → handbook 反向更新。Mnemon 在做"忘掉某条 memory"时也应该走 deletion + 反查引用，而非直接 grep replace。
- **保守默认值**：Codex 默认每次启动只处理 2 个 rollout，避免 token 浪费。Mnemon 的后台 reflection 也应给出非常小的默认 batch。
- **rate-limit guard**：Codex 直接查询后端 rate-limit 决定是否跑后台任务。Mnemon 即便没有后端配额，也可以加一个"用户最近 N 分钟有交互就推迟反思"的开关。

## 参考来源

- 官方文档: [Custom instructions with AGENTS.md](https://developers.openai.com/codex/guides/agents-md)
- 官方文档: [Codex Hooks](https://developers.openai.com/codex/hooks)
- 官方文档: [Configuration Reference](https://developers.openai.com/codex/config-reference)
- 官方文档: [Codex Memories](https://developers.openai.com/codex/memories)
- 本地源码: `/tmp/mnemon-agent-research-sources/codex/codex-rs/core/src/agents_md.rs`
- 本地源码: `/tmp/mnemon-agent-research-sources/codex/codex-rs/hooks/src/`
- 本地源码: `/tmp/mnemon-agent-research-sources/codex/codex-rs/memories/`
- 本地源码: `/tmp/mnemon-agent-research-sources/codex/codex-rs/config/src/types.rs`
- 本地源码: `/tmp/mnemon-agent-research-sources/codex/codex-rs/features/src/lib.rs`
