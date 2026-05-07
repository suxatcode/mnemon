# OpenClaw memory lifecycle 细节

## 核心判断

OpenClaw 是本轮调研中工程化程度最高的 memory runtime。它把 Markdown 文件、semantic search、active recall、compaction 前 flush、dreaming consolidation、wiki compiler 和 cron sweep 组合成一套完整系统。

这给 Mnemon 的启发是「上限参考」而不是「第一阶段照搬」。Mnemon 应学习它的 reviewable artifacts、compaction 前保存和分阶段 consolidation，但暂不复制 active-memory hidden subagent、wiki compiler 和 dreaming scheduler。

## 生命周期详表

| 维度 | 观察 |
|---|---|
| 主要记忆载体 | `MEMORY.md`、`memory/YYYY-MM-DD.md`、`DREAMS.md`、`memory/.dreams/`、可选 wiki vault。 |
| 存储位置 | agent workspace，默认 `~/.openclaw/workspace`。 |
| 加载路径 | `MEMORY.md` 在每个 DM session start 加载；today/yesterday daily notes 自动加载；更多历史通过 tools 搜索/读取。 |
| 工具路径 | `memory_search` 做 broad/semantic recall；`memory_get` 精确读取文件或行范围。 |
| 后台召回 | `active-memory` 可在主回复前运行 blocking recall subagent，输出紧凑 summary 或 `NONE`。 |
| 长度限制 | 没有单个 `MEMORY.md` 公共硬限制；实际由上下文预算、索引 chunk、active-memory 输出上限、tool timeout 和 compaction 机制控制。 |
| active-memory 限制 | 默认 summary max chars 220；user turn chars 220；assistant turn chars 180；timeout 15s；partial transcript max chars 32000；read max lines 2000；read max bytes 50MB；search query max chars 480。 |
| search/index 限制 | local embedding context 默认 4096；常见 chunk 128-512 tokens；multimodal max file bytes 10,000,000；embedding cache max entries 50,000 但默认 disabled。 |
| 超出处理 | session 接近或超过 context window 时 auto-compaction 默认启用；compaction 前可运行 silent memory flush turn，把 durable notes 写入磁盘。 |
| 整理方式 | Dreaming light/REM/deep 三阶段巩固；memory-wiki 可把 durable knowledge 编译成有 evidence/freshness/contradiction 的 wiki。 |
| 定时任务 | Dreaming opt-in，默认 disabled；启用后 `memory-core` auto-manages cron job，默认 `0 3 * * *`。 |
| promotion 阈值 | deep phase 使用 min score、min recall count、min unique queries；源码默认 min score 0.8、min recall count 3、min unique queries 3、max age 30 days。 |
| 安全边界 | transcript ingestion 会 redaction；Dream Diary/report artifacts 不作为 promotion source；长期 promotion 只写 `MEMORY.md`。 |

## 文件层级

OpenClaw 的 memory 文件非常接近 Mnemon 讨论中的 Markdown-first 形态：

```text
workspace/
  MEMORY.md
  DREAMS.md
  memory/
    YYYY-MM-DD.md
    .dreams/
    dreaming/<phase>/YYYY-MM-DD.md
```

关键区别在于 OpenClaw 不把所有 Markdown 都直接放进上下文。`MEMORY.md` 是长期 root，daily notes 是短期工作记忆，历史通过 `memory_search` 和 `memory_get` 按需进入上下文。

## Dreaming 整理机制

Dreaming 是 OpenClaw 的核心记忆巩固机制：

| 阶段 | 读取 | 写入 | 是否 promotion |
|---|---|---|---|
| Light | recent daily memory、recall traces、redacted transcripts | candidate lines、phase signals | 否 |
| REM | short-term traces、theme signals | `DREAMS.md` 的反思/主题块 | 否 |
| Deep | staged candidates、recall evidence、phase reinforcement | promoted entries 到 `MEMORY.md` | 是 |

deep ranking 的公开权重包括：

- relevance 0.30；
- frequency 0.24；
- query diversity 0.15；
- recency 0.15；
- consolidation 0.10；
- conceptual richness 0.06。

Dreaming 的好处是可解释：候选、评分、diary、promotion 都有 artifact。代价是 runtime 复杂、后台任务复杂、配置面复杂。

## 超出与 compaction 处理

OpenClaw 对上下文超出的策略是先保存，再压缩：

1. session 接近上下文窗口或 provider 返回 overflow。
2. auto-compaction 触发。
3. compaction 前可运行 silent memory flush turn，提醒 agent 把关键 durable context 写入 memory files。
4. 使用 compacted context retry 原请求。
5. 原始 conversation 仍保留在磁盘，compaction 只影响下一次模型上下文。

这点对 Mnemon 非常重要：memory hook 不应只在 turn end 运行，也应有 pre-compact/pre-stop 的「连续性捕获」职责。

## 定时与后台任务

OpenClaw 中有两类后台能力：

- active-memory：主回复前的同步/阻塞召回，适合在每轮回答前补上下文。
- dreaming：启用后由 cron 定期运行 full sweep，默认每天 03:00。

Mnemon 第一阶段不应做长期驻留 scheduler。更好的做法是让 INSTALL 文档说明：如果目标 agent 支持 scheduled tasks，可以可选安装一个「weekly memory review」或「pre-compact save」任务；默认只依赖 hooks 和手动命令。

## 对 Mnemon 的启发

- 采用 `NONE` gate：没有相关记忆时明确不注入，避免噪音。
- 把 daily notes、long-term facts、review diary 分开。
- 在 compaction 前保存关键状态。
- promotion 必须有 evidence、recency、frequency 或用户确认。
- 定时 dreaming 可以作为未来高级能力，不放入第一阶段核心。

## 参考来源

- 官方文档: [OpenClaw Memory Overview](https://docs.openclaw.ai/concepts/memory)
- 官方文档: [OpenClaw Dreaming](https://docs.openclaw.ai/concepts/dreaming)
- 官方文档: [OpenClaw Compaction](https://docs.openclaw.ai/concepts/compaction)
- 本地源码: `/tmp/mnemon-agent-research-sources/openclaw/extensions/active-memory/index.ts`
- 本地源码: `/tmp/mnemon-agent-research-sources/openclaw/extensions/memory-core/src/dreaming.ts`
- 本地源码: `/tmp/mnemon-agent-research-sources/openclaw/src/memory-host-sdk/dreaming.ts`
- 本地源码: `/tmp/mnemon-agent-research-sources/openclaw/src/agents/pi-embedded-runner/run/preemptive-compaction.ts`
