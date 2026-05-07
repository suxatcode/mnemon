# Hermes 的记忆、Markdown 与 Prompt 用法

## 记忆处理方案

Hermes 内置 memory 由两个 bounded Markdown 文件组成：

| 文件 | 用途 |
|---|---|
| `~/.hermes/memories/MEMORY.md` | agent 对环境、项目、事实、决策的 durable memory |
| `~/.hermes/memories/USER.md` | 用户偏好、用户画像、交互风格 |

文档中给出了字符限制：`MEMORY.md` 约 2200 chars，`USER.md` 约 1375 chars。它们在 session start 注入为 frozen system prompt block。这样做保护 prefix cache：session 中 memory 文件变化会持久化，但当前 session 不会动态改变已缓存 system prefix。

Hermes 还提供：

- `memory` tool：add/replace/remove；
- `session_search`：SQLite FTS5 + LLM summarization；
- external memory providers：Honcho、Mem0、Hindsight 等，作为 provider plugin；
- prompt-injection 扫描和 invisible unicode 防护。

## Skills 是 procedural memory

Hermes 文档明确区分：

- memory 是 facts；
- skills 是 procedures。

典型 skill 目录：

```text
~/.hermes/skills/<skill>/
  SKILL.md
  references/
  templates/
  scripts/
  assets/
```

`SKILL.md` 带 YAML frontmatter，包含 name、description、version、platforms、metadata.hermes 等。agent 可通过 `skill_manage` 创建、更新、删除 skills。复杂任务后，Hermes 会主动提出把做法保存为 skill。

## 特殊 prompt

`prompt_builder.py` 中几个 prompt section 值得 Mnemon 直接参考：

- `MEMORY_GUIDANCE`：何时保存 memory，何时不保存；
- `SESSION_SEARCH_GUIDANCE`：何时搜索过去 session；
- `SKILLS_GUIDANCE`：何时创建/更新 skill；
- context 文件扫描：过滤 prompt injection、credential exfiltration、invisible unicode。

这些 prompt 不是一次性长说明，而是每次 session 的稳定行为宪法。

## 自进化方案

Hermes 自进化分两层：

1. **运行时轻量演化**：agent 使用 `skill_manage` 将成功 workflow 写成 skill，或 patch 过时 skill。
2. **外部优化 pipeline**：`hermes-agent-self-evolution` 使用 DSPy + GEPA 读取当前 skill/prompt/tool description，生成 eval dataset，优化候选，输出可审查改动。

`PLAN.md` 还明确哪些内容可演化：

- `MEMORY_GUIDANCE`
- `SESSION_SEARCH_GUIDANCE`
- `SKILLS_GUIDANCE`
- identity / platform hints / tool descriptions

不可演化：

- 用户真实 memory block；
- generated memory data；
- 当前上下文文件。

## 对 Mnemon 的设计判断

Hermes 是 Mnemon 第一阶段最好的参考：

- 用 Markdown 指导 agent 行为；
- 用 bounded memory 防止无限膨胀；
- 用 skills 承载 procedures；
- 用 session search 召回过去对话；
- 自进化先输出 Markdown diff，而不是自动改代码。

Mnemon 当前应采用 Hermes 风格，而不是 OpenClaw 风格：

```text
memory facts
  + skills as procedures
  + guideline as behavior policy
  + hook reminders
  + reviewed markdown evolution
```

## 参考来源

- 本地源码: `website/docs/user-guide/features/memory.md`
- 本地源码: `website/docs/user-guide/features/skills.md`
- 本地源码: `website/docs/guides/work-with-skills.md`
- 本地源码: `agent/prompt_builder.py`
- 本地源码: `agent/curator.py`
- 本地源码: `hermes-agent-self-evolution/README.md`
- 本地源码: `hermes-agent-self-evolution/PLAN.md`
- 公开站点: [Hermes Agent](https://hermes-ai.net/)
