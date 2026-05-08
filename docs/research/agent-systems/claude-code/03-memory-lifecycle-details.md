# Claude Code memory lifecycle 细节

> 边界：本页只基于 Claude Code 官方公开文档与公开可见行为，不使用泄漏源码或非公开实现细节。所有数字与字段名引自 `code.claude.com/docs/en/*`。

## 核心判断

Claude Code 的 memory 设计是「启动时加载 Markdown 指令 + auto memory（agent 自写）+ 长会话时 compaction + session-scoped 自动化」。它没有把 memory 做成独立数据库 runtime，而是让 `CLAUDE.md`、`.claude/rules/`、auto memory、skills、hooks 与 scheduled tasks 共同构成行为层。

这对 Mnemon 的意义是：第一阶段可以把安装说明、行为 guideline 和 hook 阶段写成 Markdown，让 agent 按文档为自己安装，而不必先做复杂 adapter。

## 生命周期详表

| 维度 | 公开观察 |
|---|---|
| 主要记忆载体 | 项目 `./CLAUDE.md` 或 `./.claude/CLAUDE.md`；用户 `~/.claude/CLAUDE.md`；本地 `./CLAUDE.local.md`；managed `CLAUDE.md`（macOS `/Library/Application Support/ClaudeCode/CLAUDE.md`、Linux/WSL `/etc/claude-code/CLAUDE.md`、Windows `C:\Program Files\ClaudeCode\CLAUDE.md`）；`.claude/rules/*.md`；auto memory `~/.claude/projects/<project>/memory/MEMORY.md` 与 topic 文件；skills 与 subagent 自身 memory。 |
| 存储位置 | 组织 / 项目 / 用户 / 本地四 scope；项目级随仓库提交，本地级应加入 `.gitignore`；auto memory 默认按 git repo 隔离，可由 managed/user 设置 `autoMemoryDirectory` 重定向（不接受 project/local 设置以防被劫持）。 |
| 加载时机 | 启动时沿目录层级加载工作目录及其祖先目录的 `CLAUDE.md` 与 `CLAUDE.local.md`；子目录 `CLAUDE.md` 与 path-scoped rules 在读取匹配文件时按需加载；auto memory 在每次会话起始注入「前 200 行或 25KB，先到为准」；skill body 在被调用时整段注入。 |
| 装载顺序 | 文件系统 root 方向靠前，工作目录靠后；同一目录 `CLAUDE.local.md` 排在 `CLAUDE.md` 之后；`@path` import 在 host 文件位置原地展开；递归 import 最大深度 5 跳。 |
| 读路径 | Claude 把已加载的 Markdown 放入当前上下文；`/memory` 列出所有当前会话已加载的 `CLAUDE.md` / `CLAUDE.local.md` / rules，并切换 auto memory 开关；`/context` 给出按类别的 token 占用与建议。 |
| 写路径 | 人类直接编辑文件；`/init`（含 `CLAUDE_CODE_NEW_INIT=1` 多阶段流程）生成初稿；用户对 Claude 说「remember」「always do X」一类话由 Claude 写入 auto memory；说「add this to CLAUDE.md」由 Claude 改写 `CLAUDE.md`；hooks 可以输出 `additionalContext` 但不直接改写文件。 |
| 长度建议 | `CLAUDE.md` 单文件目标 ≤200 行；超长会消耗 token、降低遵循度。 |
| Auto memory 注入 | `MEMORY.md` 注入「前 200 行或 25KB，先到为准」；超出部分不在启动时加载；topic 文件（如 `debugging.md`）按需用普通文件读取工具读入。 |
| Skill body 注入 | 调用时整段注入并保留至会话结束；compaction 后每个被调用过的 skill 至多保留 5,000 tokens、所有 skill 合计上限 25,000 tokens，按调用时间从新到旧填，超出从旧到新丢弃，截断保留文件起始部分。 |
| Skill 列表预算 | skill 描述列表按上下文窗口的 1% 动态预算（fallback 8,000 字符）截断；每条 `description` + `when_to_use` 合计上限 1,536 字符；可由 `SLASH_COMMAND_TOOL_CHAR_BUDGET` 环境变量上调，或用 `skillOverrides` 设 `"name-only"` / `"off"` 节省预算。 |
| Import 限制 | `@path` 递归 import 最大深度 5；首次见到外部 import 会弹出审批对话框，拒绝后该 import 永久禁用且不再询问。 |
| Hook 输出限制 | hook 注入 context 的总文本（`additionalContext` + `systemMessage` + plain stdout）capped at **10,000 字符**，超出落盘并以预览 + 路径形式出现。 |
| Hook 默认超时 | command 600s、HTTP 30s、prompt 30s、agent 60s；可逐 hook 用 `timeout` 字段覆盖。 |
| 超出处理 | 长会话通过 `/compact`（手动）或自动 compaction 把历史替换为结构化摘要；详见下节。 |
| 整理方式 | 主要依赖人工或 agent 按文档重写 Markdown；官方建议把最重要内容放前面、保持具体、用标题组织、单文件 ≤200 行；auto memory 由 Claude 自维护索引和分主题文件。 |
| 定时任务 | `/loop` bundled skill 在当前 session 内反复运行 prompt；`CronCreate` / `CronList` / `CronDelete` 工具直接被 Claude 调用；最小 1 分钟间隔，秒级输入向上取整；session 同时容纳上限 50 个任务；recurring 任务 7 天后自动到期；`Esc` 取消等待中的 `/loop`。 |
| 持久性 | `/loop` 与 cron 任务都是 session-scoped；`--resume` 或 `--continue` 仅恢复未到期的（recurring 创建后 7 天内、one-shot 时间未过）；新 conversation 清空。Routines / Desktop scheduled tasks / GitHub Actions 才适合跨 session 自动化。 |
| 安全边界 | 组织 / 项目 / 用户 / 本地 scope 分层；本地文件不应提交；外部 import 首次审批；hooks 可在关键事件插入检查；`allowManagedHooksOnly` 可阻断非 managed hook；plugin subagent 不允许 `hooks` / `mcpServers` / `permissionMode`；`disableSkillShellExecution: true` 可禁用 skill 的 shell 注入。 |

