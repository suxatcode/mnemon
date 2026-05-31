# Mnemon Memory Loop Harness

This directory is the canonical memory loop template. It is host-agnostic: a
capable host agent can read these Markdown assets, while host adapters project
the loop into concrete runtimes such as Claude Code or Codex.

## File Tree

```text
harness/loops/memory/
├── README.md
├── loop.json
├── env.sh
├── GUIDE.md
├── MEMORY.md
├── hook-prompts/
│   ├── prime.md
│   ├── remind.md
│   ├── nudge.md
│   └── compact.md
├── skills/
│   ├── memory-get/
│   │   └── SKILL.md
│   └── memory-set/
│       └── SKILL.md
├── subagents/
│   └── dreaming.md
```

## Core Parts

| Part | Role |
| --- | --- |
| HostAgent | The host agent runtime. It owns task execution, model judgment, and native hook/skill/subagent mechanisms. |
| `MEMORY.md` | Prompt-facing working memory. It is loaded at Prime and kept compact. |
| Mnemon | Long-term memory binary and store. It is installed separately and accessed through skill/subagent protocols. |

## Support Assets

| Asset | Purpose |
| --- | --- |
| `loop.json` | Machine-readable loop manifest for standard lifecycle events, assets, state, and host adapters. |
| `env.sh` | Runtime config: memory directory, env path, and dreaming threshold. |
| `GUIDE.md` | Policy: when to read memory, when to write memory, and what is worth keeping. |
| `hook-prompts/*.md` | Four lifecycle reminders: Prime, Remind, Nudge, and Compact. |
| `skills/memory-get/SKILL.md` | Online long-term recall skill backed by `mnemon recall`. |
| `skills/memory-set/SKILL.md` | Online working-memory update skill backed by `MEMORY.md` edits. |
| `subagents/dreaming.md` | Offline consolidation worker backed by Mnemon writes and `MEMORY.md` compaction. |
| Host adapter | Host-specific projection lives outside the loop under `harness/hosts/<host>/`. |

## Runtime Directory Protocol

All reusable assets resolve their runtime files through one environment
config file and environment variables:

```text
$MNEMON_MEMORY_LOOP_DIR/
├── env.sh
├── GUIDE.md
└── MEMORY.md
```

`env.sh` defines:

```bash
MNEMON_MEMORY_LOOP_ENV=<canonical-state>/harness/memory/env.sh
MNEMON_MEMORY_LOOP_DIR=<canonical-state>/harness/memory
MNEMON_MEMORY_LOOP_MAX_NON_EMPTY_LINES=200
```

`memory-set`, `memory-get`, and `dreaming.md` should never hard-code a
Claude Code path. They should use `$MNEMON_MEMORY_LOOP_DIR` when it is available.
If the host runtime cannot pass environment variables to skills, the Prime hook
must inject the resolved path into the HostAgent context.

`MNEMON_MEMORY_LOOP_MAX_NON_EMPTY_LINES` controls when hook prompts should suggest
`mnemon-dreaming` for an oversized `MEMORY.md`.

## Boundary

The harness does not provide a custom agent runtime. It provides Markdown
materials that a HostAgent can mount into its existing instruction, hook, skill,
and subagent systems.

The key split is:

```text
GUIDE.md decides when memory behavior is useful.
memory-get maps read-memory behavior to Mnemon recall.
memory-set maps write-memory behavior to MEMORY.md edits.
dreaming.md maps maintenance behavior to Mnemon write + MEMORY.md compaction.
```

## Claude Code Install

Install into the current project:

```bash
bash harness/ops/install.sh --host claude-code --loop memory
```

Install globally:

```bash
bash harness/ops/install.sh --host claude-code --loop memory --global
```

Remove the installed Claude Code integration while preserving `MEMORY.md`:

```bash
bash harness/ops/uninstall.sh --host claude-code --loop memory
```
