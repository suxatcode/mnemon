# Hermes 的记忆、Markdown 与 Prompt 用法

## 记忆处理方案

Hermes 内置 memory 由两个 bounded Markdown 文件组成：

| 文件 | 用途 | 默认上限 | 定义位置 |
|---|---|---|---|
| `~/.hermes/memories/MEMORY.md` | agent 对环境、项目、事实、决策的 durable memory | 2200 chars (~800 tokens) | `tools/memory_tool.py:118` |
| `~/.hermes/memories/USER.md` | 用户偏好、用户画像、交互风格 | 1375 chars (~500 tokens) | `tools/memory_tool.py:118` |

两者在 session start 注入为 frozen system prompt block。这样做保护 prefix cache：session 中 memory 文件变化会持久化，但当前 session 不会动态改变已缓存 system prefix（`tools/memory_tool.py:11-14` 与 `:361-372` 的 `format_for_system_prompt` 注释）。

### 真实注入格式

`tools/memory_tool.py:393-409` 的 `_render_block` 决定了模型实际看到的样子：

```
══════════════════════════════════════════════
MEMORY (your personal notes) [67% — 1,474/2,200 chars]
══════════════════════════════════════════════
User's project is a Rust web service at ~/code/myapi using Axum + SQLx
§
This machine runs Ubuntu 22.04, has Docker and Podman installed
§
User prefers concise responses, dislikes verbose explanations
```

字段含义：

- 分隔符 `§` 来自 `ENTRY_DELIMITER = "\n§\n"`（`tools/memory_tool.py:59`），允许条目本身包含换行。
- header 显示百分比与 `current/limit`，让模型自己判断是否到了 consolidation 阈值。
- USER.md header 改写为 `USER PROFILE (who the user is) [...]`，仍同一类格式。

### 工具入口与 schema

`tools/memory_tool.py:515-564` 中 `MEMORY_SCHEMA` 是 Hermes 暴露给模型的唯一 memory 工具：

- `action` enum：`add` / `replace` / `remove`（没有 `read`，因为读取来自 system prompt 注入）。
- `target` enum：`memory` / `user`。
- `replace` / `remove` 用 `old_text` 做"短唯一子串"匹配（`MemoryStore.replace` / `:269-325`）。如果匹配多条且文本不同，工具返回 80 字符 preview 列表让 agent 重选。

写路径执行细节（`tools/memory_tool.py:224-267`）：

1. `content.strip()`，空内容直接 reject。
2. `_scan_memory_content`：检查 `_MEMORY_THREAT_PATTERNS`（13 条 prompt injection / role hijack / credential exfil 正则）和 `_INVISIBLE_CHARS` 集合（zero-width 与方向控制字符）。
3. 进 `_file_lock` 文件锁，再 `_reload_target` 重新读盘，避免并发 session 互踩。
4. duplicate 检查：完全相同条目直接返回"no duplicate added"，不报错。
5. 容量预测：`new_total = len(ENTRY_DELIMITER.join(new_entries))`，超限时返回结构化错误并附 `current_entries` + `usage`，让模型有足够上下文做 replace/remove。
6. 通过则 `_write_file` 用 `tempfile.mkstemp` + `atomic_replace` 写入。

### 外部 memory provider

`agent/memory_manager.py:204-251` 的 `add_provider` 强制"only ONE external plugin provider at a time"，避免 schema 膨胀和 backend 冲突。`agent/memory_manager.py:1-60` 还提供 `<memory-context>` fence 与"System note: …"系统注解的扫除逻辑，防止 provider 注入物伪装成用户消息。Honcho、Mem0、Hindsight 等都按 plugin 接口实现，挂在同一管理器之下。

### 容量回收的标准动作

`website/docs/user-guide/features/memory.md:124-143` 给出文档建议：超过 80% 时主动 consolidation。具体步骤是 agent 自己读 error 中的 `current_entries`，用 `replace` 把多条相关事实合并成更短的一条，再尝试 `add`。这是 agent-level 的 GC，不是后台 daemon。

## Skills 是 procedural memory

Hermes 文档明确区分（`website/docs/user-guide/features/memory.md` 与 `website/docs/user-guide/features/skills.md`）：

- memory 是 declarative facts；
- skills 是 procedures。

典型 skill 目录：

```text
~/.hermes/skills/<skill>/
  SKILL.md
  references/
  templates/
  scripts/
  assets/
```

`tools/skill_manager_tool.py:170-171` 的 `ALLOWED_SUBDIRS = {"references", "templates", "scripts", "assets"}` 决定了 `write_file` / `remove_file` 只允许写到这四个子目录。

### SKILL.md 真实 schema

`website/docs/user-guide/features/skills.md:58-91` 给出的 frontmatter：

