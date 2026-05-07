# Claude Code 的记忆、Markdown 与 Prompt 用法

## 记忆处理方案

Claude Code 的公开 memory 设计重点不是一个单独的外部数据库，而是多种 Markdown 上下文机制：

- `CLAUDE.md`：项目/用户/本地指令入口。
- `@path` imports：把长指令拆成多个文件。
- `.claude/rules/`：更结构化的项目规则。
- settings hooks：在 session start、user prompt、tool use、stop、compact 等阶段注入提醒。
- subagents：把复杂任务放进独立上下文。
- skills / commands：把可复用流程写成 Markdown，可被用户或模型调用。

Claude Code 的实际「记忆」更像文件化操作系统上下文，而不是单一 memory store。用户和团队把稳定信息写入文件，agent 在启动或调用时读取。

## Markdown 文件用法

| Markdown 资产 | 用途 | 对 Mnemon 的启发 |
|---|---|---|
| `CLAUDE.md` | 总入口，项目规则和 imports | Mnemon 可用 `GUIDELINE.md` 做行为总纲 |
| `.claude/agents/*.md` | subagent 定义 | 记忆整理可选用 subagent，但不是必需 |
| skills / commands | 可执行流程说明 | `SKILL.md` 应教命令，流程进入 skill |
| imported docs | 长规范、标准、背景资料 | `INSTALL.md` 可导入或引用 guideline |

## 特殊 prompt 形态

Claude Code 的 prompt 资产有两个共同点：

1. **YAML frontmatter + Markdown body**：subagents 和 skills 都采用类似形态，frontmatter 描述用途、工具、模型、可见性，body 是执行指令。
2. **hook additional context**：hook 不一定产生聊天消息，而是把 `additionalContext` 或 stdout 注入为系统提醒。

这说明 Mnemon 的 hook 输出应短小、上下文型、可忽略，而不是长 prompt 或强制命令。

## 智能体演化方案

Claude Code 的公开机制支持演化，但主要是人工/agent 协作修改 Markdown 资产：

- `/init` 或人工维护 `CLAUDE.md`；
- 创建/更新 skills；
- 创建/更新 subagents；
- 用 hooks 做安全、日志、验证或上下文注入；
- 社区实践常把「学到的流程」写回命令、skills 或项目规则。

它不是自动重写 runtime 的系统。演化边界仍是可审查的文件变更。

## 社区实践信号

公开社区讨论中常见共识：

- 主 `CLAUDE.md` 应短而稳定；
- 长流程应拆成 skills/commands；
- subagent 用于上下文隔离；
- hooks 适合安全检查、决策捕获、session 总结、持久规则提醒；
- 单纯把所有东西塞进主指令会浪费 context 并降低可维护性。

这些信号支持 Mnemon 当前方案：把能力、安装和判断分别放入 `SKILL.md`、`INSTALL.md`、`GUIDELINE.md`。

## 风险

- Markdown 过多会造成发现困难。
- hooks 过强会变成隐式控制器。
- subagent 太多会增加延迟和调试成本。
- 旧文件指令可能覆盖当前事实，需要明确 stale memory 处理规则。

## 参考来源

- 官方文档: [Memory](https://code.claude.com/docs/en/memory)
- 官方文档: [Hooks](https://code.claude.com/docs/en/hooks)
- 官方文档: [Subagents](https://code.claude.com/docs/en/sub-agents)
- 官方文档: [Skills / custom commands](https://code.claude.com/docs/en/slash-commands)
- 社区讨论样例: [Claude Code build system discussion](https://www.reddit.com/r/ClaudeCode/comments/1swcwb6/claude_code_is_a_build_system_not_a_chatbot_13/)
