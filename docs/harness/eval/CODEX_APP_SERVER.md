# Codex App-Server Eval

Codex app-server is the current reference HostAgent runner for Mnemon's
LLM-supervised lifecycle jobs. It lets Mnemon run semantic work through the host
agent instead of embedding a new LLM runtime inside the daemon.

The eval mode uses the real Codex app-server rather than a mock server. It
creates an isolated run directory under `.testdata`, projects Mnemon loop
templates into a generated workspace, then starts:

```bash
codex app-server --listen stdio://
```

In the lifecycle architecture, the same mechanism generalizes beyond eval:

```text
mnemon-daemon schedules job
        |
        v
Codex app-server starts HostAgent task
        |
        v
HostAgent reads job spec, GUIDE, state, recent events
        |
        v
LLM produces structured result
        |
        v
daemon validates result and records accepted events
```

Subagent markdown files such as `memory/subagents/dreaming.md`,
`skill/subagents/curator.md`, and `eval/subagents/evaluator.md` should be read
as portable lifecycle job specs. Claude Code may run them as native subagents;
Codex runs the same class of work through app-server tasks.

The default smoke flow sends JSON-RPC requests for `initialize`, `skills/list`,
and `thread/start`. This verifies that the real Codex app-server can read the
harness-injected `.codex` skills and `.mnemon` state:

```bash
make codex-app-eval
```

The memory/skill scenario suite starts real Codex turns and asserts loop
behavior:

```bash
make codex-app-eval-suite
```

The suite currently covers local-context memory skip, focused long-term recall,
durable `MEMORY.md` writes, transient no-pollution behavior, and skill evidence
logging.

For longer memory regression, run:

```bash
make codex-memory-deep-eval
```

The deep memory suite adds noisy recall filtering, stale-memory supersession,
uncertain-preference rejection, secret-like value rejection, and multi-turn
continuity through persisted `MEMORY.md`.

For longer skill regression, run:

```bash
make codex-skill-deep-eval
```

The deep skill suite adds transient evidence skip, missing-skill evidence,
approved active skill creation, host-surface preservation, and proposal-first
curation checks, plus reviewable skill authoring drafts.

To trigger a real Codex turn, opt in explicitly:

```bash
python3 scripts/codex_app_server_eval.py --agent-turn
```

A real turn uses local Codex authentication and may consume model credits.

Each run writes:

```text
.testdata/codex-app-eval/<timestamp>/
├── workspace/          # isolated project root seen by Codex
├── workspace/.codex/   # Codex host projection
├── .mnemon/            # Mnemon canonical harness state
├── logs/               # app-server stderr
└── reports/            # JSON eval report
```