## CLAUDE.md 装载次序与字符成本

公开 memory + context-window 文档给出可观察的 CLAUDE.md 行为：

- 启动时沿目录树向上遍历，所有命中文件 **拼接** 进上下文，不互相覆盖；root 方向靠前，工作目录靠后；同目录 `CLAUDE.local.md` 排在 `CLAUDE.md` 之后。
- 子目录的 `CLAUDE.md` 与 `CLAUDE.local.md` 不在启动时加载；Claude 读取该子目录文件时才注入 message history。
- managed `CLAUDE.md` 始终被加载；用户的 `claudeMdExcludes` glob 不能跳过 managed 路径，仅能跳过非 managed 文件。
- block-level HTML 注释（`<!-- ... -->`）在注入前被剥离，可写人类维护笔记不消耗 token；代码块中的注释保留；Read 工具直接读 `CLAUDE.md` 时注释也保留。
- `@path` import 在 host 文件位置原地展开；相对路径以宿主文件为基准（不是工作目录）；递归 import 最大深度 5 跳；首次外部 import 弹审批，拒绝后永久禁用。
- `--add-dir` 默认不加载该目录的 `CLAUDE.md`；设 `CLAUDE_CODE_ADDITIONAL_DIRECTORIES_CLAUDE_MD=1` 才加载，且加载范围包括 `CLAUDE.md` / `.claude/CLAUDE.md` / `.claude/rules/*.md` / `CLAUDE.local.md`（`local` 可被 `--setting-sources` 排除）。

文档建议每个 `CLAUDE.md` ≤200 行；超长会消耗 token 并降低遵循度。`@path` import 不会减少 token 占用，仅是组织上的拆分；要节省 token 应把内容搬到 `.claude/rules/` 并加 `paths:` frontmatter，使其按需加载。

## 写入与整理机制

Claude Code 的写入路径偏 Markdown-native：

