# Hermes memory lifecycle 细节

## 核心判断

Hermes 是最接近 Mnemon 当前思路的系统：bounded Markdown facts、skills as procedures、session search for ephemeral history、background curator for skill library。它没有把记忆系统做成厚重数据库 adapter，而是让 agent 通过 Markdown 和工具自己维护行为资产。

这与 Mnemon 的目标高度一致：`GUIDELINE.md` 负责初始行为原则，`INSTALL.md` 说明如何安装 hooks，`SKILL.md` 承载 workflow，memory 只保存 durable facts。

## 源码地图：所有数字都能定位到常量

| 数字 / 阈值 | 含义 | 源码位置 |
|---|---|---|
| 2,200 chars | `MEMORY.md` 默认 char 上限 (~800 tokens) | `tools/memory_tool.py:118` (`memory_char_limit=2200`) |
| 1,375 chars | `USER.md` 默认 char 上限 (~500 tokens) | `tools/memory_tool.py:118` (`user_char_limit=1375`) |
| `\n§\n` | 条目分隔符 | `tools/memory_tool.py:59` (`ENTRY_DELIMITER`) |
| 80% | consolidation 建议阈值 | `website/docs/user-guide/features/memory.md:143` |
| 64 chars | skill name 长度上限 | `tools/skill_manager_tool.py:111` (`MAX_NAME_LENGTH=64`) |
| 1,024 chars | skill description 长度上限 | `tools/skill_manager_tool.py:112` (`MAX_DESCRIPTION_LENGTH=1024`) |
| 100,000 chars | SKILL.md 内容上限 (~36k tokens at 2.75 chars/token) | `tools/skill_manager_tool.py:164` (`MAX_SKILL_CONTENT_CHARS=100_000`) |
| 1,048,576 bytes (1 MiB) | 单个 skill 支持文件大小上限 | `tools/skill_manager_tool.py:165` (`MAX_SKILL_FILE_BYTES=1_048_576`) |
| `references/`, `templates/`, `scripts/`, `assets/` | skill 子目录白名单 | `tools/skill_manager_tool.py:171` (`ALLOWED_SUBDIRS`) |
| 7 days | curator 默认间隔 | `agent/curator.py:56` (`DEFAULT_INTERVAL_HOURS=24*7`) |
| 2 hours | curator 触发前最小空闲时间 | `agent/curator.py:57` (`DEFAULT_MIN_IDLE_HOURS=2`) |
| 30 days | skill stale 阈值 | `agent/curator.py:58` (`DEFAULT_STALE_AFTER_DAYS=30`) |
| 90 days | skill archive 阈值 | `agent/curator.py:59` (`DEFAULT_ARCHIVE_AFTER_DAYS=90`) |
| 10 turns | memory nudge 间隔 | `run_agent.py:1736` (`_memory_nudge_interval=10`) |
| 10 iters | skill nudge 间隔 | `run_agent.py:1843` (`_skill_nudge_interval=10`) |
| 15,000 chars | self-evolution skill 体积目标 | `evolution/core/config.py:26` (`max_skill_size=15_000`) |
| 500 chars | tool description 上限 | `evolution/core/config.py:27` (`max_tool_desc_size=500`) |
| 200 chars | tool parameter description 上限 | `evolution/core/config.py:28` (`max_param_desc_size=200`) |
| 20% | prompt section 演化最大增长率 | `evolution/core/config.py:29` (`max_prompt_growth=0.2`) |
| 300s | 演化候选 pytest gate timeout | `evolution/core/constraints.py:62` (`timeout=300`) |

这张表是回答"那个数字哪儿来"的唯一来源。文档内任何提到限制时都应当能 ground 到上表。

## 生命周期详表

