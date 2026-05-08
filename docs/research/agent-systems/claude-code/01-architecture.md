# Claude Code 架构观察

> 边界：本文件不使用泄漏源码，只基于公开官方文档、公开社区讨论和可观察行为。文中所有数字和字段名引自 `code.claude.com/docs/en/*` 公开页面。

## 一句话结论

Claude Code 的整体形态是「agent runtime + Markdown 行为资产 + settings/hooks 扩展点 + subagent 隔离执行」。它并不要求项目为长期记忆实现复杂 adapter，而是把大部分行为表达在 `CLAUDE.md`、skills、commands、subagents 和 settings hooks 中。

## 公开架构面

Claude Code 公开文档体现出四个层次：

| 层 | 公开机制 | 作用 |
|---|---|---|
| 持久项目上下文 | `CLAUDE.md`、`@path` imports、`.claude/rules/`、auto memory | 给主 agent 注入项目规范、偏好、工作流，并允许 agent 自行积累学习 |
| 运行时配置 | `settings.json`、managed/user/project/local scope | 权限、hooks、env、模型、sandbox、plugin 启用 |
| 扩展动作 | skills（含原 commands）、`/loop` 与 cron tools | 把可复用操作和流程写成 Markdown，按需加载 |
| 隔离执行 | subagents（built-in 与自定义）、worktree isolation、agent teams | 把探索、评审、测试、记忆整理移出主上下文 |

官方 settings 文档把配置分为 managed、user、project、local 四个 scope，并明确给出文件位置：`.claude/settings.json`、`.claude/settings.local.json`、`~/.claude/settings.json`，企业 managed scope 在 macOS 是 `/Library/Application Support/ClaudeCode/managed-settings.json`，Linux/WSL 是 `/etc/claude-code/managed-settings.json`，Windows 是 `C:\Program Files\ClaudeCode\managed-settings.json`，外加 `managed-settings.d/` 目录按字母序合并。Subagents 文档说明 subagent 是 Markdown + YAML frontmatter 定义的专用 agent，有自己的 context window、system prompt、工具权限、模型选择、可选 worktree 隔离。

## settings 与 CLAUDE.md 的装载次序

公开 settings 页给出明确的优先级（高 → 低）：

1. Managed settings（不可被覆盖）
2. 命令行 `--settings` 参数
3. `.claude/settings.local.json`（本地，gitignored）
4. `.claude/settings.json`（项目共享）
5. `~/.claude/settings.json`（用户全局）

数组类设置（`permissions`、`sandbox.filesystem.allowWrite`、`enabledMcpjsonServers`、`claudeMdExcludes` 等）跨 scope **拼接并去重**，而不是覆盖。标量字段则按上述优先级取首个非空值。文档明确举例：用户允许某权限、项目 deny 同一权限时，project deny 胜出。Managed-only 字段（如 `allowManagedHooksOnly`、`allowManagedMcpServersOnly`、`allowManagedPermissionRulesOnly`、`forceLoginMethod`、`forceLoginOrgUUID`、`strictKnownMarketplaces`、`blockedMarketplaces`、`forceRemoteSettingsRefresh`、`channelsEnabled`、`pluginTrustMessage`、`wslInheritsWindowsSettings`）只能放在 managed scope，其他 scope 中即使写入也不生效。

公开文档还列出 settings 中常见的 key：`permissions.allow / deny / ask`、`permissions.defaultMode`、`permissions.additionalDirectories`、`model`、`availableModels`、`effortLevel`、`alwaysThinkingEnabled`、`env`、`hooks`、`allowedHttpHookUrls`、`httpHookAllowedEnvVars`、`disableAllHooks`、`enabledPlugins`、`extraKnownMarketplaces`、`sandbox.*`、`allowedMcpServers` / `deniedMcpServers`、`outputStyle`、`autoMemoryEnabled`、`autoMemoryDirectory`、`claudeMdExcludes`、`cleanupPeriodDays`、`disableSkillShellExecution`、`skillOverrides`。运行 `/status` 可看到当前生效的层来源（remote managed、plist、HKLM、文件等）。

