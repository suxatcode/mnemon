# Mnemon Harness Install Guide

> Installable artifact derived from [HARNESS.md](HARNESS.md). Give this file to
> the target agent and ask it to install Mnemon into its own runtime.

## Goal

Install Mnemon as a lightweight memory harness:

```text
SKILL.md teaches commands.
GUIDELINE.md teaches judgment.
Hooks remind at lifecycle boundaries.
mnemon executes deterministic memory operations.
```

Do not build a custom adapter unless the runtime truly needs automation. A
capable agent should map these instructions onto its own native mechanisms.

## Prerequisites

Verify that the `mnemon` binary is available:

```bash
mnemon --version
```

If missing, install it with a supported project method, for example:

```bash
brew install mnemon-dev/tap/mnemon
```

or:

```bash
go install github.com/mnemon-dev/mnemon@latest
```

## Install Steps

1. Install `SKILL.md` into the runtime's skill, rule, command, or instruction
   mechanism.
2. Install `GUIDELINE.md` where the runtime can read it at session start and
   before memory-sensitive decisions.
3. Configure a project-scoped Mnemon store unless the user explicitly asks for a
   global store.
4. Add the four hook phases when the runtime supports hooks.
5. If hooks are unavailable, encode the same phase checks as persistent rules.
6. Run the verification checklist below.

## Hook Phases

Each hook may simply emit a short natural-language reminder. Hook scripts should
not force memory operations.

| Phase | Runtime Moment | Required Reminder |
|---|---|---|
| Prime | Session start / bootstrap | Load Mnemon skill, guideline, and active store info |
| Remind | User prompt submit / before planning | Decide whether recall could change this task |
| Nudge | Stop / after response | Decide whether durable writeback is justified |
| Compact | Before context compaction | Preserve only critical continuity |

If the runtime supports only some hook moments, install the available ones and
keep the missing checks in persistent instructions.

## Runtime Mapping Examples

Use the closest native equivalent:

| Runtime | Installation Target |
|---|---|
| Codex | `AGENTS.md`, skills, local instructions, and hooks when enabled |
| Claude Code | `CLAUDE.md`, skills, slash commands, settings hooks, project/user memory |
| OpenClaw | Plugin hooks and skills |
| Skill-first agents | Skills, memory guidance, and lightweight reminders |
| Minimal CLI | A rule file or system instruction that references the skill and guideline |

These mappings are examples. Preserve the behavior contract even if paths or
file names differ.

## Verification

The installation is acceptable when the agent can:

1. Explain when Mnemon recall is useful and when it should be skipped.
2. Run `mnemon recall "<focused query>" --limit 5` for a relevant task.
3. Write one durable memory with provenance.
4. Skip memory for a trivial task.
5. Preserve only critical continuity before compaction if the runtime exposes
   that event.

If memory is used on every prompt, if ordinary chat is saved as memory, or if
stale memory overrides current user instructions and repository facts, the
installation is not acceptable.