| 维度 | 观察 |
|---|---|
| 主要记忆载体 | `~/.hermes/memories/MEMORY.md` 与 `~/.hermes/memories/USER.md`，路径由 `get_memory_dir()` 解析（`tools/memory_tool.py:55-57`），随 `HERMES_HOME` 切换。 |
| 文件语义 | `MEMORY.md` 存环境/项目/事实/决策；`USER.md` 存用户偏好和画像。判别标准在 `MEMORY_SCHEMA` 的 description（`tools/memory_tool.py:533-538`）。 |
| 长度限制 | `MEMORY.md` 默认 2,200 chars；`USER.md` 默认 1,375 chars；二者均可被 config `memory.memory_char_limit` / `memory.user_char_limit` 覆盖（`run_agent.py:1747-1750`）。 |
| 条目格式 | 条目用 `\n§\n` 分隔；header 显示 percent + char count（`_render_block`，`tools/memory_tool.py:393-409`）。 |
| 加载时机 | session start 由 `MemoryStore.load_from_disk()` 注入为 frozen prompt snapshot；mid-session 写入持久化但不刷新当前 system prompt。 |
| 写路径 | agent 调 `memory` tool 的 add/replace/remove；无 read action（系统 prompt 已含 snapshot）。`MemoryStore._reload_target` 在锁内重新读盘以避免并发覆盖。 |
| 超出处理 | add 超限返回 `{"success": false, "error": "...", "current_entries": [...], "usage": "..."}` (`tools/memory_tool.py:250-261`)；agent 必须 consolidate / replace / remove 后再添加。 |
| 整理建议 | 文档建议超过 80% capacity 时主动 consolidation（`memory.md:143`）；流程性内容禁止进 memory，转入 skills（`MEMORY_GUIDANCE`，`prompt_builder.py:160-167`）。 |
| 重复处理 | exact duplicate 静默成功，附 message "Entry already exists" (`tools/memory_tool.py:243-244`)。 |
| 安全处理 | `_scan_memory_content` 在 add/replace 前跑 invisible unicode 与 13 条 threat regex（`tools/memory_tool.py:67-104`）。 |
| 历史召回 | `session_search` 走 SQLite FTS5 + 辅助模型 summarization（`tools/session_search_tool.py:325-530`），独立于 durable memory。 |
| skill 存储 | `~/.hermes/skills/<skill>/SKILL.md` (+ references/templates/scripts/assets)；可叠加 `skills.external_dirs` 只读外挂（`prompt_builder.py:731-737`）。 |
| skill 限制 | name ≤64、description ≤1024、SKILL.md ≤100,000 chars、单文件 ≤1 MiB；演化 pipeline 还加 15KB / 500 / 200 软目标 + 20% growth 限。 |
| 定时任务 | v0.12.0 引入 Autonomous Curator，inactivity-triggered（`agent/curator.py:5-7`），默认 7 天周期、2 小时空闲门槛，写 `logs/curator/<run>/run.json` 与 `REPORT.md`。 |
| 行为 nudge | `run_agent.py:10783-10789` 每 10 turn 在系统 prompt 后追加一段 memory 提醒；skills 同样 10 iter 一次（`14211-14212`）。 |

## 写入规则

`prompt_builder.py:150-168` 的 `MEMORY_GUIDANCE` 强制三类信息分流：

- durable facts → `MEMORY.md` / `USER.md`；
- procedures / workflows → skill；
- task progress / session outcomes / TODO → 不写 durable memory，需要时用 `session_search`。

并明确"declarative vs imperative"：例 `User prefers concise responses ✓` / `Always respond concisely ✗`。原因写在原 prompt 里："Imperative phrasing gets re-read as a directive in later sessions and can cause repeated work or override the user's current request."

这正是 Mnemon 需要的分层。"用户纠正""工具坑点""稳定偏好""环境事实"进 memory；"如何执行某类任务"进 skill；"本轮做到哪里"只作短期状态或 session artifact。

## 溢出与 consolidation

`MemoryStore.add` (`tools/memory_tool.py:224-267`) 的实际 reject 流程：

1. content 非空校验。
2. `_scan_memory_content`（threat regex + invisible unicode）。
3. 进 `_file_lock`，重新 reload 取最新条目。
4. exact duplicate 直接成功返回。
5. 计算 `new_total = len(ENTRY_DELIMITER.join(entries + [content]))`。
6. 超限分支返回结构化错误：

```json
{
  "success": false,
  "error": "Memory at 2,100/2,200 chars. Adding this entry (250 chars) would exceed the limit. Replace or remove existing entries first.",
  "current_entries": ["..."],
  "usage": "2,100/2,200"
}
```

注意 `current_entries` 是完整列表，不是截断。模型据此挑选 consolidation 目标。Mnemon 可以采用同类策略：memory store 给出 hard cap；超过阈值时不自动塞入，而是要求 agent 输出 consolidation patch（携带当前条目作为上下文）。

## Skills 与渐进披露

Hermes skills 是 procedural memory：

