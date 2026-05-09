# Hermes 自进化 Harness：源码闭环、社区共识与可安装 framework

本文把原 `docs/research/hermes-self-evolution/` 下的分篇研究收敛为一份单文档。研究目标不是把 Hermes 复制成另一个 memory adapter，也不是设计一个新的 agent framework，而是从 Hermes Agent 源码中抽出一套 **agent 无关的 self-evolution harness framework**：它通过 `INSTALL.md`、`GUIDELINE.md`、skills、hooks、state、reports 和可选 cold-memory provider 安装到任意 host agent 上，让该 agent 获得自进化能力。

## 摘要

Hermes 的自进化不是一个单独 memory 模块，而是一套 behavioral artifact control loop。抽象成 harness 后，host agent 仍负责模型调用、工具执行、UI 和权限；harness 只提供可安装的行为层和维护层：

```text
turn_delivered
  -> Reflection Harness Job(memory+skills only)
  -> memory / skill patch
  -> provenance + usage sidecar
  -> curator consolidation / archive / report / rollback
  -> offline evaluator proposes high-risk prompt/tool/code changes
```

最值得抽取的是这条链路，而不是某个具体工具函数或 Hermes 的 agent runtime。它把日常任务中的经验变成可治理的行为资产，再通过空闲维护和离线评测防止资产膨胀、过时或失控。

核心判断：

1. **Memory 是事实层，skill 是行为层，system prompt 是热路径预算。**
2. **自进化主对象应是可读、可 diff、可 patch、可 archive 的 Markdown artifact。**
3. **Markdown 是热存，不是容量层。** 长期容量需要 filesystem、index、传统 memory model 和 hot/cold exchange。
4. **Hook 是触发底座。** 没有 recall/observe/reflect/curate 事件，自进化只能靠模型偶尔想起。
5. **Provenance 是安全边界。** 自动治理只能处理明确 self-authored / agent-created 的资产。
6. **Curator 必须 dry-run/report/backup/archive-first。** 高风险演化必须走 eval 和 PR gate。
7. **这是 harness framework，不是 agent framework。** 安装目标是 Claude Code、Codex、Cursor、Continue、Hermes、OpenClaw 或任意 generic agent；harness 不拥有 agent loop，只绑定 host lifecycle。
8. **Harness 需要自己的 canonical filesystem。** 默认放在 repo-local `.mnemon/`；host 原生文件应是 projection/binding，而不是唯一 source of truth。

## 0. Harness Framework, Not Agent Framework

这里的 harness framework 指一个可安装的外骨骼，而不是一个新的 agent runtime。

| 维度 | Agent framework | Harness framework |
|---|---|---|
| 拥有什么 | LLM loop、planner、tool router、UI、权限模型 | skills、hooks、guidelines、state、reports、memory layout |
| 如何运行 | 用户直接使用这个 agent | 安装到已有 host agent 上，由 host agent 运行 |
| 与模型关系 | 选择/封装模型 | 不关心模型，只通过 host lifecycle 触发 |
| 与工具关系 | 定义工具协议和执行器 | 只声明需要的 hook/skill 能力，复用 host 工具 |
| 与平台关系 | 需要专门 adapter | 用 `INSTALL.md` 做 declarative host binding，尽量不写厚 adapter |
| 迁移方式 | 移植 runtime | 复制 skill/hook pack + 安装契约 |

Harness 的交付物应是：

```text
.mnemon/
  INSTALL.md          # host agent 如何安装本 harness
  GUIDELINE.md        # 安装后的记忆与自进化行为准则
  fs.yaml             # canonical filesystem 与 projection policy
  bindings/           # active host bindings 与 projection metadata
  skills/             # recall / observe / reflect / curate / research
  hooks/              # 四阶段语义 hook 的脚本或 prompt 模板
  memory/             # hot / cold 与 exchange artifact 的文件布局
  state/              # usage/provenance/pins/curator state
  reports/            # review/curator/eval 输出
  schemas/            # hook IO、proposal、report schema
```

安装后，host agent 不需要变成 Hermes，也不需要接入 Hermes runtime。它只需要能做到几件事：

1. 读取 `GUIDELINE.md` 或把它纳入自己的 project instruction。
2. 发现并调用 `skills/`。
3. 在可用 lifecycle 上安装或模拟 recall / observe / reflect / curate hooks。
4. 允许 harness 写 `memory/`、`state/`、`reports/`。
5. 对高风险修改保留 human approval。

不同 host 的能力不同，因此 harness 应有降级等级：

| 等级 | Host 能力 | 自进化能力 |
|---|---|---|
| L0: skill-only | 只能读 Markdown/skills | agent 可按 guideline 手动 reflect/curate，不能自动触发 |
| L1: instruction + skill | 支持 project instruction 和 skills | 可稳定遵循 memory/skill 边界，能主动提出 proposal |
| L2: lifecycle hooks | 支持 pre/post prompt/tool/session hooks | 可自动 recall/observe/reflect |
| L3: scheduled/idle | 支持 scheduled task、cron、idle hook | 可自动 curator/dreaming |
| L4: eval/CI | 支持 tests、benchmarks、PR flow | 可做离线 self-evolution |

因此，harness 的核心不是“写一个万能 adapter”，而是定义一份 host agent 能读懂的安装契约和一套可降级的语义能力。

No mandatory agent runtime guarantee：

```text
Harness core 不要求常驻进程。
Harness 不持有 agent state。
Harness 不拦截 LLM 调用。
Harness 不实现 hook bus、prompt assembler、scheduler、tool router、reflection executor。
Harness 只贡献 `.mnemon` 文件布局、Markdown 资产、JSON schema、prompt 模板和可由 host 调用的脚本。
所有执行都发生在 host agent 或 host 平台中。
Harness 可以提供可选 maintenance runner，但它只能执行 curator/dreaming/index/eval/post-turn review 等维护 job，不能接管 host agent loop。
Host 原生模板通过 managed block、pointer、symlink/copy projection 或 import report 挂载 `.mnemon`。
```

## 调研范围

本地源码快照：

| 仓库 | commit | 作用 |
|---|---:|---|
| `NousResearch/hermes-agent` | `5643c297901312d817713a8cc870a28a439e3114` | Hermes 主体：memory、skills、curator、hooks、cron |
| `NousResearch/hermes-agent-self-evolution` | `4693c8f0eed21e39f065c6f38d98d2a403a04095` | 离线 GEPA/DSPy self-evolution 管线 |

重点源码：

