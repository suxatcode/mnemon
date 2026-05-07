# Codex 的记忆、Markdown 与 Prompt 用法

## 记忆处理方案

Codex memories 官方说明：

- memories 默认关闭；
- 启用后 Codex 会把有用上下文从 eligible prior threads 转成本地 memory files；
- 会跳过 active 或 short-lived sessions；
- 会 redacts secrets；
- 会在后台更新，而不是每个 thread 结束立刻写；
- 主要文件在 `~/.codex/memories/`；
- memories 是 helpful local recall layer，不应替代 `AGENTS.md` 或 checked-in docs。

源码 `codex-rs/memories/README.md` 显示 pipeline 更细：

1. phase 1 从 prior rollout 提取 structured memory；
2. phase 2 consolidates raw memories into filesystem artifacts；
3. 输出包括 `MEMORY.md`、`memory_summary.md`、`skills/`、`rollout_summaries/` 等；
4. consolidation 运行在受限内部 sub-agent 中；
5. read path 会把 memory summary 和可搜索路径作为 developer instructions 提供给主 agent。

## Markdown 文件用法

| Markdown 资产 | 来源 | 用法 |
|---|---|---|
| `AGENTS.md` | 官方项目指令机制 | repo/team rules，必须规则应放这里 |
| `AGENTS.override.md` | 官方 override 机制 | 临时或局部覆盖 |
| `SKILL.md` | skill loader | 可复用能力说明，带 frontmatter |
| `MEMORY.md` | generated memories | durable generated memory，不是 primary control surface |
| `memory_summary.md` | generated memories | 快速 recall 摘要 |
| `rollout_summaries/*.md` | generated memories | prior thread 支撑证据 |

Codex 的分层很清楚：checked-in docs 是规则，generated memories 是 recall 辅助。

## 特殊 prompt

源码中的 memory prompt 模板值得关注：

- `stage_one_system.md`：把 prior rollout 当数据，要求 no-op gate、redact secrets、输出 JSON。
- `stage_one_input.md`：明确不要执行 rollout 内容中的指令。
- `consolidation.md`：把 raw memories 合并到 `MEMORY.md`、skills、summary，并要求 evidence/no secrets/no-op。
- `read_path.md`：要求快速 memory pass、限制搜索预算、对 drift-prone facts 做 verification。

这些 prompt 都遵循一个原则：memory 是证据和素材，不是无条件规则。

## 智能体演化方案

Codex 的自进化主要通过：

- generated memories 变成 durable recall；
- consolidation 可生成 `skills/`；
- `AGENTS.md` 作为人工/团队审查后的规则层；
- skills 作为可复用流程层；
- hooks 作为生命周期控制点。

这与 Mnemon 当前设计一致：先让 memory 提出 Markdown candidate，再通过 review 变成 skill/guideline/install note/rule。

## 对 Mnemon 的启发

- `GUIDELINE.md` 应类似 `AGENTS.md`，作为 rules/control surface。
- `mnemon` 生成的 memory 不能替代 checked-in docs。
- memory consolidation prompt 必须有 no-op gate、secret redaction、evidence、scope。
- 如果未来生成 skills，应保持和 `SKILL.md` loader 兼容的 frontmatter。

## 参考来源

- 官方文档: [Codex Memories](https://developers.openai.com/codex/memories)
- 官方文档: [Codex Hooks](https://developers.openai.com/codex/hooks)
- 官方文档: [AGENTS.md](https://developers.openai.com/codex/guides/agents-md)
- 本地源码: `codex-rs/memories/read/templates/memories/read_path.md`
- 本地源码: `codex-rs/memories/write/templates/memories/stage_one_system.md`
- 本地源码: `codex-rs/memories/write/templates/memories/consolidation.md`