```text
~/.hermes/skills/<skill>/
  SKILL.md
  references/
  templates/
  scripts/
  assets/
```

子目录是白名单的（`ALLOWED_SUBDIRS`），任何 `write_file`/`remove_file` 调用 `_validate_file_path` (`tools/skill_manager_tool.py:298-336`) 校验路径不能逃逸或写到根。

progressive disclosure 三层（`website/docs/user-guide/features/skills.md:44-52`、`prompt_builder.py:718-840+`）：

- Level 0：`skills_list()` 只给 name + description + category，约 3k tokens。
- Level 1：`skill_view(name)` 读取完整 `SKILL.md`。
- Level 2：`skill_view(name, path)` 读取 `references/<x>.md` 等。

只有 Level 0 进入系统 prompt，其余按需打开。这对 Mnemon 很重要：`GUIDELINE.md` 不应包含所有细节；INSTALL 只说明如何安装；具体 workflow 放 skill 并按需 `recall`。

## 定时 curator 的实际行为

`RELEASE_v0.12.0.md:12` 与 `agent/curator.py` 配对来看：

- 触发：inactivity-triggered，不是 cron daemon。`should_run_now` (`:198-253`) 检查 `last_run_at` 与 `interval_hours`。
- 默认配置（可被 `~/.hermes/config.yaml` 的 `curator.*` 覆盖，`:131-182`）：
  - `enabled=True`
  - `interval_hours=168`（7 天）
  - `min_idle_hours=2`
  - `stale_after_days=30`
  - `archive_after_days=90`
- 自动转移：`apply_automatic_transitions` (`:255-295`) 按 `last_activity` 时间戳把 active→stale→archived；任何 archive 都是把目录搬到 `~/.hermes/skills/.archive/`，可恢复（`:346-348`）。
- review prompt：`CURATOR_REVIEW_PROMPT` (`:329-444`) 强制 umbrella-first；硬规则包括"never delete"、"never touch pinned/bundled/hub"、"don't use use_count as reason to skip"；output 必须含结构化 YAML：

```yaml
consolidations:
  - from: <old-skill-name>
    into: <umbrella-skill-name>
    reason: <one short sentence>
prunings:
  - name: <skill-name>
    reason: <one short sentence>
```

- dry-run：`CURATOR_DRY_RUN_BANNER` (`:302-326`) 强制只读，对应 `hermes curator run --dry-run`，输出仍是同结构的 YAML 但描述"would do"。
- 报告落盘：`logs/curator/<YYYYMMDD-HHMMSS>/run.json` 与 `REPORT.md`（`RELEASE_v0.12.0.md:12-13`，`agent/curator.py:879-912`）。
- 客户端隔离：注释 `agent/curator.py:18-19` 写明"Uses the auxiliary client; never touches the main session's prompt cache"——curator 走 `auxiliary.curator` 配置选定的辅助模型，不污染主对话。

这个机制适合长期运行的 Hermes，但 Mnemon 第一阶段不需要默认开启。更合理的是在 INSTALL 中把它定义为可选维护任务：例如让用户每周手动跑一次 `mnemon review`，输出可审查 diff 与 YAML 总结。

## 失败模式与边界

| 场景 | 触发位置 | 行为 |
|---|---|---|
| memory add 超限 | `tools/memory_tool.py:250-261` | 结构化 reject + `current_entries` + `usage`；agent 自行 consolidate |
| memory replace 多匹配且文本不同 | `tools/memory_tool.py:292-301` | reject + 80 字符 preview 列表 |
| memory invisible unicode | `tools/memory_tool.py:94-97` | 拒绝 + codepoint 报告 |
| memory threat regex 命中 | `tools/memory_tool.py:99-103` | 拒绝 + pattern id（如 `prompt_injection`） |
| skill name 不合法 | `tools/skill_manager_tool.py:178-187` | reject + 规则提示 |
| SKILL.md > 100,000 chars | `tools/skill_manager_tool.py:256-269` | reject + 实际 size 与上限 |
| skill 支持文件 > 1 MiB | `tools/skill_manager_tool.py:622-635` | reject + 1 MiB 提示 |
| pinned skill delete | `tools/skill_manager_tool.py:137-161` | reject + 提示 `hermes curator unpin <name>` |
| curator dry-run 误调 mutating tool | `agent/curator.py:323-325` | banner 要求模型自报 + reviewer 决定回滚 |
| 演化候选超过 size limit | `evolution/core/constraints.py:95-117` | `ConstraintResult(passed=False, constraint_name="size_limit", ...)` |
| 演化候选增长 >20% | `evolution/core/constraints.py:119-134` | `ConstraintResult(passed=False, constraint_name="growth_limit", ...)` |
| 演化候选缺 frontmatter | `evolution/core/constraints.py:150-174` | `skill_structure` 失败，列出缺失字段 |
| 演化候选 pytest 失败 | `evolution/core/constraints.py:55-93` | `test_suite` 失败，附最后 5 行 stdout |

