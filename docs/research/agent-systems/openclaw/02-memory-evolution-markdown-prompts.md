# OpenClaw 的记忆、Markdown 与 Prompt 用法

## 记忆处理方案

OpenClaw memory 是多组件协作：

| 组件 | 作用 |
|---|---|
| `memory-core` | 默认 file-backed memory backend、search/get tools、dreaming |
| `active-memory` | 主回复前的 blocking recall sub-agent |
| `memory-wiki` | 编译知识 vault，保留 provenance |
| `memory-lancedb` / QMD 等 | 可选 backend |
| `DREAMS.md` | dreaming diary 和 phase summaries |

`memory_search` 是 broad recall，`memory_get` 是精确读取。文档强调 `MEMORY.md` 与 `memory/*.md` 被索引成 chunk，embedding provider 存在时可做 hybrid search。

## Active Memory Prompt 形态

`extensions/active-memory/index.ts` 中的 recall prompt 形态很关键：

- 它明确告诉子 agent：另一个模型会生成最终回答；
- 子 agent 只能用 memory tools；
- 输出必须是 `NONE` 或紧凑 plain-text summary；
- 有 timeout、cache、circuit breaker；
- 支持 balanced/strict/contextual/recall-heavy/preference-only 等 prompt styles；
- 会保存 hidden subagent transcript 供调试。

这比 Mnemon 当前需要的提醒重很多，但其中的 bounded output 和 `NONE` gate 值得借鉴。

## Markdown 文件用法

| 文件 | 角色 |
|---|---|
| `AGENTS.md` | 稳定 standing orders |
| `USER.md` | 用户/身份上下文 |
| `MEMORY.md` | long-term memory |
| `memory/*.md` | daily memory / indexed notes |
| `DREAMS.md` | dreaming diary，人类审查 |
| wiki vault pages | compiled durable knowledge |

OpenClaw 的 key insight 是：并不是所有 Markdown 都直接进 context。`MEMORY.md` 可作为 root memory，`memory/*.md` 多数时候通过 tools 访问。

## Dreaming 演化方案

Dreaming 是 OpenClaw 的自进化/记忆巩固路径：

- light phase：聚合短期信号，不写 `MEMORY.md`；
- REM phase：重组/叙事，不写 `MEMORY.md`；
- deep phase：评分并 promotion durable candidates，写 `MEMORY.md`；
- `DREAMS.md` 记录 diary 和 review trail；
- session transcripts 可 redaction 后进入 dreaming corpus；
- cron 定时 sweep，默认由 `memory-core` 管理。

这是一种强工程化的「记忆睡眠」机制。它强调可解释和 reviewable artifacts，这一点适合 Mnemon，但 cron/background/phase engine 对当前 Mnemon 太重。

## 对 Mnemon 的设计判断

OpenClaw 支持一个结论：memory-driven 自进化可以很强，但工程复杂度会迅速吞噬可移植性。

Mnemon 第一阶段应吸收：

- `NONE` gate；
- provenance；
- compaction 前 continuity capture；
- reviewable Markdown artifacts；
- memory tools 与 bootstrap docs 分离。

暂不吸收：

- active-memory hidden subagent runtime；
- memory wiki compiler；
- dreaming cron；
- 多 backend slot。

## 参考来源

- 本地源码: `extensions/active-memory/index.ts`
- 本地源码: `extensions/memory-core/src/prompt-section.ts`
- 本地源码: `extensions/memory-wiki/src/prompt-section.ts`
- 本地源码: `docs/concepts/dreaming.md`
- 本地源码: `docs/concepts/memory.md`
- 公开文档: [OpenClaw Active memory](https://docs.openclaw.ai/concepts/active-memory)
- 社区/博客信号: [OpenClaw Dreaming explained](https://openclawdc.com/blog/openclaw-dreaming-memory/)
