# Mnemon Harness Public Beta

`mnemon-harness` is an experimental beta layer for attaching host agents to
project-local governed state. It is source-build only and intentionally separate
from the stable `mnemon` CLI.

Stable Mnemon remains a memory and recall tool. The harness adds lifecycle
exchange, evidence, proposals, audit, coordination topology, and a review TUI
around host agents such as Codex and Claude Code.

## 1. What It Is

Mnemon Harness is a governed agent-state substrate.

```text
host agent
  <-> Lifecycle Exchange
      context out: .codex/.claude projection files
      signal in:   .mnemon/events.jsonl
  <-> governed project state
      profile + goals + proposals + audit + coordination
```

The host directories are projection surfaces. Canonical state lives in the
append-only event log and governed records under `.mnemon/`.

## 2. Current Beta Surface

The public beta includes:

- lifecycle event append/status/daemon commands
- Codex and Claude Code projection surfaces
- projection envelope and readback verification
- profile projection into host context
- goal, eval, proposal, apply, and audit commands
- coordination topology and governed coordination apply
- TUI views for hosts, evidence, proposals, profile, coordination, and traces
- Codex runner checks behind explicit user action and cost gates

It does not promise production readiness, automatic apply, broad org/team scope
composition, or a full multi-agent runtime.

## 3. Separation From Stable Mnemon

`mnemon-harness` is built from `./harness/cmd/mnemon-harness`.

The stable `mnemon` binary does not import harness packages. It exposes only a
small default-off event seam so a project can write events that the harness may
later read.

```sh
MNEMON_HARNESS_EVENT_EMIT=1 mnemon remember "..." --cat note
mnemon event emit custom.observed --payload '{"ok":true}'
```

Without the opt-in environment variable or explicit `mnemon event` command,
stable Mnemon behavior is unchanged.

## 4. Try It

Build both binaries:

```sh
go build -o mnemon .
go build -o mnemon-harness ./harness/cmd/mnemon-harness
```

Run the no-model smoke path:

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

See [USAGE.md](USAGE.md) for command examples.

## 5. Release Boundary

This beta intentionally ships minimal public documentation. Internal planning,
internal validation artifacts, generated site HTML, and detailed future plans are
not part of this branch.
