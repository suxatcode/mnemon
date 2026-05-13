# Modular Agent Harness 设计

Mnemon 的核心优势是 modular agent 模型：自进化能力应该作为外置
harness 挂载到已有 agent 上，而不是重新实现一个 agent framework。

## 核心判断

任何支持标准扩展点的宿主 agent，都可以通过安装 Mnemon harness module
获得自进化能力。

宿主 agent 拥有 ReAct loop：

```text
观察上下文 -> 推理 -> 调用工具 -> 检查结果 -> 继续或停止
```

Mnemon 在这个 runtime 外围挂载额外 loop：

```text
Memory Loop：经验 -> working memory -> long-term memory -> recall
Skill Loop：重复 workflow -> evidence -> proposal -> skill lifecycle
Future Loops：evaluation、risk review、safety checks、benchmark feedback
```

## 宿主与 Harness 分工

| 层 | 所属 | 职责 |
| --- | --- | --- |
| ReAct loop | Host agent | 任务执行、规划、工具调用、验证、用户交互。 |
| Prompt assembly | Host agent | 决定哪些上下文进入模型。 |
| Tool routing | Host agent | 在宿主权限模型下选择和执行工具。 |
| Native skills | Host agent | 使用宿主自己的机制发现和调用 skill。 |
| Evolution modules | Mnemon harness | 通过可挂载资产增加 memory、skill evolution、evaluation、review loop。 |
| Canonical state | Mnemon harness | 保存持久记忆、skill lifecycle state、evidence、proposal 和 report。 |

这个分工让 Mnemon 保持可移植。宿主可以只采用某一个 module，而不必更换
runtime。

## 标准接入面

| 原语 | Harness 用法 |
| --- | --- |
| Hooks | 在 Prime、Remind、Nudge、Compact 或等价宿主事件上安装生命周期提醒。 |
| Skills | 暴露 `memory_get`、`memory_set`、`skill_observe`、`skill_manage` 等 protocol 操作。 |
| Subagents | 在在线任务路径之外运行 dreaming、curator review 等较重的维护任务。 |
| Filesystem | 在可预测目录和 project/user scope 下保存 canonical module state。 |
| Environment | 让 protocol skill 通过环境变量解析路径，而不是写死某个宿主 agent。 |

最低要求是宿主具备 hook-like 生命周期机制。Skills 和 subagents 会让集成更
自然，但有能力的 agent 也可以直接遵循 Markdown protocol。

## 当前 Module

| Module | 目的 | 当前参考宿主 |
| --- | --- | --- |
| Memory Loop | 增加 working memory、long-term memory 和 dreaming consolidation。 | Claude Code setup 位于 `harness/memory-loop/setup/claude-code`。 |
| Skill Loop | 增加 active/stale/archived skill lifecycle、evidence capture、curator proposal 和批准后的 lifecycle mutation。 | Claude Code setup 位于 `harness/skill-loop/setup/claude-code`。 |

## Memory 差异化

Memory module 使用冷热记忆模型：

- Working memory 面向模型。它是小型 Markdown 上下文，进入 prompt，由
  agent 维护。
- Long-term memory 面向工程。Mnemon 在 prompt 外保存更大、更持久的记忆，
  并按需召回。
- Dreaming 负责二者之间的巩固：把 durable working memory 写入 Mnemon，
  然后压缩或淘汰 prompt-facing working memory。

这保留了 Markdown memory 的模型友好性，同时避免单个 always-loaded 文件的
容量上限。

## 未来 Module

同样的 harness 模式可以继续支持更多 loop：

- Eval loop：收集结果、运行 benchmark，并把失败反馈为 proposal。
- Risk loop：在 skill 或 memory 变更生效前进行扫描。
- Review loop：协调人工审批、checkpoint 和 release gate。
- Policy loop：维护宿主特定的安全与权限策略。

每个 module 都应保持可独立安装。

## 非目标

- 不替换宿主 agent runtime。
- 不要求唯一通用 skill 格式。
- 不把所有 state 注入 prompt。
- 不在缺少明确策略和 review 的情况下进行 self-modifying change。

## Reference Case

Claude Code 是第一个 modular-agent case，因为它已经暴露 hooks、skills、
subagents、filesystem config 和 project/user scope。Claude Code setup 能验证
外挂模型，但 Mnemon 的目标是任何具备类似扩展点的宿主 agent。