```text
run_agent.py
agent/prompt_builder.py
agent/curator.py
agent/curator_backup.py
agent/memory_manager.py
agent/memory_provider.py
tools/memory_tool.py
tools/skills_tool.py
tools/skill_manager_tool.py
tools/skill_usage.py
tools/skill_provenance.py
cron/scheduler.py
cron/jobs.py
cli.py
hermes_cli/curator.py
hermes_cli/hooks.py
agent/shell_hooks.py
evolution/core/config.py
evolution/core/constraints.py
```

社区/生态参考包括 Hermes 官方文档、Claude Code memory/skills/hooks、OpenAI Codex AGENTS.md、Cursor rules、Continue rules、OpenClaw skills/dreaming、MemGPT/Letta 记忆分层。公开文档与源码有少量漂移；涉及 Hermes 行为时，本文以本地源码为准。

Claude Code 也参与了多轮只读审阅。它的主要建议已合入本文：把 Hermes 的 after-turn reflection 主链路前置；把方案从 runtime object 改成 artifacts、schemas、prompt templates、hook scripts 和 install maps；把 INSTALL/GUIDELINE、hot/cold exchange、dry-run 权限、no mandatory agent runtime 边界和源码数字锚点补齐。

## 1. 自进化是系统工程

Hermes 的架构至少有三档自进化能力：

| 层次 | 机制 | 作用 |
|---|---|---|
| 运行时沉淀 | `memory` tool、`skill_manage`、background review | 把稳定事实或可复用流程保存为 memory/skill |
| 长期治理 | usage sidecar、curator、archive、report、backup | 防止 agent-created skills 无限堆积、重复或过期 |
| 离线演化 | Hermes Self-Evolution 的 DSPy/GEPA/eval/constraint/PR | 优化 skills、tool descriptions、prompt sections、code |

三档的风险不同：

- 事实记忆污染未来上下文。
- skill 错误会让错误流程被复用。
- prompt/tool/code 演化会改变全局行为。

因此 Hermes 没有把所有东西交给一个后台 agent 自动改写。低风险的 after-turn review 只给 memory/skills 工具；curator 聚焦 skill library；高风险演化走离线评估和 PR。

自进化 harness 必须暴露这些表面：

| 表面 | 目的 | 缺失时的失败模式 |
|---|---|---|
| 可演化 artifacts | 明确什么能被改：memory、skill、guideline、hook prompt、reports | 模型把所有上下文都当成可重写对象 |
| 不可演化边界 | 当前用户指令、secrets、raw evidence、runtime schema | 旧记忆覆盖当前事实或后台误改配置 |
| 触发点 | session start、pre LLM、post tool、turn end、pre compact、idle | 只能靠模型主观想起要保存 |
| 记忆分层 | hot 给模型，warm 整理，cold 容量 | 单个 Markdown 越写越长 |
| provenance | 区分 user、agent、package、imported、curator | 无法判断是否可自动覆盖 |
| 使用统计 | view/use/patch/state/pinned/archive | 无法知道什么该保留、合并、归档 |
| 审查与回滚 | dry-run、report、backup、archive | 后台改写不可解释 |
| 评估 gate | size、tests、benchmark、LLM judge、human review | 演化凭模型感觉，容易回归 |

## 2. Hermes 源码闭环

### System Prompt 是热路径预算

`run_agent.py::_build_system_prompt()` 组装系统提示：identity、用户/平台提示、`MEMORY.md`/`USER.md` 快照、`MEMORY_GUIDANCE`、`SESSION_SEARCH_GUIDANCE`、`SKILLS_GUIDANCE`、skills system prompt、context files、日期时间、外部 memory provider 静态 block。

关键点是：Hermes 在会话开始或压缩边界构建 system prompt，并尽量复用缓存。内置 memory 中途写盘不会立刻刷新当前 system prompt。这个设计把热记忆定义为“小而稳定的启动上下文”，而不是实时日志。

`agent/prompt_builder.py` 的边界也很清楚：

| 内容 | Hermes 方向 |
|---|---|
| 用户偏好、环境细节、工具/API 坑点、稳定项目约定 | 写 memory |
| 一次性任务进度、完成记录、临时 TODO | 不写 memory |
| 工作流、操作流程、可复用方法 | 写 skill |
| 指令式长期规则 | 避免写成 memory，防止覆盖当前用户请求 |

### 内置 Memory 是 Bounded Markdown

`tools/memory_tool.py` 实现两个文件：

```text
~/.hermes/memories/MEMORY.md
~/.hermes/memories/USER.md
```

源码行为：

| 机制 | 实现 |
|---|---|
| 默认容量 | `MEMORY.md` 2200 chars，`USER.md` 1375 chars |
| entry delimiter | `\n§\n` |
| 支持动作 | `add`、`replace`、`remove` |
| 去重 | load 和 add 时按 exact match 去重 |
| 并发 | lock file + tempfile + fsync + atomic replace |
| 安全 | 写入前扫描 prompt injection、secret exfil、隐形字符 |
| prompt 策略 | 会话中写盘，但 system prompt 使用 frozen snapshot |
| 超限策略 | 拒绝写入，返回 current entries/usage，要求先整理 |

这解释了为什么 Hermes 没有先做厚工程化记忆：模型直接消费的热记忆被压得很小，容量问题被推到 external provider、session search、curator 和离线整理。

### Skill 是主要行为资产

Hermes 把流程性经验放进 skill，而不是塞进 memory。核心工具：

| 文件 | 作用 |
|---|---|
| `tools/skills_tool.py` | `skills_list`、`skill_view`，负责发现和渐进披露 |
| `tools/skill_manager_tool.py` | `skill_manage`，负责 create/edit/patch/delete/write_file/remove_file |

Skill 读路径是 progressive disclosure：

1. `skills_list` 只返回 name、description、category、count。
2. `skill_view` 才加载完整 `SKILL.md`。
3. `skill_view(file_path=...)` 才读取 `references/`、`templates/`、`scripts/`、`assets/`。
4. 成功 view 会 bump usage，让 curator 知道活跃度。

Skill 写路径的硬约束：

| 约束 | 值或行为 |
|---|---|
| name | filesystem-safe，最长 64 |
| description | 最长 1024 |
| `SKILL.md` | 必须 YAML frontmatter，含 `name` 和 `description` |
| skill body | 最大 100,000 chars |
| 支持文件 | 最大 1 MiB |
| 支持目录 | `references/`、`templates/`、`scripts/`、`assets/` |
| patch | old/new string，支持 fuzzy replacement，默认唯一匹配 |
| pinned | 阻止 delete，不阻止 patch/edit |

Hermes 的 review prompt 强调 class-first / umbrella skill，而不是 one-session-one-skill。更好的模式是把多个窄问题合并成类级别 skill：