CLAUDE.md 的装载是「从工作目录沿目录树向上遍历」，所有命中文件 **拼接进上下文**，而不是覆盖。文件系统 root 方向的内容靠前，工作目录的 `CLAUDE.md` 靠后；同一目录内 `CLAUDE.local.md` 排在 `CLAUDE.md` 之后。位于工作目录之下的子目录 `CLAUDE.md` 与 `CLAUDE.local.md` **不在启动时加载**，等 Claude 读取该子目录文件时再注入。`@path` imports 在启动时随宿主文件展开，相对路径以宿主文件为基准，递归 import 最大深度为 5。Block-level HTML 注释（`<!-- ... -->`）会在注入前被剥离，可用于不消耗 token 的人类注释。

CLAUDE.md scope 与位置同样有四层：

| Scope | 位置 |
|---|---|
| 组织级 managed | macOS `/Library/Application Support/ClaudeCode/CLAUDE.md`；Linux/WSL `/etc/claude-code/CLAUDE.md`；Windows `C:\Program Files\ClaudeCode\CLAUDE.md` |
| 项目 | `./CLAUDE.md` 或 `./.claude/CLAUDE.md` |
| 用户 | `~/.claude/CLAUDE.md` |
| 本地 | `./CLAUDE.local.md`，应加入 `.gitignore` |

文档建议每个 `CLAUDE.md` 控制在 200 行以下；超长会消耗 token、降低遵循度。`AGENTS.md` 不被直接读取，需要在 `CLAUDE.md` 中写 `@AGENTS.md` 显式 import。

Auto memory（v2.1.59+ 引入，默认开）每个 git 仓库一个目录：`~/.claude/projects/<project>/memory/`，入口文件 `MEMORY.md`，每次会话启动注入「前 200 行或 25KB，先到为准」，剩余 topic 文件按需读取。可通过 `autoMemoryDirectory` 重定向，但该 key 仅接受 managed/user 设置或 `--settings`，不接受 project/local，以防被克隆仓库劫持。

## Hook 模型

Claude Code hooks 是生命周期扩展点，而不是完整 workflow engine。官方 hooks 页列出了一长串事件（精确名称见公开文档），常用的包括：`SessionStart`、`Setup`、`UserPromptSubmit`、`UserPromptExpansion`、`PreToolUse`、`PostToolUse`、`PostToolUseFailure`、`PostToolBatch`、`PermissionRequest`、`PermissionDenied`、`SubagentStart`/`SubagentStop`、`Stop`、`StopFailure`、`PreCompact`/`PostCompact`、`InstructionsLoaded`、`ConfigChange`、`CwdChanged`、`FileChanged`、`Notification`、`SessionEnd` 等。

执行模型：

- exit code `0` 表示成功，stdout 若是合法 JSON 会被解析为输出协议（包括 `continue`、`stopReason`、`suppressOutput`、`systemMessage`、`hookSpecificOutput.additionalContext`、`hookSpecificOutput.permissionDecision` 等字段）。
- exit code `2` 表示阻断；具体语义因事件而异：`PreToolUse` 阻断该工具调用、`UserPromptSubmit` 拒绝并擦除该 prompt、`Stop`/`SubagentStop` 阻止结束、`PreCompact` 阻止 compaction、`PostToolUse`/`PostToolUseFailure` 不能阻断（因为工具已执行）但 stderr 会反馈给 Claude。
- 其他非零退出码视为非阻断错误，stderr 第一行会显示在 transcript，全文写 debug 日志，会话继续。
- hook 注入到上下文的内容（`additionalContext`、`systemMessage`、纯 stdout）有 **10,000 字符** 上限，超出会落盘并以预览 + 路径出现。
- 默认超时：command hook 600 秒、HTTP hook 30 秒、prompt hook 30 秒、agent hook 60 秒，可在每个 hook 上用 `timeout` 字段覆盖。
- HTTP hook 的 2xx 空 body 等价 exit 0，2xx 纯文本会作为 context 注入，2xx JSON 按 JSON 协议解析；非 2xx 与连接失败均按非阻断错误处理。