1. `CLAUDE.md` 保存项目架构、构建/测试命令、代码风格、工作流、常见坑。
2. 用户级 `~/.claude/CLAUDE.md` 保存个人偏好。
3. 本地 `CLAUDE.local.md` 保存不该提交的个人 / 环境信息。
4. 大型项目用 `@path` imports 拆分，或 `.claude/rules/*.md` 加 `paths:` 做路径作用域。
5. 成熟流程放入 skills 或 slash commands，而不是不断追加到主 memory。
6. Auto memory 由 Claude 自己写入 `~/.claude/projects/<project>/memory/`，索引文件 `MEMORY.md` 保持简短，详细笔记移入同目录的 topic 文件。

这说明 memory 文件不是无限增长的日志。好的做法是把条目整理成稳定政策、短流程、命令索引和路径规则。Claude Code 自身没有公开的 cron-driven memory consolidation；整理仍是「人 + agent 协作改 Markdown」。

## Skill body 在长会话中的命运

Skill body 的生命周期和 `CLAUDE.md` 不同：

- 调用时整段注入到当前消息流，并保留到会话结束；Claude Code 不会在后续 turn 重读 skill 文件。
- 若 skill 行为「在第一条响应后变弱」，文档解释多半是模型选择了别的工具，而不是 skill 内容被丢弃。建议加强 `description` 与 instruction，或用 hook 强制行为。
- compaction 后，每个**被调用过的** skill 会重新注入；每个上限 5,000 tokens、所有 skill 合计 25,000 tokens；按调用时间从新到旧填，超出从最旧的整段丢弃；截断保留文件起始部分（因此重要内容应放 `SKILL.md` 顶部）。
- skill 描述列表（启动时让 Claude 知道有哪些 skill 可调）**不会** 在 compaction 后重注入。这意味着调过的 skill body 还在，但「该不该再调用某 skill」的判断信号会缺失，Mnemon 在跨 runtime 时不应假设「曾经显示过的 skill 仍可被自主选择」。
- 想在 compaction 后强制刷新 skill 信号，应在 `SessionStart` (matcher `compact`) 或 `PostCompact` hook 中重新注入摘要。

## Compaction 行为

Claude Code 的上下文页明确给出 compaction 后各机制的命运：

| 机制 | Compaction 后行为 |
|---|---|
| system prompt 与 output style | 不变；不属于消息历史 |
| 项目 root `CLAUDE.md` 与 unscoped rules | 从磁盘重新注入 |
| Auto memory（`MEMORY.md`） | 从磁盘重新注入 |
| 带 `paths:` 的 rules | 丢失，直到再次读取匹配文件 |
| 子目录嵌套的 `CLAUDE.md` | 丢失，直到再次读取该子目录中的文件 |
| 已调用的 skill bodies | 重新注入；每个 skill 上限 5,000 tokens、所有 skill 合计 25,000 tokens；超出从最旧的开始整段丢；截断保留文件起始部分 |
| Skill 描述列表 | **不重新注入**；只有真正被调用过的 skill 会保留 |
| Hooks | 不适用（hook 是代码执行，不是上下文内容） |

`PreCompact` hook（matcher `manual` / `auto`）可在 compaction 前执行任意逻辑，并可通过 exit code 2 阻断；`PostCompact` 仅通知，不能阻断。`SessionStart` hook 的 `source` 字段在 compaction 后会以 `compact` 触发，可借此重新注入提醒。

这对 Mnemon 很关键：必须持久存在的安装指引应放 root-level guideline 或 INSTALL；路径 / 阶段细节可以放 skill 或 hook prompt，但不能假设它们在 compaction 后一直完整可见。同样，靠 skill 描述识别「该不该走某流程」的设计在 compaction 后会失效，必须由 hook 或主 `CLAUDE.md` 重新提示。

## 失败与拒绝场景

公开文档明确给出的可观察行为：

