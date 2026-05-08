# Codex memory lifecycle 细节

## 核心判断

Codex 的 memories 是「线程提取 + 后台合并 + 生成式文件系统 memory」路线。官方文档强调 memories 默认关闭，启用后从 eligible prior threads 中提取稳定上下文，并在后台更新本地 memory files。源码快照显示它进一步分成 phase 1 extraction 和 phase 2 consolidation，并且每个步骤都有明确的 leases、watermarks、rate-limit guard 和 git baseline diff。

对 Mnemon 来说，Codex 证明了一个重要边界：必须规则放 `AGENTS.md` 或仓库文档，generated memories 只作为 recall layer。Mnemon 的 `GUIDELINE.md`/`INSTALL.md` 也应是受审查的规则层，memory 只提出候选。

## 容量常量定位

所有数字都对应到源码具体行：

| 概念 | 默认值 | 上下界 | 源码位置 |
|---|---|---|---|
| `max_rollouts_per_startup` | `2` | clamp `[1, 128]` | `codex-rs/config/src/types.rs:45, 53-54, 347-353` |
| `max_rollout_age_days` | `10` | clamp `[0, 90]` | `codex-rs/config/src/types.rs:46, 343-346` |
| `min_rollout_idle_hours` | `6` | clamp `[1, 48]` | `codex-rs/config/src/types.rs:47, 354-357` |
| `min_rate_limit_remaining_percent` | `25` | clamp `[0, 100]` | `codex-rs/config/src/types.rs:48, 358-361` |
| `max_raw_memories_for_consolidation` | `256` | clamp `[1, 4096]` | `codex-rs/config/src/types.rs:49, 51-52, 332-338` |
| `max_unused_days` | `30` | clamp `[0, 365]` | `codex-rs/config/src/types.rs:50, 339-342` |
| `project_doc_max_bytes` | `32 * 1024` | 0 表示禁用 | `codex-rs/config/src/config_toml.rs:68, 78-80, 231-232` |
| stage 1 model | `gpt-5.4-mini` | — | `codex-rs/memories/write/src/lib.rs:79` |
| stage 1 reasoning effort | `Low` | — | `codex-rs/memories/write/src/lib.rs:80-81` |
| stage 1 concurrency | `8` | — | `codex-rs/memories/write/src/lib.rs:82` |
| stage 1 lease | `3600s` | — | `codex-rs/memories/write/src/lib.rs:83` |
| stage 1 retry delay | `3600s` | — | `codex-rs/memories/write/src/lib.rs:84` |
| stage 1 thread scan limit | `5000` | — | `codex-rs/memories/write/src/lib.rs:85` |
| prune batch size | `200` | — | `codex-rs/memories/write/src/lib.rs:86` |
| stage 1 rollout token fallback | `150 000` | — | `codex-rs/memories/write/src/lib.rs:93` |
| stage 1 context window 占比 | `70%` | — | `codex-rs/memories/write/src/lib.rs:100` |
| stage 2 model | `gpt-5.4` | — | `codex-rs/memories/write/src/lib.rs:104` |
| stage 2 reasoning effort | `Medium` | — | `codex-rs/memories/write/src/lib.rs:105-106` |
| stage 2 lease | `3600s` | — | `codex-rs/memories/write/src/lib.rs:107` |
| stage 2 heartbeat | `90s` | — | `codex-rs/memories/write/src/lib.rs:109` |
| workspace diff 上限 | `4 MiB` | — | `codex-rs/memories/write/src/lib.rs:115` |
| extension 资源保留 | `7 days` | — | `codex-rs/memories/write/src/lib.rs:43` |
| memory_summary 注入 token 上限 | `5 000` | — | `codex-rs/memories/read/src/lib.rs:16` |
| MCP `list` 默认/上限 | `2 000 / 2 000` | — | `codex-rs/memories/mcp/src/backend.rs:6-7` |
| MCP `search` 默认/上限 | `200 / 200` | — | `codex-rs/memories/mcp/src/backend.rs:8-9` |
| MCP `read` token 默认 | `20 000` | — | `codex-rs/memories/mcp/src/backend.rs:10` |
| 历史文件 `history.max_bytes` | 用户配置 | 无强制默认 | `codex-rs/config/src/types.rs:165-172` |
| `model_auto_compact_token_limit` | 用户配置 | 无默认 | `codex-rs/config/src/config_toml.rs:106` |
| `tool_output_token_limit` | 用户配置 | 无默认 | `codex-rs/config/src/config_toml.rs:239` |

