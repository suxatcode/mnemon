# Claude Code 架构观察

> 边界：本文件不使用泄漏源码，只基于公开官方文档、公开社区讨论和可观察行为。

## 一句话结论

Claude Code 的整体形态是「agent runtime + Markdown 行为资产 + settings/hooks 扩展点 + subagent 隔离执行」。它并不要求项目为长期记忆实现复杂 adapter，而是把大部分行为表达在 `CLAUDE.md`、skills、commands、subagents 和 settings hooks 中。

## 公开架构面

Claude Code 公开文档体现出四个层次：

| 层 | 公开机制 | 作用 |
|---|---|---|
| 持久项目上下文 | `CLAUDE.md`、imports、rules | 给主 agent 注入项目规范、偏好、工作流 |
| 运行时配置 | `settings.json`、managed settings、local settings | 权限、hooks、插件、scope 和安全策略 |
| 扩展动作 | skills、slash/custom commands | 把可复用操作和流程写成 Markdown |
| 隔离执行 | subagents、agent teams | 把探索、评审、测试等任务移出主上下文 |

官方 settings 文档把配置分为 managed、user、project、local scopes，并明确 `.claude/settings.json`、`.claude/settings.local.json`、`~/.claude/settings.json` 等位置。官方 subagents 文档说明 subagent 是 Markdown + YAML frontmatter 定义的专用 agent，有自己的 context window、system prompt、工具权限和模型选择。

## 指令装载模型

Claude Code 使用 `CLAUDE.md` 作为主要项目记忆/指令入口。公开 memory 文档说明：

- Claude Code 读取 `CLAUDE.md`，不是 `AGENTS.md`；
- 如果仓库已有 `AGENTS.md`，可以在 `CLAUDE.md` 中用 `@AGENTS.md` import；
- imports 可以组织个人偏好、项目指令等；
- settings 文档列出 user/project/local scope 中 `CLAUDE.md` 的位置。

这说明 Claude Code 的 memory 不只是「向量库」问题，而是一个文件化上下文系统。稳定规则进入 `CLAUDE.md` 或 rules；重复流程进入 skills/commands；探索性任务进入 subagents。

## Hook 模型

Claude Code hooks 是生命周期扩展点，而不是完整 workflow engine。官方 hooks 文档展示了：

- `SessionStart` 可以向 Claude 添加启动上下文；
- `UserPromptSubmit` 可以添加上下文或阻止 prompt；
- `PreToolUse` 可以在工具执行前拦截；
- `PostToolUse` 在工具执行后反馈；
- `Stop` / `SubagentStop` 可以阻止停止并要求继续；
- `PreCompact` 可以阻止或处理 compaction。

重要设计点：大多数事件下 exit code `2` 才表示阻断；stdout 是否注入上下文取决于事件。hook output 有长度限制，并且文档强调输入校验、绝对路径、跳过敏感文件等安全规则。

## Subagent 模型

Subagent 的关键不是「多 agent 炫技」，而是上下文隔离：

- 探索型任务不会污染主上下文；
- 子 agent 有独立 prompt 与工具权限；
- 项目级 `.claude/agents/` 可提交到仓库；
- 用户级 `~/.claude/agents/` 可跨项目复用；
- subagent 文件本身是 Markdown frontmatter + body prompt。

这对 Mnemon 的启发是：memory writeback review 可以由 subagent 执行，但不应成为架构必需。轻量 harness 应允许主 agent 直接做判断，也允许 runtime 有能力时委派。

## 适合 Mnemon 参考的部分

- 使用 `CLAUDE.md` / imports 承载稳定指令。
- 使用 settings hooks 在生命周期点注入短提醒。
- 使用 skills/commands 表达可复用工作流。
- 使用 subagents 隔离大规模探索或长上下文记忆整理。

## 不应照搬的部分

- 不应把 Mnemon 设计成 Claude Code 专属 adapter。
- 不应依赖 Claude Code 的未公开内部行为。
- 不应把 hook 写成强制每轮 recall/writeback 的控制器。

## 参考来源

- 官方文档: [Claude Code memory](https://code.claude.com/docs/en/memory)
- 官方文档: [Claude Code settings](https://code.claude.com/docs/en/settings)
- 官方文档: [Claude Code hooks](https://code.claude.com/docs/en/hooks)
- 官方文档: [Claude Code subagents](https://code.claude.com/docs/en/sub-agents)