`PreToolUse` 的 `permissionDecision` 字段支持 `allow` / `deny` / `ask` / `defer`，多个 hook 同时返回时优先级为 `deny > defer > ask > allow`。`defer`（v2.1.89+）只在非交互模式（`-p` flag）下有效，把 Claude 暂停在该工具调用，等待外部决策；返回 `stop_reason: "tool_deferred"` 与 `deferred_tool_use` payload，恢复时再返回 `allow` / `deny`。`SessionStart`、`Setup`、`CwdChanged`、`FileChanged` 这一类事件还能向 `CLAUDE_ENV_FILE` 写入 `export VAR=value` 来持久化环境变量，供后续工具调用使用。Plain stdout 的处理因事件而异：`SessionStart` / `UserPromptSubmit` 等事件下纯 stdout 会被当作 context 注入，而 `PostToolUse` 等事件的 plain stdout 仅写 debug 日志。

Hook handler 类型有 5 种（`type: command | http | mcp_tool | prompt | agent`）。Command hook 支持 `async`（后台运行，不阻断）与 `asyncRewake`（后台运行 + exit 2 唤醒 Claude，stderr/stdout 作为 system reminder 注入）。Hook 配置可来自六个层级（高 → 低）：managed settings → `.claude/settings.local.json` → `.claude/settings.json` → `~/.claude/settings.json` → 启用插件的 `hooks/hooks.json` → skill / agent frontmatter `hooks:` 段。Matcher 字符串只含字母 / 数字 / `_` / `|` 时按精确匹配或 `|` 分隔列表处理；含其他字符时按 JavaScript regex 评估。`InstructionsLoaded` 事件的 matcher 取值为 `session_start` / `nested_traversal` / `path_glob_match` / `include` / `compact`，可用于精确观察哪些指令在何时进入上下文。

文档给出的安全建议：在命令中使用 `"$CLAUDE_PROJECT_DIR"` 双引号，避免空格；HTTP header 中使用 `allowedEnvVars` 白名单；高安全场景下 admin 设 `allowManagedHooksOnly: true` 以禁用项目/用户 hooks（仅放行 managed 与显式启用的 plugin）。`disableAllHooks: true` 可一刀切关闭所有 hook 而不删除配置，便于排错。

## Hook handler 类型与连接方式

公开 hooks 文档说明 5 种 handler，可对应不同的 Mnemon 接入路径：

- `command`：执行 shell 命令；通用，最适合 Mnemon 的 CLI 注入；支持 `async`（后台运行不阻断）与 `asyncRewake`（后台运行 + exit 2 时唤醒 Claude，stderr/stdout 进入 system reminder）。`shell` 字段可选 `bash`（默认）或 `powershell`。
- `http`：发送 POST 到 URL；2xx + 空 body 等价 exit 0；2xx + 纯文本作为 context 注入；2xx + JSON 按 JSON 协议解析；非 2xx 与连接失败按非阻断错误处理；`headers` 支持 `$VAR` / `${VAR}` 插值，`allowedEnvVars` 列出可插值的环境变量，`allowedHttpHookUrls` 给 URL 加 glob 白名单。
- `mcp_tool`：调用已配置的 MCP server 工具；`server` + `tool` 必填，`input` 支持从 hook JSON 输入做 `${path}` 取值；输出文本等同于 command stdout，JSON 等同于 JSON 协议；MCP 未连接或 `isError: true` 视为非阻断错误；在 `SessionStart` / `Setup` 阶段 MCP 可能未连，可能失败。
- `prompt`：把 hook 输入 JSON 通过 `$ARGUMENTS` 嵌进 prompt 文本，发给指定 model（默认 fast model）；默认超时 30s。
- `agent`：类似 `prompt`，但走 agent 流程，默认超时 60s。

环境变量约定：`$CLAUDE_PROJECT_DIR`（项目根）、`${CLAUDE_PLUGIN_ROOT}` / `${CLAUDE_PLUGIN_DATA}`（plugin 上下文）、`CLAUDE_ENV_FILE`（在 `SessionStart` / `Setup` / `CwdChanged` / `FileChanged` 中可写以持久化环境变量）、`CLAUDE_CODE_REMOTE`（远程 web 环境为 `"true"`）。Hook 可选 `if` 字段把执行条件写成 permission rule 字符串（如 `Bash(git *)`），仅工具事件支持。

## Hook 事件契约一览

下面按公开 hooks 文档整理出每个事件的输入字段、是否可阻断、stdout 注入语义。所有事件共有的输入字段：`session_id`、`transcript_path`、`cwd`、`permission_mode`、`hook_event_name`，subagent 上下文还会带 `agent_id` / `agent_type`。