```text
bad:
  fix-nextjs-port-3000
  fix-nextjs-port-3001
  recover-vite-dev-server

good:
  dev-server-troubleshooting
    - port occupied
    - stale process
    - env mismatch
    - framework-specific commands
    - verification checklist
```

### Provenance 决定能治理什么

`tools/skill_provenance.py` 用 `ContextVar` 标记写入来源。正常前台 agent 是 `foreground`；`run_agent.py::_spawn_background_review()` 会把 review fork 设为 `background_review`。`skill_manage(create)` 成功后，只有在 `is_background_review()` 为真时才调用 `skill_usage.mark_agent_created()`。

源码层面的安全规则：

| 来源 | 是否进入自动 curator 治理面 |
|---|---|
| background review fork 创建的 skill | 是 |
| 用户前台要求 agent 创建的 skill | 否 |
| bundled skill | 否 |
| hub-installed skill | 否 |
| 只被查看/使用过的手写本地 skill | 不因 usage 自动进入 candidate |

这与 Hermes 公开文档的部分描述不同。公开 curator 文档把“非 bundled/hub 的本地 skill”描述得更宽；本文以源码为准。通用 harness 应采用更保守规则：自动治理只动明确 self-authored / agent-created 的资产。

### Usage Sidecar 是工程治理面

`tools/skill_usage.py` 维护：

```text
~/.hermes/skills/.usage.json
~/.hermes/skills/.archive/
~/.hermes/skills/.bundled_manifest
~/.hermes/skills/.hub/lock.json
```

记录字段包括 `created_by`、`agent_created`、view/use/patch counts、last timestamps、`state`、`pinned`、`archived_at`。自动归档使用 `archive_skill()` 移到 `.archive/`；`restore_skill()` 可恢复。

关键抽象：Markdown 给模型读，sidecar 给工程层做状态机。治理元数据不污染 `SKILL.md`。

### Post-Turn Reflection 是自我修正核心

`run_agent.py` 维护两个 nudge counter：

| counter | 触发 |
|---|---|
| `_turns_since_memory` | user turn 计数，默认 memory nudge interval 为 10 |
| `_iters_since_skill` | tool-calling iteration 计数，默认 skill nudge interval 为 10 |

触发后不是在当前主回复里反思，而是在主回复完成后调用 `_spawn_background_review()`：

1. 选择 memory、skill 或 combined review prompt。
2. 启动 daemon thread `bg-review`。
3. fork 新 `AIAgent`，继承 parent runtime。
4. `max_iterations=16`，`quiet_mode=True`。
5. 只启用 `enabled_toolsets=["memory", "skills"]`。
6. 设置 `_memory_write_origin="background_review"`。
7. 共享 memory store，关闭自己的 memory/skill nudges，避免递归。
8. approval callback 自动 deny，防止后台卡交互。
9. 运行 review，并把 tool actions 总结为用户可见 self-improvement summary。

这条链路是 Hermes 自进化的心脏。抽成 harness 后，不要求 host agent 真的支持 fork；它只要求 host 能在主回复交付后运行一个受限 reflection 语义事件。Hermes 的实现是 forked `AIAgent`，Claude Code 可以是 `Stop`/`SessionEnd` hook，generic agent 可以是手动 `reflect` skill 或 scheduled prompt：

```text
主任务完成
  -> 用户先收到回复
  -> 受限副 agent 回看 conversation
  -> 只允许 memory/skill 写
  -> 写入打 provenance
  -> curator 后续长期治理
```

如果只抽 `skill_manage` 而不抽 after-turn reflection job，就只得到“手动写 skill 的 IDE”，不是自进化 harness。

在非 Hermes host 上，“受限”不能靠 harness 自己的 tool router，因为 harness 没有 runtime。它只能提供：

- `prompts/reflection.md`：只允许提出 memory/skill 更新的 scoped prompt template。
- `schemas/write-target-allowlist.json`：声明可写目标，例如 `memory/**`、`skills/**`、`reports/**`。
- `hooks/reflect.*`：host 可调用的 hook template。
- `reports/reflection/`：当 host 不能限制 toolset 时，reflection 降级为 proposal-only，只写 report，不直接 patch。

host 如果没有权限层或工具 allowlist，就只能安装 L0/L1 模式，不能自动 patch。

### Curator 是长期整理器

`agent/curator.py` 负责周期治理 agent-created skills。默认值：

| 配置 | 默认 |
|---|---:|
| `interval_hours` | 168 小时 |
| `min_idle_hours` | 2 小时 |
| `stale_after_days` | 30 天 |
| `archive_after_days` | 90 天 |

运行条件：enabled、not paused、首次只 seed 状态、不立即运行；超过 interval 且 idle 足够才运行。

一次 curator run 分两段：

1. `apply_automatic_transitions()`：不用 LLM，按 usage metadata 将 active -> stale 或 stale -> archive。
2. `_run_llm_review()`：fork auxiliary `AIAgent`，让模型合并、patch、archive agent-created skills，并输出结构化 YAML。

curator prompt 的重点不是找重复文件，而是 umbrella-building：

- skip pinned。
- skip bundled/hub。
- 不把 use_count 作为保留理由。
- 不因为触发场景不同就拒绝合并。
- 优先 class-level skill。
- 窄内容降级到 `references/`、`templates/`、`scripts/`。
- 每个被移走的 skill 必须在 report 中分类为 consolidation 或 pruning。

报告写入：

```text
~/.hermes/logs/curator/<YYYYMMDD-HHMMSS>/run.json
~/.hermes/logs/curator/<YYYYMMDD-HHMMSS>/REPORT.md
```

### Backup、Rollback 和 Cron Rewrite 是安全阀

`agent/curator_backup.py` 在真实 curator run 前创建 snapshot：

```text
~/.hermes/skills/.curator_backups/<utc-id>/
  skills.tar.gz
  manifest.json
  cron-jobs.json
```

snapshot 包含 skill tree、`.usage.json`、`.archive/`、`.curator_state`、`.bundled_manifest` 和 cron skill links。默认保留 5 个 snapshot。rollback 前还会为当前状态做 pre-rollback snapshot。

`cron/jobs.py::rewrite_skill_refs()` 在 skill consolidation/pruning 后修复 scheduled jobs：

- consolidated old skill 替换为 umbrella target。
- pruned skill 从 job skill list 删除。
- 去重并同步 legacy `skill` 字段。

这说明 Hermes 把自进化视为会破坏引用关系的变更，因此需要迁移和回滚。

### External Memory Provider 是冷层扩展点

Hermes 不只有 Markdown。`agent/memory_provider.py` 定义 provider lifecycle：

