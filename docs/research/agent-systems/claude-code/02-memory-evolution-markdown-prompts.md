# Claude Code 的记忆、Markdown 与 Prompt 用法

> 边界：本文件不使用泄漏源码，只基于公开官方文档和公开社区讨论。所有字段名和数字引自 `code.claude.com/docs/en/*`。

## 记忆处理方案

Claude Code 的公开 memory 设计重点不是单一外部数据库，而是多种 Markdown 上下文机制 + 一个 agent 自维护的 auto memory：

- `CLAUDE.md`：项目/用户/本地/managed 四个 scope 的指令入口，全部在启动时拼接进上下文。
- `@path` imports：把长指令拆成多个文件，递归 import 最大深度 5 跳，相对路径以宿主文件为基准。
- `.claude/rules/`：更结构化的项目规则，每个 `.md` 一个主题，可加 `paths:` frontmatter 做路径作用域。
- Auto memory：`~/.claude/projects/<project>/memory/MEMORY.md` 由 Claude 自己写入，每次会话注入「前 200 行或 25KB，先到为准」，topic 文件 `debugging.md` 等按需读取。
- settings hooks：在 `SessionStart`、`UserPromptSubmit`、`PreToolUse`、`PostToolUse`、`Stop` / `SubagentStop`、`PreCompact` / `PostCompact`、`InstructionsLoaded`、`CwdChanged` 等阶段注入提醒或修改决策。
- Subagents：把复杂任务放进独立 context window，可选 `memory: user|project|local` 给 subagent 自己的持久目录。
- Skills（合并了原 commands）：把可复用流程写成 Markdown 目录，按需加载 body，可附支持文件、脚本、模板。

Claude Code 的实际「记忆」更像文件化操作系统上下文，而不是单一 memory store。用户和团队把稳定信息写入 `CLAUDE.md` / rules / skills，agent 把自己学到的内容写入 auto memory。

## Markdown 文件用法

| Markdown 资产 | 用途 | 文件位置示例 | 对 Mnemon 的启发 |
|---|---|---|---|
| 项目 `CLAUDE.md` | 团队共享指令、构建命令、约定 | `./CLAUDE.md` 或 `./.claude/CLAUDE.md` | Mnemon `GUIDELINE.md` 同样属于稳定行为总纲 |
| 用户 `CLAUDE.md` | 个人偏好（跨项目） | `~/.claude/CLAUDE.md` | Mnemon 用户级 guideline 可以同位置 |
| 本地 `CLAUDE.local.md` | 不入版本库的个人项目偏好 | `./CLAUDE.local.md`，应 gitignore | Mnemon 本地偏好同样应排除版本库 |
| Managed `CLAUDE.md` | 组织强制注入的策略 | macOS `/Library/Application Support/ClaudeCode/CLAUDE.md` 等 | Mnemon 第一阶段不需要 managed scope |
| `.claude/rules/*.md` | 模块化规则，可路径作用域 | 项目内 | Mnemon 可考虑按 path 拆分 guideline |
| Auto memory `MEMORY.md` + topic 文件 | agent 自写的学习记录 | `~/.claude/projects/<proj>/memory/` | Mnemon 用 SQLite 存事实，可借鉴「索引 + topic」的拆分思路 |
| `.claude/agents/*.md` | subagent 定义 | 项目或用户级 | 记忆整理可选 subagent，但非必需 |
| `.claude/skills/<slug>/SKILL.md` | 可执行流程说明 | 项目或用户级 | Mnemon `SKILL.md` 应教命令，流程进入 skill |
| `.claude/commands/*.md`（旧路径） | 与 skill 等价 | 项目或用户级 | 与 skill 同名时 skill 优先 |

## 特殊 prompt 形态

Claude Code 的 prompt 资产共享几种形态：

