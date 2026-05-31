# Mnemon Harness Usage

These commands assume you built:

```sh
go build -o mnemon .
go build -o mnemon-harness ./harness/cmd/mnemon-harness
```

Use a temporary root while exploring.

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

Preview before writing to a project:

```sh
./mnemon-harness loop validate
./mnemon-harness loop diff --host codex --loop memory --project-root .
```

Install a projection only after reviewing the diff:

```sh
./mnemon-harness loop install --host codex --loop memory --project-root .
```

Projected files under `.codex/` or `.claude/` are host surfaces. The host can
read `PROJECTION.json` and echo `projection_ref` plus `context_digest` on later
writeback events. The harness uses that echo to distinguish observed, mismatch,
unattributed, silent, and stale host behavior.

## 3. Profile And Governance

Add a reviewed profile entry through the governed proposal route:

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

The apply path writes profile state and audit records. Direct mutation should be
kept out of host tools.

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

Coordination is represented as events and governed proposals, not chat logs.

```sh
./mnemon-harness supervisor --root "$tmpdir" context --format json
./mnemon-harness supervisor --root "$tmpdir" propose --kind rule
./mnemon-harness ui --root "$tmpdir"
```

Use the TUI to inspect hosts, evidence, proposals, profile, coordination, and
trace links before applying changes.
