# Hermes 架构观察

## 一句话结论

Hermes 是本次调研中最接近 Mnemon 当前设计方向的系统。它明确把 facts 放进 bounded memory，把 procedures 放进 skills，把过往 session 做 FTS5 search，把复杂任务后的经验沉淀成 `SKILL.md`。它的核心不是复杂 adapter，而是 agent 读写 Markdown 资产并在运行中改进它们。

## 关键源码证据

本地源码快照：

- Hermes Agent: `/tmp/mnemon-agent-research-sources/hermes-agent`, HEAD `04918345ea31b1106d2ee6d4f42822f4f57616ee`
- Hermes Self-Evolution: `/tmp/mnemon-agent-research-sources/hermes-agent-self-evolution`, HEAD `4693c8f0eed21e39f065c6f38d98d2a403a04095`

| 位置 | 观察 |
|---|---|
| `README.md` | 宣称 closed learning loop：memory nudges、autonomous skill creation、skill self-improvement、FTS5 session search、Honcho user modeling |
| `agent/prompt_builder.py` | 组装 identity、memory guidance、session search guidance、skills guidance、context files |
| `website/docs/user-guide/features/memory.md` | `MEMORY.md` / `USER.md` 的用途、限制、最佳实践 |
| `website/docs/user-guide/features/skills.md` | skills 是 procedural memory，目录中有 `SKILL.md`、references、templates、scripts |
| `agent/curator.py` | 处理 skill 管理、自我整理和 skill patch/create/delete |
| `hermes-agent-self-evolution/README.md` | 使用 DSPy/GEPA 优化 skills、tool descriptions、system prompts、code |
| `hermes-agent-self-evolution/PLAN.md` | 明确 evolvable sections 包括 `MEMORY_GUIDANCE`、`SESSION_SEARCH_GUIDANCE`、`SKILLS_GUIDANCE` |

## 架构层次

```text
interfaces / messaging / CLI
  -> AIAgent loop
  -> prompt_builder
  -> tools
  -> memory files + providers
  -> session DB + FTS5
  -> skills directory
  -> curator / self-evolution pipeline
```

Hermes 的核心机制很直观：

- `prompt_builder.py` 构造系统 prompt；
- memory、session_search、skills 都以 guidance 形式进入 prompt；
- agent 通过工具保存 memory 或管理 skills；
- session history 存入 SQLite/FTS5，用 `session_search` 回忆；
- skills 存成 Markdown 目录，agent 可创建和 patch；
- self-evolution 是外部 pipeline，输出可审查变更。

## Prompt Builder 的关键边界

`agent/prompt_builder.py` 中的 guidance 体现了 Hermes 的思想：

- memory 用于 durable facts；
- session_search 用于过去对话；
- skills 用于 procedures；
- 复杂任务、修复 tricky error、发现 workflow 后可以保存 skill；
- 不要把 task progress/session outcomes/TODO 写进 memory；
- declarative facts 进 memory，procedures 进 skills。

这几乎就是 Mnemon 当前 `GUIDELINE.md` 要表达的判断。

## Profile 与隔离

Hermes 文档显示 profiles 有自己的 memory store、session database、skills directory。这个隔离设计对 Mnemon store strategy 有参考价值：默认 project-scoped，global 只存稳定跨项目偏好。

## 对 Mnemon 的启发

Hermes 证明轻量路线可行：

- 不需要每个 runtime 先做厚 adapter；
- memory guideline 可以直接作为 prompt/skill guidance；
- procedures 应转成 skills；
- agent 可以创建/更新 skills，但应保留 review；
- self-evolution 可以作为外部 pipeline，而不是 runtime 内核。

## 参考来源

- 本地源码: `hermes-agent/README.md`
- 本地源码: `hermes-agent/agent/prompt_builder.py`
- 本地源码: `hermes-agent/agent/curator.py`
- 本地源码: `hermes-agent/website/docs/user-guide/features/memory.md`
- 本地源码: `hermes-agent/website/docs/user-guide/features/skills.md`
- 本地源码: `hermes-agent-self-evolution/README.md`
- 本地源码: `hermes-agent-self-evolution/PLAN.md`
- 公开站点: [Hermes Agent](https://hermes-ai.net/)
