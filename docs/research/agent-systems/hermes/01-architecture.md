# Hermes 架构观察

## 一句话结论

Hermes 是本次调研中最接近 Mnemon 当前设计方向的系统。它明确把 facts 放进 bounded memory，把 procedures 放进 skills，把过往 session 做 FTS5 search，把复杂任务后的经验沉淀成 `SKILL.md`。它的核心不是复杂 adapter，而是 agent 读写 Markdown 资产并在运行中改进它们。

## 关键源码证据

本地源码快照：

- Hermes Agent: `/tmp/mnemon-agent-research-sources/hermes-agent`, HEAD `04918345ea31b1106d2ee6d4f42822f4f57616ee`
- Hermes Self-Evolution: `/tmp/mnemon-agent-research-sources/hermes-agent-self-evolution`, HEAD `4693c8f0eed21e39f065c6f38d98d2a403a04095`

### 源码地图

| 文件 | 行号 | 作用 |
|---|---|---|
| `tools/memory_tool.py` | 107–462 | `MemoryStore` 类：bounded `MEMORY.md` / `USER.md`、frozen snapshot、文件锁、原子写、duplicate/threat 扫描 |
| `tools/memory_tool.py` | 465–503, 515–564 | `memory_tool` 派发函数与 `MEMORY_SCHEMA` OpenAI function-calling 描述 |
| `agent/prompt_builder.py` | 150–183 | `MEMORY_GUIDANCE` / `SESSION_SEARCH_GUIDANCE` / `SKILLS_GUIDANCE` 三段稳定 prompt 字面量 |
| `agent/prompt_builder.py` | 718–840+ | `build_skills_system_prompt`：两层缓存的 skill 索引装配，遵循 progressive disclosure |
| `agent/prompt_builder.py` | 1147–1186 | `build_context_files_prompt`：注入 AGENTS.md/SOUL.md 等项目上下文文件 |
| `agent/memory_manager.py` | 1–60 | provider sanitize 与 `<memory-context>` fence 处理，约束外部 provider 的注入边界 |
| `agent/memory_manager.py` | 190–265 | `MemoryManager` 单插件原则与 `build_system_prompt` 拼装入口 |
| `agent/memory_manager.py` | 285–456 | `prefetch_all` / `sync_all` / `on_session_end` / `on_pre_compress` 等 lifecycle hook |
| `agent/curator.py` | 56–60 | `DEFAULT_INTERVAL_HOURS = 24*7` 等 curator 默认常量 |
| `agent/curator.py` | 198–295 | `should_run_now` / `apply_automatic_transitions`，state→stale→archive 自动推进 |
| `agent/curator.py` | 302–444 | `CURATOR_DRY_RUN_BANNER` 与 `CURATOR_REVIEW_PROMPT`，决定 curator 行为宪法 |
| `tools/skill_manager_tool.py` | 111–171 | 名称、描述、内容、文件大小常量及 `ALLOWED_SUBDIRS` |
| `tools/skill_manager_tool.py` | 373–800 | `_create_skill` / `_edit_skill` / `_patch_skill` / `_delete_skill` / `_write_file` / `_remove_file` |
| `tools/skill_manager_tool.py` | 797–909 | `SKILL_MANAGE_SCHEMA` 工具描述与 enum |
| `tools/session_search_tool.py` | 5–60, 325–530 | FTS5 召回 + 辅助模型 summarization 流程 |
| `run_agent.py` | 1733–1753 | `MemoryStore` 初始化与 `load_from_disk()` 调用位置 |
| `run_agent.py` | 4963–5071 | `_build_system_prompt`：identity → guidance → memory snapshot → user snapshot → provider block → skills index → context files |
| `run_agent.py` | 10780–10810 | memory nudge 计数（每 N 轮注入一次提示） |
| `RELEASE_v0.12.0.md` | 12–60 | Autonomous Curator 默认 7 天周期，写入 `logs/curator/run.json` 与 `REPORT.md` |
| `hermes-agent-self-evolution/PLAN.md` | 460–510, 670–700 | evolvable section 列表与硬约束（size/growth/caching/preservation） |
| `hermes-agent-self-evolution/evolution/core/config.py` | 26–35 | `max_skill_size=15_000`、`max_tool_desc_size=500`、`max_param_desc_size=200`、`max_prompt_growth=0.2` |
| `hermes-agent-self-evolution/evolution/core/constraints.py` | 24–175 | hard-gate validator：size、growth、structure、test suite |

