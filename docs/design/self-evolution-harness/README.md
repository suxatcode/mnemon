# Self-Evolution Harness Design

This directory contains the design materials for the Mnemon self-evolution harness.

The current MVP is split into two loop designs. Both use the same harness vocabulary:

| Concept | Meaning |
| --- | --- |
| GUIDE | Markdown policy for deciding when a loop should act. |
| setup | Installation and mounting into a host agent. |
| hook | Host lifecycle timing: Prime, Remind, Nudge, and Compact. |
| protocol | Markdown skills that define reusable operations. |
| subagent | Background maintenance agent for heavier review or consolidation. |

## Loop Designs

| Loop | Design | Visualization |
| --- | --- | --- |
| Memory Loop | [EN](memory-loop/DESIGN.md) / [中文](memory-loop/DESIGN.zh.md) | [memory-loop/site/index.html](memory-loop/site/index.html) |
| Skill Loop | [EN](skill-loop/DESIGN.md) / [中文](skill-loop/DESIGN.zh.md) | [skill-loop/site/index.html](skill-loop/site/index.html) |

## Architecture Context

- [SELF_EVOLUTION_HARNESS.md](SELF_EVOLUTION_HARNESS.md) is the broader v0.2 harness architecture.
- [research/agent-systems/README.md](research/agent-systems/README.md) records condensed research references.

The loop-specific pages are intentionally narrower. They document the first practical MVP slice rather than the full future architecture.
