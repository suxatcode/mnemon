# Codex 架构观察

## 一句话结论

Codex 是一个本地优先的 coding agent runtime：配置、项目指令、skills、hooks、memories、subagents、MCP/apps 等都被组装进一次会话的开发者上下文。它非常适合验证 Mnemon 的轻量 harness 思路，因为 Codex 官方本身就把 `AGENTS.md`、skills、hooks 和 generated memories 分成不同责任层。

## 关键源码证据

本地源码快照：`/tmp/mnemon-agent-research-sources/codex`

| 位置 | 观察 |
|---|---|
| `docs/agents_md.md` | 指向官方 `AGENTS.md` 文档，并说明 `child_agents_md` feature 会追加 scope/precedence guidance |
| `codex-rs/core/src/session/mod.rs` | 会话初始化时组合 base instructions、developer instructions、user instructions、skills、memories、plugins 等上下文 |
| `codex-rs/config/src/types.rs` | 定义 memories、hooks、skills、model instructions 等配置结构 |
| `codex-rs/features/src/lib.rs` | `memories`、`codex_hooks`、`multi_agent`、`skills` 等 feature flags |
| `codex-rs/hooks/` | hooks discovery、dispatcher、schema、event handlers |
| `codex-rs/memories/` | memories read/write/mcp pipeline |
| `codex-rs/core-skills/` | `SKILL.md` loader、frontmatter、metadata |

## 架构层次

| 层 | 机制 | 作用 |
|---|---|---|
| 配置层 | `~/.codex/config.toml`, project `.codex/config.toml` | feature flags、model、hooks、skills、memories、sandbox |
| 指令层 | `AGENTS.md`, `model_instructions_file`, `developer_instructions` | 持久项目规则与开发者约束 |
| 扩展层 | skills、plugins、MCP/apps | 可复用工具说明和外部能力 |
| 生命周期层 | hooks | `SessionStart`, `UserPromptSubmit`, `PreToolUse`, `PostToolUse`, `Stop` 等事件 |
| 记忆层 | `~/.codex/memories/` | generated local memory files，作为 helpful recall layer |
| 多 agent 层 | worker/explorer 等 subagents | 并行探索、实现、审查 |

## `AGENTS.md` 装载模型

官方文档说明 Codex 在开始工作前读取 `AGENTS.md`：

- global scope: `~/.codex/AGENTS.override.md` 优先，否则 `~/.codex/AGENTS.md`；
- project scope: 从项目 root 到 cwd 逐级读取；
- 每层优先 `AGENTS.override.md`，再 `AGENTS.md`，再 fallback filenames；
- root-to-leaf 合并，越接近 cwd 越晚出现，因此优先级更高；
- 默认总大小限制为 `project_doc_max_bytes = 32 KiB`。

这是一种明确的 Markdown 指令层，而不是 memory database。

## Hooks 架构

官方 hooks 文档和源码 `codex-rs/hooks/` 一致：

- hooks 需要 `[features] codex_hooks = true`；
- 位置包括 `~/.codex/hooks.json`、`~/.codex/config.toml`、repo `.codex/hooks.json`、repo `.codex/config.toml`；
- 多个 matching hooks 都会执行；
- `SessionStart`、`UserPromptSubmit` 可以加入上下文；
- `PreToolUse` / `PermissionRequest` 可做工具级 guardrail；
- `PostToolUse` 可反馈工具结果；
- `Stop` 可让 Codex 继续一轮。

这给 Mnemon 的四 phase hook 提供了直接映射：Prime 对应 `SessionStart`，Remind 对应 `UserPromptSubmit`，Nudge 对应 `Stop`，Compact 可由 compaction prompt 或未来 lifecycle hook 模拟。

## 与 Mnemon 设计的关系

Codex 的架构支持 Mnemon 的轻量安装方式：

- `SKILL.md` 可作为 Codex skill；
- `GUIDELINE.md` 可进入 `AGENTS.md` 或 project docs；
- `INSTALL.md` 可指导 Codex 为自己安装 hooks；
- memories 本身是 generated state，不应替代 checked-in rules。

## 参考来源

- 官方文档: [Custom instructions with AGENTS.md](https://developers.openai.com/codex/guides/agents-md)
- 官方文档: [Codex Hooks](https://developers.openai.com/codex/hooks)
- 官方文档: [Configuration Reference](https://developers.openai.com/codex/config-reference)
- 本地源码: `/tmp/mnemon-agent-research-sources/codex/codex-rs/hooks/`
- 本地源码: `/tmp/mnemon-agent-research-sources/codex/codex-rs/memories/`