- Hook exit code `2` 在不同事件下含义不同：`PreToolUse` 阻断该工具调用、`UserPromptSubmit` 拒绝并擦除该 prompt、`Stop` / `SubagentStop` 阻止结束、`PreCompact` 阻止 compaction、`PostToolUse` / `PostToolUseFailure` 不能阻断（仅 stderr 反馈给 Claude）。
- Hook exit 非 0 非 2：非阻断错误，stderr 第一行进 transcript，全文写 debug 日志，会话继续。
- Hook 注入 context 超过 10,000 字符：超出部分写到文件，模型只看到预览 + 路径。
- HTTP hook 非 2xx / 连接失败 / 超时（默认 30s）：非阻断错误。
- Skill 调用时若用户用 `permissions.deny` 中加 `Skill(name)`：直接拒绝。
- Subagent `bypassPermissions` 仍触发 root / 家目录的断路器（如 `rm -rf /`）。
- Auto memory 写入路径被 `autoMemoryDirectory` 重定向，但该 key 仅 managed/user 设置或 `--settings` 接受，避免被克隆仓库劫持到敏感位置。
- `/loop` 与 cron 任务最小间隔 1 分钟，秒级输入向上取整；不规则间隔（如 `7m`、`90m`）会被取整到最近的合法 cron step；recurring 任务有 7 天到期机制。
- `CLAUDE_CODE_DISABLE_CRON=1` 可彻底关掉调度，已存在任务停火。

## 定时任务与后台任务

Claude Code 的 scheduled tasks 三类（公开 scheduled-tasks 页给出对照表）：

| 维度 | Cloud / Routines | Desktop scheduled tasks | `/loop` |
|---|---|---|---|
| 运行位置 | Anthropic 托管 | 本机 | 本机 |
| 需要机器开机 | 否 | 是 | 是 |
| 需要会话开启 | 否 | 否 | 是 |
| 重启后保留 | 是 | 是 | `--resume` 时若未到期则恢复 |
| 访问本地文件 | 否（fresh clone） | 是 | 是 |
| MCP servers | 每任务单独配置 | 配置文件 + connectors | 继承当前会话 |
| 权限提示 | 否（自动运行） | 每任务可配 | 继承会话 |
| 最小间隔 | 1 小时 | 1 分钟 | 1 分钟 |

`/loop` 行为：

- `/loop 5m check the deploy`：cron 化为固定间隔。
- `/loop check the deploy`：每轮 Claude 自选 1 分钟到 1 小时间隔（Bedrock / Vertex / Foundry 上回退为固定 10 分钟）。
- `/loop`：运行内置 maintenance prompt，或项目级 `.claude/loop.md` / 用户级 `~/.claude/loop.md`（前者优先），文件超 25,000 bytes 会被截断。

公开文档没有把这些任务描述为自动整理 `CLAUDE.md` 的内置机制。它们可以被用来触发「检查记忆候选」「总结最近工作」「提醒保存状态」一类 prompt，但 memory 的最终整理仍应是 Markdown diff + review，而不是默认自动改写。Jitter 规则：recurring 任务在调度时刻后最多 30 分钟内触发（hourly 以下取间隔一半），one-shot 整点 / 半点任务最早提前 90 秒触发，offset 由任务 ID 决定可重复。

## Subagent 自身的记忆生命周期

公开文档让 subagent 可以拥有自己的 `MEMORY.md`，独立于主会话的 auto memory：

- frontmatter `memory: user|project|local` 决定持久目录位置：`~/.claude/agent-memory/<name>/`、`.claude/agent-memory/<name>/`、`.claude/agent-memory-local/<name>/`。
- 启用后 Read / Write / Edit 工具自动开启，subagent 可主动维护自己的笔记。
- system prompt 中包含「读取并维护此目录」的指导，并注入 `MEMORY.md` 的「前 200 行 / 25KB，先到为准」。
- 文档建议在 subagent body 里写明「开工前查 memory，结束前更新 memory」，让 agent 自己驱动学习闭环。

这一设计对 Mnemon 的启发：每种「角色化的整理任务」都可以拥有自己的独立 memory 目录，避免和主会话的事实库混在一起。例如「review subagent」记录代码评审中反复出现的模式；「debug subagent」记录调试套路。Mnemon 数据库表结构可以为「来源 agent」加索引，模拟同样的隔离。

## /loop 与 cron 的可观察行为