```text
initialize()
system_prompt_block()
prefetch(query)
queue_prefetch(query)
sync_turn(user, assistant)
get_tool_schemas()
handle_tool_call()
shutdown()
```

可选 hooks 包括 `on_turn_start`、`on_session_end`、`on_session_switch`、`on_pre_compress`、`on_memory_write`、`on_delegation`。`MemoryManager` 只允许一个 external provider，避免 tool schema 膨胀和多后端冲突。

prefetch 返回的动态 recall 会包进 `<memory-context>` 注入当前 request，而不是写回 system prompt。这是冷热分层的源码证据：热层是 bounded Markdown，冷层是 provider、sync、prefetch 和工具。

抽成 harness 时，这一层不应变成内置 `MemoryManager`。Harness 只定义 cold-memory protocol：tool schema、payload schema、lifecycle event 名称、recall 输出格式和 write policy。具体 provider manager、单 provider 限制、并发策略都归 host 或外部服务。

### Hooks 提供 Nudge/Remind 插桩点

Hermes 的 plugin/shell hooks 和 run loop 提供这些关键事件：

| hook | 自进化用途 |
|---|---|
| `on_session_start` | system prompt 构建后触发，加载启动状态 |
| `pre_llm_call` | 返回 context 注入当前 user message，不持久化 |
| `pre_tool_call` | 安全扫描、权限控制 |
| `post_tool_call` | 记录工具结果、错误、duration、evidence |
| `on_pre_compress` | 压缩前提取将丢失的连续性 |
| `on_memory_write` | 内置 memory 写入后镜像给外部 provider |
| `on_session_end` | 真实 session 结束时 flush |
| finalization path | 主回复结束后触发 background review 和 sync |

没有这些 hook，memory/skill 只能依赖模型“想起来保存”，那不是系统能力。

### Self-Evolution 仓是离线优化器

`hermes-agent-self-evolution` 与运行时 curator 不在同一时间尺度。它用于生成候选并通过 eval/constraint/PR gate 落地。

`evolution/core/config.py` 默认值：

| 参数 | 默认 |
|---|---:|
| iterations | 10 |
| population_size | 5 |
| optimizer_model | `openai/gpt-4.1` |
| eval_model | `openai/gpt-4.1-mini` |
| judge_model | `openai/gpt-4.1` |
| max_skill_size | 15,000 chars |
| max_tool_desc_size | 500 chars |
| max_param_desc_size | 200 chars |
| max_prompt_growth | 20% |
| eval_dataset_size | 20 |

风险分级：

| 目标 | 风险 | Gate |
|---|---|---|
| skill 文件 | 低到中 | frontmatter、size、eval、tests |
| tool description | 中 | length、parameter desc、semantic preservation |
| system prompt section | 中到高 | growth cap、behavior regression、benchmark |
| tool implementation code | 高 | full tests、benchmark、human review、PR |

高风险演化不应在用户会话中热替换。

## 3. 社区共识：为什么 Markdown-first

主流 agent 都把长期行为约束、项目知识或 skill 做成 Markdown 或类 Markdown：

| 系统 | 机制 | 共同点 |
|---|---|---|
| Claude Code | `CLAUDE.md`、auto memory、rules、skills、hooks | project/user/org instructions，auto memory，按需 skill |
| OpenAI Codex | `AGENTS.md` repository instructions | repo-local guidance，适合测试、约定、工作流说明 |
| Cursor | `.cursor/rules/*.mdc` | Markdown + frontmatter + globs/alwaysApply |
| Continue | `.continue/rules/*.md` | Markdown rules 注入 system message |
| OpenClaw | `SKILL.md`、`MEMORY.md`、`DREAMS.md` | skills + dreaming + compaction |
| Hermes | `MEMORY.md`、`USER.md`、`SKILL.md`、curator reports | bounded Markdown + usage sidecar + LLM curator |

Cursor、Continue 和 Codex 主要证明 Markdown/rules 是静态行为控制面的共识；Claude Code 和 OpenClaw 证明 hooks、skills、scheduled tasks 可以让它变成可运行维护面；Hermes 是少数把 after-turn review、curator、usage sidecar、backup 和 eval pipeline 串成完整自进化闭环的实现。

社区选择 Markdown 的原因：

1. LLM 原生可读，不需要额外 schema 解释。
2. LLM 可直接提出 patch。
3. 用户可 review、diff、commit、rollback。
4. 可以用 frontmatter 加最少结构。
5. 可以和 Git、filesystem、hooks、skills 直接组合。
6. 跨 agent 安装友好，不依赖厚 adapter。

Markdown 的限制也很明确：

| 限制 | 后果 |
|---|---|
| 上下文预算 | 文件太长挤压任务上下文，降低遵循度 |
| 线性结构 | 难表达复杂关系，同义/冲突/重复难发现 |
| 弱 schema | 格式漂移，模型写法不一致 |
| 并发弱 | 多后台任务写入会冲突 |
| 过时难识别 | 没有 sidecar 时不知道 last_used/provenance |
| 检索弱 | 一个大文件不好查，容易读太多或读不到 |

因此正确结论不是“只用 Markdown”，而是：

```text
Markdown = 热行为控制面
Filesystem / sidecar = 可审查治理面
Index / retrieval / memory model = 冷容量面
Evaluator / report = 演化安全面
```

## 4. Everything Is Skill

“Everything is skill” 不表示一切都写进 `SKILL.md`。更准确的边界是：

```text
事实、偏好、环境细节 -> memory
流程、工具经验、反复出现的任务模式 -> skill
一次性进度、临时 TODO、当前会话状态 -> session artifact
```

自进化要解决的问题不是“记住更多”，而是“未来做得更好”。这更像行为资产管理，而不是事实存储。

| 需求 | 放哪里 |
|---|---|
| 用户偏好 | memory |
| 项目固定事实 | memory 或 project guideline |
| 多步骤调试流程 | skill |
| 工具错误规避方法 | 简短事实可进 memory，完整方法进 skill |
| 模板、脚本、参考文件 | skill support files |
| 当前任务进度 | session summary |

Skill 的结构建议：

```yaml
---
name: memory-review
description: Review recent work and propose durable memory or skill updates.
scope: project
created_by: agent
risk: medium
---
```

```text
skills/
  memory-review/
    SKILL.md
    references/
      rubric.md
      examples.md
    templates/
      report.md
    scripts/
      check-memory-budget.sh
```

Skill 生命周期：

```text
candidate -> active -> stale -> archived
```

自动化规则必须保守：

- patch existing skill first。
- 只有真正新类别才 create skill。
- 长内容放 support files。
- agent-created 且长期 unused 才 stale/archive。
- archive，不 delete。
- pinned / user / package / imported 默认不自动改。
- 所有合并输出 report。

