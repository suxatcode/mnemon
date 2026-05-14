# Mnemon Skill Loop Harness

This directory is the first installable version of the skill loop harness. It is
agent-agnostic: a host agent keeps its native skill runtime, while Mnemon owns
the canonical skill lifecycle state and the evidence used to evolve it.

## File Tree

```text
harness/skill-loop/
├── README.md
├── module.json
├── env.sh
├── GUIDE.md
├── hooks/
│   ├── prime.md
│   ├── remind.md
│   ├── nudge.md
│   └── compact.md
├── skills/
│   ├── skill_observe.md
│   ├── skill_curate.md
│   └── skill_manage.md
├── subagents/
│   └── curator.md
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
| HostAgent | Owns the ReAct loop, tool routing, native skill discovery, and subagent execution. |
| Host Skill Surface | The host-native skill directory, such as `.claude/skills`. It is a generated view. |
| Mnemon Skill Library | Canonical skill state under `mnemon-skill-loop/skills/{active,stale,archived}`. |

## Support Assets

| Asset | Purpose |
| --- | --- |
| `module.json` | Machine-readable loop manifest for standard lifecycle events, assets, state, and host adapters. |
| `env.sh` | Runtime config: canonical skill library, host skill surface, usage log, and proposal paths. |
| `GUIDE.md` | Policy for evidence, review triggers, lifecycle movement, and proposal-first changes. |
| `hooks/*.md` | Four lifecycle reminders. Prime syncs active skills; Nudge records evidence; Compact may trigger review; Remind is no-op by default. |
| `skills/skill_observe.md` | Online evidence capture protocol. |
| `skills/skill_curate.md` | Protocol for starting a curator review. |
| `skills/skill_manage.md` | Approved lifecycle mutation protocol. |
| `subagents/curator.md` | Background reviewer that proposes create, patch, consolidate, stale, archive, or restore actions. |
| `setup/claude-code/` | First concrete setup implementation for Claude Code. |

## Runtime Directory Protocol

Installed runtime files resolve through one environment config:

```text
$MNEMON_SKILL_LOOP_DIR/
├── env.sh
├── GUIDE.md
├── skills/
│   ├── active/
│   ├── stale/
│   ├── archived/
│   └── .usage.jsonl
└── proposals/
```

`env.sh` defines:

```bash
MNEMON_SKILL_LOOP_ENV=<host-agent-config>/mnemon-skill-loop/env.sh
MNEMON_SKILL_LOOP_DIR=<host-agent-config>/mnemon-skill-loop
MNEMON_SKILL_LOOP_HOST_SKILLS_DIR=<host-agent-config>/skills
MNEMON_SKILL_LOOP_ACTIVE_DIR=$MNEMON_SKILL_LOOP_DIR/skills/active
MNEMON_SKILL_LOOP_STALE_DIR=$MNEMON_SKILL_LOOP_DIR/skills/stale
MNEMON_SKILL_LOOP_ARCHIVED_DIR=$MNEMON_SKILL_LOOP_DIR/skills/archived
MNEMON_SKILL_LOOP_USAGE_FILE=$MNEMON_SKILL_LOOP_DIR/skills/.usage.jsonl
MNEMON_SKILL_LOOP_PROPOSALS_DIR=$MNEMON_SKILL_LOOP_DIR/proposals
```

Protocol skills should never hard-code a Claude Code path. They should resolve
state from these variables or from the path injected by Prime.

## Boundary

The harness does not replace the host skill runtime. It only maintains canonical
skill state and projects `active` skills into the host skill surface at Prime.

The key split is:

```text
GUIDE.md decides when skill evolution behavior is useful.
skill_observe.md records evidence only.
curator.md reviews evidence and proposes changes.
skill_manage.md applies approved changes to canonical state.
prime.sh projects active canonical skills into the host skill surface.
```

## Claude Code Install

Install into the current project:

```bash
bash harness/skill-loop/setup/claude-code/install.sh
```

Install globally:

```bash
bash harness/skill-loop/setup/claude-code/install.sh --global
```

Remove the installed Claude Code integration while preserving the canonical
skill library:

```bash
bash harness/skill-loop/setup/claude-code/uninstall.sh
```
