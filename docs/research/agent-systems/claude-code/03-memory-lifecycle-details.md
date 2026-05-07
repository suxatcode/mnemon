# Claude Code memory lifecycle 细节

> 边界：本页只基于 Claude Code 官方公开文档与公开可见行为，不使用泄漏源码或非公开实现细节。

## 核心判断

Claude Code 的 memory 设计是「启动时加载 Markdown 指令/记忆 + 长会话时 compaction + session scoped 自动化」。它没有把 memory 做成独立数据库运行时，而是让 `CLAUDE.md`、project rules、skills、hooks 和 scheduled tasks 共同构成行为层。

这对 Mnemon 的意义是：第一阶段可以把安装说明、行为 guideline 和 hook 阶段写成 Markdown，让 agent 按文档为自己安装，而不必先做复杂 adapter。

## 生命周期详表

| 维度 | 观察 |
|---|---|
| 主要记忆载体 | `CLAUDE.md`、`.claude/CLAUDE.md`、用户级 `~/.claude/CLAUDE.md`、本地 `CLAUDE.local.md`、project rules、skills。 |
| 存储位置 | 组织级、项目级、用户级、本地级都有对应位置；项目级可随仓库提交，本地级应加入 `.gitignore`。 |
| 加载时机 | 启动时沿目录层级加载 root 与父目录指令；子目录 `CLAUDE.md`/rules 在读取匹配文件时按需加载。 |
| 读路径 | Claude 把已加载的 Markdown 放入当前上下文；`/memory` 可检查加载了哪些 memory 文件；`/context` 可查看上下文占用。 |
| 写路径 | 人类直接编辑、`/init` 初始化、`/memory` 管理、对 Claude 使用 `#` 快捷保存记忆，或通过 hooks/commands 引导生成候选修改。 |
| 长度限制 | 官方文档未给出 `CLAUDE.md` 字符硬上限；实际受模型上下文、启动加载成本和 compaction 压力约束。 |
| skill 限制 | compaction 后已调用 skill bodies 会重新注入，但每个 skill body capped at 5,000 tokens，总量 capped at 25,000 tokens，旧的先丢。 |
| import 限制 | `@path` import 用于拆分文件；公开 memory 文档中说明 import 有深度限制，应避免多层链式依赖。 |
| 超出处理 | 长会话通过 `/compact` 或自动 compaction 把历史替换成摘要；root 指令与 auto memory 从磁盘重新注入，路径触发的规则要等再次读取匹配文件才回来。 |
| 整理方式 | 主要依赖人工或 agent 按文档重写 Markdown；官方强调把最重要内容放前面、保持具体、用标题组织。 |
| 定时任务 | Claude Code 支持 `/loop` 与 cron scheduling tools，任务可按间隔重跑 prompt；这些是通用自动化，不是专门的 memory consolidation scheduler。 |
| 持久性 | `/loop` 任务是 session-scoped；新 conversation 会清掉，resume 只恢复未过期任务。Cloud routines / Desktop tasks / GitHub Actions 才适合跨 session 自动化。 |
| 安全边界 | 组织/项目/用户/本地 scope 分层；本地文件不应提交；外部 import 首次会审批；hooks 可在关键事件插入检查。 |

## 写入与整理机制

Claude Code 的写入路径偏 Markdown-native：

1. `CLAUDE.md` 保存项目架构、测试命令、代码风格、工作流、常见坑。
2. 用户级 `~/.claude/CLAUDE.md` 保存个人偏好。
3. 本地 `CLAUDE.local.md` 保存不该提交的个人/环境信息。
4. 大型项目用 imports 或 rules 拆分主题和路径作用域。
5. 成熟流程放入 skills 或 slash commands，而不是不断追加到主 memory。

这说明 memory 文件不是无限增长的日志。好的做法是把条目整理成稳定政策、短流程、命令索引和路径规则。

## 超出与 compaction 行为

Claude Code 的上下文页明确区分哪些机制会在 compaction 后幸存：

- system prompt 和 output style 不属于普通消息历史，保持不变。
- project-root `CLAUDE.md` 和 unscoped rules 会从磁盘重新注入。
- auto memory 会从磁盘重新注入。
- path-scoped rules 和 nested `CLAUDE.md` 会被总结掉，直到再次读取匹配路径才重新加载。
- 已调用 skill bodies 会重新注入，但有 per-skill 和总 token cap。
- hooks 是代码执行，不是上下文内容，不适用 compaction。

这对 Mnemon 很关键：必须持久存在的安装指引应放 root-level guideline 或 INSTALL；路径/阶段细节可以放 skill 或 hook prompt，但不能假设它们在压缩后一直完整可见。

## 定时任务与后台任务

Claude Code 的 scheduled tasks 分三类：

- `/loop`：当前 session 内反复运行 prompt，适合临时轮询。
- Desktop scheduled tasks：本机调度，适合需要本地文件和工具的任务。
- Cloud routines：Anthropic 托管调度，适合无需本机状态的任务。

公开文档没有把这些任务描述为自动整理 `CLAUDE.md` 的内置机制。它们可以被用户用来触发「检查记忆候选」「总结最近工作」「提醒保存状态」一类 prompt，但 memory 的最终整理仍应是 Markdown diff + review，而不是默认自动改写。

## 对 Mnemon 的启发

Mnemon 应学习 Claude Code 的轻量边界：

- `INSTALL.md` 说明如何把 Mnemon hook 安装到当前 agent。
- `GUIDELINE.md` 保存稳定行为原则，并保持 root-level 可见。
- skill 负责过程，memory 负责事实，不把所有东西塞进一份主文件。
- hook 可以在 session start、prompt submit、tool 后、stop/compact 前提醒 agent 执行记忆动作。
- 对可能膨胀的内容使用「候选 patch + review」而不是自动追加。

## 参考来源

- 官方文档: [Claude Code Memory](https://code.claude.com/docs/en/memory)
- 官方文档: [Claude Code Context Window](https://code.claude.com/docs/en/context-window)
- 官方文档: [Claude Code Scheduled Tasks](https://code.claude.com/docs/en/scheduled-tasks)