## 5. Hot / Cold 记忆与交换协议

单个 Markdown 文件短期有效，长期会遇到容量、质量和控制问题。建议 harness 使用两层主模型：

| 层 | 内容 | 是否直接进 prompt |
|---|---|---|
| Hot | `MEMORY.md`、`USER.md`、当前 guideline、当前任务相关 skill 摘要 | 是，严格短预算 |
| Cold | Mnemon/RAG/DB/FTS/vector、raw evidence、session transcript、历史 report、archive、index、usage events | 不直接进，只作为检索、recall 和 dreaming 输入 |

中间的 topic capsule、session summary、promotion candidate 应属于 `memory/exchange/`，是冷热切换协议状态，不是第三层主 memory。

Filesystem 是可审查真相层，数据库/向量/FTS 是召回加速层。重要事实最终应能落到可读 artifact 上，而不是只存在 embedding 里。

概念目录：

```text
self-evolution/
  GUIDELINE.md
  INSTALL.md
  memory/
    hot/
      MEMORY.md
      USER.md
      project.md
    cold/
      evidence/
      transcripts/
      summaries/
      topics/
      archive/
      index/
    exchange/
      candidates/
      promotions/
      demotions/
      decisions/
  skills/
  state/
    usage.json
    curator_state.json
    pins.json
  reports/
    review/
    curator/
    eval/
  backups/
```

Promotion：

```yaml
candidate:
  target: memory/hot/project.md
  reason: "被最近 3 次任务复用，且用户确认过"
  evidence:
    - memory/cold/transcripts/2026-05-01.md
    - reports/review/2026-05-04.md
  patch:
    - add concise fact
```

Demotion：

```text
hot/project.md 删除过细条目
cold/archive/hot/... 保留原条目
cold/evidence/... 保留原始来源
exchange/demotions/... 记录 demotion proposal
reports/curator/... 记录迁移原因
```

## 6. Hook、Nudge 与 Remind

Hook 是自进化触发底座：

```text
session start -> load guideline and hot memory
pre prompt -> recall and remind
pre tool -> guard and annotate
post tool -> observe and collect evidence
pre compact -> flush continuity
post response / stop -> reflect and propose
session end -> write summary
scheduled / idle -> curate and dream
```

区别：

| 类型 | 含义 | 示例 |
|---|---|---|
| remind | 把已有规则或记忆在合适时刻提醒模型 | 当前项目测试命令是 `pnpm test` |
| nudge | 推动模型执行维护动作 | 本轮出现可复用工具坑点，请提出 skill patch |

四阶段 hook：

| 阶段 | 触发 | 职责 | 边界 |
|---|---|---|---|
| Recall | session start、user prompt submit、pre LLM | 加载 guideline、hot memory、相关 warm/cold recall | 不永久写，不注入长历史 |
| Observe | pre tool、post tool、approval、file changed | 记录工具错误、成功命令、用户纠正、evidence | 默认不写 hot，只写 evidence |
| Reflect | post LLM、stop、session end、subagent stop | 生成 durable fact / skill patch proposal | proposal-first，一次性进度只进 session |
| Curate | idle、scheduled、manual、pre compact | 合并 skill、demote hot、promote cold、archive stale | dry-run-first、pinned 不动 |

平台映射：

| Mnemon 阶段 | Hermes | Claude Code | OpenClaw |
|---|---|---|---|
| recall | `on_session_start`, `pre_llm_call` | `SessionStart`, `UserPromptSubmit` | bootstrap, message preprocess |
| observe | `pre_tool_call`, `post_tool_call` | `PreToolUse`, `PostToolUse` | command/session/message hooks |
| reflect | `post_llm_call`, finalization, `on_session_end` | `Stop`, `SessionEnd` | command reset/new, session hooks |
| curate | curator idle check, cron ticker, manual CLI | scheduled tasks, manual command | cron/dreaming, compaction hooks |

Hook 输出应短、结构化、可返回 `NONE`：

```yaml
type: recall
status: ok
context:
  - source: memory/hot/project.md
    text: "Use pnpm for this repository."
```

```yaml
type: reflection
proposals:
  - target: skills/debugging/SKILL.md
    action: patch
    reason: "Repeated dev-server port collision workaround succeeded."
    risk: low
```

## 7. Curator、Dreaming 与长期生命周期

Hermes curator 是轻量治理：skill usage sidecar + deterministic transitions + LLM review + report + backup。OpenClaw dreaming 是更重的记忆 consolidation：Light / REM / Deep 阶段把短期信号整理、打分并 promotion 到长期 memory。

两者可以组合成三阶段路线：

| 阶段 | 目标 | 默认写入 |
|---|---|---|
| Reviewable curator | 治理 skills/hot memory，合并、demote、archive | report/proposal |
| Pre-compact flush | 上下文压缩前保存关键连续性 | warm session capsule |
| Dreaming promotion | 从 cold/warm 中筛高频、高置信、近期、跨任务候选 | promotion proposal |

OpenClaw dreaming 的关键点：

- Light：整理近期短期材料，不写长期 memory。
- REM：反思主题和信号，写 diary/report，不作为 promotion source。
- Deep：score + gate + promote durable candidates 到 `MEMORY.md`。
- Deep ranking 使用 frequency、relevance、query diversity、recency、consolidation、conceptual richness 等信号。

Hermes 的关键点：

- curator first-run defer。
- idle-triggered，不污染 active conversation。
- deterministic transitions 与 LLM review 分离。
- `REPORT.md` + `run.json`。
- archive recoverable。
- rollback captures skill tree and cron skill links。

## 8. Harness 安装契约：INSTALL.md 与 GUIDELINE.md

如果 harness 要跨 agent 安装，`INSTALL.md` 不能只是说明文，而应是 host agent 可执行的安装契约。它的目的不是解释理论，而是让 host agent 根据自己的能力完成绑定。

