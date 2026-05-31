# Mnemon Harness 公开 Beta

`mnemon-harness` 是一个实验性 beta 层，用来把 host agent 接入项目本地的受治理状态。它目前只支持源码构建，并且有意和稳定的 `mnemon` CLI 保持分离。

稳定版 Mnemon 仍然专注于记忆与召回。Harness 在 Codex、Claude Code 等 host agent 周围加入 lifecycle exchange、evidence、proposal、audit、coordination topology 和审阅 TUI。

## 1. What It Is

Mnemon Harness 是一个 governed agent-state substrate。

```text
host agent
  <-> Lifecycle Exchange
      context out: .codex/.claude projection files
      signal in:   .mnemon/events.jsonl
  <-> governed project state
      profile + goals + proposals + audit + coordination
```

`.codex`、`.claude` 等目录只是投影表面。真正的 canonical state 是 `.mnemon/` 下的 append-only event log 和受治理记录。

## 2. Current Beta Surface

公开 beta 包含：

- lifecycle event append/status/daemon 命令
- Codex 与 Claude Code projection surface
- projection envelope 与 readback verification
- profile 投影到 host context
- goal、eval、proposal、apply、audit 命令
- coordination topology 与 governed coordination apply
- hosts、evidence、proposals、profile、coordination、trace 的 TUI 视图
- 由显式用户动作和 cost gate 保护的 Codex runner check

它不承诺生产可用、自动 apply、完整个人/team/org scope composition，或完整多 agent runtime。

## 3. Separation From Stable Mnemon

`mnemon-harness` 从 `./harness/cmd/mnemon-harness` 构建。

稳定版 `mnemon` binary 不 import harness package。它只暴露一个很窄、默认关闭的 event seam，让项目可以写入 harness 之后会读取的事件。

```sh
MNEMON_HARNESS_EVENT_EMIT=1 mnemon remember "..." --cat note
mnemon event emit custom.observed --payload '{"ok":true}'
```

如果没有 opt-in 环境变量或显式 `mnemon event` 命令，稳定版 Mnemon 的行为不变。

## 4. Try It

构建两个 binary：

```sh
go build -o mnemon .
go build -o mnemon-harness ./harness/cmd/mnemon-harness
```

运行 no-model smoke：

```sh
tmpdir="$(mktemp -d)"
./mnemon-harness lifecycle --root "$tmpdir" init
./mnemon-harness lifecycle --root "$tmpdir" event append --json '{
  "schema_version": 1,
  "id": "evt_harness_smoke_001",
  "ts": "2026-05-31T00:00:00Z",
  "type": "memory.hot_write_observed",
  "loop": "memory",
  "host": "codex",
  "actor": "host-agent",
  "source": "harness-smoke",
  "correlation_id": "corr_harness_smoke",
  "payload": {"reason": "smoke"}
}'
./mnemon-harness lifecycle --root "$tmpdir" status refresh
./mnemon-harness ui --root "$tmpdir"
```

更多命令示例见 [USAGE.md](USAGE.md)。

## 5. Release Boundary

这个 beta 只发布最少量公开文档。内部计划、内部验证材料、生成站点 HTML 和详细未来计划不进入这个分支。