```markdown
---
name: my-skill
description: Brief description of what this skill does
version: 1.0.0
platforms: [macos, linux]
metadata:
  hermes:
    tags: [python, automation]
    category: devops
    fallback_for_toolsets: [web]
    requires_toolsets: [terminal]
    config:
      - key: my.setting
        description: "What this controls"
        default: "value"
        prompt: "Prompt for setup"
---

# Skill Title

## When to Use
触发条件。

## Procedure
1. 第 1 步（含具体命令）
2. 第 2 步

## Pitfalls
- 已知失败模式 + 解决办法

## Verification
如何确认 skill 运行成功。
```

`tools/skill_manager_tool.py:217-248` 的 `_validate_frontmatter` 强制 `description` 字段存在且不超过 `MAX_DESCRIPTION_LENGTH=1024`。`name` 受 `MAX_NAME_LENGTH=64` 与 `VALID_NAME_RE = ^[a-z0-9][a-z0-9._-]*$` 限制，文件大小受 `MAX_SKILL_CONTENT_CHARS=100_000` 与 `MAX_SKILL_FILE_BYTES=1_048_576`（1 MiB）限制。

### `skill_manage` 真实 actions

`tools/skill_manager_tool.py:797-909` 的 `SKILL_MANAGE_SCHEMA` 列出 6 个 action：`create`、`patch`、`edit`、`delete`、`write_file`、`remove_file`。其中：

- `patch` 用 `old_string` / `new_string` / `replace_all` 做行内替换（"preferred for fixes"，schema 描述原话）。
- `edit` 是整体重写，要求先 `skill_view` 读出当前 SKILL.md。
- `delete` 必须传 `absorbed_into=<umbrella>`（合并到伞型 skill）或 `absorbed_into=""`（纯剪枝）；这是 v0.12.0 curator 区分"consolidation vs pruning"的关键。

pinned 状态由 `tools/skill_manager_tool.py:137-161` 的 `_pinned_guard` 保护：pinned skill 仍可被 patch/edit，只是 delete 被拒绝。

### Progressive disclosure 三层

`website/docs/user-guide/features/skills.md:44-52` 的层级与 `agent/prompt_builder.py:718-840+` 的实现：

- Level 0：`skills_list()` 返回 name+description+category 列表，约 3k tokens。
- Level 1：`skill_view(name)` 读完整 `SKILL.md`。
- Level 2：`skill_view(name, path)` 读 `references/<x>.md` 等具体文件。

只有 Level 0 进入系统 prompt，其余按需打开。

## 特殊 prompt

`agent/prompt_builder.py` 的字面量片段（直接截取）：

`MEMORY_GUIDANCE`（`:150-168`）核心三句：

> "Write memories as declarative facts, not instructions to yourself."
> "'User prefers concise responses' ✓ — 'Always respond concisely' ✗."
> "Procedures and workflows belong in skills, not memory."

`SESSION_SEARCH_GUIDANCE`（`:170-174`）只有一段：

> "When the user references something from a past conversation or you suspect relevant cross-session context exists, use session_search to recall it before asking them to repeat themselves."

`SKILLS_GUIDANCE`（`:176-183`）：

> "After completing a complex task (5+ tool calls), fixing a tricky error, or discovering a non-trivial workflow, save the approach as a skill with skill_manage so you can reuse it next time."

`run_agent.py:5000` 把 `MEMORY_GUIDANCE` 通过 `tool_guidance.append(...)` 注入；`5057-5066` 注入 memory/user frozen block；`5071` 追加 external memory provider 块。这就是 system prompt 真正的拼装顺序。

## 自进化方案

Hermes 自进化分两层：

1. **运行时 curator（v0.12.0）**：`agent/curator.py` 实现，inactivity-triggered（注释 `:5-7`），在主循环空闲且距离上次运行 ≥ `DEFAULT_INTERVAL_HOURS=24*7` 时 fork 一个辅助 agent 做 review。`apply_automatic_transitions`（`:255-295`）按 `DEFAULT_STALE_AFTER_DAYS=30` 与 `DEFAULT_ARCHIVE_AFTER_DAYS=90` 把 skill 从 active → stale → archive 推进。`CURATOR_REVIEW_PROMPT`（`:329-444`）告诉它必须按 prefix cluster 做"umbrella-ification"，并写出结构化 YAML 总结 `consolidations` / `prunings`。
2. **离线 DSPy + GEPA pipeline**（`hermes-agent-self-evolution`）：`evolution/core/config.py:26-35` 定义 `max_skill_size=15_000`、`max_tool_desc_size=500`、`max_param_desc_size=200`、`max_prompt_growth=0.2`。`evolution/core/constraints.py` 的 `validate_all` 把 size、growth、structure 全部当成硬 gate；`run_test_suite` 跑全量 pytest，timeout 300s。

