# Hook、Nudge 与 Remind

## 结论

自进化需要触发点。没有 hook，记忆系统只能依赖模型“想起来要记”，这不是系统能力。

Mnemon 应把 hook 看成 memory-driven framework 的骨架：

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

nudge/remind 不是额外功能，而是让模型在正确时刻执行正确记忆动作的方式。

## Hermes 的 hook 形态

Hermes 有三类 hook：

| 类型 | 运行范围 | 典型用途 |
|---|---|---|
| Gateway hooks | gateway only | messaging/gateway lifecycle |
| Plugin hooks | CLI + Gateway | tool/LLM/session/gateway events |
| Shell hooks | CLI + Gateway | 配置里的命令式触发 |

Hermes plugin hooks 提供的事件非常适合 memory-driven framework：

| Hook | 对 Mnemon 的用途 |
|---|---|
| `pre_llm_call` | 在当前 turn 注入 recall/reminder，保持 system prompt cache 稳定 |
| `post_llm_call` | 观察输出，生成 reflection 候选 |
| `pre_tool_call` | 阻断危险工具，或提醒记录关键 evidence |
| `post_tool_call` | 捕获工具结果、错误、持续时间、可复用经验 |
| `on_session_start` | 加载 hot memory、guideline、安装状态 |
| `on_session_end` | 写 session summary 和候选 memory updates |
| `on_session_finalize` | 结束前最后一次 flush |
| `subagent_stop` | 汇总子任务结果和可复用流程 |
| `pre_gateway_dispatch` | 改写或跳过 gateway message |
| `pre_approval_request` | 在权限请求前注入安全 reminder |
| `post_approval_response` | 记录用户审批偏好或拒绝原因 |

Hermes 文档明确说明 `pre_llm_call` 的返回内容可以注入当前 turn user message，而不是修改 system prompt。这对 Mnemon 很重要：召回内容应尽量 ephemeral，避免破坏 prompt cache，也避免把临时 recall 永久化。

## Claude Code 的 hook 参照

Claude Code 的 hook 文档显示，hook 可以在多种生命周期事件触发。几个对 Mnemon 特别重要：

| Hook | 作用 |
|---|---|
| `SessionStart` | 启动时注入上下文 |
| `UserPromptSubmit` | 用户 prompt 进入模型前注入或阻断 |
| `PreToolUse` | 工具调用前允许/拒绝/提示 |
| `PostToolUse` | 工具调用后观察结果 |
| `Stop` | 模型结束前可要求继续执行保存动作 |
| `PreCompact` | 压缩前保存连续性 |
| `PostCompact` | 压缩后恢复摘要或提示 |

Claude Code 对 hook 输出也有容量限制。hook 注入上下文不能无限长，超过限制会被保存成文件并以预览替代。这进一步说明：hook 应注入短 reminder，不应把冷记忆原样塞进 prompt。

## OpenClaw 的 hook 参照

OpenClaw 的 hook 系统提供了 compaction 事件：

- `session:compact:before`，包含 messageCount、tokenCount。
- `session:compact:after`，包含 compactedCount、summaryLength、tokensBefore、tokensAfter。

它还有 bundled `session-memory` hook，能把最近 user/assistant 消息保存到 workspace 的 `memory/` 目录。OpenClaw 还支持 bootstrap-extra-files，把 `AGENTS.md`、`SOUL.md`、`TOOLS.md`、`IDENTITY.md`、`USER.md`、`HEARTBEAT.md`、`BOOTSTRAP.md`、`MEMORY.md` 等文件作为启动材料。

这说明 hook 不只是安全拦截，也可以是记忆落盘和启动引导机制。

## Nudge 和 Remind 的区别

建议 Mnemon 区分 nudge 与 remind。

| 类型 | 含义 | 示例 |
|---|---|---|
| remind | 把已有规则或记忆在合适时刻提醒模型 | “当前项目测试命令是 pnpm test” |
| nudge | 推动模型执行一个维护动作 | “本轮出现可复用工具坑点，请考虑提出 skill patch” |

remind 主要服务当前任务，nudge 主要服务长期演化。

## 四阶段 hook 设计

用户提到“四个阶段要做 hook”。结合 Hermes/OpenClaw/Claude Code，Mnemon 可以定义为：

### 1. Recall Hook

时机：

- session start。
- user prompt submit。
- pre LLM call。

职责：

- 读取 `GUIDELINE.md`。
- 加载热记忆 capsule。
- 根据当前任务召回 cold/warm 相关内容。
- 输出短上下文或 `NONE`。

边界：

- 不永久写 memory。
- 不注入长历史。
- 不覆盖当前用户指令。

### 2. Observe Hook

时机：