## 架构层次

```text
interfaces / messaging / CLI
  -> AIAgent loop (run_agent.py)
  -> _build_system_prompt (prompt_builder.py)
       -> DEFAULT_AGENT_IDENTITY
       -> MEMORY_GUIDANCE / SESSION_SEARCH_GUIDANCE / SKILLS_GUIDANCE
       -> MemoryStore.format_for_system_prompt('memory' | 'user') (frozen snapshot)
       -> MemoryManager.build_system_prompt() (external provider, 单插件)
       -> build_skills_system_prompt(...)
       -> build_context_files_prompt(cwd)
  -> 工具调用：memory / skill_manage / skills_list / skill_view / session_search
  -> SQLite 会话库 ~/.hermes/state.db (FTS5)
  -> ~/.hermes/skills/<name>/SKILL.md (+ references/templates/scripts/assets)
  -> Curator (auxiliary client，inactivity-triggered)
  -> Self-evolution pipeline (外部仓库, DSPy + GEPA)
```

Hermes 的核心机制可以拆成三个独立平面，彼此正交：

1. **Prompt 平面**：`prompt_builder.py` 把 identity、guidance、memory、skills、context 文件拼成系统 prompt。这一层是无状态的纯函数，在 `run_agent.py:4963` 的 `_build_system_prompt` 中被组合。
2. **存储平面**：MEMORY.md、USER.md、SKILL.md、`~/.hermes/state.db`、`~/.hermes/skills/.archive/`。所有写都走原子 rename（`MemoryStore._write_file`）或 `_atomic_write_text`，避免读到半写文件。
3. **维护平面**：Autonomous Curator（运行时 inactivity 触发，默认 7 天）和 self-evolution pipeline（离线 DSPy/GEPA）。两者都不直接动 in-flight session 的 prompt cache。

## Prompt Builder 的关键边界

`agent/prompt_builder.py:150-183` 的三段 guidance 字面量，是 Hermes 的"行为宪法"：

- `MEMORY_GUIDANCE` 强调"declarative facts"而不是"instructions"，举的反例就是"Always respond concisely ✗"。这条规则比单纯说"memory 用来存事实"更具操作性。
- `SESSION_SEARCH_GUIDANCE` 极短，只触发一种行为：用户引用过去对话时先 search，再问。
- `SKILLS_GUIDANCE` 给出量化触发条件——complex task ≥5 tool calls、tricky error、non-trivial workflow。

`run_agent.py:4963-5071` 把这三段以 `tool_guidance.append(...)` 形式无条件追加到 prompt，因此它们对 agent 是"每 session 必读"的。这与 Mnemon 想要在 `GUIDELINE.md` 里表达的 judgment 在结构上完全等价。

## Memory Snapshot 的 Frozen 模式

`tools/memory_tool.py:118-142` 显式区分两套状态：

- `_system_prompt_snapshot`：`load_from_disk()` 时一次性快照，给 system prompt 注入。
- `memory_entries` / `user_entries`：tool 调用时实时更新并落盘。

之所以这么做，注释 `tools/memory_tool.py:11-14` 写得很清楚："Mid-session writes update files on disk immediately (durable) but do NOT change the system prompt — this preserves the prefix cache for the entire session." 即写入是 durable 的，但当前 session 看到的仍是 session start 时的快照。下一次 session 才会刷新。

这个 trade-off 对 Mnemon 很有价值：写"立刻持久"不等于写"立刻可见"，前者保证不丢，后者保证 prefix cache 命中率。

## Skill 索引：两层缓存

`agent/prompt_builder.py:718-840` 的 `build_skills_system_prompt`：

1. 进程内 LRU（`_SKILLS_PROMPT_CACHE`），key 包含 skills_dir、external_dirs、tool/toolset 集合、平台、disabled 列表。
2. 磁盘快照 `.skills_prompt_snapshot.json`，由 mtime/size manifest 校验。
3. 全部 miss 才走文件系统扫描并回写快照。