注意之前的口语描述「default 16, cap 128」与源码不符：`max_rollouts_per_startup` 默认是 `2`，cap 才是 `128`。这是一份保守缺省，Codex 后台每次只啃 2 个旧 thread。

## 生命周期详表

| 维度 | 观察 |
|---|---|
| 主要记忆载体 | `~/.codex/memories/` 下的 generated artifact：`memory_summary.md`、`MEMORY.md`、`raw_memories.md`、`rollout_summaries/`、`skills/`、`extensions/` |
| 项目规则载体 | `AGENTS.md`、checked-in docs、skills、hooks。required team guidance 不应只放 memories |
| 启用方式 | `[features] memories = true` 即 `Feature::MemoryTool`；默认关闭（`features/src/lib.rs:136, 791-796`） |
| 线程级控制 | `/memories` 控制当前 thread 是否使用既有 memories、是否允许它生成未来 memories；以及 toml 中的 `MemoriesToml.use_memories` / `generate_memories`（`config/src/types.rs:264-267`） |
| 写入触发 | `start_memories_startup_task`（`memories/write/src/start.rs:22-75`）在 root session start 后 `tokio::spawn` 后台任务 |
| 速率保护 | `guard::rate_limits_ok`（`memories/write/src/guard.rs:9-47`）查询后端 rate-limit 快照，primary/secondary 两个窗口都要满足 `used_percent <= 100 - min_remaining_percent` |
| Eligibility 过滤 | `INTERACTIVE_SESSION_SOURCES`（`rollout/src/lib.rs:23-30`）= CLI/VSCode/atlas/chatgpt；`claim_stage1_jobs_for_startup` 用 `max_age_days`、`min_rollout_idle_hours`、`scan_limit=5000`、`max_claimed=max_rollouts_per_startup` 过滤 |
| 排他性 | phase 1 用 stage1 job lease（3600s）防并发写同一个 rollout；phase 2 用 `try_claim_global_phase2_job`（`memories/write/src/phase2.rs:215-249`）取全局锁 |
| 长度/数量限制 | 见上节常量表 |
| 上下文限制 | `model_auto_compact_token_limit` 控制自动历史压缩阈值；`model_context_window` 可声明模型上下文；`tool_output_token_limit` 限制单工具输出进入历史的 token；`history.max_bytes` 裁剪本地 history.jsonl |
| 项目文档限制 | `project_doc_max_bytes` 限制读取 `AGENTS.md` 总字节，0 表示禁用 |
| 整理方式 | phase 2 consolidation agent 按 `consolidation.md` prompt 把 raw memories 合并到 `MEMORY.md`、`memory_summary.md`、`skills/`，并 prune 过期 rollout summary |
| 超出处理 | raw memory 候选按数量、年龄、unused days、usage/recentness 选择；上下文通过 history compaction；工具输出通过 token limit 截断 |
| 定时/后台 | 不是 cron；在 startup/resume 等时机异步后台处理，且需要 thread idle 足够久 |
| 安全边界 | 生成字段会 redact secrets；可配置 `disable_on_external_context` 让用过 MCP/web/tool search 的 thread 标记为 `polluted`，不进入 memory generation（`config/src/types.rs:262-263`） |

## 源码快照中的双阶段机制

实际代码路径（用 file:line 引用）：