`PLAN.md:460-510` 列出可演化与不可演化的 prompt section：

可演化：

- `DEFAULT_AGENT_IDENTITY`
- `MEMORY_GUIDANCE`
- `SESSION_SEARCH_GUIDANCE`
- `SKILLS_GUIDANCE`
- `PLATFORM_HINTS`

不可演化：

- 用户真实 memory block（user data）；
- 自动生成的 skills index；
- 项目上下文文件（AGENTS.md、`.cursorrules`）。

`PLAN.md:687-694` 的 caching 规则：所有演化产物只在 NEW session 生效，从不 hot-swap 到正在跑的对话——和运行时 frozen snapshot 是同一原则的延伸。

## 失败模式与边界

| 场景 | 触发位置 | 处理 |
|---|---|---|
| add 超限 | `tools/memory_tool.py:250-261` | 返回结构化错误 + `current_entries` + `usage`，agent 自己 consolidate |
| replace 多匹配 | `:292-301` | 返回 80 字符 preview 列表，要求更具体 |
| exact duplicate | `:243-244` | 静默成功，message="Entry already exists (no duplicate added)" |
| invisible unicode | `:94-97` | 拒绝并报告 codepoint |
| prompt injection / exfil | `:99-103` | 拒绝并报告 pattern id（如 `prompt_injection`、`exfil_curl`） |
| skill 名称非法 | `tools/skill_manager_tool.py:178-187` | 拒绝并提示规则（lowercase、`[a-z0-9._-]*`、≤64） |
| skill content 超限 | `:256-269` | 拒绝并报实际 size 与 100_000 上限 |
| skill 文件 >1 MiB | `:622-635` | 拒绝并报 1 MiB 限 |
| skill name 已存在 | `:393-399` | create 直接 fail；要求用 patch/edit |
| pinned skill 被 delete | `:137-161` | 拒绝并提示 `hermes curator unpin <name>` |
| curator 跑 mutation 在 dry-run 模式 | `agent/curator.py:302-326` | banner 强制只读，模型若误调 mutating tool 必须自报 |

这些边界都是同步、可审计、错误信息结构化的，没有"静默丢内容"或"后台改写"的设计。

## 对 Mnemon 的设计判断

- **memory 边界要复刻**：bounded char count + 阈值 + 错误式 reject + agent 自己 consolidate。这是最便宜的不膨胀方案。
- **frontmatter 直接照抄**：`name`、`description`、`version`、`platforms`、`metadata.<vendor>` 五件套已被 Hermes/Anthropic skills 共同采用，Mnemon 也应走这一格式而不是发明新 schema。
- **provider 单插件**：如果引入向量/图谱后端，按 `MemoryManager` 的"one provider at a time"约束就够了，不必做更复杂的多 backend 路由。
- **演化分两层**：运行时 curator 处理常见维护（merge / archive），离线 pipeline 处理跨工件的演化。Mnemon 第一阶段只需做"运行时 review + 离线 patch 输出"两条路径。
- **size limit 写在 config，不写在 hardcoded 常量**：Hermes 把 2200/1375 暴露在 `mem_config`，把 15_000/500/200 暴露在 `EvolutionConfig`，对 Mnemon 也成立。

Mnemon 当前应采用 Hermes 风格而不是 OpenClaw 风格：

```text
memory facts (bounded char)
  + skills as procedures (progressive disclosure + 子目录约束)
  + guideline as behavior policy (declarative facts vs imperative rules)
  + hook reminders (定时/事件 nudge)
  + reviewed markdown evolution (offline diff，不动 in-flight prompt)
```

## 参考来源

- 本地源码: `hermes-agent/website/docs/user-guide/features/memory.md`
- 本地源码: `hermes-agent/website/docs/user-guide/features/skills.md`
- 本地源码: `hermes-agent/website/docs/user-guide/features/curator.md`
- 本地源码: `hermes-agent/agent/prompt_builder.py:150-183, 718-840`
- 本地源码: `hermes-agent/agent/memory_manager.py:1-265`
- 本地源码: `hermes-agent/agent/curator.py:56-444`
- 本地源码: `hermes-agent/tools/memory_tool.py:55-564`
- 本地源码: `hermes-agent/tools/skill_manager_tool.py:107-909`
- 本地源码: `hermes-agent/run_agent.py:1733-1753, 4963-5071`
- 本地源码: `hermes-agent/RELEASE_v0.12.0.md`
- 本地源码: `hermes-agent-self-evolution/PLAN.md:460-694`
- 本地源码: `hermes-agent-self-evolution/evolution/core/config.py`
- 本地源码: `hermes-agent-self-evolution/evolution/core/constraints.py`
- 公开站点: [Hermes Agent](https://hermes-ai.net/)
