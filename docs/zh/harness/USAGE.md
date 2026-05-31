# Mnemon Harness 使用说明

以下命令假设你已经构建：

```sh
go build -o mnemon .
go build -o mnemon-harness ./harness/cmd/mnemon-harness
```

探索时建议使用临时 root。

## 1. Lifecycle Basics

```sh
tmpdir="$(mktemp -d)"

./mnemon-harness lifecycle --root "$tmpdir" init
./mnemon-harness lifecycle --root "$tmpdir" event append --json '{
  "schema_version": 1,
  "id": "evt_001",
  "ts": "2026-05-31T00:00:00Z",
  "type": "memory.hot_write_observed",
  "loop": "memory",
  "host": "codex",
  "actor": "host-agent",
  "source": "manual",
  "correlation_id": "corr_001",
  "payload": {"note": "hello"}
}'
./mnemon-harness lifecycle --root "$tmpdir" status refresh
```

## 2. Projection And Readback

写入真实项目之前先预览：

```sh
./mnemon-harness loop validate
./mnemon-harness loop diff --host codex --loop memory --project-root .
```

确认 diff 后再安装 projection：

```sh
./mnemon-harness loop install --host codex --loop memory --project-root .
```

`.codex/` 或 `.claude/` 下的投影文件是 host surface。host 可以读取 `PROJECTION.json`，并在之后的 writeback event 中回传 `projection_ref` 和 `context_digest`。Harness 用这个回传区分 observed、mismatch、unattributed、silent 和 stale。

## 3. Profile And Governance

通过受治理 proposal route 添加 profile entry：

```sh
./mnemon-harness proposal --root "$tmpdir" create \
  --proposal-id profile-preference-001 \
  --route memory \
  --title "Remember project preference" \
  --target profile:project \
  --payload '{"summary":"Prefer concise public docs","projection_targets":[{"host":"codex","loop":"memory"}]}'

./mnemon-harness proposal --root "$tmpdir" approve --proposal-id profile-preference-001
./mnemon-harness proposal --root "$tmpdir" apply --proposal-id profile-preference-001
./mnemon-harness audit --root "$tmpdir" list
```

Apply path 会写入 profile state 和 audit record。Host tool 不应该直接修改 canonical state。

## 4. Goals And Evidence

```sh
./mnemon-harness goal --root "$tmpdir" init \
  --goal-id beta-smoke \
  --objective "Exercise the public beta"

./mnemon-harness goal --root "$tmpdir" plan \
  --goal-id beta-smoke \
  --summary "Run no-model checks" \
  --step init \
  --step verify

./mnemon-harness goal --root "$tmpdir" evidence append \
  --goal-id beta-smoke \
  --evidence-id evidence-beta-smoke \
  --type verification \
  --status accepted \
  --summary "Lifecycle smoke completed"

./mnemon-harness goal --root "$tmpdir" verify \
  --goal-id beta-smoke \
  --gate no-model-smoke \
  --summary "Smoke passed"
```

## 5. Coordination And TUI

Coordination 被表示为 event 和 governed proposal，而不是 chat log。

```sh
./mnemon-harness supervisor --root "$tmpdir" context --format json
./mnemon-harness supervisor --root "$tmpdir" propose --kind rule
./mnemon-harness ui --root "$tmpdir"
```

使用 TUI 检查 hosts、evidence、proposals、profile、coordination 和 trace link，然后再 apply 变更。