1. **入口**：`memories/write/src/start.rs:22-75` 的 `start_memories_startup_task`。如果 `config.ephemeral || !MemoryTool || sub-agent` 直接返回；state DB 为空也返回。
2. **prune 老 stage1 行**：`phase1::prune`（`phase1.rs:111-132`）按 `max_unused_days` 删除过期 stage1 输出，`PRUNE_BATCH_SIZE = 200`。
3. **rate-limit guard**：`guard::rate_limits_ok` 失败则记 `skipped_rate_limit` 并退出。
4. **phase 1 主流程**（`phase1.rs:70-108`）：
   - `claim_startup_jobs` 通过 `Stage1StartupClaimParams { scan_limit, max_claimed, max_age_days, min_rollout_idle_hours, allowed_sources, lease_seconds }` 选取候选 rollout；
   - 每个 claim 进 `job::run`，通过 `stage_one_input.md` + `stage_one_system.md` 跑一次模型；
   - `output_schema()`（`phase1.rs:135-146`）强制返回 `{rollout_summary, rollout_slug, raw_memory}`；
   - `serialize_filtered_rollout_response_items`（`phase1.rs:394+`）过滤掉非 memory-relevant 的 ResponseItem，并对 secret 调用 `redact`。
   - 失败的 job 进 retry backoff (`JOB_RETRY_DELAY_SECONDS = 3600s`)，不会热循环。
5. **phase 2 主流程**（`phase2.rs:45-199`，10 步注释）：
   1. `job::claim` 拿全局锁；
   2. `prepare_memory_workspace` 确保 `~/.codex/memories/.git` baseline 存在（`codex-git-utils`）；
   3. `agent::get_config` 构造 sandbox 配置：`SandboxPolicy::WorkspaceWrite` + 禁网（`phase2.rs:295-353`）；
   4. `db.get_phase2_input_selection(max_raw_memories, max_unused_days)` 取 top-N raw memories，按 `usage_count` 降序、再按 `last_usage`/`generated_at` 排序；
   5. `sync_phase2_workspace_inputs` 重写 `raw_memories.md`、同步 `rollout_summaries/`、prune extension 老资源；
   6. `memory_workspace_diff` 用 git status 判断脏；不脏则记 `succeeded_no_workspace_changes` 并退；
   7. `write_workspace_diff` 把 git-style diff 写到 `phase2_workspace_diff.md`（4 MiB 上限）；
   8. `spawn_consolidation_agent` 启动子 agent 跑 `consolidation.md` prompt；
   9. `agent::handle` 持有 `JOB_HEARTBEAT_SECONDS = 90s` 心跳，agent 完成后 reset git baseline 并删除 diff 文件；
   10. emit metrics。
6. **read path**：`build_memory_tool_developer_instructions`（`memories/read/src/prompts.rs:28-52`）把 `memory_summary.md` 截到 5000 tokens 后渲染进 developer instructions；其他 artifact 通过 memory MCP server (`memories/mcp/`) 暴露 list/read/search 三个 read-only tool。

这套设计非常完整，但也明显比 Mnemon 第一阶段重。Mnemon 不需要复制 state DB、lease、internal consolidation agent 和 generated workspace，只需要借鉴「候选提取 -> Markdown patch -> 审查安装」。

## Hooks 契约

`codex-rs/hooks/src/events/*.rs` 与 `schema.rs` 共同定义每个事件的 input/output。下表用 Rust 结构体对应：

| 事件 | Request 字段（节选） | Outcome 字段（节选） | 主要行为 |
|---|---|---|---|
| `SessionStart` (`session_start.rs:22-53`) | `session_id`、`cwd`、`transcript_path?`、`model`、`permission_mode`、`source`(Startup/Resume/Clear) | `additional_contexts`、`should_stop`、`stop_reason?` | stdout 纯文本→`additionalContext`；JSON 出 `continue=false` 即 stop |
| `UserPromptSubmit` (`user_prompt_submit.rs:22-46`) | session/turn id、`prompt` | `additional_contexts`、`should_stop` | 注入 contextual 提醒或 block 输入 |
| `PreToolUse` (`pre_tool_use.rs`) | tool_name、tool_input、matcher_aliases、tool_use_id | `decision (allow/deny/ask)`、`reason?`、`hook_specific_output` | 工具级 guardrail，可直接拒绝执行 |
| `PermissionRequest` (`permission_request.rs`) | 同 PreToolUse + permission scope | `PermissionRequestDecision` | 把人工 approval 决策外包给脚本 |
| `PostToolUse` (`post_tool_use.rs:22-43`) | tool_name、tool_input、tool_response、tool_use_id | `additional_contexts`、`feedback_message?`、`decision (block?)` | 反馈结果或终止当前 turn |
| `PreCompact` / `PostCompact` (`compact.rs`) | compaction 触发上下文 | `StatelessHookOutcome` | 在 history 压缩前后做记录或 abort |
| `Stop` (`stop.rs:22-42`) | `stop_hook_active`、`last_assistant_message?` | `should_stop`、`should_block`、`continuation_fragments` | 让 agent 继续一轮（注入 prompt fragment）或最终结束 |