- 调度器每秒检查到期任务，并按低优先级入队；任务在 Claude 的 turn 之间触发，不打断当前回答。
- 时间均按本地时区解析；`0 9 * * *` 是本地 9am 而非 UTC。
- Jitter 规则：recurring 任务在调度时刻后最多 30 分钟内触发（hourly 以下取间隔一半）；one-shot 整点 / 半点任务最早提前 90 秒触发；offset 由任务 ID 决定，可重复。如要精确触发，避开 `:00` 与 `:30`。
- 一个 session 同时容纳 50 个调度任务上限。
- `CronCreate` 接受 5 字段标准 cron（分 时 日 月 周），`*` / 单值 / 步长 `*/15` / 范围 `1-5` / 列表 `1,15,30` 都支持；不支持 `L` / `W` / `?` 与名字别名。
- Bedrock / Vertex AI / Microsoft Foundry 上 `/loop` 不带 prompt 时打印用法，不带 interval 但有 prompt 时回退为 10 分钟固定间隔。
- 设 `CLAUDE_CODE_DISABLE_CRON=1` 关闭整个调度器，已存在任务停火。

## 对 Mnemon 的启发

Mnemon 应学习 Claude Code 的轻量边界，并区分「可借鉴」与「Claude Code 独有」：

可借鉴：

- `INSTALL.md` 说明如何把 Mnemon hook 安装到当前 agent；类比 Claude Code 的 `/init` 思路。
- `GUIDELINE.md` 保存稳定行为原则，并保持 root-level 可见、单文件控制规模。
- skill 负责过程，memory 负责事实，不把所有东西塞进一份主文件；类比 skills 与 `CLAUDE.md` 的分工。
- hook 在 session start、prompt submit、tool 后、stop / compact 前提醒 agent 执行记忆动作；输出限定为短 `additionalContext` 形态，控制 1KB 内远低于 10K 上限。
- 对可能膨胀的内容使用「候选 patch + review」而不是自动追加；类比 Claude Code 把 auto memory 暴露为可审查的 plain Markdown。

Claude Code 独有、不应在 Mnemon 第一阶段照搬：

- worktree isolation 与 plan mode 依赖 Claude Code 的 runtime；
- 内置 `Explore` / `Plan` subagent 与 agent teams 是产品级特性，本地 CLI 无法 1:1 复刻；
- `permissions.allow / deny / ask` 与 sandbox config 是 Claude Code 的安全模型，Mnemon 不需要在 hook 层重做；
- `/compact` 自动重注入 `CLAUDE.md` 与 auto memory 是 Claude Code runtime 的能力，本地 CLI 中由 agent 自行决定何时重读相关文件即可。

## InstructionsLoaded 揭示的加载链路

公开 `InstructionsLoaded` hook 的 matcher 取值可解释 5 种加载触发原因：

- `session_start`：会话启动时遍历到的 `CLAUDE.md` / unscoped rule 加载；
- `nested_traversal`：Claude 读取子目录文件，触发该子目录 `CLAUDE.md` / `CLAUDE.local.md` 加载；
- `path_glob_match`：path-scoped rule 的 `paths:` 命中触发文件读取后加载；
- `include`：`@path` import 展开时加载；
- `compact`：compaction 后从磁盘重新注入 root `CLAUDE.md` / unscoped rules / auto memory。

输入字段含 `file_path`、`memory_type`（`Project` / `User` / `Local` / `Managed` / `Auto` 等）、`load_reason`、`globs`、`trigger_file_path`、`parent_file_path`，可精确观察哪些指令在何时进入上下文。Mnemon 在跨 runtime 设计 hook 时可以借鉴这一观测能力，把每次注入的来源、原因、触发文件写入日志，便于事后审查 stale memory 与 race condition。

## 装载次序与启动 token 占用

公开 context-window 文档以一个交互演示给出会话起始的代表性 token 量级（仅作示意）：

1. system prompt（~4,200 tokens，不可见）
2. auto memory `MEMORY.md`（前 200 行 / 25KB，先到为准）
3. environment info（cwd、平台、shell、OS、git 状态，~280 tokens）
4. MCP 工具名（默认 deferred schemas，可由 `ENABLE_TOOL_SEARCH` 改为 `auto` 或 `false`）
5. skill 描述列表（按 1% 上下文窗口或 fallback 8,000 字符截断）
6. 用户级 `~/.claude/CLAUDE.md`
7. 项目 `CLAUDE.md`（含 imports）
8. 工作目录及其祖先目录的其他 `CLAUDE.md` / `CLAUDE.local.md` / 无 `paths:` 的 rules