- pre tool。
- post tool。
- approval request/response。
- file changed。

职责：

- 记录工具错误和成功命令。
- 捕获用户审批偏好。
- 捕获重复出现的问题。
- 写 cold evidence。

边界：

- 默认不写 hot memory。
- secret 和敏感内容先过滤。
- 只写 evidence，不写结论。

### 3. Reflect Hook

时机：

- post LLM。
- stop。
- session end。
- subagent stop。

职责：

- 判断是否有 durable fact。
- 判断是否需要 patch skill。
- 生成 review proposal。
- 写 warm session summary。

边界：

- proposal-first。
- 高风险变更不自动落地。
- 一次性进度只进 session summary。

### 4. Curate Hook

时机：

- idle。
- scheduled task。
- 手动命令。
- pre compact 前的轻量 flush。

职责：

- 合并重复 skill。
- demote 过长 hot memory。
- promote 高价值 cold memory。
- 生成 report。
- archive stale artifacts。

边界：

- dry-run-first。
- archive 不 delete。
- pinned 不动。
- bundled/package 不动。

## Hook 输出的设计规则

Hook 输出要尽量结构化。

### Recall 输出

```yaml
type: recall
status: ok
context:
  - source: hot/project.md
    text: "Use pnpm for this repository."
  - source: skills/research/SKILL.md
    text: "Prefer official docs for current behavior."
```

如果无相关内容：

```yaml
type: recall
status: none
reason: "No relevant memory above threshold."
```

### Reflect 输出

```yaml
type: reflection
proposals:
  - target: skills/debugging/SKILL.md
    action: patch
    reason: "Repeated dev-server port collision workaround succeeded."
    risk: low
  - target: memory/hot/project.md
    action: add
    reason: "User confirmed project uses pnpm."
    risk: low
```

### Curate 输出

```yaml
type: curate
consolidations:
  - from: debug-vite-port
    into: dev-server-troubleshooting
    reason: "Narrow case covered by umbrella skill."
archives:
  - target: old-release-checklist
    reason: "Unused for 120 days and superseded."
```

Hermes curator 的结构化 YAML 输出方式值得复用。

## 安全与失控边界

Hook 也可能制造问题。Mnemon 应默认限制：

| 风险 | 约束 |
|---|---|
| hook 无限注入上下文 | 输出预算和 `NONE` gate |
| hook 隐式改行为 | 所有持久修改走 proposal/report |
| hook 阻断正常工作 | 默认非阻塞，只有安全策略可阻断 |
| scheduled task 递归 | 维护任务不能创建同类维护任务 |
| secret 被写入 memory | pre-write scanner 和 redaction |
| 旧 memory 覆盖新指令 | 当前用户指令优先，recall 只作辅助 |
| 多 hook 并发写 | lock + atomic write + report |

Claude Code 对 blocking hook 使用明确 exit code，Hermes hook 错误会记录但不崩溃 agent，OpenClaw workspace hooks 默认需要显式启用。这些都是防失控设计。

## 安装方式

Mnemon 的 `INSTALL.md` 不应要求所有 agent 使用同一个实现。它应该描述：

1. 当前 agent 支持哪些 hook。
2. 如何安装 recall/observe/reflect/curate 四类 hook。
3. 每类 hook 的输入输出。
4. 哪些变更允许自动写。
5. 哪些变更只允许 proposal。
6. 如何禁用、回滚、查看 report。

目标 agent 根据自己的平台完成安装：

- Hermes：plugin hook 或 shell hook。
- Claude Code：`.claude/settings*.json` hooks、skills、rules。
- OpenClaw：workspace hooks、plugin hooks、bootstrap files。
- Codex：skills、hooks、AGENTS.md 或项目规则。

这比写一个巨大的 universal adapter 更符合 Markdown-first 和 agent-installable 的思路。

## 设计判断

Mnemon 的 nudge/remind 体系应该是低侵入、可审查、可分层的：

- recall hook 只注入短上下文。
- observe hook 只落 evidence。
- reflect hook 只提 proposal。
- curate hook 默认 dry-run。

这样既能让系统长期自我演化，又不会变成后台自动改写一切的黑箱。

## 参考来源

- Hermes hooks: <https://hermes-agent.nousresearch.com/docs/user-guide/features/hooks>
- Hermes cron: <https://hermes-agent.nousresearch.com/docs/user-guide/features/cron>
- Claude Code hooks: <https://code.claude.com/docs/en/hooks>
- Claude Code scheduled tasks: <https://code.claude.com/docs/en/scheduled-tasks>
- OpenClaw hooks: <https://docs.openclaw.ai/automation/hooks>
- OpenClaw compaction: <https://docs.openclaw.ai/concepts/compaction>