通用输出字段在 `schema.rs:60-72` 的 `HookUniversalOutputWire`：`continue`、`stopReason`、`suppressOutput`、`systemMessage`。事件特定字段挂在 `hookSpecificOutput`（每个事件 wire 都有 `deny_unknown_fields`）。Hooks 可以同时存在于 user/project/system/MDM layer，全部 matching 都会执行；信任来源由 `hook_metadata_for_config_layer_source` 决定。

## 超出与整理策略

Codex 对超出的处理不是单点截断，而是多层预算：

- **thread eligibility**：年龄 (`max_rollout_age_days=10`)、idle 时间 (`min_rollout_idle_hours=6`)、active 状态、startup 处理数量 (`max_rollouts_per_startup=2`)。
- **raw memory pool**：phase 2 选择 `max_raw_memories_for_consolidation=256` 项；忽略 `max_unused_days=30` 之外的 memory；缺 `last_usage` 时 fallback 到 `generated_at`，并按 usage_count 优先排序。
- **project instructions**：`AGENTS.md` 字节预算 32 KiB，按 root→cwd 顺序消耗预算，超出截断 + warning。
- **history**：自动 compaction (`model_auto_compact_token_limit`)、工具输出 token (`tool_output_token_limit`)、本地 history file (`history.max_bytes`) 三层。
- **consolidation**：phase 2 prompt (`consolidation.md`) 显式要求 INCREMENTAL UPDATE 模式；只在 git diff 表明 workspace 真的脏时才启动 agent，否则视为 no-op 成功；deleted rollout summary 触发 deletion-only forgetting。
- **memory_summary 注入**：再单独被 5000 tokens 截断，确保 always-loaded 内容不会爆 context。

## Eligibility 决策树

把 phase 1 的 thread 选择逻辑画成决策树（结合 `phase1.rs:148-183` 与 `state DB::claim_stage1_jobs_for_startup`）：

```text
candidate rollout
  -> source ∈ INTERACTIVE_SESSION_SOURCES?  (CLI/VSCode/atlas/chatgpt)
  -> age <= max_rollout_age_days (default 10)?
  -> idle >= min_rollout_idle_hours (default 6)?
  -> not currently leased by another phase-1 worker?
  -> within scan_limit (5000) AND under max_claimed (max_rollouts_per_startup, default 2)?
  -> memory_mode != "disabled"?
  -> memory_mode != "polluted" (when disable_on_external_context && thread used MCP/web/tool search)?
  -> session not ephemeral && session not sub-agent?
  -> rate-limit primary/secondary windows: used_percent <= 100 - min_rate_limit_remaining_percent (default 25)?
  -> all yes => claim & extract; otherwise: skipped & counted in metrics
```

每条边都对应明确的 metric 标签，便于运维。Mnemon 在做 reflection trigger 时可以借鉴这种"多门控 + 全部计数"的可观测设计。

## Phase 2 selection rank

`db.get_phase2_input_selection(max_raw_memories, max_unused_days)` 的排序口径（结合 README 与代码注释）：

1. 排除 `last_usage` 早于 `now - max_unused_days` 的行；`last_usage` 为空时 fallback 到 `generated_at`，让全新 memory 仍能进 selection。
2. 按 `usage_count` 降序优先；高频使用的 memory 优先保留。
3. 同 `usage_count` 内按 `last_usage`/`generated_at` 降序。
4. 取前 `max_raw_memories_for_consolidation` 项；超出的留在 DB 但本轮不进 staging。
5. successful Phase 2 完成时把这批行标 `selected_for_phase2 = 1` 并记录 `selected_for_phase2_source_updated_at`。
6. 后续 phase 1 的 upsert 不会清除这个 baseline，下一次 phase 2 仍能通过 git diff 看到 "上一轮选过的 vs 这一轮选的" 的差异。

