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
surfaces. Mnemon packages those capabilities as harness loops instead of
building another runtime.

Mnemon is also not only a set of skills. It owns a harness runtime substrate:
loop layout, ops, environment, state, reports, proposals, locks, queues,
projection into host surfaces, and optional daemon scheduling.

## Core Positioning

| Topic | Design |
| --- | --- |
| Modular Agent Harness | [EN](modular-agent/DESIGN.md) / [中文](../zh/harness/modular-agent/DESIGN.md) |
| Loop Standard | [EN](LOOP_STANDARD.md) / [中文](../zh/harness/LOOP_STANDARD.md) |
| Host Projection | [EN](HOST_PROJECTION.md) / [中文](../zh/harness/HOST_PROJECTION.md) |
| Harness Roadmap | [EN](ROADMAP.md) / [中文](../zh/harness/ROADMAP.md) |
| YC Evolving Design Philosophy | [EN](YC_EVOLVING_DESIGN_PHILOSOPHY.md) / [中文](../zh/harness/YC_EVOLVING_DESIGN_PHILOSOPHY.md) |
| Lifecycle Control Plane | [EN](LIFECYCLE_CONTROL_PLANE.md) / [中文](../zh/harness/LIFECYCLE_CONTROL_PLANE.md) / [site](../site/lifecycle-control-plane/index.html) |
| AI-Native Lifecycle Runtime | [EN](LIFECYCLE_RUNTIME.md) / [中文](../zh/harness/LIFECYCLE_RUNTIME.md) / [site](../site/lifecycle-runtime/index.html) |
| System Flow | [EN](SYSTEM_FLOW.md) / [中文](../zh/harness/SYSTEM_FLOW.md) / [site](../site/system-flow/index.html) |
| Memory Loop | [EN](memory/DESIGN.md) / [中文](../zh/harness/memory/DESIGN.md) / [site](../site/memory/index.html) |
| Skill Loop | [EN](skill/DESIGN.md) / [中文](../zh/harness/skill/DESIGN.md) / [site](../site/skill/index.html) |
| Eval Loop | [EN](eval/DESIGN.md) / [中文](../zh/harness/eval/DESIGN.md) |

## Installable Assets

| Harness Loop | Implementation |
| --- | --- |
| Memory Loop | [harness/loops/memory](../../harness/loops/memory/README.md) |
| Skill Loop | [harness/loops/skill](../../harness/loops/skill/README.md) |
| Eval Loop | [harness/loops/eval](../../harness/loops/eval/README.md) |

## Repository Layout

| Directory | Role |
| --- | --- |
| `harness/loops/` | Canonical host-agnostic loop templates. |
| `harness/hosts/` | Host projection adapters such as Claude Code and future Codex support. |
| `harness/bindings/` | Loop x host binding definitions. |
| `harness/control/` | Shared control-plane contracts. |
| `harness/ops/` | Shared install, status, and uninstall entrypoints that compose loops with hosts. |

## Vocabulary

| Concept | Meaning |
| --- | --- |
| loop template | Standard package shape for one attachable harness loop. |
| GUIDE | Markdown policy for deciding when a loop should act. |
| ops | Installation, status, validation, and uninstall operations. |
| hook | Host lifecycle timing such as Prime, Remind, Nudge, and Compact. |
| protocol | Markdown skills that define reusable operations. |
| subagent | Background maintenance agent for heavier review or consolidation. |
| projection | Host-specific rendering of canonical loop assets into `.claude`, `.codex`, or another runtime surface. |
| host manifest | Machine-readable record of projected loops, paths, lifecycle mappings, and host capabilities. |
| daemon | Optional harness maintenance runner for scheduled loop work. |
| substrate | Mnemon-owned runtime base for loop state, ops, projection, scheduling, and cross-loop protocols. |
| system flow | End-to-end feedback path from a bare HostAgent through bootstrap, hooks, daemon reconcile, `.mnemon` state, and host projection. |

## Boundary

The host agent keeps the ReAct loop, prompt assembly, tool routing, native skill
runtime, permission model, and UI. Mnemon provides attachable harness loops
that make the host agent more durable and self-improving.

In short: the host agent is the execution runtime; Mnemon is the harness runtime
substrate.

Claude Code is the first reference host because it exposes hooks, skills, and
subagents. The architecture is intentionally broader than Claude Code.

`mnemon-daemon` may later provide a background maintenance runner for harness
loops. It is part of the harness layer, not a host agent runtime.
