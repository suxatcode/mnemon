# Memory Loop MVP Design

This document describes the first implementation slice of the memory loop. The goal is to keep the harness small: install a few hook prompts and Markdown-based capabilities around an existing host agent, while using Mnemon as the long-term memory backend.

Related visualization: [site/index.html](./site/index.html)

Chinese version: [README.zh.md](./README.zh.md)

Reference implementation: [harness/memory-loop](../../../../harness/memory-loop)

## Core Model

The MVP has three core parts:

| Part | Role | Boundary |
| --- | --- | --- |
| HostAgent | The host agent runtime. It runs the task, receives hook injections, and decides whether to load a memory skill or spawn the dreaming subagent. | It does not own memory storage protocols. |
| MEMORY.md | The working memory file. It is small, prompt-facing, and loaded into the system prompt at Prime. | It is maintained by `memory_set.md` and the dreaming subagent. |
| Mnemon | The long-term memory store and binary. It is installed separately, for example with `brew install`. | It is accessed through `memory_get.md` and the dreaming subagent protocol. |

Everything else is a support asset around these three parts.

## Maintained Assets

The first version should maintain the following assets:

| Asset | Kind | Purpose |
| --- | --- | --- |
| `env.sh` | Config | Defines `MNEMON_MEMORY_LOOP_ENV`, `MNEMON_MEMORY_LOOP_DIR`, and memory-size threshold variables. |
| `GUIDE.md` | Manual | Describes when to read memory, when to write memory, and what kind of information is worth keeping. |
| Claude Code setup scripts | Setup | First concrete installation path. It installs project/user Claude Code hooks, skills, subagent, and memory files. |
| Prime hook | Hook | Loads `MEMORY.md` and `GUIDE.md` into the system prompt. |
| Remind hook | Hook | Reminds the HostAgent to decide whether memory should be read. |
| Nudge hook | Hook | Reminds the HostAgent to decide whether memory should be accumulated. |
| Compact hook | Hook | Reminds the HostAgent to preserve important information before context compaction. |
| `memory_get.md` | Skill | Defines how to recall long-term memory from Mnemon. |
| `memory_set.md` | Skill | Defines how to edit `MEMORY.md`. |
| dreaming subagent spec | Subagent | Defines how to consolidate `MEMORY.md` into Mnemon and compact or evict working memory entries. |

## Policy And Implementation Split

`GUIDE.md` is intentionally abstract. It should describe memory behavior, not storage mechanics.

It should answer questions like:

- Should the agent read memory now?
- Should the agent write memory now?
- Is this information stable enough to keep?
- Is this a durable preference, project convention, or reusable fact?

It should not require the HostAgent to decide whether the target is `MEMORY.md` or Mnemon. That decision is pushed into the capability layer. Reusable capabilities locate their runtime directory through `MNEMON_MEMORY_LOOP_DIR`.

- `memory_get.md` maps read-memory behavior to Mnemon recall.
- `memory_set.md` maps write-memory behavior to `$MNEMON_MEMORY_LOOP_DIR/MEMORY.md` edits.
- The dreaming subagent maps consolidation behavior to Mnemon write plus `$MNEMON_MEMORY_LOOP_DIR/MEMORY.md` compaction.

This split keeps the guide portable across different host agents.

## Runtime Flow

### Prime

Prime is the only direct loading path.

Inputs:

- `MEMORY.md`
- `GUIDE.md`

Action:

- Inject both into the HostAgent system prompt.

Boundary:

- Prime does not call `memory_get.md`.
- Prime does not recall Mnemon.
- Prime does not write long-term memory.

### Remind / Recall

Remind creates the opportunity to read memory.

Flow:

1. Remind asks the HostAgent to judge whether memory should be read according to `GUIDE.md`.
2. If yes, the HostAgent loads `memory_get.md`.
3. `memory_get.md` explains how to call Mnemon recall.
4. Mnemon returns bounded recall context to the HostAgent.

Boundary:

- Long-term memory is not fully injected.
- Recall results are not automatically written back to `MEMORY.md`.
- `GUIDE.md` does not need to know Mnemon protocol details.

### Nudge / Accumulate

Nudge creates the opportunity to write working memory.

Flow:

1. Nudge asks the HostAgent to judge whether memory should be accumulated according to `GUIDE.md`.
2. If yes, the HostAgent loads `memory_set.md`.
3. `memory_set.md` explains how to add, replace, or remove entries in `MEMORY.md`.

Boundary:

- Online memory accumulation writes only to `MEMORY.md`.
- It does not directly write Mnemon.
- It should avoid transcripts, one-off progress, and low-confidence observations.

### Compact

Compact is a boundary-time version of Nudge.

Flow:

1. Before context compaction, Compact asks the HostAgent to judge whether important information may be lost.
2. If yes, the HostAgent loads `memory_set.md`.
3. `memory_set.md` writes the necessary final patch into `MEMORY.md`.

Boundary:

- Compact is not dreaming.
- Compact does not perform full working memory cleanup.
- Compact does not write long-term memory directly.

### Dreaming

Dreaming is a maintenance process, not a normal online hook.

Flow:

1. The HostAgent spawns a dedicated dreaming subagent.
2. The subagent reads the full `MEMORY.md`.
3. The subagent writes the current working memory into Mnemon using the Mnemon protocol.
4. The subagent compacts, organizes, or evicts entries in `MEMORY.md`.

Possible triggers:

- `MEMORY.md` exceeds quota.
- Before context compaction.
- Manual user or HostAgent request.

Boundary:

- Dreaming is responsible for consolidation and cleanup.
- It does not replace Remind, Nudge, or Compact.
- It should preserve prompt-facing usefulness while moving durable information into long-term memory.

## First-Version Scope

The MVP should include:

- A minimal `GUIDE.md`.
- Claude Code setup scripts that mount Prime, Remind, Nudge, and Compact into `.claude/settings.json`.
- A `MEMORY.md` template.
- A `memory_get.md` skill for Mnemon recall.
- A `memory_set.md` skill for `MEMORY.md` edits.
- A dreaming subagent spec.
- Clear assumptions that Mnemon is installed separately as the binary and long-term store.

The MVP should not include:

- A custom agent runtime.
- A complex adapter framework.
- A second working-memory format.
- A direct long-term-memory write path from normal online hooks.

## Design Principle

The harness should remain agent-agnostic. It gives a host agent the materials needed to install memory behavior into itself:

- manuals for rules and scripts for installation;
- hooks for timing;
- skills for online memory operations;
- a subagent for offline consolidation;
- Mnemon for long-term storage.

This keeps the first version implementable while preserving the intended memory loop: `MEMORY.md` provides prompt-facing working memory, Mnemon provides durable long-term memory, and dreaming moves information between them.