```text
# INSTALL.md

## Host detection
- 如何识别 Claude Code / Codex / Cursor / Continue / Hermes / OpenClaw / generic agent。
- 识别 host 支持哪些 capability level: skill-only / hooks / scheduled / eval。

## Files to install
- GUIDELINE.md 应放到哪里。
- skills/ 应如何注册或复制。
- memory/、state/、reports/ 的默认位置。
- schemas/ 和 hook templates 应如何放置。

## Hook mapping
- recall: session_start / pre_llm_call / user-prompt-submit。
- observe: pre_tool_call / post_tool_call。
- reflect: turn_delivered / stop / session_end。
- curate: idle_tick / scheduled task / manual command。

## Permissions
- 哪些 hook 只读。
- 哪些 hook 可写 reports。
- 哪些 hook 可 patch memory/skills。
- 哪些动作必须 human approval。

## Fallbacks
- host 没有 hook 时，如何用 skill-only 模式手动 recall/reflect/curate。
- host 没有 scheduled task 时，如何用 manual command 或外部 cron。
- host 没有 native skill system 时，如何用 Markdown instruction + file references 模拟。

## Verification
- dry-run 命令。
- report 路径。
- 禁用方式。
- rollback 方式。

## Upgrade and uninstall
- harness_version 字段。
- 升级不得清空用户 memory、archive、usage sidecar、pinned 标记。
- schema migration 必须写 report。
- uninstall 只移除 harness 安装文件和 hook binding，不删除用户 memory/archive/reports。
```

安装契约应有机器可读形态。可以是 `harness.yaml`，也可以是 `INSTALL.md` 中的 fenced YAML：

```yaml
harness:
  name: self-evolution-harness
  version: 0.1.0
  capabilities:
    required:
      - read_markdown
      - write_reports
    optional:
      - native_skills
      - lifecycle_hooks
      - scheduled_tasks
      - eval_ci
  writable_targets:
    - memory/**
    - skills/**
    - state/**
    - reports/**
  protected_targets:
    - GUIDELINE.md
    - INSTALL.md
  install_maps:
    claude-code:
      detect:
        commands: ["claude"]
        files_any: ["CLAUDE.md", ".claude/"]
      instruction_targets: ["CLAUDE.md", ".claude/CLAUDE.md"]
      skill_targets: [".claude/skills/"]
      hooks:
        recall: ["SessionStart", "UserPromptSubmit"]
        observe: ["PreToolUse", "PostToolUse"]
        reflect: ["Stop", "SessionEnd"]
        curate: ["scheduled", "manual"]
    codex:
      detect:
        files_any: ["AGENTS.md", ".codex/"]
      instruction_targets: ["AGENTS.md"]
      skill_targets: ["docs/agent-skills/", "skills/"]
      hooks:
        recall: ["manual"]
        observe: ["manual"]
        reflect: ["manual"]
        curate: ["manual"]
```

Host detection signals 应只用于安装期判断，不形成长期 adapter：

| Host | Detection signal | 主要安装面 |
|---|---|---|
| Hermes | `hermes` command、`~/.hermes/config.yaml`、`~/.hermes/skills` | native skills、plugin/shell hooks、curator |
| Claude Code | `claude` command、`CLAUDE.md`、`.claude/` | `CLAUDE.md`、skills、hooks |
| Codex | `AGENTS.md`、`.codex/` | repo instruction，manual skill pack |
| Cursor | `.cursor/rules/` | MDC rules，external scripts |
| Continue | `.continue/rules/` | rules/context providers |
| Generic | none | Markdown instruction + manual skills |

Capability levels map to concrete files:

| Level | Installed artifacts |
|---|---|
| L0 skill-only | `GUIDELINE.md`、`skills/recall/`、`skills/reflect/`、`skills/curate/` |
| L1 instruction + skill | L0 + host instruction snippet + merge/report script |
| L2 lifecycle hooks | L1 + `hooks/recall.*`、`hooks/observe.*`、`hooks/reflect.*`、hook IO schemas |
| L3 scheduled/idle | L2 + `hooks/curate.*`、scheduled job descriptor、backup/report templates |
| L4 eval/CI | L3 + eval dataset schema、constraints、PR template |

`GUIDELINE.md` 是行为契约：

```text
# GUIDELINE.md

## What to remember
durable facts, user preferences, stable project conventions, repeated tool quirks.

## What not to remember
task progress, transient TODOs, unverified guesses, secrets, one-off outcomes.

## Memory vs skill
facts/preferences -> hot memory; procedures/workflows -> skill; raw evidence -> cold memory.

## Update policy
patch existing skill first; create new class-level skill only when no umbrella exists.

## Safety
current user request wins; archive over delete; pinned assets are not auto-curated.
```

第一批 core skills 可以很少：

| Skill | 作用 |
|---|---|
| `install` | 根据 `INSTALL.md` 为当前 agent 安装 hook/guideline |
| `recall` | 根据当前任务召回 hot/warm/cold 相关内容 |
| `reflect` | 在任务结束时提出 memory/skill 更新 |
| `curate` | 合并、demote、archive 记忆和 skill |
| `research` | 调研外部系统时保存 evidence 与 source map |

Host binding 应是声明式映射，不应变成厚 adapter：

| Host | Instruction 安装 | Skill 安装 | Hook 安装 | 降级策略 |
|---|---|---|---|---|
| Hermes | context/guidance | `~/.hermes/skills` | plugin/shell hooks、curator、cron | 原生支持最完整 |
| Claude Code | `CLAUDE.md` / rules | `.claude/skills` | `SessionStart`、`UserPromptSubmit`、`Stop`、`PreCompact` 等 | scheduled/HTTP hooks 可选 |
| Codex | `AGENTS.md` | 用 repo docs/skills 或 prompt-discovered skill pack | 若无 hook，则 skill-only + manual review | 以 repo instructions 为主 |
| Cursor | `.cursor/rules/*.mdc` | rules 或文档化 skill pack | 依赖规则与外部脚本能力 | 静态 rules 强，自动维护弱 |
| Continue | `.continue/rules/*.md` | rules/context providers | 依赖配置与外部工具 | 适合 recall/remind |
| Generic agent | project instruction | Markdown skill directory | wrapper script 或 manual command | 至少 L0/L1 |

## 9. Harness Framework 抽取

不要抽 Hermes 的产品形态，也不要抽一个新的 agent runtime。应抽“可安装的自进化 harness”：一组 host-agnostic artifacts + semantic lifecycle + safety contracts。

### Harness Artifacts

Harness 不导出 class，也不要求 host link 一个 runtime library。下面列的是语义角色，必须落到文件、schema、prompt 模板或可选脚本上：