每条都返回结构化字段，便于 reviewer / curator 自行决策。Mnemon 的 hook 与 review 命令都应保持这种"reject-with-evidence"风格。

## 对 Mnemon 的启发

Hermes 给 Mnemon 的直接模板：

```text
bounded fact memory (tools/memory_tool.py:118)
  + skill procedures (tools/skill_manager_tool.py:373-800)
  + session search for old transcripts (tools/session_search_tool.py)
  + reviewed markdown edits (agent/curator.py + self-evolution PLAN.md)
  + optional scheduled curator (DEFAULT_INTERVAL_HOURS=168)
```

具体建议：

- `GUIDELINE.md` 写"什么该记、什么不该记、如何提议修改"，引用 Hermes `MEMORY_GUIDANCE` 的 declarative vs imperative 区分。
- `INSTALL.md` 写"四个 hook 阶段怎么安装、每个 hook 做什么"，并把 Mnemon 的 review/dream 任务定义为 inactivity-triggered 而非定时 cron，参照 `agent/curator.py:5-7` 的设计动机。
- hook 产出"候选"，不直接无限追加 memory；让 LLM 走 `memory tool` 风格的 reject-with-evidence 路径。
- 超过容量阈值进入整理模式，error payload 携带当前条目，避免后台静默改写。
- workflow 一律沉淀成 skill，遵循 `name`/`description`/`version`/`platforms`/`metadata` frontmatter 与 `references/templates/scripts/assets` 子目录约束。
- 自进化第一阶段只输出 Markdown diff 加结构化 YAML 总结，参照 `CURATOR_REVIEW_PROMPT` 的 `consolidations` / `prunings` 块，方便 review/rollback。
- 数字阈值全部进 config（参照 Hermes `mem_config` 与 `EvolutionConfig`），不写死在代码里。

## 参考来源

- 公开站点: [Hermes Agent](https://hermes-ai.net/)
- 本地源码: `/tmp/mnemon-agent-research-sources/hermes-agent/website/docs/user-guide/features/memory.md`
- 本地源码: `/tmp/mnemon-agent-research-sources/hermes-agent/website/docs/user-guide/features/skills.md`
- 本地源码: `/tmp/mnemon-agent-research-sources/hermes-agent/website/docs/user-guide/features/curator.md`
- 本地源码: `/tmp/mnemon-agent-research-sources/hermes-agent/agent/prompt_builder.py:150-183`
- 本地源码: `/tmp/mnemon-agent-research-sources/hermes-agent/agent/memory_manager.py:1-265`
- 本地源码: `/tmp/mnemon-agent-research-sources/hermes-agent/agent/curator.py:56-444`
- 本地源码: `/tmp/mnemon-agent-research-sources/hermes-agent/tools/memory_tool.py:55-564`
- 本地源码: `/tmp/mnemon-agent-research-sources/hermes-agent/tools/skill_manager_tool.py:107-909`
- 本地源码: `/tmp/mnemon-agent-research-sources/hermes-agent/tools/session_search_tool.py:1-600`
- 本地源码: `/tmp/mnemon-agent-research-sources/hermes-agent/run_agent.py:1733-1850, 4963-5071, 10780-10810`
- 本地源码: `/tmp/mnemon-agent-research-sources/hermes-agent/RELEASE_v0.12.0.md`
- 本地源码: `/tmp/mnemon-agent-research-sources/hermes-agent-self-evolution/README.md`
- 本地源码: `/tmp/mnemon-agent-research-sources/hermes-agent-self-evolution/PLAN.md:460-694`
- 本地源码: `/tmp/mnemon-agent-research-sources/hermes-agent-self-evolution/evolution/core/config.py`
- 本地源码: `/tmp/mnemon-agent-research-sources/hermes-agent-self-evolution/evolution/core/constraints.py`