之后才是用户首条 prompt。子目录的 `CLAUDE.md` 与 path-scoped rules 在 Claude 读取匹配文件后才进入 message history。

## 失败/拒绝场景的 Markdown 化补充

下面把公开文档与上下文文档中分散的失败语义集中成一组对 Mnemon 可观察的事件清单，便于 Mnemon hook 在跨 runtime 时给出一致的回退：

- `CLAUDE.md` 文件不存在或被 `claudeMdExcludes` 跳过：不报错；`/memory` 中不会列出。
- `@path` 指向不存在的文件：路径被作为字面文本保留在上下文中，社区观察上 Claude 通常会忽略它。
- `@path` 外部 import 被用户首次拒绝：永久禁用，不再显示审批对话；除非删除并重新加入。
- `MEMORY.md` 超过 200 行 / 25KB：超出部分不在启动注入，但仍可被 Claude 通过 Read 工具按需读取；文档建议 Claude 主动把详细内容搬到 topic 文件并保持索引短。
- skill body 在 compaction 后超过单 skill 5,000 token：截断保留文件起始；超过总 25,000 token：从最旧调用开始整段丢弃。
- skill 描述列表超过 1% 上下文窗口（fallback 8,000 字符）：按字符串预算截断，可能截掉关键 trigger 词，导致 Claude 不再认得该 skill。
- hook command 超 600s（HTTP 30s / prompt 30s / agent 60s）：非阻断错误，stderr 第一行进 transcript。
- hook 注入文本超 10,000 字符：超出落盘，模型只看到预览 + 路径。
- `permissions.deny` 中加 `Skill(name)` 命中：调用直接拒绝；加 `Skill` 单独条目则禁用所有 skill。
- `disableSkillShellExecution: true` 命中：``!`cmd``` 与 ```` ```! ```` 替换为 `[shell command execution disabled by policy]`，body 其他部分保留。
- subagent `bypassPermissions` 试图删除 root / 家目录：触发硬断路器，仍然弹权限提示。
- plugin subagent 写了 `hooks` / `mcpServers` / `permissionMode`：字段被静默忽略。
- `/loop` 任务最小间隔 1 分钟，秒级输入向上取整；不规则间隔（如 `7m` / `90m`）取整到最近合法 cron step；recurring 任务 7 天后自动到期并最后触发一次后删除。
- 关闭终端或 session 退出：所有 session-scoped 任务停火；`--resume` 仅恢复未到期任务（recurring 创建后 7 天内 / one-shot 时间未过）。

## 与 Mnemon SQLite 模型的差异

Claude Code 的 memory 是 plain Markdown，全部内容都可以被人 `cat` 出来；Mnemon 用 SQLite 存事实、关系与时间线，是结构化的。借鉴时要分清：

- Claude Code 的「索引 + topic」拆分给 Mnemon 的启发是 **导出层** 的形态：Mnemon 数据库可以导出一个 `MEMORY.md` 索引和若干 topic 文件用于 review，但权威数据仍在 SQLite 中。
- Claude Code 的 `MEMORY.md` 注入容量上限（前 200 行 / 25KB）给 Mnemon 的启发是 **prompt 注入层** 的形态：每次 hook 给 agent 的事实摘要也应有明确字符上限，而不是无脑全量注入。
- Claude Code 的 compaction 行为给 Mnemon 的启发是 **持久层 vs 会话层** 的边界：Mnemon SQLite 是持久层、可随时重读；hook 注入文本是会话层、在 compaction 后会被摘要替代，必须由后续 hook 重新注入。

## 参考来源

- 官方文档: [Claude Code Memory](https://code.claude.com/docs/en/memory)
- 官方文档: [Claude Code Settings](https://code.claude.com/docs/en/settings)
- 官方文档: [Claude Code Hooks](https://code.claude.com/docs/en/hooks)
- 官方文档: [Claude Code Subagents](https://code.claude.com/docs/en/sub-agents)
- 官方文档: [Claude Code Skills / Slash commands](https://code.claude.com/docs/en/slash-commands)
- 官方文档: [Claude Code Context Window](https://code.claude.com/docs/en/context-window)
- 官方文档: [Claude Code Scheduled Tasks](https://code.claude.com/docs/en/scheduled-tasks)
