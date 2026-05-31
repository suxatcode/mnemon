# Mnemon Harness

`mnemon-harness` is an experimental beta layer for connecting host agents to
project-local Mnemon state.

It is separate from the stable `mnemon` CLI. Stable Mnemon stores and recalls
memory. The harness adds a governed agent-state substrate around host agents:
events, projected context, readback verification, proposals, apply, audit, and
coordination topology.

The current beta is source-build only, not production-ready, and has no
compatibility guarantee. Commands, file layouts, schemas, projected surfaces,
and behavior may change in breaking ways before a stable release.

## Mental Model

```text
host agent lifecycle
        |
        v
Lifecycle Exchange
  context out: projection files under .codex/.claude/...
  signal in:   events written to .mnemon/events.jsonl
        |
        v
governed agent-state substrate
  eventlog + profile + goals + proposals + audit + coordination
        |
        v
next host run inherits reviewed state
```

Host directories such as `.codex` and `.claude` are projection surfaces, not
canonical state. The event log and governed records under `.mnemon/` are the
source of truth.

## What Works In This Beta

- project-local lifecycle event log
- Codex and Claude Code projection surfaces
- projection envelope and readback verification
- profile entries projected back into host context
- goal, eval, proposal, apply, and audit commands
- coordination topology events and governed coordination apply
- a TUI for evidence, hosts, proposals, profile, coordination, and trace review
- a Codex runner path behind explicit checks and cost gates

This is not a production multi-agent runtime. Auto-apply, broad org/team scope
composition, and production-grade autonomous coordination are not promised by
this beta.

## Build

From the repository root:

```sh
go build -o mnemon .
go build -o mnemon-harness ./harness/cmd/mnemon-harness
```

Validate harness declarations:

```sh
make harness-validate
```

## Try The Harness

Initialize a temporary project and append a no-model event:

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

Install projected context into a real project only after reviewing the diff:

```sh
./mnemon-harness loop validate
./mnemon-harness loop diff --host codex --loop memory --project-root .
./mnemon-harness loop install --host codex --loop memory --project-root .
```

More command examples are in `docs/harness/USAGE.md`.