| 语义角色 | Harness artifact | Host 负责什么 |
|---|---|---|
| Harness package | `harness.yaml`、`INSTALL.md`、`GUIDELINE.md` | 读取安装契约，决定支持级别 |
| Host binding | `install/hosts/*.yaml` 或 `INSTALL.md` fenced YAML | 在安装期映射 instruction、skills、hooks、scheduler |
| Skill pack | `skills/*/SKILL.md` + support files | 注册或按需读取 skill |
| Prompt assets | `GUIDELINE.md`、`prompts/recall.md`、`prompts/reflection.md`、`prompts/curator.md` | 注入或调用 prompt 模板 |
| Hook templates | `hooks/recall.*`、`hooks/observe.*`、`hooks/reflect.*`、`hooks/curate.*` | 在 host lifecycle 中执行 |
| Hot memory schema | `schemas/hot-memory.schema.json`、`memory/hot/*.md` | host 或 hook 写入并控制预算 |
| Skill schema | `schemas/skill.schema.json` | host 或脚本校验 frontmatter、size、support dirs |
| Usage/provenance sidecar | `state/usage.json`、`schemas/usage.schema.json` | host/hook 更新 view/use/patch/state/pinned |
| Safety scripts | `scripts/scan-memory-write`、`scripts/validate-skill`、`scripts/check-target-allowlist` | host 在写前调用；不能调用则降级 proposal-only |
| Write allowlist | `schemas/write-target-allowlist.json` | host permission 层强制限制可写目标 |
| Report templates | `reports/templates/*.md`、`schemas/report.schema.json` | host 写 review/curator/eval report |
| Backup policy | `schemas/backup-policy.json`、`scripts/snapshot`、`scripts/rollback` | host 执行或替换为自身备份能力 |
| Cold memory protocol | `schemas/cold-memory-*.json`、`prompts/recall.md` | 外部服务或 host 实现 sync/prefetch |
| Eval gate | `eval/constraints.yaml`、`eval/templates/pr.md` | CI 或 host 执行测试、benchmark、PR |

因此，`PromptAssembler`、`HookBus`、`Scheduler`、`ToolRouter`、`ReflectionExecutor` 都不是 harness 内部组件。它们属于 host。Harness 只提供可被这些 host 能力消费的 artifacts。

### Semantic Events

这些是 harness 的语义事件，host binding 负责映射到具体 agent 的事件名或 fallback：

| 事件 | 目的 | 无原生 hook 时的 fallback |
|---|---|---|
| `session_start` | 加载 hot memory、guideline、skill index | project instruction 中要求每次启动先读 |
| `pre_llm_call` | 注入 recall、hook context、reminder | `recall` skill 手动调用 |
| `pre_tool_call` | 安全扫描、权限控制 | safety guideline + host permission model |
| `post_tool_call` | 记录工具坑点、usage、evidence | `observe` skill 或 session-end summary |
| `turn_delivered` | 用户已收到回复后，异步启动受限 reflection | `reflect` skill / `Stop` hook / manual command |
| `pre_compact` | 从即将丢失的上下文提取连续性 | `/compact` 前手动 flush skill |
| `session_end` | flush、summarize、review | end checklist |
| `idle_tick` | curator、dreaming、archive、backup | manual `curate` run |
| `scheduled_tick` | 定期维护和 eval | external cron / CI |
| `manual_review` | 用户主动 dry-run / apply | 必须支持 |

### Lifecycle

```text
Hot path:
  host loads harness guideline -> answer task -> sync cold memory -> optional reflection job

Warm maintenance:
  after-turn review -> memory/skill patch -> action summary

Cold maintenance:
  idle curator -> consolidate/archive -> rewrite references -> report -> backup

Offline evolution:
  dataset -> candidate generation -> constraints/tests -> proposal/PR
```

这是三速 + 离线模型。它不要求 harness 接管 agent loop，只要求 host 在对应生命周期点执行 harness 的语义动作。当前任务不被整理污染；整理有自己的权限、预算和报告；高风险演化需要 eval 和人工合并。

### MVP

最小可用 harness 应保留五组 artifacts：

1. `memory/hot/MEMORY.md`、`memory/hot/USER.md`、`schemas/hot-memory.schema.json`、`scripts/scan-memory-write`。
2. `skills/*/SKILL.md` 目录规范、`schemas/skill.schema.json`、`scripts/validate-skill`。
3. `state/usage.json`、`schemas/usage.schema.json`，字段包含 `created_by`、`provenance`、view/use/patch、state、pinned、archive。
4. `schemas/write-target-allowlist.json`，默认只允许 `memory/**`、`skills/**`、`state/**`、`reports/**`。
5. `skills/reflect/`、`prompts/reflection.md`、`hooks/reflect.*`，用于 post-turn reflection；如果 host 不能限制 toolset，则只写 `reports/reflection/` proposal。

MVP+ 再加：

6. `skills/curate/`、`prompts/curator.md`、`reports/templates/curator.md`，默认 dry-run。
7. `scripts/snapshot`、`scripts/rollback`、`schemas/backup-policy.json`，真实 mutation 前 snapshot。
8. `harness.yaml` 与 `INSTALL.md` host binding。

验收标准：

- reflection job 写入的 skill 能打上 self-authored provenance。
- 前台用户创建的 skill 不进入自动 curator candidate。
- hot memory 超预算时拒写，而不是截断。
- host 的 after-turn reflection binding 不阻塞主回复，也不改当前 system prompt cache。
- curator mutation 先写 report；真实 apply 前有 snapshot。

### Full Version

完整版本增加：

1. 冷记忆 protocol：session/evidence/index/prefetch 的 schemas、prompts、tool contract。
2. pre-compact flush。
3. dreaming：topic consolidation、promotion/demotion proposals。
4. scheduled jobs 引用 rewrite。
5. LLM curator structured YAML reconciliation。
6. dry-run 工具层强制 read-only。
7. eval-driven optimizer。
8. 跨 agent install maps。

## 10. 源码级注意点

### `skill_manage(delete)` 与 archive 语义不一致

Curator prompt 强调“不要 delete，最大破坏动作是 archive”，但当前源码中 `tools/skill_manager_tool.py::_delete_skill()` 实际 `shutil.rmtree(skill_dir)` 并 `forget(name)`。真正 recoverable archive 在 `tools/skill_usage.py::archive_skill()`，会移动到 `.archive/`。

抽取 harness 时应提供一等 `archive_skill()` mutation API，不要让 LLM 用 delete 表达 archive。

### Dry-run 不能只靠 prompt

`CURATOR_DRY_RUN_BANNER` 要求 report-only，但 `_run_llm_review()` 仍然 fork 常规 agent。抽取 harness 时 dry-run 应在 tool router 层只暴露 read-only 工具。

### Curator 权限比 Background Review 更宽

Background review 明确 `enabled_toolsets=["memory","skills"]`，`max_iterations=16`。Curator fork 没有同样清晰的 toolset 限制，prompt 甚至允许 terminal move。抽取时应拆分权限：

| 模式 | 工具 |
|---|---|
| dry-run | list/view/report only |
| proposal | read + write report |
| apply | skill patch/archive + backup + reference rewrite |
| rollback | backup restore only |

### 文档与源码会漂移