排序口径意味着：(a) 旧但常用的 memory 比新但未用的优先；(b) 真正长期不用的 memory 通过 `max_unused_days` 自然失效；(c) 没有 hard delete，只有 selection 出局，和 git workspace 的"未被引用"自然 merge。

## Forgetting 机制

Codex 不做时间衰减式遗忘，而是通过 selection 出局 + workspace deletion + consolidation 反向更新：

1. **selection 出局**：phase 2 这一轮没选中的 raw memory 不写入 staging，其对应 `rollout_summaries/<slug>.md` 在 `sync_rollout_summaries_from_memories` 中被删除（`memories/write/src/lib.rs` 与 `phase2.rs:201-210`）。
2. **workspace diff**：被删除的 summary 进入 `phase2_workspace_diff.md`，consolidation prompt 显式要求按 deleted file 反查 `MEMORY.md` 中的 `### rollout_summary_files` 引用，删除支持依据已不存在的 task block。
3. **共享证据保护**：若 `MEMORY.md` block 同时引用已删除和仍存在的多个 summary，prompt 要求 split / rewrite 而非整块删除（`consolidation.md:170-172`）。
4. **memory_summary 跟随**：`MEMORY.md` 清理后再回写 `memory_summary.md`，删除已经无对应 handbook entry 的索引行。
5. **extension 资源衰减**：extension resources 7 天后被 `prune_old_extension_resources` 清理（`memories/write/src/lib.rs:43`），靠 deletion 信号引导 consolidation agent 移除依赖该资源的 memory。

这种"删除驱动的反向更新"避免了时间衰减导致的误删，但要求 selection rank 与 sync 步骤足够稳定。

## 失败模式

- **eligibility 不足**：`claim_stage1_jobs_for_startup` 返回空 → phase 1 计 `skipped_no_candidates` 并退；phase 2 仍会尝试合并已有 stage1 输出，但若 selection 也为空，会清空 `raw_memories.md` 与 `rollout_summaries/`。
- **rate-limit 不足**：guard 失败时整个 startup 任务 abort，本次启动不抽取也不合并。
- **state DB 不可用**：直接 `warn!` 然后跳过，root session 仍能正常使用旧 memory 但不会生成新 memory。
- **idle 不够久**：`min_rollout_idle_hours` 默认 6 小时；正在编辑或不久前结束的 thread 永远不会被抽取，避免和当前用户行为竞争。
- **token budget 超限**：phase 1 `DEFAULT_ROLLOUT_TOKEN_LIMIT=150000` 与 70% context window 占比保证 stage 1 prompt 不会爆 context；超长 rollout 会被截断到该上限。
- **consolidation agent 失败**：不重置 git baseline，下次 phase 2 仍会看到同样的 dirty workspace，可重试。
- **secret 泄漏**：靠 prompt 强制的 `[REDACTED_SECRET]` + phase 1 序列化前的 `sanitize_response_item_for_memories` 双层防护，但官方仍标注 "memory 永远不应存 credential"。
- **prompt injection**：`stage_one_input.md` 显式说明 rollout 内容是数据；`consolidation.md` 把 rollout 视为 immutable 证据。
- **child agent 进化**：sub-agent session 会被 `start.rs` 跳过，避免循环写 memory。

## State DB 角色

phase 1/phase 2 之间通过 SQLite state DB 传递候选与结果（`Feature::Sqlite`，`features/src/lib.rs:134`）。关键表/字段：

- **stage1_output**：每个 rollout 抽取出的 raw memory 行，包含 `thread_id`、`raw_memory`、`rollout_summary`、`rollout_slug`、`generated_at`、`last_usage`、`usage_count`、`source_updated_at`、`selected_for_phase2` 标志、`selected_for_phase2_source_updated_at`。
- **stage1_job**：claim 表，含 `ownership_token`、`lease_until`、retry backoff 计数。
- **phase2_job**：全局 lock 行，记录 `input_watermark`（claim 时已知最新输入时间）和 completion watermark（实际消费的最新输入时间）。