只在系统 prompt 注入"Level 0"——name + description 列表。Level 1（`skill_view(name)`）和 Level 2（`skill_view(name, path)`）按需打开。这是 Hermes 实现 progressive disclosure 的具体路径。

## Profile 与隔离

`get_hermes_home()` 是动态解析（`tools/memory_tool.py:55-57` 注释解释了为什么不用模块级常量），HERMES_HOME 切换会直接改变 memory、skills、state.db 的根目录。这意味着不同 profile 天然拥有独立的 memory store、session 历史、skill 库。

对 Mnemon `store strategy` 的参考：profile 隔离不需要任何复杂层，只要把根目录解析推迟到调用点，profile 切换就是改一个环境变量的事。

## 端到端流程：一次"用户纠正"被沉淀的链路

举例追踪 `agent/prompt_builder.py:150-168` 描述的场景"用户说 don't do that again"：

1. 用户消息进入 `run_agent.py:10791` 的 user msg 队列。
2. `_build_system_prompt` 已在 session start 时拼装完成（含 `MEMORY_GUIDANCE`），注入了"Save user corrections to memory"的指令。
3. agent 决策调用 `memory(action="add", target="user", content="...")`。
4. 进入 `tools/memory_tool.py:224-267` 的 `MemoryStore.add`：
   - `_scan_memory_content` 检查 invisible unicode、prompt injection、credential exfil（`_MEMORY_THREAT_PATTERNS` 有 13 条规则）。
   - 加文件锁，重新 `_reload_target` 拉取最新条目，避免被另一个 session 的写入覆盖。
   - 如果新条目会让总长度超过 `user_char_limit=1375`，直接返回错误并附 `current_entries` 与 `usage`。
   - 否则 append + `save_to_disk`（原子 rename）。
5. 返回 JSON 给 agent，附 `usage` 百分比让模型自己感知容量。
6. 当前 session 的 system prompt 不变，frozen snapshot 还是旧的——下次 session 启动时通过 `load_from_disk` 才看到新条目。

整条链路里没有任何后台任务、向量库、embedding。只有一个文件、一把锁、一组正则。

## 端到端流程：一次"复杂任务被保存为 skill"

`prompt_builder.py:176-183` 的 `SKILLS_GUIDANCE` 定义触发条件（5+ tool calls / tricky error / non-trivial workflow）。当条件命中：

1. agent 在主循环里看到 `SKILLS_GUIDANCE`，但不会立刻动手——它会先判断任务是否真的复杂。`run_agent.py:1843-1846` 的 `_skill_nudge_interval=10` 与 `:14211-14212` 的逻辑保证如果 skill 长时间没被新建，会再追一次提示。
2. agent 调用 `skill_manage(action="create", name=..., content=<完整 SKILL.md>)`。
3. 进入 `tools/skill_manager_tool.py:373-427` 的 `_create_skill`：
   - `_validate_name` 检查 `MAX_NAME_LENGTH=64` 与 `VALID_NAME_RE`。
   - `_validate_frontmatter` 强制 description 存在且不超过 1024 chars。
   - `_validate_content_size` 检查 ≤ `MAX_SKILL_CONTENT_CHARS=100_000`。
   - `_find_skill` 检测命名冲突（含 external_dirs）。
   - 创建目录、`_atomic_write_text(skill_md, content)`。
   - `_security_scan_skill` 跑安全扫描；命中则 `shutil.rmtree` 回滚。
4. 返回 `{success, message, path, skill_md, hint}`。`hint` 字段直接告诉 agent 下一步用 `write_file` 加 references / templates / scripts。
5. 后续 agent 可以 `skill_manage(action="patch", old_string=..., new_string=...)` 在 SKILL.md 中做精准更新。
6. 下个 session 启动时 `build_skills_system_prompt` 通过两层缓存把新 skill 加入 Level 0 索引。

整个 create→patch→view 链是用纯 string IO + 路径校验实现的，没有 DB schema 迁移、没有索引重建。

## Curator 流程：从 inactivity 到 archive

`agent/curator.py` 的执行链（注释 `:1-20`）：

