# OpenClaw 架构观察

## 一句话结论

OpenClaw 是本次调研中最重工程化的 agent runtime：它有 plugin SDK、workspace bootstrap、tool registry、memory slot、active-memory 子 agent、memory wiki、dreaming consolidation、compaction hooks。它适合作为能力上限参考，但不适合作为 Mnemon 第一阶段的实现模板。

## 关键源码证据

本地源码快照：`/tmp/mnemon-agent-research-sources/openclaw`

| 位置 | 观察 |
|---|---|
| `docs/concepts/agent-loop.md` | agent loop 中有 `before_prompt_build`、`before_compaction`、`after_compaction` 等 hook |
| `src/plugins/memory-runtime.ts` | 解析 active memory slot，加载 memory plugin runtime |
| `src/plugins/memory-state.ts` | 定义 memory capability、promptBuilder、flushPlanResolver、runtime、publicArtifacts |
| `extensions/memory-core/` | 默认 file-backed memory search、CLI、tools、prompt section |
| `extensions/active-memory/` | conversational turn 前运行 blocking memory sub-agent |
| `extensions/memory-wiki/` | 编译 wiki vault，提供 provenance-rich knowledge layer |
| `packages/memory-host-sdk/` | memory backend/search/session/dreaming host SDK |
| `docs/concepts/dreaming.md` | background memory consolidation phase 文档 |

## 运行时架构

OpenClaw 的核心是 plugin 化 runtime：

```text
channel / UI / gateway
  -> agent session
  -> plugin hooks
  -> prompt build
  -> tools
  -> memory runtime
  -> compaction / dreaming / wiki
```

重要点：

- plugin 可以注册 hooks、tools、commands、prompt contribution；
- `before_prompt_build` 是动态上下文注入点；
- `before_compaction` / `after_compaction` 是压缩生命周期点；
- memory 由 slot 管理，默认 active memory plugin 是 `memory-core`；
- memory artifacts 可是 markdown/json/text；
- workspace bootstrap 会读取固定 Markdown 文件。

## Workspace Markdown Bootstrap

OpenClaw 文档显示 bootstrap 会识别固定文件名：

- `AGENTS.md`
- `SOUL.md`
- `TOOLS.md`
- `IDENTITY.md`
- `USER.md`
- `HEARTBEAT.md`
- `BOOTSTRAP.md`
- `MEMORY.md`

`docs/concepts/system-prompt.md` 还说明 `memory/*.md` daily files 不属于普通 bootstrap context，通常通过 `memory_search` 和 `memory_get` 按需访问。这是一个重要边界：稳定规则自动进 prompt，长期记忆按需检索。

## Memory 架构

OpenClaw 的 memory 至少分四层：

1. **root memory**：`MEMORY.md` 表达 long-term durable facts。
2. **daily memory**：`memory/*.md`，按需检索。
3. **active-memory**：在主回复前运行 bounded memory sub-agent，只允许 memory tools。
4. **memory-wiki**：把 durable memory 编译成 wiki vault，支持 claims、dashboard、provenance。
5. **dreaming**：后台 consolidation，将强短期信号推广到 `MEMORY.md`，输出 `DREAMS.md` 和 phase reports。

这已经超过「memory tool」范畴，是完整 memory runtime。

## Hook 架构

关键 hooks：

- `before_prompt_build`：动态插入 memory recall 或 system prompt contribution；
- `before_compaction`：压缩前处理保存；
- `after_compaction`：压缩后注释或修复；
- plugin hooks 可设置超时、顺序和 scoped behavior。

这证明 Mnemon 的四 phase hook 是合理的，但也警告：hook 太重会让系统复杂度快速上升。

## 对 Mnemon 的启发

可吸收：

- 固定 Markdown bootstrap 文件名；
- memory search/get 工具分离；
- active recall 应 bounded，有 `NONE` 输出；
- dreaming 的 reviewable artifacts；
- compaction 前保存关键连续性。

不应照搬：

- 多 memory plugin slot；
- wiki compiler 第一阶段；
- background dreaming cron；
- 大型 plugin SDK；
- runtime 内部 memory engine。

Mnemon 更适合先做可安装 Markdown harness，把 heavy capabilities 留作未来可选层。

## 参考来源

- 本地源码: `docs/concepts/agent-loop.md`
- 本地源码: `docs/concepts/memory.md`
- 本地源码: `docs/concepts/dreaming.md`
- 本地源码: `extensions/memory-core/`
- 本地源码: `extensions/active-memory/`
- 本地源码: `extensions/memory-wiki/`
- 官方/公开文档: [Active memory](https://docs.openclaw.ai/concepts/active-memory)
