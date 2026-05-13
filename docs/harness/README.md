# Mnemon Harness

Mnemon Harness is the formal documentation entry for Mnemon's modular
self-evolution harness.

Mnemon is not trying to replace an agent runtime. It attaches external evolution
loops to an existing host agent through standard extension points such as hooks,
skills, subagents, filesystem assets, and environment configuration.

## Core Positioning

| Topic | Design |
| --- | --- |
| Modular Agent Harness | [EN](modular-agent/DESIGN.md) / [中文](modular-agent/DESIGN.zh.md) |
| Memory Loop | [EN](memory-loop/DESIGN.md) / [中文](memory-loop/DESIGN.zh.md) / [site](memory-loop/site/index.html) |
| Skill Loop | [EN](skill-loop/DESIGN.md) / [中文](skill-loop/DESIGN.zh.md) / [site](skill-loop/site/index.html) |

## Installable Assets

| Harness Module | Implementation |
| --- | --- |
| Memory Loop | [harness/memory-loop](../../harness/memory-loop/README.md) |
| Skill Loop | [harness/skill-loop](../../harness/skill-loop/README.md) |

## Vocabulary

| Concept | Meaning |
| --- | --- |
| GUIDE | Markdown policy for deciding when a loop should act. |
| setup | Installation and mounting into a host agent. |
| hook | Host lifecycle timing such as Prime, Remind, Nudge, and Compact. |
| protocol | Markdown skills that define reusable operations. |
| subagent | Background maintenance agent for heavier review or consolidation. |

## Boundary

The host agent keeps the ReAct loop, prompt assembly, tool routing, native skill
runtime, permission model, and UI. Mnemon provides attachable harness modules
that make the host agent more durable and self-improving.

Claude Code is the first reference host because it exposes hooks, skills, and
subagents. The architecture is intentionally broader than Claude Code.

Historical v0.2 architecture context remains in
[docs/design/self-evolution-harness](../design/self-evolution-harness/README.md).