- `SessionStart`：matcher 取值 `startup` / `resume` / `clear` / `compact`；输入额外含 `source` 与 `model`；不能通过 exit 2 阻断会话；plain stdout 直接作为 context 注入；可写 `CLAUDE_ENV_FILE` 持久化环境变量；只支持 `command` 与 `mcp_tool` 两种 handler。
- `Setup`：matcher 取值 `init` / `maintenance`；用于 `--init-only` 或 `-p --init` / `--maintenance` 流程；不能阻断；plain stdout 仅写 debug 日志。
- `UserPromptSubmit`：无 matcher；输入额外含 `prompt`；可通过 `decision: "block"` + `reason` 阻断并擦除 prompt；可输出 `sessionTitle` 设置会话标题；plain stdout 直接作为 context 注入。
- `UserPromptExpansion`：matcher 是命令名（slash command）或 MCP server 名；输入含 `expansion_type` / `command_name` / `command_args` / `command_source`；可阻断扩展。
- `PreToolUse`：matcher 是工具名；输入含 `tool_name` / `tool_input` / `tool_use_id`；通过 `permissionDecision` (`allow` / `deny` / `ask` / `defer`) 控制；`updatedInput` 字段可在执行前改写工具参数；多 hook 优先级 `deny > defer > ask > allow`。
- `PermissionRequest`：matcher 是工具名；输入含 `tool_name` / `tool_input` / `permission_suggestions`；可输出 `decision` 决定是否允许并附带 `updatedInput` / `updatedPermissions`。
- `PermissionDenied`：通知性事件，exit code 被忽略。
- `PostToolUse` / `PostToolUseFailure`：matcher 是工具名；输入含 `tool_output` 或 `tool_error`；不能阻断（工具已执行），但 `decision: "block"` 会停止 agentic loop，`additionalContext` 进入下一轮。
- `PostToolBatch`：无 matcher；输入含 `tool_calls` 数组；`decision: "block"` 终止 agentic loop。
- `SubagentStart` / `SubagentStop`：matcher 是 agent 类型；前者不能阻断；后者可通过 `decision: "block"` 阻止结束。
- `Stop` / `StopFailure`：`Stop` 可阻断并要求继续；`StopFailure` 不能阻断，matcher 为 `rate_limit` / `authentication_failed` / `oauth_org_not_allowed` / `billing_error` / `invalid_request` / `server_error` / `max_output_tokens` / `unknown` 等错误类型。
- `Elicitation` / `ElicitationResult`：MCP server 请求 / 接收用户输入时；matcher 为 server 名；可输出 `action` (`accept` / `decline` / `cancel`) 与 `content`。
- `InstructionsLoaded`：通知性，matcher 是加载原因；输入含 `file_path` / `memory_type` / `load_reason` / `globs` / `trigger_file_path` / `parent_file_path`，是观测 `CLAUDE.md` 与 rule 加载链路的最佳手段。
- `ConfigChange`：matcher 是配置来源（`user_settings` / `project_settings` / `local_settings` / `policy_settings` / `skills`）；可阻断，但 `policy_settings` 类不可阻断。
- `CwdChanged` / `FileChanged`：通知性，可写 `CLAUDE_ENV_FILE`；`FileChanged` 的 `matcher` 是 `|` 分隔的字面文件名列表（如 `.envrc|.env`）。
- `WorktreeCreate` / `WorktreeRemove`：前者要求 stdout 输出 worktree 路径，任何非零 exit 都判失败并替代默认 git 行为；后者只通知。
- `PreCompact` / `PostCompact`：matcher `manual` / `auto`；`PreCompact` 可阻断 compaction，`PostCompact` 仅通知。
- `Notification`：通知性，matcher 是通知类型（`permission_prompt` / `idle_prompt` / `auth_success` / `elicitation_dialog` / `elicitation_complete` / `elicitation_response`）。
- `SessionEnd`：matcher 是结束原因（`clear` / `resume` / `logout` / `prompt_input_exit` / `bypass_permissions_disabled` / `other`）；不能阻断。

