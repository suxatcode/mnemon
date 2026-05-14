# Mnemon Harness Eval

This directory documents eval modes for host-wrapped loop testing.

## Codex App-Server Eval

The Codex app-server eval uses the real Codex app-server protocol instead of a
mock server. It creates an isolated run directory under `.testdata`, installs
Mnemon loop modules into a generated workspace, starts:

```bash
codex app-server --listen stdio://
```

Then it sends JSON-RPC requests for `initialize`, `skills/list`, and
`thread/start`. The default path is a smoke check that does not start a model
turn:

```bash
make codex-app-eval
```

Run the real memory/skill scenario suite with:

```bash
make codex-app-eval-suite
```

To run an actual Codex turn, use:

```bash
python3 scripts/codex_app_server_eval.py --agent-turn
```

The real turn may use the local Codex authentication and consume model credits.
Each run writes a JSON report and app-server stderr log under:

```text
.testdata/codex-app-eval/<timestamp>/
```

## Isolation Model

Each eval run has:

- `workspace/`: a throwaway project root read by Codex
- `workspace/.codex/`: projected Codex skills
- `.mnemon/`: canonical Mnemon harness state
- `logs/`: app-server logs
- `reports/`: machine-readable eval reports

## Scenario Suite

The default suite covers:

- `memory-skip-local`: visible workspace context should not trigger recall
- `memory-focused-recall`: relevant seeded long-term memory should be recalled
- `memory-write-decision`: durable decisions should update `MEMORY.md`
- `memory-no-pollution`: transient tokens should not be stored
- `skill-observe-evidence`: reusable workflow evidence should append JSONL