Hermes 官方 curator 文档和当前源码在 candidate 范围等细节上存在漂移。自进化系统必须让 report、source map、tests 和源码锚点成为规范的一部分。

### 自动治理只动明确 self-authored 资产

不要因为文件在同一目录就自动治理。必须保留 `created_by`、`risk`、`pinned`、`source`、`absorbed_into` 等字段。

## 11. 不应抽出的 Hermes 细节

| Hermes 细节 | 原因 |
|---|---|
| TUI/CLI 输出 | UI 层，不是自进化核心 |
| provider/model/runtime resolution | 每个平台 credential/runtime 不同 |
| gateway、Telegram、Discord 适配 | 平台集成，不是 harness 内核 |
| 完整 `AIAgent` | harness 不引入 agent 抽象；agent runtime 完全归 host |
| hub/bundled skill 安装细节 | package-source adapter |
| OpenRouter/Ollama/NVIDIA 配置 | runtime plugin |
| v0.13 prompt 文案 | 应抽原则和模板，不照搬 |

## 12. 推荐实施顺序

1. 写清 `GUIDELINE.md`：memory vs skill、proposal-first、热/温/冷分层。
2. 写清 `INSTALL.md`：四阶段 hook 和平台映射。
3. 定义 3 到 5 个 core skills。
4. 实现 report 格式，不急着自动改文件。
5. 实现 hot memory budget 和 demotion proposal。
6. 实现 skill curator proposal。
7. 接 cold memory index/search。
8. 做 pre-compact flush。
9. 做 dreaming promotion。
10. 做 eval-driven self-evolution。

## 13. 最终判断

如果直接抽 Hermes 的自进化 harness，最好的形态不是：

```text
memory database + thick adapter
or
new agent framework
```

而是：

```text
installable harness package
+ host binding contract
+ Markdown-first behavioral artifacts
+ skill-first procedural memory
+ bounded hot memory
+ warm capsules
+ cold memory providers
+ hook-driven nudges/reminders
+ after-turn self-review
+ curator/dreaming maintenance
+ usage/provenance sidecar
+ reports/backups/rollback
+ eval-driven offline evolution
```

这套 harness 的核心价值在于：不接管 host agent，却能让 host agent 读写热行为资产，让人类 review，让工程层治理容量、权限、provenance 和回滚。Hermes 源码证明轻量路径可以形成闭环；社区实践说明 Markdown 是当前 agent 生态最可迁移的控制面；hot/warm/cold 和 curator/dreaming 则是解决长期增长的必要补充。

## 14. 源码证据索引

| 主题 | 源码位置 |
|---|---|
| bounded `MEMORY.md` / `USER.md` | `tools/memory_tool.py` 的 `MemoryStore`、`memory_tool`、`MEMORY_SCHEMA` |
| prompt guidance | `agent/prompt_builder.py` 的 `MEMORY_GUIDANCE`、`SESSION_SEARCH_GUIDANCE`、`SKILLS_GUIDANCE` |
| skill 读路径 | `tools/skills_tool.py` 的 `skills_list`、`skill_view`、usage bump wrapper |
| skill 写路径 | `tools/skill_manager_tool.py` 的 `skill_manage`、frontmatter/size/path validators |
| provenance | `tools/skill_provenance.py` 的 `ContextVar` 和 `is_background_review()` |
| usage/state/archive | `tools/skill_usage.py` 的 `.usage.json`、`archive_skill()`、`restore_skill()` |
| after-turn review | `run_agent.py::_spawn_background_review()` 和主循环 finalization path |
| external memory | `agent/memory_manager.py`、`agent/memory_provider.py`、`run_agent.py::_sync_external_memory_for_turn()` |
| curator | `agent/curator.py` 的 `should_run_now()`、`apply_automatic_transitions()`、`run_curator_review()`、`_write_run_report()` |
| backup/rollback | `agent/curator_backup.py` 的 `snapshot_skills()`、`rollback()` |
| cron skill refs | `cron/jobs.py::rewrite_skill_refs()`、`cron/scheduler.py` skill loading |
| hooks | `hermes_cli/hooks.py`、`agent/shell_hooks.py`、`run_agent.py` plugin hook call sites |
| offline evolution | `hermes-agent-self-evolution` 的 `PLAN.md`、`evolution/core/config.py`、`evolution/core/constraints.py` |

关键数值事实基于上述 commits：

| 数值 | 源码锚点 |
|---|---|
| memory 目录与 `§` delimiter | `tools/memory_tool.py:55-59` |
| memory threat scanner | `tools/memory_tool.py:67-104` |
| `MEMORY.md` 2200 chars、`USER.md` 1375 chars | `tools/memory_tool.py:118-124` |
| skill body 100,000 chars、支持文件 1 MiB、支持目录白名单 | `tools/skill_manager_tool.py:164-171` |
| background review `max_iterations=16`、只启用 `memory`/`skills`、origin=`background_review` | `run_agent.py:3703-3717` |
| curator 7d interval、2h idle、30d stale、90d archive | `agent/curator.py:56-59` |
| curator backup 默认保留 5 份 | `agent/curator_backup.py:57` |
| self-evolution iterations/population/model/size/growth/eval split 默认值 | `evolution/core/config.py:17-35` |

## 15. 参考来源

- Hermes Agent curator: <https://hermes-agent.nousresearch.com/docs/user-guide/features/curator>
- Hermes Agent memory: <https://hermes-agent.nousresearch.com/docs/user-guide/features/memory>
- Hermes Agent hooks: <https://hermes-agent.nousresearch.com/docs/user-guide/features/hooks>
- Hermes Agent cron: <https://hermes-agent.nousresearch.com/docs/user-guide/features/cron>
- Hermes Agent Self-Evolution: <https://github.com/NousResearch/hermes-agent-self-evolution>
- Claude Code memory: <https://code.claude.com/docs/en/memory>
- Claude Code skills: <https://code.claude.com/docs/en/skills>
- Claude Code hooks: <https://code.claude.com/docs/en/hooks>
- OpenAI Codex AGENTS.md / Codex introduction: <https://openai.com/index/introducing-codex/>
- OpenAI Codex agent loop: <https://openai.com/index/unrolling-the-codex-agent-loop/>
- Cursor rules: <https://docs.cursor.com/en/context>
- Continue rules: <https://docs.continue.dev/customize/rules>
- OpenClaw skills: <https://docs.openclaw.ai/tools/creating-skills>
- OpenClaw dreaming: <https://docs.openclaw.ai/concepts/dreaming>
- MemGPT paper: <https://arxiv.org/abs/2310.08560>
- Anthropic Agent Skills: <https://docs.claude.com/en/docs/agents-and-tools/agent-skills>
