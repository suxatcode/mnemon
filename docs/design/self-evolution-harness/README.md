# Self-Evolution Harness Design Archive

This directory keeps historical v0.2 architecture context and condensed research
material for the Mnemon self-evolution harness.

The current formal harness documentation lives in [docs/harness](../../harness/README.md).

## Current Harness Docs

| Topic | Design |
| --- | --- |
| Modular Agent Harness | [EN](../../harness/modular-agent/DESIGN.md) / [中文](../../harness/modular-agent/DESIGN.zh.md) |
| Memory Loop | [EN](../../harness/memory-loop/DESIGN.md) / [中文](../../harness/memory-loop/DESIGN.zh.md) / [site](../../harness/memory-loop/site/index.html) |
| Skill Loop | [EN](../../harness/skill-loop/DESIGN.md) / [中文](../../harness/skill-loop/DESIGN.zh.md) / [site](../../harness/skill-loop/site/index.html) |

The loop MVP uses the same harness vocabulary:

| Concept | Meaning |
| --- | --- |
| GUIDE | Markdown policy for deciding when a loop should act. |
| setup | Installation and mounting into a host agent. |
| hook | Host lifecycle timing: Prime, Remind, Nudge, and Compact. |
| protocol | Markdown skills that define reusable operations. |
| subagent | Background maintenance agent for heavier review or consolidation. |

## Architecture Context

- [SELF_EVOLUTION_HARNESS.md](SELF_EVOLUTION_HARNESS.md) is the broader historical v0.2 harness architecture.
- [research/agent-systems/README.md](research/agent-systems/README.md) records condensed research references.

The current loop-specific pages are intentionally narrower. They document the
first practical MVP slice rather than the full future architecture.