watermark 行为（`memories/README.md` 与 `phase2.rs:512-523` `get_watermark`）：

- 全局 phase-2 锁 **不** 用 watermark 判脏，而是用 git workspace 是否 dirty 决定是否需要再跑 agent。
- watermark 取 `claim.watermark` 与所有实际加载的 stage1 inputs 的 `source_updated_at` 最大值，避免回退。
- 这种设计让 forgetting 通过 git diff 自动反映：deleted summary 也是一个变更，consolidation agent 会读到 deletion-only diff，从而清理 `MEMORY.md` 中相应引用。

selection 规则（`README.md` 中 phase 2 段落 + `phase2.rs:92-110`）：

- 排除 `last_usage` 超过 `max_unused_days` 的 memory；
- 没有 `last_usage` 时 fallback 到 `generated_at`，让全新未使用的 memory 仍能进 selection；
- 按 `usage_count` 降序优先，相同 usage 后按 `last_usage`/`generated_at` 排序；
- 只取前 `max_raw_memories_for_consolidation` 项进入 staging。

successful Phase 2 会把它消费的 stage1 行标记 `selected_for_phase2 = 1`；下一轮 phase 1 在 upsert 同一 thread 的新输出时不会清掉这个 baseline，便于 phase 2 通过 git diff 看到"哪些 baseline 变了"。

## AGENTS.md 解析与合并次序

实战流程（按 `agents_md.rs` 行号给出）：

1. **入口**：`AgentsMdManager::user_instructions_with_fs`（`90-127`）先取 `config.user_instructions`（来自 toml `instructions` / `developer_instructions` / `model_instructions_file`），然后调 `read_agents_md`，最后视 `Feature::ChildAgentsMd` 决定是否追加 hierarchical 提示。
2. **Global**：`load_global_instructions`（`61-78`）只在 `~/.codex/` 下查 `AGENTS.override.md` → `AGENTS.md`，第一个非空就返回。它不会进入 root-to-cwd 合并，作为 caller 单独使用。
3. **root marker 收集**：`agents_md_paths`（`213-303`）从 cwd 的 canonicalized 形式开始，跳过 `Project` layer 的 marker 配置（避免循环），合并其余 layer 的 `project_root_markers`。默认 marker 列表为 `default_project_root_markers()`（仅 `.git`）。
4. **search_dirs 排序**：`266-283` 从 cwd 沿 `parent()` 走到 marker 命中目录，再 `reverse()`，得到 root → cwd。无 marker 时退化为只含 cwd 一项。
5. **per-directory 文件名**：`candidate_filenames`（`305-320`）= `[AGENTS.override.md, AGENTS.md, ...project_doc_fallback_filenames]`；同目录第一个 hit 即停。
6. **字节预算**：`read_agents_md`（`149-206`）按 root → cwd 顺序消耗 `project_doc_max_bytes`（默认 32 KiB）；超出当前 budget 的文件被截断，仍不会跨过 root 继续搜索。
7. **拼接**：每条非空内容用 `"\n\n"` 连；`user_instructions` 与 docs 之间用 `AGENTS_MD_SEPARATOR = "\n\n--- project-doc ---\n\n"`。
8. **child agents 提示**：`hierarchical_agents_message.md` 解释了 deeper > higher、prompt > AGENTS.md 的优先级关系，附在末尾让模型理解层级语义。

合并次序的语义影响：先出现的（root）通常被解释为 "general rule"，后出现的（cwd）会覆盖或细化；`Feature::ChildAgentsMd` 提示明确告诉模型 "deeper overrides higher"。这是一种依靠 prompt 而非 deterministic merger 的 conflict resolution。Mnemon 在合并多层 `GUIDELINE.md` 时也可考虑同样的 "顺序 + 提示" 组合，避免做复杂的字段级 merge。

## Hooks 与 Mnemon 四阶段

Codex hooks 支持 `SessionStart`、`UserPromptSubmit`、`PreToolUse`、`PermissionRequest`、`PostToolUse`、`PreCompact`、`PostCompact`、`Stop`（`hooks/src/lib.rs:18-27`）。其中最适合 Mnemon 的四阶段映射：