1. agent 主循环空闲，调用 `should_run_now`（`:198-253`）。
2. 检查 `is_paused()`、`is_enabled()`、`last_run_at + interval_hours <= now`、`min_idle_hours` 已过。
3. 通过则 fork 一个辅助 AIAgent，使用 `auxiliary.curator` 配置的 model / api_key。
4. 这个 fork 跑 `apply_automatic_transitions`：
   - 如果 anchor (last_activity 或 created_at) ≤ archive_cutoff 且非 archived → `archive_skill`（移到 `.archive/`）。
   - 否则 ≤ stale_cutoff 且 active → 设 stale。
   - 如果之前 stale 但又有活动 → 复活成 active。
5. 然后跑 `CURATOR_REVIEW_PROMPT`（`:329-444`），这段 prompt 是 Hermes 行为最复杂的字面量之一：
   - 强制 umbrella-first（"would a human maintainer write this as N skills, or one with N subsections"）。
   - 三种合并方式：merge into existing umbrella / create new umbrella / demote to references|templates|scripts。
   - 强制结构化 YAML 输出 `consolidations:` / `prunings:`，区分"被合并 vs 被剪枝"。
6. 写报告：`logs/curator/<YYYYMMDD-HHMMSS>/run.json` 与 `REPORT.md`。
7. 更新 `~/.hermes/skills/.curator_state`（`load_state` / `save_state`，`:81-115`）。

注意三条不变量（注释 `:15-19`）：

- 只动 agent-created skills（bundled 与 hub 安装的不动）。
- 永不 delete，最多 archive（可恢复）。
- pinned skill 跳过自动转移。

这套设计对 Mnemon 的 `mnemon review` 命令几乎是 1:1 模板：

- 用辅助 client 执行；
- inactivity-triggered 而非 cron；
- 只产出可审查 diff 与结构化 YAML；
- 不可逆操作走"archive"语义而不是真删；
- 用户 pin 的 skill / memory 跳过自动整理。

## 对 Mnemon 的具体启发

- **三段 guidance 直接可借鉴**：`prompt_builder.py:150-183` 字面量的结构（save / not-save / 用 declarative 而非 imperative）就是 Mnemon `GUIDELINE.md` 写作模板。
- **frozen snapshot vs live state**：写盘和注入解耦，前者保证不丢、后者保证 prefix cache 不动，下个 session 自动刷新。
- **progressive disclosure 三层**：list → SKILL.md → 引用文件，对应 Mnemon 的 `recall` 应当默认只返 metadata。
- **profile = 根目录**：不要在 store 上加 namespace 字段，只要解析根目录的函数支持 env 覆盖即可。
- **维护任务用辅助 client**：curator 在 `agent/curator.py:18-19` 注释明确"never touches the main session's prompt cache"。Mnemon 的 `mnemon review` 也应当走单独 LLM 客户端。
- **size limit 写在配置里**：Hermes 的 2200/1375 是 `MemoryStore.__init__` 默认值（`tools/memory_tool.py:118`），可被 `mem_config` 覆盖（`run_agent.py:1748-1749`）。Mnemon 同样应允许 user 改阈值而非硬编码。

## 参考来源

- 本地源码: `hermes-agent/README.md`
- 本地源码: `hermes-agent/agent/prompt_builder.py`
- 本地源码: `hermes-agent/agent/memory_manager.py`
- 本地源码: `hermes-agent/agent/curator.py`
- 本地源码: `hermes-agent/run_agent.py`
- 本地源码: `hermes-agent/tools/memory_tool.py`
- 本地源码: `hermes-agent/tools/skill_manager_tool.py`
- 本地源码: `hermes-agent/tools/session_search_tool.py`
- 本地源码: `hermes-agent/website/docs/user-guide/features/memory.md`
- 本地源码: `hermes-agent/website/docs/user-guide/features/skills.md`
- 本地源码: `hermes-agent/RELEASE_v0.12.0.md`
- 本地源码: `hermes-agent-self-evolution/PLAN.md`
- 本地源码: `hermes-agent-self-evolution/evolution/core/config.py`
- 本地源码: `hermes-agent-self-evolution/evolution/core/constraints.py`
- 公开站点: [Hermes Agent](https://hermes-ai.net/)
