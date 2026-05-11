# Mnemon Memory Loop Harness

This directory is the first installable version of the memory loop harness. It is
agent-agnostic: a capable host agent can read these Markdown assets and install
the loop into its own runtime without a custom adapter.

## File Tree

```text
harness/memory-loop/
├── README.md
├── GUIDE.md
├── MEMORY.md
├── hooks/
│   ├── prime.md
│   ├── remind.md
│   ├── nudge.md
│   └── compact.md
├── skills/
│   ├── memory_get.md
│   └── memory_set.md
├── subagents/
│   └── dreaming.md
└── setup/
    └── claude-code/
        ├── install.sh
        ├── uninstall.sh
        ├── hooks/
        │   ├── prime.sh
        │   ├── remind.sh
        │   ├── nudge.sh
        │   └── compact.sh
        └── scripts/
            └── update_settings.py
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
| `GUIDE.md` | Policy: when to read memory, when to write memory, and what is worth keeping. |
| `hooks/*.md` | Four lifecycle reminders: Prime, Remind, Nudge, and Compact. |
| `skills/memory_get.md` | Online long-term recall skill backed by `mnemon recall`. |
| `skills/memory_set.md` | Online working-memory update skill backed by `MEMORY.md` edits. |
| `subagents/dreaming.md` | Offline consolidation worker backed by Mnemon writes and `MEMORY.md` compaction. |
| `setup/claude-code/` | First concrete setup implementation. It maps the harness onto Claude Code project or user config. |

## Runtime Directory Protocol

All reusable assets resolve their runtime files through one environment
variable:

```bash
MNEMON_MEMORY_LOOP_DIR=<host-agent-config>/mnemon-memory-loop
```

The directory must contain:

```text
$MNEMON_MEMORY_LOOP_DIR/
├── GUIDE.md
└── MEMORY.md
```

`memory_set.md`, `memory_get.md`, and `dreaming.md` should never hard-code a
Claude Code path. They should use `$MNEMON_MEMORY_LOOP_DIR` when it is available.
If the host runtime cannot pass environment variables to skills, the Prime hook
must inject the resolved path into the HostAgent context.

## Boundary

The harness does not provide a custom agent runtime. It provides Markdown
materials that a HostAgent can mount into its existing instruction, hook, skill,
and subagent systems.

The key split is:

```text
GUIDE.md decides when memory behavior is useful.
memory_get.md maps read-memory behavior to Mnemon recall.
memory_set.md maps write-memory behavior to MEMORY.md edits.
dreaming.md maps maintenance behavior to Mnemon write + MEMORY.md compaction.
```

## Claude Code Install

Install into the current project:

```bash
bash harness/memory-loop/setup/claude-code/install.sh
```

Install globally:

```bash
bash harness/memory-loop/setup/claude-code/install.sh --global
```

Remove the installed Claude Code integration while preserving `MEMORY.md`:

```bash
bash harness/memory-loop/setup/claude-code/uninstall.sh
```