通用输出字段：`continue`（默认 `true`，置 `false` 让 Claude 整体停下）、`stopReason`、`suppressOutput`（屏蔽 debug 日志中的 stdout）、`systemMessage`（向用户显示警告）、`hookSpecificOutput.additionalContext`（注入上下文，`PostToolUse` / `PostToolUseFailure` / `PostToolBatch` 时与该轮工具结果并列、`SessionStart` / `Setup` / `SubagentStart` 时插入对话起始、`UserPromptSubmit` / `UserPromptExpansion` 时与提交的 prompt 并列）。中途事件的 `additionalContext` 文本会写入 transcript，会话 resume 时直接 replay 而不会重跑 hook。

## Subagent 模型

Subagent 的关键不是「多 agent 炫技」，而是上下文隔离：

- 每个 subagent 有独立 context window、独立 system prompt、独立 tool 集与权限模式。
- 文件位置 `.claude/agents/`（项目）或 `~/.claude/agents/`（用户），加上 managed scope、`--agents` CLI JSON、plugin 共五个来源；同名时优先级为 managed > CLI > project > user > plugin。
- 文件本身是 Markdown frontmatter + body prompt。frontmatter 字段（仅 `name` 与 `description` 必填）包括 `tools` / `disallowedTools` / `model` / `permissionMode` / `maxTurns` / `skills` / `mcpServers` / `hooks` / `memory` / `background` / `effort` / `isolation` / `color` / `initialPrompt`。
- `model` 可填 `sonnet` / `opus` / `haiku` / 完整 model id / `inherit`，默认 `inherit`。
- `tools` 是白名单，`disallowedTools` 是黑名单；同时存在时先减后筛。
- `permissionMode` 与父会话冲突时父优先：父 `bypassPermissions` 或 `acceptEdits` 不可被子覆盖；父 `auto` 则子 `permissionMode` 直接被忽略。
- `skills` 字段把指定 skill 的完整 body 在 subagent 启动时注入，subagent 不会继承父会话的 skill 集；不能 preload `disable-model-invocation: true` 的 skill。
- `memory: user|project|local` 给 subagent 一个 `~/.claude/agent-memory/<name>/` 之类的持久目录，其 `MEMORY.md` 同样按「前 200 行或 25KB，先到为准」注入。
- `isolation: worktree` 把工作树切到临时 git worktree，无修改时自动清理。
- 内置 subagent：`Explore`（Haiku，read-only）、`Plan`（plan mode 内部使用，read-only）、`general-purpose`（继承全部工具）。

Subagent 不能再 spawn subagent（防止递归）。Plugin subagent 不允许使用 `hooks` / `mcpServers` / `permissionMode` 字段。Subagent 在主会话当前工作目录启动；其内部 `cd` 不持久化到下一个 Bash / PowerShell 调用、也不影响主会话工作目录；如需仓库隔离副本，使用 `isolation: worktree`，subagent 无修改时该 worktree 自动清理。

Subagent 默认 system prompt 是「subagent 自身 frontmatter body + 基本环境信息」，**不包含** Claude Code 的完整 system prompt，也不包含主会话的 auto memory 与 conversation 历史。除内置 `Explore` 与 `Plan` 外，subagent 默认会加载项目 `CLAUDE.md`（计入子上下文，不是主上下文）。Subagent 在选 model 时按以下顺序解析：`CLAUDE_CODE_SUBAGENT_MODEL` 环境变量 → 调用方传入的 `model` → frontmatter `model` → 主会话 model。

Subagent 可从命令行用 `--agents` 传入 JSON 临时定义（不落盘，仅本次 session），适合测试或脚本自动化。文档明确允许的 frontmatter 字段集合除上文列出之外还包括 `description`、`prompt`（即 system prompt body）、`color`（`red` / `blue` / `green` / `yellow` / `purple` / `orange` / `pink` / `cyan`）。

## Skill 与 subagent 双向协作

公开 skills 文档说明 skill 与 subagent 的协作有两个方向：

- skill 设 `context: fork` + `agent: <type>`：skill body 作为 subagent 的 task prompt，agent 类型决定执行环境（model / tools / permissions）；`agent` 默认 `general-purpose`，可用 `Explore` / `Plan` 或自定义 subagent 名。这种用法适合「研究类 skill」，避免主上下文被探索结果污染。
- subagent frontmatter `skills:` 列出名字：subagent 启动时把这些 skill 的完整 body 注入子上下文；subagent 不会继承父会话的 skill 集；不能 preload `disable-model-invocation: true` 的 skill。