| Mnemon 阶段 | Codex hook 对应 | 作用 |
|---|---|---|
| 启动召回 (Prime) | `SessionStart` | 注入 guideline、项目 memory 索引、最近关键状态 |
| 输入前判定 (Remind) | `UserPromptSubmit` | 判断本轮是否需要 recall、是否有隐私/安全风险 |
| 工具后采样 (Nudge) | `PostToolUse` | 记录命令结果、失败原因、可复用 workflow 证据 |
| 结束沉淀 (Compact) | `Stop` + `PreCompact` | 要求 agent 总结候选 memory/skill/guideline patch；compaction 前抓最后一次状态 |

四个 hook 都可同时部署 user-level 与 project-level 实例，靠 `hook_metadata_for_config_layer_source` 区分信任。Mnemon 设计 `INSTALL.md` 时应同样区分用户级（`~/.codex/hooks.json`）和项目级（`.codex/hooks.json`），并保证两者契约相同。

## 对 Mnemon 的具体启发

- **memory 默认应是辅助召回，不替代 `GUIDELINE.md`**。
- **安装层应通过 `INSTALL.md` 让 agent 自己配置 hooks**，参考 Codex 双层 hooks 配置位置。
- **每个 hook 只做轻量提醒或产出候选**，不应强行接管 agent loop（Codex hook stdout 默认走 `additionalContext`，stop 是显式选项）。
- **memory 需要 no-op gate、secret redaction、evidence、scope (`applies_to`) 和 outdated handling**：直接照搬 `stage_one_system.md` 的 4 块结构。
- **进化提案要带 diff**：参考 `phase2_workspace_diff.md`，让 reflection prompt 接收 diff 而非全文。
- **长流程沉淀成 `SKILL.md`**，事实和偏好沉淀成 bounded memory，规范沉淀到 `GUIDELINE.md`。
- **rate-limit 与 idle guard**：Mnemon 在做后台反思时也要避免抢占当前用户操作；可借用 `min_rollout_idle_hours` 的思路。
- **forgetting 要靠 input deletion 触发**：Codex phase 2 通过 deleted summary 反查 `MEMORY.md`，而非定时清理；这降低了误删风险。
- **always-loaded 摘要要 token-bounded**：Mnemon 的 always-on guideline summary 必须设置类似 5000 tokens 的硬截断。

## 参考来源

- 官方文档: [Codex Memories](https://developers.openai.com/codex/memories)
- 官方文档: [Codex Hooks](https://developers.openai.com/codex/hooks)
- 官方文档: [Codex Config Reference](https://developers.openai.com/codex/config-reference)
- 官方文档: [AGENTS.md](https://developers.openai.com/codex/guides/agents-md)
- 本地源码: `/tmp/mnemon-agent-research-sources/codex/codex-rs/memories/README.md`
- 本地源码: `/tmp/mnemon-agent-research-sources/codex/codex-rs/memories/read/templates/memories/read_path.md`
- 本地源码: `/tmp/mnemon-agent-research-sources/codex/codex-rs/memories/write/templates/memories/stage_one_system.md`
- 本地源码: `/tmp/mnemon-agent-research-sources/codex/codex-rs/memories/write/templates/memories/consolidation.md`
- 本地源码: `/tmp/mnemon-agent-research-sources/codex/codex-rs/memories/write/src/{lib,start,phase1,phase2,guard}.rs`
- 本地源码: `/tmp/mnemon-agent-research-sources/codex/codex-rs/memories/read/src/{lib,prompts}.rs`
- 本地源码: `/tmp/mnemon-agent-research-sources/codex/codex-rs/memories/mcp/src/backend.rs`
- 本地源码: `/tmp/mnemon-agent-research-sources/codex/codex-rs/config/src/{types,config_toml}.rs`
- 本地源码: `/tmp/mnemon-agent-research-sources/codex/codex-rs/hooks/src/{lib,schema,events/*}.rs`
- 本地源码: `/tmp/mnemon-agent-research-sources/codex/codex-rs/rollout/src/lib.rs`
- 本地源码: `/tmp/mnemon-agent-research-sources/codex/codex-rs/core/src/agents_md.rs`
