# Mnemon Harness

Mnemon Harness is the formal documentation entry for Mnemon's modular
self-evolution harness.

Mnemon is built around a memory-driven principle: durable agents should turn
experience into governed long-term state, then use that state to improve future
behavior.

Mnemon is not trying to replace an agent runtime. It attaches external evolution
loops to an existing host agent through standard extension points such as hooks,
skills, subagents, filesystem assets, and environment configuration.

The key assumption is that many behavior-level agent capabilities can be
externalized when the host already has a ReAct loop and readable extension
surfaces. Mnemon packages those capabilities as harness modules instead of
building another runtime.

Mnemon is also not only a set of skills. It owns a harness runtime substrate:
module layout, setup, environment, state, reports, proposals, locks, queues,
projection into host surfaces, and optional daemon scheduling.

## Core Positioning

| Topic | Design |
| --- | --- |
| Modular Agent Harness | [EN](modular-agent/DESIGN.md) / [中文](../zh/harness/modular-agent/DESIGN.md) |
| Loop Module Standard | [EN](LOOP_MODULE_STANDARD.md) / [中文](../zh/harness/LOOP_MODULE_STANDARD.md) |
| Harness Roadmap | [EN](ROADMAP.md) / [中文](../zh/harness/ROADMAP.md) |
| Memory Loop | [EN](memory-loop/DESIGN.md) / [中文](../zh/harness/memory-loop/DESIGN.md) / [site](../site/memory-loop/site.html) |
| Skill Loop | [EN](skill-loop/DESIGN.md) / [中文](../zh/harness/skill-loop/DESIGN.md) / [site](../site/skill-loop/site.html) |

## Installable Assets

| Harness Module | Implementation |
| --- | --- |
| Memory Loop | [harness/memory-loop](../../harness/memory-loop/README.md) |
| Skill Loop | [harness/skill-loop](../../harness/skill-loop/README.md) |

## Vocabulary

| Concept | Meaning |
| --- | --- |
| loop module | Standard package shape for one attachable harness loop. |
| GUIDE | Markdown policy for deciding when a loop should act. |
| setup | Installation and mounting into a host agent. |
| hook | Host lifecycle timing such as Prime, Remind, Nudge, and Compact. |
| protocol | Markdown skills that define reusable operations. |
| subagent | Background maintenance agent for heavier review or consolidation. |
| projection | Host-specific rendering of canonical loop assets into `.claude`, `.codex`, or another runtime surface. |
| daemon | Optional harness maintenance runner for scheduled module work. |
| substrate | Mnemon-owned runtime base for module state, setup, projection, scheduling, and cross-module protocols. |

## Boundary

The host agent keeps the ReAct loop, prompt assembly, tool routing, native skill
runtime, permission model, and UI. Mnemon provides attachable harness modules
that make the host agent more durable and self-improving.

In short: the host agent is the execution runtime; Mnemon is the harness runtime
substrate.

Claude Code is the first reference host because it exposes hooks, skills, and
subagents. The architecture is intentionally broader than Claude Code.

`mnemon-daemon` may later provide a background maintenance runner for harness
modules. It is part of the harness layer, not a host agent runtime.