下表对比两条路径：

| 维度 | skill `context: fork` | subagent `skills:` |
|---|---|---|
| 系统提示来源 | agent 类型（`Explore` 等） | subagent 自身 markdown body |
| Task | SKILL.md 内容 | Claude 的委派消息 |
| 额外加载 | 默认加 `CLAUDE.md` | preload skills + `CLAUDE.md` |

这两条路径共享同一个底层系统，但语义不同：前者用 skill 写「任务」，后者用 subagent 定义「角色」并把 skill 当作背景知识。Mnemon 第一阶段不需要复刻这套双向机制，但理解它能避免把记忆整理 subagent 与「整理 skill」搞混。

## Subagent 隔离边界详解

公开 sub-agents 文档明确了几条「subagent 不会自动得到」的资源边界：

- 不继承父会话的 conversation 历史；
- 不继承父会话的 auto memory；
- 不继承父会话的 skills（除非在 frontmatter `skills:` 中显式 preload，或父会话用 skill 的 `context: fork` 把 skill body 作为 task prompt 发起 subagent）；
- 默认看不到父会话用过的 `--append-system-prompt` 文本；
- 内置 `Explore` 与 `Plan` 跳过 `CLAUDE.md` 加载（节省子上下文），自定义 subagent 默认会加载；
- 默认 **不能 spawn 其他 subagent**；只有 `claude --agent` 启动的主线 agent 才能用 `Agent` 工具触发其他 subagent，可用 `Agent(worker, researcher)` 语法限制可调类型。

frontmatter `mcpServers` 字段允许 inline 定义（`stdio` / `http` / `sse` / `ws`），inline server 仅在 subagent 生命周期内连接，结束后断开。这给 Mnemon 借鉴的启发：在轻量 harness 中可以让记忆整理 subagent 临时连接 SQLite 工具，而不污染主会话的工具列表。

## 启动加载顺序与 token 占用

公开 context-window 页用一个交互演示给出会话起始的代表性 token 估算（仅作示意，非保证值）：system prompt（约 4,200 tokens，不可见）→ auto memory `MEMORY.md`（首 200 行 / 25KB）→ environment info（cwd、平台、shell、OS、git 状态约 280 tokens）→ MCP 工具名（默认仅列名，schemas 按 `ENABLE_TOOL_SEARCH` 默认 deferred）→ skill 描述列表（按 1% 上下文窗口或 fallback 8,000 字符截断）→ 用户级 `~/.claude/CLAUDE.md` → 项目 `CLAUDE.md`（包含 `@path` import 展开内容）。这一启动块在 compaction 后会从磁盘整体重注入，**唯一例外是 skill 描述列表不会重注入**——只有真正被调用过的 skill body 才会重新注入并受 5,000 / 25,000 token 双重上限约束。

`/context` 命令展示的 7 类 token 占用（system / memory / env / MCP / skills / CLAUDE.md / messages）让用户可以判断主动减负的方向。文档明确建议：把仅在某些路径下需要的指令搬到 `.claude/rules/` 并加 `paths:` frontmatter，使其按需加载；把多步流程放进 skill（按调用计费而非启动注入）；把大段一次性研究放进 subagent 以避免污染主上下文。

## skills 与 commands 的合并

公开文档明确：「Custom commands 已合并入 skills」。`.claude/commands/deploy.md` 与 `.claude/skills/deploy/SKILL.md` 都生成 `/deploy`，行为等价；`commands/` 目录下的旧文件继续工作，但同名时 skill 胜出。skill 是一个目录，`SKILL.md` 是入口，可附带模板、示例、脚本（通过 `${CLAUDE_SKILL_DIR}` 引用）。skill 位置优先级 enterprise > personal > project，plugin skill 走独立 namespace。