1. **YAML frontmatter + Markdown body**。subagents 与 skills 都采用同一形态，frontmatter 描述身份、工具、模型、可见性、加载条件，body 是执行指令。
2. **Skill frontmatter 字段**：`name`（默认取目录名，最多 64 字符，限小写字母/数字/连字符）、`description`（与 `when_to_use` 合计上限 1,536 字符）、`allowed-tools`、`disable-model-invocation`（默认 `false`，设 `true` 后只能由用户显式调用）、`user-invocable`（默认 `true`，设 `false` 隐藏出 `/` 菜单）、`model`、`effort`、`context: fork`、`agent`、`paths`、`hooks`、`shell`（`bash` 默认或 `powershell`）、`arguments`。占位符 `$ARGUMENTS` / `$N` / `${CLAUDE_SESSION_ID}` / `${CLAUDE_SKILL_DIR}` 让 skill 既能接收参数也能定位自身目录。
3. **Subagent frontmatter 字段**：仅 `name` 与 `description` 必填；常用字段 `tools` / `disallowedTools` / `model` / `permissionMode` / `maxTurns` / `skills` / `mcpServers` / `hooks` / `memory` / `background` / `effort` / `isolation` / `color` / `initialPrompt`。subagent 默认 `model: inherit`。
4. **hook additional context**：hook 不一定产生聊天消息，而是把 `hookSpecificOutput.additionalContext` 注入为系统提醒；plain stdout 在部分事件下也会注入（`SessionStart`、`UserPromptSubmit`），但在 `PostToolUse` 等事件下仅写 debug 日志。注入文本上限 10,000 字符。
5. **dynamic context injection**：skill body 中 ``!`cmd``` 与 ```` ```! ```` 在送给模型前先在本地 shell 执行，结果替换占位符，可被 settings 的 `disableSkillShellExecution` 关闭。

这说明 Mnemon 的 hook 输出应短小、上下文型、可忽略，而不是长 prompt 或强制命令；建议每个 hook 输出 ≤ 1KB 文本，结构化字段对齐 Claude Code 的 `additionalContext`。

## /memory 与 /context 暴露的运行时视图

公开 memory 与 context-window 文档明确两个对调试至关重要的命令：

- `/memory`：列出当前会话已加载的所有 `CLAUDE.md` / `CLAUDE.local.md` / rule 文件，提供 auto memory 开关与文件夹打开入口；选中任意文件可直接在编辑器打开。如果某个 `CLAUDE.md` 不在列表中，Claude 看不到它。
- `/context`：以代表性 token 数展示按类别（system / memory / env / MCP / skills / CLAUDE.md / messages）的占用，并给出优化建议。
- `/status`：列出每个 settings key 的有效来源（remote managed、plist、HKLM、文件等），帮助定位「为什么我的设置没生效」。
- `/init`：生成 `CLAUDE.md` 起始版本；若已存在则建议改进而非覆盖；`CLAUDE_CODE_NEW_INIT=1` 启用多阶段交互流程，agent 用 subagent 探索仓库后呈现可 review 的 proposal 再写入。

这些可观察接口是 Mnemon 借鉴的关键：Mnemon 应该提供等价的 `mnemon memory show` / `mnemon hooks show` / `mnemon settings show` 命令，让用户随时审查注入栈，而不是靠盲信 hook。

## 智能体演化方案

Claude Code 的公开机制支持演化，但主要是人工 / agent 协作修改 Markdown 资产 + agent 自写 auto memory：

- `/init` 或 `CLAUDE_CODE_NEW_INIT=1` 多阶段 init 生成初始 `CLAUDE.md`、skills、hooks 草案；
- `/memory` 浏览/编辑当前会话加载的 `CLAUDE.md` / rules / auto memory 文件，并切换 auto memory 开关；
- 用户对 Claude 说「always use pnpm」一类话，Claude 会写入 auto memory；用户说「add this to CLAUDE.md」则写入项目指令；
- 创建/更新 skills、subagents 是通过编辑 Markdown 完成；`/agents` 提供向导；
- hooks 做安全、日志、验证或上下文注入，但不会自动改写 Markdown；
- 社区实践常把「学到的流程」写回命令、skills 或项目规则。

它不是自动重写 runtime 的系统。即使 auto memory 自动写入，也仅仅是 plain Markdown 文件，用户可随时 `/memory` 查看或删除。演化边界仍是可审查的文件变更。

## skills/commands 文件结构

skill 是一个目录，`SKILL.md` 是入口，可包含支持文件：

```
my-skill/
├── SKILL.md           # 入口，包含 frontmatter + body
├── reference.md       # 详细参考，按需读
├── examples/
│   └── sample.md
└── scripts/
    └── helper.py      # 通过 ${CLAUDE_SKILL_DIR}/scripts/helper.py 引用
