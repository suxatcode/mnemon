# Modular Agent Harness Design

Mnemon's main advantage is the modular agent model: self-evolution should be an
external harness that can attach to existing agents, not a new agent framework
that replaces them.

## Thesis

Any host agent that supports standard extension points can gain self-evolution
capabilities by installing Mnemon harness modules.

The host agent owns the ReAct loop:

```text
observe context -> reason -> call tools -> inspect results -> continue or stop
```

Mnemon attaches additional loops around that runtime:

```text
Memory Loop: experience -> working memory -> long-term memory -> recall
Skill Loop: repeated workflow -> evidence -> proposal -> skill lifecycle
Future Loops: evaluation, risk review, safety checks, benchmark feedback
```

## Host And Harness Split

| Layer | Owner | Responsibility |
| --- | --- | --- |
| ReAct loop | Host agent | Task execution, planning, tool calls, verification, user interaction. |
| Prompt assembly | Host agent | Decides which context enters the model. |
| Tool routing | Host agent | Chooses and executes tools under the host permission model. |
| Native skills | Host agent | Discovers and invokes skills using the host's own runtime. |
| Evolution modules | Mnemon harness | Adds memory, skill evolution, evaluation, and review loops through attachable assets. |
| Canonical state | Mnemon harness | Stores durable memory, skill lifecycle state, evidence, proposals, and reports. |

This split keeps Mnemon portable. A host can adopt one module without adopting a
new runtime.

## Standard Integration Surface

| Primitive | Harness Use |
| --- | --- |
| Hooks | Install lifecycle nudges at Prime, Remind, Nudge, Compact, or equivalent host events. |
| Skills | Expose reusable protocol operations such as `memory_get`, `memory_set`, `skill_observe`, and `skill_manage`. |
| Subagents | Run heavier maintenance jobs such as dreaming and curator review outside the online task path. |
| Filesystem | Store canonical module state in predictable directories and project/user scopes. |
| Environment | Let protocol skills resolve paths without hard-coding a specific host agent. |

The minimal requirement is a hook-like lifecycle mechanism. Skills and subagents
make the integration cleaner, but a capable agent can also follow the Markdown
protocols directly.

## Current Modules

| Module | Purpose | Current Reference Host |
| --- | --- | --- |
| Memory Loop | Adds working memory, long-term memory, and dreaming consolidation. | Claude Code setup under `harness/memory-loop/setup/claude-code`. |
| Skill Loop | Adds active/stale/archived skill lifecycle, evidence capture, curator proposals, and approved lifecycle mutation. | Claude Code setup under `harness/skill-loop/setup/claude-code`. |

## Memory Differentiator

The memory module uses a hot/cold memory model:

- Working memory is model-friendly. It is small Markdown context loaded into the
  prompt and maintained by the agent.
- Long-term memory is engineering-friendly. Mnemon stores larger durable memory
  outside the prompt and recalls it on demand.
- Dreaming consolidates between them by writing durable working memory into
  Mnemon and compacting or evicting the prompt-facing working memory.

This keeps the best part of Markdown memory while avoiding the capacity ceiling
of a single always-loaded file.

## Future Modules

The same harness pattern can support more loops:

- Eval loop: collect outcomes, run benchmarks, and feed failures into proposals.
- Risk loop: scan proposed skill or memory changes before they become active.
- Review loop: coordinate human approval, checkpoints, and release gates.
- Policy loop: maintain host-specific safety and permission guidance.

Each module should remain independently installable.

## Non-Goals

- Do not replace the host agent runtime.
- Do not require one universal skill format.
- Do not inject all state into the prompt.
- Do not make self-modifying changes without explicit policy and review.

## Reference Case

Claude Code is the first modular-agent case because it already exposes hooks,
skills, subagents, filesystem configuration, and project/user scopes. A working
Claude Code setup proves the attachment model, but Mnemon's target is any host
agent with comparable extension points.