skill frontmatter 关键字段：`name`（默认取目录名，最多 64 字符，限小写字母、数字、连字符）、`description`（推荐填写，与 `when_to_use` 合计 1,536 字符上限）、`allowed-tools`、`disable-model-invocation`、`user-invocable`、`model`、`effort`、`context: fork` / `agent`、`paths`、`hooks`、`shell`、`arguments`。占位符包括 `$ARGUMENTS`、`$ARGUMENTS[N]` / `$N`、`$<named>`、`${CLAUDE_SESSION_ID}`、`${CLAUDE_EFFORT}`、`${CLAUDE_SKILL_DIR}`。``!`cmd``` 内联或 ```` ```! ```` 块会在 skill 内容送给模型前先执行，结果替换原文。

skill 列表（Claude 看到的「有哪些 skill 可调用」）按上下文窗口的 1% 动态字符预算（fallback 8,000 字符）截断。每个 skill 的 `description` + `when_to_use` 合计上限 1,536 字符。`SLASH_COMMAND_TOOL_CHAR_BUDGET` 环境变量可上调预算；`skillOverrides` 设置可把单个 skill 标为 `"on"` / `"name-only"` / `"user-invocable-only"` / `"off"` 来节省预算（在 `/skills` 菜单按 `Space` 切换、`Enter` 保存到 `.claude/settings.local.json`）。skill 触发条件：`disable-model-invocation: true` 时不进入 skill 索引，零 token 直到用户 `/name` 显式调用；`user-invocable: false` 时不出现在 `/` 菜单，但仍然在 skill 索引中供 Claude 自动调用。

## CLAUDE.md / settings 装载的可观察行为

公开文档明确以下行为可被用户复现：

- 运行 `/memory` 列出当前会话所有已加载的 `CLAUDE.md` / `CLAUDE.local.md` / rules，并提供 auto memory 开关与文件夹快捷打开。
- 运行 `/context` 看 token 占用按类别分解。
- 运行 `/status` 看每个 settings key 的有效来源（remote managed、plist、HKLM、文件等）。
- 启用 `InstructionsLoaded` hook，可记录每个指令文件何时、为何被加载（matcher 取值揭示 `session_start` / `nested_traversal` / `path_glob_match` / `include` / `compact` 五种触发原因）。
- 设 `CLAUDE_CODE_ADDITIONAL_DIRECTORIES_CLAUDE_MD=1` 让 `--add-dir` 添加的目录也加载 `CLAUDE.md` / `.claude/rules/` / `CLAUDE.local.md`，否则 `--add-dir` 仅授予文件访问权而不加载配置。
- `claudeMdExcludes` 数组（可放任意 scope，managed 也参与合并）按绝对路径 glob 跳过特定 `CLAUDE.md`，但 managed 路径下的 `CLAUDE.md` 不可被排除。

## 适合 Mnemon 参考的部分

- 使用 `CLAUDE.md` / imports 承载稳定指令，且控制单文件在 200 行以内；与 Mnemon 的 `GUIDELINE.md` 短而稳定的方向一致。
- 使用 settings hooks 在生命周期点注入短提醒；Mnemon 的「session 起始 / prompt 提交 / tool 之后 / stop 之前」与 Claude Code 的事件名一一对应，hook 输出严格走 `additionalContext` 形态、控制在 10K 字符内。建议 Mnemon hook 输出 ≤ 1KB，避免逼近上限。
- 使用 skills/commands 表达可复用工作流；Mnemon 的 `SKILL.md` 可借鉴 frontmatter + body + 占位符的形态，并区分 `disable-model-invocation` 与 `user-invocable` 两类语义。
- 使用 subagents 隔离大规模探索或长上下文记忆整理；Mnemon 的 memory writeback review 可委派给 subagent，但不应作为架构必需。
- 借鉴 auto memory 的「按 git repo 隔离 + 容量上限注入 + 索引文件 + topic 文件按需读取」模式，避免无限增长的单文件 memory。Mnemon 的 SQLite 表已经天然按 fact 拆分，但「索引 markdown + 全量数据库」的双层观感对人类 review 仍有价值。
- 借鉴 settings 的 4-scope（managed / project / user / local）+ 数组合并策略，让 Mnemon 的 GUIDELINE 与 SKILL 也按 scope 拼接而非覆盖。

## 不应照搬的部分

- 不应把 Mnemon 设计成 Claude Code 专属 adapter；Claude Code 的 hook 触发链、模型路由、worktree 隔离均依赖自身 runtime，本地 CLI agent 无法复刻。
- 不应依赖 Claude Code 的未公开内部行为；公开文档之外的字段或顺序假设都需要写明「社区观察」。
- 不应把 hook 写成强制每轮 recall/writeback 的控制器；exit code 2 阻断、`continue: false` 终止、bypass 权限提升等能力如果误用会让 agent 不可控。
- 不应假设 path-scoped rule 与 nested `CLAUDE.md` 在 `/compact` 后仍然在线，详见生命周期文档。
- 不应在 Mnemon 中模仿 `Skill(name)` 的 permission 规则、`disableSkillShellExecution`、`allowManagedHooksOnly` 一类企业策略字段，这些是 Claude Code runtime 的安全模型而非通用 memory 模式。

## Sandbox、permissions 与安全模型

公开 settings 文档展示 Claude Code 把安全控制写在 settings 中而不是 hook 里：

- `permissions.allow / deny / ask` 用规则字符串描述工具调用，例如 `Bash(npm run lint)`、`Read(./.env)`、`Bash(git push *)`；规则跨 scope 拼接，project deny 优先于 user allow。
- `permissions.defaultMode`：`default` / `acceptEdits` / `plan` / `auto` / `dontAsk` / `bypassPermissions`。
- `permissions.additionalDirectories`：扩展 Claude 可访问的目录范围，但 `--add-dir` 不会自动加载该目录的 settings 与 subagent 定义（除 skills 外）。
- `sandbox.enabled` 启用 sandbox 后，`sandbox.filesystem.allowWrite / denyWrite / allowRead / denyRead` 控制磁盘访问，`sandbox.network.allowedDomains / deniedDomains` 控制网络出站，`sandbox.network.allowUnixSockets` 允许具体的 Unix socket（如 `~/.ssh/agent-socket`）。
- `disableAllHooks: true` 一刀切关闭 hook；`allowManagedHooksOnly: true` 仅放行 managed 与显式 plugin hook。

这部分对 Mnemon 的意义是：Mnemon 不应试图重做权限系统，应让 hook 发出建议性 context，由宿主 runtime 自己执行真正的拦截。

## 与 Mnemon 当前设计的对照

Mnemon 第一阶段使用 SQLite 存事实、Markdown 存指引（`SKILL.md` / `INSTALL.md` / `GUIDELINE.md`）、shell 命令注入 hook。把 Claude Code 的机制按这一拆分映射：

| Mnemon 资产 | Claude Code 对应 | 映射说明 |
|---|---|---|
| `GUIDELINE.md` | 项目 `CLAUDE.md` + `.claude/rules/`（无 `paths`） | 都是稳定行为总纲，启动时常驻；建议 ≤200 行 |
| `INSTALL.md` | `/init` 流程 + managed CLAUDE.md 场景下的安装说明 | 安装/接入文档，不进入主 prompt |
| `SKILL.md` | `~/.claude/skills/<name>/SKILL.md` | 同样按需加载，可附支持文件 |
| Mnemon hook 注入点 | `SessionStart` / `UserPromptSubmit` / `PostToolUse` / `Stop` / `PreCompact` | 注入文本走 `additionalContext`，控制 ≤1KB |
| Mnemon 数据库内的 fact | Claude Code auto memory `MEMORY.md` 索引 + topic 文件 | 借鉴「索引 + 详情拆分」与「容量上限注入」 |
| Mnemon CLI 命令（`remember` / `recall` / `link`） | Claude Code skill body 中的 ``!`mnemon …``` | 通过 dynamic shell injection 把当前事实灌入 prompt |

## 参考来源

- 官方文档: [Claude Code memory](https://code.claude.com/docs/en/memory)
- 官方文档: [Claude Code settings](https://code.claude.com/docs/en/settings)
- 官方文档: [Claude Code hooks](https://code.claude.com/docs/en/hooks)
- 官方文档: [Claude Code subagents](https://code.claude.com/docs/en/sub-agents)
- 官方文档: [Claude Code skills / slash commands](https://code.claude.com/docs/en/slash-commands)
- 官方文档: [Claude Code context window](https://code.claude.com/docs/en/context-window)
- 官方文档: [Claude Code scheduled tasks](https://code.claude.com/docs/en/scheduled-tasks)