```

slug 直接来自目录名，限小写字母/数字/连字符，最多 64 字符。`disable-model-invocation: true` 让 skill 只能由用户显式调用，启动时不在 skill 索引中出现，零 token 成本直到被调用。文档提示 `SKILL.md` 控制在 500 行以下，详细参考写到独立文件。

`.claude/commands/*.md` 仍可使用，与 skill 等价；同名时 skill 优先。

## subagent 隔离边界

subagent 启动时的上下文与父会话隔离：

- 独立 context window，独立 system prompt；
- 不继承父会话历史与 auto memory；
- 默认会加载 `CLAUDE.md`（内置 `Explore` / `Plan` 跳过以节省上下文）；
- 不继承父的 skill 集，需要在 frontmatter `skills:` 显式 preload 完整 body；
- 工具默认全继承，可 `tools` 白名单或 `disallowedTools` 黑名单缩减；
- 默认 **不能再 spawn subagent**，防止递归；
- `permissionMode` 与父冲突时父优先（详见 01 文档）；
- `memory:` scope 决定 agent memory 目录在 `~/.claude/agent-memory/<name>/`、`.claude/agent-memory/<name>/` 或 `.claude/agent-memory-local/<name>/`，启用后 Read/Write/Edit 工具自动开启。

## 社区实践信号

公开社区讨论中常见共识：

- 主 `CLAUDE.md` 应短而稳定（社区与官方建议都指向 ≤200 行）；
- 长流程应拆成 skills/commands；
- subagent 用于上下文隔离，特别是 codebase 探索；
- hooks 适合安全检查、决策捕获、session 总结、持久规则提醒；
- 单纯把所有东西塞进主指令会浪费 context 并降低可维护性。

这些信号支持 Mnemon 当前方案：把能力、安装和判断分别放入 `SKILL.md`、`INSTALL.md`、`GUIDELINE.md`。

## 失败与拒绝场景

来自官方 hooks/skills/sub-agents 文档的明确行为：

- hook 超时（默认 command 600s / HTTP 30s / prompt 30s / agent 60s）按非阻断错误处理，stderr 第一行进 transcript，会话继续。
- hook 注入 context 超 10,000 字符时，超出部分写到文件，模型只看到预览 + 路径。
- HTTP hook 非 2xx 响应或连接失败：非阻断错误，会话继续。
- `disableSkillShellExecution: true` 时，所有 skill 与 custom command 来源（user / project / plugin / additional-directory）的 `` !`cmd` `` 与 ```` ```! ```` 块会被替换为 `[shell command execution disabled by policy]`。bundled / managed skill 不受影响。
- `permissions.deny` 中加 `Skill(name)` 或 `Skill(name *)` 可阻断特定 skill；加 `Skill` 直接禁用所有 skill。
- subagent `permissionMode: bypassPermissions` 仍受 root/家目录删除断路器约束；`rm -rf /` 一类命令仍会提示。
- plugin subagent 中的 `hooks` / `mcpServers` / `permissionMode` 字段被忽略（出于安全）。

## Auto memory 的写入闭环

公开 memory 页给出 auto memory 的完整闭环：

- `autoMemoryEnabled` 默认 `true`（v2.1.59+）；`/memory` 内可切换；`CLAUDE_CODE_DISABLE_AUTO_MEMORY=1` 也可禁用。
- 存储位置由 git 仓库决定：`~/.claude/projects/<project>/memory/`，所有 worktree 与子目录共享同一目录；非 git 仓库以根目录为 project 标识。
- `autoMemoryDirectory` 重定向位置时只接受 managed / user 设置或 `--settings`，project / local 不接受（防止恶意 clone 把 memory 写到敏感位置）。
- 文件结构：入口 `MEMORY.md` + 任意数量 topic 文件；Claude 写入时会在 UI 显示 "Writing memory" 或 "Recalled memory" 提示；用户可随时 Read / Edit / 删除。
- 注入策略：会话起始注入 `MEMORY.md` 前 200 行 / 25KB（先到为准）；topic 文件不在启动时加载，按需用 Read 工具读取。
- 与 `CLAUDE.md` 的边界：用户对 Claude 说「always use pnpm」一类话进入 auto memory；说「add this to CLAUDE.md」则 Claude 改写 `CLAUDE.md`；两者都是 plain Markdown，可互相替代但语义不同。
- 文档明确：「Claude 不是每次会话都会写入 auto memory，它会判断是否值得记录」。

这套闭环让 Mnemon 借鉴时分两层：人写的稳定指令进 `GUIDELINE.md`（类比 `CLAUDE.md`），agent 自写的学习进 SQLite（类比 auto memory），并对外提供 `mnemon memory show` 之类命令做 `/memory` 等价的 review 能力。

## CLAUDE.md / settings 装载次序

理解装载次序对 Mnemon 设计 INSTALL 与 GUIDELINE 直接相关。公开文档给出的精确规则：

settings 优先级（高 → 低）：

1. Managed settings：macOS `/Library/Application Support/ClaudeCode/managed-settings.json`、Linux/WSL `/etc/claude-code/managed-settings.json`、Windows `C:\Program Files\ClaudeCode\managed-settings.json`，外加 `managed-settings.d/` 目录与 Windows 注册表 `HKLM\SOFTWARE\Policies\ClaudeCode`；
2. 命令行 `--settings` 标志；
3. `.claude/settings.local.json`（本机本仓库）；
4. `.claude/settings.json`（项目共享）；
5. `~/.claude/settings.json`（用户全局）。

数组类（`permissions.allow / deny / ask`、`sandbox.filesystem.allowWrite` 等）跨 scope 拼接 + 去重；标量类按上述顺序取首个非空值。`autoMemoryDirectory` 仅 managed / user 设置或 `--settings` 接受，project / local 不接受（防止克隆仓库劫持）。

CLAUDE.md 装载：

- 从工作目录沿目录树向上遍历，所有命中文件 **拼接**进上下文；root 方向靠前，工作目录靠后；同目录 `CLAUDE.local.md` 排在 `CLAUDE.md` 之后。
- 子目录的 `CLAUDE.md` 与 `CLAUDE.local.md` 不在启动时加载，等 Claude 读取该子目录文件时再注入到 message history。
- managed CLAUDE.md 始终被加载且不可被 `claudeMdExcludes` 排除；用户的排除规则只能跳过非 managed 文件。
- `@path` import 在 host 文件位置原地展开；相对路径以宿主文件为基准；递归 import 最大深度 5 跳；首次见到外部 import 弹出审批，拒绝后该 import 永久禁用。

## 风险

- Markdown 过多会造成发现困难；建议 `description` / `when_to_use` 关键字写在前面，因为公开文档说 skill 列表会按 1% context window（fallback 8,000 字符）的预算截断。
- hooks 过强会变成隐式控制器；exit code 2、`continue: false`、`bypassPermissions` 等能力如果误用会破坏可控性。
- subagent 太多会增加延迟和调试成本；不能 spawn 嵌套 subagent，但每多一层都额外加载一份 `CLAUDE.md`。
- 旧文件指令可能覆盖当前事实，需要明确 stale memory 处理规则；auto memory 是 plain Markdown 而非黑盒，可随时 `/memory` 审查。

## Hook 输出契约的 Markdown 视角

虽然 hook 是代码执行而不是文件资产，它注入到上下文的内容仍然是 Markdown 风格的文本。理解每个事件能注入什么、是否阻断，对 Mnemon 设计 hook 文本生成策略很关键：

- `SessionStart` 与 `Setup` 的 `additionalContext` 插入到对话起始；可以用来告知 agent「以下事实由 Mnemon 注入」。
- `UserPromptSubmit` 与 `UserPromptExpansion` 的 `additionalContext` 插入到提交的 prompt 旁边；适合做「相关记忆推送」。
- `PreToolUse` / `PostToolUse` / `PostToolUseFailure` / `PostToolBatch` 的 `additionalContext` 与该轮工具结果并列；适合做「该工具刚刚发现了一个事实，建议记下来」。
- `Stop` / `SubagentStop` 没有结构化注入位（这两个事件只控制是否结束），需要靠 `decision: "block"` + `reason` 让 agent 继续，效果上是再多说一段话。
- `PreCompact` 没有注入位，但可阻断 compaction；`SessionStart` 在 compaction 后会以 `source: "compact"` matcher 再次触发，是「compaction 后重新注入提醒」的最佳 hook 点。

这套契约对 Mnemon 的 4 个 hook 阶段（session start / user prompt submit / post tool / pre stop）几乎一一对应。Mnemon 在跨 runtime 设计时可以把 Claude Code 的字段视作目标抽象，再为 Codex / Hermes 等其他 runtime 做映射。

## 何时用哪种 Markdown 资产

公开文档对资产选择给出清晰的决策（基于 memory / skills / hooks / sub-agents 页面交叉引用）：

- 若是「每次会话都需要的事实」，写入 `CLAUDE.md`；超过 200 行考虑拆分到 `.claude/rules/` 或 imports。
- 若仅在某些路径下需要，写入 `.claude/rules/` 并加 `paths:` frontmatter；该 rule 只在读取匹配文件时进入 message history。
- 若是「多步流程或 checklist」，写入 skill；body 仅在调用时加载，按调用计费。
- 若是「Claude 自己学到的偏好」，让其写入 auto memory（`MEMORY.md` + topic 文件）；用户随时可 `/memory` 审查或编辑。
- 若是「必须在某个 lifecycle 时刻发生的动作」（如 commit 前格式化、prompt 提交时注入分支信息），写为 hook，而不是放在 `CLAUDE.md` 里。
- 若是「会污染主上下文的大段探索」，委派给 subagent；只把摘要带回主会话。
- 若是「需要在 session 结束后仍然继续的工作」，使用 cloud routines / desktop scheduled tasks / GitHub Actions，而不是 session-scoped 的 `/loop`。

## Skill body 与 dynamic shell injection

Skill 内容支持两种动态注入语法：

- 内联 ``!`cmd``` ：在送给模型之前先执行 `cmd`，结果文本替换原占位符；
- 块级 ```` ```! ```` ：多行 shell 块，整体执行，stdout 替换块。

执行 shell 之前 settings 的 `disableSkillShellExecution: true` 可以禁掉所有 user / project / plugin / additional-directory 来源 skill 的 shell 注入；bundled / managed skill 不受影响。这一字段最适合放在 managed scope 防被本地覆盖。`shell` frontmatter 字段（`bash` 默认或 `powershell`）控制使用的 shell；`powershell` 需要 `CLAUDE_CODE_USE_POWERSHELL_TOOL=1`。

字符串占位符可分为三组：

- 用户参数：`$ARGUMENTS`（全部参数原文）、`$ARGUMENTS[N]` / `$N`（按位置）、`$<named>`（按 frontmatter `arguments` 命名映射）；
- session 元数据：`${CLAUDE_SESSION_ID}`、`${CLAUDE_EFFORT}`；
- 资源定位：`${CLAUDE_SKILL_DIR}`（指向当前 skill 的 `SKILL.md` 所在目录，可在 bash 注入中跨平台引用脚本）。

Mnemon 借鉴这套机制时可以让 SKILL 中通过 ``!`mnemon recall …``` 把当前事实灌入 prompt，避免 hook 与 skill 重复维护事实拉取逻辑。

## 对 Mnemon 的具体启发

- Mnemon 的 SKILL.md 应同时定义「Claude 自动调用的入口（默认）」和「用户显式调用的高风险流程」（对应 `disable-model-invocation: true`），以避免误触。
- Mnemon 的 hook 输出应严格使用「短上下文 + 结构化字段」，而不是长 prompt；目标 ≤1KB，绝不接近 Claude Code 10,000 字符上限。
- Mnemon 不需要复刻 Claude Code 的 `permissions.deny` 体系，但可借鉴「数组合并 + 高 scope 胜出」的 settings 模型，让组织级 / 项目级 / 用户级偏好按 scope 拼接。
- Mnemon 的「fact + topic 拆分」应遵循 `MEMORY.md` 索引模式：索引文件保持简短常驻，详细笔记按主题落到独立文件，需要时再读。
- Mnemon 的 hook 不应假设 Claude Code 的注入字段（`additionalContext`、`permissionDecision` 等）在其他 runtime 上存在；这些是 Claude Code 专属契约，跨 runtime 时需要写入纯文本回退。

## 参考来源

- 官方文档: [Memory](https://code.claude.com/docs/en/memory)
- 官方文档: [Hooks](https://code.claude.com/docs/en/hooks)
- 官方文档: [Subagents](https://code.claude.com/docs/en/sub-agents)
- 官方文档: [Skills / custom commands](https://code.claude.com/docs/en/slash-commands)
- 官方文档: [Settings](https://code.claude.com/docs/en/settings)
- 官方文档: [Context window](https://code.claude.com/docs/en/context-window)
- 社区讨论样例: [Claude Code build system discussion](https://www.reddit.com/r/ClaudeCode/comments/1swcwb6/claude_code_is_a_build_system_not_a_chatbot_13/)
